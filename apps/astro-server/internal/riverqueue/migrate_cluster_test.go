package riverqueue

import "testing"

func TestMigrateDeploymentClusterArgs_Kind(t *testing.T) {
	if got := (MigrateDeploymentClusterArgs{}).Kind(); got != "deployment.migrate_cluster" {
		t.Fatalf("Kind() = %q, want deployment.migrate_cluster", got)
	}
}

func TestMigrateDeploymentClusterArgs_InsertOpts(t *testing.T) {
	opts := (MigrateDeploymentClusterArgs{}).InsertOpts()
	if opts.Queue != queueDeploy {
		t.Fatalf("queue = %q, want %q", opts.Queue, queueDeploy)
	}
	if opts.MaxAttempts != 3 {
		t.Fatalf("MaxAttempts = %d, want 3", opts.MaxAttempts)
	}
	if !opts.UniqueOpts.ByArgs {
		t.Fatal("expected unique by args")
	}
}
