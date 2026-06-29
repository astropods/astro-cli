package evaldataset

import "math"

const targetBadShare = 0.10
const targetJudgmentsForFullVolume = 100
const minimumFailureCoverageMultiplier = 0.55

// gradeBands lists letter grades floor-up to ceiling. The first band is the
// fallback used when score is below every floor; the last band is the top
// grade (its floor doubles as the A threshold for progress math).
type gradeBand struct {
	letter string
	floor  float64
}

var gradeBands = []gradeBand{
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
	band, next, ok := nextBandForScore(s)
	if !ok {
		return "", 1
	}
	return next.letter, (s - band.floor) / (next.floor - band.floor)
}

// CasesToNextGrade returns a lower bound for how many additional judged cases
// could reach the next letter grade. The search assumes those future labels can
// include the bad sample coverage that the grade model rewards. No data and
// already-A datasets return nil because there is no concrete next grade target.
func CasesToNextGrade(good, bad int) *int {
	s, ok := score(good, bad)
	if !ok {
		return nil
	}

	_, next, ok := nextBandForScore(s)
	if !ok {
		return nil
	}

	total := good + bad
	maxAdditionalCases := max(200, min(10000, total*10+100))
	searchLimit := reachableAdditionalCaseUpperBound(good, bad, maxAdditionalCases, next.floor)
	for additionalCases := 1; additionalCases <= searchLimit; additionalCases++ {
		if reachesFloorWithAdditionalCases(good, bad, additionalCases, next.floor) {
			result := additionalCases
			return &result
		}
	}

	return nil
}

func reachableAdditionalCaseUpperBound(good, bad, maxAdditionalCases int, floor float64) int {
	upperBound := maxAdditionalCases
	low := 1
	high := maxAdditionalCases

	// Exact reachability has small integer-rounding dips around the target bad
	// share, so the binary pass only finds a safe true upper bound. The forward
	// scan above preserves the first-reachable-case semantics.
	for low <= high {
		mid := low + (high-low)/2
		if reachesFloorWithAdditionalCases(good, bad, mid, floor) {
			upperBound = mid
			high = mid - 1
		} else {
			low = mid + 1
		}
	}

	return upperBound
}

// bandFor returns the highest band whose floor s clears.
func bandFor(s float64) gradeBand {
	band := gradeBands[0]
	for _, b := range gradeBands {
		if s >= b.floor {
			band = b
		}
	}
	return band
}

func nextBandForScore(s float64) (gradeBand, gradeBand, bool) {
	for i := 0; i < len(gradeBands)-1; i++ {
		band := gradeBands[i]
		next := gradeBands[i+1]
		if s < next.floor {
			return band, next, true
		}
	}
	return gradeBands[len(gradeBands)-1], gradeBand{}, false
}

// For a fixed total, score is maximized at the target bad share and falls off
// toward both boundaries. If score() stops being unimodal around targetBadShare,
// revisit this shortcut; the only possible maxima are the boundaries and the
// integers bracketing targetBadShare*total.
func reachesFloorWithAdditionalCases(good, bad, additionalCases int, floor float64) bool {
	total := good + bad + additionalCases
	minBad := bad
	maxBad := bad + additionalCases
	targetBad := targetBadShare * float64(total)
	floorBad := int(math.Floor(targetBad))
	ceilBad := int(math.Ceil(targetBad))

	return reachesFloorWithBadCount(total, minBad, floor) ||
		reachesFloorWithBadCount(total, maxBad, floor) ||
		reachesFloorWithBadCount(total, clampBadCount(floorBad-1, minBad, maxBad), floor) ||
		reachesFloorWithBadCount(total, clampBadCount(floorBad, minBad, maxBad), floor) ||
		reachesFloorWithBadCount(total, clampBadCount(ceilBad, minBad, maxBad), floor) ||
		reachesFloorWithBadCount(total, clampBadCount(ceilBad+1, minBad, maxBad), floor)
}

func reachesFloorWithBadCount(total, bad int, floor float64) bool {
	good := total - bad
	s, ok := score(good, bad)
	return ok && s >= floor
}

func clampBadCount(value, minBad, maxBad int) int {
	return max(minBad, min(maxBad, value))
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
