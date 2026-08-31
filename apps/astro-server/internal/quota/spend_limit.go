package quota

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// KeySpendLimit is a requestable quota key that is not a resource. Its
// account_limits row holds a ceiling in whole dollars, not a limit.
const KeySpendLimit = "spend_limit"

// IsRequestable reports whether key may be submitted as a quota increase
// request. Wider than IsResource: nothing here counts a spend limit.
func IsRequestable(key string) bool {
	return IsResource(key) || key == KeySpendLimit
}

// SpendCeilingUSD is the highest monthly spend limit an account may set for
// itself: an approved grant, else billing.MaxSelfServeSpendUSD. Never returns
// less than that default, so the -1/0 sentinels and a nil db are safe.
func SpendCeilingUSD(ctx context.Context, db *sql.DB, accountID string) (float64, error) {
	if db == nil {
		return billing.MaxSelfServeSpendUSD, nil
	}
	var granted int64
	err := db.QueryRowContext(ctx,
		`SELECT limit_value FROM account_limits WHERE account_id = $1 AND resource = $2`,
		accountID, KeySpendLimit,
	).Scan(&granted)
	if errors.Is(err, sql.ErrNoRows) {
		return billing.MaxSelfServeSpendUSD, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read granted spend ceiling: %w", err)
	}
	if float64(granted) < billing.MaxSelfServeSpendUSD {
		return billing.MaxSelfServeSpendUSD, nil
	}
	return float64(granted), nil
}
