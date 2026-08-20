package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/gin-gonic/gin"
)

const (
	validTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	feedbackTraceID  = "4bf92f3577b34da6a3ce929d0e0e4736"
	// mirrors the unexported key in internal/middleware/internal_auth.go
	deploymentIDCtxKey = "deploy_token_deployment_id"
)

func TestTraceIDFromFeedbackContext(t *testing.T) {
	got, ok := traceIDFromFeedbackContext(FeedbackTraceContext{
		Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	})
	if !ok {
		t.Fatal("expected traceparent to parse")
	}
	if got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace id: got %q", got)
	}

	_, ok = traceIDFromFeedbackContext(FeedbackTraceContext{})
	if ok {
		t.Fatal("expected empty trace context to fail")
	}
}

func TestLangfuseScoreFromFeedbackThumbs(t *testing.T) {
	score, ok := langfuseScoreFromFeedback("dep_123", "4bf92f3577b34da6a3ce929d0e0e4736", FeedbackScoreRequest{
		Source:     "slack",
		ResponseID: "1700000000.000002",
		User:       FeedbackScoreUser{ID: "U123"},
		Feedback: FeedbackScoreFeedback{
			Kind: "thumbs_down",
		},
	})
	if !ok {
		t.Fatal("expected thumbs feedback to produce a score")
	}
	if score.Name != userFeedbackScoreName {
		t.Fatalf("score name: got %q", score.Name)
	}
	if score.DataType != "CATEGORICAL" {
		t.Fatalf("data type: got %q", score.DataType)
	}
	if score.Value != "thumbs_down" {
		t.Fatalf("value: got %#v", score.Value)
	}
	if score.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace id: got %q", score.TraceID)
	}
	if score.Comment != "" {
		t.Fatalf("comment: got %q", score.Comment)
	}
}

func TestLangfuseScoreFromFeedbackCommentTruncatesText(t *testing.T) {
	score, ok := langfuseScoreFromFeedback("dep_123", "4bf92f3577b34da6a3ce929d0e0e4736", FeedbackScoreRequest{
		Source:     "slack",
		ResponseID: "1700000000.000002",
		User:       FeedbackScoreUser{ID: "U123"},
		Feedback: FeedbackScoreFeedback{
			Kind: "comment",
			Text: strings.Repeat("a", maxLangfuseFeedbackTextChars+10),
		},
	})
	if !ok {
		t.Fatal("expected comment feedback to produce a score")
	}
	if score.Name != userFeedbackScoreName {
		t.Fatalf("score name: got %q", score.Name)
	}
	if score.DataType != "CATEGORICAL" {
		t.Fatalf("data type: got %q", score.DataType)
	}
	if score.Value != userFeedbackCommentValue {
		t.Fatalf("value: got %#v", score.Value)
	}
	if got := len(score.Comment); got != maxLangfuseFeedbackTextChars {
		t.Fatalf("text length: got %d", got)
	}
	if strings.Contains(score.Comment, "source=slack") {
		t.Fatalf("comment should contain only submitted feedback text: %q", score.Comment)
	}
}

func TestLangfuseScoreFromFeedbackUsesDistinctStableIDs(t *testing.T) {
	req := FeedbackScoreRequest{
		Source:     "slack",
		ResponseID: "1700000000.000002",
		User:       FeedbackScoreUser{ID: "U123"},
	}

	reactionReq := req
	reactionReq.Feedback = FeedbackScoreFeedback{Kind: "thumbs_up"}
	reactionScore, ok := langfuseScoreFromFeedback("dep_123", "4bf92f3577b34da6a3ce929d0e0e4736", reactionReq)
	if !ok {
		t.Fatal("expected thumbs feedback to produce a score")
	}

	commentReq := req
	commentReq.Feedback = FeedbackScoreFeedback{Kind: "comment", Text: "useful but incomplete"}
	commentScore, ok := langfuseScoreFromFeedback("dep_123", "4bf92f3577b34da6a3ce929d0e0e4736", commentReq)
	if !ok {
		t.Fatal("expected comment feedback to produce a score")
	}

	if reactionScore.Name != commentScore.Name {
		t.Fatalf("expected unified score name, got %q and %q", reactionScore.Name, commentScore.Name)
	}
	if reactionScore.ID == commentScore.ID {
		t.Fatal("expected reaction and comment scores to use distinct stable IDs")
	}
}

// An empty deploymentID leaves the context unset, exercising the 401 path. The
// returned cfg is the pointer the handler captured, so mutating it takes effect.
func setupFeedbackScoreRouter(t *testing.T, deploymentID string, upstream http.HandlerFunc) (*traceDetailFixture, *config.Config) {
	t.Helper()
	f, log, cfg, _, deployStore, langfuseStore := newLangfuseFixture(t, false, upstream)
	f.router.Use(func(c *gin.Context) {
		if deploymentID != "" {
			c.Set(deploymentIDCtxKey, deploymentID)
		}
		c.Next()
	})
	f.router.POST("/api/v1/deployments/feedback/scores",
		PostDeploymentFeedbackScore(log, cfg, deployStore, langfuseStore))
	return f, cfg
}

