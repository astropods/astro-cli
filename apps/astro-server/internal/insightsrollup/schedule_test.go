package insightsrollup

import (
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Mid-afternoon, to prove the wall-clock time of day never leaks into the
// selected days.
var now = time.Date(2026, 8, 4, 15, 30, 0, 0, time.UTC)

// A cold account backfills the full window, ending yesterday. Today is excluded
// because it is incomplete, and the page reports the horizon instead.
func TestDaysToRollColdAccountBackfillsWindowEndingYesterday(t *testing.T) {
	days := DaysToRoll(State{}, now)

	if len(days) != MaxBackfillDays {
		t.Fatalf("days = %d, want %d", len(days), MaxBackfillDays)
	}
	if want := day(2026, 8, 3); !days[len(days)-1].Equal(want) {
		t.Errorf("last day = %v, want %v (yesterday)", days[len(days)-1], want)
	}
	if want := day(2026, 5, 6); !days[0].Equal(want) {
		t.Errorf("first day = %v, want %v", days[0], want)
	}
}

// A warm account re-rolls the trailing window as well as the gap, so late
// arrivals land without re-reading history.
func TestDaysToRollReRollsTrailingWindow(t *testing.T) {
	days := DaysToRoll(State{RolledUpThrough: day(2026, 8, 2)}, now)

	// Watermark 8-02 minus 3 trailing days → 7-30 .. 8-03 inclusive.
	if len(days) != 5 {
		t.Fatalf("days = %d, want 5: %v", len(days), days)
	}
	if want := day(2026, 7, 30); !days[0].Equal(want) {
		t.Errorf("first day = %v, want %v", days[0], want)
	}
	if want := day(2026, 8, 3); !days[len(days)-1].Equal(want) {
		t.Errorf("last day = %v, want %v", days[len(days)-1], want)
	}
}

// Already current still re-rolls the trailing window rather than doing nothing:
// yesterday's traces can still be arriving.
func TestDaysToRollWhenCurrentStillReRolls(t *testing.T) {
	days := DaysToRoll(State{RolledUpThrough: day(2026, 8, 3)}, now)

	if len(days) != TrailingReRollDays+1 {
		t.Fatalf("days = %d, want %d", len(days), TrailingReRollDays+1)
	}
	if want := day(2026, 8, 3); !days[len(days)-1].Equal(want) {
		t.Errorf("last day = %v, want %v", days[len(days)-1], want)
	}
}

// A watermark ahead of yesterday (clock skew, or a restored backup) must not
// produce a reversed or empty-but-inverted range.
func TestDaysToRollWatermarkInFutureYieldsNothing(t *testing.T) {
	if days := DaysToRoll(State{RolledUpThrough: day(2026, 12, 1)}, now); days != nil {
		t.Fatalf("days = %v, want nil", days)
	}
}

// A very old watermark is clamped to the backfill window rather than walking
// back years one day at a time.
func TestDaysToRollClampsAncientWatermark(t *testing.T) {
	days := DaysToRoll(State{RolledUpThrough: day(2020, 1, 1)}, now)

	if len(days) != MaxBackfillDays {
		t.Fatalf("days = %d, want %d", len(days), MaxBackfillDays)
	}
	if want := day(2026, 5, 6); !days[0].Equal(want) {
		t.Errorf("first day = %v, want %v", days[0], want)
	}
}

// Days must be contiguous and ascending: the worker advances the watermark to
// the last element, so a gap would be silently claimed as rolled.
func TestDaysToRollIsContiguousAndAscending(t *testing.T) {
	days := DaysToRoll(State{RolledUpThrough: day(2026, 7, 1)}, now)

	for i := 1; i < len(days); i++ {
		if want := days[i-1].AddDate(0, 0, 1); !days[i].Equal(want) {
			t.Fatalf("days[%d] = %v, want %v", i, days[i], want)
		}
	}
}

// Windowing groups the planned days for fetching; it must not change which days
// get rolled. A boundary that dropped a day would leave a hole the watermark
// then claims as covered, and one that repeated a day would double the upstream
// cost for no gain.
func TestWindowsCoverEveryDayExactlyOnceInOrder(t *testing.T) {
	for _, count := range []int{
		0, 1,
		MaxDaysPerWindow - 1, MaxDaysPerWindow, MaxDaysPerWindow + 1,
		2 * MaxDaysPerWindow, 2*MaxDaysPerWindow + 1,
		MaxBackfillDays,
	} {
		days := make([]time.Time, count)
		for i := range days {
			days[i] = day(2026, time.January, 1).AddDate(0, 0, i)
		}

		var flat []time.Time
		for _, w := range Windows(days) {
			if len(w) == 0 {
				t.Fatalf("count %d: empty window", count)
			}
			if len(w) > MaxDaysPerWindow {
				t.Fatalf("count %d: window of %d days exceeds %d", count, len(w), MaxDaysPerWindow)
			}
			flat = append(flat, w...)
		}
		if len(flat) != count {
			t.Fatalf("count %d: windows carry %d days", count, len(flat))
		}
		for i := range days {
			if !flat[i].Equal(days[i]) {
				t.Fatalf("count %d: day %d = %s, want %s", count, i,
					flat[i].Format(time.DateOnly), days[i].Format(time.DateOnly))
			}
		}
	}
}

// A window has to be a consecutive run, because the producer turns it into a
// single [first, last+1) range query. A gap inside one would silently pull in
// days the caller never asked to roll.
func TestWindowsAreConsecutiveRuns(t *testing.T) {
	days := DaysToRoll(State{}, now)
	for _, w := range Windows(days) {
		for i := 1; i < len(w); i++ {
			if got := w[i].Sub(w[i-1]); got != 24*time.Hour {
				t.Fatalf("gap of %v between %s and %s", got,
					w[i-1].Format(time.DateOnly), w[i].Format(time.DateOnly))
			}
		}
	}
}

// The steady-state tick is a handful of days, so it must stay a single window
// and therefore a single round trip per query.
func TestSteadyStateTickIsOneWindow(t *testing.T) {
	state := State{RolledUpThrough: day(2026, time.August, 3)}
	if got := len(Windows(DaysToRoll(state, now))); got != 1 {
		t.Errorf("steady-state windows = %d, want 1", got)
	}
}
