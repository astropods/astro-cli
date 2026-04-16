package githubbuild

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ecr"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// mockECR implements ecrAPI for testing.
type mockECR struct {
	describeErr error
	createErr   error
	// Track calls for assertions.
	describedRepos []string
	createdRepos   []string
}

func (m *mockECR) DescribeRepositories(_ context.Context, in *ecr.DescribeRepositoriesInput, _ ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error) {
	m.describedRepos = append(m.describedRepos, in.RepositoryNames...)
	if m.describeErr != nil {
		return nil, m.describeErr
	}
	return &ecr.DescribeRepositoriesOutput{}, nil
}

func (m *mockECR) CreateRepository(_ context.Context, in *ecr.CreateRepositoryInput, _ ...func(*ecr.Options)) (*ecr.CreateRepositoryOutput, error) {
	m.createdRepos = append(m.createdRepos, *in.RepositoryName)
	if m.createErr != nil {
		return nil, m.createErr
	}
	return &ecr.CreateRepositoryOutput{}, nil
}

func testBuilder(mock *mockECR) *Builder {
	return &Builder{
		cfg: &config.Config{},
		log: logger.New("error", "text"),
		ecr: mock,
	}
}

// --- ecrRepoName tests ---

func TestEcrRepoName(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		want        string
		wantErr     bool
	}{
		{
			name:        "standard ECR destination",
			destination: "969403051954.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-7cc6d592-fd64-4715-b9a8-8fcb76a5a2be/test:e285692f",
			want:        "prod-tenant-7cc6d592-fd64-4715-b9a8-8fcb76a5a2be/test",
		},
		{
			name:        "nested repo path",
			destination: "123456.dkr.ecr.us-east-1.amazonaws.com/dev-tenant-abc123/my-agent-tool-search:build42",
			want:        "dev-tenant-abc123/my-agent-tool-search",
		},
		{
			name:        "no slash",
			destination: "invalid-no-slash:tag",
			wantErr:     true,
		},
		{
			name:        "no colon",
			destination: "host/repo-no-tag",
			wantErr:     true,
		},
		{
			name:        "empty string",
			destination: "",
			wantErr:     true,
		},
		{
			name:        "colon before slash",
			destination: "host:port/repo",
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ecrRepoName(tt.destination)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// --- EnsureRepository tests ---

func TestEnsureRepository_AlreadyExists(t *testing.T) {
	mock := &mockECR{} // DescribeRepositories succeeds → repo exists
	b := testBuilder(mock)

	err := b.EnsureRepository(context.Background(), "123456.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-abc/myapp:build1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.describedRepos) != 1 || mock.describedRepos[0] != "prod-tenant-abc/myapp" {
		t.Errorf("expected describe for prod-tenant-abc/myapp, got %v", mock.describedRepos)
	}
	if len(mock.createdRepos) != 0 {
		t.Errorf("should not have called CreateRepository, but got %v", mock.createdRepos)
	}
}

func TestEnsureRepository_CreatesWhenMissing(t *testing.T) {
	mock := &mockECR{
		describeErr: fmt.Errorf("RepositoryNotFoundException"),
	}
	b := testBuilder(mock)

	err := b.EnsureRepository(context.Background(), "123456.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-abc/myapp:build1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.createdRepos) != 1 || mock.createdRepos[0] != "prod-tenant-abc/myapp" {
		t.Errorf("expected create for prod-tenant-abc/myapp, got %v", mock.createdRepos)
	}
}

func TestEnsureRepository_RaceCondition(t *testing.T) {
	mock := &mockECR{
		describeErr: fmt.Errorf("RepositoryNotFoundException"),
		createErr:   fmt.Errorf("RepositoryAlreadyExistsException: The repository already exists"),
	}
	b := testBuilder(mock)

	err := b.EnsureRepository(context.Background(), "123456.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-abc/myapp:build1")
	if err != nil {
		t.Fatalf("should succeed on race condition, got: %v", err)
	}
}

func TestEnsureRepository_CreateError(t *testing.T) {
	mock := &mockECR{
		describeErr: fmt.Errorf("RepositoryNotFoundException"),
		createErr:   fmt.Errorf("AccessDeniedException: not authorized"),
	}
	b := testBuilder(mock)

	err := b.EnsureRepository(context.Background(), "123456.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-abc/myapp:build1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "create ECR repository prod-tenant-abc/myapp: AccessDeniedException: not authorized" {
		t.Errorf("unexpected error message: %s", got)
	}
}

func TestEnsureRepository_InvalidDestination(t *testing.T) {
	mock := &mockECR{}
	b := testBuilder(mock)

	err := b.EnsureRepository(context.Background(), "invalid")
	if err == nil {
		t.Fatal("expected error for invalid destination")
	}
	if len(mock.describedRepos) != 0 {
		t.Error("should not have called DescribeRepositories for invalid destination")
	}
}

// --- ECRImagePath tests ---

func TestECRImagePath(t *testing.T) {
	b := &Builder{
		cfg: &config.Config{
			Deployment: config.DeploymentConfig{
				RegistryURL: "https://123456.dkr.ecr.us-east-1.amazonaws.com",
				Environment: "prod",
			},
		},
	}

	got := b.ECRImagePath("account-uuid", "my-agent", "build42")
	want := "123456.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-account-uuid/my-agent:build42"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
