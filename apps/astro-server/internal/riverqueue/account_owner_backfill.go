package riverqueue

import (
	"context"
	"database/sql"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/org"
)

// AccountOwnerBackfillArgs are the job arguments for the account owner backfill.
type AccountOwnerBackfillArgs struct{}

func (AccountOwnerBackfillArgs) Kind() string { return "account.owner_backfill" }

func (AccountOwnerBackfillArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueMaintenance}
}

func init() {
	registerJobKind[AccountOwnerBackfillArgs]()
}

// AccountOwnerBackfillWorker fills accounts.owner_user_id for accounts that
// predate the column, and for any created before the server writes it itself.
// It runs on startup, so a deploy is what picks up whatever the last pass
// missed.
//
// The pass is idempotent. Only NULL owners are touched, so an account whose
// owner was set deliberately is never revisited.
type AccountOwnerBackfillWorker struct {
	river.WorkerDefaults[AccountOwnerBackfillArgs]
	db  *sql.DB
	org *org.Client
	log *logger.Logger
}

func (w *AccountOwnerBackfillWorker) Work(ctx context.Context, _ *river.Job[AccountOwnerBackfillArgs]) error {
	adopted, err := w.adoptSoleMember(ctx)
	if err != nil {
		return err
	}

	resolved, unresolved, err := w.resolveFromWorkOS(ctx)
	if err != nil {
		return err
	}

	w.log.Info("account owner backfill: completed",
		"sole_member", adopted,
		"workos_owner", resolved,
		"unresolved", unresolved,
	)
	return nil
}

// adoptSoleMember owns every account that has exactly one member. That covers
// all personal accounts and any org never shared, and needs no WorkOS call
// because a single member cannot be the wrong answer.
func (w *AccountOwnerBackfillWorker) adoptSoleMember(ctx context.Context) (int64, error) {
	res, err := w.db.ExecContext(ctx, `
		UPDATE accounts a
		   SET owner_user_id = m.user_id, updated_at = now()
		  FROM (
			SELECT account_id, min(user_id) AS user_id
			  FROM account_members
			 GROUP BY account_id
			HAVING count(*) = 1
		  ) m
		 WHERE m.account_id = a.id
		   AND a.owner_user_id IS NULL
		   AND a.deleted_at IS NULL
	`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// resolveFromWorkOS owns the remaining accounts, all of which have several
// members, from the WorkOS `owner` role. Anything WorkOS does not answer
// cleanly is counted and left alone: picking the earliest member instead would
// silently reassign an org that already transferred ownership.
func (w *AccountOwnerBackfillWorker) resolveFromWorkOS(ctx context.Context) (resolved, unresolved int, err error) {
	if w.org == nil {
		remaining, countErr := w.countUnowned(ctx)
		if countErr != nil {
			return 0, 0, countErr
		}
		if remaining > 0 {
			w.log.Warn("account owner backfill: no WorkOS client, multi-member accounts left unowned", "count", remaining)
		}
		return 0, remaining, nil
	}

	const batchSize = 100
	var lastID string

	for {
		rows, qErr := w.db.QueryContext(ctx, `
			SELECT a.id, a.name, ao.workos_org_id
			  FROM accounts a
			  LEFT JOIN account_organizations ao ON ao.account_id = a.id
			 WHERE a.owner_user_id IS NULL
			   AND a.deleted_at IS NULL
			   AND ($1 = '' OR a.id > $1::uuid)
			 ORDER BY a.id
			 LIMIT $2
		`, lastID, batchSize)
		if qErr != nil {
			return resolved, unresolved, qErr
		}

		type candidate struct{ id, name, workosOrgID string }
		var batch []candidate
		for rows.Next() {
			var c candidate
			var workosOrgID sql.NullString
			if scanErr := rows.Scan(&c.id, &c.name, &workosOrgID); scanErr != nil {
				_ = rows.Close()
				return resolved, unresolved, scanErr
			}
			c.workosOrgID = workosOrgID.String
			batch = append(batch, c)
		}
		iterErr := rows.Err()
		_ = rows.Close()
		if iterErr != nil {
			return resolved, unresolved, iterErr
		}
		if len(batch) == 0 {
			break
		}
		lastID = batch[len(batch)-1].id

		for _, c := range batch {
			ownerUserID, reason := w.workosOwner(ctx, c.workosOrgID)
			if ownerUserID == "" {
				w.log.Warn("account owner backfill: owner unresolved, left for review",
					"account", c.name, "account_id", c.id, "reason", reason)
				unresolved++
				continue
			}
			if _, execErr := w.db.ExecContext(ctx, `
				UPDATE accounts SET owner_user_id = $1, updated_at = now()
				 WHERE id = $2 AND owner_user_id IS NULL
			`, ownerUserID, c.id); execErr != nil {
				return resolved, unresolved, execErr
			}
			resolved++
		}
	}
	return resolved, unresolved, nil
}

// workosOwner returns the sole active owner of a WorkOS org, or "" and the
// reason it could not be decided.
func (w *AccountOwnerBackfillWorker) workosOwner(ctx context.Context, workosOrgID string) (userID, reason string) {
	if workosOrgID == "" {
		return "", "account has no WorkOS organization"
	}
	// Owners are few; one page of 100 covers any real org.
	mems, err := w.org.ListMemberships(ctx, workosOrgID, org.ListOpts{Limit: 100})
	if err != nil {
		return "", "WorkOS membership lookup failed: " + err.Error()
	}
	var owners []string
	for _, m := range mems {
		if m.RoleSlug == "owner" && m.Status == "active" {
			owners = append(owners, m.UserID)
		}
	}
	switch len(owners) {
	case 1:
		return owners[0], ""
	case 0:
		return "", "no active WorkOS owner"
	default:
		return "", "several active WorkOS owners"
	}
}

func (w *AccountOwnerBackfillWorker) countUnowned(ctx context.Context) (int, error) {
	var n int
	err := w.db.QueryRowContext(ctx, `
		SELECT count(*) FROM accounts WHERE owner_user_id IS NULL AND deleted_at IS NULL
	`).Scan(&n)
	return n, err
}
