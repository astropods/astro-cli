package clusterstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestValidateID(t *testing.T) {
	cases := []struct {
		id    string
		valid bool
	}{
		{"us-east-1-managed", true},
		{"eu", true},
		{"prod-us-east-1-managed", true},
		{"a1", true},
		{"", false},
		{"a", false},
		{"-leading-dash", false},
		{"trailing-dash-", false},
		{"UPPER", false},
		{"with_underscore", false},
		{"with.dot", false},
	}
	for _, tc := range cases {
		err := ValidateID(tc.id)
		if (err == nil) != tc.valid {
			t.Errorf("ValidateID(%q): valid=%v but err=%v", tc.id, tc.valid, err)
		}
	}
}

// fullCluster returns a Cluster populated with every required field. Tests
// override only the field under test so a single source of "valid" lives here.
func fullCluster() *Cluster {
	return &Cluster{
		ID:                     "us-east-1-managed",
		Region:                 "us-east-1",
		EKSClusterName:         "prod-managed-eks",
		EKSClusterEndpoint:     "https://eks.example",
		EKSClusterCA:           []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n"),
		AgentIngressDomain:     "agents.example.com",
		IngestionIngressDomain: "ingestion.example.com",
		LangfuseBaseURLExt:     "http://langfuse.platform.astroids.ai:3000",
		LangfuseVPCEIPs:        "10.0.1.10,10.0.2.10",
		PodSubnetCIDRs:         "10.0.0.0/24,10.1.0.0/24",
	}
}

// fakeCA returns the same PEM blob fullCluster uses, for tests that compare or
// pass CA explicitly outside the helper.
func fakeCA() []byte {
	return []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n")
}

