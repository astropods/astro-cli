package handlers

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/peerdomain"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

const (
	flowsDefaultLimit = 50
	flowsMaxLimit     = 200
)

// deploymentContext holds the resolved deployment plus the Prometheus filter
// derived from it. Returned by resolveDeploymentContext for handlers that need
// to query metrics scoped to a single deployment's pods.
//
// Beyla doesn't expose arbitrary pod labels, so we scope by the two labels it
// always emits with k8s decoration enabled: k8s_namespace_name (per-account)
// and service_name (per-agent, derived from app.kubernetes.io/name). Together
// they uniquely identify a deployment's series.
type deploymentContext struct {
	Deployment    *deploymentstore.Deployment
	Namespace     string // k8s_namespace_name label value
	ServiceName   string // service_name label value (sanitized agent name)
	ClusterFilter string // ",cluster=\"X\"" or "" — append inside metric selectors
	// PromClient is the deployment's cluster's own Prometheus client when its
	// cluster has a prometheus_url override, otherwise the caller's default
	// client. May be nil if neither is configured. See
	// k8s.Registry.PrometheusClientFor.
	PromClient *promquery.Client
}

// resolveDeploymentContext validates auth, looks up the deployment, checks
// account membership, and computes the Prometheus filter pieces.
// Routes: /api/v1/deployments/:id/network/...
func resolveDeploymentContext(
	c *gin.Context,
	deploymentStore *deploymentstore.Store,
	accountStore *account.AccountStore,
	k8sReg *k8s.Registry,
	promClient *promquery.Client,
) (*deploymentContext, bool) {
	access, ok := resolveDeploymentAccess(c, accountStore, deploymentStore)
	if !ok {
		return nil, false
	}
	dep := access.Deployment

	resolvedProm := k8sReg.PrometheusClientFor(c.Request.Context(), dep.EffectiveClusterID(), promClient)
	clusterFilter := ""
	if resolvedProm != nil {
		clusterFilter = k8sReg.PrometheusClusterFilter(c.Request.Context(), dep.EffectiveClusterID())
	}

	return &deploymentContext{
		Deployment:    dep,
		Namespace:     dep.Namespace,
		ServiceName:   deployment.SanitizeName(dep.AgentName),
		ClusterFilter: clusterFilter,
		PromClient:    resolvedProm,
	}, true
}

// networkWindow holds a validated [from, to] window plus the PromQL range
// expression that covers it (e.g. "3600s").
type networkWindow struct {
	From  time.Time
	To    time.Time
	Range string
}

// parseNetworkWindow reads start_time / end_time RFC3339 query params.
// Both absent → default to last 1h. Exactly one set → 400. Range floored at 60s
// so PromQL increase() has at least two scrape samples (Beyla scrapes every 30s).
func parseNetworkWindow(c *gin.Context) (networkWindow, bool) {
	startStr := c.Query("start_time")
	endStr := c.Query("end_time")

	if (startStr == "") != (endStr == "") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start_time and end_time must both be provided or both omitted"})
		return networkWindow{}, false
	}

	now := time.Now().UTC()
	from := now.Add(-time.Hour)
	to := now

	if startStr != "" {
		var err error
		from, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_time: must be RFC3339"})
			return networkWindow{}, false
		}
		to, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end_time: must be RFC3339"})
			return networkWindow{}, false
		}
		if !to.After(from) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "end_time must be after start_time"})
			return networkWindow{}, false
		}
	}

	seconds := int(to.Sub(from).Seconds())
	if seconds < 60 {
		seconds = 60
	}

	return networkWindow{
		From:  from,
		To:    to,
		Range: fmt.Sprintf("%ds", seconds),
	}, true
}

// directionSpec describes how to query one of the three logical directions.
// histogramMetrics names the Beyla histogram families (sum/count/bucket suffixes
// applied at query time). Multiple entries get OR'd into a single __name__ regex
// — only safe to union when families share `le` boundaries and the label set
// used in `by()` clauses, which is why RPC is not folded in here.
type directionSpec struct {
	histogramMetrics []string
	sizeMetrics      []string // counter families for bytes; empty for db
	peerLabel        string   // "http_route" | "server_address" | "db_system_name"
	hasStatusCode    bool     // true when http_response_status_code label is present
}

