package admingrpc

import (
	"context"
	"sync"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

// inMemoryCache is a tiny k8scache.Cache fake that lets these tests assert
// which keys the handler deleted. Mirrors the mapCache in the riverqueue tests
// but kept local here so we don't have to re-export it.
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
	c.deletedAt[key] = c.deletedAt[key] + 1
	return nil
}

func TestInvalidateAccountCaches_BustsDeployAndObsKeys(t *testing.T) {
	db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer db.Close()

	// GetActiveDeploymentsByAccount: return two active rows for acct-1.
	mock.ExpectQuery(`SELECT`).
		WithArgs("acct-1").
		WillReturnRows(activeDeploymentRows("dep-a", "dep-b"))

	store := deploymentstore.NewStore(db)
	cache := newInMemoryCache()
	// Pre-seed so we can verify the keys were actually deleted.
	_ = cache.Set(context.Background(), deploycache.KeyFor("acct-1"), []byte("x"), time.Minute)
	_ = cache.Set(context.Background(), obssummary.KeyFor("dep-a"), []byte("x"), time.Minute)
	_ = cache.Set(context.Background(), obssummary.KeyFor("dep-b"), []byte("x"), time.Minute)

	srv := &Server{
		log:         logger.New("error", "json"),
		deployStore: store,
		cache:       cache,
	}

	resp, err := srv.InvalidateAccountCaches(context.Background(), &adminv1.InvalidateAccountCachesRequest{
		AccountID: "acct-1",
	})
	if err != nil {
		t.Fatalf("InvalidateAccountCaches: %v", err)
	}
	if resp.AccountsBusted != 1 {
		t.Errorf("accounts_busted = %d, want 1", resp.AccountsBusted)
	}
	if resp.DeploymentsBusted != 2 {
		t.Errorf("deployments_busted = %d, want 2", resp.DeploymentsBusted)
	}
	for _, k := range []string{
		deploycache.KeyFor("acct-1"),
		obssummary.KeyFor("dep-a"),
		obssummary.KeyFor("dep-b"),
	} {
		if cache.deletedAt[k] == 0 {
			t.Errorf("expected Invalidate(%q) to be called, was not", k)
		}
		if _, ok := cache.data[k]; ok {
			t.Errorf("expected %q to be gone from cache, still present", k)
		}
	}
}

func TestInvalidateAccountCaches_MissingAccountID(t *testing.T) {
	srv := &Server{log: logger.New("error", "json")}
	_, err := srv.InvalidateAccountCaches(context.Background(), &adminv1.InvalidateAccountCachesRequest{})
	if err == nil {
		t.Fatal("expected error for missing account_id, got nil")
	}
}

// activeDeploymentRows returns sqlmock rows matching GetActiveDeploymentsByAccount's
// column list. Only the id column matters to the handler, but we still need to
// fill every scanned field.
func activeDeploymentRows(ids ...string) *sqlmock.Rows {
	now := time.Now()
	cols := []string{
		"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
		"display_name", "deployment_spec_json", "status", "deployed_at", "undeployed_at",
	}
	r := sqlmock.NewRows(cols)
	for _, id := range ids {
		r = r.AddRow(
			id, "acct-1", nil, "my-agent", "build-1", "ns",
			"My Agent", []byte(`{}`), "active", now, (*time.Time)(nil),
		)
	}
	return r
}
