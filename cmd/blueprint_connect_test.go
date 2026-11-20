package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitrepo "github.com/astropods/astro-cli/internal/git"
)

const (
	testAuthorizeURL = "https://github.example/authorize?state=abc"
	testConnectBuild = "build-123"
)

type connectReply struct {
	status int
	body   any
}

// connectServer stands in for the endpoints connect calls. Every reply
// defaults to success, so a case overrides only the one it's about.
type connectServer struct {
	grant       connectReply
	grantPolls  []connectReply
	repo        connectReply
	scan        connectReply
	connections connectReply
	create      connectReply
	link        connectReply
	rebuild     connectReply

	mu         sync.Mutex
	calls      []string
	repoQuery  string
	scanQuery  url.Values
	linkBody   map[string]string
	createBody map[string]any
}

func newConnectServer() *connectServer {
	return &connectServer{
		grant:       connectReply{http.StatusOK, map[string]any{"connected": true, "github_login": "octocat"}},
		repo:        connectReply{http.StatusOK, map[string]any{"repo_full_name": "acme/agents", "default_branch": "main"}},
		scan:        connectReply{http.StatusOK, map[string]any{"found": true, "readable": true}},
		connections: connectReply{http.StatusOK, map[string]any{"connections": []any{}}},
		create:      connectReply{http.StatusCreated, map[string]any{"account": "testaccount", "name": "my-agent"}},
		link:        connectReply{http.StatusCreated, map[string]any{}},
		rebuild: connectReply{http.StatusAccepted, map[string]any{
			"build_id": testConnectBuild, "commit_sha": "abcdef1234567", "commit_message": "feat: first agent",
		}},
	}
}

func (s *connectServer) record(call string, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
	switch call {
	case "repo":
		s.repoQuery = r.URL.Query().Get("repo")
	case "scan":
		s.scanQuery = r.URL.Query()
	case "link":
		json.NewDecoder(r.Body).Decode(&s.linkBody) //nolint:errcheck
	case "create":
		json.NewDecoder(r.Body).Decode(&s.createBody) //nolint:errcheck
	}
}

func (s *connectServer) recorded() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

func (s *connectServer) handler() http.Handler {
	mux := http.NewServeMux()
	route := func(pattern, call string, reply func() connectReply) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			s.record(call, r)
			rep := reply()
			jsonHandler(rep.status, rep.body)(w, r)
		})
	}
	fixed := func(r *connectReply) func() connectReply { return func() connectReply { return *r } }
	polls := 0
	route("POST /api/v1/accounts/testaccount/github/connect", "grant", fixed(&s.grant))
	route("GET /api/v1/accounts/testaccount/github", "poll", func() connectReply {
		s.mu.Lock()
		defer s.mu.Unlock()
		rep := s.grantPolls[min(polls, len(s.grantPolls)-1)]
		polls++
		return rep
	})
	route("GET /api/v1/accounts/testaccount/github/repo", "repo", fixed(&s.repo))
	route("GET /api/v1/accounts/testaccount/github/scan", "scan", fixed(&s.scan))
	route("GET /api/v1/accounts/testaccount/github/connections", "connections", fixed(&s.connections))
	route("POST /api/v1/agents/testaccount", "create", fixed(&s.create))
	route("POST /api/v1/agents/testaccount/my-agent/github/link", "link", fixed(&s.link))
	route("POST /api/v1/agents/testaccount/my-agent/github/rebuild", "rebuild", fixed(&s.rebuild))
	return mux
}

