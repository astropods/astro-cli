package admingrpc

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/peerdomain"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	outboundDefaultDays  = 30
	outboundDefaultLimit = 200
	outboundMaxLimit     = 2000
	// Matches managed_vm_retention. A longer window would report a coverage the
	// store cannot have: the response labels results with the window queried,
	// so asking for 90d over a 30d store reads as three months of data.
	outboundMaxDays = 30
	// Only len(hosts) and the first entry are ever read downstream, and a
	// catch-all bucket like cluster.local carries thousands of them.
	outboundMaxHosts = 20
	// A month of high-cardinality series takes minutes; the CLI allows five.
	outboundQueryTimeout = 4 * time.Minute
)

// SetPrometheusClient wires the metrics client used by ListOutboundDomains.
// Nil until called; the RPC then reports FailedPrecondition rather than failing
// obscurely, matching the behaviour of the per-deployment network endpoints
// when PROMETHEUS_URL is unset.
func (s *Server) SetPrometheusClient(c *promquery.Client) {
	s.promClient = c
}

// ListOutboundDomains aggregates every destination agents call, fleet-wide, so
// brand-icon coverage can be prioritised against real traffic. Deliberately
// aggregate: two numbers per domain — how often it is hit and how many
// deployments hit it — and no account or deployment identity, because neither
// informs whether an icon is worth authoring.
func (s *Server) ListOutboundDomains(ctx context.Context, req *adminv1.ListOutboundDomainsRequest) (*adminv1.ListOutboundDomainsResponse, error) {
	if s.promClient == nil {
		return nil, status.Error(codes.FailedPrecondition, "metrics client not configured (PROMETHEUS_URL unset)")
	}

	days := int(req.Days)
	if days <= 0 {
		days = outboundDefaultDays
	}
	if days > outboundMaxDays {
		days = outboundMaxDays
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = outboundDefaultLimit
	}
	if limit > outboundMaxLimit {
		limit = outboundMaxLimit
	}
	window := strconv.Itoa(days) + "d"

	// Registered clusters may not all share one Prometheus — a cluster with
	// its own prometheus_url override needs its own query, since a cluster
	// label filter can't reach series that live in a different backend
	// entirely. Clusters without an override still share one combined query
	// against the default client, same as before.
	groups := s.outboundClusterGroups(ctx)

	var (
		mu                    sync.Mutex
		requests, deployments []promquery.Sample
	)
	g, gCtx := errgroup.WithContext(ctx)
	for i, grp := range groups {
		// No namespace or service_name filter — that is what makes this
		// fleet-wide, where the per-deployment endpoints in handlers/network.go
		// pin both.
		counter := `http_client_request_duration_seconds_count` + grp.selector
		requestsQL := fmt.Sprintf(
			`sum by (server_address) (increase(%s[%s]))`, counter, window)
		// One series per (host, deployment); the fold below dedupes to a set per
		// domain. Collapsing with an outer count here would lose the deployment
		// identity a multi-host vendor needs to be counted once.
		// (k8s_namespace_name, service_name) is the pair Beyla emits per deployment.
		//
		// This is the one part of the RPC with no cardinality ceiling — it scales
		// with total outbound edges, and `limit` cannot bound it because ranking
		// happens after the PSL fold. Its series count is logged either way so a
		// query-limit rejection has a breadcrumb.
		deploymentsQL := fmt.Sprintf(
			`count by (server_address, k8s_namespace_name, service_name) (increase(%s[%s]))`,
			counter, window)

		// Logged per query, not after g.Wait: on a cardinality rejection the error
		// path returns early and errgroup cancels the siblings, so a combined log
		// after the fact would never record the counts a failure needs explaining.
		g.Go(func() error {
			out, err := grp.client.QueryWithTimeout(gCtx, requestsQL, outboundQueryTimeout)
			s.log.Info("outbound domains: outbound domain host query done", "group", i, "window", window, "host_series", len(out), "error", err)
			mu.Lock()
			requests = append(requests, out...)
			mu.Unlock()
			return err
		})
		g.Go(func() error {
			out, err := grp.client.QueryWithTimeout(gCtx, deploymentsQL, outboundQueryTimeout)
			s.log.Info("outbound domains: outbound domain edge query done", "group", i, "window", window, "edge_series", len(out), "error", err)
			mu.Lock()
			deployments = append(deployments, out...)
			mu.Unlock()
			return err
		})
	}
	if err := g.Wait(); err != nil {
		s.log.Error("outbound domains: outbound domain query failed", "error", err, "window", window)
		return nil, status.Errorf(codes.Internal, "query metrics: %v", err)
	}

	return &adminv1.ListOutboundDomainsResponse{
		Domains: aggregateOutboundDomains(requests, deployments, limit),
		Window:  window,
	}, nil
}

