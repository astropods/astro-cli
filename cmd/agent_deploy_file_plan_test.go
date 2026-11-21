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
	assert.Equal(t, deployPlanRow{Label: "Build", Before: "2f1c4341", After: "2f1c4341"}, plan.Build)
	assert.Empty(t, plan.Latest, "the kept build is already the latest")
	assert.Equal(t, []deployPlanRow{
		{Label: "Cluster", Before: "use1", After: "euw1"},
		{Label: "Model", Key: "default", Before: "claude-sonnet-4-6", After: "claude-opus-4-8"},
		{Label: "Interfaces", Before: "web, mesh", After: "web"},
		{Label: "Public", Before: "web off, custom off", After: "web on, custom off"},
		{Label: "Variables", After: "API_KEY (ref API_KEY), LOG_LEVEL", NotCompared: true},
		{Label: "Schedule", Key: "nightly", Before: "0 3 * * *", After: "0 3 * * *"},
		{Label: "Knowledge", Key: "docs", Before: "kb-docs", After: "kb-docs-v2"},
	}, plan.Rows)
	assert.Equal(t, []deployPlanResource{{
		Name:   "agent",
		Before: &deployPlanSizing{Mode: "auto", CPU: "25m–1", Memory: "256Mi–4Gi"},
		After:  deployPlanSizing{Mode: "fixed", CPU: "1", Memory: "2Gi"},
	}}, plan.Resources)
	assert.Empty(t, plan.Kept, "the file sets every field")
}

func TestBuildDeployPlanListsWhatTheFileLeavesOut(t *testing.T) {
	stored := planTemplate(t, storedSasbot)

	plan := buildDeployPlan(planEntry(t, "    branch: main\n"), &stored, stored, false, "2f1c4341")

	assert.Empty(t, plan.Rows)
	assert.Empty(t, plan.Resources)
	assert.Equal(t, []string{"cluster", "models", "interfaces", "public", "variables", "schedules", "knowledge bindings", "resources"}, plan.Kept)
}

func TestBuildDeployPlanReportsANewerBuild(t *testing.T) {
	stored := planTemplate(t, storedSasbot)

	plan := buildDeployPlan(planEntry(t, "    branch: main\n"), &stored, stored, false, "ba7eeeff")

	assert.Equal(t, "ba7eeeff", plan.Latest, "a redeploy keeps its build, so the plan must say a newer one exists")
}

func plainPlan(plan deployPlan) string {
	return ansiRE.ReplaceAllString(msgDeployFilePlan(plan), "")
}

func TestMsgDeployFilePlanLayout(t *testing.T) {
	kept := deployPlanRow{Label: "Build", Before: "2f1c4341", After: "2f1c4341"}
	auto := deployPlanSizing{Mode: "auto", CPU: "25m–1", Memory: "256Mi–4Gi"}
	cases := []struct {
		name string
		plan deployPlan
		want string
	}{
		{
			name: "a new agent with a high availability knowledge entry",
			plan: deployPlan{New: true, Build: deployPlanRow{Label: "Build", After: "e67f35a5"},
				Rows: []deployPlanRow{
					{Label: "Model", Key: "default", After: "claude-sonnet-4-6"},
					{Label: "Interfaces", After: "web, mesh"},
				},
				Resources: []deployPlanResource{
					{Name: "agent", After: auto},
					{Name: "journal", After: deployPlanSizing{Mode: "fixed", CPU: "500m", Memory: "1Gi", HA: "on, 3 instances"}},
				},
				Kept: []string{"cluster", "variables"}},
			want: "    Build        e67f35a5  latest registered\n" +
				"    Model        default  claude-sonnet-4-6\n" +
				"    Interfaces   web, mesh\n" +
				"\n" +
				"    Resources    sizing   cpu     memory      high availability\n" +
				"    agent        auto     25m–1   256Mi–4Gi   –\n" +
				"    journal      fixed    500m    1Gi         on, 3 instances\n" +
				"\n" +
				"    Defaults: cluster, variables\n",
		},
		{
			name: "a redeploy shows what changes and folds the rest into Kept",
			plan: deployPlan{Build: kept, Latest: "ba7eeeff",
				Rows: []deployPlanRow{
					{Label: "Model", Key: "default", Before: "claude-sonnet-4-6", After: "claude-opus-4-8"},
					{Label: "Interfaces", Before: "web", After: "web"},
					{Label: "Variables", After: "LOG_LEVEL", NotCompared: true},
				},
				Resources: []deployPlanResource{{Name: "agent", Before: &auto, After: deployPlanSizing{Mode: "fixed", CPU: "1", Memory: "2Gi"}}},
				Kept:      []string{"cluster"}},
			want: "    Build       2f1c4341  kept, ba7eeeff is newer: use --build ba7eeeff\n" +
				"    Model       default  claude-sonnet-4-6 → claude-opus-4-8\n" +
				"    Variables   LOG_LEVEL\n" +
				"\n" +
				"    Resources   sizing         cpu         memory\n" +
				"    agent       auto → fixed   25m–1 → 1   256Mi–4Gi → 2Gi\n" +
				"\n" +
				"    Kept: cluster, interfaces\n",
		},
		{
			name: "a redeploy that changes nothing keeps the build and every field",
			plan: deployPlan{Build: kept, Resources: []deployPlanResource{{Name: "agent", Before: &auto, After: auto}}},
			want: "    Build       2f1c4341  kept, the latest build\n" +
				"\n" +
				"    Kept: agent sizing\n",
		},
		{
			name: "a pinned build",
			plan: deployPlan{Pinned: true, Build: deployPlanRow{Label: "Build", Before: "2f1c4341", After: "ba7eeeff"}},
			want: "    Build       2f1c4341 → ba7eeeff\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, plainPlan(tc.plan))
		})
	}
}