// connectCheckout makes a throwaway clone with origin as its remote and moves
// into subdir, where it writes astropods.yml. Tests must never read the
// remotes of the repository they run in.
func connectCheckout(t *testing.T, origin, subdir string) string {
	t.Helper()
	root := t.TempDir()
	pipelineGitCommand(t, root, "init", "-q", "--initial-branch=main")
	pipelineGitCommand(t, root, "config", "user.email", "test@example.com")
	pipelineGitCommand(t, root, "config", "user.name", "Astro Test")
	pipelineGitCommand(t, root, "config", "commit.gpgsign", "false")
	pipelineGitCommand(t, root, "commit", "-q", "--allow-empty", "-m", "first")
	if origin != "" {
		pipelineGitCommand(t, root, "remote", "add", "origin", origin)
	}
	dir := filepath.Join(root, subdir)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "astropods.yml"), []byte("name: my-agent\n"), 0o600))
	t.Chdir(dir)
	return root
}

func fastGrantPolls(t *testing.T, timeout time.Duration) {
	t.Helper()
	interval, prevTimeout := githubGrantPollInterval, githubGrantTimeout
	githubGrantPollInterval, githubGrantTimeout = time.Millisecond, timeout
	t.Cleanup(func() { githubGrantPollInterval, githubGrantTimeout = interval, prevTimeout })
}

func runConnect(t *testing.T, s *connectServer, flags map[string]string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	setupBlueprintTest(t, s.handler())
	var errBuf bytes.Buffer
	blueprintConnectCmd.SetErr(&errBuf)
	t.Cleanup(func() { blueprintConnectCmd.SetErr(nil) })
	stdout, err = runBlueprintSubcommand(t, blueprintConnectCmd, flags, func() error {
		return runBlueprintConnect(blueprintConnectCmd, args)
	})
	return stripANSI(stdout), stripANSI(errBuf.String()), err
}

func TestBlueprintConnectLinksTheCheckoutAndBuilds(t *testing.T) {
	connectCheckout(t, "git@github.com:acme/agents.git", "")
	s := newConnectServer()

	out, _, err := runConnect(t, s, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"grant", "repo", "scan", "link", "rebuild"}, s.recorded())
	assert.Equal(t, "acme/agents", s.repoQuery)
	assert.Equal(t, map[string]string{"repo_full_name": "acme/agents", "branch": "main"}, s.linkBody)
	assert.Contains(t, out, msgConnected("my-agent", "acme/agents", "main"))
	assert.Contains(t, out, msgConnectBuildsUseLogin("octocat"))
	assert.Contains(t, out, msgConnectBuildStarted(testConnectBuild, "abcdef1234567", "feat: first agent"))
	assert.Contains(t, out, msgTailBuildLogs("my-agent", testConnectBuild))
}

func TestBlueprintConnectResolvesTheTarget(t *testing.T) {
	cases := []struct {
		name      string
		origin    string
		subdir    string
		flags     map[string]string
		args      []string
		repo      connectReply
		wantQuery string
		wantRepo  string
		wantScan  string
		wantBody  map[string]string
	}{
		{
			name:      "GitHub's casing replaces the remote's",
			origin:    "https://github.com/ACME/Agents.git",
			wantQuery: "ACME/Agents",
			wantBody:  map[string]string{"repo_full_name": "acme/agents", "branch": "main"},
		},
		{
			name:      "the directory holding astropods.yml is the subdirectory",
			origin:    "git@github.com:acme/agents.git",
			subdir:    "services/summarizer",
			wantQuery: "acme/agents",
			wantScan:  "acme/agents/services/summarizer",
			wantBody:  map[string]string{"repo_full_name": "acme/agents/services/summarizer", "branch": "main"},
		},
		{
			name:      "--path replaces the inferred subdirectory",
			origin:    "git@github.com:acme/agents.git",
			subdir:    "services/summarizer",
			flags:     map[string]string{"path": "/agents/other/"},
			wantQuery: "acme/agents",
			wantBody:  map[string]string{"repo_full_name": "acme/agents/agents/other", "branch": "main"},
		},
		{
			name:      "--repo ignores the checkout and takes the default branch",
			origin:    "git@github.com:acme/agents.git",
			subdir:    "services/summarizer",
			flags:     map[string]string{"repo": "https://github.com/acme/other"},
			repo:      connectReply{http.StatusOK, map[string]any{"repo_full_name": "acme/other", "default_branch": "develop"}},
			wantQuery: "acme/other",
			wantBody:  map[string]string{"repo_full_name": "acme/other", "branch": "develop"},
		},
		{
			name:      "--branch replaces the checked out branch",
			origin:    "git@github.com:acme/agents.git",
			flags:     map[string]string{"branch": "release"},
			wantQuery: "acme/agents",
			wantBody:  map[string]string{"repo_full_name": "acme/agents", "branch": "release"},
		},
		{
			name:      "an argument names the blueprint",
			origin:    "git@github.com:acme/agents.git",
			args:      []string{"my-agent"},
			wantQuery: "acme/agents",
			wantBody:  map[string]string{"repo_full_name": "acme/agents", "branch": "main"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			connectCheckout(t, tc.origin, tc.subdir)
			s := newConnectServer()
			if tc.repo.status != 0 {
				s.repo = tc.repo
			}

			_, _, err := runConnect(t, s, tc.flags, tc.args...)
			require.NoError(t, err)
			assert.Equal(t, tc.wantQuery, s.repoQuery)
			assert.Equal(t, tc.wantBody, s.linkBody)
			wantScan := tc.wantScan
			if wantScan == "" {
				wantScan = tc.wantBody["repo_full_name"]
			}
			assert.Equal(t, wantScan, s.scanQuery.Get("repo"))
			assert.Equal(t, tc.wantBody["branch"], s.scanQuery.Get("branch"))
		})
	}
}