var directionSpecs = map[string]directionSpec{
	"inbound": {
		histogramMetrics: []string{"http_server_request_duration_seconds"},
		sizeMetrics:      []string{"http_server_request_size_bytes"},
		peerLabel:        "http_route",
		hasStatusCode:    true,
	},
	"outbound": {
		histogramMetrics: []string{"http_client_request_duration_seconds"},
		sizeMetrics:      []string{"http_client_request_size_bytes"},
		peerLabel:        "server_address",
		hasStatusCode:    true,
	},
	"database": {
		histogramMetrics: []string{"db_client_operation_duration_seconds"},
		sizeMetrics:      nil,
		peerLabel:        "db_system_name",
		hasStatusCode:    false,
	},
}

// nameSelector builds a {__name__=~"a|b",k8s_namespace_name="ns",service_name="svc",cluster="Y",extra}
// matcher for the requested metric suffix (e.g. "_count", "_bucket"). The
// namespace + service_name pair is what Beyla actually emits and is what the
// OBI Grafana dashboard filters on.
func nameSelector(metrics []string, suffix, namespace, serviceName, clusterFilter, extra string) string {
	expr := ""
	for i, m := range metrics {
		if i > 0 {
			expr += "|"
		}
		expr += m + suffix
	}
	return fmt.Sprintf(`{__name__=~"%s",k8s_namespace_name="%s",service_name="%s"%s%s}`,
		expr, namespace, serviceName, clusterFilter, extra)
}

