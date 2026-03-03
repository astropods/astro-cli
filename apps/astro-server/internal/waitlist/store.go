package waitlist

import (
	"database/sql"
	"fmt"
	"time"
)

// Entry represents a waitlist signup
type Entry struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Email     string     `json:"email"`
	InvitedAt *time.Time `json:"invited_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Store manages waitlist persistence in PostgreSQL
type Store struct {
	db *sql.DB
}

// NewStore creates a new waitlist store with the given database connection
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Add inserts a new email into the waitlist.
// Returns the created entry or an error if the email already exists.
func (s *Store) Add(email, name string) (*Entry, error) {
	var entry Entry
	err := s.db.QueryRow(`
		INSERT INTO waitlist (email, name)
		VALUES ($1, $2)
		RETURNING id, name, email, invited_at, created_at
	`, email, name).Scan(&entry.ID, &entry.Name, &entry.Email, &entry.InvitedAt, &entry.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to add to waitlist: %w", err)
	}
	return &entry, nil
}

// GetByEmail retrieves a waitlist entry by email
func (s *Store) GetByEmail(email string) (*Entry, error) {
	var entry Entry
	err := s.db.QueryRow(`
		SELECT id, name, email, invited_at, created_at
		FROM waitlist
		WHERE email = $1
	`, email).Scan(&entry.ID, &entry.Name, &entry.Email, &entry.InvitedAt, &entry.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query waitlist: %w", err)
	}
	return &entry, nil
}
