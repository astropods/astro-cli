package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sync"

	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestNameSelector(t *testing.T) {
	cases := []struct {
		name        string
		metrics     []string
		suffix      string
		namespace   string
		serviceName string
		cluster     string
		extra       string
		expected    string
	}{
		{
			name:        "single metric, no cluster",
			metrics:     []string{"http_server_request_duration_seconds"},
			suffix:      "_count",
			namespace:   "astro-acct",
			serviceName: "bot",
			cluster:     "",
			extra:       "",
			expected:    `{__name__=~"http_server_request_duration_seconds_count",k8s_namespace_name="astro-acct",service_name="bot"}`,
		},
		{
			name:        "union metrics with cluster",
			metrics:     []string{"http_server_request_duration_seconds", "rpc_server_duration_seconds"},
			suffix:      "_bucket",
			namespace:   "astro-acct",
			serviceName: "bot",
			cluster:     `,cluster="prod"`,
			extra:       "",
			expected:    `{__name__=~"http_server_request_duration_seconds_bucket|rpc_server_duration_seconds_bucket",k8s_namespace_name="astro-acct",service_name="bot",cluster="prod"}`,
		},
		{
			name:        "extra label appended",
			metrics:     []string{"http_server_request_duration_seconds"},
			suffix:      "_count",
			namespace:   "astro-acct",
			serviceName: "bot",
			cluster:     `,cluster="prod"`,
			extra:       `,http_response_status_code=~"4..|5.."`,
			expected:    `{__name__=~"http_server_request_duration_seconds_count",k8s_namespace_name="astro-acct",service_name="bot",cluster="prod",http_response_status_code=~"4..|5.."}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nameSelector(tc.metrics, tc.suffix, tc.namespace, tc.serviceName, tc.cluster, tc.extra)
			if got != tc.expected {
				t.Errorf("got %q\nwant %q", got, tc.expected)
			}
		})
	}
}

func TestParseNetworkWindow(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		expectOK   bool
		expectCode int
		check      func(t *testing.T, w networkWindow)
	}{
		{
			name:     "both omitted defaults to last hour",
			query:    "",
			expectOK: true,
			check: func(t *testing.T, w networkWindow) {
				if d := w.To.Sub(w.From); d < 59*time.Minute || d > 61*time.Minute {
					t.Errorf("default window = %v, want ~1h", d)
				}
				if w.Range != "3600s" {
					t.Errorf("range = %q, want 3600s", w.Range)
				}
			},
		},
		{
			name:       "only start_time provided",
			query:      "start_time=2026-05-21T00:00:00Z",
			expectOK:   false,
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "only end_time provided",
			query:      "end_time=2026-05-21T01:00:00Z",
			expectOK:   false,
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "invalid start_time format",
			query:      "start_time=not-a-date&end_time=2026-05-21T01:00:00Z",
			expectOK:   false,
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "end before start",
			query:      "start_time=2026-05-21T01:00:00Z&end_time=2026-05-21T00:00:00Z",
			expectOK:   false,
			expectCode: http.StatusBadRequest,
		},
		{
			name:     "valid bounded window",
			query:    "start_time=2026-05-21T00:00:00Z&end_time=2026-05-21T00:30:00Z",
			expectOK: true,
			check: func(t *testing.T, w networkWindow) {
				if w.Range != "1800s" {
					t.Errorf("range = %q, want 1800s", w.Range)
				}
			},
		},
		{
			name:     "tiny window floored at 60s",
			query:    "start_time=2026-05-21T00:00:00Z&end_time=2026-05-21T00:00:05Z",
			expectOK: true,
			check: func(t *testing.T, w networkWindow) {
				if w.Range != "60s" {
					t.Errorf("range = %q, want 60s (floor)", w.Range)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/?"+tc.query, nil)

			w, ok := parseNetworkWindow(c)
			if ok != tc.expectOK {
				t.Fatalf("ok = %v, want %v (body: %s)", ok, tc.expectOK, recorder.Body.String())
			}
			if !tc.expectOK && recorder.Code != tc.expectCode {
				t.Errorf("status = %d, want %d", recorder.Code, tc.expectCode)
			}
			if tc.expectOK && tc.check != nil {
				tc.check(t, w)
			}
		})
	}
}

// fakePromHandler scripts Prometheus instant-query responses by query-string match.
// Useful for asserting both wire-level PromQL and the handler's response parsing.
type fakePromHandler struct {
	t        *testing.T
	expected map[string]string // PromQL → JSON body
	mu       sync.Mutex
	seen     map[string]bool
}

func (f *fakePromHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("query")
	if body, ok := f.expected[q]; ok {
		f.mu.Lock()
		f.seen[q] = true
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
		return
	}
	f.t.Errorf("unexpected PromQL: %q", q)
	w.WriteHeader(http.StatusInternalServerError)
}

func vectorResp(value string) string {
	return `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[0,"` + value + `"]}]}}`
}

func emptyVector() string {
	return `{"status":"success","data":{"resultType":"vector","result":[]}}`
}

func TestFillDirectionSummary_Inbound(t *testing.T) {
	ns := "astro-acct"
	svc := "bot"
	clusterFilter := `,cluster="prod"`
	window := networkWindow{Range: "3600s"}

	// Pre-compute the exact PromQL strings the handler will send.
	bucketSel := `{__name__=~"http_server_request_duration_seconds_bucket",k8s_namespace_name="astro-acct",service_name="bot",cluster="prod"}`
	countSel := `{__name__=~"http_server_request_duration_seconds_count",k8s_namespace_name="astro-acct",service_name="bot",cluster="prod"}`
	errSel := `{__name__=~"http_server_request_duration_seconds_count",k8s_namespace_name="astro-acct",service_name="bot",cluster="prod",http_response_status_code=~"4..|5.."}`
	bytesSel := `{__name__=~"http_server_request_size_bytes",k8s_namespace_name="astro-acct",service_name="bot",cluster="prod"}`

	expected := map[string]string{
		`sum(increase(` + countSel + `[3600s]))`:                                 vectorResp("1000"),
		`sum(increase(` + errSel + `[3600s]))`:                                   vectorResp("50"),
		`histogram_quantile(0.5, sum by (le) (rate(` + bucketSel + `[3600s])))`:  vectorResp("0.025"), // 25 ms
		`histogram_quantile(0.95, sum by (le) (rate(` + bucketSel + `[3600s])))`: vectorResp("0.1"),   // 100 ms
		`histogram_quantile(0.99, sum by (le) (rate(` + bucketSel + `[3600s])))`: vectorResp("0.5"),   // 500 ms
		`count(sum by (http_route) (increase(` + countSel + `[3600s])) > 0)`:     vectorResp("7"),
		`sum(increase(` + bytesSel + `[3600s]))`:                                 vectorResp("12345678"),
	}

	srv := httptest.NewServer(&fakePromHandler{
		t:        t,
		expected: expected,
		seen:     map[string]bool{},
	})
	defer srv.Close()

	client := promquery.NewClient(srv.URL, "prod")
	dctx := &deploymentContext{
		Namespace:     ns,
		ServiceName:   svc,
		ClusterFilter: clusterFilter,
	}

	var got DirectionSummary
	if err := fillDirectionSummary(context.Background(), client, directionSpecs["inbound"], dctx, window, &got); err != nil {
		t.Fatalf("fillDirectionSummary: %v", err)
	}

	if got.RequestCount != 1000 {
		t.Errorf("RequestCount = %d, want 1000", got.RequestCount)
	}
	if got.ErrorCount != 50 {
		t.Errorf("ErrorCount = %d, want 50", got.ErrorCount)
	}
	if got.ErrorRate != 0.05 {
		t.Errorf("ErrorRate = %f, want 0.05", got.ErrorRate)
	}
	if got.LatencyP50Ms == nil || *got.LatencyP50Ms != 25 {
		t.Errorf("LatencyP50Ms = %v, want 25", got.LatencyP50Ms)
	}
	if got.LatencyP95Ms == nil || *got.LatencyP95Ms != 100 {
		t.Errorf("LatencyP95Ms = %v, want 100", got.LatencyP95Ms)
	}
	if got.LatencyP99Ms == nil || *got.LatencyP99Ms != 500 {
		t.Errorf("LatencyP99Ms = %v, want 500", got.LatencyP99Ms)
	}
	if got.UniquePeerCount != 7 {
		t.Errorf("UniquePeerCount = %d, want 7", got.UniquePeerCount)
	}
	if got.BytesTotal != 12345678 {
		t.Errorf("BytesTotal = %d, want 12345678", got.BytesTotal)
	}
}

func TestStatusClass(t *testing.T) {
	cases := map[string]string{
		"200": "2xx",
		"201": "2xx",
		"299": "2xx",
		"301": "3xx",
		"404": "4xx",
		"500": "5xx",
		"599": "5xx",
		"100": "other",
		"":    "other",
		"abc": "other",
	}
	for in, want := range cases {
		if got := statusClass(in); got != want {
			t.Errorf("statusClass(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSortFlows(t *testing.T) {
	p95High := 200.0
	p95Low := 50.0
	flows := []NetworkFlow{
		{Peer: "a", RequestCount: 10, ErrorCount: 0, LatencyP95Ms: &p95Low},
		{Peer: "b", RequestCount: 100, ErrorCount: 5, LatencyP95Ms: &p95High},
		{Peer: "c", RequestCount: 50, ErrorCount: 10, LatencyP95Ms: nil},
	}

	t.Run("requests desc", func(t *testing.T) {
		cp := append([]NetworkFlow(nil), flows...)
		sortFlows(cp, "requests")
		if cp[0].Peer != "b" || cp[1].Peer != "c" || cp[2].Peer != "a" {
			t.Errorf("got %v", peers(cp))
		}
	})
	t.Run("errors desc", func(t *testing.T) {
		cp := append([]NetworkFlow(nil), flows...)
		sortFlows(cp, "errors")
		if cp[0].Peer != "c" || cp[1].Peer != "b" || cp[2].Peer != "a" {
			t.Errorf("got %v", peers(cp))
		}
	})
	t.Run("latency_p95 desc with nils last", func(t *testing.T) {
		cp := append([]NetworkFlow(nil), flows...)
		sortFlows(cp, "latency_p95")
		if cp[0].Peer != "b" || cp[1].Peer != "a" || cp[2].Peer != "c" {
			t.Errorf("got %v", peers(cp))
		}
	})
}

func peers(flows []NetworkFlow) []string {
	out := make([]string, len(flows))
	for i, f := range flows {
		out[i] = f.Peer
	}
	return out
}

func TestCollectFlows_InboundHTTP(t *testing.T) {
	window := networkWindow{Range: "3600s"}
	countSel := `{__name__=~"http_server_request_duration_seconds_count",k8s_namespace_name="astro-acct",service_name="bot"}`
	bucketSel := `{__name__=~"http_server_request_duration_seconds_bucket",k8s_namespace_name="astro-acct",service_name="bot"}`
	bytesSel := `{__name__=~"http_server_request_size_bytes",k8s_namespace_name="astro-acct",service_name="bot"}`

	// Two routes: /users gets 200/500 traffic, /health is 200-only.
	countResp := `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{"http_route":"/users","http_response_status_code":"200"},"value":[0,"100"]},
		{"metric":{"http_route":"/users","http_response_status_code":"500"},"value":[0,"5"]},
		{"metric":{"http_route":"/users","http_response_status_code":"404"},"value":[0,"2"]},
		{"metric":{"http_route":"/health","http_response_status_code":"200"},"value":[0,"1000"]}
	]}}`
	p50Resp := `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{"http_route":"/users"},"value":[0,"0.02"]},
		{"metric":{"http_route":"/health"},"value":[0,"0.005"]}
	]}}`
	p95Resp := `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{"http_route":"/users"},"value":[0,"0.1"]},
		{"metric":{"http_route":"/health"},"value":[0,"0.02"]}
	]}}`
	bytesResp := `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{"http_route":"/users"},"value":[0,"500000"]},
		{"metric":{"http_route":"/health"},"value":[0,"100"]}
	]}}`

	expected := map[string]string{
		`sum by (http_route,http_response_status_code) (increase(` + countSel + `[3600s]))`: countResp,
		`histogram_quantile(0.5, sum by (http_route,le) (rate(` + bucketSel + `[3600s])))`:  p50Resp,
		`histogram_quantile(0.95, sum by (http_route,le) (rate(` + bucketSel + `[3600s])))`: p95Resp,
		`sum by (http_route) (increase(` + bytesSel + `[3600s]))`:                           bytesResp,
	}

	srv := httptest.NewServer(&fakePromHandler{t: t, expected: expected, seen: map[string]bool{}})
	defer srv.Close()

	client := promquery.NewClient(srv.URL, "")
	dctx := &deploymentContext{Namespace: "astro-acct", ServiceName: "bot"}

	flows, err := collectFlows(context.Background(), client, directionSpecs["inbound"], dctx, window, "inbound")
	if err != nil {
		t.Fatalf("collectFlows: %v", err)
	}
	if len(flows) != 2 {
		t.Fatalf("expected 2 peers, got %d: %+v", len(flows), flows)
	}

	byPeer := map[string]NetworkFlow{}
	for _, f := range flows {
		byPeer[f.Peer] = f
	}

	users := byPeer["/users"]
	if users.RequestCount != 107 {
		t.Errorf("/users RequestCount = %d, want 107", users.RequestCount)
	}
	if users.ErrorCount != 7 {
		t.Errorf("/users ErrorCount = %d, want 7", users.ErrorCount)
	}
	if users.StatusCodes["2xx"] != 100 || users.StatusCodes["4xx"] != 2 || users.StatusCodes["5xx"] != 5 {
		t.Errorf("/users StatusCodes = %v", users.StatusCodes)
	}
	if users.LatencyP95Ms == nil || *users.LatencyP95Ms != 100 {
		t.Errorf("/users p95 = %v, want 100", users.LatencyP95Ms)
	}
	if users.BytesTotal != 500000 {
		t.Errorf("/users BytesTotal = %d, want 500000", users.BytesTotal)
	}
	if users.PeerKind != "route" {
		t.Errorf("/users PeerKind = %q, want route", users.PeerKind)
	}

	health := byPeer["/health"]
	if health.ErrorCount != 0 || health.ErrorRate != 0 {
		t.Errorf("/health expected no errors, got %+v", health)
	}
}

func TestBuildTimeseriesQL(t *testing.T) {
	httpInbound := directionSpecs["inbound"]
	httpOutbound := directionSpecs["outbound"]
	db := directionSpecs["database"]
	ns := "astro-acct"
	svc := "bot"
	cluster := ""
	rw := "120s"

	cases := []struct {
		name           string
		spec           directionSpec
		metric         string
		groupBy        string
		expectQL       string
		expectLabelKey string
	}{
		{
			name:     "rate total",
			spec:     httpInbound,
			metric:   "rate",
			groupBy:  "",
			expectQL: `sum(rate({__name__=~"http_server_request_duration_seconds_count",k8s_namespace_name="astro-acct",service_name="bot"}[120s]))`,
		},
		{
			name:           "rate by peer (topk wrap)",
			spec:           httpOutbound,
			metric:         "rate",
			groupBy:        "peer",
			expectQL:       `topk(8, sum by (server_address) (rate({__name__=~"http_client_request_duration_seconds_count",k8s_namespace_name="astro-acct",service_name="bot"}[120s])))`,
			expectLabelKey: "server_address",
		},
		{
			name:           "rate by status_class",
			spec:           httpInbound,
			metric:         "rate",
			groupBy:        "status_class",
			expectQL:       `sum by (http_response_status_code) (rate({__name__=~"http_server_request_duration_seconds_count",k8s_namespace_name="astro-acct",service_name="bot"}[120s]))`,
			expectLabelKey: "http_response_status_code",
		},
		{
			name:     "errors total filters status",
			spec:     httpInbound,
			metric:   "errors",
			groupBy:  "",
			expectQL: `sum(rate({__name__=~"http_server_request_duration_seconds_count",k8s_namespace_name="astro-acct",service_name="bot",http_response_status_code=~"4..|5.."}[120s]))`,
		},
		{
			name:     "latency_p95 total",
			spec:     httpInbound,
			metric:   "latency_p95",
			groupBy:  "",
			expectQL: `histogram_quantile(0.95, sum by (le) (rate({__name__=~"http_server_request_duration_seconds_bucket",k8s_namespace_name="astro-acct",service_name="bot"}[120s])))`,
		},
		{
			name:           "latency_p95 by peer (topk wrap)",
			spec:           db,
			metric:         "latency_p95",
			groupBy:        "peer",
			expectQL:       `topk(8, histogram_quantile(0.95, sum by (db_system_name,le) (rate({__name__=~"db_client_operation_duration_seconds_bucket",k8s_namespace_name="astro-acct",service_name="bot"}[120s]))))`,
			expectLabelKey: "db_system_name",
		},
		{
			name:     "bytes total",
			spec:     httpOutbound,
			metric:   "bytes",
			groupBy:  "",
			expectQL: `sum(rate({__name__=~"http_client_request_size_bytes",k8s_namespace_name="astro-acct",service_name="bot"}[120s]))`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotQL, gotLabel := buildTimeseriesQL(tc.spec, tc.metric, tc.groupBy, ns, svc, cluster, rw)
			if gotQL != tc.expectQL {
				t.Errorf("ql:\n got  %s\n want %s", gotQL, tc.expectQL)
			}
			if gotLabel != tc.expectLabelKey {
				t.Errorf("labelKey = %q, want %q", gotLabel, tc.expectLabelKey)
			}
		})
	}
}

func TestFoldStatusClassSeries(t *testing.T) {
	t0 := time.Unix(1700000000, 0)
	t1 := time.Unix(1700000060, 0)
	matrix := []promquery.MatrixSample{
		{
			Labels: map[string]string{"http_response_status_code": "200"},
			Points: []promquery.Point{{Timestamp: t0, Value: 10}, {Timestamp: t1, Value: 20}},
		},
		{
			Labels: map[string]string{"http_response_status_code": "201"},
			Points: []promquery.Point{{Timestamp: t0, Value: 5}},
		},
		{
			Labels: map[string]string{"http_response_status_code": "404"},
			Points: []promquery.Point{{Timestamp: t0, Value: 2}},
		},
		{
			Labels: map[string]string{"http_response_status_code": "500"},
			Points: []promquery.Point{{Timestamp: t1, Value: 3}},
		},
	}

	out := foldStatusClassSeries(matrix)
	if len(out) != 3 {
		t.Fatalf("expected 3 series (2xx,4xx,5xx), got %d: %+v", len(out), out)
	}
	if out[0].Label != "2xx" || out[1].Label != "4xx" || out[2].Label != "5xx" {
		t.Errorf("label order wrong: %s/%s/%s", out[0].Label, out[1].Label, out[2].Label)
	}
	if len(out[0].Points) != 2 {
		t.Fatalf("2xx points = %d, want 2", len(out[0].Points))
	}
	// 200+201 at t0 = 15, 200 at t1 = 20
	if out[0].Points[0].Value != 15 || out[0].Points[1].Value != 20 {
		t.Errorf("2xx values = %+v, want [15, 20]", out[0].Points)
	}
}

func TestFillDirectionSummary_DatabaseSkipsErrorsAndBytes(t *testing.T) {
	window := networkWindow{Range: "60s"}

	// Database direction has no status code and no size metric. Verify
	// the handler omits those queries.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		if strings.Contains(q, "http_response_status_code") {
			t.Errorf("unexpected error-count query for db direction: %q", q)
		}
		if strings.Contains(q, "request_size_bytes") {
			t.Errorf("unexpected bytes query for db direction: %q", q)
		}
		w.Write([]byte(emptyVector()))
	}))
	defer srv.Close()

	client := promquery.NewClient(srv.URL, "")
	dctx := &deploymentContext{Namespace: "astro-acct", ServiceName: "bot"}

	var got DirectionSummary
	if err := fillDirectionSummary(context.Background(), client, directionSpecs["database"], dctx, window, &got); err != nil {
		t.Fatalf("fillDirectionSummary: %v", err)
	}
	if got.ErrorCount != 0 || got.BytesTotal != 0 || got.ErrorRate != 0 {
		t.Errorf("expected zero error/bytes for db: %+v", got)
	}
}