// GetNetworkSummary returns RED-style aggregates per direction over a time window.
// Empty window defaults to the last hour.
// GET /api/v1/deployments/:id/network/summary
func GetNetworkSummary(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	k8sReg *k8s.Registry,
	promClient *promquery.Client,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		dctx, ok := resolveDeploymentContext(c, deploymentStore, accountStore, k8sReg, promClient)
		if !ok {
			return
		}
		window, ok := parseNetworkWindow(c)
		if !ok {
			return
		}

		// No Prometheus configured → return a zero-valued response so the
		// frontend can render the empty state without erroring out.
		if dctx.PromClient == nil {
			c.JSON(http.StatusOK, NetworkSummaryResponse{
				WindowFrom: window.From,
				WindowTo:   window.To,
			})
			return
		}

		resp := NetworkSummaryResponse{
			WindowFrom: window.From,
			WindowTo:   window.To,
		}
		targets := map[string]*DirectionSummary{
			"inbound":  &resp.Inbound,
			"outbound": &resp.Outbound,
			"database": &resp.Database,
		}

		g, gCtx := errgroup.WithContext(c.Request.Context())
		for dirName, sum := range targets {
			g.Go(func() error {
				return fillDirectionSummary(gCtx, dctx.PromClient, directionSpecs[dirName], dctx, window, sum)
			})
		}
		if err := g.Wait(); err != nil {
			log.Error("Network summary query failed", "deployment_id", dctx.Deployment.ID, "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query network metrics"})
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

// fillDirectionSummary issues the six PromQL queries that populate a single
// direction's summary block. Returns the first transport-level error; per-metric
// failures (e.g. no histogram samples) fall back to zero/nil values.
func fillDirectionSummary(
	ctx context.Context,
	promClient *promquery.Client,
	spec directionSpec,
	dctx *deploymentContext,
	window networkWindow,
	out *DirectionSummary,
) error {
	ns := dctx.Namespace
	svc := dctx.ServiceName
	cluster := dctx.ClusterFilter
	w := window.Range

	// Request count: sum increase of *_count across the window.
	reqQL := fmt.Sprintf(`sum(increase(%s[%s]))`,
		nameSelector(spec.histogramMetrics, "_count", ns, svc, cluster, ""), w)
	if s, err := promClient.Query(ctx, reqQL); err != nil {
		return fmt.Errorf("request count: %w", err)
	} else if len(s) > 0 {
		out.RequestCount = int64(math.Round(s[0].Value))
	}

	// Error count: same metric, restricted to 4xx/5xx status. Only HTTP-family
	// metrics carry http_response_status_code, so this stays zero for database.
	if spec.hasStatusCode {
		errQL := fmt.Sprintf(`sum(increase(%s[%s]))`,
			nameSelector(spec.histogramMetrics, "_count", ns, svc, cluster, `,http_response_status_code=~"4..|5.."`), w)
		if s, err := promClient.Query(ctx, errQL); err != nil {
			return fmt.Errorf("error count: %w", err)
		} else if len(s) > 0 {
			out.ErrorCount = int64(math.Round(s[0].Value))
		}
		if out.RequestCount > 0 {
			out.ErrorRate = float64(out.ErrorCount) / float64(out.RequestCount)
		}
	}

	// Latency percentiles from histogram_quantile on the bucket family.
	bucketSel := nameSelector(spec.histogramMetrics, "_bucket", ns, svc, cluster, "")
	for q, dst := range map[float64]**float64{
		0.5:  &out.LatencyP50Ms,
		0.95: &out.LatencyP95Ms,
		0.99: &out.LatencyP99Ms,
	} {
		latQL := fmt.Sprintf(`histogram_quantile(%g, sum by (le) (rate(%s[%s])))`, q, bucketSel, w)
		s, err := promClient.Query(ctx, latQL)
		if err != nil {
			return fmt.Errorf("latency p%g: %w", q*100, err)
		}
		if len(s) > 0 && !math.IsNaN(s[0].Value) {
			ms := s[0].Value * 1000
			*dst = &ms
		}
	}

	// Unique peers: count of distinct peer-label values that had traffic.
	peerQL := fmt.Sprintf(`count(sum by (%s) (increase(%s[%s])) > 0)`,
		spec.peerLabel,
		nameSelector(spec.histogramMetrics, "_count", ns, svc, cluster, ""),
		w,
	)
	if s, err := promClient.Query(ctx, peerQL); err != nil {
		return fmt.Errorf("unique peers: %w", err)
	} else if len(s) > 0 {
		out.UniquePeerCount = int(math.Round(s[0].Value))
	}

	// Bytes total. db_client metrics have no request-size counter.
	if len(spec.sizeMetrics) > 0 {
		bytesQL := fmt.Sprintf(`sum(increase(%s[%s]))`,
			nameSelector(spec.sizeMetrics, "", ns, svc, cluster, ""), w)
		if s, err := promClient.Query(ctx, bytesQL); err != nil {
			return fmt.Errorf("bytes total: %w", err)
		} else if len(s) > 0 {
			out.BytesTotal = int64(math.Round(s[0].Value))
		}
	}

	return nil
}

// peerKindFor maps the direction's peer label to the response's peer_kind tag.
func peerKindFor(direction string) string {
	switch direction {
	case "inbound":
		return "route"
	case "outbound":
		return "address"
	case "database":
		return "db_system"
	}
	return ""
}

// GetNetworkFlows returns the top-N peers for one direction, ranked by the
// requested sort key. Each row is the per-peer rollup over the requested window.
// GET /api/v1/deployments/:id/network/flows
func GetNetworkFlows(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	k8sReg *k8s.Registry,
	promClient *promquery.Client,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		direction := c.Query("direction")
		spec, ok := directionSpecs[direction]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "direction must be inbound, outbound, or database"})
			return
		}

		sortKey := c.DefaultQuery("sort", "requests")
		switch sortKey {
		case "requests", "latency_p95", "errors":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "sort must be requests, latency_p95, or errors"})
			return
		}

		limit := flowsDefaultLimit
		if raw := c.Query("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
				return
			}
			limit = parsed
			if limit > flowsMaxLimit {
				limit = flowsMaxLimit
			}
		}

		dctx, ok := resolveDeploymentContext(c, deploymentStore, accountStore, k8sReg, promClient)
		if !ok {
			return
		}
		window, ok := parseNetworkWindow(c)
		if !ok {
			return
		}

		resp := NetworkFlowsResponse{Direction: direction, Flows: []NetworkFlow{}}
		if dctx.PromClient == nil {
			c.JSON(http.StatusOK, resp)
			return
		}

		flows, err := collectFlows(c.Request.Context(), dctx.PromClient, spec, dctx, window, direction)
		if err != nil {
			log.Error("Network flows query failed",
				"deployment_id", dctx.Deployment.ID, "direction", direction, "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query network flows"})
			return
		}

		sortFlows(flows, sortKey)
		if len(flows) > limit {
			flows = flows[:limit]
		}
		resp.Flows = flows
		c.JSON(http.StatusOK, resp)
	}
}

