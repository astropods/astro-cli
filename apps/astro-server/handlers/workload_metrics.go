package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// workloadMetricsRanges maps the API's range presets onto a (window, step)
// pair sized so each query produces roughly 100–250 points — dense enough
// for a smooth chart and cheap enough on Prometheus.
var workloadMetricsRanges = map[string]struct {
	window time.Duration
	step   time.Duration
}{
	"1h":  {window: time.Hour, step: 30 * time.Second},
	"6h":  {window: 6 * time.Hour, step: 2 * time.Minute},
	"24h": {window: 24 * time.Hour, step: 10 * time.Minute},
	"7d":  {window: 7 * 24 * time.Hour, step: time.Hour},
}

// rate() needs at least two scrape samples to produce a value. cAdvisor is
// typically scraped every 30s, so floor the lookback at four samples to keep
// short ranges from yielding empty buckets while padding tolerates a missed
// scrape.
func rateWindowFor(step time.Duration) time.Duration {
	return max(step*4, 2*time.Minute)
}

// MetricPoint is one (timestamp, value) sample in a workload metric series.
type MetricPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// WorkloadMetricsResponse is returned by GET /deployments/:id/pods/:pod/metrics.
// Each series is sum'd across the pod's containers (excluding the pause
// container) so callers see one line per resource. StorageUsed and
// StorageCapacity are sum'd across the pod's bound PVCs and are empty arrays
// when the pod mounts no PVC. Restarts and OOMs are discrete event
// timestamps emitted as chart markers rather than continuous series.
type WorkloadMetricsResponse struct {
	Pod             string        `json:"pod"`
	Range           string        `json:"range"`
	Step            string        `json:"step"`
	CPU             []MetricPoint `json:"cpu"`              // vCPU cores in use
	Memory          []MetricPoint `json:"memory"`           // working-set bytes
	StorageUsed     []MetricPoint `json:"storage_used"`     // bytes used across all PVCs
	StorageCapacity []MetricPoint `json:"storage_capacity"` // bytes provisioned across all PVCs
	NetworkRx       []MetricPoint `json:"network_rx"`       // ingress bytes/sec
	NetworkTx       []MetricPoint `json:"network_tx"`       // egress bytes/sec
	FsRead          []MetricPoint `json:"fs_read"`          // disk read bytes/sec
	FsWrite         []MetricPoint `json:"fs_write"`         // disk write bytes/sec
	Restarts        []time.Time   `json:"restarts"`         // container restart timestamps within window
	OOMs            []time.Time   `json:"ooms"`             // OOM-kill timestamps within window
	// CPULimit is the pod-level CPU limit (vCPU cores), summed across the
	// pod's regular containers and any Always-restart init sidecars. 0 when
	// the pod spec wasn't reachable or no limit was set.
	CPULimit float64 `json:"cpu_limit"`
	// MemoryLimit is the pod-level memory limit in bytes, summed the same way
	// as CPULimit. 0 when unknown.
	MemoryLimit int64 `json:"memory_limit"`
}

