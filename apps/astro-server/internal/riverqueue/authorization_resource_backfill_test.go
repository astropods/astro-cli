package riverqueue

import (
	"testing"
	"time"
)

func TestAuthorizationResourceBackfillWorkerTimeout(t *testing.T) {
	t.Parallel()

	worker := &AuthorizationResourceBackfillWorker{}
	if got := worker.Timeout(nil); got != 30*time.Minute {
		t.Fatalf("Timeout() = %v, want 30m", got)
	}
}