var langfuseCredColumns = []string{
	"account_id", "langfuse_project_id", "langfuse_public_key", "langfuse_secret_key", "created_at",
}

func langfuseCredsRow() *sqlmock.Rows {
	return sqlmock.NewRows(langfuseCredColumns).
		AddRow("acct-1", "proj-1", "pk-lf", "sk-lf", time.Now())
}

func expectFeedbackCreds(f *traceDetailFixture) {
	expectDeploymentLookup(f.deployMock, "dep-1", "acct-1", "sasbot", "build-1", "ns-1")
	f.langfuseMock.ExpectQuery("SELECT .+ FROM account_langfuse").
		WithArgs("acct-1").
		WillReturnRows(langfuseCredsRow())
}

func serveOwnedTrace(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(langfuse.TraceDetail{
		Trace: langfuse.Trace{ID: feedbackTraceID, Tags: []string{"deployment:dep-1"}},
	})
}

// withOwnedTrace serves the ownership-check trace lookup with a trace tagged for
// dep-1, and routes the score POST to scoreHandler.
func withOwnedTrace(scoreHandler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/public/traces/") {
			serveOwnedTrace(w)
			return
		}
		scoreHandler(w, r)
	}
}

func postFeedback(t *testing.T, f *traceDetailFixture, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deployments/feedback/scores", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func feedbackBody(kind string, text ...string) string {
	req := FeedbackScoreRequest{
		Source:       "slack",
		ResponseID:   "1700000000.000002",
		TraceContext: FeedbackTraceContext{Traceparent: validTraceparent},
		Feedback:     FeedbackScoreFeedback{Kind: kind},
		User:         FeedbackScoreUser{ID: "U123"},
	}
	if len(text) > 0 {
		req.Feedback.Text = text[0]
	}
	b, _ := json.Marshal(req)
	return string(b)
}