// GetWorkloadMetrics returns CPU, memory, and storage time series for a
// single pod inside a deployment.
// GET /api/v1/deployments/:id/pods/:pod/metrics?range=1h|6h|24h|7d
func GetWorkloadMetrics(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	promClient *promquery.Client,
	k8sReg *k8s.Registry,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		podName := c.Param("pod")
		if podName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pod path parameter is required"})
			return
		}

		rangeKey := c.DefaultQuery("range", "1h")
		preset, ok := workloadMetricsRanges[rangeKey]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "range must be one of 1h, 6h, 24h, 7d"})
			return
		}

		dctx, ok := resolveDeploymentContext(c, deploymentStore, accountStore, k8sReg, promClient)
		if !ok {
			return
		}

		end := time.Now().UTC()
		start := end.Add(-preset.window)

		resp := WorkloadMetricsResponse{
			Pod:             podName,
			Range:           rangeKey,
			Step:            preset.step.String(),
			CPU:             []MetricPoint{},
			Memory:          []MetricPoint{},
			StorageUsed:     []MetricPoint{},
			StorageCapacity: []MetricPoint{},
			NetworkRx:       []MetricPoint{},
			NetworkTx:       []MetricPoint{},
			FsRead:          []MetricPoint{},
			FsWrite:         []MetricPoint{},
			Restarts:        []time.Time{},
			OOMs:            []time.Time{},
		}

		if promClient == nil {
			c.JSON(http.StatusOK, resp)
			return
		}

		// Resolve the pod's PVCs and resource limits once up-front. Storage
		// queries are skipped when the cluster client is unavailable or the
		// pod mounts no PVC — the empty arrays still match the response
		// contract. Limits fall back to 0 (i.e. "unknown") on the same path.
		podInfo := podClusterInfo(c.Request.Context(), k8sReg, dctx.Deployment, podName)
		pvcNames := podInfo.pvcs
		resp.CPULimit = podInfo.cpuLimitCores
		resp.MemoryLimit = podInfo.memLimitBytes

		cpuQL, memQL := buildWorkloadMetricQueries(dctx.Namespace, podName, dctx.ClusterFilter, preset.step)
		storageUsedQL, storageCapQL := buildStorageQueries(dctx.Namespace, pvcNames, dctx.ClusterFilter)
		netRxQL, netTxQL, fsReadQL, fsWriteQL := buildIOQueries(dctx.Namespace, podName, dctx.ClusterFilter, preset.step)
		restartsQL, oomsQL := buildEventQueries(dctx.Namespace, podName, dctx.ClusterFilter, preset.step)

		// Run queries in parallel — they're independent and all round-trip
		// to the same Prometheus.
		g, gctx := errgroup.WithContext(c.Request.Context())
		var cpuPoints, memPoints, storageUsedPoints, storageCapPoints []MetricPoint
		var netRxPoints, netTxPoints, fsReadPoints, fsWritePoints []MetricPoint
		var restartEvents, oomEvents []time.Time
		g.Go(func() error {
			pts, err := singleSeriesMatrix(gctx, promClient, cpuQL, start, end, preset.step)
			if err != nil {
				return fmt.Errorf("cpu: %w", err)
			}
			cpuPoints = pts
			return nil
		})
		g.Go(func() error {
			pts, err := singleSeriesMatrix(gctx, promClient, memQL, start, end, preset.step)
			if err != nil {
				return fmt.Errorf("memory: %w", err)
			}
			memPoints = pts
			return nil
		})
		if storageUsedQL != "" {
			g.Go(func() error {
				pts, err := singleSeriesMatrix(gctx, promClient, storageUsedQL, start, end, preset.step)
				if err != nil {
					return fmt.Errorf("storage used: %w", err)
				}
				storageUsedPoints = pts
				return nil
			})
			g.Go(func() error {
				pts, err := singleSeriesMatrix(gctx, promClient, storageCapQL, start, end, preset.step)
				if err != nil {
					return fmt.Errorf("storage capacity: %w", err)
				}
				storageCapPoints = pts
				return nil
			})
		}
		for _, q := range []struct {
			name string
			ql   string
			out  *[]MetricPoint
		}{
			{"network rx", netRxQL, &netRxPoints},
			{"network tx", netTxQL, &netTxPoints},
			{"fs read", fsReadQL, &fsReadPoints},
			{"fs write", fsWriteQL, &fsWritePoints},
		} {
			g.Go(func() error {
				pts, err := singleSeriesMatrix(gctx, promClient, q.ql, start, end, preset.step)
				if err != nil {
					return fmt.Errorf("%s: %w", q.name, err)
				}
				*q.out = pts
				return nil
			})
		}
		g.Go(func() error {
			ts, err := eventTimestamps(gctx, promClient, restartsQL, start, end, preset.step)
			if err != nil {
				return fmt.Errorf("restarts: %w", err)
			}
			restartEvents = ts
			return nil
		})
		g.Go(func() error {
			ts, err := eventTimestamps(gctx, promClient, oomsQL, start, end, preset.step)
			if err != nil {
				return fmt.Errorf("ooms: %w", err)
			}
			oomEvents = ts
			return nil
		})
		if err := g.Wait(); err != nil {
			log.Error("Workload metrics query failed",
				"deployment_id", dctx.Deployment.ID, "pod", podName, "range", rangeKey, "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query workload metrics"})
			return
		}

		// Only overwrite the empty-slice initializers when a goroutine
		// actually ran and produced data — otherwise resp.StorageUsed /
		// StorageCapacity stay []MetricPoint{} (which marshals as []) rather
		// than turning into nil (which would marshal as JSON null and trip up
		// clients that do storage_used.length).
		resp.CPU = cpuPoints
		resp.Memory = memPoints
		if storageUsedPoints != nil {
			resp.StorageUsed = storageUsedPoints
		}
		if storageCapPoints != nil {
			resp.StorageCapacity = storageCapPoints
		}
		if netRxPoints != nil {
			resp.NetworkRx = netRxPoints
		}
		if netTxPoints != nil {
			resp.NetworkTx = netTxPoints
		}
		if fsReadPoints != nil {
			resp.FsRead = fsReadPoints
		}
		if fsWritePoints != nil {
			resp.FsWrite = fsWritePoints
		}
		if restartEvents != nil {
			resp.Restarts = restartEvents
		}
		if oomEvents != nil {
			resp.OOMs = oomEvents
		}
		c.JSON(http.StatusOK, resp)
	}
}