func TestMsgDeployFilePlanWeights(t *testing.T) {
	auto := deployPlanSizing{Mode: "auto"}
	out := msgDeployFilePlan(deployPlan{Build: deployPlanRow{Label: "Build", Before: "2f1c4341", After: "ba7eeeff"},
		Rows:      []deployPlanRow{{Label: "Model", Key: "default", Before: "claude-sonnet-4-6", After: "claude-opus-4-8"}},
		Resources: []deployPlanResource{{Name: "journal", Before: &auto, After: deployPlanSizing{Mode: "fixed", HA: "on"}}},
		Kept:      []string{"cluster"}})

	assert.Contains(t, out, colorDim+"claude-sonnet-4-6 →"+colorReset+" "+colorBold+"claude-opus-4-8"+colorReset, "the old value is faint and the new one bold")
	assert.Contains(t, out, "    Model       default  ", "labels and keys stay regular weight")
	assert.Contains(t, out, colorBold+"Resources"+colorReset, "the table heading is bold")
	assert.Contains(t, out, colorDim+"sizing"+colorReset, "column headings are faint")
	assert.Contains(t, out, colorBold+"on"+colorReset, "high availability is a value, so it is bold")
	assert.Contains(t, out, colorDim+"Kept: cluster"+colorReset, "what the deploy leaves alone is faint")
}

func TestDeployFileDryRunPrintsThePlan(t *testing.T) {
	setupEnvTest(t)
	setDeployFlags(t, "file", writeDeployFileProject(t, "spec: deploy/v1\nenvironments: {main: {}, qa: {}}\n"), "dry-run", "true")

	out, err := runWithOutput(t, blueprintDeployCmd, runBlueprintDeploy)

	require.NoError(t, err)
	assert.Contains(t, out, "    Kept: cluster, models, interfaces, public, variables, schedules, knowledge bindings, resources\n",
		"main holds an agent, so its plan lists what the redeploy keeps")
	assert.Contains(t, out, "    Defaults: ", "qa has no agent, so its plan lists what a new agent gets by default")
	assert.Contains(t, out, msgDeployFileNothingDeployed(2))
}

func TestTemplateSizingShowsHighAvailability(t *testing.T) {
	on, off := true, false
	fixed := &deployTemplateSize{CPU: "500m", Memory: "1Gi"}
	cases := []struct {
		name string
		ha   *bool
		want deployPlanSizing
	}{
		{"on", &on, deployPlanSizing{Mode: "fixed", CPU: "500m", Memory: "1Gi", HA: "on, 3 instances"}},
		{"off", &off, deployPlanSizing{Mode: "fixed", CPU: "500m", Memory: "1Gi"}},
		{"not echoed", nil, deployPlanSizing{Mode: "fixed", CPU: "500m", Memory: "1Gi"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := planTemplate(t, `{"template": {"knowledge": {"journal": {"replicas": 3}}}}`)
			tmpl.Provisioning.Knowledge = map[string]deployTemplateComponent{"journal": {Compute: fixed, HighAvailability: tc.ha}}
			assert.Equal(t, tc.want, templateSizing(tmpl, "knowledge.journal"), "the dry run must show whether the entry runs as a database cluster")
		})
	}
}

func TestResourceNameUsesTheGroupOnlyWhenNamesCollide(t *testing.T) {
	keys := []string{"agent", "models.default", "knowledge.default", "knowledge.journal"}
	assert.Equal(t, "agent", resourceName("agent", keys))
	assert.Equal(t, "journal", resourceName("knowledge.journal", keys))
	assert.Equal(t, "models.default", resourceName("models.default", keys), "two entries named default need their group to tell them apart")
}
