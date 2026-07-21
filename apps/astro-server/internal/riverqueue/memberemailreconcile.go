package riverqueue

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/riverqueue/river"
	"github.com/workos/workos-go/v6/pkg/workos_errors"

	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/memberemails"
)

// workOSNotFound reports a definitive WorkOS 404 (member gone), as opposed to a
// transient failure (rate limit, 5xx, network) that should just retry.
func workOSNotFound(err error) bool {
	var httpErr workos_errors.HTTPError
	return errors.As(err, &httpErr) && httpErr.Code == http.StatusNotFound
}

const (
	// memberEmailReconcileLimit bounds how many missing emails one run resolves,
	// so a large first backfill drains over several runs, not one WorkOS burst.
	memberEmailReconcileLimit = 500
	// memberEmailRetryBackoff is how long an unresolvable user is skipped before
	// the reconcile re-tries them, so we don't re-query WorkOS every run forever.
	memberEmailRetryBackoff = 6 * time.Hour
)

// MemberEmailReconcileArgs backfills and heals the account_member_emails
// mirror: it resolves members that have no recorded email via WorkOS. Auth-time
// capture (login + account create) keeps the mirror fresh going forward; this
// seeds pre-existing members and covers any gaps.
type MemberEmailReconcileArgs struct{}

func (MemberEmailReconcileArgs) Kind() string { return "workos.member_email_reconcile" }

func init() {
	registerJobKind[MemberEmailReconcileArgs]()
}

func (MemberEmailReconcileArgs) InsertOpts() river.InsertOpts {
	// Default queue: a long backfill run shares the general worker pool.
	return river.InsertOpts{Queue: river.QueueDefault}
}

type MemberEmailReconcileWorker struct {
	river.WorkerDefaults[MemberEmailReconcileArgs]
	workosClient *auth.WorkOSClient
	emails       *memberemails.Store
	log          *logger.Logger
}

func (w *MemberEmailReconcileWorker) Work(ctx context.Context, _ *river.Job[MemberEmailReconcileArgs]) error {
	if w.workosClient == nil {
		return nil
	}
	userIDs, err := w.emails.UserIDsMissingEmail(ctx, memberEmailReconcileLimit, time.Now().Add(-memberEmailRetryBackoff))
	if err != nil {
		w.log.Error("member email reconcile: list missing failed", "error", err)
		return nil
	}
	if len(userIDs) == 0 {
		return nil
	}

	var (
		wg       sync.WaitGroup
		resolved atomic.Int64
	)
	sem := make(chan struct{}, 8) // bound concurrent WorkOS calls
	for _, uid := range userIDs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			backoff := func() {
				if aerr := w.emails.RecordReconcileAttempt(ctx, uid); aerr != nil {
					w.log.Warn("member email reconcile: record attempt failed", "user_id", uid, "error", aerr)
				}
			}
			u, err := w.workosClient.GetUser(ctx, uid)
			if err != nil {
				if workOSNotFound(err) {
					// Member is gone from WorkOS — back off like any other
					// definitively-unresolvable user, not re-queried every run.
					backoff()
					return
				}
				// Transient failure (rate limit, 5xx, network) — retry next run so a
				// rate-limited first backfill still drains.
				w.log.Warn("member email reconcile: get user failed", "user_id", uid, "error", err)
				return
			}
			if u == nil || u.Email == "" {
				backoff() // definitively has no email
				return
			}
			if err := w.emails.UpsertWorkOS(ctx, uid, u.Email, u.EmailVerified); err != nil {
				w.log.Warn("member email reconcile: upsert failed", "user_id", uid, "error", err)
				return
			}
			resolved.Add(1)
		}()
	}
	wg.Wait()
	w.log.Info("member email reconcile complete", "missing", len(userIDs), "resolved", resolved.Load())
	return nil
}
