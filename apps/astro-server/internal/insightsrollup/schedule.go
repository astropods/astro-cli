package insightsrollup

import "time"

const (
	// RollupInterval is how often each account's roll-up runs. Daily, because
	// the unit of work is a completed day — unlike the v1 refresh interval, this
	// is not a throttle on upstream load, since a day is fetched once and then
	// never again.
	RollupInterval = 24 * time.Hour

	// TrailingReRollDays is how far back each tick re-rolls beyond the
	// watermark. Traces arrive late — agents buffer, collectors retry, laptops
	// go offline — so a day is not final the moment it ends. Re-rolling is free
	// of merge hazards because every write is a full replace.
	TrailingReRollDays = 3

	// MaxBackfillDays bounds how far back a cold account is rolled up, matching
	// the 90-day window the page has always shown. History accumulates forward
	// from here; it is not a retention limit.
	MaxBackfillDays = 90

	// MaxDaysPerWindow bounds how many days one upstream query covers. Days come
	// back as buckets of a single range query, so this trades round trips against
	// response size: the whole backfill in one request would be three round trips
	// but an unbounded payload on a busy account.
	MaxDaysPerWindow = 30
)

// Windows splits days into consecutive slices of at most MaxDaysPerWindow, the
// unit one range query and one watermark advance cover.
func Windows(days []time.Time) [][]time.Time {
	var out [][]time.Time
	for start := 0; start < len(days); start += MaxDaysPerWindow {
		out = append(out, days[start:min(start+MaxDaysPerWindow, len(days))])
	}
	return out
}

// DaysToRoll returns the UTC days a tick should roll up, oldest first, given the
// current watermark and the wall clock.
//
// Only *complete* days are returned, and today is deliberately never one of
// them. Insights is a daily report: the fact table holds whole days, the read
// path reports the horizon it has through as_of, and nothing queries upstream on
// the request path. A partial day would break all three, and a later tick would
// have to correct it.
func DaysToRoll(state State, now time.Time) []time.Time {
	lastComplete := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -1)
	earliest := lastComplete.AddDate(0, 0, -(MaxBackfillDays - 1))

	start := earliest
	if !state.RolledUpThrough.IsZero() {
		// Re-roll the trailing window as well as the gap, so late arrivals land
		// without re-reading history.
		start = state.RolledUpThrough.UTC().Truncate(24*time.Hour).AddDate(0, 0, -TrailingReRollDays)
		if start.Before(earliest) {
			start = earliest
		}
	}
	if start.After(lastComplete) {
		return nil
	}

	var days []time.Time
	for d := start; !d.After(lastComplete); d = d.AddDate(0, 0, 1) {
		days = append(days, d)
	}
	return days
}
