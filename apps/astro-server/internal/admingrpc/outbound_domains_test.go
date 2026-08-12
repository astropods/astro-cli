package admingrpc

import (
	"context"
	"database/sql/driver"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

// clusterRowWithProm is clusterRow plus a prometheus_url override, for tests
// that need a cluster with its own isolated metrics backend.
func clusterRowWithProm(id, eksName, prometheusURL string) []driver.Value {
	row := clusterRow(id, "eu-west-1", eksName, "https://"+eksName+".example", true, time.Now())
	row[13] = prometheusURL // loki_url, prometheus_url follow pod_subnet_ipv6_cidrs in clusterColumns
	return row
}

func reqSample(host string, v float64) promquery.Sample {
	return promquery.Sample{Labels: map[string]string{"server_address": host}, Value: v}
}

func depSample(host, ns, svc string) promquery.Sample {
	return promquery.Sample{Labels: map[string]string{
		"server_address":     host,
		"k8s_namespace_name": ns,
		"service_name":       svc,
	}, Value: 1}
}

func TestAggregateOutboundDomains_FoldsHostsOntoDomain(t *testing.T) {
	out := aggregateOutboundDomains(
		[]promquery.Sample{reqSample("api.openai.com", 100), reqSample("cdn.openai.com", 20)},
		nil, 10)

	if len(out) != 1 {
		t.Fatalf("got %d domains, want 1", len(out))
	}
	if out[0].Domain != "openai.com" {
		t.Errorf("domain = %q, want openai.com", out[0].Domain)
	}
	if out[0].RequestCount != 120 {
		t.Errorf("request_count = %d, want 120", out[0].RequestCount)
	}
	if out[0].HostCount != 2 {
		t.Errorf("host_count = %d, want 2", out[0].HostCount)
	}
	// Busiest host first.
	if out[0].Hosts[0] != "api.openai.com" {
		t.Errorf("hosts[0] = %q, want api.openai.com", out[0].Hosts[0])
	}
}

// A deployment calling several hosts under one domain counts once; disjoint
// sets across hosts add up. Taking the max over hosts reports 2 here.
func TestAggregateOutboundDomains_DeploymentCountIsASet(t *testing.T) {
	out := aggregateOutboundDomains(
		[]promquery.Sample{reqSample("api.openai.com", 1), reqSample("cdn.openai.com", 1)},
		[]promquery.Sample{
			depSample("api.openai.com", "acct-a", "agent-1"),
			depSample("api.openai.com", "acct-b", "agent-2"),
			// agent-2 also calls the CDN — must not double-count.
			depSample("cdn.openai.com", "acct-b", "agent-2"),
			depSample("cdn.openai.com", "acct-c", "agent-3"),
		}, 10)

	if len(out) != 1 {
		t.Fatalf("got %d domains, want 1", len(out))
	}
	if out[0].DeploymentCount != 3 {
		t.Errorf("deployment_count = %d, want 3", out[0].DeploymentCount)
	}
}

func TestAggregateOutboundDomains_RoundsExtrapolatedCounts(t *testing.T) {
	out := aggregateOutboundDomains([]promquery.Sample{reqSample("api.acme.io", 99.6)}, nil, 10)
	if out[0].RequestCount != 100 {
		t.Errorf("request_count = %d, want 100 (truncation would give 99)", out[0].RequestCount)
	}
}

func TestAggregateOutboundDomains_KeepsUnreducibleHosts(t *testing.T) {
	// alice.github.io and bob.github.io are unrelated parties under a public
	// suffix; an IP has no registrable domain and stands alone.
	out := aggregateOutboundDomains([]promquery.Sample{
		reqSample("alice.github.io", 3),
		reqSample("bob.github.io", 2),
		reqSample("10.0.14.22", 1),
	}, nil, 10)

	got := map[string]bool{}
	for _, d := range out {
		got[d.Domain] = true
	}
	for _, want := range []string{"alice.github.io", "bob.github.io", "10.0.14.22"} {
		if !got[want] {
			t.Errorf("missing domain %q, got %v", want, got)
		}
	}
}

func TestAggregateOutboundDomains_SkipsEmptyServerAddress(t *testing.T) {
	out := aggregateOutboundDomains([]promquery.Sample{
		{Labels: map[string]string{}, Value: 50},
		reqSample("api.acme.io", 1),
	}, nil, 10)

	if len(out) != 1 || out[0].Domain != "acme.io" {
		t.Fatalf("got %+v, want only acme.io", out)
	}
}

func TestAggregateOutboundDomains_TruncatesHostsButKeepsCount(t *testing.T) {
	var requests []promquery.Sample
	for i := 0; i < outboundMaxHosts+5; i++ {
		requests = append(requests, reqSample(string(rune('a'+i))+".acme.io", float64(i)))
	}

	out := aggregateOutboundDomains(requests, nil, 10)
	if len(out[0].Hosts) != outboundMaxHosts {
		t.Errorf("len(hosts) = %d, want %d", len(out[0].Hosts), outboundMaxHosts)
	}
	if out[0].HostCount != int32(outboundMaxHosts+5) {
		t.Errorf("host_count = %d, want %d", out[0].HostCount, outboundMaxHosts+5)
	}
}

func TestAggregateOutboundDomains_RanksByRequestsAndTruncates(t *testing.T) {
	out := aggregateOutboundDomains([]promquery.Sample{
		reqSample("a.acme.io", 5),
		reqSample("b.beta.io", 50),
		reqSample("c.gamma.io", 500),
	}, nil, 2)

	if len(out) != 2 {
		t.Fatalf("got %d domains, want 2", len(out))
	}
	if out[0].Domain != "gamma.io" || out[1].Domain != "beta.io" {
		t.Errorf("got %q, %q; want gamma.io, beta.io", out[0].Domain, out[1].Domain)
	}
}

// promStub captures the PromQL it receives and answers with an empty vector.
func promStub(t *testing.T, queries *[]string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*queries = append(*queries, r.URL.Query().Get("query"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
}

func TestListOutboundDomains_RequiresPrometheus(t *testing.T) {
	srv := &Server{log: logger.New("error", "json")}
	_, err := srv.ListOutboundDomains(context.Background(), &adminv1.ListOutboundDomainsRequest{})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", status.Code(err))
	}
}

func TestListOutboundDomains_ClampsWindow(t *testing.T) {
	for _, tc := range []struct {
		name string
		days int32
		want string
	}{
		{"defaults", 0, "30d"},
		{"negative defaults", -5, "30d"},
		{"honours request", 7, "7d"},
		{"clamps to retention", 365, "30d"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var queries []string
			prom := promStub(t, &queries)
			defer prom.Close()

			srv := &Server{log: logger.New("error", "json"), promClient: promquery.NewClient(prom.URL, "")}
			resp, err := srv.ListOutboundDomains(context.Background(),
				&adminv1.ListOutboundDomainsRequest{Days: tc.days})
			if err != nil {
				t.Fatalf("ListOutboundDomains: %v", err)
			}
			if resp.Window != tc.want {
				t.Errorf("window = %q, want %q", resp.Window, tc.want)
			}
			for _, q := range queries {
				if !strings.Contains(q, "["+tc.want+"]") {
					t.Errorf("query %q does not use window %q", q, tc.want)
				}
			}
		})
	}
}

// The selector must span every registered cluster: pinning the primary would
// silently drop agents on additional clusters from a fleet-wide query.
func TestListOutboundDomains_SelectorSpansRegisteredClusters(t *testing.T) {
	var queries []string
	prom := promStub(t, &queries)
	defer prom.Close()

	srv := &Server{
		log:        logger.New("error", "json"),
		promClient: promquery.NewClient(prom.URL, "primary-eks"),
		k8sRegistry: k8s.NewRegistryForTest(nil, nil,
			k8s.RegistryConfig{EKSBootstrapName: "primary-eks"}),
	}
	if _, err := srv.ListOutboundDomains(context.Background(),
		&adminv1.ListOutboundDomainsRequest{}); err != nil {
		t.Fatalf("ListOutboundDomains: %v", err)
	}

	if len(queries) != 2 {
		t.Fatalf("got %d queries, want 2", len(queries))
	}
	for _, q := range queries {
		if !strings.Contains(q, `cluster=~"primary-eks"`) {
			t.Errorf("query %q lacks a cluster selector", q)
		}
	}
}

// A registry that cannot answer falls back to the primary rather than
// unscoping, which would let another environment's traffic into the results.
func TestListOutboundDomains_FallsBackToPrimaryWithoutRegistry(t *testing.T) {
	var queries []string
	prom := promStub(t, &queries)
	defer prom.Close()

	srv := &Server{log: logger.New("error", "json"), promClient: promquery.NewClient(prom.URL, "primary-eks")}
	if _, err := srv.ListOutboundDomains(context.Background(),
		&adminv1.ListOutboundDomainsRequest{}); err != nil {
		t.Fatalf("ListOutboundDomains: %v", err)
	}
	for _, q := range queries {
		if !strings.Contains(q, `cluster=~"primary-eks"`) {
			t.Errorf("query %q should be scoped to the primary", q)
		}
	}
}

// A cluster with its own prometheus_url override lives in a different backend
// entirely — a cluster label filter on the default client's query can't reach
// it, so it needs its own separate query against its own client. This guards
// against the bug ListOutboundDomains used to have: querying only the default
// backend (with a combined label selector) silently dropped every cluster
// that had its own isolated Prometheus.
func TestListOutboundDomains_QueriesEachClusterOwnPrometheus(t *testing.T) {
	var primaryQueries, euQueries []string
	primaryProm := promStub(t, &primaryQueries)
	defer primaryProm.Close()
	euProm := promStub(t, &euQueries)
	defer euProm.Close()

	clusterDB, clusterMock, _ := sqlmock.New()
	defer clusterDB.Close()
	clusterMock.ExpectQuery(`SELECT id, region, eks_cluster_name, eks_cluster_endpoint,`).
		WillReturnRows(sqlmock.NewRows(clusterColumns).
			AddRow(clusterRowWithProm("eu-west-1-managed", "eks-eu", euProm.URL)...))

	srv := &Server{
		log:        logger.New("error", "json"),
		promClient: promquery.NewClient(primaryProm.URL, "primary-eks"),
		k8sRegistry: k8s.NewRegistryForTest(nil, clusterstore.New(clusterDB),
			k8s.RegistryConfig{EKSBootstrapName: "primary-eks"}),
	}
	if _, err := srv.ListOutboundDomains(context.Background(),
		&adminv1.ListOutboundDomainsRequest{}); err != nil {
		t.Fatalf("ListOutboundDomains: %v", err)
	}

	if len(primaryQueries) != 2 {
		t.Fatalf("got %d primary queries, want 2", len(primaryQueries))
	}
	for _, q := range primaryQueries {
		if !strings.Contains(q, `cluster=~"primary-eks"`) {
			t.Errorf("primary query %q should be scoped to primary-eks only, got: %s", q, q)
		}
	}
	if len(euQueries) != 2 {
		t.Fatalf("got %d eu queries, want 2 — the eu cluster's own Prometheus was never queried", len(euQueries))
	}
	for _, q := range euQueries {
		if !strings.Contains(q, `cluster=~"eks-eu"`) {
			t.Errorf("eu query %q should be scoped to eks-eu", q)
		}
	}
	if err := clusterMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet cluster expectations: %v", err)
	}
}

