package admingrpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/billing/noop"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/quota"
)

var deploymentFullColumns = []string{
	"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
	"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
	"status", "error_message", "error_details", "status_changed_at", "current_revision",
	"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
}

func expectClusterGet(mock sqlmock.Sqlmock, clusterID string) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM clusters").
		WithArgs(clusterID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "region", "eks_cluster_name", "eks_cluster_endpoint", "eks_cluster_ca",
			"agent_ingress_domain", "agent_public_ingress_domain", "ingestion_ingress_domain",
			"langfuse_base_url_ext", "langfuse_vpce_ips", "pod_subnet_cidrs", "pod_subnet_ipv6_cidrs",
			"loki_url", "prometheus_url", "tenant_router_internal_url",
			"pull_credential", "pull_key_hash",
			"created_at", "updated_at",
		}).AddRow(
			clusterID, "region-a", "eks-"+clusterID, "https://eks.example", []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n"),
			"agents.example.com", "agents.public.example.com", "ingestion.example.com",
			"http://langfuse.example:3000", "10.0.1.10,10.0.2.10", "10.0.0.0/24,10.1.0.0/24", "",
			"", "", "",
			nil, nil,
			now, now,
		))
}

func expectAccountGetByID(mock sqlmock.Sqlmock, accountID string) {
	now := time.Now()
	mock.ExpectQuery("SELECT a.id, a.name, a.type").
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRow(accountID, "test-org", "organization", nil, nil, now, now)...))
}

type mockAdminJobQueue struct {
	migrateCalls []migrateQueueCall
	failOnCall   int
	failErr      error

	billingProvisionCalls []string
	billingResumeCalls    []string
	billingErr            error
}

type migrateQueueCall struct {
	deploymentID    string
	targetClusterID string
	sourceClusterID string
}

func (m *mockAdminJobQueue) InsertUndeployJob(context.Context, string, string) error { return nil }
func (m *mockAdminJobQueue) InsertWakeUpJob(context.Context, string, string) error   { return nil }
func (m *mockAdminJobQueue) InsertDeployJob(context.Context, string, string) error   { return nil }
func (m *mockAdminJobQueue) TriggerJob(context.Context, string, json.RawMessage) (int64, error) {
	return 0, nil
}
func (m *mockAdminJobQueue) CancelJob(context.Context, int64) error        { return nil }
func (m *mockAdminJobQueue) RetryJob(context.Context, int64) (bool, error) { return true, nil }
func (m *mockAdminJobQueue) PauseQueue(context.Context, string) error      { return nil }
func (m *mockAdminJobQueue) ResumeQueue(context.Context, string) error     { return nil }

func (m *mockAdminJobQueue) InsertBillingProvision(_ context.Context, accountID string) error {
	m.billingProvisionCalls = append(m.billingProvisionCalls, accountID)
	return m.billingErr
}

func (m *mockAdminJobQueue) InsertBillingResume(_ context.Context, accountID string) error {
	m.billingResumeCalls = append(m.billingResumeCalls, accountID)
	return m.billingErr
}

func (m *mockAdminJobQueue) InsertMigrateDeploymentClusterJob(_ context.Context, deploymentID, targetClusterID, sourceClusterID string) error {
	m.migrateCalls = append(m.migrateCalls, migrateQueueCall{deploymentID, targetClusterID, sourceClusterID})
	if m.failOnCall > 0 && len(m.migrateCalls) == m.failOnCall {
		if m.failErr != nil {
			return m.failErr
		}
		return errors.New("enqueue failed")
	}
	return nil
}

// fakeClusterBindings records what an RPC asked the binding table to do, so
// these tests assert the admin action rather than the statements underneath it.
type fakeClusterBindings struct {
	list []account.ClusterBinding

	added    []string // "clusterID:setDefault" per Add
	removed  []string
	defaults []string

	addErr, removeErr, defaultErr error
}

func (f *fakeClusterBindings) List(string) ([]account.ClusterBinding, error) {
	return f.list, nil
}

