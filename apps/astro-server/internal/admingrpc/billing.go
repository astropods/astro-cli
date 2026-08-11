package admingrpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Vendor dashboards own the money operations. These links hand the operator to
// them rather than reimplementing those flows behind a second audit trail.
const (
	metronomeDashboardURL = "https://app.metronome.com/"
	stripeCustomerURL     = "https://dashboard.stripe.com/customers/"
	stripeTestCustomerURL = "https://dashboard.stripe.com/test/customers/"
)

// GetAccountBillingDetail returns the billing picture only astro holds: status,
// provisioning outcome, and the contract verdict provisioning acted on. A
// failing source becomes a warning, so an outage does not empty the view.
func (s *Server) GetAccountBillingDetail(ctx context.Context, req *adminv1.GetAccountBillingDetailRequest) (*adminv1.GetAccountBillingDetailResponse, error) {
	if req.AccountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}

	resp := &adminv1.GetAccountBillingDetailResponse{
		Billing:  &adminv1.AccountBillingInfo{},
		Enforced: s.billingEnforced,
		Coverage: "unknown",
	}

	var provisionedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(metronome_customer_id, ''),
			COALESCE(stripe_customer_id, ''),
			COALESCE(bifrost_customer_id, ''),
			billing_provisioned_at
		FROM accounts WHERE id = $1
	`, req.AccountID).Scan(
		&resp.Billing.MetronomeCustomerID,
		&resp.Billing.StripeCustomerID,
		&resp.Billing.BifrostCustomerID,
		&provisionedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "account not found: %s", req.AccountID)
	}
	if err != nil {
		return nil, fmt.Errorf("get account billing ids: %w", err)
	}
	if provisionedAt.Valid {
		resp.ProvisionedAt = provisionedAt.Time.Format(time.RFC3339)
	}

	// No status row means never billed, not an error.
	var reason sql.NullString
	var dunningSince, updatedAt sql.NullTime
	err = s.db.QueryRowContext(ctx,
		`SELECT status, reason, dunning_since, alert_active, updated_at
		 FROM account_billing_status WHERE account_id = $1`,
		req.AccountID,
	).Scan(&resp.Billing.Status, &reason, &dunningSince, &resp.Billing.AlertActive, &updatedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get billing status: %w", err)
	}
	if reason.Valid {
		resp.Billing.Reason = reason.String
	}
	if dunningSince.Valid {
		resp.Billing.DunningSince = dunningSince.Time.Format(time.RFC3339)
	}
	if updatedAt.Valid {
		resp.Billing.UpdatedAt = updatedAt.Time.Format(time.RFC3339)
	}

	// From the deployments, not the status: a suspension outlives enforcement
	// being turned off, and the status then no longer says so.
	if s.deployStore != nil {
		suspended, err := s.deployStore.HasBillingSuspended(ctx, req.AccountID)
		if err != nil {
			resp.Warnings = append(resp.Warnings, "workload suspension check failed: "+err.Error())
		} else {
			resp.WorkloadsSuspended = suspended
		}
	}

	job, err := s.billingProvisionJob(ctx, req.AccountID)
	if err != nil {
		resp.Warnings = append(resp.Warnings, "provision job lookup failed: "+err.Error())
	} else {
		resp.ProvisionJob = job
	}

	if resp.Billing.MetronomeCustomerID != "" {
		resp.MetronomeURL = s.metronomeCustomerURL(resp.Billing.MetronomeCustomerID)
		resp.Coverage, resp.Contracts, err = s.contractCoverage(ctx, resp.Billing.MetronomeCustomerID, req.AccountID)
		if err != nil {
			resp.Warnings = append(resp.Warnings, "contract lookup failed: "+err.Error())
		}

		// Spend is read in parts; a failure is reported alongside what was read,
		// not instead of it. The presence flags say which numbers are real.
		if reporter, ok := s.billingProvider.(billing.SpendReporter); ok {
			spend, err := reporter.CustomerSpend(ctx, resp.Billing.MetronomeCustomerID)
			if err != nil {
				resp.Warnings = append(resp.Warnings, "spend lookup failed: "+err.Error())
			}
			if spend.HasCredit || spend.HasCurrentSpend || spend.HasLastInvoice {
				resp.Spend = spendToProto(spend)
			}
		}
	}

	if resp.Billing.StripeCustomerID != "" {
		resp.StripeURL = s.stripeCustomerURL(resp.Billing.StripeCustomerID)
		if s.paymentProvider != nil {
			card, err := s.paymentProvider.DefaultCard(ctx, resp.Billing.StripeCustomerID)
			if err != nil {
				resp.Warnings = append(resp.Warnings, "card lookup failed: "+err.Error())
			} else if card != nil {
				resp.Card = &adminv1.BillingCard{
					Brand:    card.Brand,
					Last4:    card.Last4,
					ExpMonth: int32(card.ExpMonth), //nolint:gosec // month
					ExpYear:  int32(card.ExpYear),  //nolint:gosec // year
				}
			}
		}
	}

	return resp, nil
}

// contractCoverage returns the verdict provisioning would reach. A provider
// that cannot answer reports "unknown"; "none" would read as safe to provision.
func (s *Server) contractCoverage(ctx context.Context, customerID, accountID string) (string, []*adminv1.BillingContract, error) {
	inspector, ok := s.billingProvider.(billing.ContractInspector)
	if !ok {
		return "unknown", nil, nil
	}
	coverage, err := inspector.ContractCoverage(ctx, customerID, accountID)
	if err != nil {
		return "unknown", nil, err
	}
	contracts := make([]*adminv1.BillingContract, 0, len(coverage.Contracts))
	for _, c := range coverage.Contracts {
		out := &adminv1.BillingContract{
			ID:            c.ID,
			Name:          c.Name,
			UniquenessKey: c.UniquenessKey,
			RateCardID:    c.RateCardID,
			Ours:          c.Ours,
		}
		if !c.StartingAt.IsZero() {
			out.StartingAt = c.StartingAt.Format(time.RFC3339)
		}
		if !c.EndingBefore.IsZero() {
			out.EndingBefore = c.EndingBefore.Format(time.RFC3339)
		}
		contracts = append(contracts, out)
	}
	return coverage.State, contracts, nil
}

func spendToProto(s billing.Spend) *adminv1.BillingSpend {
	out := &adminv1.BillingSpend{
		Currency:         s.Currency,
		CreditRemaining:  s.CreditRemaining,
		HasCredit:        s.HasCredit,
		CurrentSpend:     s.CurrentSpend,
		HasCurrentSpend:  s.HasCurrentSpend,
		LastInvoiceTotal: s.LastInvoiceTotal,
		HasLastInvoice:   s.HasLastInvoice,
	}
	if !s.CurrentPeriodEnd.IsZero() {
		out.CurrentPeriodEnd = s.CurrentPeriodEnd.Format(time.RFC3339)
	}
	if !s.LastInvoiceAt.IsZero() {
		out.LastInvoiceAt = s.LastInvoiceAt.Format(time.RFC3339)
	}
	return out
}

// billingProvisionJob returns the latest provisioning attempt. River prunes
// finished jobs, so no row means "nothing recent", not "never ran".
func (s *Server) billingProvisionJob(ctx context.Context, accountID string) (*adminv1.BillingProvisionJob, error) {
	args, err := json.Marshal(map[string]string{"account_id": accountID})
	if err != nil {
		return nil, err
	}

	var job adminv1.BillingProvisionJob
	var createdAt time.Time
	var finalizedAt sql.NullTime
	var lastError sql.NullString
	// errors is jsonb[], not jsonb, so the newest is indexed out in SQL.
	err = s.db.QueryRowContext(ctx, `
		SELECT id, state, attempt, created_at, finalized_at,
		       errors[array_length(errors, 1)] ->> 'error'
		FROM river.river_job
		WHERE kind = 'billing.provision' AND args @> $1::jsonb
		ORDER BY created_at DESC
		LIMIT 1
	`, string(args)).Scan(&job.ID, &job.State, &job.Attempt, &createdAt, &finalizedAt, &lastError)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	job.CreatedAt = createdAt.Format(time.RFC3339)
	if finalizedAt.Valid {
		job.FinalizedAt = finalizedAt.Time.Format(time.RFC3339)
	}
	if lastError.Valid {
		job.LastError = lastError.String
	}
	return &job, nil
}

// metronomeCustomerURL carries the environment as a path segment; without it
// the link opens the default environment and reports no such customer.
func (s *Server) metronomeCustomerURL(customerID string) string {
	if s.metronomeDashboardEnv != "" {
		return metronomeDashboardURL + s.metronomeDashboardEnv + "/customers/" + customerID
	}
	return metronomeDashboardURL + "customers/" + customerID
}

// stripeCustomerURL follows the key's mode; a test key against the live
// dashboard shows "customer not found".
func (s *Server) stripeCustomerURL(customerID string) string {
	if s.paymentProvider != nil && strings.HasPrefix(s.paymentProvider.PublishableKey(), "pk_test_") {
		return stripeTestCustomerURL + customerID
	}
	return stripeCustomerURL + customerID
}

// RetryBillingProvision re-enqueues provisioning, sparing an operator
// hand-written SQL against river. An already-provisioned account is reported,
// not re-enqueued.
func (s *Server) RetryBillingProvision(ctx context.Context, req *adminv1.RetryBillingProvisionRequest) (*adminv1.RetryBillingProvisionResponse, error) {
	if req.AccountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	if s.queue == nil {
		return nil, status.Error(codes.FailedPrecondition, "queue not configured")
	}

	var provisionedAt sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT billing_provisioned_at FROM accounts WHERE id = $1`, req.AccountID,
	).Scan(&provisionedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "account not found: %s", req.AccountID)
	}
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	if provisionedAt.Valid {
		return &adminv1.RetryBillingProvisionResponse{Status: "already_provisioned"}, nil
	}

	if err := s.queue.InsertBillingProvision(ctx, req.AccountID); err != nil {
		return nil, fmt.Errorf("enqueue billing provision: %w", err)
	}

	s.log.Info("Re-enqueued billing provisioning", "account_id", req.AccountID)
	if s.auditStore != nil {
		evt := auditlog.ForAdmin(req.AccountID, "grpc")
		evt.Action = auditlog.BillingRetryProvision
		evt.ResourceType = "account"
		evt.ResourceID = req.AccountID
		evt.Description = "Admin re-enqueued billing provisioning"
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.RetryBillingProvisionResponse{Status: "enqueued"}, nil
}

