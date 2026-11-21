package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func successPipeline(specPath string) *PushPipeline {
	p := NewPushPipeline(context.Background(), PushPipelineConfig{
		Account:   "astro-preview",
		AgentName: "credit-eater",
		SpecPath:  specPath,
	})
	p.tag = "09182293"
	p.visibility = VisibilityPublic
	return p
}

func TestMsgPushSuccessLayout(t *testing.T) {
	out := ansiRE.ReplaceAllString(msgPushSuccess("sasbot", "e67f35a5", "private", "https://astropod.ai/saswatds/sasbot", "ast deploy -f astropods.deploy.yml --build e67f35a5"), "")

	assert.Equal(t, "\n"+
		"✓ Pushed sasbot\n"+
		"\n"+
		"    Build        e67f35a5\n"+
		"    Visibility   private\n"+
		"    View         https://astropod.ai/saswatds/sasbot\n"+
		"\n"+
		"    Deploy it:  ast deploy -f astropods.deploy.yml --build e67f35a5\n"+
		"\n", out)
}

func TestMsgPushSuccessWeights(t *testing.T) {
	out := msgPushSuccess("sasbot", "e67f35a5", "private", "https://astropod.ai/saswatds/sasbot", "ast deploy --build e67f35a5")

	assert.Contains(t, out, "Pushed "+colorBold+"sasbot"+colorReset, "the blueprint name is bold")
	assert.Contains(t, out, "    Build        "+colorBold+"e67f35a5"+colorReset, "labels are regular and the build ID, the value to copy, is bold")
	assert.Contains(t, out, colorDim+"Deploy it:"+colorReset+"  ast deploy --build e67f35a5", "the hint label is faint and the command plain, so it copies cleanly")
}

func TestMsgPushSuccessOmitsAnUnresolvedVisibility(t *testing.T) {
	out := ansiRE.ReplaceAllString(msgPushSuccess("sasbot", "e67f35a5", "", "https://x", "ast deploy --build e67f35a5"), "")

	assert.NotContains(t, out, "Visibility")
}

func TestPrintSuccessPrintsTheSummaryForThisPush(t *testing.T) {
	dir := t.TempDir()
	p := successPipeline(filepath.Join(dir, "astropods.yml"))

	out := captureStdout(t, p.PrintSuccess)

	assert.Equal(t, msgPushSuccess("credit-eater", "09182293", "public",
		buildinfo.DefaultServerURL+"/astro-preview/credit-eater",
		buildinfo.BinaryName+" deploy --build 09182293"), out)
}

func TestPushDeployCommand(t *testing.T) {
	cases := []struct {
		name       string
		deployFile bool
		want       string
	}{
		{"with a deploy file next to astropods.yml", true, buildinfo.BinaryName + " deploy -f astropods.deploy.yml --build 09182293"},
		{"without one", false, buildinfo.BinaryName + " deploy --build 09182293"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			if tc.deployFile {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "astropods.deploy.yml"), []byte("spec: deploy/v1\n"), 0o600))
			}

			assert.Equal(t, tc.want, successPipeline(filepath.Join(dir, "astropods.yml")).deployCommand(),
				"the hint must be a command that deploys exactly the build just pushed")
		})
	}
}
