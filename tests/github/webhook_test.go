package github_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"testing"

	"github.com/devr-tools/merger/internal/github"
)

func TestWebhookDecoderRejectsInvalidSignature(t *testing.T) {
	decoder := github.NewWebhookDecoder("secret")
	req := httptest.NewRequest("POST", "/webhooks/github", bytes.NewBufferString(`{"action":"opened"}`))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", "sha256=bad")

	if _, err := decoder.Decode(req); err == nil {
		t.Fatal("expected invalid signature error")
	}
}

func TestWebhookDecoderAcceptsValidSignature(t *testing.T) {
	body := []byte(`{"action":"opened","repository":{"name":"repo","full_name":"acme/repo","owner":{"login":"acme"}},"pull_request":{"number":1,"title":"x","body":"y","html_url":"https://example.com","head":{"sha":"head"},"base":{"sha":"base"},"user":{"login":"bot","type":"Bot"}}}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	decoder := github.NewWebhookDecoder("secret")
	req := httptest.NewRequest("POST", "/webhooks/github", bytes.NewBuffer(body))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", signature)

	hook, err := decoder.Decode(req)
	if err != nil {
		t.Fatalf("decode webhook: %v", err)
	}
	if hook.Payload.Action != "opened" {
		t.Fatalf("expected opened action, got %s", hook.Payload.Action)
	}
	if hook.CheckRun != nil {
		t.Fatal("did not expect a check run payload for a pull_request event")
	}
}

func TestWebhookDecoderDecodesCheckRunEvent(t *testing.T) {
	body := []byte(`{
"action":"completed",
"installation":{"id":42},
"repository":{"name":"repo","full_name":"acme/repo","owner":{"login":"acme"}},
"check_run":{
  "id":99,
  "name":"integration-tests",
  "head_sha":"abc123",
  "status":"completed",
  "conclusion":"success",
  "details_url":"https://ci.example/runs/99",
  "output":{"title":"Integration tests","summary":"All tests passed"},
  "app":{"id":1234,"slug":"acme-ci"},
  "pull_requests":[{"number":7}]
}}`)
	decoder := github.NewWebhookDecoder("")
	req := httptest.NewRequest("POST", "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "check_run")
	req.Header.Set("X-GitHub-Delivery", "delivery-99")

	hook, err := decoder.Decode(req)
	if err != nil {
		t.Fatalf("decode check run webhook: %v", err)
	}
	if hook.Event != "check_run" || hook.DeliveryID != "delivery-99" {
		t.Fatalf("unexpected webhook metadata: %#v", hook)
	}
	if hook.CheckRun == nil {
		t.Fatal("expected check run payload")
	}
	if hook.Payload.Action != "" {
		t.Fatalf("expected pull request payload to remain empty, got %#v", hook.Payload)
	}

	payload := hook.CheckRun
	if payload.Action != "completed" || payload.Installation.ID != 42 || payload.Repository.FullName != "acme/repo" {
		t.Fatalf("unexpected check run envelope: %#v", payload)
	}
	if payload.CheckRun.ID != 99 || payload.CheckRun.Name != "integration-tests" || payload.CheckRun.HeadSHA != "abc123" {
		t.Fatalf("unexpected check run identity: %#v", payload.CheckRun)
	}
	if payload.CheckRun.Status != "completed" || payload.CheckRun.Conclusion != "success" || payload.CheckRun.DetailsURL != "https://ci.example/runs/99" {
		t.Fatalf("unexpected check run status: %#v", payload.CheckRun)
	}
	if payload.CheckRun.Output.Title != "Integration tests" || payload.CheckRun.Output.Summary != "All tests passed" {
		t.Fatalf("unexpected check run output: %#v", payload.CheckRun.Output)
	}
	if payload.CheckRun.App.ID != 1234 || payload.CheckRun.App.Slug != "acme-ci" {
		t.Fatalf("unexpected check run app: %#v", payload.CheckRun.App)
	}
	if len(payload.CheckRun.PullRequests) != 1 || payload.CheckRun.PullRequests[0].Number != 7 {
		t.Fatalf("unexpected associated pull requests: %#v", payload.CheckRun.PullRequests)
	}
}
