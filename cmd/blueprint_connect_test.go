package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// connectStub is the server half of a connect run: which GitHub state the
// account is in, and what each endpoint answers.
type connectStub struct {
	grantConnected bool
	// grantAfterPolls makes the status endpoint report connected only once it
	// has been asked this many times, standing in for the browser round trip.
	grantAfterPolls int

	linkStatus int
	linkBody   any

	rebuildStatus int
	rebuildBody   any

	repairStatus int
	repairBody   any

	// observed
	linkBody4Server map[string]string
	polls           int
	links           int
	rebuilds        int
	repairs         int
}

func (s *connectStub) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/accounts/testaccount/github/connect", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		if s.grantConnected {
			jsonHandler(http.StatusOK, map[string]any{"connected": true, "github_login": "octocat"})(w, r)
			return
		}
		jsonHandler(http.StatusOK, map[string]any{"redirect_url": "https://example.invalid/authorize"})(w, r)
	})

	mux.HandleFunc("/api/v1/accounts/testaccount/github", func(w http.ResponseWriter, r *http.Request) {
		s.polls++
		connected := s.polls >= s.grantAfterPolls
		jsonHandler(http.StatusOK, map[string]any{"connected": connected, "github_login": "octocat"})(w, r)
	})

	mux.HandleFunc("/api/v1/agents/testaccount/my-agent/github/link", func(w http.ResponseWriter, r *http.Request) {
		s.links++
		assert.Equal(t, http.MethodPost, r.Method)
		json.NewDecoder(r.Body).Decode(&s.linkBody4Server) //nolint:errcheck
		jsonHandler(s.linkStatus, s.linkBody)(w, r)
	})

	mux.HandleFunc("/api/v1/agents/testaccount/my-agent/github/rebuild", func(w http.ResponseWriter, r *http.Request) {
		s.rebuilds++
		jsonHandler(s.rebuildStatus, s.rebuildBody)(w, r)
	})

	mux.HandleFunc("/api/v1/agents/testaccount/my-agent/github/webhook/repair", func(w http.ResponseWriter, r *http.Request) {
		s.repairs++
		jsonHandler(s.repairStatus, s.repairBody)(w, r)
	})

	return mux
}

// newConnectCmd returns a bare command carrying only a context and buffers, so
// a test never depends on flag state left behind by a sibling.
func newConnectCmd(t *testing.T) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetContext(context.Background())
	return cmd, &out, &errOut
}

func boolPtr(b bool) *bool { return &b }