// collectFlows fans out the per-peer PromQL queries and merges them into a
// flat slice keyed by the peer label value.
func collectFlows(
	ctx context.Context,
	promClient *promquery.Client,
	spec directionSpec,
	dctx *deploymentContext,
	window networkWindow,
	direction string,
) ([]NetworkFlow, error) {
	ns := dctx.Namespace
	svc := dctx.ServiceName
	cluster := dctx.ClusterFilter
	w := window.Range
	peerLabel := spec.peerLabel
	peerKind := peerKindFor(direction)

	// Per-peer request count + (for HTTP) status-code breakdown in one query.
	// Sum by (peer, status_code) — collapsing status_code in Go gives requests,
	// errors, and the 2xx/4xx/5xx buckets.
	countSel := nameSelector(spec.histogramMetrics, "_count", ns, svc, cluster, "")
	bucketSel := nameSelector(spec.histogramMetrics, "_bucket", ns, svc, cluster, "")

	groupLabels := peerLabel
	if spec.hasStatusCode {
		groupLabels = peerLabel + ",http_response_status_code"
	}
	countQL := fmt.Sprintf(`sum by (%s) (increase(%s[%s]))`, groupLabels, countSel, w)
	p50QL := fmt.Sprintf(`histogram_quantile(0.5, sum by (%s,le) (rate(%s[%s])))`, peerLabel, bucketSel, w)
	p95QL := fmt.Sprintf(`histogram_quantile(0.95, sum by (%s,le) (rate(%s[%s])))`, peerLabel, bucketSel, w)

	var (
		countSamples, p50Samples, p95Samples, bytesSamples []promquery.Sample
	)
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		s, err := promClient.Query(gCtx, countQL)
		countSamples = s
		return err
	})
	g.Go(func() error {
		s, err := promClient.Query(gCtx, p50QL)
		p50Samples = s
		return err
	})
	g.Go(func() error {
		s, err := promClient.Query(gCtx, p95QL)
		p95Samples = s
		return err
	})
	if len(spec.sizeMetrics) > 0 {
		bytesQL := fmt.Sprintf(`sum by (%s) (increase(%s[%s]))`,
			peerLabel, nameSelector(spec.sizeMetrics, "", ns, svc, cluster, ""), w)
		g.Go(func() error {
			s, err := promClient.Query(gCtx, bytesQL)
			bytesSamples = s
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	flowByPeer := map[string]*NetworkFlow{}
	getFlow := func(peer string) *NetworkFlow {
		if f, ok := flowByPeer[peer]; ok {
			return f
		}
		f := &NetworkFlow{Peer: peer, PeerKind: peerKind}
		if peerKind == "address" {
			f.RegistrableDomain = peerdomain.Registrable(peer)
		}
		if spec.hasStatusCode {
			f.StatusCodes = map[string]int64{}
		}
		flowByPeer[peer] = f
		return f
	}

	for _, s := range countSamples {
		peer := s.Labels[peerLabel]
		if peer == "" {
			peer = "(unknown)"
		}
		f := getFlow(peer)
		count := int64(math.Round(s.Value))
		f.RequestCount += count
		if spec.hasStatusCode {
			class := statusClass(s.Labels["http_response_status_code"])
			f.StatusCodes[class] += count
			if class == "4xx" || class == "5xx" {
				f.ErrorCount += count
			}
		}
	}

	for _, s := range p50Samples {
		peer := s.Labels[peerLabel]
		if peer == "" || math.IsNaN(s.Value) {
			continue
		}
		f := getFlow(peer)
		ms := s.Value * 1000
		f.LatencyP50Ms = &ms
	}
	for _, s := range p95Samples {
		peer := s.Labels[peerLabel]
		if peer == "" || math.IsNaN(s.Value) {
			continue
		}
		f := getFlow(peer)
		ms := s.Value * 1000
		f.LatencyP95Ms = &ms
	}
	for _, s := range bytesSamples {
		peer := s.Labels[peerLabel]
		if peer == "" {
			peer = "(unknown)"
		}
		f := getFlow(peer)
		f.BytesTotal += int64(math.Round(s.Value))
	}

	flows := make([]NetworkFlow, 0, len(flowByPeer))
	for _, f := range flowByPeer {
		if f.RequestCount > 0 {
			f.ErrorRate = float64(f.ErrorCount) / float64(f.RequestCount)
		}
		flows = append(flows, *f)
	}
	return flows, nil
}

// statusClass buckets an HTTP status code into "2xx"/"3xx"/"4xx"/"5xx".
// Unknown / non-numeric codes fall into "other".
func statusClass(code string) string {
	if code == "" {
		return "other"
	}
	n, err := strconv.Atoi(code)
	if err != nil {
		return "other"
	}
	switch {
	case n >= 200 && n < 300:
		return "2xx"
	case n >= 300 && n < 400:
		return "3xx"
	case n >= 400 && n < 500:
		return "4xx"
	case n >= 500 && n < 600:
		return "5xx"
	}
	return "other"
}

const (
	timeseriesDefaultStep = 30 * time.Second
	timeseriesMinStep     = 15 * time.Second
	timeseriesTopK        = 8
)

// GetNetworkTimeseries returns bucketed series for one (direction, metric)
// combination, optionally grouped. Bounded windows are required (no auto-default
// here — charts always specify a range).
// GET /api/v1/deployments/:id/network/timeseries
func GetNetworkTimeseries(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	k8sReg *k8s.Registry,
	promClient *promquery.Client,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		direction := c.Query("direction")
		spec, ok := directionSpecs[direction]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "direction must be inbound, outbound, or database"})
			return
		}

		metric := c.Query("metric")
		switch metric {
		case "rate", "latency_p95":
		case "errors", "bytes":
			if !spec.hasStatusCode {
				c.JSON(http.StatusBadRequest, gin.H{"error": metric + " is not available for the database direction"})
				return
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "metric must be rate, errors, latency_p95, or bytes"})
			return
		}

		groupBy := c.Query("group_by")
		switch groupBy {
		case "", "peer":
		case "status_class":
			if !spec.hasStatusCode {
				c.JSON(http.StatusBadRequest, gin.H{"error": "status_class is not available for the database direction"})
				return
			}
			if metric == "latency_p95" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "status_class group_by is not supported for latency"})
				return
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "group_by must be peer or status_class"})
			return
		}

		step := timeseriesDefaultStep
		if raw := c.Query("step"); raw != "" {
			parsed, err := time.ParseDuration(raw)
			if err != nil || parsed < timeseriesMinStep {
				c.JSON(http.StatusBadRequest, gin.H{"error": "step must be a duration >= 15s (e.g. 30s, 1m)"})
				return
			}
			step = parsed
		}

		dctx, ok := resolveDeploymentContext(c, deploymentStore, accountStore, k8sReg, promClient)
		if !ok {
			return
		}
		window, ok := parseNetworkWindow(c)
		if !ok {
			return
		}

		resp := NetworkTimeseriesResponse{
			Direction: direction,
			Metric:    metric,
			Step:      step.String(),
			Series:    []NetworkSeries{},
		}
		if dctx.PromClient == nil {
			c.JSON(http.StatusOK, resp)
			return
		}

		series, err := queryTimeseries(c.Request.Context(), dctx.PromClient, spec, dctx, window, step, metric, groupBy)
		if err != nil {
			log.Error("Network timeseries query failed",
				"deployment_id", dctx.Deployment.ID, "direction", direction, "metric", metric, "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query network timeseries"})
			return
		}
		resp.Series = series
		c.JSON(http.StatusOK, resp)
	}
}

