package evaldataset

import "math"

const targetBadShare = 0.10
const gradeAThreshold = 0.85
const targetJudgmentsForFullVolume = 100
const minimumFailureCoverageMultiplier = 0.55

// Grade returns the dataset's A–F letter grade. Volume, good ratio, and bad
// sample coverage all contribute: a dataset with no failures is incomplete as a
// regression baseline, even when every judged success is good. Empty totals
// render as an em dash.
func Grade(good, bad int) string {
	score, ok := score(good, bad)
	if !ok {
		return "—"
	}
	switch {
	case score >= gradeAThreshold:
		return "A"
	case score >= 0.70:
		return "B"
	case score >= 0.55:
		return "C"
	case score >= 0.40:
		return "D"
	case score >= 0.25:
		return "E"
	default:
		return "F"
	}
}

// GradeProgress returns progress toward an A grade as a 0..1 ratio.
func GradeProgress(good, bad int) float64 {
	score, ok := score(good, bad)
	if !ok {
		return 0
	}
	return math.Min(1, score/gradeAThreshold)
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
