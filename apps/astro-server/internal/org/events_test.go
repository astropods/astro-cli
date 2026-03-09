package org

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/workos/workos-go/v4/pkg/events"
)

func newTestConsumer(t *testing.T) (*EventsConsumer, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	log := logger.New("test", "debug")
	store := account.NewAccountStore(db)
	ec := &EventsConsumer{
		accountStore: store,
		db:           db,
		log:          log,
	}
	return ec, mock
}

func makeEvent(eventType string, data any) events.Event {
	raw, _ := json.Marshal(data)
	return events.Event{
		ID:        "evt_" + eventType,
		Event:     eventType,
		Data:      raw,
		CreatedAt: time.Now(),
	}
}

// --- Membership events ---

func TestProcessEvent_MembershipCreated(t *testing.T) {
	ec, mock := newTestConsumer(t)
	now := time.Now()

	// GetByWorkOSOrganizationID
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}).
			AddRow("acct-1", "myorg", "organization", "org_1", nil, now, now))

	// UpsertMemberByWorkosMembershipID
	mock.ExpectExec("INSERT INTO account_members .+ ON CONFLICT").
		WithArgs("acct-1", "user-1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO account_member_workos .+ ON CONFLICT").
		WithArgs("acct-1", "user-1", "mem-1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	event := makeEvent("organization_membership.created", map[string]any{
		"id":              "mem-1",
		"user_id":         "user-1",
		"organization_id": "org_1",
		"role":            map[string]string{"slug": "member"},
		"status":          "active",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessEvent_MembershipDeleted(t *testing.T) {
	ec, mock := newTestConsumer(t)
	now := time.Now()

	// GetByWorkOSOrganizationID
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}).
			AddRow("acct-1", "myorg", "organization", "org_1", nil, now, now))

	// GetMemberByWorkosMembershipID
	mock.ExpectQuery("SELECT .+ FROM account_member_workos mw JOIN account_members am").
		WithArgs("mem-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "workos_membership_id", "created_at"}).
			AddRow("acct-1", "user-1", "mem-1", now))

	// RemoveMember
	mock.ExpectExec("DELETE FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	event := makeEvent("organization_membership.deleted", map[string]any{
		"id":              "mem-1",
		"user_id":         "user-1",
		"organization_id": "org_1",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessEvent_MembershipDeleted_AlreadyGone(t *testing.T) {
	ec, mock := newTestConsumer(t)
	now := time.Now()

	// GetByWorkOSOrganizationID
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}).
			AddRow("acct-1", "myorg", "organization", "org_1", nil, now, now))

	// GetMemberByWorkosMembershipID — not found
	mock.ExpectQuery("SELECT .+ FROM account_member_workos mw JOIN account_members am").
		WithArgs("mem-gone").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "workos_membership_id", "created_at"}))

	event := makeEvent("organization_membership.deleted", map[string]any{
		"id":              "mem-gone",
		"user_id":         "user-1",
		"organization_id": "org_1",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessEvent_Membership_NoLocalAccount(t *testing.T) {
	ec, mock := newTestConsumer(t)

	// GetByWorkOSOrganizationID — not found
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_unknown").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}))

	event := makeEvent("organization_membership.created", map[string]any{
		"id":              "mem-1",
		"user_id":         "user-1",
		"organization_id": "org_unknown",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- Organization events ---

func TestProcessEvent_OrgCreated_AlreadyLinked(t *testing.T) {
	ec, mock := newTestConsumer(t)
	now := time.Now()

	// GetByWorkOSOrganizationID — found, already linked
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}).
			AddRow("acct-1", "myorg", "organization", "org_1", nil, now, now))

	event := makeEvent("organization.created", map[string]any{
		"id":   "org_1",
		"name": "myorg",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessEvent_OrgCreated_WithExternalID(t *testing.T) {
	ec, mock := newTestConsumer(t)
	now := time.Now()

	// GetByWorkOSOrganizationID — not found
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_new").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}))

	// GetByID — account exists with this external_id
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("acct-existing").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}).
			AddRow("acct-existing", "myorg", "organization", nil, nil, now, now))

	// SetWorkOSOrganizationID
	mock.ExpectExec("INSERT INTO account_organizations").
		WithArgs("acct-existing", "org_new").
		WillReturnResult(sqlmock.NewResult(0, 1))

	event := makeEvent("organization.created", map[string]any{
		"id":          "org_new",
		"name":        "myorg",
		"external_id": "acct-existing",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessEvent_OrgCreated_External(t *testing.T) {
	ec, mock := newTestConsumer(t)
	now := time.Now()

	// GetByWorkOSOrganizationID — not found
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_ext").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}))

	// CreateWithoutOwner
	mock.ExpectQuery("INSERT INTO accounts").
		WithArgs("external-org", "organization", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}).
			AddRow("acct-new", "external-org", "organization", now, now))

	// SetWorkOSOrganizationID
	mock.ExpectExec("INSERT INTO account_organizations").
		WithArgs("acct-new", "org_ext").
		WillReturnResult(sqlmock.NewResult(0, 1))

	event := makeEvent("organization.created", map[string]any{
		"id":   "org_ext",
		"name": "external-org",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessEvent_OrgCreated_ExternalLinkFailure_Cleans(t *testing.T) {
	ec, mock := newTestConsumer(t)
	now := time.Now()

	// GetByWorkOSOrganizationID — not found
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_ext").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}))

	// CreateWithoutOwner
	mock.ExpectQuery("INSERT INTO accounts").
		WithArgs("external-org", "organization", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}).
			AddRow("acct-new", "external-org", "organization", now, now))

	// SetWorkOSOrganizationID — fails
	mock.ExpectExec("INSERT INTO account_organizations").
		WithArgs("acct-new", "org_ext").
		WillReturnError(sqlmock.ErrCancelled)

	// DeleteByID — cleanup
	mock.ExpectExec("DELETE FROM accounts WHERE id").
		WithArgs("acct-new").
		WillReturnResult(sqlmock.NewResult(0, 1))

	event := makeEvent("organization.created", map[string]any{
		"id":   "org_ext",
		"name": "external-org",
	})

	if err := ec.processEvent(context.TODO(), event); err == nil {
		t.Fatal("expected error on link failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessEvent_OrgUpdated_RenamesAccount(t *testing.T) {
	ec, mock := newTestConsumer(t)
	now := time.Now()

	// GetByWorkOSOrganizationID
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}).
			AddRow("acct-1", "old-name", "organization", "org_1", nil, now, now))

	// Rename
	mock.ExpectExec("UPDATE accounts SET name").
		WithArgs("new-name", sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	event := makeEvent("organization.updated", map[string]any{
		"id":   "org_1",
		"name": "new-name",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessEvent_OrgUpdated_SameName_NoOp(t *testing.T) {
	ec, mock := newTestConsumer(t)
	now := time.Now()

	// GetByWorkOSOrganizationID
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}).
			AddRow("acct-1", "same-name", "organization", "org_1", nil, now, now))

	// No Rename expected

	event := makeEvent("organization.updated", map[string]any{
		"id":   "org_1",
		"name": "same-name",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessEvent_OrgUpdated_NoLocalAccount(t *testing.T) {
	ec, mock := newTestConsumer(t)

	// GetByWorkOSOrganizationID — not found
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_unknown").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}))

	event := makeEvent("organization.updated", map[string]any{
		"id":   "org_unknown",
		"name": "whatever",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessEvent_OrgDeleted(t *testing.T) {
	ec, mock := newTestConsumer(t)
	now := time.Now()

	// GetByWorkOSOrganizationID
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}).
			AddRow("acct-1", "myorg", "organization", "org_1", nil, now, now))

	// MarkDeleted
	mock.ExpectExec("UPDATE accounts SET deleted_at").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	event := makeEvent("organization.deleted", map[string]any{
		"id":   "org_1",
		"name": "myorg",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessEvent_OrgDeleted_AlreadyGone(t *testing.T) {
	ec, mock := newTestConsumer(t)

	// GetByWorkOSOrganizationID — not found
	mock.ExpectQuery("SELECT .+ FROM accounts a JOIN account_organizations ao").
		WithArgs("org_gone").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at"}))

	event := makeEvent("organization.deleted", map[string]any{
		"id":   "org_gone",
		"name": "gone-org",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- User events ---

func TestProcessEvent_UserCreated_NoOp(t *testing.T) {
	ec, _ := newTestConsumer(t)

	event := makeEvent("user.created", map[string]any{
		"id": "user-1",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessEvent_UserUpdated_NoOp(t *testing.T) {
	ec, _ := newTestConsumer(t)

	event := makeEvent("user.updated", map[string]any{
		"id": "user-1",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessEvent_UserDeleted(t *testing.T) {
	ec, mock := newTestConsumer(t)

	// RemoveUserFromAllAccounts
	mock.ExpectExec("DELETE FROM account_members WHERE user_id").
		WithArgs("user-1").
		WillReturnResult(sqlmock.NewResult(0, 3))

	event := makeEvent("user.deleted", map[string]any{
		"id": "user-1",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestProcessEvent_UserDeleted_NoMemberships(t *testing.T) {
	ec, mock := newTestConsumer(t)

	// RemoveUserFromAllAccounts — no rows
	mock.ExpectExec("DELETE FROM account_members WHERE user_id").
		WithArgs("user-gone").
		WillReturnResult(sqlmock.NewResult(0, 0))

	event := makeEvent("user.deleted", map[string]any{
		"id": "user-gone",
	})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- Edge cases ---

func TestProcessEvent_UnknownEventType(t *testing.T) {
	ec, _ := newTestConsumer(t)

	event := makeEvent("something.unknown", map[string]any{"id": "x"})

	if err := ec.processEvent(context.TODO(), event); err != nil {
		t.Fatalf("unexpected error for unknown event: %v", err)
	}
}

func TestProcessEvent_InvalidJSON(t *testing.T) {
	ec, _ := newTestConsumer(t)

	event := events.Event{
		ID:    "evt_bad",
		Event: "organization_membership.created",
		Data:  json.RawMessage(`{invalid`),
	}

	if err := ec.processEvent(context.TODO(), event); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