// Every one of these is answered from the checkout, before authenticating.
func TestBlueprintConnectLocalFailures(t *testing.T) {
	t.Run("no origin remote", func(t *testing.T) {
		connectCheckout(t, "", "")
		s := newConnectServer()
		_, _, err := runConnect(t, s, nil)
		require.EqualError(t, err, errConnectNoRemote(gitrepo.ErrNoOrigin).Error())
		assert.Empty(t, s.recorded())
	})

	t.Run("not a repository", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "astropods.yml"), []byte("name: my-agent\n"), 0o600))
		t.Chdir(dir)
		s := newConnectServer()
		_, _, err := runConnect(t, s, nil)
		require.EqualError(t, err, errConnectNoRemote(gitrepo.ErrNotARepo).Error())
		assert.Empty(t, s.recorded())
	})

	t.Run("a remote that isn't GitHub", func(t *testing.T) {
		connectCheckout(t, "git@gitlab.com:acme/agents.git", "")
		s := newConnectServer()
		_, _, err := runConnect(t, s, nil)
		require.EqualError(t, err, errConnectNotGitHub("git@gitlab.com:acme/agents.git").Error())
		assert.Empty(t, s.recorded())
	})

	t.Run("detached HEAD", func(t *testing.T) {
		root := connectCheckout(t, "git@github.com:acme/agents.git", "")
		pipelineGitCommand(t, root, "checkout", "-q", "--detach")
		s := newConnectServer()
		_, _, err := runConnect(t, s, nil)
		require.EqualError(t, err, errConnectNeedsBranch(gitrepo.ErrDetachedHead).Error())
		assert.Empty(t, s.recorded())
	})

	t.Run("no name and no spec", func(t *testing.T) {
		t.Chdir(t.TempDir())
		s := newConnectServer()
		_, _, err := runConnect(t, s, nil)
		require.EqualError(t, err, errConnectNeedsBlueprintName().Error())
		assert.Empty(t, s.recorded())
	})
}

