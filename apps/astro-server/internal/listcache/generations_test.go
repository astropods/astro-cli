package listcache

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestGenerationsStayBoundedAndEvictEarliestExpiry(t *testing.T) {
	g := NewGenerations("test:", time.Hour, 2)
	g.entries["expired"] = generationEntry{value: "old", expiresAt: time.Now().Add(-time.Minute)}
	g.entries["later"] = generationEntry{value: "keep", expiresAt: time.Now().Add(time.Hour)}
	g.remember("new", "value")
	if _, ok := g.entries["expired"]; ok {
		t.Fatal("earliest-expiring generation was not evicted")
	}
	if len(g.entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(g.entries))
	}

	for i := 0; i < 20; i++ {
		g.remember(fmt.Sprintf("account-%d", i), "value")
	}
	if len(g.entries) != 2 {
		t.Fatalf("entries = %d, want bounded registry", len(g.entries))
	}
}

func TestGenerationsWorkWithoutRedis(t *testing.T) {
	g := NewGenerations("test:", time.Hour, 4)
	if err := g.Invalidate(context.Background(), nil, "account-a"); err != nil {
		t.Fatal(err)
	}
	values := g.Values(context.Background(), nil, []string{"account-a"})
	if len(values) != 1 || values[0] == "account-a:0" {
		t.Fatalf("values = %#v, want local generation", values)
	}
}
