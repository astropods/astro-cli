package evaldefinitionstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Definition struct {
	EvaluationRef  string
	DefinitionJSON json.RawMessage
	CreatedAt      time.Time
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(ctx context.Context, evaluationRef string, definitionJSON json.RawMessage) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO eval_definitions (evaluation_ref, definition_json)
		VALUES ($1, $2)
		ON CONFLICT (evaluation_ref) DO NOTHING
	`, evaluationRef, definitionJSON); err != nil {
		return fmt.Errorf("evaldefinitionstore create: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, evaluationRef string) (*Definition, error) {
	var def Definition
	err := s.db.QueryRowContext(ctx, `
		SELECT evaluation_ref, definition_json, created_at
		FROM eval_definitions
		WHERE evaluation_ref = $1
	`, evaluationRef).Scan(&def.EvaluationRef, &def.DefinitionJSON, &def.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("evaldefinitionstore get: %w", err)
	}
	return &def, nil
}
