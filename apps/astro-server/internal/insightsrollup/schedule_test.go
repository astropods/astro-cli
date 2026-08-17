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