func TestBlueprintConnectBranchChecks(t *testing.T) {
	t.Run("a branch GitHub doesn't have is refused before linking", func(t *testing.T) {
		connectCheckout(t, "git@github.com:acme/agents.git", "")
		s := newConnectServer()
		s.scan = connectReply{http.StatusOK, map[string]any{"found": false, "readable": false}}

		_, _, err := runConnect(t, s, nil)
		require.EqualError(t, err, errConnectBranchNotOnGitHub("acme/agents", "main").Error())
		assert.Equal(t, []string{"grant", "repo", "scan"}, s.recorded())
	})

	t.Run("a branch without astropods.yml connects without building", func(t *testing.T) {
		connectCheckout(t, "git@github.com:acme/agents.git", "")
		s := newConnectServer()
		s.scan = connectReply{http.StatusOK, map[string]any{"found": false, "readable": true}}

		out, _, err := runConnect(t, s, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"grant", "repo", "scan", "link"}, s.recorded())
		assert.Contains(t, out, msgConnectNoSpecOnBranch("acme/agents", "main"))
	})

	t.Run("--no-build connects without building", func(t *testing.T) {
		connectCheckout(t, "git@github.com:acme/agents.git", "")
		s := newConnectServer()

		_, _, err := runConnect(t, s, map[string]string{"no-build": "true"})
		require.NoError(t, err)
		assert.Equal(t, []string{"grant", "repo", "scan", "link"}, s.recorded())
	})

	t.Run("a refused build doesn't fail the connection", func(t *testing.T) {
		connectCheckout(t, "git@github.com:acme/agents.git", "")
		s := newConnectServer()
		s.rebuild = connectReply{http.StatusNotFound, map[string]any{"error": "no GitHub connection for this agent"}}

		out, _, err := runConnect(t, s, nil)
		require.NoError(t, err)
		assert.Contains(t, out, msgConnectBuildNotStarted("my-agent", explainError(errRebuildUnavailable("my-agent", "testaccount"))))
	})
}

func TestBlueprintConnectServerRefusals(t *testing.T) {
	cases := []struct {
		name    string
		set     func(*connectServer)
		wantErr error
	}{
		{
			name: "repository not found",
			set: func(s *connectServer) {
				s.repo = connectReply{http.StatusNotFound, map[string]any{"error": "repository_not_found"}}
			},
			wantErr: errConnectRepoNotFound("acme/agents"),
		},
		{
			name: "repository access denied",
			set: func(s *connectServer) {
				s.repo = connectReply{http.StatusForbidden, map[string]any{"error": "repository_access_denied"}}
			},
			wantErr: errConnectRepoAccessDenied("acme/agents"),
		},
		{
			name: "GitHub authorization went stale",
			set: func(s *connectServer) {
				s.repo = connectReply{http.StatusUnprocessableEntity, map[string]any{"error": "github_not_connected"}}
			},
			wantErr: errConnectGitHubNotConnected(),
		},
		{
			name: "rate limited during the scan",
			set: func(s *connectServer) {
				s.scan = connectReply{http.StatusTooManyRequests, map[string]any{"error": "rate_limit_exceeded"}}
			},
			wantErr: errConnectGitHubRateLimited(),
		},
		{
			name: "push access required",
			set: func(s *connectServer) {
				s.link = connectReply{http.StatusForbidden, map[string]any{"error": "repository_push_required"}}
			},
			wantErr: errConnectRepoPushRequired("acme/agents"),
		},
		{
			name: "webhook permission required",
			set: func(s *connectServer) {
				s.link = connectReply{http.StatusForbidden, map[string]any{"error": "github_webhook_permission_required"}}
			},
			wantErr: errConnectWebhookPermission("acme/agents"),
		},
		{
			name: "webhook setup unavailable",
			set: func(s *connectServer) {
				s.link = connectReply{http.StatusServiceUnavailable, map[string]any{"error": "webhook_setup_unavailable"}}
			},
			wantErr: errConnectWebhookUnavailable(),
		},
		{
			name: "repository already connected to another blueprint",
			set: func(s *connectServer) {
				s.link = connectReply{http.StatusConflict, map[string]any{"error": `repo "acme/agents" is already connected to agent "other"`}}
			},
			wantErr: errConnectRepoAlreadyConnected("acme/agents", errors.New(`Repo "acme/agents" is already connected to agent "other".`)),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			connectCheckout(t, "git@github.com:acme/agents.git", "")
			s := newConnectServer()
			tc.set(s)

			_, _, err := runConnect(t, s, nil)
			require.EqualError(t, err, tc.wantErr.Error())
			assert.NotContains(t, s.recorded(), "rebuild")
		})
	}

	t.Run("an error without a code keeps the server's explanation", func(t *testing.T) {
		connectCheckout(t, "git@github.com:acme/agents.git", "")
		s := newConnectServer()
		s.link = connectReply{http.StatusUnprocessableEntity, map[string]any{"error": `subdirectory "x" not found in acme/agents on branch main`}}

		_, _, err := runConnect(t, s, nil)
		var apiErr *apiError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, `subdirectory "x" not found in acme/agents on branch main`, apiErr.Message)
	})
}