func TestConnectBlueprintRepo(t *testing.T) {
	linkedWithWebhook := map[string]any{
		"repo_full_name": "acme/agents", "branch": "main", "webhook_installed": true,
	}

	cases := []struct {
		name     string
		stub     connectStub
		opts     connectOptions
		wantErr  error
		wantOut  []string
		wantSkip []string
		assert   func(t *testing.T, s *connectStub)
	}{
		{
			name: "already authorized links and builds without a browser",
			stub: connectStub{
				grantConnected: true,
				linkStatus:     http.StatusCreated, linkBody: linkedWithWebhook,
				rebuildStatus: http.StatusAccepted, rebuildBody: map[string]any{"build_id": "bld_1"},
			},
			opts:     connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main"},
			wantOut:  []string{msgConnectLinkedAndBuilding("acme/agents", "main"), msgConnectBuildStarted("bld_1")},
			wantSkip: []string{msgConnectGitHubAuthorizationNeeded()},
			assert: func(t *testing.T, s *connectStub) {
				assert.Zero(t, s.polls, "no poll when the account is already authorized")
				assert.Equal(t, "acme/agents", s.linkBody4Server["repo_full_name"])
				assert.Equal(t, "main", s.linkBody4Server["branch"])
			},
		},
		{
			name: "not authorized waits for the grant, then links",
			stub: connectStub{
				grantConnected: false, grantAfterPolls: 2,
				linkStatus: http.StatusCreated, linkBody: linkedWithWebhook,
				rebuildStatus: http.StatusAccepted, rebuildBody: map[string]any{"build_id": "bld_2"},
			},
			opts:    connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main", NoBrowser: true},
			wantOut: []string{msgConnectGitHubAuthorizationNeeded(), msgConnectGitHubAuthorized("octocat")},
			assert: func(t *testing.T, s *connectStub) {
				assert.Equal(t, 2, s.polls)
				assert.Equal(t, 1, s.links)
			},
		},
		{
			name: "webhook did not install says pushes will not build",
			stub: connectStub{
				grantConnected: true,
				linkStatus:     http.StatusCreated,
				linkBody:       map[string]any{"repo_full_name": "acme/agents", "branch": "main", "webhook_installed": false},
				rebuildStatus:  http.StatusAccepted, rebuildBody: map[string]any{"build_id": "bld_3"},
			},
			opts: connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main"},
			wantOut: []string{
				msgConnectLinked("acme/agents", "main"),
				msgConnectWebhookMissing(),
				msgConnectWebhookMissingRemedy(),
			},
			wantSkip: []string{msgConnectLinkedAndBuilding("acme/agents", "main")},
		},
		{
			name: "server that does not report webhook state says so",
			stub: connectStub{
				grantConnected: true,
				linkStatus:     http.StatusCreated,
				linkBody:       map[string]any{"repo_full_name": "acme/agents", "branch": "main"},
				rebuildStatus:  http.StatusAccepted, rebuildBody: map[string]any{"build_id": "bld_4"},
			},
			opts: connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main"},
			wantOut: []string{
				msgConnectLinked("acme/agents", "main"),
				msgConnectWebhookUnknown(),
			},
			wantSkip: []string{msgConnectWebhookMissing()},
		},
		{
			name: "no-build skips the build",
			stub: connectStub{
				grantConnected: true,
				linkStatus:     http.StatusCreated, linkBody: linkedWithWebhook,
			},
			opts:    connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main", NoBuild: true},
			wantOut: []string{msgConnectNoBuildRequested()},
			assert: func(t *testing.T, s *connectStub) {
				assert.Zero(t, s.rebuilds)
			},
		},
		{
			name: "a refused build warns but keeps the connection",
			stub: connectStub{
				grantConnected: true,
				linkStatus:     http.StatusCreated, linkBody: linkedWithWebhook,
				rebuildStatus: http.StatusForbidden, rebuildBody: map[string]any{"error": "not allowed"},
			},
			opts:    connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main"},
			wantOut: []string{msgConnectLinkedAndBuilding("acme/agents", "main")},
			assert: func(t *testing.T, s *connectStub) {
				assert.Equal(t, 1, s.rebuilds)
			},
		},
		{
			name: "repair reinstalls the webhook without relinking",
			stub: connectStub{
				grantConnected: true,
				repairStatus:   http.StatusOK, repairBody: map[string]any{"webhook_installed": true},
				rebuildStatus: http.StatusAccepted, rebuildBody: map[string]any{"build_id": "bld_5"},
			},
			opts:    connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main", Repair: true, NoBuild: true},
			wantOut: []string{msgConnectLinkedAndBuilding("acme/agents", "main")},
			assert: func(t *testing.T, s *connectStub) {
				assert.Equal(t, 1, s.repairs)
				assert.Zero(t, s.links, "repair must not relink")
			},
		},
		{
			name: "repair with no connection to repair",
			stub: connectStub{
				grantConnected: true,
				repairStatus:   http.StatusNotFound, repairBody: map[string]any{"error": "no GitHub connection for this agent"},
			},
			opts:    connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main", Repair: true},
			wantErr: errConnectNothingToRepair("my-agent"),
		},
		{
			name: "repair against a server that has no repair endpoint",
			stub: connectStub{
				grantConnected: true,
				// The route itself is absent, so the body is not our JSON error.
				repairStatus: http.StatusNotFound, repairBody: nil,
			},
			opts:    connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main", Repair: true},
			wantErr: errConnectRepairUnsupported(),
		},
		{
			name: "repo already connected to another blueprint",
			stub: connectStub{
				grantConnected: true,
				linkStatus:     http.StatusConflict,
				linkBody:       map[string]any{"error": `repo "acme/agents" is already connected to agent "other"`},
			},
			opts:    connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main"},
			wantErr: errConnectRepoTakenByAnotherBlueprint("acme/agents", `repo "acme/agents" is already connected to agent "other"`),
		},
		{
			name: "connection belongs to another member",
			stub: connectStub{
				grantConnected: true,
				linkStatus:     http.StatusForbidden,
				linkBody:       map[string]any{"error": "this GitHub connection belongs to another member"},
			},
			opts:    connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main"},
			wantErr: errConnectOwnedByAnotherMember(),
		},
		{
			name: "blueprint does not exist",
			stub: connectStub{
				grantConnected: true,
				linkStatus:     http.StatusNotFound,
				linkBody:       map[string]any{"error": `agent "my-agent" not found`},
			},
			opts:    connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main"},
			wantErr: errConnectBlueprintNotFound("my-agent", "testaccount"),
		},
		{
			name: "github authorization went stale between calls",
			stub: connectStub{
				grantConnected: true,
				linkStatus:     http.StatusUnprocessableEntity,
				linkBody:       map[string]any{"error": githubNotConnected},
			},
			opts:    connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main"},
			wantErr: errConnectGitHubDisconnected(),
		},
		{
			name: "subdirectory missing from the repository",
			stub: connectStub{
				grantConnected: true,
				linkStatus:     http.StatusUnprocessableEntity,
				linkBody:       map[string]any{"error": `subdirectory "services/nope" not found in acme/agents on branch main`},
			},
			opts:    connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main", Path: "services/nope"},
			wantErr: errConnectRejected(`subdirectory "services/nope" not found in acme/agents on branch main`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := tc.stub
			setupBlueprintTest(t, stub.handler(t))
			shrinkGrantPoll(t)

			cmd, out, _ := newConnectCmd(t)
			err := connectBlueprintRepo(cmd, AccountToken{Account: "testaccount", Token: "tok"}, tc.opts, "acme/agents", "main", false)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.Equal(t, tc.wantErr.Error(), err.Error())
				return
			}
			require.NoError(t, err)
			for _, want := range tc.wantOut {
				assert.Contains(t, out.String(), want)
			}
			for _, unwanted := range tc.wantSkip {
				assert.NotContains(t, out.String(), unwanted)
			}
			if tc.assert != nil {
				tc.assert(t, &stub)
			}
		})
	}
}