func TestClusterSelector(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []k8s.ClusterEntry
		want    string
	}{
		{"no clusters is unscoped", nil, ""},
		{"blank names are unscoped", []k8s.ClusterEntry{{EKSClusterName: ""}}, ""},
		{
			"primary only",
			[]k8s.ClusterEntry{{EKSClusterName: "prod-eks", IsPrimary: true}},
			`{cluster=~"prod-eks"}`,
		},
		{
			// The case that matters: additional clusters must be in the selector
			// or their agents vanish from a fleet-wide aggregation.
			"spans additional clusters",
			[]k8s.ClusterEntry{
				{EKSClusterName: "prod-eks", IsPrimary: true},
				{EKSClusterName: "prod-managed-eu-west-1-a"},
				{EKSClusterName: "prod-managed-us-east-2-a"},
			},
			`{cluster=~"prod-eks|prod-managed-eu-west-1-a|prod-managed-us-east-2-a"}`,
		},
		{
			"escapes regex metacharacters",
			[]k8s.ClusterEntry{{EKSClusterName: "prod.eks+1"}},
			`{cluster=~"prod\\.eks\\+1"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := clusterSelector(tc.entries); got != tc.want {
				t.Errorf("clusterSelector() = %q, want %q", got, tc.want)
			}
		})
	}
}