// podMetricsInfo is the per-pod static context the metrics handler reads
// straight from the K8s spec (rather than from Prometheus): which PVCs the
// pod mounts plus the aggregated CPU/memory limits the scheduler enforces.
type podMetricsInfo struct {
	pvcs          []string
	cpuLimitCores float64
	memLimitBytes int64
}

// podClusterInfo fetches the pod from the tenant cluster and returns both
// its mounted PVC names and its aggregated CPU/memory limits. Returns a
// zero-valued struct on any failure — every chart degrades gracefully when
// the lookup can't run.
func podClusterInfo(ctx context.Context, k8sReg *k8s.Registry, dep *deploymentstore.Deployment, podName string) podMetricsInfo {
	var info podMetricsInfo
	if k8sReg == nil {
		return info
	}
	k8sClient, err := deploymentClusterClient(ctx, k8sReg, dep)
	if err != nil || k8sClient == nil {
		return info
	}
	pod, err := k8sClient.Clientset().CoreV1().Pods(dep.Namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return info
	}
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName != "" {
			info.pvcs = append(info.pvcs, v.PersistentVolumeClaim.ClaimName)
		}
	}
	addLimits := func(c corev1.Container) {
		if q, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
			info.cpuLimitCores += q.AsApproximateFloat64()
		}
		if q, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
			info.memLimitBytes += q.Value()
		}
	}
	for _, ctr := range pod.Spec.Containers {
		addLimits(ctr)
	}
	// Native sidecars (Always-restart init containers) run concurrently with
	// regular containers, so their limits contribute to the pod-level limit.
	for _, ctr := range pod.Spec.InitContainers {
		if ctr.RestartPolicy != nil && *ctr.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			addLimits(ctr)
		}
	}
	return info
}

// buildIOQueries returns PromQL for the four pod-level IO throughput
// series — receive/transmit network bytes per second and read/write
// filesystem bytes per second — each sum'd across the pod's interfaces or
// devices respectively. Network counters are pod-scoped (the pause
// container owns the netns) so they have no container label; FS counters
// do carry one but we sum across containers + devices for a per-pod view.
func buildIOQueries(namespace, pod, clusterFilter string, step time.Duration) (rx, tx, fsRead, fsWrite string) {
	rw := rateWindowFor(step)
	rateSecs := int(rw.Seconds())

	netSel := fmt.Sprintf(`namespace="%s",pod="%s"%s`, namespace, pod, clusterFilter)
	rx = fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{%s}[%ds]))`, netSel, rateSecs)
	tx = fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{%s}[%ds]))`, netSel, rateSecs)

	fsSel := fmt.Sprintf(`namespace="%s",pod="%s",container!="POD",container!=""%s`, namespace, pod, clusterFilter)
	fsRead = fmt.Sprintf(`sum(rate(container_fs_reads_bytes_total{%s}[%ds]))`, fsSel, rateSecs)
	fsWrite = fmt.Sprintf(`sum(rate(container_fs_writes_bytes_total{%s}[%ds]))`, fsSel, rateSecs)
	return
}

// buildEventQueries returns PromQL for restart and OOM event counters,
// wrapped in changes(...) so each evaluation point reports the number of
// counter increments inside the preceding step window. Bucket timestamps
// where the value > 0 are emitted as event markers by eventTimestamps.
//
//   - Restarts come from kube-state-metrics, which has no per-container
//     restart counter — kube_pod_container_status_restarts_total is per
//     (pod, container); we sum across containers in the pod.
//   - OOMs come from cAdvisor's container_oom_events_total; same idea,
//     summed across containers.
func buildEventQueries(namespace, pod, clusterFilter string, step time.Duration) (string, string) {
	stepSecs := max(int(step.Seconds()), 60)
	restartsSel := fmt.Sprintf(`namespace="%s",pod="%s"%s`, namespace, pod, clusterFilter)
	oomsSel := fmt.Sprintf(`namespace="%s",pod="%s",container!="POD",container!=""%s`, namespace, pod, clusterFilter)
	restarts := fmt.Sprintf(`sum(changes(kube_pod_container_status_restarts_total{%s}[%ds]))`, restartsSel, stepSecs)
	ooms := fmt.Sprintf(`sum(changes(container_oom_events_total{%s}[%ds]))`, oomsSel, stepSecs)
	return restarts, ooms
}

