package checks_test

import (
	"context"
	"strings"
	"testing"

	"github.com/devr-tools/merger/internal/checks"
	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/github"
)

func TestPublishNoopsWithoutClient(t *testing.T) {
	publisher := checks.NewGitHubCheckPublisher(nil)

	err := publisher.Publish(context.Background(), domain.ChangePacket{})
	if err != nil {
		t.Fatalf("publish returned error: %v", err)
	}
}

func TestPublishUsesInstallationBoundClient(t *testing.T) {
	root := &stubCheckService{}
	publisher := checks.NewGitHubCheckPublisher(root)

	packet := domain.ChangePacket{
		Repo:      domain.RepoRef{Owner: "acme", Name: "merger"},
		PR:        domain.PullRequestRef{HeadSHA: "abc123"},
		MergeLane: domain.MergeLaneGreen,
		RiskSummary: domain.RiskSummary{
			Score: 12,
		},
		Mutations: []domain.Mutation{{ID: "m1"}},
		Decision: domain.PolicyDecision{
			AppliedPolicies: []string{"requires_tests"},
		},
		Metadata: map[string]string{"installation_id": "42"},
	}

	if err := publisher.Publish(context.Background(), packet); err != nil {
		t.Fatalf("publish returned error: %v", err)
	}

	if root.boundInstallationID != 42 {
		t.Fatalf("expected installation binding, got %d", root.boundInstallationID)
	}
	if len(root.bound.published) != 1 {
		t.Fatalf("expected one published check run, got %d", len(root.bound.published))
	}

	input := root.bound.published[0]
	if input.Conclusion != "success" {
		t.Fatalf("expected success conclusion, got %q", input.Conclusion)
	}
	if input.Summary == "" {
		t.Fatal("expected non-empty summary")
	}
}

func TestPublishMapsLaneToConclusion(t *testing.T) {
	testCases := []struct {
		name       string
		lane       domain.MergeLane
		conclusion string
	}{
		{name: "green", lane: domain.MergeLaneGreen, conclusion: "success"},
		{name: "yellow", lane: domain.MergeLaneYellow, conclusion: "neutral"},
		{name: "red", lane: domain.MergeLaneRed, conclusion: "neutral"},
		{name: "black", lane: domain.MergeLaneBlack, conclusion: "action_required"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := &stubCheckService{}
			publisher := checks.NewGitHubCheckPublisher(client)

			err := publisher.Publish(context.Background(), domain.ChangePacket{
				Repo:        domain.RepoRef{Owner: "acme", Name: "merger"},
				PR:          domain.PullRequestRef{HeadSHA: "abc123"},
				MergeLane:   tc.lane,
				RiskSummary: domain.RiskSummary{Score: 44},
			})
			if err != nil {
				t.Fatalf("publish returned error: %v", err)
			}

			if len(client.published) != 1 {
				t.Fatalf("expected one check run, got %d", len(client.published))
			}
			if client.published[0].Conclusion != tc.conclusion {
				t.Fatalf("expected conclusion %q, got %q", tc.conclusion, client.published[0].Conclusion)
			}
		})
	}
}

func TestPublishIncludesActionableMarkdownSummary(t *testing.T) {
	client := &stubCheckService{}
	publisher := checks.NewGitHubCheckPublisher(client)
	packet := domain.ChangePacket{
		Repo:        domain.RepoRef{Owner: "acme", Name: "payments"},
		PR:          domain.PullRequestRef{HeadSHA: "abc123"},
		MergeLane:   domain.MergeLaneRed,
		RiskSummary: domain.RiskSummary{Score: 67, Severity: domain.SeverityHigh},
		Mutations: []domain.Mutation{{
			Kind:     domain.MutationAuthBehaviorChange,
			Severity: domain.SeverityHigh,
			Title:    "authentication behavior changed",
		}},
		Risks: []domain.Risk{{
			Type:        domain.RiskSecurity,
			Score:       35,
			Summary:     "security boundary changed",
			Mitigations: []string{"run authentication integration tests"},
		}},
		Decision: domain.PolicyDecision{
			Status:          domain.DecisionPending,
			Summary:         "security review and evidence are required",
			AppliedPolicies: []string{"auth_requires_security_review"},
		},
		Reviewers: []domain.ReviewerRequirement{{Team: "security", Mandatory: true, Reason: "required by policy"}},
		Evidence: []domain.EvidenceRequirement{{
			Name:     "auth_integration_tests",
			Type:     domain.EvidenceAuthTests,
			Required: true,
			Reason:   "required by policy",
		}},
		Deployment: domain.DeploymentRequirement{
			Strategy:             domain.DeployCanary,
			RequiresCanary:       true,
			RequiresRollbackPlan: true,
		},
	}

	if err := publisher.Publish(context.Background(), packet); err != nil {
		t.Fatalf("publish returned error: %v", err)
	}
	if len(client.published) != 1 {
		t.Fatalf("expected one check run, got %d", len(client.published))
	}

	summary := client.published[0].Summary
	for _, want := range []string{
		"## Merger change control",
		"**Why:** security review and evidence are required",
		"### Detected mutations",
		"### Risk and mitigation",
		"### Required reviewers",
		"### Evidence checklist",
		"Canary rollout required",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("expected check summary to contain %q, got:\n%s", want, summary)
		}
	}
}

func TestPublishWithEvidenceIncludesExecutionStatuses(t *testing.T) {
	client := &stubCheckService{}
	publisher := checks.NewGitHubCheckPublisher(client)
	packet := domain.ChangePacket{
		Repo:      domain.RepoRef{Owner: "acme", Name: "payments"},
		PR:        domain.PullRequestRef{HeadSHA: "abc123"},
		MergeLane: domain.MergeLaneYellow,
		Evidence: []domain.EvidenceRequirement{
			{Name: "integration_tests", Required: true},
			{Name: "security_review", Required: true},
		},
	}
	executions := []domain.EvidenceExecution{
		{Name: "integration_tests", Status: domain.EvidenceSatisfied},
		{Name: "security_review", Status: domain.EvidenceFailed},
	}

	if err := publisher.PublishWithEvidence(context.Background(), packet, executions); err != nil {
		t.Fatalf("publish with evidence returned error: %v", err)
	}

	summary := client.published[0].Summary
	for _, want := range []string{
		"- [x] `integration_tests` (required) — **satisfied**",
		"- [ ] `security_review` (required) — **failed**",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("expected check summary to contain %q, got:\n%s", want, summary)
		}
	}
}

type stubCheckService struct {
	published           []github.CheckRunInput
	bound               *stubCheckService
	boundInstallationID int64
}

func (s *stubCheckService) PublishCheckRun(_ context.Context, input github.CheckRunInput) error {
	s.published = append(s.published, input)
	return nil
}

func (s *stubCheckService) ForInstallation(installationID int64) github.Service {
	s.boundInstallationID = installationID
	s.bound = &stubCheckService{}
	return s.bound
}

func (s *stubCheckService) GetPullRequest(context.Context, string, string, int) (github.PullRequest, error) {
	return github.PullRequest{}, nil
}

func (s *stubCheckService) GetPullRequestDiff(context.Context, string, string, int) (string, error) {
	return "", nil
}

func (s *stubCheckService) GetFileContent(context.Context, string, string, string, string) ([]byte, error) {
	return nil, nil
}
