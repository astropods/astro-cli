package commalist

import "testing"

func TestParse(t *testing.T) {
	if got := Parse("10.0.1.10, 10.0.2.10"); len(got) != 2 || got[0] != "10.0.1.10" || got[1] != "10.0.2.10" {
		t.Fatalf("got %v", got)
	}
	if len(Parse(" , ")) != 0 {
		t.Fatal("whitespace-only should produce no tokens")
	}
	if len(Parse("")) != 0 {
		t.Fatal("empty should produce no tokens")
	}
}
