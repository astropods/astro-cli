package handlers

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// The embedded interface is nil: only the fakeLimits readers are reached.
type stubProvider struct {
	billing.BillingProvider
	fakeLimits
}

type fakeLimits struct {
	spendLimit  float64
	hasSpend    bool
	usageSpend  float64
	caps        map[billing.UsageMetric]float64
	used        map[billing.UsageMetric]float64
	quantityErr error
}

func (f fakeLimits) CustomerSpendThresholds(context.Context, string) (billing.SpendThresholds, error) {
	return billing.SpendThresholds{
		HasLimit: f.hasSpend,
		Limit:    billing.SpendThreshold{Amount: f.spendLimit},
	}, nil
}

func (f fakeLimits) CustomerSpend(context.Context, string) (billing.Spend, error) {
	return billing.Spend{UsageSpend: f.usageSpend, HasUsageSpend: true}, nil
}

func (f fakeLimits) CustomerUsageThresholds(context.Context, string) (map[billing.UsageMetric]billing.UsageThresholds, error) {
	out := make(map[billing.UsageMetric]billing.UsageThresholds, len(f.caps))
	for m, amount := range f.caps {
		out[m] = billing.UsageThresholds{HasLimit: true, Limit: billing.UsageThreshold{Amount: amount}}
	}
	return out, nil
}

func (f fakeLimits) CustomerMetricUsage(_ context.Context, _ string, m billing.UsageMetric) (float64, error) {
	return f.used[m], f.quantityErr
}

func TestSelfLimitReached_MeasuresEachLimitAgainstThePeriod(t *testing.T) {
	compute := billing.UsageMetricCompute
	gateway := billing.UsageMetricGateway
	cases := []struct {
		name    string
		limits  fakeLimits
		want    bool
		wantErr bool
	}{
		{
			name:   "nothing is capped",
			limits: fakeLimits{},
		},
		{
			name:   "the spend limit is above what the period spent",
			limits: fakeLimits{hasSpend: true, spendLimit: 5000, usageSpend: 30},
		},
		{
			name:   "the spend limit is at what the period spent",
			limits: fakeLimits{hasSpend: true, spendLimit: 5000, usageSpend: 50},
			want:   true,
		},
		{
			name:   "a quantity cap was lowered under what the period used",
			limits: fakeLimits{caps: map[billing.UsageMetric]float64{compute: 4}, used: map[billing.UsageMetric]float64{compute: 9}},
			want:   true,
		},
		{
			name:   "one metric is clear while the other is over",
			limits: fakeLimits{caps: map[billing.UsageMetric]float64{compute: 100, gateway: 5}, used: map[billing.UsageMetric]float64{compute: 9, gateway: 7}},
			want:   true,
		},
		{
			name:   "every cap is above what the period used",
			limits: fakeLimits{hasSpend: true, spendLimit: 5000, usageSpend: 10, caps: map[billing.UsageMetric]float64{compute: 100}, used: map[billing.UsageMetric]float64{compute: 9}},
		},
		{
			name:    "a failed measurement is not evidence the account is clear",
			limits:  fakeLimits{caps: map[billing.UsageMetric]float64{compute: 4}, quantityErr: errors.New("provider unavailable")},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selfLimitReached(context.Background(), tc.limits, "cust_1")
			if tc.wantErr {
				if err == nil {
					t.Fatal("want an error, so the caller leaves the account stopped")
				}
				return
			}
			if err != nil {
				t.Fatalf("selfLimitReached: %v", err)
			}
			if got != tc.want {
				t.Errorf("reached = %v, want %v", got, tc.want)
			}
		})
	}
}

// The alert is archived and recreated on a write, so the provider emits no
// resolve of its own. A swallowed read failure leaves the account stopped.
func TestLiftSelfLimit_ReportsAProviderReadFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("").WithArgs("acct_1").WillReturnRows(sqlmock.NewRows([]string{
		"status", "reason", "dunning_since", "alert_active",
		"force_suspended", "credits_exhausted", "has_payment_method", "pay_link",
		"usage_limit_active", "not_provisioned",
	}).AddRow("suspended", billing.ReasonUsageLimit, nil, false, false, false, false, nil, true, false))

	readFailed := errors.New("provider unreachable")
	provider := stubProvider{fakeLimits: fakeLimits{
		caps:        map[billing.UsageMetric]float64{billing.UsageMetricGateway: 50},
		used:        map[billing.UsageMetric]float64{},
		quantityErr: readFailed,
	}}

	err = liftSelfLimit(context.Background(), billing.NewStatusStore(db, 7), nil, provider, "acct_1", "cus_1")
	if !errors.Is(err, readFailed) {
		t.Fatalf("liftSelfLimit = %v, want the provider read error", err)
	}
}
