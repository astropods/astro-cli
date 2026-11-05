package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	deployspec "github.com/astropods/astro-spec/deploy"
)

func planTemplate(t *testing.T, raw string) deployTemplateResponse {
	t.Helper()
	var resp deployTemplateResponse
	require.NoError(t, json.Unmarshal([]byte(raw), &resp))
	return resp
}

func planEntry(t *testing.T, yaml string) deployspec.Environment {
	t.Helper()
	doc, err := deployspec.Parse("spec: deploy/v1\nenvironments:\n  main:\n" + yaml)
	require.NoError(t, err)
	return doc.Environments["main"]
}

const storedSasbot = `{
	"template": {"source": {"build": "2f1c4341"}, "target": {"cluster_id": "use1"}},
	"models": [{"name": "default", "selected": "claude-sonnet-4-6"}],
	"interfaces": {"adapters": ["web", "mesh"], "auth": {"web": {"public": false}}},
	"schedules": {"nightly": "0 3 * * *"},
	"bindings": {"knowledge": {"docs": {"name": "kb-docs", "arn": "arn:kb:docs"}}},
	"provisioning": {"agent": {"sizing": {"mode": "auto", "min": {"cpu": "25m", "memory": "256Mi"}, "max": {"cpu": "1", "memory": "4Gi"}}}}
}`

func TestBuildDeployPlanComparesEachFieldTheFileSets(t *testing.T) {
	stored := planTemplate(t, storedSasbot)
	proposed := planTemplate(t, `{
		"template": {"source": {"build": "2f1c4341"}, "target": {"cluster_id": "euw1"}},
		"models": [{"name": "default", "selected": "claude-opus-4-8"}],
		"interfaces": {"adapters": ["web"], "auth": {"web": {"public": true}}},
		"schedules": {"nightly": "0 3 * * *"},
		"bindings": {"knowledge": {"docs": {"name": "kb-docs-v2"}}},
		"provisioning": {"agent": {"compute": {"cpu": "1", "memory": "2Gi"}, "sizing": {"mode": "fixed"}}}
	}`)
	entry := planEntry(t, `    cluster: euw1
    interfaces: [web]
    public: { web: true }
    models: { default: claude-opus-4-8 }
    schedules: { nightly: "0 3 * * *" }
    knowledge: { docs: kb-docs-v2 }
    variables: { LOG_LEVEL: info, API_KEY: { ref: API_KEY } }
    resources:
      agent: { sizing: fixed, cpu: "1", memory: 2Gi }
`)

	plan := buildDeployPlan(entry, &stored, proposed, false, "2f1c4341")

	assert.False(t, plan.New)
	assert.Equal(t, deployPlanRow{Label: "build", Before: "2f1c4341", After: "2f1c4341"}, plan.Build)
	assert.Empty(t, plan.Latest, "the kept build is already the latest")
	assert.Equal(t, []deployPlanRow{
		{Label: "cluster", Before: "use1", After: "euw1"},
		{Label: "model default", Before: "claude-sonnet-4-6", After: "claude-opus-4-8"},
		{Label: "interfaces", Before: "web, mesh", After: "web"},
		{Label: "public", Before: "web off, custom off", After: "web on, custom off"},
		{Label: "variables", After: "API_KEY (ref API_KEY), LOG_LEVEL", NotCompared: true},
		{Label: "schedule nightly", Before: "0 3 * * *", After: "0 3 * * *"},
		{Label: "knowledge docs", Before: "kb-docs", After: "kb-docs-v2"},
		{Label: "sizing agent", Before: "auto (cpu 25m–1, memory 256Mi–4Gi)", After: "fixed (cpu 1, memory 2Gi)"},
	}, plan.Rows)
	assert.Empty(t, plan.Kept, "the file sets every field")
}

func TestBuildDeployPlanListsWhatTheFileLeavesOut(t *testing.T) {
	stored := planTemplate(t, storedSasbot)

	plan := buildDeployPlan(planEntry(t, "    branch: main\n"), &stored, stored, false, "2f1c4341")

	assert.Empty(t, plan.Rows)
	assert.Equal(t, []string{"cluster", "models", "interfaces", "public", "variables", "schedules", "knowledge", "resources"}, plan.Kept)
}

func TestBuildDeployPlanReportsANewerBuild(t *testing.T) {
	stored := planTemplate(t, storedSasbot)

	plan := buildDeployPlan(planEntry(t, "    branch: main\n"), &stored, stored, false, "ba7eeeff")

	assert.Equal(t, "ba7eeeff", plan.Latest, "a redeploy keeps its build, so the plan must say a newer one exists")
}

func TestMsgDeployFilePlan(t *testing.T) {
	kept := deployPlanRow{Label: "build", Before: "2f1c4341", After: "2f1c4341"}
	cases := []struct {
		name string
		plan deployPlan
		want string
	}{
		{
			name: "a redeploy that changes a field and keeps the rest",
			plan: deployPlan{Build: kept, Rows: []deployPlanRow{
				{Label: "model default", Before: "claude-sonnet-4-6", After: "claude-opus-4-8"},
				{Label: "interfaces", Before: "web", After: "web"},
				{Label: "variables", After: "LOG_LEVEL", NotCompared: true},
			}, Kept: []string{"cluster", "schedules"}},
			want: "    build              2f1c4341 (kept, the latest build)\n" +
				"    model default      claude-sonnet-4-6 → claude-opus-4-8\n" +
				"    interfaces         web (unchanged)\n" +
				"    variables          set LOG_LEVEL\n" +
				"    not in file        cluster, schedules (kept)\n",
		},
		{
			name: "a redeploy with a newer build available",
			plan: deployPlan{Build: kept, Latest: "ba7eeeff"},
			want: "    build              2f1c4341 (kept; ba7eeeff is newer, deploy it with --build ba7eeeff)\n",
		},
		{
			name: "a pinned build",
			plan: deployPlan{Pinned: true, Build: deployPlanRow{Label: "build", Before: "2f1c4341", After: "ba7eeeff"}},
			want: "    build              2f1c4341 → ba7eeeff\n",
		},
		{
			name: "a new agent",
			plan: deployPlan{New: true, Build: deployPlanRow{Label: "build", After: "ba7eeeff"},
				Rows: []deployPlanRow{{Label: "interfaces", After: "web"}}, Kept: []string{"models"}},
			want: "    build              ba7eeeff (latest registered)\n" +
				"    interfaces         web\n" +
				"    not in file        models (defaults)\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, msgDeployFilePlan(tc.plan))
		})
	}
}

func TestDeployFileDryRunPrintsThePlan(t *testing.T) {
	setupEnvTest(t)
	setDeployFlags(t, "file", writeDeployFileProject(t, "spec: deploy/v1\nenvironments: {main: {}, qa: {}}\n"), "dry-run", "true")

	out, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy)

	require.NoError(t, err)
	assert.Contains(t, out, "    not in file        cluster, models, interfaces, public, variables, schedules, knowledge, resources (kept)\n",
		"main holds an agent, so its plan lists what the redeploy keeps")
	assert.Contains(t, out, "(defaults)\n", "qa has no agent, so its plan lists what a new agent gets by default")
	assert.Contains(t, out, msgDeployFileNothingDeployed(2))
}
