package appstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/lib/pq"
)

var ErrNameTaken = errors.New("app name already taken")

type App struct {
	ID                  string    `json:"id"`
	AccountID           string    `json:"account_id"`
	Name                string    `json:"name"`
	Description         string    `json:"description"`
	WorkOSApplicationID string    `json:"-"`
	ClientID            string    `json:"client_id"`
	Scopes              []string  `json:"scopes"`
	CreatedBy           string    `json:"created_by"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func ValidateName(name string) error {
	if name == "" {
		return errors.New("name must not be empty")
	}
	if len(name) > 100 {
		return errors.New("name must be at most 100 characters")
	}
	if strings.ContainsAny(name, "\n\r\t") {
		return errors.New("name must not contain line breaks or tabs")
	}
	return nil
}

const appColumns = `id, account_id, name, description, workos_application_id, client_id,
       scopes, created_by, created_at, updated_at`

func scanApp(row interface{ Scan(...any) error }) (*App, error) {
	var a App
	err := row.Scan(&a.ID, &a.AccountID, &a.Name, &a.Description, &a.WorkOSApplicationID,
		&a.ClientID, pq.Array(&a.Scopes), &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

type CreateParams struct {
	AccountID           string
	Name                string
	Description         string
	WorkOSApplicationID string
	ClientID            string
	Scopes              []string
	CreatedBy           string
}

func (s *Store) Create(ctx context.Context, p CreateParams) (*App, error) {
	// pq encodes a nil slice as SQL NULL, which overrides the column default
	// and trips the not-null constraint. Normalize here so no caller has to.
	if p.Scopes == nil {
		p.Scopes = []string{}
	}
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO account_apps
			(id, account_id, name, description, workos_application_id, client_id, scopes, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+appColumns,
		deployid.New(), p.AccountID, p.Name, p.Description,
		p.WorkOSApplicationID, p.ClientID, pq.Array(p.Scopes), p.CreatedBy,
	)
	a, err := scanApp(row)
	if isUniqueViolation(err, "account_apps_account_name_key") {
		return nil, ErrNameTaken
	}
	if err != nil {
		return nil, fmt.Errorf("create app: %w", err)
	}
	return a, nil
}

func (s *Store) GetByID(ctx context.Context, id string) (*App, error) {
	return s.get(ctx, `WHERE id = $1`, id)
}

func (s *Store) GetByClientID(ctx context.Context, clientID string) (*App, error) {
	return s.get(ctx, `WHERE client_id = $1`, clientID)
}

func (s *Store) get(ctx context.Context, where string, arg any) (*App, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("app store is not configured")
	}
	a, err := scanApp(s.db.QueryRowContext(ctx, `SELECT `+appColumns+` FROM account_apps `+where, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}
	return a, nil
}

func (s *Store) ListByAccount(ctx context.Context, accountID string) ([]*App, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+appColumns+`
		FROM account_apps WHERE account_id = $1
		ORDER BY created_at DESC, id DESC`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	apps := make([]*App, 0)
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (s *Store) UpdateScopes(ctx context.Context, id string, scopes []string) (*App, error) {
	if scopes == nil {
		scopes = []string{}
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE account_apps SET scopes = $2, updated_at = now()
		WHERE id = $1
		RETURNING `+appColumns, id, pq.Array(scopes))
	a, err := scanApp(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update app scopes: %w", err)
	}
	return a, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM account_apps WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete app: %w", err)
	}
	return nil
}

func isUniqueViolation(err error, constraint string) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	return pqErr.Code.Name() == "unique_violation" && pqErr.Constraint == constraint
}
