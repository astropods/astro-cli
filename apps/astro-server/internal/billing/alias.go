package billing

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// CustomerIDLookup resolves an account's hosted billing customer id for a
// backend. Satisfied by *account.AccountStore.
type CustomerIDLookup interface {
	GetBillingCustomerID(accountID, backend string) (string, error)
}

// AliasSyncer links an account's Bifrost customer id onto its billing customer
// as an ingest alias, so gateway usage rolls up to the same customer.
type AliasSyncer struct {
	provider BillingProvider
	lookup   CustomerIDLookup
	backend  string
	log      *logger.Logger
}

// NewAliasSyncer constructs an AliasSyncer.
func NewAliasSyncer(provider BillingProvider, lookup CustomerIDLookup, backend string, log *logger.Logger) *AliasSyncer {
	return &AliasSyncer{provider: provider, lookup: lookup, backend: backend, log: log}
}

// SyncBifrostAlias sets the account's billing customer ingest aliases to the
// account id plus its Bifrost customer id. No-op when there is no billing
// customer yet or the backend keeps no customers. Best-effort: failures are
// logged, not returned, so they never block key minting.
func (s *AliasSyncer) SyncBifrostAlias(ctx context.Context, accountID, bifrostCustomerID string) error {
	if s == nil || s.provider == nil {
		return nil
	}
	customerID, err := s.lookup.GetBillingCustomerID(accountID, s.backend)
	if err != nil {
		s.log.Warn("alias: load billing customer for alias sync failed", "error", err, "account_id", accountID)
		return nil
	}
	if customerID == "" {
		return nil
	}
	if err := s.provider.SetIngestAliases(ctx, customerID, []string{accountID, bifrostCustomerID}); err != nil {
		s.log.Warn("alias: sync Bifrost ingest alias failed", "error", err, "account_id", accountID)
		return fmt.Errorf("set ingest aliases: %w", err)
	}
	return nil
}