func TestPostFeedbackScore_Unauthorized(t *testing.T) {
	f, _ := setupFeedbackScoreRouter(t, "", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("upstream must not be called without a deployment ID")
	})

	rec := postFeedback(t, f, feedbackBody("thumbs_up"))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostFeedbackScore_InvalidBody(t *testing.T) {
	f, _ := setupFeedbackScoreRouter(t, "dep-1", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("upstream must not be called for a malformed body")
	})

	rec := postFeedback(t, f, "{not json")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostFeedbackScore_MissingTraceparent(t *testing.T) {
	f, _ := setupFeedbackScoreRouter(t, "dep-1", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("upstream must not be called without trace context")
	})

	body := `{"source":"slack","response_id":"r1","feedback":{"kind":"thumbs_up"},"user":{"id":"U123"}}`
	rec := postFeedback(t, f, body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostFeedbackScore_UnsupportedFeedback(t *testing.T) {
	f, _ := setupFeedbackScoreRouter(t, "dep-1", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("upstream must not be called for unsupported feedback")
	})

	rec := postFeedback(t, f, feedbackBody("banana"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostFeedbackScore_MissingRequiredFields(t *testing.T) {
	cases := map[string]string{
		"empty source":      `{"source":"","response_id":"r1","trace_context":{"traceparent":"` + validTraceparent + `"},"feedback":{"kind":"thumbs_up"},"user":{"id":"U123"}}`,
		"empty response_id": `{"source":"slack","response_id":"","trace_context":{"traceparent":"` + validTraceparent + `"},"feedback":{"kind":"thumbs_up"},"user":{"id":"U123"}}`,
		"empty user.id":     `{"source":"slack","response_id":"r1","trace_context":{"traceparent":"` + validTraceparent + `"},"feedback":{"kind":"thumbs_up"},"user":{"id":""}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			f, _ := setupFeedbackScoreRouter(t, "dep-1", func(_ http.ResponseWriter, _ *http.Request) {
				t.Error("upstream must not be called when a required field is missing")
			})

			rec := postFeedback(t, f, body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestPostFeedbackScore_DeploymentNotFound(t *testing.T) {
	f, _ := setupFeedbackScoreRouter(t, "dep-1", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("upstream must not be called for an unknown deployment")
	})
	expectDeploymentNotFound(f.deployMock, "dep-1")

	rec := postFeedback(t, f, feedbackBody("thumbs_up"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostFeedbackScore_NoCredentialsIs503(t *testing.T) {
	f, _ := setupFeedbackScoreRouter(t, "dep-1", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("upstream must not be called when the account has no Langfuse creds")
	})
	expectDeploymentLookup(f.deployMock, "dep-1", "acct-1", "sasbot", "build-1", "ns-1")
	f.langfuseMock.ExpectQuery("SELECT .+ FROM account_langfuse").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows(langfuseCredColumns))

	rec := postFeedback(t, f, feedbackBody("thumbs_up"))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostFeedbackScore_NoBaseURLIs503(t *testing.T) {
	f, cfg := setupFeedbackScoreRouter(t, "dep-1", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("upstream must not be called when no Langfuse base URL is configured")
	})
	cfg.Deployment.LangfuseBaseURL = ""
	expectFeedbackCreds(f)

	rec := postFeedback(t, f, feedbackBody("thumbs_up"))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostFeedbackScore_UpstreamErrorIs502(t *testing.T) {
	f, _ := setupFeedbackScoreRouter(t, "dep-1", withOwnedTrace(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	expectFeedbackCreds(f)

	rec := postFeedback(t, f, feedbackBody("thumbs_up"))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostFeedbackScore_ReactionHappyPath(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	f, _ := setupFeedbackScoreRouter(t, "dep-1", withOwnedTrace(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	expectFeedbackCreds(f)

	rec := postFeedback(t, f, feedbackBody("thumbs_up"))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if gotPath != "/api/public/scores" {
		t.Fatalf("upstream path = %q, want /api/public/scores", gotPath)
	}

	wantID := stableScoreID("dep-1", "slack", "1700000000.000002", "U123", userFeedbackScoreName, userFeedbackReactionScoreKey)
	if gotBody["id"] != wantID {
		t.Errorf("score id = %v, want %v", gotBody["id"], wantID)
	}
	if gotBody["traceId"] != feedbackTraceID {
		t.Errorf("traceId = %v, want %v", gotBody["traceId"], feedbackTraceID)
	}
	if gotBody["name"] != userFeedbackScoreName {
		t.Errorf("name = %v, want %v", gotBody["name"], userFeedbackScoreName)
	}
	if gotBody["value"] != "thumbs_up" {
		t.Errorf("value = %v, want thumbs_up", gotBody["value"])
	}
	if gotBody["dataType"] != "CATEGORICAL" {
		t.Errorf("dataType = %v, want CATEGORICAL", gotBody["dataType"])
	}
	if _, present := gotBody["comment"]; present {
		t.Errorf("reaction score must not carry a comment, got %v", gotBody["comment"])
	}

	var resp FeedbackScoreResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.ScoreID != wantID || resp.TraceID != feedbackTraceID {
		t.Errorf("response = %+v, want score_id=%s trace_id=%s", resp, wantID, feedbackTraceID)
	}
}

func TestPostFeedbackScore_CommentHappyPath(t *testing.T) {
	var gotBody map[string]any
	f, _ := setupFeedbackScoreRouter(t, "dep-1", withOwnedTrace(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	expectFeedbackCreds(f)

	rec := postFeedback(t, f, feedbackBody("comment", "great answer"))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}

	wantID := stableScoreID("dep-1", "slack", "1700000000.000002", "U123", userFeedbackScoreName, userFeedbackCommentScoreKey)
	if gotBody["id"] != wantID {
		t.Errorf("score id = %v, want %v", gotBody["id"], wantID)
	}
	if gotBody["value"] != userFeedbackCommentValue {
		t.Errorf("value = %v, want %v", gotBody["value"], userFeedbackCommentValue)
	}
	if gotBody["dataType"] != "CATEGORICAL" {
		t.Errorf("dataType = %v, want CATEGORICAL", gotBody["dataType"])
	}
	if gotBody["comment"] != "great answer" {
		t.Errorf("comment = %v, want %q", gotBody["comment"], "great answer")
	}
}

func TestPostFeedbackScore_RejectsForeignTrace(t *testing.T) {
	scorePosted := false
	f, _ := setupFeedbackScoreRouter(t, "dep-1", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/public/scores") {
			scorePosted = true
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(langfuse.TraceDetail{
			Trace: langfuse.Trace{ID: feedbackTraceID, Tags: []string{"deployment:other-dep"}},
		})
	})
	expectFeedbackCreds(f)

	rec := postFeedback(t, f, feedbackBody("thumbs_up"))

	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("expected a client-error status for a foreign trace, got %d: %s", rec.Code, rec.Body.String())
	}
	if scorePosted {
		t.Error("score must not be created for a trace owned by another deployment")
	}
}

// Feedback routinely arrives before Langfuse ingests the trace, so a not-found
// ownership lookup must not block scoring — the score is written anyway and
// Langfuse associates it to the trace by ID once it lands.
func TestPostFeedbackScore_MissingTraceWritesScore(t *testing.T) {
	scorePosted := false
	f, _ := setupFeedbackScoreRouter(t, "dep-1", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/public/scores") {
			scorePosted = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, `{"message":"trace not found"}`, http.StatusNotFound)
	})
	expectFeedbackCreds(f)

	rec := postFeedback(t, f, feedbackBody("thumbs_up"))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if !scorePosted {
		t.Error("score should be written even when the trace is not yet queryable")
	}
}

// A genuine (non-not-found) lookup failure is not best-effort: the score must
// not be written and the client sees 502.
func TestPostFeedbackScore_LookupErrorIs502(t *testing.T) {
	f, _ := setupFeedbackScoreRouter(t, "dep-1", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/public/scores") {
			t.Error("score must not be created when the trace lookup fails")
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	expectFeedbackCreds(f)

	rec := postFeedback(t, f, feedbackBody("thumbs_up"))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
}
