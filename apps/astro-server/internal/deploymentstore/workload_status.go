package deploymentstore

import (
	"fmt"
	"time"
)

// Workload health phases persisted in deployment_workload_status.phase. These
// describe the observed state of a single workload as derived from live K8s
// state by the deployment controller — distinct from the deployment-level
// lifecycle Status* constants in status.go.
const (
	WorkloadPhasePending     = "pending"     // created, not yet progressing (no pods / not scheduled)
	WorkloadPhaseProgressing = "progressing" // rolling out; some but not all replicas ready
	WorkloadPhaseReady       = "ready"       // desired replicas ready and at the current generation
	WorkloadPhaseComplete    = "complete"    // one-shot workload (Job) finished successfully
	WorkloadPhaseFailed      = "failed"      // terminal failure (image pull, crashloop, deadline, job failed)
	WorkloadPhaseUnknown     = "unknown"     // could not determine (transient read error)
)

// WorkloadStatus is one row of deployment_workload_status: the observed health
// of a single workload within a deployment.
type WorkloadStatus struct {
	DeploymentID       string
	WorkloadName       string
	WorkloadType       string // deployment, statefulset, job, cronjob
	Phase              string // one of WorkloadPhase*
	Reason             string // short machine code, e.g. "ImagePullBackOff", "ProgressDeadlineExceeded"
	Message            string // human-readable detail
	ObservedReady      int
	ObservedDesired    int
	ObservedGeneration int64
	ObservedAt         time.Time
}

// ReplaceWorkloadStatuses replaces the full set of workload-status rows for a
// deployment in one transaction: existing rows are deleted, then the given set
// is inserted. This matches the controller's sync model (each sync derives the
// complete workload set from live K8s state), and prunes rows for workloads that
// no longer exist. Passing an empty slice clears all rows for the deployment.
//
// ObservedAt is stamped by the DB default when zero, so callers may leave it unset.
func (s *Store) ReplaceWorkloadStatuses(deploymentID string, statuses []WorkloadStatus) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		`DELETE FROM deployment_workload_status WHERE deployment_id = $1`,
		deploymentID,
	); err != nil {
		return fmt.Errorf("delete existing: %w", err)
	}

	for _, w := range statuses {
		observedAt := w.ObservedAt
		if observedAt.IsZero() {
			observedAt = time.Now().UTC()
		}
		if _, err := tx.Exec(`
			INSERT INTO deployment_workload_status
				(deployment_id, workload_name, workload_type, phase, reason, message,
				 observed_ready, observed_desired, observed_generation, observed_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		`, deploymentID, w.WorkloadName, w.WorkloadType, w.Phase, w.Reason, w.Message,
			w.ObservedReady, w.ObservedDesired, w.ObservedGeneration, observedAt); err != nil {
			return fmt.Errorf("insert %s/%s: %w", deploymentID, w.WorkloadName, err)
		}
	}

	return tx.Commit()
}

// GetWorkloadStatuses returns the persisted workload statuses for a deployment,
// ordered by workload name. Returns an empty slice when none have been observed.
func (s *Store) GetWorkloadStatuses(deploymentID string) ([]WorkloadStatus, error) {
	rows, err := s.db.Query(`
		SELECT deployment_id, workload_name, workload_type, phase, reason, message,
		       observed_ready, observed_desired, observed_generation, observed_at
		FROM deployment_workload_status
		WHERE deployment_id = $1
		ORDER BY workload_name
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []WorkloadStatus
	for rows.Next() {
		var w WorkloadStatus
		if err := rows.Scan(&w.DeploymentID, &w.WorkloadName, &w.WorkloadType,
			&w.Phase, &w.Reason, &w.Message,
			&w.ObservedReady, &w.ObservedDesired, &w.ObservedGeneration, &w.ObservedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		result = append(result, w)
	}
	return result, rows.Err()
}
