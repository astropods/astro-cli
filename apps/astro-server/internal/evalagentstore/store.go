package evalagentstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

var ErrDefinitionNotFound = errors.New("evaluation definition not found")

type AgentEvaluation struct {
	AccountID     string
	AgentName     string
	EvaluationRef string
	UpdatedAt     time.Time
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Get(ctx context.Context, accountID, agentName string) (*AgentEvaluation, error) {
	var ae AgentEvaluation
	err := s.db.QueryRowContext(ctx, `
		SELECT account_id, agent_name, evaluation_ref, updated_at
		FROM agent_evaluations
		WHERE account_id = $1 AND agent_name = $2
	`, accountID, agentName).Scan(&ae.AccountID, &ae.AgentName, &ae.EvaluationRef, &ae.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("evalagentstore get: %w", err)
	}
	return &ae, nil
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *Store) Set(ctx context.Context, accountID, agentName, evaluationRef string) error {
	return setAgentEvaluation(ctx, s.db, accountID, agentName, evaluationRef)
}

func SetTx(ctx context.Context, tx *sql.Tx, accountID, agentName, evaluationRef string) error {
	return setAgentEvaluation(ctx, tx, accountID, agentName, evaluationRef)
}

func setAgentEvaluation(ctx context.Context, db execer, accountID, agentName, evaluationRef string) error {
	if _, err := db.ExecContext(ctx, `
		INSERT INTO agent_evaluations (account_id, agent_name, evaluation_ref, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (account_id, agent_name)
		DO UPDATE SET evaluation_ref = $3, updated_at = now()
	`, accountID, agentName, evaluationRef); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23503" && pqErr.Constraint == "agent_evaluations_definition_fkey" {
			return ErrDefinitionNotFound
		}
		return fmt.Errorf("evalagentstore set: %w", err)
	}
	return nil
}

func (s *Store) Clear(ctx context.Context, accountID, agentName string) error {
	return clearAgentEvaluation(ctx, s.db, accountID, agentName)
}

func ClearTx(ctx context.Context, tx *sql.Tx, accountID, agentName string) error {
	return clearAgentEvaluation(ctx, tx, accountID, agentName)
}

func clearAgentEvaluation(ctx context.Context, db execer, accountID, agentName string) error {
	if _, err := db.ExecContext(ctx, `
		DELETE FROM agent_evaluations
		WHERE account_id = $1 AND agent_name = $2
	`, accountID, agentName); err != nil {
		return fmt.Errorf("evalagentstore clear: %w", err)
	}
	return nil
}
