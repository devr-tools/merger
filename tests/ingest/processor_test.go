package ingest_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/events"
	"github.com/devr-tools/merger/internal/github"
	"github.com/devr-tools/merger/internal/ingest"
	"github.com/devr-tools/merger/internal/lanes"
	"github.com/devr-tools/merger/internal/mutations"
	"github.com/devr-tools/merger/internal/policy"
	"github.com/devr-tools/merger/internal/risk"
	"github.com/devr-tools/merger/internal/runtimegraph"
	"github.com/devr-tools/merger/internal/telemetry"
)

func TestProcessPROpenedBuildsChangePacket(t *testing.T) {
	processor := ingest.NewProcessor(
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

	packet, err := processor.ProcessPROpened(context.Background(), github.PullRequestWebhookPayload{
		Action: "opened",
		Repository: struct {
			Name     string `json:"name"`
			FullName string `json:"full_name"`
			Owner    struct {
				Login string `json:"login"`
			} `json:"owner"`
		}{
			Name:     "repo",
			FullName: "acme/repo",
			Owner: struct {
				Login string `json:"login"`
			}{Login: "acme"},
		},
		PullRequest: struct {
			Number  int    `json:"number"`
			Title   string `json:"title"`
			Body    string `json:"body"`
			HTMLURL string `json:"html_url"`
			Head    struct {
				SHA string `json:"sha"`
			} `json:"head"`
			Base struct {
				SHA string `json:"sha"`
			} `json:"base"`
			User struct {
				Login string `json:"login"`
				Type  string `json:"type"`
			} `json:"user"`
		}{
			Number:  42,
			Title:   "add endpoint",
			Body:    "body",
			HTMLURL: "https://example.com/pr/42",
			Head: struct {
				SHA string `json:"sha"`
			}{SHA: "headsha"},
			Base: struct {
				SHA string `json:"sha"`
			}{SHA: "basesha"},
			User: struct {
				Login string `json:"login"`
				Type  string `json:"type"`
			}{Login: "bot", Type: "Bot"},
		},
	})
	if err != nil {
		t.Fatalf("process PR opened: %v", err)
	}
	if packet.Repo.FullName != "acme/repo" {
		t.Fatalf("expected repo full name, got %s", packet.Repo.FullName)
	}
	if packet.PR.Number != 42 {
		t.Fatalf("expected PR number 42, got %d", packet.PR.Number)
	}
	if len(packet.Mutations) == 0 {
		t.Fatal("expected mutations to be detected")
	}
}

func TestProcessPROpenedReturnsErrorWhenDiffRetrievalFails(t *testing.T) {
	processor := ingest.NewProcessor(
		telemetry.NewLogger("error"),
		telemetry.NewTracer(),
		events.NewMemoryBus(),
		failingDiffGitHubService{},
		mutations.DefaultEngine(),
		risk.DefaultEngine(),
		policy.NewRuleEngine(policy.Config{}),
		lanes.NewAssigner(lanes.Config{GreenMax: 20, YellowMax: 55, RedMax: 85}),
		stubCheckPublisher{},
		runtimegraph.NewResolver(runtimegraph.Options{}),
		nil,
	)

	packet, err := processor.ProcessPROpened(context.Background(), github.PullRequestWebhookPayload{
		Repository: struct {
			Name     string `json:"name"`
			FullName string `json:"full_name"`
			Owner    struct {
				Login string `json:"login"`
			} `json:"owner"`
		}{
			Name:     "repo",
			FullName: "acme/repo",
			Owner: struct {
				Login string `json:"login"`
			}{Login: "acme"},
		},
		PullRequest: struct {
			Number  int    `json:"number"`
			Title   string `json:"title"`
			Body    string `json:"body"`
			HTMLURL string `json:"html_url"`
			Head    struct {
				SHA string `json:"sha"`
			} `json:"head"`
			Base struct {
				SHA string `json:"sha"`
			} `json:"base"`
			User struct {
				Login string `json:"login"`
				Type  string `json:"type"`
			} `json:"user"`
		}{Number: 42},
	})

	if err == nil {
		t.Fatal("expected diff retrieval error")
	}
	if packet != nil {
		t.Fatalf("expected no packet, got %#v", packet)
	}
	if !strings.Contains(err.Error(), "get pull request diff for acme/repo#42") {
		t.Fatalf("expected contextual diff error, got %v", err)
	}
	if !errors.Is(err, errDiffUnavailable) {
		t.Fatalf("expected original diff error to be preserved, got %v", err)
	}
}

type stubGitHubService struct{}

var errDiffUnavailable = errors.New("diff unavailable")

type failingDiffGitHubService struct {
	stubGitHubService
}

func (failingDiffGitHubService) GetPullRequestDiff(context.Context, string, string, int) (string, error) {
	return "", errDiffUnavailable
}

func (stubGitHubService) GetPullRequest(_ context.Context, owner, repo string, number int) (github.PullRequest, error) {
	return github.PullRequest{
		Owner:   owner,
		Repo:    repo,
		Number:  number,
		Title:   "add endpoint",
		Body:    "body",
		Author:  "bot",
		URL:     "https://example.com/pr/42",
		HeadSHA: "headsha",
		BaseSHA: "basesha",
	}, nil
}

func (stubGitHubService) GetPullRequestDiff(_ context.Context, _, _ string, _ int) (string, error) {
	return "diff --git a/internal/auth/jwt.go b/internal/auth/jwt.go\n--- a/internal/auth/jwt.go\n+++ b/internal/auth/jwt.go\n+func Authorize() {}\n", nil
}

func (stubGitHubService) GetFileContent(_ context.Context, _, _, path, _ string) ([]byte, error) {
	if path == "internal/auth/jwt.go" {
		return []byte("package auth\nfunc Authorize() {}\n"), nil
	}
	return nil, nil
}

func (stubGitHubService) PublishCheckRun(context.Context, github.CheckRunInput) error { return nil }

type stubCheckPublisher struct{}

func (stubCheckPublisher) Publish(context.Context, domain.ChangePacket) error { return nil }
