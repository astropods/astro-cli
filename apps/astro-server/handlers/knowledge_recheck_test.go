package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// recheckKMS is an identity KMS (Decrypt returns the ciphertext unchanged) so
// the rewrite path runs without real KMS.
type recheckKMS struct{ key []byte }

func (k *recheckKMS) GenerateDataKey(_ context.Context, _ *kms.GenerateDataKeyInput, _ ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	cp := func() []byte { b := make([]byte, len(k.key)); copy(b, k.key); return b }
	return &kms.GenerateDataKeyOutput{Plaintext: cp(), CiphertextBlob: cp()}, nil
}

func (k *recheckKMS) Decrypt(_ context.Context, params *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	plain := make([]byte, len(params.CiphertextBlob))
	copy(plain, params.CiphertextBlob)
	return &kms.DecryptOutput{Plaintext: plain}, nil
}

// recheckEC2 returns a single available VPC endpoint with the given DNS.
type recheckEC2 struct {
	knowledgestore.EC2Client
	dns string
}

func (e *recheckEC2) DescribeVpcEndpoints(_ context.Context, _ *ec2.DescribeVpcEndpointsInput, _ ...func(*ec2.Options)) (*ec2.DescribeVpcEndpointsOutput, error) {
	return &ec2.DescribeVpcEndpointsOutput{
		VpcEndpoints: []ec2types.VpcEndpoint{{
			State:      ec2types.State("available"),
			DnsEntries: []ec2types.DnsEntry{{DnsName: aws.String(e.dns)}},
		}},
	}, nil
}

func endpointColumnNames() []string {
	return []string{
		"knowledge_store_id", "cloud_provider", "endpoint_service", "region",
		"endpoint_id", "endpoint_dns", "status", "error", "created_at", "updated_at",
	}
}

// externalStoreRowWithKey builds a knowledge_stores row for an external store
// that has an encrypted data key (so the rewrite path is active).
func externalStoreRowWithKey(id, name string, key []byte) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(knowledgeColumns).AddRow(
		id, testAccount().ID, name, "arn:knowledge:acme:"+name,
		"postgres", "external", "ready", "10Gi", nil,
		false, nil, key, nil, nil, now, now,
	)
}

func TestRecheckKnowledgeStore_NotExternal(t *testing.T) {
	router, ksStore, mock := setupKS()
	router.POST("/knowledge/:name/recheck", RecheckKnowledgeStore(logger.New("error", "json"), ksStore, nil, &recheckKMS{}))

	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(knowledgeRow("id1", testAccount().ID, "pg-main", "postgres", "ready")) // managed

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/knowledge/pg-main/recheck", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for managed store, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRecheckKnowledgeStore_NoEndpoint(t *testing.T) {
	router, ksStore, mock := setupKS()
	router.POST("/knowledge/:name/recheck", RecheckKnowledgeStore(logger.New("error", "json"), ksStore, nil, &recheckKMS{}))

	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(externalKnowledgeRow("id1", testAccount().ID, "ext", "postgres", "ready"))
	mock.ExpectQuery("SELECT .+ FROM knowledge_store_endpoints").
		WillReturnError(sql.ErrNoRows)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/knowledge/ext/recheck", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for store without endpoint, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRecheckKnowledgeStore_Success(t *testing.T) {
	router, ksStore, mock := setupKS()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	const resolvedDNS = "vpce-0abc.vpce-svc-0def.us-east-1.vpce.amazonaws.com"

	router.POST("/knowledge/:name/recheck", RecheckKnowledgeStore(
		logger.New("error", "json"), ksStore,
		func(context.Context) (knowledgestore.EC2Client, error) { return &recheckEC2{dns: resolvedDNS}, nil },
		&recheckKMS{key: key},
	))

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(externalStoreRowWithKey("id1", "ext", key))
	mock.ExpectQuery("SELECT .+ FROM knowledge_store_endpoints").
		WillReturnRows(sqlmock.NewRows(endpointColumnNames()).AddRow(
			"id1", "aws", "com.amazonaws.vpce.us-east-1.vpce-svc-0def", "us-east-1",
			"vpce-0abc123", "com.amazonaws.vpce.us-east-1.vpce-svc-0def", // stale endpoint_dns
			"ready", nil, now, now,
		))
	// SetEndpointReady writes the freshly resolved DNS.
	mock.ExpectExec("UPDATE knowledge_store_endpoints").
		WithArgs("vpce-0abc123", resolvedDNS, knowledgestore.StatusReady, "id1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// RewriteHostCredential upserts ONLY the HOST row, in a transaction.
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO knowledge_store_credentials").
		WithArgs("id1", knowledgestore.HostCredentialKey, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/knowledge/ext/recheck", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp KnowledgeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Endpoint == nil || resp.Endpoint.EndpointDNS == nil || *resp.Endpoint.EndpointDNS != resolvedDNS {
		t.Errorf("response endpoint DNS: got %+v, want %q", resp.Endpoint, resolvedDNS)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

// ensure envelope import is used even if the rewrite helper signature changes.
var _ envelope.KMSClient = (*recheckKMS)(nil)