func TestConnectBlueprintRepoJSON(t *testing.T) {
	stub := connectStub{
		grantConnected: true,
		linkStatus:     http.StatusCreated,
		linkBody:       map[string]any{"repo_full_name": "acme/agents", "branch": "main", "webhook_installed": true},
		rebuildStatus:  http.StatusAccepted, rebuildBody: map[string]any{"build_id": "bld_9"},
	}
	setupBlueprintTest(t, stub.handler(t))
	shrinkGrantPoll(t)

	cmd, out, errOut := newConnectCmd(t)
	opts := connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main", JSON: true}
	require.NoError(t, connectBlueprintRepo(cmd, AccountToken{Account: "testaccount", Token: "tok"}, opts, "acme/agents", "main", false))

	var got connectResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.Equal(t, connectResult{
		Blueprint:        "my-agent",
		Repo:             "acme/agents",
		Branch:           "main",
		WebhookInstalled: boolPtr(true),
		BuildID:          "bld_9",
	}, got)
	// Progress must not contaminate the JSON on stdout.
	assert.Contains(t, errOut.String(), "Connecting")
}

func TestConnectGrantTimesOut(t *testing.T) {
	// grantAfterPolls beyond anything the shrunken deadline allows.
	stub := connectStub{grantConnected: false, grantAfterPolls: 1000}
	setupBlueprintTest(t, stub.handler(t))
	githubGrantPollInterval = time.Millisecond
	githubGrantTimeout = 20 * time.Millisecond
	t.Cleanup(restoreGrantPoll(3*time.Second, 5*time.Minute))

	cmd, _, _ := newConnectCmd(t)
	opts := connectOptions{Name: "my-agent", Repo: "acme/agents", Branch: "main", NoBrowser: true}
	err := connectBlueprintRepo(cmd, AccountToken{Account: "testaccount", Token: "tok"}, opts, "acme/agents", "main", false)

	require.Error(t, err)
	assert.Equal(t, errConnectGitHubTimedOut().Error(), err.Error())
}

func TestResolveRepoAndBranch(t *testing.T) {
	ctx := context.Background()

	t.Run("flags", func(t *testing.T) {
		cases := []struct {
			name       string
			opts       connectOptions
			wantRepo   string
			wantBranch string
			wantErr    error
		}{
			{
				name:     "owner/repo and branch",
				opts:     connectOptions{Repo: "acme/agents", Branch: "main"},
				wantRepo: "acme/agents", wantBranch: "main",
			},
			{
				name:     "ssh url",
				opts:     connectOptions{Repo: "git@github.com:acme/agents.git", Branch: "dev"},
				wantRepo: "acme/agents", wantBranch: "dev",
			},
			{
				name:     "subdirectory is appended",
				opts:     connectOptions{Repo: "acme/agents", Branch: "main", Path: "/services/summarizer/"},
				wantRepo: "acme/agents/services/summarizer", wantBranch: "main",
			},
			{
				name:    "non-github host",
				opts:    connectOptions{Repo: "https://gitlab.com/acme/agents.git", Branch: "main"},
				wantErr: errConnectNotGitHub("https://gitlab.com/acme/agents.git"),
			},
			{
				name:    "unreadable repo",
				opts:    connectOptions{Repo: "https://github.com/acme", Branch: "main"},
				wantErr: errConnectUnreadableRepo("https://github.com/acme"),
			},
			{
				name:    "explicit repo without a branch",
				opts:    connectOptions{Repo: "acme/agents"},
				wantErr: errConnectNeedsBranch(),
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				repo, branch, err := resolveRepoAndBranch(ctx, tc.opts)
				if tc.wantErr != nil {
					require.Error(t, err)
					assert.Equal(t, tc.wantErr.Error(), err.Error())
					return
				}
				require.NoError(t, err)
				assert.Equal(t, tc.wantRepo, repo)
				assert.Equal(t, tc.wantBranch, branch)
			})
		}
	})

	t.Run("infers both from the checkout", func(t *testing.T) {
		chdirToStubRepo(t, "git@github.com:acme/agents.git")
		repo, branch, err := resolveRepoAndBranch(ctx, connectOptions{})
		require.NoError(t, err)
		assert.Equal(t, "acme/agents", repo)
		assert.Equal(t, "main", branch)
	})

	t.Run("outside a repository", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, _, err := resolveRepoAndBranch(ctx, connectOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not read a repository from this directory")
	})
}

