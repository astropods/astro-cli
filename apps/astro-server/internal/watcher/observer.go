package watcher

import (
	"context"

	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// AuditObserver enrolls the actor of a deployment audit event as a watcher.
// Registering it on the audit store is what makes watcher membership implicit:
// any handler that already audits a deployment mutation enrolls its actor for
// free, and a new action added to the audit trail is covered without a second
// edit here.
type AuditObserver struct {
	store *Store
	log   *logger.Logger
}

func NewAuditObserver(store *Store, log *logger.Logger) *AuditObserver {
	return &AuditObserver{store: store, log: log}
}

// OnAudit implements auditlog.Observer. Enrollment is best-effort by design:
// the audit row is the record of what happened, and failing to subscribe
// someone to future alerts must not fail the action they just performed.
func (o *AuditObserver) OnAudit(ctx context.Context, e auditlog.Event) {
	if o == nil || o.store == nil {
		return
	}
	if !Enrolls(e.Action, e.ResourceType, string(e.ActorType)) {
		return
	}
	if err := o.store.Record(ctx, e.AccountID, e.ResourceID, e.ActorID, e.Action); err != nil && o.log != nil {
		o.log.Warn("watcher: enroll failed",
			"error", err, "action", e.Action, "deployment_id", e.ResourceID, "user_id", e.ActorID)
	}
}
