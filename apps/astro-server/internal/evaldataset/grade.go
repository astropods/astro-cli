package evaldataset

import "math"

const targetBadShare = 0.10
const targetJudgmentsForFullVolume = 100
const minimumFailureCoverageMultiplier = 0.55

// gradeBands lists letter grades floor-up to ceiling. The first band is the
// fallback used when score is below every floor; the last band is the top
// grade (its floor doubles as the A threshold for progress math).
var gradeBands = []struct {
	letter string
	floor  float64
}{
	{"F", 0.0},
	{"D", 0.60},
	{"C", 0.70},
	{"B", 0.80},
	{"A", 0.90},
}

// Grade returns the dataset's A–F letter grade on the standard US percentage
// scale (A ≥ 90%, B ≥ 80%, C ≥ 70%, D ≥ 60%, else F). The underlying score
// blends volume, good ratio, and bad sample coverage so that a dataset with
// no failures is treated as incomplete even when every judged success is
// good. Empty totals render as an em dash.
func Grade(good, bad int) string {
	s, ok := score(good, bad)
	if !ok {
		return "—"
	}
	return bandFor(s).letter
}

// NextGradeProgress returns the letter of the next grade level up from the
// current one and the dataset's progress within the current band as a 0..1
// ratio. Already at the top grade → ("", 1). No data → ("", 0).
func NextGradeProgress(good, bad int) (string, float64) {
	s, ok := score(good, bad)
	if !ok {
		return "", 0
	}
	for i, band := range gradeBands {
		if i == len(gradeBands)-1 {
			// Already in the top band.
			return "", 1
		}
		next := gradeBands[i+1]
		if s < next.floor {
			return next.letter, (s - band.floor) / (next.floor - band.floor)
		}
	}
	return "", 1
}

// bandFor returns the highest band whose floor s clears.
func bandFor(s float64) struct {
	letter string
	floor  float64
} {
	band := gradeBands[0]
	for _, b := range gradeBands {
		if s >= b.floor {
			band = b
		}
	}
	return band
}

func score(good, bad int) (float64, bool) {
	total := good + bad
	if total <= 0 {
		return 0, false
	}

	goodShare := float64(good) / float64(total)
	volume := math.Min(1, math.Log10(float64(total)+1)/math.Log10(float64(targetJudgmentsForFullVolume)+1))
	badShare := float64(bad) / float64(total)
	failureCoverage := math.Min(1, badShare/targetBadShare)
	failureCoverageMultiplier := minimumFailureCoverageMultiplier + (1-minimumFailureCoverageMultiplier)*failureCoverage

	return goodShare * volume * failureCoverageMultiplier, true
}
