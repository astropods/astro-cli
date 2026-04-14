package knowledgestore

import (
	"database/sql"
	"os"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	_ "github.com/lib/pq"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	return db
}

func ensureTestKSAccount(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	err := db.QueryRow(`
		INSERT INTO accounts (name, type) VALUES ('test-ks-account', 'personal')
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`).Scan(&id)
	if err != nil {
		t.Fatalf("ensure test account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM knowledge_stores WHERE account_id = $1`, id)
	})
	return id
}

func TestStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)
	accountID := ensureTestKSAccount(t, db)

	p := CreateParams{
		ID:        deployid.New(),
		AccountID: accountID,
		Name:      "pg-main",
		ARN:       "arn:knowledge:test-ks-account:pg-main",
		Provider:  "postgres",
		Storage:   "10Gi",
	}

	ks, err := s.Create(p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if ks.ID != p.ID {
		t.Errorf("ID: want %q, got %q", p.ID, ks.ID)
	}
	if ks.Status != StatusProvisioning {
		t.Errorf("Status: want %q, got %q", StatusProvisioning, ks.Status)
	}

	byID, err := s.GetByID(ks.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID == nil || byID.ARN != p.ARN {
		t.Errorf("GetByID returned unexpected record")
	}

	byName, err := s.GetByName(accountID, "pg-main")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if byName == nil || byName.Provider != "postgres" {
		t.Errorf("GetByName returned unexpected record")
	}

	byARN, err := s.GetByARN(p.ARN)
	if err != nil {
		t.Fatalf("GetByARN: %v", err)
	}
	if byARN == nil || byARN.ID != p.ID {
		t.Errorf("GetByARN returned unexpected record")
	}
}

func TestStore_GetByID_NotFound(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)

	ks, err := s.GetByID("xxx-xxx-xxx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ks != nil {
		t.Error("expected nil for missing ID")
	}
}

func TestStore_Create_UniqueConflict(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)
	accountID := ensureTestKSAccount(t, db)

	p := CreateParams{
		ID:        deployid.New(),
		AccountID: accountID,
		Name:      "dup-store",
		ARN:       "arn:knowledge:test-ks-account:dup-store",
		Provider:  "postgres",
		Storage:   "10Gi",
	}
	if _, err := s.Create(p); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	p2 := p
	p2.ID = deployid.New()
	_, err := s.Create(p2)
	if err == nil {
		t.Fatal("expected unique conflict error, got nil")
	}
}

func TestStore_ListByAccount(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)
	accountID := ensureTestKSAccount(t, db)

	for i, name := range []string{"store-a", "store-b", "store-c"} {
		_, err := s.Create(CreateParams{
			ID:        deployid.New(),
			AccountID: accountID,
			Name:      name,
			ARN:       "arn:knowledge:test-ks-account:" + name,
			Provider:  "qdrant",
			Storage:   "5Gi",
		})
		if err != nil {
			t.Fatalf("Create[%d]: %v", i, err)
		}
	}

	stores, err := s.ListByAccount(accountID)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(stores) < 3 {
		t.Errorf("expected at least 3 stores, got %d", len(stores))
	}
}

