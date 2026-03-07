package nsscan

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScanHook is called after each scan with the result. Migration-specific logic
// lives in hooks so it can be removed cleanly after migration.
type ScanHook func(ctx context.Context, result *ScanResult) error

// ScanResult summarises a single reconciliation pass.
type ScanResult struct {
	Tracked  int      // namespaces upserted into namespace_ownership
	Orphaned []string // K8s namespaces with no matching DB deployment
	Stale    []string // DB deployments whose K8s namespace no longer exists
	Drifted  []string // namespace_ownership rows not refreshed this scan
}

// Scanner periodically reconciles DB deployments against K8s namespaces and
// maintains the namespace_ownership table.
type Scanner struct {
	db        *sql.DB
	k8sClient k8s.ClusterClient
	log       *logger.Logger
	hooks     []ScanHook

	stopOnce sync.Once
	stopCh   chan struct{}
}

// New creates a scanner. k8sClient may be nil (scan will skip K8s reconciliation).
func New(db *sql.DB, k8sClient k8s.ClusterClient, log *logger.Logger) *Scanner {
	return &Scanner{
		db:        db,
		k8sClient: k8sClient,
		log:       log,
		stopCh:    make(chan struct{}),
	}
}

// AddHook registers a hook that runs after each scan.
func (s *Scanner) AddHook(h ScanHook) {
	s.hooks = append(s.hooks, h)
}

// Start runs Scan once immediately, then on a ticker. Non-blocking.
func (s *Scanner) Start(ctx context.Context, interval time.Duration) {
	go func() {
		result, err := s.Scan(ctx)
		if err != nil {
			s.log.Error("Initial namespace scan failed", "error", err)
		} else {
			s.logResult(result)
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				r, err := s.Scan(ctx)
				if err != nil {
					s.log.Error("Periodic namespace scan failed", "error", err)
				} else {
					s.logResult(r)
				}
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			}
		}
	}()
}

// Stop signals the background loop to exit.
func (s *Scanner) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

// Scan performs a single idempotent reconciliation pass.
func (s *Scanner) Scan(ctx context.Context) (*ScanResult, error) {
	scanTime := time.Now()
	result := &ScanResult{}

	// 1. Query all active deployments from DB
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, account_id, agent_name, namespace
		FROM deployments
		WHERE status = 'active'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // rows.Close error is insignificant after successful read

	type dbDeploy struct {
		id, accountID, agentName, namespace string
	}
	dbNamespaces := make(map[string]dbDeploy) // namespace -> deploy info
	for rows.Next() {
		var d dbDeploy
		if err := rows.Scan(&d.id, &d.accountID, &d.agentName, &d.namespace); err != nil {
			return nil, err
		}
		dbNamespaces[d.namespace] = d
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 2. Upsert into namespace_ownership
	for ns, d := range dbNamespaces {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO namespace_ownership (namespace, account_id, agent_name, deployment_id, scanned_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (namespace) DO UPDATE
			SET account_id = EXCLUDED.account_id,
			    agent_name = EXCLUDED.agent_name,
			    deployment_id = EXCLUDED.deployment_id,
			    scanned_at = EXCLUDED.scanned_at
		`, ns, d.accountID, d.agentName, d.id, scanTime)
		if err != nil {
			s.log.Warn("Failed to upsert namespace_ownership", "namespace", ns, "error", err)
			continue
		}
		result.Tracked++
	}

	// 3. Detect drifted rows (not refreshed this scan)
	driftRows, err := s.db.QueryContext(ctx, `
		SELECT namespace FROM namespace_ownership WHERE scanned_at < $1
	`, scanTime)
	if err == nil {
		defer driftRows.Close() //nolint:errcheck // rows.Close error is insignificant after successful read
		for driftRows.Next() {
			var ns string
			if err := driftRows.Scan(&ns); err == nil {
				result.Drifted = append(result.Drifted, ns)
			}
		}
	}

	// 4. Cross-reference with K8s (optional)
	if s.k8sClient != nil {
		k8sNamespaces := make(map[string]bool)
		nsList, err := s.k8sClient.Clientset().CoreV1().Namespaces().List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/managed-by=astro-server",
		})
		if err != nil {
			s.log.Warn("Failed to list K8s namespaces during scan", "error", err)
		} else {
			for _, ns := range nsList.Items {
				k8sNamespaces[ns.Name] = true
			}

			// Orphaned: in K8s but not in DB
			for ns := range k8sNamespaces {
				if _, ok := dbNamespaces[ns]; !ok {
					result.Orphaned = append(result.Orphaned, ns)
				}
			}

			// Stale: in DB but not in K8s
			for ns := range dbNamespaces {
				if !k8sNamespaces[ns] {
					result.Stale = append(result.Stale, ns)
				}
			}
		}
	}

	// 5. Run hooks
	for _, h := range s.hooks {
		if err := h(ctx, result); err != nil {
			s.log.Warn("Scan hook failed", "error", err)
		}
	}

	return result, nil
}

func (s *Scanner) logResult(r *ScanResult) {
	s.log.Info("Namespace scan complete",
		"tracked", r.Tracked,
		"orphaned", len(r.Orphaned),
		"stale", len(r.Stale),
		"drifted", len(r.Drifted),
	)
}