func TestBlueprintCreateConnects(t *testing.T) {
	stub := connectStub{
		grantConnected: true,
		linkStatus:     http.StatusCreated,
		linkBody:       map[string]any{"repo_full_name": "acme/agents", "branch": "main", "webhook_installed": true},
		rebuildStatus:  http.StatusAccepted, rebuildBody: map[string]any{"build_id": "bld_c"},
	}
	mux := http.NewServeMux()
	created := 0
	mux.HandleFunc("/api/v1/agents/testaccount", func(w http.ResponseWriter, r *http.Request) {
		created++
		jsonHandler(http.StatusCreated, map[string]any{"account": "testaccount", "name": "my-agent"})(w, r)
	})
	inner := stub.handler(t)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { inner.ServeHTTP(w, r) })

	setupBlueprintTest(t, mux)
	shrinkGrantPoll(t)

	require.NoError(t, blueprintCreateCmd.Flags().Set("repo", "acme/agents"))
	require.NoError(t, blueprintCreateCmd.Flags().Set("branch", "main"))
	t.Cleanup(func() {
		blueprintCreateCmd.Flags().Set("repo", "")   //nolint:errcheck
		blueprintCreateCmd.Flags().Set("branch", "") //nolint:errcheck
	})
	var out bytes.Buffer
	blueprintCreateCmd.SetOut(&out)
	blueprintCreateCmd.SetContext(context.Background())

	require.NoError(t, runBlueprintCreate(blueprintCreateCmd, []string{"my-agent"}))

	assert.Equal(t, 1, created)
	assert.Equal(t, 1, stub.links, "--repo implies --connect")
	assert.Contains(t, out.String(), msgConnectLinkedAndBuilding("acme/agents", "main"))
}

func shrinkGrantPoll(t *testing.T) {
	t.Helper()
	githubGrantPollInterval = time.Millisecond
	githubGrantTimeout = 5 * time.Second
	t.Cleanup(restoreGrantPoll(3*time.Second, 5*time.Minute))
}

func restoreGrantPoll(interval, timeout time.Duration) func() {
	return func() {
		githubGrantPollInterval = interval
		githubGrantTimeout = timeout
	}
}

// chdirToStubRepo makes a throwaway repository with the given origin and moves
// the test into it, so remote inference runs against real git.
func chdirToStubRepo(t *testing.T, origin string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "first"},
		{"remote", "add", "origin", origin},
	} {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	t.Chdir(dir)
}

// A repository the CLI cannot read must stop the run before the blueprint is
// created, or a failed connect leaves a blueprint nobody asked for on its own.
func TestBlueprintCreateValidatesRepoBeforeCreating(t *testing.T) {
	created := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/agents/testaccount", func(w http.ResponseWriter, r *http.Request) {
		created++
		jsonHandler(http.StatusCreated, map[string]any{"account": "testaccount", "name": "my-agent"})(w, r)
	})
	setupBlueprintTest(t, mux)

	require.NoError(t, blueprintCreateCmd.Flags().Set("repo", "https://gitlab.com/acme/agents.git"))
	require.NoError(t, blueprintCreateCmd.Flags().Set("branch", "main"))
	t.Cleanup(func() {
		blueprintCreateCmd.Flags().Set("repo", "")   //nolint:errcheck
		blueprintCreateCmd.Flags().Set("branch", "") //nolint:errcheck
	})
	var out bytes.Buffer
	blueprintCreateCmd.SetOut(&out)
	blueprintCreateCmd.SetContext(context.Background())

	err := runBlueprintCreate(blueprintCreateCmd, []string{"my-agent"})

	require.Error(t, err)
	assert.Equal(t, errConnectNotGitHub("https://gitlab.com/acme/agents.git").Error(), err.Error())
	assert.Zero(t, created, "the blueprint must not be created when the repository cannot be read")
}