func (f *fakeClusterBindings) Add(_, clusterID string, setDefault bool) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, fmt.Sprintf("%s:%t", clusterID, setDefault))
	return nil
}

func (f *fakeClusterBindings) Remove(_, clusterID string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, clusterID)
	return nil
}

func (f *fakeClusterBindings) SetDefault(_, clusterID string) error {
	if f.defaultErr != nil {
		return f.defaultErr
	}
	f.defaults = append(f.defaults, clusterID)
	return nil
}

func TestAddAccountCluster_PassesTheDefaultFlagAndReturnsTheList(t *testing.T) {
	bindings := &fakeClusterBindings{
		list: []account.ClusterBinding{{ClusterID: "cluster-a", Region: "region-a", IsDefault: true}},
	}
	s := &Server{bindings: bindings, log: logger.New("error", "json")}

	resp, err := s.AddAccountCluster(context.Background(), &adminv1.AddAccountClusterRequest{
		AccountID:  "acct-1",
		ClusterID:  "cluster-a",
		SetDefault: true,
	})
	if err != nil {
		t.Fatalf("AddAccountCluster: %v", err)
	}
	if len(bindings.added) != 1 || bindings.added[0] != "cluster-a:true" {
		t.Fatalf("added = %v, want [cluster-a:true]", bindings.added)
	}
	if len(resp.Clusters) != 1 || resp.Clusters[0].ClusterID != "cluster-a" || !resp.Clusters[0].IsDefault {
		t.Fatalf("resp = %+v", resp.Clusters)
	}
}

func TestAddAccountCluster_RequiresBothIDs(t *testing.T) {
	bindings := &fakeClusterBindings{}
	s := &Server{bindings: bindings, log: logger.New("error", "json")}

	if _, err := s.AddAccountCluster(context.Background(), &adminv1.AddAccountClusterRequest{AccountID: "acct-1"}); err == nil {
		t.Fatal("expected an error without a cluster id")
	}
	if len(bindings.added) != 0 {
		t.Errorf("nothing should have been bound: %v", bindings.added)
	}
}

// A cluster with deployments still on it stays attached: removing it would
// orphan them, so the operator has to migrate first.
func TestRemoveAccountCluster_RefusesWhileInUse(t *testing.T) {
	s := &Server{
		bindings: &fakeClusterBindings{removeErr: fmt.Errorf("2 deployment(s) still on it: %w", account.ErrClusterInUse)},
		log:      logger.New("error", "json"),
	}

	_, err := s.RemoveAccountCluster(context.Background(), &adminv1.RemoveAccountClusterRequest{
		AccountID: "acct-1",
		ClusterID: "cluster-a",
	})
	if !errors.Is(err, account.ErrClusterInUse) {
		t.Fatalf("err = %v, want ErrClusterInUse", err)
	}
	if !strings.Contains(err.Error(), "move or undeploy") {
		t.Errorf("error should say what to do, got %v", err)
	}
}

func TestRemoveAccountCluster_ReturnsTheRemainingList(t *testing.T) {
	bindings := &fakeClusterBindings{
		list: []account.ClusterBinding{{ClusterID: "cluster-b", Region: "region-b", IsDefault: true}},
	}
	s := &Server{bindings: bindings, log: logger.New("error", "json")}

	resp, err := s.RemoveAccountCluster(context.Background(), &adminv1.RemoveAccountClusterRequest{
		AccountID: "acct-1",
		ClusterID: "cluster-a",
	})
	if err != nil {
		t.Fatalf("RemoveAccountCluster: %v", err)
	}
	if len(bindings.removed) != 1 || bindings.removed[0] != "cluster-a" {
		t.Fatalf("removed = %v, want [cluster-a]", bindings.removed)
	}
	if len(resp.Clusters) != 1 || resp.Clusters[0].ClusterID != "cluster-b" {
		t.Fatalf("resp = %+v", resp.Clusters)
	}
}