// eventTimestamps runs a range query whose result is a counter-diff series
// (typically produced by sum(changes(...))) and returns the bucket
// timestamps where at least one event occurred. Always returns a non-nil
// slice so the JSON response carries [] rather than null.
func eventTimestamps(ctx context.Context, client *promquery.Client, ql string, start, end time.Time, step time.Duration) ([]time.Time, error) {
	matrix, err := client.QueryRange(ctx, ql, start, end, step)
	if err != nil {
		return nil, err
	}
	out := []time.Time{}
	for _, m := range matrix {
		for _, p := range m.Points {
			if p.Value > 0 {
				out = append(out, p.Timestamp)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out, nil
}

// buildStorageQueries returns PromQL for "used" and "capacity" series across
// the pod's PVCs, summed so callers get one line for each. Returns ("", "")
// when the pod has no PVCs — the handler skips storage queries entirely in
// that case.
func buildStorageQueries(namespace string, pvcNames []string, clusterFilter string) (string, string) {
	if len(pvcNames) == 0 {
		return "", ""
	}
	// PromQL regex alternation; escape regex meta chars defensively even
	// though K8s PVC names are restricted to RFC 1123 (a-z0-9-).
	parts := make([]string, 0, len(pvcNames))
	for _, n := range pvcNames {
		parts = append(parts, regexEscape(n))
	}
	selector := fmt.Sprintf(`namespace="%s",persistentvolumeclaim=~"%s"%s`, namespace, strings.Join(parts, "|"), clusterFilter)
	used := fmt.Sprintf(`sum(kubelet_volume_stats_used_bytes{%s})`, selector)
	capacity := fmt.Sprintf(`sum(kubelet_volume_stats_capacity_bytes{%s})`, selector)
	return used, capacity
}

// regexEscape escapes the small set of regex metacharacters that can appear
// in defensively-handled PVC names. K8s names are normally just [a-z0-9-]
// so this is belt-and-suspenders.
func regexEscape(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`.`, `\.`,
		`+`, `\+`,
		`*`, `\*`,
		`?`, `\?`,
		`(`, `\(`,
		`)`, `\)`,
		`[`, `\[`,
		`]`, `\]`,
		`{`, `\{`,
		`}`, `\}`,
		`|`, `\|`,
		`^`, `\^`,
		`$`, `\$`,
	)
	return replacer.Replace(s)
}

// buildWorkloadMetricQueries returns the PromQL for CPU (cores) and memory
// (working-set bytes) of a single pod, summed across its containers.
// container!="POD" excludes the pause container; container!="" excludes the
// pod-level rollups cAdvisor emits without a container label.
func buildWorkloadMetricQueries(namespace, pod, clusterFilter string, step time.Duration) (string, string) {
	rw := rateWindowFor(step)
	selector := fmt.Sprintf(`namespace="%s",pod="%s",container!="POD",container!=""%s`, namespace, pod, clusterFilter)
	cpu := fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s}[%ds]))`, selector, int(rw.Seconds()))
	memory := fmt.Sprintf(`sum(container_memory_working_set_bytes{%s})`, selector)
	return cpu, memory
}

// singleSeriesMatrix runs a range query that is expected to collapse to one
// series (via `sum()` in the caller) and returns its points in time order.
// Missing series → empty slice, not an error.
func singleSeriesMatrix(ctx context.Context, client *promquery.Client, ql string, start, end time.Time, step time.Duration) ([]MetricPoint, error) {
	matrix, err := client.QueryRange(ctx, ql, start, end, step)
	if err != nil {
		return nil, err
	}
	if len(matrix) == 0 {
		return []MetricPoint{}, nil
	}
	// Flatten — `sum()` should yield a single series, but if Prometheus
	// returns multiple (e.g. across-cluster), append all points and sort.
	points := make([]MetricPoint, 0, len(matrix[0].Points))
	for _, m := range matrix {
		for _, p := range m.Points {
			points = append(points, MetricPoint{Timestamp: p.Timestamp, Value: p.Value})
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Timestamp.Before(points[j].Timestamp) })
	return points, nil
}
