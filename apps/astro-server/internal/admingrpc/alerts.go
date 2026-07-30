package admingrpc

import (
	"context"
	"fmt"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/observation"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

// condMeta is the title/severity for a condition name, from the fixed catalog.
type condMeta struct {
	title    string
	severity string
}

func catalogByName() map[string]condMeta {
	m := make(map[string]condMeta, len(observation.Conditions))
	for _, c := range observation.Conditions {
		m[c.Name] = condMeta{title: c.Title, severity: c.Severity.String()}
	}
	return m
}

// ListAlerts returns the fixed observation-alert catalog plus every currently
// tracked breach across all deployments, enriched with deployment/account
// identity and mute state. Mutes with no active breach are surfaced too (state
// "ok", muted=true) so an admin can still see and lift them.
func (s *Server) ListAlerts(ctx context.Context, _ *adminv1.ListAlertsRequest) (*adminv1.ListAlertsResponse, error) {
	now := time.Now()

	catalog := make([]*adminv1.AlertCondition, 0, len(observation.Conditions))
	for _, c := range observation.Conditions {
		catalog = append(catalog, &adminv1.AlertCondition{
			Name:        c.Name,
			Title:       c.Title,
			Description: c.Description,
			Severity:    c.Severity.String(),
		})
	}

	tracked, err := s.alertStore.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	mutes, err := s.alertStore.ListMutes(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("list alert mutes: %w", err)
	}

	// Index mutes by (deployment, condition) so we can flag tracked rows and
	// detect mutes that have no matching breach.
	type muteKey struct{ dep, cond string }
	muteByKey := make(map[muteKey]time.Time, len(mutes))
	for _, m := range mutes {
		muteByKey[muteKey{m.DeploymentID, m.Condition}] = m.MutedUntil
	}

	// Resolve deployment identity in one pass (id → agent/account).
	deps, err := s.deployStore.ListAllWithAccount()
	if err != nil {
		return nil, fmt.Errorf("resolve deployments for alerts: %w", err)
	}
	type depIdentity struct{ agent, accountID, accountName string }
	depByID := make(map[string]depIdentity, len(deps))
	for _, d := range deps {
		depByID[d.ID] = depIdentity{agent: d.AgentName, accountID: d.AccountID, accountName: d.AccountName}
	}
	meta := catalogByName()

	active := make([]*adminv1.ActiveAlert, 0, len(tracked)+len(mutes))
	seenMuteKeys := make(map[muteKey]struct{}, len(tracked))
	for _, t := range tracked {
		state := "pending"
		if t.Notified {
			state = "firing"
		}
		id := depByID[t.DeploymentID]
		cm := meta[t.Condition]
		a := &adminv1.ActiveAlert{
			DeploymentID: t.DeploymentID,
			AgentName:    id.agent,
			AccountID:    id.accountID,
			AccountName:  id.accountName,
			Workload:     t.Workload,
			Condition:    t.Condition,
			Title:        cm.title,
			Severity:     cm.severity,
			State:        state,
			ActiveSince:  t.ActiveSince.UTC().Format(time.RFC3339),
		}
		key := muteKey{t.DeploymentID, t.Condition}
		if until, ok := muteByKey[key]; ok {
			a.Muted = true
			a.MutedUntil = until.UTC().Format(time.RFC3339)
			seenMuteKeys[key] = struct{}{}
		}
		active = append(active, a)
	}

	// Surface mutes that don't correspond to any active breach so they remain
	// visible/manageable.
	for _, m := range mutes {
		key := muteKey{m.DeploymentID, m.Condition}
		if _, ok := seenMuteKeys[key]; ok {
			continue
		}
		id := depByID[m.DeploymentID]
		cm := meta[m.Condition]
		active = append(active, &adminv1.ActiveAlert{
			DeploymentID: m.DeploymentID,
			AgentName:    id.agent,
			AccountID:    id.accountID,
			AccountName:  id.accountName,
			Condition:    m.Condition,
			Title:        cm.title,
			Severity:     cm.severity,
			State:        "ok",
			Muted:        true,
			MutedUntil:   m.MutedUntil.UTC().Format(time.RFC3339),
		})
	}

	return &adminv1.ListAlertsResponse{Catalog: catalog, Active: active}, nil
}

// ClearAlert removes a tracked firing-state row. If the condition is still
// breaching, the next sweep re-detects it (and re-notifies subject to the daily
// cap) — this is a manual reset/acknowledge, not a permanent silence.
func (s *Server) ClearAlert(ctx context.Context, req *adminv1.ClearAlertRequest) (*adminv1.ClearAlertResponse, error) {
	if req.DeploymentID == "" || req.Condition == "" {
		return nil, fmt.Errorf("clear alert: deployment_id and condition are required")
	}
	if err := s.alertStore.Clear(ctx, req.DeploymentID, req.Workload, req.Condition); err != nil {
		return nil, fmt.Errorf("clear alert: %w", err)
	}
	return &adminv1.ClearAlertResponse{}, nil
}

// MuteAlert silences notifications for a (deployment, condition) for
// duration_seconds. Detection continues; only the notification is suppressed.
func (s *Server) MuteAlert(ctx context.Context, req *adminv1.MuteAlertRequest) (*adminv1.MuteAlertResponse, error) {
	if req.DeploymentID == "" || req.Condition == "" {
		return nil, fmt.Errorf("mute alert: deployment_id and condition are required")
	}
	if req.DurationSeconds <= 0 {
		return nil, fmt.Errorf("mute alert: duration_seconds must be positive")
	}
	until := time.Now().Add(time.Duration(req.DurationSeconds) * time.Second)
	if err := s.alertStore.Mute(ctx, req.DeploymentID, req.Condition, until); err != nil {
		return nil, fmt.Errorf("mute alert: %w", err)
	}
	return &adminv1.MuteAlertResponse{}, nil
}

// UnmuteAlert lifts a mute for a (deployment, condition).
func (s *Server) UnmuteAlert(ctx context.Context, req *adminv1.UnmuteAlertRequest) (*adminv1.UnmuteAlertResponse, error) {
	if req.DeploymentID == "" || req.Condition == "" {
		return nil, fmt.Errorf("unmute alert: deployment_id and condition are required")
	}
	if err := s.alertStore.Unmute(ctx, req.DeploymentID, req.Condition); err != nil {
		return nil, fmt.Errorf("unmute alert: %w", err)
	}
	return &adminv1.UnmuteAlertResponse{}, nil
}
