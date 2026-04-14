package knowledgestore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Endpoint represents a PrivateLink endpoint for an external knowledge store.
type Endpoint struct {
	KnowledgeStoreID string
	CloudProvider    string
	EndpointService  string
	Region           string
	EndpointID       *string
	EndpointDNS      *string
	Status           string
	Error            *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// EndpointParams holds parameters for creating a new endpoint.
type EndpointParams struct {
	KnowledgeStoreID string
	CloudProvider    string
	EndpointService  string
	Region           string
}

const endpointColumns = `knowledge_store_id, cloud_provider, endpoint_service, region,
       endpoint_id, endpoint_dns, status, error, created_at, updated_at`

func scanEndpoint(row interface{ Scan(dest ...any) error }) (*Endpoint, error) {
	var e Endpoint
	err := row.Scan(
		&e.KnowledgeStoreID, &e.CloudProvider, &e.EndpointService, &e.Region,
		&e.EndpointID, &e.EndpointDNS, &e.Status, &e.Error,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// CreateEndpoint inserts a new PrivateLink endpoint record.
func (s *Store) CreateEndpoint(p EndpointParams) (*Endpoint, error) {
	row := s.db.QueryRow(`
		INSERT INTO knowledge_store_endpoints
		  (knowledge_store_id, cloud_provider, endpoint_service, region)
		VALUES ($1, $2, $3, $4)
		RETURNING `+endpointColumns,
		p.KnowledgeStoreID, p.CloudProvider, p.EndpointService, p.Region,
	)
	return scanEndpoint(row)
}

// GetEndpoint retrieves the PrivateLink endpoint for a store. Returns nil, nil if not found.
func (s *Store) GetEndpoint(storeID string) (*Endpoint, error) {
	row := s.db.QueryRow(
		`SELECT `+endpointColumns+` FROM knowledge_store_endpoints WHERE knowledge_store_id = $1`,
		storeID,
	)
	ep, err := scanEndpoint(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ep, err
}

// ListEndpointsByStatus returns all endpoints matching any of the given statuses.
// Used by the reconciler to poll connecting/pending-acceptance endpoints.
func (s *Store) ListEndpointsByStatus(statuses ...string) ([]*Endpoint, error) {
	if len(statuses) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(statuses))
	args := make([]any, len(statuses))
	for i, st := range statuses {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = st
	}

	query := `SELECT ` + endpointColumns + ` FROM knowledge_store_endpoints WHERE status IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var endpoints []*Endpoint
	for rows.Next() {
		ep, err := scanEndpoint(rows)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, ep)
	}
	return endpoints, rows.Err()
}

// SetEndpointStatus updates the endpoint status.
func (s *Store) SetEndpointStatus(storeID, status string) error {
	_, err := s.db.Exec(
		`UPDATE knowledge_store_endpoints SET status = $1, error = NULL, updated_at = now() WHERE knowledge_store_id = $2`,
		status, storeID,
	)
	return err
}

// SetEndpointVPCEID records the AWS VPC endpoint ID after creation.
func (s *Store) SetEndpointVPCEID(storeID, endpointID string) error {
	_, err := s.db.Exec(
		`UPDATE knowledge_store_endpoints SET endpoint_id = $1, updated_at = now() WHERE knowledge_store_id = $2`,
		endpointID, storeID,
	)
	return err
}

// SetEndpointReady marks the endpoint as ready with its resolved VPCE ID and DNS name.
func (s *Store) SetEndpointReady(storeID, endpointID, dns string) error {
	_, err := s.db.Exec(
		`UPDATE knowledge_store_endpoints SET endpoint_id = $1, endpoint_dns = $2, status = $3, error = NULL, updated_at = now() WHERE knowledge_store_id = $4`,
		endpointID, dns, StatusReady, storeID,
	)
	return err
}

// SetEndpointError marks the endpoint as failed with an error message.
func (s *Store) SetEndpointError(storeID, errMsg string) error {
	_, err := s.db.Exec(
		`UPDATE knowledge_store_endpoints SET status = $1, error = $2, updated_at = now() WHERE knowledge_store_id = $3`,
		StatusError, errMsg, storeID,
	)
	return err
}

// DeleteEndpoint removes the endpoint record for a store. Idempotent.
func (s *Store) DeleteEndpoint(storeID string) error {
	_, err := s.db.Exec(`DELETE FROM knowledge_store_endpoints WHERE knowledge_store_id = $1`, storeID)
	return err
}