// queryTimeseries builds the appropriate PromQL for the (metric, group_by)
// combo, runs the range query, and shapes the matrix into NetworkSeries rows.
func queryTimeseries(
	ctx context.Context,
	promClient *promquery.Client,
	spec directionSpec,
	dctx *deploymentContext,
	window networkWindow,
	step time.Duration,
	metric, groupBy string,
) ([]NetworkSeries, error) {
	ns := dctx.Namespace
	svc := dctx.ServiceName
	cluster := dctx.ClusterFilter

	// rateWindow is the lookback inside rate()/increase() — Beyla scrapes every
	// 30s, so [step*4] guarantees ≥4 samples per bucket.
	rateWindow := step * 4
	if rateWindow < 60*time.Second {
		rateWindow = 60 * time.Second
	}
	rw := fmt.Sprintf("%ds", int(rateWindow.Seconds()))

	q, labelKey := buildTimeseriesQL(spec, metric, groupBy, ns, svc, cluster, rw)
	matrix, err := promClient.QueryRange(ctx, q, window.From, window.To, step)
	if err != nil {
		return nil, err
	}

	// status_class grouping needs Go-side bucketing (Prom returns raw status
	// codes; we collapse them into 2xx/4xx/5xx).
	if groupBy == "status_class" {
		return foldStatusClassSeries(matrix), nil
	}

	series := make([]NetworkSeries, 0, len(matrix))
	for _, m := range matrix {
		label := "total"
		if labelKey != "" {
			label = m.Labels[labelKey]
			if label == "" {
				label = "(unknown)"
			}
		}
		series = append(series, NetworkSeries{
			Label:  label,
			Points: matrixToPoints(m, metric == "rate" || metric == "errors" || metric == "bytes"),
		})
	}
	return series, nil
}

