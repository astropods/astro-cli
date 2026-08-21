package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

func deploymentOn(clusterID string) *deploymentstore.Deployment {
	return &deploymentstore.Deployment{ClusterID: &clusterID}
}

func TestResolveTemplateClusterID(t *testing.T) {
	clusterA := account.ClusterBinding{ClusterID: "cluster-a", Region: "region-a", IsDefault: true}
	clusterB := account.ClusterBinding{ClusterID: "cluster-b", Region: "region-b"}

	tests := []struct {
		name           string
		requested      string
		allowed        []account.ClusterBinding
		current        *deploymentstore.Deployment
		defaultCluster string
		want           string
		wantErr        error
	}{
		{
			name:    "no pick uses the default binding",
			allowed: []account.ClusterBinding{clusterA, clusterB},
			want:    "cluster-a",
		},
		{
			name:    "no pick, no bindings, no default configured leaves it empty",
			allowed: nil,
			want:    "",
		},
		{
			name:    "no pick and no flagged default takes the first binding",
			allowed: []account.ClusterBinding{clusterB},
			want:    "cluster-b",
		},
		{
			name:           "bindings without a default never resolve to the primary",
			allowed:        []account.ClusterBinding{clusterB},
			defaultCluster: "cluster-default",
			want:           "cluster-b",
		},
		{
			name:      "pick on the allowed list is honored",
			requested: "cluster-b",
			allowed:   []account.ClusterBinding{clusterA, clusterB},
			want:      "cluster-b",
		},
		{
			name:      "pick off the allowed list is rejected",
			requested: "cluster-c",
			allowed:   []account.ClusterBinding{clusterA},
			wantErr:   ErrClusterNotAllowed,
		},
		{
			name:    "redeploy keeps the deployment's current cluster",
			allowed: []account.ClusterBinding{clusterA, clusterB},
			current: deploymentOn("cluster-b"),
			want:    "cluster-b",
		},
		{
			name:    "redeploy keeps an unbound current cluster instead of relocating it",
			allowed: []account.ClusterBinding{clusterA},
			current: deploymentOn("cluster-c"),
			want:    "cluster-c",
		},
		{
			name:      "an explicit pick moves a deployment, for the migrate dispatch to carry out",
			requested: "cluster-a",
			allowed:   []account.ClusterBinding{clusterA, clusterB},
			current:   deploymentOn("cluster-b"),
			want:      "cluster-a",
		},
		{
			name:      "an explicit pick naming the current cluster is honored",
			requested: "cluster-b",
			allowed:   []account.ClusterBinding{clusterA, clusterB},
			current:   deploymentOn("cluster-b"),
			want:      "cluster-b",
		},
		{
			name:           "a redeploy on the primary stays there when the default sits elsewhere",
			allowed:        []account.ClusterBinding{clusterB},
			current:        deploymentOn(""),
			defaultCluster: "cluster-primary",
			want:           "cluster-primary",
		},
		{
			name:           "naming the primary is no move for a deployment already on it",
			requested:      "cluster-primary",
			allowed:        []account.ClusterBinding{{ClusterID: "cluster-primary", Region: "region-a", IsDefault: true}},
			current:        deploymentOn(""),
			defaultCluster: "cluster-primary",
			want:           "cluster-primary",
		},
		{
			name:           "the implicit default binding records the real id",
			allowed:        []account.ClusterBinding{{ClusterID: "cluster-default", Region: "region-a", IsDefault: true}},
			defaultCluster: "cluster-default",
			want:           "cluster-default",
		},
		{
			name:           "naming the default cluster outright records it too",
			requested:      "cluster-default",
			allowed:        []account.ClusterBinding{{ClusterID: "cluster-default", Region: "region-a", IsDefault: true}},
			defaultCluster: "cluster-default",
			want:           "cluster-default",
		},
		{
			name:           "no bindings still resolve to the default cluster",
			defaultCluster: "cluster-default",
			want:           "cluster-default",
		},
		{
			name:           "a non-default cluster keeps its id",
			requested:      "cluster-a",
			allowed:        []account.ClusterBinding{clusterA},
			defaultCluster: "cluster-default",
			want:           "cluster-a",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ds := &deployment.AstroDeploymentSpec{Spec: "deployment/v1"}
			err := resolveTemplateClusterID(ds, tc.requested, tc.allowed, tc.current, clusterid.New(tc.defaultCluster))
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ds.Target.ClusterID != tc.want {
				t.Fatalf("cluster_id = %q, want %q", ds.Target.ClusterID, tc.want)
			}
		})
	}
}