func TestUpsertFromConfig_InsertsNewCluster(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("INSERT INTO clusters").
		WithArgs(
			"us-east-1-managed", "us-east-1",
			"prod-managed-eks", "https://eks.example", fakeCA(),
			"agents.example.com", "", "ingestion.example.com",
			"http://langfuse.platform.astroids.ai:3000", "10.0.1.10,10.0.2.10", "10.0.0.0/24,10.1.0.0/24", "",
			"", "", "",
			sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	c := fullCluster()
	if err := store.UpsertFromConfig(context.Background(), c, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUpsertFromConfig_RejectsInvalidID(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := New(db)

	c := fullCluster()
	c.ID = "BAD"
	if err := store.UpsertFromConfig(context.Background(), c, false); err == nil {
		t.Error("expected error for invalid id")
	}
}

func TestUpsertFromConfig_RejectsMissingRequiredFields(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := New(db)

	mutate := map[string]func(*Cluster){
		"missing region":                   func(c *Cluster) { c.Region = "" },
		"missing eks name":                 func(c *Cluster) { c.EKSClusterName = "" },
		"missing endpoint":                 func(c *Cluster) { c.EKSClusterEndpoint = "" },
		"missing eks_cluster_ca":           func(c *Cluster) { c.EKSClusterCA = nil },
		"missing agent_ingress_domain":     func(c *Cluster) { c.AgentIngressDomain = "" },
		"missing ingestion_ingress_domain": func(c *Cluster) { c.IngestionIngressDomain = "" },
		"missing langfuse_base_url_ext":    func(c *Cluster) { c.LangfuseBaseURLExt = "" },
		"missing pod_subnet_cidrs":         func(c *Cluster) { c.PodSubnetCIDRs = "" },
	}
	for name, mut := range mutate {
		c := fullCluster()
		mut(c)
		if err := store.UpsertFromConfig(context.Background(), c, false); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

// TestUpsertFromConfig_LangfuseVPCEIPsOptional covers every cluster, not just
// the default one: only clusters that need a PrivateLink netpol exception to
// reach Langfuse set langfuse_vpce_ips at all.
func TestUpsertFromConfig_LangfuseVPCEIPsOptional(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("INSERT INTO clusters").
		WillReturnResult(sqlmock.NewResult(0, 1))

	c := fullCluster()
	c.LangfuseVPCEIPs = ""
	if err := store.UpsertFromConfig(context.Background(), c, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUpsertFromConfig_DefaultClusterAllowsMissingCA(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("INSERT INTO clusters").
		WillReturnResult(sqlmock.NewResult(0, 1))

	c := fullCluster()
	c.EKSClusterCA = nil
	if err := store.UpsertFromConfig(context.Background(), c, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUpsertFromConfig_DefaultClusterStillRequiresIngressFields(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := New(db)

	c := fullCluster()
	c.AgentIngressDomain = ""
	if err := store.UpsertFromConfig(context.Background(), c, true); err == nil {
		t.Error("expected error for missing agent_ingress_domain even on the default cluster")
	}
}

func TestUpsertFromConfig_RejectsInvalidLangfuseBaseURL(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := New(db)

	c := fullCluster()
	c.LangfuseBaseURLExt = "not a url"
	if err := store.UpsertFromConfig(context.Background(), c, false); err == nil {
		t.Fatal("expected error for unparseable langfuse_base_url_ext")
	}
}

func TestUpsertFromConfig_RejectsLangfuseBaseURLWrongScheme(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := New(db)

	c := fullCluster()
	c.LangfuseBaseURLExt = "ftp://langfuse.example:3000"
	if err := store.UpsertFromConfig(context.Background(), c, false); err == nil {
		t.Fatal("expected error for non-http langfuse_base_url_ext scheme")
	}
}

func TestUpsertFromConfig_RejectsLangfuseVPCEWithPrefix(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := New(db)

	c := fullCluster()
	c.LangfuseVPCEIPs = "10.0.1.10/32,10.0.2.10"
	if err := store.UpsertFromConfig(context.Background(), c, false); err == nil {
		t.Fatal("expected error for CIDR notation in langfuse_vpce_ips")
	}
}

func TestUpsertFromConfig_RejectsWhitespaceOnlyLangfuseVPCEIPs(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := New(db)

	c := fullCluster()
	c.LangfuseVPCEIPs = " , "
	if err := store.UpsertFromConfig(context.Background(), c, false); err == nil {
		t.Fatal("expected error for whitespace-only langfuse_vpce_ips")
	}
}

func TestUpsertFromConfig_RejectsWhitespaceOnlyPodSubnetCIDRs(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := New(db)

	c := fullCluster()
	c.PodSubnetCIDRs = "  "
	if err := store.UpsertFromConfig(context.Background(), c, false); err == nil {
		t.Fatal("expected error for whitespace-only pod_subnet_cidrs")
	}
}

func TestUpsertFromConfig_RejectsInvalidPodSubnetCIDR(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := New(db)

	c := fullCluster()
	c.PodSubnetCIDRs = "10.0.0.0-24"
	if err := store.UpsertFromConfig(context.Background(), c, false); err == nil {
		t.Fatal("expected error for invalid pod_subnet_cidrs")
	}
}

func TestGet_Found(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM clusters WHERE id = \\$1").
		WithArgs("us-east-1-managed").
		WillReturnRows(fullClusterRow(clusterRows(), "us-east-1-managed", "us-east-1", "prod-managed-eks", "https://eks.example", now))

	c, err := store.Get(context.Background(), "us-east-1-managed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ID != "us-east-1-managed" || c.Region != "us-east-1" {
		t.Errorf("got %+v", c)
	}
}

func TestGet_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectQuery("SELECT .+ FROM clusters WHERE id = \\$1").
		WithArgs("missing").
		WillReturnRows(clusterRows())

	_, err := store.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestList_All(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	now := time.Now()
	rows := clusterRows()
	fullClusterRow(rows, "a", "ap-southeast-2", "eks-a", "https://a", now)
	fullClusterRow(rows, "b", "us-east-1", "eks-b", "https://b", now)
	mock.ExpectQuery("SELECT .+ FROM clusters ORDER BY region ASC, id ASC").
		WillReturnRows(rows)

	cs, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cs) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(cs))
	}
}

func TestEnsurePullCredential_Backfills(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("UPDATE clusters SET pull_credential = \\$1, pull_key_hash = \\$2").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "eu-west-1-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	generated, err := store.EnsurePullCredential(context.Background(), "eu-west-1-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !generated {
		t.Error("expected generated=true when the guarded UPDATE affects a row")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestEnsurePullCredential_NoOpWhenAlreadyIssued(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("UPDATE clusters SET pull_credential = \\$1, pull_key_hash = \\$2").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "eu-west-1-a").
		WillReturnResult(sqlmock.NewResult(0, 0))

	generated, err := store.EnsurePullCredential(context.Background(), "eu-west-1-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if generated {
		t.Error("expected generated=false when the row already has a credential")
	}
}

func TestDeregister_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("DELETE FROM clusters WHERE id = \\$1").
		WithArgs("us-east-1-managed").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.Deregister(context.Background(), "us-east-1-managed"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeregister_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("DELETE FROM clusters WHERE id = \\$1").
		WithArgs("missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := store.Deregister(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestDeregister_InUse covers the case where a non-undeployed deployment is
// still genuinely blocking: the self-heal clears stale undeployed rows, but
// the retry hits the same FK because a live deployment remains.
func TestDeregister_InUse(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("DELETE FROM clusters WHERE id = \\$1").
		WithArgs("us-east-1-managed").
		WillReturnError(&pq.Error{Code: pgForeignKeyViolation, Constraint: "deployments_cluster_id_fkey"})
	mock.ExpectExec("UPDATE deployments SET cluster_id = NULL WHERE cluster_id = \\$1 AND status = 'undeployed'").
		WithArgs("us-east-1-managed").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM clusters WHERE id = \\$1").
		WithArgs("us-east-1-managed").
		WillReturnError(&pq.Error{Code: pgForeignKeyViolation, Constraint: "deployments_cluster_id_fkey"})

	if err := store.Deregister(context.Background(), "us-east-1-managed"); !errors.Is(err, ErrInUseByDeployments) {
		t.Errorf("expected ErrInUseByDeployments, got %v", err)
	}
}

// TestDeregister_SelfHealsStaleUndeployedRows covers the case this feature
// exists for: the only blockers are deployments already undeployed before
// updateStatusTx started clearing cluster_id on undeploy. The self-heal
// clears them and the retried delete succeeds without any manual DB fix.
func TestDeregister_SelfHealsStaleUndeployedRows(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("DELETE FROM clusters WHERE id = \\$1").
		WithArgs("us-east-1-managed").
		WillReturnError(&pq.Error{Code: pgForeignKeyViolation, Constraint: "deployments_cluster_id_fkey"})
	mock.ExpectExec("UPDATE deployments SET cluster_id = NULL WHERE cluster_id = \\$1 AND status = 'undeployed'").
		WithArgs("us-east-1-managed").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM clusters WHERE id = \\$1").
		WithArgs("us-east-1-managed").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.Deregister(context.Background(), "us-east-1-managed"); err != nil {
		t.Errorf("expected self-heal to unblock deletion, got %v", err)
	}
}

func TestDeregister_InUseByAccounts(t *testing.T) {
	for _, constraint := range []string{"account_clusters_cluster_id_fkey", "accounts_cluster_id_fkey"} {
		t.Run(constraint, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			store := New(db)

			mock.ExpectExec("DELETE FROM clusters WHERE id = \\$1").
				WithArgs("us-east-1-managed").
				WillReturnError(&pq.Error{Code: pgForeignKeyViolation, Constraint: constraint})

			if err := store.Deregister(context.Background(), "us-east-1-managed"); !errors.Is(err, ErrInUseByAccounts) {
				t.Errorf("expected ErrInUseByAccounts, got %v", err)
			}
		})
	}
}

func TestDeregister_InUseUnknownConstraint(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectExec("DELETE FROM clusters WHERE id = \\$1").
		WithArgs("us-east-1-managed").
		WillReturnError(&pq.Error{Code: pgForeignKeyViolation, Constraint: "some_other_fkey"})

	if err := store.Deregister(context.Background(), "us-east-1-managed"); !errors.Is(err, ErrInUse) {
		t.Errorf("expected ErrInUse, got %v", err)
	}
}

func TestBlockers(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM accounts WHERE cluster_id = \\$1").
		WithArgs("us-east-1-managed").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT id, name FROM accounts WHERE cluster_id = \\$1 ORDER BY name LIMIT \\$2").
		WithArgs("us-east-1-managed", 25).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow("acct-1", "acme"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM deployments WHERE cluster_id = \\$1").
		WithArgs("us-east-1-managed").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT id, agent_name, status FROM deployments WHERE cluster_id = \\$1 ORDER BY deployed_at DESC LIMIT \\$2").
		WithArgs("us-east-1-managed", 25).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agent_name", "status"}).AddRow("dep-1", "pirate-parrot", "undeployed"))

	accounts, accountCount, deployments, deploymentCount, err := store.Blockers(context.Background(), "us-east-1-managed")
	if err != nil {
		t.Fatalf("Blockers returned error: %v", err)
	}
	if accountCount != 1 || len(accounts) != 1 || accounts[0].Name != "acme" {
		t.Errorf("unexpected accounts result: count=%d rows=%v", accountCount, accounts)
	}
	if deploymentCount != 1 || len(deployments) != 1 || deployments[0].Name != "pirate-parrot" || deployments[0].Status != "undeployed" {
		t.Errorf("unexpected deployments result: count=%d rows=%v", deploymentCount, deployments)
	}
}

func TestDeleteRemoved_DeletesAndReportsBlocked(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := New(db)

	mock.ExpectQuery(`SELECT id FROM clusters`).
		WithArgs(pq.Array([]string{"eu-west-1-a"})).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("stale-cluster").AddRow("in-use-cluster"))
	mock.ExpectExec("DELETE FROM clusters WHERE id = \\$1").
		WithArgs("stale-cluster").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM clusters WHERE id = \\$1").
		WithArgs("in-use-cluster").
		WillReturnError(&pq.Error{Code: pgForeignKeyViolation, Constraint: "accounts_cluster_id_fkey"})

	deleted, blocked, err := store.DeleteRemoved(context.Background(), []string{"eu-west-1-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != "stale-cluster" {
		t.Errorf("deleted = %v, want [stale-cluster]", deleted)
	}
	if len(blocked) != 1 || blocked[0] != "in-use-cluster" {
		t.Errorf("blocked = %v, want [in-use-cluster]", blocked)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// clusterRows returns a sqlmock.Rows with the column projection used by
// baseSelect. Test rows can be appended via .AddRow.
func clusterRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "region", "eks_cluster_name", "eks_cluster_endpoint", "eks_cluster_ca",
		"agent_ingress_domain", "agent_public_ingress_domain", "ingestion_ingress_domain",
		"langfuse_base_url_ext", "langfuse_vpce_ips", "pod_subnet_cidrs", "pod_subnet_ipv6_cidrs",
		"loki_url", "prometheus_url", "tenant_router_internal_url",
		"pull_credential", "pull_key_hash",
		"created_at", "updated_at",
	})
}

// fullClusterRow appends a row populated with non-empty ingress fields. Use
// it in tests that just need a well-formed cluster — ingress values are
// irrelevant to the assertion.
func fullClusterRow(rows *sqlmock.Rows, id, region, eksName, eksEndpoint string, now time.Time) *sqlmock.Rows {
	return rows.AddRow(
		id, region, eksName, eksEndpoint, fakeCA(),
		"agents.example.com", "agents.public.example.com", "ingestion.example.com",
		"http://langfuse.platform.astroids.ai:3000", "10.0.1.10,10.0.2.10", "10.0.0.0/24,10.1.0.0/24", "",
		"", "", "",
		nil, nil,
		now, now,
	)
}