// buildTimeseriesQL produces the PromQL for a (metric, group_by) combination
// and returns the label key the response should pull from each series' metric
// labels (empty when no grouping → series labelled "total").
func buildTimeseriesQL(spec directionSpec, metric, groupBy, namespace, serviceName, cluster, rw string) (string, string) {
	countSel := nameSelector(spec.histogramMetrics, "_count", namespace, serviceName, cluster, "")
	bucketSel := nameSelector(spec.histogramMetrics, "_bucket", namespace, serviceName, cluster, "")
	bytesSel := ""
	if len(spec.sizeMetrics) > 0 {
		bytesSel = nameSelector(spec.sizeMetrics, "", namespace, serviceName, cluster, "")
	}

	groupLabel := ""
	switch groupBy {
	case "peer":
		groupLabel = spec.peerLabel
	case "status_class":
		groupLabel = "http_response_status_code"
	}

	// errors restricts the same _count metric to 4xx/5xx via an extra matcher.
	if metric == "errors" {
		countSel = nameSelector(spec.histogramMetrics, "_count", namespace, serviceName, cluster, `,http_response_status_code=~"4..|5.."`)
	}

	switch metric {
	case "rate", "errors":
		if groupLabel == "" {
			return fmt.Sprintf(`sum(rate(%s[%s]))`, countSel, rw), ""
		}
		base := fmt.Sprintf(`sum by (%s) (rate(%s[%s]))`, groupLabel, countSel, rw)
		if groupBy == "peer" {
			base = fmt.Sprintf(`topk(%d, %s)`, timeseriesTopK, base)
		}
		return base, groupLabel
	case "bytes":
		if groupLabel == "" {
			return fmt.Sprintf(`sum(rate(%s[%s]))`, bytesSel, rw), ""
		}
		base := fmt.Sprintf(`sum by (%s) (rate(%s[%s]))`, groupLabel, bytesSel, rw)
		if groupBy == "peer" {
			base = fmt.Sprintf(`topk(%d, %s)`, timeseriesTopK, base)
		}
		return base, groupLabel
	case "latency_p95":
		if groupLabel == "" {
			return fmt.Sprintf(`histogram_quantile(0.95, sum by (le) (rate(%s[%s])))`, bucketSel, rw), ""
		}
		base := fmt.Sprintf(`histogram_quantile(0.95, sum by (%s,le) (rate(%s[%s])))`, groupLabel, bucketSel, rw)
		if groupBy == "peer" {
			base = fmt.Sprintf(`topk(%d, %s)`, timeseriesTopK, base)
		}
		return base, groupLabel
	}
	return "", ""
}