func TestSetAccountDefaultCluster_RejectsUnbound(t *testing.T) {
	s := &Server{
		bindings: &fakeClusterBindings{defaultErr: account.ErrClusterNotAllowed},
		log:      logger.New("error", "json"),
	}

	_, err := s.SetAccountDefaultCluster(context.Background(), &adminv1.SetAccountDefaultClusterRequest{
		AccountID: "acct-1",
		ClusterID: "cluster-c",
	})
	if !errors.Is(err, account.ErrClusterNotAllowed) {
		t.Fatalf("err = %v, want ErrClusterNotAllowed", err)
	}
}

// fakeReporter returns a fixed usage map so GetAccount can be tested without a
// real quota checker.
type fakeReporter struct {
	usage map[string]quota.ResourceUsage
}

func (f fakeReporter) Report(_ context.Context, _ string, resources ...string) (map[string]quota.ResourceUsage, error) {
	out := make(map[string]quota.ResourceUsage, len(resources))
	for _, r := range resources {
		out[r] = f.usage[r]
	}
	return out, nil
}

func TestGetAccount_AggregatesBillingLimitsAndMembers(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(`FROM accounts a`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "owner_user_id", "member_count", "has_langfuse",
			"deleted_at", "created_at", "updated_at", "cluster_id", "cluster_count",
			"metronome_customer_id", "stripe_customer_id", "bifrost_customer_id", "langfuse_project_id",
		}).AddRow("acct-1", "Acme", "team", "user-1", 2, true, nil, now, now, "", 0, "mtr-1", "cus-1", "", "lf-proj-1"))

	mock.ExpectQuery(`FROM account_billing_status`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"status", "reason", "dunning_since", "alert_active", "updated_at"}).
			AddRow("suspended", "payment_failed", nil, true, now))

	mock.ExpectQuery(`FROM account_members`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "email", "created_at"}).
			AddRow("user-1", "owner@acme.test", now).
			AddRow("user-2", "member@acme.test", now))

	srv := &Server{
		db:            db,
		log:           logger.New("error", "json"),
		quotaReporter: fakeReporter{usage: map[string]quota.ResourceUsage{"blueprints": {Used: 3, Limit: 10}}},
	}

	resp, err := srv.GetAccount(context.Background(), &adminv1.GetAccountRequest{AccountID: "acct-1"})
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}

	if resp.Account.Name != "Acme" || resp.Account.BillingStatus != "suspended" {
		t.Errorf("account = %+v", resp.Account)
	}
	if resp.Billing.Status != "suspended" || resp.Billing.Reason != "payment_failed" || !resp.Billing.AlertActive {
		t.Errorf("billing = %+v", resp.Billing)
	}
	if resp.Billing.MetronomeCustomerID != "mtr-1" || resp.Billing.StripeCustomerID != "cus-1" || resp.Billing.BifrostCustomerID != "" {
		t.Errorf("linkage ids = %+v", resp.Billing)
	}
	if len(resp.Limits) != len(quota.AllResources) {
		t.Fatalf("limits len = %d, want %d", len(resp.Limits), len(quota.AllResources))
	}
	if resp.Limits[0].Resource != "blueprints" || resp.Limits[0].Used != 3 || resp.Limits[0].Limit != 10 {
		t.Errorf("limits[0] = %+v", resp.Limits[0])
	}
	if len(resp.Members) != 2 || !resp.Members[0].IsOwner || resp.Members[1].IsOwner {
		t.Errorf("members = %+v", resp.Members)
	}
	if resp.Members[0].Email != "owner@acme.test" {
		t.Errorf("owner email = %q", resp.Members[0].Email)
	}
	if resp.LangfuseProjectID != "lf-proj-1" {
		t.Errorf("langfuse project id = %q", resp.LangfuseProjectID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestGetAccount_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`FROM accounts a`).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	srv := &Server{db: db, log: logger.New("error", "json")}
	if _, err := srv.GetAccount(context.Background(), &adminv1.GetAccountRequest{AccountID: "missing"}); err == nil {
		t.Fatal("expected error for missing account, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestGetAccount_RequiresID(t *testing.T) {
	srv := &Server{log: logger.New("error", "json")}
	if _, err := srv.GetAccount(context.Background(), &adminv1.GetAccountRequest{}); err == nil {
		t.Fatal("expected error for empty account_id, got nil")
	}
}

// fakeAliasProvider embeds noop to satisfy billing.BillingProvider and overrides
// GetIngestAliases with a canned response.
type fakeAliasProvider struct {
	*noop.Provider
	aliases []string
	err     error
}

func (f fakeAliasProvider) GetIngestAliases(context.Context, string) ([]string, error) {
	return f.aliases, f.err
}

func expectAccountBillingIDs(mock sqlmock.Sqlmock, accountID, metronomeID, bifrostID string) {
	mock.ExpectQuery(`FROM accounts WHERE id`).
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id", "bifrost_customer_id"}).
			AddRow(metronomeID, bifrostID))
}

func TestGetAccountMetronomeAliases_OK(t *testing.T) {
	db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer db.Close()
	expectAccountBillingIDs(mock, "acct-1", "mtr-1", "bif-1")

	srv := &Server{db: db, log: logger.New("error", "json"),
		billingProvider: fakeAliasProvider{Provider: noop.New(), aliases: []string{"acct-1", "bif-1"}}}

	resp, err := srv.GetAccountMetronomeAliases(context.Background(), &adminv1.GetAccountMetronomeAliasesRequest{AccountID: "acct-1"})
	if err != nil {
		t.Fatalf("GetAccountMetronomeAliases: %v", err)
	}
	if !resp.Configured || !resp.OK || len(resp.Missing) != 0 {
		t.Errorf("resp = %+v, want configured+ok, no missing", resp)
	}
}

func TestGetAccountMetronomeAliases_Missing(t *testing.T) {
	db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer db.Close()
	expectAccountBillingIDs(mock, "acct-1", "mtr-1", "bif-1")

	srv := &Server{db: db, log: logger.New("error", "json"),
		billingProvider: fakeAliasProvider{Provider: noop.New(), aliases: []string{"acct-1"}}}

	resp, err := srv.GetAccountMetronomeAliases(context.Background(), &adminv1.GetAccountMetronomeAliasesRequest{AccountID: "acct-1"})
	if err != nil {
		t.Fatalf("GetAccountMetronomeAliases: %v", err)
	}
	if resp.OK || len(resp.Missing) != 1 || resp.Missing[0] != "bif-1" {
		t.Errorf("resp = %+v, want not-ok with missing bif-1", resp)
	}
}

func TestGetAccountMetronomeAliases_NoCustomer(t *testing.T) {
	db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer db.Close()
	expectAccountBillingIDs(mock, "acct-1", "", "")

	srv := &Server{db: db, log: logger.New("error", "json"),
		billingProvider: fakeAliasProvider{Provider: noop.New()}}

	resp, err := srv.GetAccountMetronomeAliases(context.Background(), &adminv1.GetAccountMetronomeAliasesRequest{AccountID: "acct-1"})
	if err != nil {
		t.Fatalf("GetAccountMetronomeAliases: %v", err)
	}
	if resp.Configured {
		t.Errorf("resp = %+v, want not configured (no metronome customer)", resp)
	}
}

// fakeCustomerStore satisfies aigateway.CustomerStore. A non-empty id makes
// EnsureCustomer short-circuit before any HTTP call.
type fakeCustomerStore struct{ id string }

func (f fakeCustomerStore) GetBifrostCustomerID(string) (string, error) { return f.id, nil }
func (f fakeCustomerStore) SetBifrostCustomerID(string, string) error   { return nil }

func TestRecoverAccountBifrost_ExistingCustomer(t *testing.T) {
	prov := aigateway.NewProvisioner(aigateway.NewClient("http://unused", "", ""), fakeCustomerStore{id: "bif-1"}, nil)
	srv := &Server{log: logger.New("error", "json"), aiGatewayProvisioner: prov}

	resp, err := srv.RecoverAccountBifrost(context.Background(), &adminv1.RecoverAccountBifrostRequest{AccountID: "acct-1"})
	if err != nil {
		t.Fatalf("RecoverAccountBifrost: %v", err)
	}
	if resp.BifrostCustomerID != "bif-1" {
		t.Errorf("customer id = %q, want bif-1", resp.BifrostCustomerID)
	}
}

func TestRecoverAccountBifrost_NotConfigured(t *testing.T) {
	srv := &Server{log: logger.New("error", "json")}
	if _, err := srv.RecoverAccountBifrost(context.Background(), &adminv1.RecoverAccountBifrostRequest{AccountID: "acct-1"}); err == nil {
		t.Fatal("expected error when ai gateway not configured, got nil")
	}
}

func TestRecoverAccountLangfuse_NotConfigured(t *testing.T) {
	srv := &Server{log: logger.New("error", "json")}
	if _, err := srv.RecoverAccountLangfuse(context.Background(), &adminv1.RecoverAccountLangfuseRequest{AccountID: "acct-1"}); err == nil {
		t.Fatal("expected error when langfuse not configured, got nil")
	}
}

// fakeRegisterProvider embeds noop and returns a fixed customer id from
// CreateCustomer so RegisterAccountMetronome can be tested.
type fakeRegisterProvider struct {
	*noop.Provider
	newID string
}

func (f fakeRegisterProvider) CreateCustomer(context.Context, billing.Account) (string, error) {
	return f.newID, nil
}

func TestRegisterAccountMetronome_CreatesAndPersists(t *testing.T) {
	db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer db.Close()

	mock.ExpectQuery(`FROM accounts WHERE id`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"name", "metronome_customer_id", "bifrost_customer_id"}).
			AddRow("Acme", "", "bif-1"))
	mock.ExpectExec(`UPDATE accounts SET metronome_customer_id`).
		WithArgs("mtr-new", sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	srv := &Server{db: db, log: logger.New("error", "json"),
		billingProvider: fakeRegisterProvider{Provider: noop.New(), newID: "mtr-new"}}

	resp, err := srv.RegisterAccountMetronome(context.Background(), &adminv1.RegisterAccountMetronomeRequest{AccountID: "acct-1"})
	if err != nil {
		t.Fatalf("RegisterAccountMetronome: %v", err)
	}
	if resp.MetronomeCustomerID != "mtr-new" {
		t.Errorf("customer id = %q, want mtr-new", resp.MetronomeCustomerID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRegisterAccountMetronome_AlreadyRegistered(t *testing.T) {
	db, mock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	defer db.Close()

	mock.ExpectQuery(`FROM accounts WHERE id`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"name", "metronome_customer_id", "bifrost_customer_id"}).
			AddRow("Acme", "mtr-existing", ""))

	srv := &Server{db: db, log: logger.New("error", "json"),
		billingProvider: fakeRegisterProvider{Provider: noop.New(), newID: "should-not-be-used"}}

	resp, err := srv.RegisterAccountMetronome(context.Background(), &adminv1.RegisterAccountMetronomeRequest{AccountID: "acct-1"})
	if err != nil {
		t.Fatalf("RegisterAccountMetronome: %v", err)
	}
	if resp.MetronomeCustomerID != "mtr-existing" {
		t.Errorf("customer id = %q, want mtr-existing", resp.MetronomeCustomerID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRegisterAccountMetronome_NotConfigured(t *testing.T) {
	srv := &Server{log: logger.New("error", "json")}
	if _, err := srv.RegisterAccountMetronome(context.Background(), &adminv1.RegisterAccountMetronomeRequest{AccountID: "acct-1"}); err == nil {
		t.Fatal("expected error when billing provider not configured, got nil")
	}
}

func TestRecoverAccountObservability_RequireID(t *testing.T) {
	srv := &Server{log: logger.New("error", "json")}
	if _, err := srv.RecoverAccountBifrost(context.Background(), &adminv1.RecoverAccountBifrostRequest{}); err == nil {
		t.Error("expected error for empty account_id (bifrost)")
	}
	if _, err := srv.RecoverAccountLangfuse(context.Background(), &adminv1.RecoverAccountLangfuseRequest{}); err == nil {
		t.Error("expected error for empty account_id (langfuse)")
	}
}
