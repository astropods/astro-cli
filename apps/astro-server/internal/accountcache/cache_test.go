package accountcache

import (
	"context"
	"sync"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/insightscache"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
)

type inMemoryCache struct {
	mu        sync.Mutex
	data      map[string][]byte
	deletedAt map[string]int
}

func newInMemoryCache() *inMemoryCache {
	return &inMemoryCache{
		data:      map[string][]byte{},
		deletedAt: map[string]int{},
	}
}

func (c *inMemoryCache) Get(_ context.Context, key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.data[key]
	return v, ok
}

func (c *inMemoryCache) Set(_ context.Context, key string, data []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = data
	return nil
}

func (c *inMemoryCache) Invalidate(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
	c.deletedAt[key]++
	return nil
}

func TestInvalidateAccountMatchesQueenAccountCacheSurface(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT`).
		WithArgs("acct-1").
		WillReturnRows(activeDeploymentRows("dep-a", "dep-b"))

	cache := newInMemoryCache()
	keys := []string{
		deploycache.KeyFor("acct-1"),
		obssummary.KeyFor("dep-a"),
		obssummary.KeyFor("dep-b"),
	}
	for _, v := range insightscache.WarmedVariants {
		keys = append(keys, insightscache.Key("acct-1", v.Endpoint, v.Params))
	}
	for _, key := range keys {
		if err := cache.Set(context.Background(), key, []byte("x"), time.Minute); err != nil {
			t.Fatalf("seed %q: %v", key, err)
		}
	}

	result, err := InvalidateAccount(
		context.Background(),
		cache,
		deploymentstore.NewStore(db),
		"acct-1",
	)
	if err != nil {
		t.Fatalf("InvalidateAccount: %v", err)
	}
	if result.AccountsBusted != 1 {
		t.Errorf("accounts busted = %d, want 1", result.AccountsBusted)
	}
	if result.DeploymentsBusted != 2 {
		t.Errorf("deployments busted = %d, want 2", result.DeploymentsBusted)
	}
	for _, key := range keys {
		if cache.deletedAt[key] == 0 {
			t.Errorf("expected Invalidate(%q) to be called", key)
		}
		if _, ok := cache.data[key]; ok {
			t.Errorf("expected %q to be removed", key)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestInvalidateAccountRequiresAccountID(t *testing.T) {
	if _, err := InvalidateAccount(context.Background(), newInMemoryCache(), nil, ""); err == nil {
		t.Fatal("expected missing account id error")
	}
}

func activeDeploymentRows(ids ...string) *sqlmock.Rows {
	now := time.Now()
	cols := []string{
		"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
		"display_name", "deployment_spec_json", "status", "deployed_at", "undeployed_at",
	}
	r := sqlmock.NewRows(cols)
	for _, id := range ids {
		r.AddRow(
			id, "acct-1", nil, "my-agent", "build-1", "ns",
			"My Agent", []byte(`{}`), "active", now, (*time.Time)(nil),
		)
	}
	return r
}