func TestBlueprintConnectGitHubAuthorization(t *testing.T) {
	needsAuthorization := connectReply{http.StatusOK, map[string]any{"redirect_url": testAuthorizeURL}}

	t.Run("waits for the server to see the authorization", func(t *testing.T) {
		connectCheckout(t, "git@github.com:acme/agents.git", "")
		fastGrantPolls(t, time.Minute)
		s := newConnectServer()
		s.grant = needsAuthorization
		s.grantPolls = []connectReply{
			{http.StatusOK, map[string]any{"connected": false}},
			{http.StatusOK, map[string]any{"connected": true, "github_login": "octocat"}},
		}

		out, _, err := runConnect(t, s, map[string]string{"no-browser": "true"})
		require.NoError(t, err)
		assert.Equal(t, []string{"grant", "poll", "poll", "repo", "scan", "link", "rebuild"}, s.recorded())
		assert.Contains(t, out, testAuthorizeURL)
		assert.Contains(t, out, msgConnectGitHubAuthorized("octocat"))
	})

	t.Run("gives up when the authorization never arrives", func(t *testing.T) {
		connectCheckout(t, "git@github.com:acme/agents.git", "")
		fastGrantPolls(t, 20*time.Millisecond)
		s := newConnectServer()
		s.grant = needsAuthorization
		s.grantPolls = []connectReply{{http.StatusOK, map[string]any{"connected": false}}}

		_, _, err := runConnect(t, s, map[string]string{"no-browser": "true"})
		require.EqualError(t, err, errConnectGitHubAuthorizationTimedOut(testAuthorizeURL).Error())
		assert.NotContains(t, s.recorded(), "link")
	})

	t.Run("non-interactive stops with the link instead of waiting", func(t *testing.T) {
		connectCheckout(t, "git@github.com:acme/agents.git", "")
		setNonInteractiveFlag(t)
		s := newConnectServer()
		s.grant = needsAuthorization

		_, _, err := runConnect(t, s, nil)
		require.EqualError(t, err, errConnectGitHubAuthorizationRequired(testAuthorizeURL).Error())
		assert.Equal(t, []string{"grant"}, s.recorded())
	})
}

func TestBlueprintConnectJSON(t *testing.T) {
	connectCheckout(t, "git@github.com:acme/agents.git", "")
	s := newConnectServer()

	out, progress, err := runConnect(t, s, map[string]string{"json": "true"})
	require.NoError(t, err)
	var got connectResult
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	assert.Equal(t, connectResult{Blueprint: "my-agent", Repo: "acme/agents", Branch: "main", BuildID: testConnectBuild}, got)
	assert.Contains(t, progress, msgConnected("my-agent", "acme/agents", "main"))
}

func runCreateConnect(t *testing.T, s *connectServer, flags map[string]string) (string, error) {
	t.Helper()
	setupBlueprintTest(t, s.handler())
	out, err := runBlueprintSubcommand(t, blueprintCreateCmd, flags, func() error {
		return runBlueprintCreate(blueprintCreateCmd, []string{"my-agent"})
	})
	return stripANSI(out), err
}

