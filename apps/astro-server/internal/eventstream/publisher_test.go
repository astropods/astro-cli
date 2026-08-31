package eventstream

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeLineage struct {
	accts []string
	err   error
}

func (f fakeLineage) ListAccountIDsWithLineageAgent(context.Context, string, string) ([]string, error) {
	return f.accts, f.err
}

func TestFanoutStampsEachRecipient(t *testing.T) {
	hub := New()
	a, cancelA := hub.Subscribe("acct-1")
	defer cancelA()
	b, cancelB := hub.Subscribe("acct-2")
	defer cancelB()

	// One stored row reaches many accounts. This is what keeps a popular
	// blueprint from writing a row per downstream consumer.
	Fanout(hub, Notification{
		Event:      Event{ID: "7", Type: "agent.build", Agent: "reviewer", Status: "registered"},
		Recipients: []string{"acct-1", "acct-2"},
	})

	for name, ch := range map[string]<-chan Event{"acct-1": a, "acct-2": b} {
		got := <-ch
		if got.AccountID != name {
			t.Fatalf("%s got AccountID %q", name, got.AccountID)
		}
		if got.ID != "7" || got.Agent != "reviewer" {
			t.Fatalf("%s got %+v", name, got)
		}
	}
}

func TestFanoutToNoRecipientsIsANoop(t *testing.T) {
	hub := New()
	ch, cancel := hub.Subscribe("acct-1")
	defer cancel()

	Fanout(hub, Notification{Event: Event{ID: "1"}, Recipients: nil})

	select {
	case e := <-ch:
		t.Fatalf("delivered with no recipients: %+v", e)
	default:
	}
}

func TestNotificationRoundTripsOverTheWire(t *testing.T) {
	// AccountID must survive JSON: the listener routes on the recipient list,
	// and each delivered event carries the account it was stamped for.
	in := Notification{
		Event:      Event{ID: "9", AccountID: "publisher", Type: "agent.build", Agent: "a", BuildID: "b1", Status: "building"},
		Recipients: []string{"acct-1"},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Notification
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Event.AccountID != "publisher" || out.Recipients[0] != "acct-1" {
		t.Fatalf("round trip lost routing data: %+v", out)
	}
	if out.Event.Status != "building" || out.Event.BuildID != "b1" {
		t.Fatalf("round trip lost build fields: %+v", out)
	}
}
