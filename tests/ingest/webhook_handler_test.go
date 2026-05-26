package ingest_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mergerhq/merger/internal/events"
	"github.com/mergerhq/merger/internal/github"
	"github.com/mergerhq/merger/internal/ingest"
	"github.com/mergerhq/merger/internal/lanes"
	"github.com/mergerhq/merger/internal/mutations"
	"github.com/mergerhq/merger/internal/policy"
	"github.com/mergerhq/merger/internal/risk"
	"github.com/mergerhq/merger/internal/runtimegraph"
	"github.com/mergerhq/merger/internal/telemetry"
)

func TestWebhookHandlerRejectsInvalidWebhook(t *testing.T) {
	handler := ingest.NewWebhookHandler(newTestProcessor(), github.NewWebhookDecoder(""))
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestWebhookHandlerIgnoresNonPullRequestEvents(t *testing.T) {
	handler := ingest.NewWebhookHandler(newTestProcessor(), github.NewWebhookDecoder(""))
	req := newWebhookRequest(t, "issues", map[string]any{"action": "opened"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
}

func TestWebhookHandlerReturnsChangePacketSummary(t *testing.T) {
	handler := ingest.NewWebhookHandler(newTestProcessor(), github.NewWebhookDecoder(""))
	req := newWebhookRequest(t, "pull_request", map[string]any{
		"action": "opened",
		"installation": map[string]any{
			"id": 42,
		},
		"repository": map[string]any{
			"name":      "repo",
			"full_name": "acme/repo",
			"owner": map[string]any{
				"login": "acme",
			},
		},
		"pull_request": map[string]any{
			"number":   42,
			"title":    "add endpoint",
			"body":     "body",
			"html_url": "https://example.com/pr/42",
			"head": map[string]any{
				"sha": "headsha",
			},
			"base": map[string]any{
				"sha": "basesha",
			},
			"user": map[string]any{
				"login": "bot",
				"type":  "Bot",
			},
		},
	})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["changePacketId"] == "" {
		t.Fatalf("expected change packet id, got %#v", payload)
	}
}

func newTestProcessor() *ingest.Processor {
	return ingest.NewProcessor(
		telemetry.NewLogger("error"),
		telemetry.NewTracer(),
		events.NewMemoryBus(),
		stubGitHubService{},
		mutations.DefaultEngine(),
		risk.DefaultEngine(),
		policy.NewRuleEngine(policy.Config{}),
		lanes.NewAssigner(lanes.Config{GreenMax: 20, YellowMax: 55, RedMax: 85}),
		stubCheckPublisher{},
		runtimegraph.NewResolver(runtimegraph.Options{}),
		nil,
	)
}

func newWebhookRequest(t *testing.T, event string, payload map[string]any) *http.Request {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-GitHub-Delivery", "delivery-1")
	return req
}