// ForceBillingResume restores billing-suspended deployments. The resume worker
// only touches what billing stopped, so nothing a customer stopped restarts.
// The billing status is left alone, so the next signal can suspend again.
func (s *Server) ForceBillingResume(ctx context.Context, req *adminv1.ForceBillingResumeRequest) (*adminv1.ForceBillingResumeResponse, error) {
	if req.AccountID == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	if s.queue == nil {
		return nil, status.Error(codes.FailedPrecondition, "queue not configured")
	}
	if s.deployStore == nil {
		return nil, status.Error(codes.FailedPrecondition, "deployment store not configured")
	}

	suspended, err := s.deployStore.HasBillingSuspended(ctx, req.AccountID)
	if err != nil {
		return nil, fmt.Errorf("check suspended workloads: %w", err)
	}
	if !suspended {
		return &adminv1.ForceBillingResumeResponse{Status: "nothing_suspended"}, nil
	}

	if err := s.queue.InsertBillingResume(ctx, req.AccountID); err != nil {
		return nil, fmt.Errorf("enqueue billing resume: %w", err)
	}

	s.log.Info("Forced billing resume", "account_id", req.AccountID)
	if s.auditStore != nil {
		evt := auditlog.ForAdmin(req.AccountID, "grpc")
		evt.Action = auditlog.BillingForceResume
		evt.ResourceType = "account"
		evt.ResourceID = req.AccountID
		evt.Description = "Admin forced billing resume"
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.ForceBillingResumeResponse{Status: "enqueued"}, nil
}
