package insightsrollup

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"
	"time"
)

var testWindow = Window{
	From: time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC),
	To:   time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
}

// Every aggregate funnels through buildWhere, so this is the single place that
// guarantees no query can omit the grain predicate — the one mistake that would
// double-count spend across grains.
func TestBuildWhereAlwaysConstrainsAccountGrainAndDay(t *testing.T) {
	where, args := buildWhere("acct_1", GrainUsage, testWindow, Filter{})

	for _, want := range []string{"account_id = $1", "grain = $2", "day BETWEEN $3 AND $4"} {
		if !strings.Contains(where, want) {
			t.Errorf("where = %q, missing %q", where, want)
		}
	}
	if len(args) != 4 {
		t.Fatalf("args = %v, want 4", args)
	}
	if args[1] != "usage" {
		t.Errorf("grain arg = %v, want usage", args[1])
	}
	// Dates are bound as YYYY-MM-DD so a caller's time-of-day can't shift the
	// window by a day.
	if args[2] != "2026-07-06" || args[3] != "2026-08-03" {
		t.Errorf("day args = %v, %v", args[2], args[3])
	}
}

// hide_sources is the negative form, so an empty filter must include every
// source rather than excluding everything.
func TestBuildWhereOmitsSourceClauseWhenNothingHidden(t *testing.T) {
	where, args := buildWhere("acct_1", GrainUsage, testWindow, Filter{})
	if strings.Contains(where, "source") {
		t.Errorf("where = %q, want no source clause", where)
	}
	if len(args) != 4 {
		t.Errorf("args = %d, want 4", len(args))
	}
}

func TestBuildWhereHidesSources(t *testing.T) {
	where, args := buildWhere("acct_1", GrainUsage, testWindow,
		Filter{HideSources: []string{"claude-code"}})

	if !strings.Contains(where, "source <> ALL($5)") {
		t.Errorf("where = %q, want source exclusion on $5", where)
	}
	if len(args) != 5 {
		t.Fatalf("args = %d, want 5", len(args))
	}
}

// Non-admin visibility is enforced in SQL, not in a Go-side fold, so raw
// per-developer spend never reaches the process.
func TestBuildWhereRestrictsActorInSQL(t *testing.T) {
	where, args := buildWhere("acct_1", GrainUsage, testWindow,
		Filter{RestrictActorKey: "user_123"})

	if !strings.Contains(where, "actor_key = $5") {
		t.Errorf("where = %q, want actor restriction on $5", where)
	}
	if args[4] != "user_123" {
		t.Errorf("actor arg = %v", args[4])
	}
}

// Parameter numbering has to stay consistent when both optional clauses apply,
// or the query binds the wrong values to the wrong columns.
func TestBuildWhereNumbersBothOptionalClauses(t *testing.T) {
	where, args := buildWhere("acct_1", GrainUsage, testWindow,
		Filter{HideSources: []string{"claude-code"}, RestrictActorKey: "user_123"})

	if !strings.Contains(where, "source <> ALL($5)") || !strings.Contains(where, "actor_key = $6") {
		t.Errorf("where = %q, want $5 then $6", where)
	}
	if len(args) != 6 {
		t.Fatalf("args = %d, want 6", len(args))
	}
}

// OnlySources is not the inverse of HideSources — it answers "does this one
// source have spend?", which is how dev-tool presence is gated. Both can apply
// at once, so their parameter numbering has to stay distinct.
func TestBuildWhereOnlySources(t *testing.T) {
	where, args := buildWhere("acct_1", GrainUsage, testWindow,
		Filter{OnlySources: []string{"claude-code"}})

	if !strings.Contains(where, "source = ANY($5)") {
		t.Errorf("where = %q, want source = ANY($5)", where)
	}
	if len(args) != 5 {
		t.Fatalf("args = %d, want 5", len(args))
	}

	where, args = buildWhere("acct_1", GrainUsage, testWindow,
		Filter{HideSources: []string{"agents"}, OnlySources: []string{"claude-code"}, RestrictActorKey: "user_1"})
	for _, want := range []string{"source <> ALL($5)", "source = ANY($6)", "actor_key = $7"} {
		if !strings.Contains(where, want) {
			t.Errorf("where = %q, missing %q", where, want)
		}
	}
	if len(args) != 7 {
		t.Fatalf("args = %d, want 7", len(args))
	}
}

// The zero Grain must never reach SQL.
func TestTotalsRejectsZeroGrain(t *testing.T) {
	store, mock := newMockStore(t)
	if _, err := store.Totals(context.Background(), "acct_1", Grain(""), testWindow, Filter{}); err == nil {
		t.Fatal("Totals with zero grain: want error, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected database calls: %v", err)
	}
}

// Sort keys are looked up in a fixed map, never interpolated from the request.
// An attacker-supplied key must fall back rather than reach the query.
func TestSortColumnsAreWhitelisted(t *testing.T) {
	if _, ok := agentSortColumns["cost_usd; DROP TABLE deployments --"]; ok {
		t.Fatal("injection string resolved to a sort column")
	}
	for key, col := range agentSortColumns {
		if strings.ContainsAny(col, " ;'\"") {
			t.Errorf("agent sort %q maps to unsafe expression %q", key, col)
		}
	}
	for key, col := range peopleSortColumns {
		if strings.ContainsAny(col, " ;'\"") {
			t.Errorf("people sort %q maps to unsafe expression %q", key, col)
		}
	}
}

// Paging clamps to the same bounds v1's request normalizer uses, so v2 can't be
// coaxed into materializing more rows than v1 would.
func TestNormalizeLimit(t *testing.T) {
	tests := []struct{ in, want int }{
		{0, 25}, {-5, 25}, {1, 1}, {25, 25}, {5000, 5000}, {5001, 5000}, {1 << 20, 5000},
	}
	for _, tt := range tests {
		if got := normalizeLimit(tt.in); got != tt.want {
			t.Errorf("normalizeLimit(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// lib/pq cannot bind a bare []string and fails at query time with "unsupported
// type []string, a slice of string". Asserting the clause and the arg count
// missed that entirely, because the conversion happens inside the driver — so
// assert the arg is something a driver can actually convert.
func TestBuildWhereBindsSourceListsAsDriverValues(t *testing.T) {
	for _, tt := range []struct {
		name   string
		filter Filter
	}{
		{"hidden sources", Filter{HideSources: []string{"claude-code"}}},
		{"only sources", Filter{OnlySources: []string{"agents"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, args := buildWhere("acct_1", GrainUsage, testWindow, tt.filter)
			last := args[len(args)-1]
			if _, ok := last.([]string); ok {
				t.Fatalf("source list bound as bare []string; lib/pq cannot convert it")
			}
			if _, ok := last.(driver.Valuer); !ok {
				t.Errorf("source list bound as %T, want a driver.Valuer such as pq.Array", last)
			}
		})
	}
}
