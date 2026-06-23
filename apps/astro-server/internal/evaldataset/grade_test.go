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
		{"all good small", 10, 0, "F"},
		{"all good large", 100, 0, "F"},
		{"mostly good without enough bad coverage", 95, 5, "C"},
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

func TestNextGradeProgress(t *testing.T) {
	cases := []struct {
		name        string
		good, bad   int
		wantNext    string
		wantMinProg float64
		wantMaxProg float64
	}{
		{"empty", 0, 0, "", 0, 0},
		// 90/10 → score 0.9 → already A.
		{"at A", 90, 10, "", 1, 1},
		// 100/0 → score ~0.55 → F, ~92% of the way to D (0.55/0.60).
		{"F most of way to D", 100, 0, "D", 0.91, 0.93},
		// 80/20 over 100: goodShare 0.8 × volume 1 × fcm 1 = 0.8 → at B,
		// 0% into the B band (just at threshold).
		{"B at floor", 80, 20, "A", 0, 0.01},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotNext, gotProg := NextGradeProgress(c.good, c.bad)
			if gotNext != c.wantNext {
				t.Fatalf("next = %q, want %q", gotNext, c.wantNext)
			}
			if gotProg < c.wantMinProg || gotProg > c.wantMaxProg {
				t.Fatalf("progress = %f, want in [%f, %f]", gotProg, c.wantMinProg, c.wantMaxProg)
			}
		})
	}
}
