package deploymentstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// DriftReport is the structured result of drift detection, stored as JSONB.
type DriftReport struct {
	DetectedAt string              `json:"detected_at"`
	Workloads  []DriftResourceItem `json:"workloads"`
	Services   []DriftResourceItem `json:"services"`
	Ingresses  []DriftResourceItem `json:"ingresses"`
	EnvVars    []DriftResourceItem `json:"env_vars,omitempty"`
	Secrets    []DriftResourceItem `json:"secrets,omitempty"`
	Summary    DriftSummary        `json:"summary"`
}

// DriftResourceItem represents a single resource's drift status.
type DriftResourceItem struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`     // deployment, statefulset, service, ingress
	Status   string            `json:"status"`   // match, missing, extra, drift
	Expected map[string]string `json:"expected"` // e.g. {Image: "foo:v1", Replicas: "2"}
	Actual   map[string]string `json:"actual"`   // e.g. {Image: "foo:v1", Replicas: "1/2"}
}

// DriftSummary holds aggregate counts from the drift report.
type DriftSummary struct {
	Total   int `json:"total"`
	Match   int `json:"match"`
	Missing int `json:"missing"`
	Extra   int `json:"extra"`
	Drift   int `json:"drift"`
}

// SaveDriftReport persists a drift report on the deployments row.
func (s *Store) SaveDriftReport(deploymentID string, report *DriftReport) error {
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal drift report: %w", err)
	}
	_, err = s.db.Exec(`
		UPDATE deployments
		SET drift_report = $1, drift_checked_at = NOW()
		WHERE id = $2
	`, reportJSON, deploymentID)
	if err != nil {
		return fmt.Errorf("save drift report: %w", err)
	}
	return nil
}

// GetDriftReport reads the stored drift report for a deployment.
func (s *Store) GetDriftReport(deploymentID string) (*DriftReport, *time.Time, error) {
	var reportJSON sql.NullString
	var checkedAt sql.NullTime
	err := s.db.QueryRow(`
		SELECT drift_report, drift_checked_at
		FROM deployments
		WHERE id = $1
	`, deploymentID).Scan(&reportJSON, &checkedAt)
	if err != nil {
		return nil, nil, fmt.Errorf("get drift report: %w", err)
	}
	if !reportJSON.Valid || reportJSON.String == "" {
		return nil, nil, nil
	}
	var report DriftReport
	if err := json.Unmarshal([]byte(reportJSON.String), &report); err != nil {
		return nil, nil, fmt.Errorf("unmarshal drift report: %w", err)
	}
	var t *time.Time
	if checkedAt.Valid {
		t = &checkedAt.Time
	}
	return &report, t, nil
}
