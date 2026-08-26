package connectapps

import (
	"context"
	"fmt"
	"time"

	workos "github.com/workos/workos-go/v10"
)

const MaxSecrets = 5

var ErrSecretLimit = fmt.Errorf("an app may hold at most %d secrets", MaxSecrets)

type Application struct {
	ID       string
	ClientID string
	Scopes   []string
}

type Secret struct {
	ID         string     `json:"id"`
	Hint       string     `json:"hint"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
}

type NewSecret struct {
	Secret
	Value string `json:"value"`
}

type Client interface {
	CreateApplication(ctx context.Context, organizationID, name, description string, scopes []string) (*Application, error)
	DeleteApplication(ctx context.Context, applicationID string) error
	CreateSecret(ctx context.Context, applicationID string) (*NewSecret, error)
	ListSecrets(ctx context.Context, applicationID string) ([]Secret, error)
	DeleteSecret(ctx context.Context, secretID string) error
}

type workosClient struct {
	connect *workos.ConnectService
}

func New(apiKey string) Client {
	if apiKey == "" {
		return nil
	}
	return &workosClient{connect: workos.NewClient(apiKey).Connect()}
}

func (c *workosClient) CreateApplication(ctx context.Context, organizationID, name, description string, scopes []string) (*Application, error) {
	// Scopes are deliberately not handed to WorkOS. They are permission slugs
	// that must already exist on the WorkOS side, and Astro authorizes from the
	// stored app row instead, which also makes a scope change take effect at
	// once rather than at the next token expiry. Pass them here once the slugs
	// are registered and the token can carry them too.
	_ = scopes
	params := &workos.ConnectCreateM2MApplicationParams{
		Name:           name,
		OrganizationID: organizationID,
	}
	if description != "" {
		params.Description = &description
	}
	app, err := c.connect.CreateM2MApplication(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create m2m application: %w", err)
	}
	return &Application{ID: app.ID, ClientID: app.ClientID, Scopes: app.Scopes}, nil
}

func (c *workosClient) DeleteApplication(ctx context.Context, applicationID string) error {
	if err := c.connect.DeleteApplication(ctx, applicationID); err != nil {
		return fmt.Errorf("delete m2m application: %w", err)
	}
	return nil
}

func (c *workosClient) CreateSecret(ctx context.Context, applicationID string) (*NewSecret, error) {
	existing, err := c.ListSecrets(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	if len(existing) >= MaxSecrets {
		return nil, ErrSecretLimit
	}
	created, err := c.connect.CreateApplicationClientSecret(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("create client secret: %w", err)
	}
	return &NewSecret{
		Secret: Secret{
			ID:         created.ID,
			Hint:       created.SecretHint,
			LastUsedAt: parseTime(created.LastUsedAt),
			CreatedAt:  parseTimeValue(created.CreatedAt),
		},
		Value: created.Secret,
	}, nil
}

func (c *workosClient) ListSecrets(ctx context.Context, applicationID string) ([]Secret, error) {
	items, err := c.connect.ListApplicationClientSecrets(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list client secrets: %w", err)
	}
	out := make([]Secret, 0, len(items))
	for _, item := range items {
		out = append(out, Secret{
			ID:         item.ID,
			Hint:       item.SecretHint,
			LastUsedAt: parseTime(item.LastUsedAt),
			CreatedAt:  parseTimeValue(item.CreatedAt),
		})
	}
	return out, nil
}

func (c *workosClient) DeleteSecret(ctx context.Context, secretID string) error {
	if err := c.connect.DeleteClientSecret(ctx, secretID); err != nil {
		return fmt.Errorf("delete client secret: %w", err)
	}
	return nil
}

func parseTime(value *string) *time.Time {
	if value == nil {
		return nil
	}
	return parseTimeValue(*value)
}

func parseTimeValue(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}
