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

type Permission struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
}

type Client interface {
	ListPermissions(ctx context.Context) ([]Permission, error)
	CreateApplication(ctx context.Context, organizationID, name, description string, scopes []string) (*Application, error)
	UpdateApplicationScopes(ctx context.Context, applicationID string, scopes []string) error
	DeleteApplication(ctx context.Context, applicationID string) error
	CreateSecret(ctx context.Context, applicationID string) (*NewSecret, error)
	ListSecrets(ctx context.Context, applicationID string) ([]Secret, error)
	DeleteSecret(ctx context.Context, secretID string) error
}

type workosClient struct {
	connect       *workos.ConnectService
	authorization *workos.AuthorizationService
}

func New(apiKey string, opts ...workos.ClientOption) Client {
	if apiKey == "" {
		return nil
	}
	client := workos.NewClient(apiKey, opts...)
	return &workosClient{connect: client.Connect(), authorization: client.Authorization()}
}

// ListPermissions returns the environment's permission slugs, which are what a
// Connect application's scopes are drawn from. The cap bounds an unbounded
// iterator; a WorkOS environment with more permissions than this has outgrown a
// single picker anyway.
func (c *workosClient) ListPermissions(ctx context.Context) ([]Permission, error) {
	const maxPermissions = 500
	it := c.authorization.ListPermissions(ctx, &workos.AuthorizationListPermissionsParams{})
	out := make([]Permission, 0)
	for it.Next() {
		p := it.Current()
		// System permissions belong to WorkOS and describe its own surface, so
		// granting one to an app would say nothing about access to Astro.
		if p.System {
			continue
		}
		entry := Permission{Slug: p.Slug, Name: p.Name, ResourceType: p.ResourceTypeSlug}
		if p.Description != nil {
			entry.Description = *p.Description
		}
		out = append(out, entry)
		if len(out) >= maxPermissions {
			break
		}
	}
	if err := it.Err(); err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	return out, nil
}

func (c *workosClient) CreateApplication(ctx context.Context, organizationID, name, description string, scopes []string) (*Application, error) {
	params := &workos.ConnectCreateM2MApplicationParams{
		Name:           name,
		OrganizationID: organizationID,
		Scopes:         scopes,
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

// UpdateApplicationScopes rewrites the granted scopes. Scopes is omitempty on
// the WorkOS params, so clearing them all needs the explicit-null field rather
// than an empty slice.
func (c *workosClient) UpdateApplicationScopes(ctx context.Context, applicationID string, scopes []string) error {
	params := &workos.ConnectUpdateApplicationParams{Scopes: scopes}
	if len(scopes) == 0 {
		params.NullFields = []string{"scopes"}
	}
	if _, err := c.connect.UpdateApplication(ctx, applicationID, params); err != nil {
		return fmt.Errorf("update m2m application scopes: %w", err)
	}
	return nil
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
