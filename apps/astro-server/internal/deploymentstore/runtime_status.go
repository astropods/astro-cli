package deploymentstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// RuntimeSnapshot is the full live-runtime view of a deployment as observed by
// the deployment controller from its informer caches and persisted as one JSONB
// document in deployment_runtime_status. It is the read model behind
// GET /deployments/:id/runtime — the endpoint deserializes it instead of
// hitting the K8s API per request.
//
// The shape is intentionally generic: every pod and every container of every
// workload is captured (not just a representative), and services are a list —
// so more pods, containers, or services are just more entries in the document,
// never a schema change.
type RuntimeSnapshot struct {
	// Ready / Replicas mirror the primary "agent" workload's observed replica
	// counts — the same top-level numbers the runtime endpoint has always
	// reported.
	Ready    int32 `json:"ready"`
	Replicas int32 `json:"replicas"`
	// ManualIngestions is not populated by the controller yet (it is sourced
	// from a namespace annotation and slated to move to the DB); kept in the
	// shape so the read path can fill it without a schema change.
	ManualIngestions []string          `json:"manual_ingestions,omitempty"`
	Services         []RuntimeService  `json:"services,omitempty"`
	Workloads        []RuntimeWorkload `json:"workloads,omitempty"`
}

// RuntimeService is one managed Service observed in the deployment namespace.
type RuntimeService struct {
	Name      string `json:"name"`
	Type      string `json:"type"` // ClusterIP, NodePort, LoadBalancer
	Component string `json:"component,omitempty"`
}

// RuntimeWorkload is one K8s workload (Deployment, StatefulSet, Job, CronJob)
// with all of its observed pods.
type RuntimeWorkload struct {
	Name      string       `json:"name"`
	Kind      string       `json:"kind"` // Deployment, StatefulSet, Job, CronJob
	Component string       `json:"component,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	Status    string       `json:"status,omitempty"`   // Job / CronJob status vocabulary
	Schedule  string       `json:"schedule,omitempty"` // CronJob cron expression
	Pods      []RuntimePod `json:"pods,omitempty"`
}

// RuntimePod is one observed pod of a workload, with all of its containers.
type RuntimePod struct {
	Name       string             `json:"name"`
	Phase      string             `json:"phase,omitempty"` // K8s pod phase (Running, Pending, …)
	BuildID    string             `json:"build_id,omitempty"`
	CreatedAt  time.Time          `json:"created_at"`
	Containers []RuntimeContainer `json:"containers,omitempty"`
}

// RuntimeContainer mirrors the per-container fields the runtime endpoint exposes
// (init + main containers merged).
type RuntimeContainer struct {
	Name         string `json:"name"`
	State        string `json:"state"` // Running, Waiting, Terminated, Unknown
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	Reason       string `json:"reason,omitempty"`
	Message      string `json:"message,omitempty"`
}

// UpsertRuntimeSnapshot writes (or replaces) the runtime snapshot for a
// deployment. The controller calls this per sync; the whole document is
// replaced atomically in one row.
func (s *Store) UpsertRuntimeSnapshot(deploymentID string, snap RuntimeSnapshot) error {
	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("marshal runtime snapshot: %w", err)
	}
	if _, err := s.db.Exec(`
		INSERT INTO deployment_runtime_status (deployment_id, snapshot, observed_at)
		VALUES ($1, $2, now())
		ON CONFLICT (deployment_id) DO UPDATE
		SET snapshot = EXCLUDED.snapshot, observed_at = EXCLUDED.observed_at
	`, deploymentID, data); err != nil {
		return fmt.Errorf("upsert runtime snapshot %s: %w", deploymentID, err)
	}
	return nil
}

// GetRuntimeSnapshot returns the persisted runtime snapshot and when it was
// observed. Returns (nil, zero, nil) when the controller has not yet observed
// the deployment — callers render an empty runtime (the UI already tolerates a
// still-loading runtime) rather than falling back to a live K8s read.
func (s *Store) GetRuntimeSnapshot(deploymentID string) (*RuntimeSnapshot, time.Time, error) {
	var data []byte
	var observedAt time.Time
	err := s.db.QueryRow(`
		SELECT snapshot, observed_at FROM deployment_runtime_status WHERE deployment_id = $1
	`, deploymentID).Scan(&data, &observedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("query runtime snapshot %s: %w", deploymentID, err)
	}
	var snap RuntimeSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, time.Time{}, fmt.Errorf("unmarshal runtime snapshot %s: %w", deploymentID, err)
	}
	return &snap, observedAt, nil
}

// DeleteRuntimeSnapshot removes the snapshot row for a deployment. The FK
// cascades on deployment delete, so this is only needed to proactively clear a
// torn-down deployment's runtime (e.g. on undeploy) before the row is GC'd.
func (s *Store) DeleteRuntimeSnapshot(deploymentID string) error {
	if _, err := s.db.Exec(`DELETE FROM deployment_runtime_status WHERE deployment_id = $1`, deploymentID); err != nil {
		return fmt.Errorf("delete runtime snapshot %s: %w", deploymentID, err)
	}
	return nil
}
