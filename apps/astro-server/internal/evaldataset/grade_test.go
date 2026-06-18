package evaldataset

import "testing"

func TestGrade(t *testing.T) {
	cases := []struct {
		name string
		good int
		bad  int
		want string
	}{
		{"empty", 0, 0, "—"},
		{"all bad small", 0, 5, "F"},
		{"all good tiny", 1, 0, "F"},
		{"all good small", 10, 0, "E"},
		{"all good large", 100, 0, "C"},
		{"mostly good without enough bad coverage", 95, 5, "B"},
		{"mixed high quality high volume", 90, 10, "A"},
		{"mixed low quality high volume", 10, 90, "F"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Grade(c.good, c.bad); got != c.want {
				t.Fatalf("Grade(%d,%d) = %q, want %q", c.good, c.bad, got, c.want)
			}
		})
	}
}

func TestGradeRewardsBadSampleCoverage(t *testing.T) {
	allGood := Grade(100, 0)
	mixed := Grade(90, 10)
	if mixed >= allGood {
		t.Fatalf("expected 90/10 to grade better than 100/0, got %q vs %q", mixed, allGood)
	}
	if mixed != "A" {
		t.Fatalf("Grade(90,10) = %q, want A", mixed)
	}
}

func TestGradeProgress(t *testing.T) {
	if got := GradeProgress(0, 0); got != 0 {
		t.Fatalf("GradeProgress(0,0) = %f, want 0", got)
	}
	if got := GradeProgress(90, 10); got != 1 {
		t.Fatalf("GradeProgress(90,10) = %f, want 1", got)
	}
	if got := GradeProgress(100, 0); got >= 1 {
		t.Fatalf("GradeProgress(100,0) = %f, want less than 1", got)
	}
}
