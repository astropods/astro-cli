package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/evalpreset"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func evaluationSetRouter(t *testing.T) (*httptest.ResponseRecorder, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.MatchExpectationsInOrder(false)

	router := authenticatedDeploymentVisibilityRouter()
	router.GET("/agents/:account/:name/evaluation-set",
		GetAgentEvaluationSet(logger.New("error", "json"), account.NewAccountStore(db), agentindex.NewIndexWithDB(db), fakeEvalSetResolver{}))

	recorder := httptest.NewRecorder()
	serve := func() {
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/agents/acme/agent/evaluation-set", nil))
	}
	return recorder, mock, serve
}

func expectEvaluationSetAgent(mock sqlmock.Sqlmock, exists bool) {
	mock.ExpectQuery(`(?s)SELECT EXISTS.*FROM agents`).WithArgs("acct-1", "agent").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
}

func TestGetAgentEvaluationSet_OK(t *testing.T) {
	recorder, mock, serve := evaluationSetRouter(t)
	expectAccountByName(mock)
	expectAccountMembership(mock)
	expectEvaluationSetAgent(mock, true)

	serve()

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var resp EvaluationSetResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.EvaluationRef != evalpreset.RefDefaultSet {
		t.Errorf("evaluation_ref = %q, want %q", resp.EvaluationRef, evalpreset.RefDefaultSet)
	}

	want, err := evalpreset.ResolveSet(evalpreset.RefDefaultSet)
	if err != nil {
		t.Fatalf("resolve set: %v", err)
	}
	if len(resp.Evaluators) != len(want) {
		t.Fatalf("evaluators len = %d, want %d", len(resp.Evaluators), len(want))
	}
	for i, definition := range want {
		got := resp.Evaluators[i]
		if got.Key != definition.Key {
			t.Errorf("evaluator %d key = %q, want %q", i, got.Key, definition.Key)
		}
		if got.Label != definition.Label {
			t.Errorf("evaluator %d label = %q, want %q", i, got.Label, definition.Label)
		}
		if got.Type != string(definition.Type) {
			t.Errorf("evaluator %d type = %q, want %q", i, got.Type, definition.Type)
		}
		if got.Output.Type != definition.Output.Type {
			t.Errorf("evaluator %d output type = %q, want %q", i, got.Output.Type, definition.Output.Type)
		}
		if len(got.Output.Options) != len(definition.Output.Options) {
			t.Errorf("evaluator %d output options = %v, want %v", i, got.Output.Options, definition.Output.Options)
		}
	}
}

func TestGetAgentEvaluationSet_AccountNotFound(t *testing.T) {
	recorder, mock, serve := evaluationSetRouter(t)
	mock.ExpectQuery(`(?s)FROM accounts a.*WHERE a.name`).WithArgs("acme").
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns))

	serve()

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestGetAgentEvaluationSet_NotAMember(t *testing.T) {
	recorder, mock, serve := evaluationSetRouter(t)
	expectAccountByName(mock)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM account_members`).WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	serve()

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestGetAgentEvaluationSet_AgentNotFound(t *testing.T) {
	recorder, mock, serve := evaluationSetRouter(t)
	expectAccountByName(mock)
	expectAccountMembership(mock)
	expectEvaluationSetAgent(mock, false)

	serve()

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