func TestResolveTemplateClusterIDSurvivesEnforcementWithoutADefault(t *testing.T) {
	allowed := []account.ClusterBinding{{ClusterID: "cluster-b", Region: "region-b"}}

	ds := &deployment.AstroDeploymentSpec{Spec: "deployment/v1"}
	if err := resolveTemplateClusterID(ds, "", allowed, nil, clusterid.New("cluster-primary")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := enforceAccountClusterPlacement(ds, allowed); err != nil {
		t.Fatalf("resolved cluster %q rejected at deploy time: %v", ds.Target.ClusterID, err)
	}
}

func TestResolveTemplateClusterIDRedeployOnTheBoundPrimary(t *testing.T) {
	allowed := []account.ClusterBinding{
		{ClusterID: "cluster-primary", Region: "region-a", IsDefault: true},
		{ClusterID: "cluster-b", Region: "region-b"},
	}

	ds := &deployment.AstroDeploymentSpec{Spec: "deployment/v1"}
	if err := resolveTemplateClusterID(ds, "", allowed, deploymentOn(""), clusterid.New("cluster-primary")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ds.Target.ClusterID != "cluster-primary" {
		t.Fatalf("cluster_id = %q, want cluster-primary", ds.Target.ClusterID)
	}
	if err := enforceAccountClusterPlacement(ds, allowed); err != nil {
		t.Fatalf("bound primary rejected at deploy time: %v", err)
	}
}

func TestEnforceAccountClusterPlacement(t *testing.T) {
	allowed := []account.ClusterBinding{{ClusterID: "cluster-a", Region: "region-a"}}

	ds := &deployment.AstroDeploymentSpec{Spec: "deployment/v1"}
	ds.Target.ClusterID = "cluster-a"
	if err := enforceAccountClusterPlacement(ds, allowed); err != nil {
		t.Fatalf("allowed cluster rejected: %v", err)
	}

	for _, target := range []string{"", "cluster-primary"} {
		ds.Target.ClusterID = target
		if err := enforceAccountClusterPlacement(ds, allowed); !errors.Is(err, ErrClusterNotAllowed) {
			t.Fatalf("target %q: err = %v, want ErrClusterNotAllowed", target, err)
		}
	}

	ds.Target.ClusterID = "cluster-b"
	if err := enforceAccountClusterPlacement(ds, allowed); !errors.Is(err, ErrClusterNotAllowed) {
		t.Fatalf("err = %v, want ErrClusterNotAllowed", err)
	}
}

func TestResolveUpdatePlacement(t *testing.T) {
	tests := []struct {
		name           string
		isUpdate       bool
		prior          string
		target         string
		defaultCluster string
		wantPersist    string
		wantMigrate    bool
	}{
		{
			name:        "first deploy never migrates",
			target:      "cluster-a",
			wantPersist: "cluster-a",
		},
		{
			name:        "redeploy to the same cluster",
			isUpdate:    true,
			prior:       "cluster-a",
			target:      "cluster-a",
			wantPersist: "cluster-a",
		},
		{
			name:        "moving to another cluster keeps the row on the old one",
			isUpdate:    true,
			prior:       "cluster-a",
			target:      "cluster-b",
			wantPersist: "cluster-a",
			wantMigrate: true,
		},
		{
			name:        "moving off the default cluster",
			isUpdate:    true,
			prior:       "",
			target:      "cluster-b",
			wantPersist: "",
			wantMigrate: true,
		},
		{
			name:        "moving onto the default cluster",
			isUpdate:    true,
			prior:       "cluster-a",
			target:      "",
			wantPersist: "cluster-a",
			wantMigrate: true,
		},
		{
			name:           "an older row on the default cluster does not migrate to itself",
			isUpdate:       true,
			prior:          "",
			target:         "cluster-default",
			defaultCluster: "cluster-default",
			wantPersist:    "cluster-default",
		},
		{
			name:           "an older row on the default cluster still migrates elsewhere",
			isUpdate:       true,
			prior:          "",
			target:         "cluster-a",
			defaultCluster: "cluster-default",
			wantPersist:    "",
			wantMigrate:    true,
		},
		{
			name:        `a cluster named "default" is no alias for the primary`,
			isUpdate:    true,
			prior:       "",
			target:      "default",
			wantPersist: "",
			wantMigrate: true,
		},
		{
			name:           "the primary named on the row is the same target as empty",
			isUpdate:       true,
			prior:          "cluster-default",
			target:         "",
			defaultCluster: "cluster-default",
			wantPersist:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			persist, migrate := resolveUpdatePlacement(tc.isUpdate, tc.prior, tc.target, clusterid.New(tc.defaultCluster))
			if persist != tc.wantPersist || migrate != tc.wantMigrate {
				t.Fatalf("got (%q, %v), want (%q, %v)", persist, migrate, tc.wantPersist, tc.wantMigrate)
			}
		})
	}
}

type recordingDeployQueue struct {
	deployJobs  [][2]string
	migrateJobs [][3]string
}

func (q *recordingDeployQueue) InsertDeployJob(_ context.Context, deploymentID, clusterID string) error {
	q.deployJobs = append(q.deployJobs, [2]string{deploymentID, clusterID})
	return nil
}
func (q *recordingDeployQueue) InsertUndeployJob(context.Context, string, string) error { return nil }
func (q *recordingDeployQueue) InsertWakeUpJob(context.Context, string, string) error   { return nil }

func (q *recordingDeployQueue) InsertMigrateDeploymentClusterJob(_ context.Context, deploymentID, target, source string) error {
	q.migrateJobs = append(q.migrateJobs, [3]string{deploymentID, target, source})
	return nil
}

func TestEnqueueDeployOrMigrate(t *testing.T) {
	in := deployEnqueue{
		DeploymentID:    "dep-1",
		ClusterID:       "cluster-a",
		TargetClusterID: "cluster-b",
		SourceClusterID: "cluster-a",
	}

	t.Run("a plain deploy enqueues a deploy job", func(t *testing.T) {
		q := &recordingDeployQueue{}
		if err := enqueueDeployOrMigrate(context.Background(), q, in); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		if len(q.deployJobs) != 1 || q.deployJobs[0] != [2]string{"dep-1", "cluster-a"} {
			t.Fatalf("deploy jobs = %v", q.deployJobs)
		}
		if len(q.migrateJobs) != 0 {
			t.Fatalf("a plain deploy must not migrate: %v", q.migrateJobs)
		}
	})

	t.Run("a move enqueues only the migrate job", func(t *testing.T) {
		q := &recordingDeployQueue{}
		moving := in
		moving.Migrating = true
		if err := enqueueDeployOrMigrate(context.Background(), q, moving); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		if len(q.migrateJobs) != 1 || q.migrateJobs[0] != [3]string{"dep-1", "cluster-b", "cluster-a"} {
			t.Fatalf("migrate jobs = %v", q.migrateJobs)
		}
		if len(q.deployJobs) != 0 {
			t.Fatalf("a move must not also enqueue a deploy: %v", q.deployJobs)
		}
	})
}

func TestClusterNotAvailableNamesTheAlternatives(t *testing.T) {
	allowed := []account.ClusterBinding{
		{ClusterID: "us-east-1", Region: "us-east-1"},
		{ClusterID: "eu-west-1", Region: "eu-west-1"},
	}

	ds := &deployment.AstroDeploymentSpec{Spec: "deployment/v1"}
	err := resolveTemplateClusterID(ds, "us-est-1", allowed, nil, clusterid.New("us-east-1"))
	if !errors.Is(err, ErrClusterNotAllowed) {
		t.Fatalf("err = %v, want it to wrap ErrClusterNotAllowed", err)
	}

	var notAvailable *ClusterNotAvailableError
	if !errors.As(err, &notAvailable) {
		t.Fatalf("err = %v, want a *ClusterNotAvailableError", err)
	}
	if notAvailable.Requested != "us-est-1" {
		t.Errorf("requested = %q, want the id the caller sent", notAvailable.Requested)
	}
	if len(notAvailable.Available) != 2 {
		t.Fatalf("available = %v, want both bindings", notAvailable.Available)
	}
	for _, want := range []string{"us-est-1", "us-east-1", "eu-west-1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q should mention %q", err.Error(), want)
		}
	}
}

func TestClusterNotAvailableWithNoBindings(t *testing.T) {
	err := errClusterNotAvailable("eu-west-1", nil)
	if !strings.Contains(err.Error(), "none") {
		t.Errorf("message %q should say there are no alternatives", err.Error())
	}
}