func TestStore_SetStatus(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)
	accountID := ensureTestKSAccount(t, db)

	id := deployid.New()
	if _, err := s.Create(CreateParams{
		ID: id, AccountID: accountID, Name: "status-store",
		ARN: "arn:knowledge:test-ks-account:status-store", Provider: "redis",
		Storage: "1Gi",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.SetStatus(id, StatusReady); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	ks, _ := s.GetByID(id)
	if ks.Status != StatusReady {
		t.Errorf("expected status %q, got %q", StatusReady, ks.Status)
	}
	if ks.Error != nil {
		t.Errorf("expected nil error after SetStatus, got %v", *ks.Error)
	}
}

func TestStore_SetError(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)
	accountID := ensureTestKSAccount(t, db)

	id := deployid.New()
	if _, err := s.Create(CreateParams{
		ID: id, AccountID: accountID, Name: "error-store",
		ARN: "arn:knowledge:test-ks-account:error-store", Provider: "redis",
		Storage: "1Gi",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.SetError(id, "failed to provision"); err != nil {
		t.Fatalf("SetError: %v", err)
	}

	ks, _ := s.GetByID(id)
	if ks.Status != StatusError {
		t.Errorf("expected status %q, got %q", StatusError, ks.Status)
	}
	if ks.Error == nil || *ks.Error != "failed to provision" {
		t.Errorf("unexpected error value: %v", ks.Error)
	}
}

func TestStore_SetPublicHost(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)
	accountID := ensureTestKSAccount(t, db)

	id := deployid.New()
	if _, err := s.Create(CreateParams{
		ID: id, AccountID: accountID, Name: "pub-store",
		ARN: "arn:knowledge:test-ks-account:pub-store", Provider: "postgres",
		Storage: "10Gi", Public: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.SetPublicHost(id, "pub-store.acme.knowledge.astropods.ai"); err != nil {
		t.Fatalf("SetPublicHost: %v", err)
	}

	ks, _ := s.GetByID(id)
	if ks.PublicHost == nil || *ks.PublicHost != "pub-store.acme.knowledge.astropods.ai" {
		t.Errorf("unexpected public_host: %v", ks.PublicHost)
	}
}

func TestStore_Delete(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)
	accountID := ensureTestKSAccount(t, db)

	id := deployid.New()
	if _, err := s.Create(CreateParams{
		ID: id, AccountID: accountID, Name: "delete-me",
		ARN: "arn:knowledge:test-ks-account:delete-me", Provider: "redis",
		Storage: "1Gi",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	ks, _ := s.GetByID(id)
	if ks != nil {
		t.Error("expected nil after delete")
	}
}

func TestStore_SaveAndGetCredentials(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)
	accountID := ensureTestKSAccount(t, db)

	id := deployid.New()
	if _, err := s.Create(CreateParams{
		ID: id, AccountID: accountID, Name: "cred-store",
		ARN: "arn:knowledge:test-ks-account:cred-store", Provider: "postgres",
		Storage: "10Gi",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	creds := []Credential{
		{Key: "POSTGRES_USER", ValueEncrypted: []byte("enc-user"), Nonce: []byte("nonce1-x")},
		{Key: "POSTGRES_PASSWORD", ValueEncrypted: []byte("enc-pass"), Nonce: []byte("nonce2-x")},
	}
	if err := s.SaveCredentials(id, creds); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	got, err := s.GetCredentials(id)
	if err != nil {
		t.Fatalf("GetCredentials: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 credentials, got %d", len(got))
	}

	byKey := make(map[string]Credential)
	for _, c := range got {
		byKey[c.Key] = c
	}
	if string(byKey["POSTGRES_USER"].ValueEncrypted) != "enc-user" {
		t.Errorf("unexpected value for POSTGRES_USER")
	}
}

func TestStore_ListProvisioning(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)
	accountID := ensureTestKSAccount(t, db)

	id := deployid.New()
	if _, err := s.Create(CreateParams{
		ID: id, AccountID: accountID, Name: "prov-store",
		ARN: "arn:knowledge:test-ks-account:prov-store", Provider: "postgres",
		Storage: "10Gi",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	stores, err := s.ListProvisioning()
	if err != nil {
		t.Fatalf("ListProvisioning: %v", err)
	}
	found := false
	for _, ks := range stores {
		if ks.ID == id {
			found = true
			break
		}
	}
	if !found {
		t.Error("new store not found in ListProvisioning")
	}

	// After marking ready, should no longer appear.
	_ = s.SetStatus(id, StatusReady)
	stores, _ = s.ListProvisioning()
	for _, ks := range stores {
		if ks.ID == id {
			t.Error("ready store should not appear in ListProvisioning")
		}
	}
}