// aggregateOutboundDomains folds hostnames onto their registrable domain,
// reusing the same PSL-backed reduction the flows endpoint applies so both
// agree on what a domain is.
func aggregateOutboundDomains(requests, deployments []promquery.Sample, limit int) []*adminv1.OutboundDomain {
	type agg struct {
		requests    int64
		hosts       map[string]int64
		deployments map[string]struct{}
	}
	byDomain := map[string]*agg{}
	get := func(domain string) *agg {
		a := byDomain[domain]
		if a == nil {
			a = &agg{hosts: map[string]int64{}, deployments: map[string]struct{}{}}
			byDomain[domain] = a
		}
		return a
	}

	for _, sample := range requests {
		host := sample.Labels["server_address"]
		if host == "" {
			continue
		}
		// increase() extrapolates, so truncating would bias every host down.
		count := int64(math.Round(sample.Value))
		a := get(outboundDomainKey(host))
		a.requests += count
		a.hosts[host] += count
	}
	for _, sample := range deployments {
		host := sample.Labels["server_address"]
		if host == "" {
			continue
		}
		// Dedupe by deployment: one that calls several hosts under a single
		// domain counts once, and disjoint sets across hosts still add up.
		key := sample.Labels["k8s_namespace_name"] + "/" + sample.Labels["service_name"]
		get(outboundDomainKey(host)).deployments[key] = struct{}{}
	}

	out := make([]*adminv1.OutboundDomain, 0, len(byDomain))
	for domain, a := range byDomain {
		hosts := make([]string, 0, len(a.hosts))
		for h := range a.hosts {
			hosts = append(hosts, h)
		}
		sort.Slice(hosts, func(i, j int) bool {
			if a.hosts[hosts[i]] != a.hosts[hosts[j]] {
				return a.hosts[hosts[i]] > a.hosts[hosts[j]]
			}
			return hosts[i] < hosts[j]
		})
		hostCount := int32(len(hosts)) //nolint:gosec // bounded by distinct series
		if len(hosts) > outboundMaxHosts {
			hosts = hosts[:outboundMaxHosts]
		}
		out = append(out, &adminv1.OutboundDomain{
			Domain:          domain,
			RequestCount:    a.requests,
			DeploymentCount: int32(len(a.deployments)), //nolint:gosec // bounded by fleet size
			Hosts:           hosts,
			HostCount:       hostCount,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RequestCount != out[j].RequestCount {
			return out[i].RequestCount > out[j].RequestCount
		}
		return out[i].Domain < out[j].Domain
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// outboundGroup is one Prometheus backend to query, plus the cluster label
// selector covering every registered cluster that backend answers for.
type outboundGroup struct {
	client   *promquery.Client
	selector string
}

// outboundClusterGroups partitions every registered, enabled cluster by which
// Prometheus client actually answers for it — the shared default for clusters
// without their own prometheus_url, and a distinct group per cluster that has
// one. The default cluster's own coverage always comes from config
// (s.promClient), never solely from its registry row: if that row is missing
// (boot sync hasn't run, or failed) it must not silently drop the default
// cluster's traffic from a fleet-wide query. Same applies if the registry
// can't answer at all.
func (s *Server) outboundClusterGroups(ctx context.Context) []outboundGroup {
	primaryOnly := outboundGroup{
		client:   s.promClient,
		selector: clusterSelector([]k8s.ClusterEntry{{EKSClusterName: s.promClient.Cluster()}}),
	}

	entries, err := s.k8sRegistry.List(ctx)
	if err != nil {
		s.log.Warn("outbound domains: cluster list failed, scoping to primary", "error", err)
		return []outboundGroup{primaryOnly}
	}

	byClient := map[*promquery.Client][]k8s.ClusterEntry{}
	var order []*promquery.Client
	includesPrimary := false
	for _, e := range entries {
		// ForEntry, not the ID-based PrometheusClientFor: entries came from
		// List, so re-resolving by id would cost a redundant clusterstore
		// round trip per cluster.
		client := s.k8sRegistry.PrometheusClientForEntry(e, s.promClient)
		if client == nil {
			continue
		}
		if client == s.promClient {
			includesPrimary = true
		}
		if _, ok := byClient[client]; !ok {
			order = append(order, client)
		}
		byClient[client] = append(byClient[client], e)
	}

	groups := make([]outboundGroup, 0, len(order)+1)
	if !includesPrimary {
		groups = append(groups, primaryOnly)
	}
	for _, client := range order {
		groups = append(groups, outboundGroup{client: client, selector: clusterSelector(byClient[client])})
	}
	return groups
}

func clusterSelector(entries []k8s.ClusterEntry) string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.EKSClusterName != "" {
			names = append(names, regexp.QuoteMeta(e.EKSClusterName))
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return fmt.Sprintf(`{cluster=~%q}`, strings.Join(names, "|"))
}

// outboundDomainKey reduces a host to what callers should group it under,
// falling back to the host itself when it is not a registrable domain (bare
// IPs, single-label internal names).
func outboundDomainKey(host string) string {
	if d := peerdomain.Registrable(host); d != "" {
		return d
	}
	return host
}