// matrixToPoints converts a single MatrixSample into response points. When
// counterScale is true, the raw rate (per second) is left as-is; latency
// quantile values are converted from seconds to milliseconds.
func matrixToPoints(m promquery.MatrixSample, counterScale bool) []NetworkPoint {
	points := make([]NetworkPoint, 0, len(m.Points))
	for _, p := range m.Points {
		if math.IsNaN(p.Value) {
			continue
		}
		v := p.Value
		if !counterScale {
			v = v * 1000 // latency seconds → ms
		}
		points = append(points, NetworkPoint{Timestamp: p.Timestamp, Value: v})
	}
	return points
}

// foldStatusClassSeries collapses raw status-code series (e.g. "200", "404",
// "500") into 2xx/3xx/4xx/5xx buckets, summing point-by-point.
func foldStatusClassSeries(matrix []promquery.MatrixSample) []NetworkSeries {
	buckets := map[string]map[time.Time]float64{}
	for _, m := range matrix {
		class := statusClass(m.Labels["http_response_status_code"])
		bucket, ok := buckets[class]
		if !ok {
			bucket = map[time.Time]float64{}
			buckets[class] = bucket
		}
		for _, p := range m.Points {
			if math.IsNaN(p.Value) {
				continue
			}
			bucket[p.Timestamp] += p.Value
		}
	}

	// Stable label order: 2xx, 3xx, 4xx, 5xx, other.
	order := []string{"2xx", "3xx", "4xx", "5xx", "other"}
	series := make([]NetworkSeries, 0, len(buckets))
	for _, class := range order {
		bucket, ok := buckets[class]
		if !ok {
			continue
		}
		timestamps := make([]time.Time, 0, len(bucket))
		for ts := range bucket {
			timestamps = append(timestamps, ts)
		}
		sort.Slice(timestamps, func(i, j int) bool { return timestamps[i].Before(timestamps[j]) })
		points := make([]NetworkPoint, 0, len(timestamps))
		for _, ts := range timestamps {
			points = append(points, NetworkPoint{Timestamp: ts, Value: bucket[ts]})
		}
		series = append(series, NetworkSeries{Label: class, Points: points})
	}
	return series
}

// sortFlows orders flows in place by the chosen key, descending. Nil-latency
// peers sort to the end when ranking by latency_p95.
func sortFlows(flows []NetworkFlow, key string) {
	sort.SliceStable(flows, func(i, j int) bool {
		switch key {
		case "latency_p95":
			a, b := flows[i].LatencyP95Ms, flows[j].LatencyP95Ms
			if a == nil && b == nil {
				return false
			}
			if a == nil {
				return false
			}
			if b == nil {
				return true
			}
			return *a > *b
		case "errors":
			return flows[i].ErrorCount > flows[j].ErrorCount
		}
		// default: requests
		return flows[i].RequestCount > flows[j].RequestCount
	})
}
