package metronome

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Metronome-Industries/metronome-go/v3/shared"
)

// Without include_balance, every credit and commit comes back with Balance
// zero (Metronome's own behavior, also why CustomerSpend sets it). A client
// reading that as "what's left" would show every grant as fully spent. These
// list endpoints take their params as a JSON body, not a query string.
func TestBalances_RequestsIncludeBalanceForCreditsAndCommits(t *testing.T) {
	var sawCreditFlag, sawCommitFlag bool

	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			IncludeBalance bool `json:"include_balance"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "customerCredits"):
			sawCreditFlag = body.IncludeBalance
			_, _ = w.Write([]byte(oneCredit))
		case strings.Contains(r.URL.Path, "customerCommits"):
			sawCommitFlag = body.IncludeBalance
			_, _ = w.Write([]byte(`{"data":[{"id":"commit_1","type":"PREPAID","balance":500}],"next_page":null}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	result, err := p.Balances(context.Background(), "cust_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawCreditFlag {
		t.Error("credits list request did not set include_balance=true")
	}
	if !sawCommitFlag {
		t.Error("commits list request did not set include_balance=true")
	}

	out, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	credits, ok := out["credits"].([]shared.Credit)
	if !ok || len(credits) != 1 {
		t.Fatalf("credits = %v, want one credit", out["credits"])
	}
	// oneCredit's balance is 250; this is what a request that dropped the
	// flag would silently zero out.
	if credits[0].Balance != 250 {
		t.Errorf("credits[0].Balance = %v, want 250", credits[0].Balance)
	}

	commits, ok := out["commits"].([]shared.Commit)
	if !ok || len(commits) != 1 {
		t.Fatalf("commits = %v, want one commit", out["commits"])
	}
	if commits[0].Balance != 500 {
		t.Errorf("commits[0].Balance = %v, want 500", commits[0].Balance)
	}
}
