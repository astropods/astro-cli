package knowledgestore

import (
	"context"
	"database/sql"
)

// BindingRef identifies a single deployment↔knowledge binding.
type BindingRef struct {
	DeploymentID  string
	KnowledgeName string
	StoreID       string
}

// SaveBindings atomically replaces all knowledge store bindings for a deployment.
// bindings maps knowledge entry name → store ID.
func (s *Store) SaveBindings(ctx context.Context, tx *sql.Tx, deploymentID string, bindings map[string]string) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM knowledge_store_bindings WHERE deployment_id = $1`, deploymentID,
	); err != nil {
		return err
	}
	for name, storeID := range bindings {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO knowledge_store_bindings (deployment_id, knowledge_name, knowledge_store_id)
			 VALUES ($1, $2, $3)`,
			deploymentID, name, storeID,
		); err != nil {
			return err
		}
	}
	return nil
}

// GetBindingsForDeployment returns knowledge entry name → store ID for a deployment.
func (s *Store) GetBindingsForDeployment(ctx context.Context, deploymentID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT knowledge_name, knowledge_store_id FROM knowledge_store_bindings WHERE deployment_id = $1`,
		deploymentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	bindings := make(map[string]string)
	for rows.Next() {
		var name, storeID string
		if err := rows.Scan(&name, &storeID); err != nil {
			return nil, err
		}
		bindings[name] = storeID
	}
	return bindings, rows.Err()
}

// GetBindingsForStore returns all deployments bound to a given store.
func (s *Store) GetBindingsForStore(ctx context.Context, storeID string) ([]BindingRef, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT deployment_id, knowledge_name, knowledge_store_id
		 FROM knowledge_store_bindings WHERE knowledge_store_id = $1`,
		storeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var refs []BindingRef
	for rows.Next() {
		var r BindingRef
		if err := rows.Scan(&r.DeploymentID, &r.KnowledgeName, &r.StoreID); err != nil {
			return nil, err
		}
		refs = append(refs, r)
	}
	return refs, rows.Err()
}

// BoundAgent describes an active deployment bound to a knowledge store.
type BoundAgent struct {
	DeploymentID  string `json:"deployment_id"`
	AgentName     string `json:"agent_name"`
	DisplayName   string `json:"display_name,omitempty"`
	KnowledgeName string `json:"knowledge_name"`
}

// GetBoundAgents returns active deployments bound to a knowledge store,
// joined with the deployments table for agent/display names.
func (s *Store) GetBoundAgents(ctx context.Context, storeID string) ([]BoundAgent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT b.deployment_id, d.agent_name, d.display_name, b.knowledge_name
		 FROM knowledge_store_bindings b
		 JOIN deployments d ON d.id = b.deployment_id
		 WHERE b.knowledge_store_id = $1 AND d.status = 'active'
		 ORDER BY d.agent_name, b.knowledge_name`,
		storeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var agents []BoundAgent
	for rows.Next() {
		var a BoundAgent
		if err := rows.Scan(&a.DeploymentID, &a.AgentName, &a.DisplayName, &a.KnowledgeName); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// DeleteBindingsForDeployment removes all bindings for a deployment.
func (s *Store) DeleteBindingsForDeployment(ctx context.Context, deploymentID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM knowledge_store_bindings WHERE deployment_id = $1`, deploymentID,
	)
	return err
}
