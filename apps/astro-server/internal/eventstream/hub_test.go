package eventstream

import (
	"sync"
	"testing"
)

func TestHubDeliversOnlyToTheAccountsSubscribers(t *testing.T) {
	h := New()
	mine, cancelMine := h.Subscribe("acct-1")
	defer cancelMine()
	theirs, cancelTheirs := h.Subscribe("acct-2")
	defer cancelTheirs()

	h.Publish(Event{ID: "1", AccountID: "acct-1", Type: "agent.build", Agent: "reviewer"})

	got := <-mine
	if got.Agent != "reviewer" {
		t.Fatalf("agent = %q, want reviewer", got.Agent)
	}
	select {
	case e := <-theirs:
		t.Fatalf("acct-2 received an acct-1 event: %+v", e)
	default:
	}
}

func TestHubPublishDoesNotBlockOnAFullSubscriber(t *testing.T) {
	h := New()
	_, cancel := h.Subscribe("acct-1")
	defer cancel()

	// One more than the buffer: without the non-blocking send the last Publish
	// would deadlock the caller, which is the shared pgnotify listener.
	done := make(chan struct{})
	go func() {
		for i := 0; i < buffer+1; i++ {
			h.Publish(Event{ID: "e", AccountID: "acct-1"})
		}
		close(done)
	}()

	<-done
	if h.Dropped() != 1 {
		t.Fatalf("Dropped() = %d, want 1", h.Dropped())
	}
}

func TestHubCancelIsIdempotentAndUnregisters(t *testing.T) {
	h := New()
	_, cancel := h.Subscribe("acct-1")
	cancel()
	cancel()

	if n := h.Subscribers("acct-1"); n != 0 {
		t.Fatalf("Subscribers = %d, want 0", n)
	}
	// Publishing to a cancelled subscriber must not panic on a closed channel.
	h.Publish(Event{ID: "1", AccountID: "acct-1"})
}

func TestHubConcurrentSubscribeCancelPublish(t *testing.T) {
	h := New()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, cancel := h.Subscribe("acct-1")
			cancel()
		}()
		go func() {
			defer wg.Done()
			h.Publish(Event{ID: "1", AccountID: "acct-1"})
		}()
	}
	wg.Wait()
}
