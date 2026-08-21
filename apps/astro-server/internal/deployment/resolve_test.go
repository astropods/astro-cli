package deployment

import (
	"strings"
	"testing"
)

// findResolution returns the first Resolution matching the role+env name,
// or nil if none. Used to assert provenance (Source, IsSecret, etc.).
func findResolution(rs []Resolution, role Role, envName string) *Resolution {
	for i := range rs {
		if rs[i].Role == role && rs[i].EnvName == envName {
			return &rs[i]
		}
	}
	return nil
}

func TestResolve_SlackVarsDoNotLeakToAgent(t *testing.T) {
	// The cross-role leak case from the bug that motivated this work:
	// a SLACK_BOT_TOKEN with target=interface.slack must not produce
	// any row for role='agent'.
	ds := &AstroDeploymentSpec{
		Source: DeploymentSource{Name: "myagent", Build: "build1"},
		Agent: DeploymentAgent{
			Image:     "agent:latest",
			Endpoints: httpEndpoints(8080),
		},
		Variables: map[string]Variable{
			"ANTHROPIC_API_KEY": {
				Secret: true, Value: "sk-ant-...",
				Targets: []string{"agent"},
			},
			"SLACK_BOT_TOKEN": {
				Secret: true, Value: "xoxb-...",
				Targets: []string{"interface.slack"},
			},
			"SLACK_APP_TOKEN": {
				Secret: true, Value: "xapp-...",
				Targets: []string{"interface.slack"},
			},
		},
		Interfaces: &DeploymentInterfaces{
			Adapters: []string{"slack"},
		},
	}

	rs, err := Resolve(ds, ResolveOptions{Namespace: "ns"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if got := findResolution(rs, RoleAgent, "SLACK_BOT_TOKEN"); got != nil {
		t.Errorf("agent should not have SLACK_BOT_TOKEN row; got %+v", got)
	}
	if got := findResolution(rs, RoleAgent, "SLACK_APP_TOKEN"); got != nil {
		t.Errorf("agent should not have SLACK_APP_TOKEN row; got %+v", got)
	}
	// Anthropic on agent only.
	got := findResolution(rs, RoleAgent, "ANTHROPIC_API_KEY")
	if got == nil {
		t.Fatal("agent missing ANTHROPIC_API_KEY")
	}
	if !got.IsSecret || got.Source != EnvSourceUserVar {
		t.Errorf("ANTHROPIC_API_KEY: expected secret user_var, got %+v", got)
	}
	// Slack vars on messaging only.
	if got := findResolution(rs, RoleMessaging, "SLACK_BOT_TOKEN"); got == nil {
		t.Error("messaging missing SLACK_BOT_TOKEN")
	}
	if got := findResolution(rs, RoleMessaging, "ANTHROPIC_API_KEY"); got != nil {
		t.Errorf("messaging should not have ANTHROPIC_API_KEY; got %+v", got)
	}
}

func TestResolve_GatewayModelInjectedAsLiteralEnv(t *testing.T) {
	// A gateway model choice lives as a literal in agent.environment (not a
	// variable) and must surface as a plain agent env row.
	ds := &AstroDeploymentSpec{
		Source: DeploymentSource{Name: "myagent", Build: "build1"},
		Agent: DeploymentAgent{
			Image:       "agent:latest",
			Endpoints:   httpEndpoints(8080),
			Environment: map[string]string{"MODEL_DEFAULT": "gpt-4o"},
		},
	}

	rs, err := Resolve(ds, ResolveOptions{Namespace: "ns"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	got := findResolution(rs, RoleAgent, "MODEL_DEFAULT")
	if got == nil {
		t.Fatal("agent missing MODEL_DEFAULT env row")
	}
	if got.Value != "gpt-4o" || got.IsSecret {
		t.Errorf("MODEL_DEFAULT = %+v, want non-secret literal gpt-4o", got)
	}
}

func TestResolve_MessagingHardcodedKnobs(t *testing.T) {
	// Messaging container's hardcoded env knobs (GRPC_*, STORAGE_TYPE,
	// DEPLOYMENT_MODE; SLACK_ENABLED / WEB_* conditionals) must appear
	// regardless of whether the user has any vars.
	ds := &AstroDeploymentSpec{
		Source: DeploymentSource{Name: "a", Build: "b"},
		Agent:  DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Interfaces: &DeploymentInterfaces{
			Adapters: []string{"slack", "web"},
			Endpoints: map[string]Endpoint{
				"grpc": {Port: 9090},
				"http": {Port: 8090},
			},
		},
	}

	rs, err := Resolve(ds, ResolveOptions{Namespace: "ns", AuthToken: "jwt-token"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	want := map[string]string{
		"GRPC_ENABLED":      "true",
		"GRPC_LISTEN_ADDR":  ":9090",
		"STORAGE_TYPE":      "memory",
		"DEPLOYMENT_MODE":   "all",
		"SLACK_ENABLED":     "true",
		"WEB_ENABLED":       "true",
		"WEB_LISTEN_ADDR":   ":8090",
		"ASTRO_AUTHZ_TOKEN": "jwt-token",
	}
	for k, wantVal := range want {
		got := findResolution(rs, RoleMessaging, k)
		if got == nil {
			t.Errorf("messaging missing %s", k)
			continue
		}
		if got.Value != wantVal {
			t.Errorf("%s: got %q, want %q", k, got.Value, wantVal)
		}
	}

	// ASTRO_AUTHZ_TOKEN should also be on the agent (same value, both
	// containers need it).
	if got := findResolution(rs, RoleAgent, "ASTRO_AUTHZ_TOKEN"); got == nil {
		t.Error("agent missing ASTRO_AUTHZ_TOKEN")
	} else if got.Source != EnvSourceAuthToken || !got.IsSecret {
		t.Errorf("ASTRO_AUTHZ_TOKEN on agent: expected secret auth_token, got %+v", got)
	}
}

func TestResolve_KnowledgePostgresPerStoreRenaming(t *testing.T) {
	// Two postgres stores in one deployment: the "primary" keeps the
	// canonical names (POSTGRES_USER / _PASSWORD); the "users" store
	// gets renamed (POSTGRES_USERS_USER / _PASSWORD). Both rows live
	// on the agent role; the per-store renaming is handled by Resolve.
	ds := &AstroDeploymentSpec{
		Source: DeploymentSource{Name: "a", Build: "b"},
		Agent:  DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Knowledge: map[string]DeploymentKnowledge{
			"postgres": {Provider: "postgres", Endpoints: map[string]Endpoint{"http": {Port: 5432}}},
			"users":    {Provider: "postgres", Endpoints: map[string]Endpoint{"http": {Port: 5432}}},
		},
	}
	opts := ResolveOptions{
		Namespace: "ns",
		BoundCredentials: map[string]string{
			"postgres.user":     "astro",
			"postgres.password": "p1",
			"postgres.database": "sasbot",
			"users.user":        "astro",
			"users.password":    "p2",
			"users.database":    "sasbot",
		},
	}

	rs, err := Resolve(ds, opts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// On the agent: POSTGRES_USER (primary), POSTGRES_USERS_USER (renamed).
	for _, name := range []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_USERS_USER", "POSTGRES_USERS_PASSWORD"} {
		got := findResolution(rs, RoleAgent, name)
		if got == nil {
			t.Errorf("agent missing %s", name)
			continue
		}
		if !got.IsSecret || got.Source != EnvSourceKnowledgeCred {
			t.Errorf("%s on agent: expected secret knowledge_cred, got %+v", name, got)
		}
	}

	// Each knowledge container reads the literal upstream key names —
	// no per-store renaming on the container itself.
	for _, role := range []Role{KnowledgeRole("postgres"), KnowledgeRole("users")} {
		for _, name := range []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"} {
			if got := findResolution(rs, role, name); got == nil {
				t.Errorf("%s missing %s", role, name)
			}
		}
		// Renamed name must NOT appear on the container itself.
		if got := findResolution(rs, role, "POSTGRES_USERS_USER"); got != nil {
			t.Errorf("%s should not have POSTGRES_USERS_USER (renamed name belongs only on agent); got %+v", role, got)
		}
	}
}

func TestResolve_ExternalKnowledgeStoreInjectsAllVars(t *testing.T) {
	// An external (bound) knowledge store is managed outside this deployment:
	// we run no container for it, but the agent still needs the COMPLETE set
	// of connection coords + credentials for its provider. The host comes from
	// the binding (BoundKnowledge), credentials from the resolved store secret
	// (BoundCredentials).
	//
	// This pins the full keyset per provider — every expected var present with
	// the right secret/source classification, and NO unexpected provider vars.
	// postgres has no URLScheme (no *_URL); redis does (REDIS_URL must appear).
	type wantVar struct {
		value  string
		secret bool
		source EnvSource
	}
	cases := []struct {
		name     string
		provider string
		host     string
		creds    map[string]string // attr → value (as the deployer emits)
		want     map[string]wantVar
	}{
		{
			name:     "postgres (no URL scheme)",
			provider: "postgres",
			host:     "vpce-0abc.vpce-svc-0def.us-east-1.vpce.amazonaws.com", // PrivateLink endpoint DNS
			creds: map[string]string{
				"user":     "astro",
				"password": "secret123",
				"database": "mydb",
			},
			want: map[string]wantVar{
				"POSTGRES_HOST":     {"vpce-0abc.vpce-svc-0def.us-east-1.vpce.amazonaws.com", false, EnvSourceServiceURL},
				"POSTGRES_PORT":     {"5432", false, EnvSourceServiceURL},
				"POSTGRES_USER":     {"astro", true, EnvSourceKnowledgeCred},
				"POSTGRES_PASSWORD": {"secret123", true, EnvSourceKnowledgeCred},
				"POSTGRES_DB":       {"mydb", true, EnvSourceKnowledgeCred},
			},
		},
		{
			name:     "redis (URL scheme + single cred)",
			provider: "redis",
			host:     "cache.internal",
			creds: map[string]string{
				"password": "rpw",
			},
			want: map[string]wantVar{
				"REDIS_HOST":     {"cache.internal", false, EnvSourceServiceURL},
				"REDIS_PORT":     {"6379", false, EnvSourceServiceURL},
				"REDIS_URL":      {"redis://cache.internal:6379", false, EnvSourceServiceURL},
				"REDIS_PASSWORD": {"rpw", true, EnvSourceKnowledgeCred},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prefixed := map[string]string{}
			for attr, v := range tc.creds {
				prefixed["db."+attr] = v
			}
			ds := &AstroDeploymentSpec{
				Source: DeploymentSource{Name: "a", Build: "b"},
				Agent:  DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
				Knowledge: map[string]DeploymentKnowledge{
					"db": {Provider: tc.provider, Binding: "arn:knowledge:acct:shared"},
				},
			}
			rs, err := Resolve(ds, ResolveOptions{
				Namespace:        "ns",
				BoundKnowledge:   map[string]BoundKnowledgeInfo{"db": {Host: tc.host, Provider: tc.provider}},
				BoundCredentials: prefixed,
			})
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}

			// Every expected var is present with the right value/classification.
			prefix := strings.ToUpper(tc.provider) + "_"
			for name, w := range tc.want {
				got := findResolution(rs, RoleAgent, name)
				if got == nil {
					t.Errorf("agent missing %s", name)
					continue
				}
				if got.Value != w.value {
					t.Errorf("%s value: got %q, want %q", name, got.Value, w.value)
				}
				if got.IsSecret != w.secret {
					t.Errorf("%s IsSecret: got %v, want %v", name, got.IsSecret, w.secret)
				}
				if got.Source != w.source {
					t.Errorf("%s source: got %q, want %q", name, got.Source, w.source)
				}
			}

			// No EXTRA provider vars leaked onto the agent (catches a stray
			// *_URL for postgres, a misnamed cred, etc.).
			for _, r := range rs {
				if r.Role != RoleAgent || !strings.HasPrefix(r.EnvName, prefix) {
					continue
				}
				if _, ok := tc.want[r.EnvName]; !ok {
					t.Errorf("unexpected agent var %s=%q (source %s)", r.EnvName, r.Value, r.Source)
				}
			}

			// Bound store is not deployed by us — no knowledge-container rows.
			for _, r := range rs {
				if r.Role == KnowledgeRole("db") {
					t.Errorf("bound store should have no knowledge:db rows; got %+v", r)
				}
			}
		})
	}
}

func TestResolve_NoDuplicateRowsForSameEnvName(t *testing.T) {
	// Schema-level: (deployment, role, env_name) is unique. Resolve
	// must never emit two entries with the same (Role, EnvName).
	// This is the structural guarantee the dedupe fix codifies.
	ds := &AstroDeploymentSpec{
		Source: DeploymentSource{Name: "a", Build: "b"},
		Agent:  DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Variables: map[string]Variable{
			"DB_URL": {
				Secret: true, Value: "postgres://x",
				Targets: []string{"agent", "ingestion"},
			},
		},
		Ingestion: map[string]DeploymentIngestion{
			"reporter": {Image: "x", Endpoints: httpEndpoints(8080)},
		},
		Knowledge: map[string]DeploymentKnowledge{
			"postgres": {Provider: "postgres"},
		},
	}
	opts := ResolveOptions{
		Namespace: "ns",
		AuthToken: "tok",
		BoundCredentials: map[string]string{
			"postgres.user":     "astro",
			"postgres.password": "p1",
			"postgres.database": "sasbot",
		},
	}

	rs, err := Resolve(ds, opts)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	seen := map[string]bool{}
	for _, r := range rs {
		key := string(r.Role) + "|" + r.EnvName
		if seen[key] {
			t.Errorf("duplicate row %s", key)
		}
		seen[key] = true
	}
}

// P5: a variable with a named ingestion target appears only in that job.
func TestResolve_NamedIngestionTargetScopedToThatJobOnly(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Source: DeploymentSource{Name: "a", Build: "b"},
		Agent:  DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Variables: map[string]Variable{
			"NIGHTLY_VAR": {Value: "v", Targets: []string{"ingestion.nightly"}},
		},
		Ingestion: map[string]DeploymentIngestion{
			"nightly": {Image: "x"},
			"hook":    {Image: "x"},
		},
	}

	rs, err := Resolve(ds, ResolveOptions{Namespace: "ns"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	for _, r := range rs {
		if r.EnvName != "NIGHTLY_VAR" {
			continue
		}
		if r.Role != IngestionRole("nightly") {
			t.Errorf("P5: NIGHTLY_VAR on unexpected role %q", r.Role)
		}
	}
	if findResolution(rs, IngestionRole("nightly"), "NIGHTLY_VAR") == nil {
		t.Error("P5: NIGHTLY_VAR missing from ingestion:nightly")
	}
	if findResolution(rs, IngestionRole("hook"), "NIGHTLY_VAR") != nil {
		t.Error("P5: NIGHTLY_VAR must not appear in ingestion:hook")
	}
	if findResolution(rs, RoleAgent, "NIGHTLY_VAR") != nil {
		t.Error("P5: NIGHTLY_VAR must not appear in agent role")
	}
}

// P11: the collector container receives its required platform env vars.
func TestResolve_CollectorReceivesPlatformEnvVars(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Source:        DeploymentSource{Name: "my-agent", Build: "build1"},
		Agent:         DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Observability: DeploymentObservability{Enabled: true, Image: "collector:latest", Port: 4318},
	}

	rs, err := Resolve(ds, ResolveOptions{Namespace: "ns", DeploymentID: "dep-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	wantKeys := []string{"ASTRO_AGENT_NAME", "ASTRO_AGENT_VERSION"}
	for _, key := range wantKeys {
		if findResolution(rs, RoleCollector, key) == nil {
			t.Errorf("P11: collector missing %s", key)
		}
	}
}

// P12: a knowledge container receives the env vars specific to its store.
func TestResolve_KnowledgeContainerReceivesStoreEnvVars(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Source: DeploymentSource{Name: "a", Build: "b"},
		Agent:  DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Knowledge: map[string]DeploymentKnowledge{
			"db": {Image: "pgvector:latest", Provider: "postgres", Endpoints: httpEndpoints(5432), Replicas: 1},
		},
	}

	// BoundCredentials simulates what the K8s applier populates after creating
	// the credential secret for the knowledge store.
	rs, err := Resolve(ds, ResolveOptions{
		Namespace: "ns",
		BoundCredentials: map[string]string{
			"db.user":     "astro",
			"db.password": "secret123",
			"db.database": "mydb",
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	knowledgeRole := KnowledgeRole("db")
	if findResolution(rs, knowledgeRole, "POSTGRES_USER") == nil {
		t.Errorf("P12: POSTGRES_USER missing from knowledge:db role")
	}
	if findResolution(rs, knowledgeRole, "POSTGRES_PASSWORD") == nil {
		t.Errorf("P12: POSTGRES_PASSWORD missing from knowledge:db role")
	}
	if findResolution(rs, knowledgeRole, "POSTGRES_DB") == nil {
		t.Errorf("P12: POSTGRES_DB missing from knowledge:db role")
	}
}

// P13: an ingestion container receives the env vars scoped to that job.
func TestResolve_IngestionContainerReceivesItsEnvVars(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Source: DeploymentSource{Name: "a", Build: "b"},
		Agent:  DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Variables: map[string]Variable{
			"SYNC_CONFIG": {Value: "v", Targets: []string{"ingestion.sync"}},
		},
		Ingestion: map[string]DeploymentIngestion{
			"sync": {Image: "sync:latest"},
		},
	}

	rs, err := Resolve(ds, ResolveOptions{Namespace: "ns"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	ingRole := IngestionRole("sync")
	if findResolution(rs, ingRole, "SYNC_CONFIG") == nil {
		t.Errorf("P13: SYNC_CONFIG missing from ingestion:sync role")
	}
	if findResolution(rs, RoleAgent, "SYNC_CONFIG") != nil {
		t.Error("P13: SYNC_CONFIG must not appear in agent role")
	}
}

func TestResolve_DBURLFansOutToAgentAndEachIngestion(t *testing.T) {
	// A user variable with Targets=["agent","ingestion"] must produce
	// one row for the agent and one for each declared ingestion role.
	ds := &AstroDeploymentSpec{
		Source: DeploymentSource{Name: "a", Build: "b"},
		Agent:  DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Variables: map[string]Variable{
			"DB_URL": {
				Secret: false, Value: "postgres://x",
				Targets: []string{"agent", "ingestion"},
			},
		},
		Ingestion: map[string]DeploymentIngestion{
			"reporter":  {Image: "x", Endpoints: httpEndpoints(8080)},
			"backfill":  {Image: "x", Endpoints: httpEndpoints(8080)},
			"unrelated": {Image: "x", Endpoints: httpEndpoints(8080)},
		},
	}

	rs, err := Resolve(ds, ResolveOptions{Namespace: "ns"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	wantRoles := map[Role]bool{
		RoleAgent:                  true,
		IngestionRole("reporter"):  true,
		IngestionRole("backfill"):  true,
		IngestionRole("unrelated"): true,
	}
	for _, r := range rs {
		if r.EnvName != "DB_URL" {
			continue
		}
		if !wantRoles[r.Role] {
			t.Errorf("DB_URL on unexpected role %s", r.Role)
			continue
		}
		delete(wantRoles, r.Role)
	}
	for role := range wantRoles {
		t.Errorf("DB_URL missing on role %s", role)
	}
}