func TestBlueprintCreateConnect(t *testing.T) {
	t.Run("creates with the repository, then links and builds", func(t *testing.T) {
		connectCheckout(t, "git@github.com:acme/agents.git", "services/summarizer")
		s := newConnectServer()

		out, err := runCreateConnect(t, s, map[string]string{"connect": "true"})
		require.NoError(t, err)
		assert.Equal(t, []string{"grant", "repo", "scan", "connections", "create", "link", "rebuild"}, s.recorded())
		assert.Equal(t, map[string]any{"repo_full_name": "acme/agents"}, s.createBody["github"])
		assert.Equal(t, "my-agent", s.createBody["name"])
		assert.Equal(t, map[string]string{"repo_full_name": "acme/agents/services/summarizer", "branch": "main"}, s.linkBody)
		assert.Contains(t, out, msgConnected("my-agent", "acme/agents/services/summarizer", "main"))
	})

	t.Run("--repo implies --connect", func(t *testing.T) {
		connectCheckout(t, "git@github.com:acme/agents.git", "")
		s := newConnectServer()

		_, err := runCreateConnect(t, s, map[string]string{"repo": "acme/agents"})
		require.NoError(t, err)
		assert.Contains(t, s.recorded(), "link")
		assert.NotNil(t, s.createBody["github"])
	})

	t.Run("a repository connected elsewhere is refused before creating", func(t *testing.T) {
		connectCheckout(t, "git@github.com:acme/agents.git", "")
		s := newConnectServer()
		s.connections = connectReply{http.StatusOK, map[string]any{"connections": []any{
			map[string]any{"agent_name": "other", "repo_full_name": "Acme/Agents"},
		}}}

		_, err := runCreateConnect(t, s, map[string]string{"connect": "true"})
		require.EqualError(t, err, errConnectRepoConnectedTo("acme/agents", "other").Error())
		assert.NotContains(t, s.recorded(), "create")
	})

	t.Run("a webhook refusal from create leaves nothing to link", func(t *testing.T) {
		connectCheckout(t, "git@github.com:acme/agents.git", "")
		s := newConnectServer()
		s.create = connectReply{http.StatusForbidden, map[string]any{"error": "github_webhook_permission_required"}}

		_, err := runCreateConnect(t, s, map[string]string{"connect": "true"})
		require.EqualError(t, err, errConnectWebhookPermission("acme/agents").Error())
		assert.NotContains(t, s.recorded(), "link")
	})

	t.Run("a branch GitHub doesn't have is refused before creating", func(t *testing.T) {
		connectCheckout(t, "git@github.com:acme/agents.git", "")
		s := newConnectServer()
		s.scan = connectReply{http.StatusOK, map[string]any{"found": false, "readable": false}}

		_, err := runCreateConnect(t, s, map[string]string{"connect": "true"})
		require.EqualError(t, err, errConnectBranchNotOnGitHub("acme/agents", "main").Error())
		assert.NotContains(t, s.recorded(), "create")
	})
}

func TestSameGitHubRepository(t *testing.T) {
	cases := []struct {
		stored, base, subPath string
		want                  bool
	}{
		{"acme/agents", "acme/agents", "", true},
		{"Acme/Agents", "acme/agents", "", true},
		{"acme/agents/svc/a", "acme/agents", "svc/a", true},
		{"acme/agents/svc/a", "acme/agents", "svc/b", false},
		{"acme/agents/Svc", "acme/agents", "svc", false},
		{"acme/agents/svc", "acme/agents", "", false},
		{"acme/other", "acme/agents", "", false},
	}
	for _, tc := range cases {
		t.Run(strings.Join([]string{tc.stored, tc.base, tc.subPath}, "|"), func(t *testing.T) {
			assert.Equal(t, tc.want, sameGitHubRepository(tc.stored, tc.base, tc.subPath))
		})
	}
}
