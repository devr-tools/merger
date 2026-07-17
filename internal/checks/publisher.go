package checks

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/github"
)

type Publisher interface {
	Publish(context.Context, domain.ChangePacket) error
}

type GitHubCheckPublisher struct {
	client github.CheckRunPublisher
}

func NewGitHubCheckPublisher(client github.CheckRunPublisher) *GitHubCheckPublisher {
	return &GitHubCheckPublisher{client: client}
}

func (p *GitHubCheckPublisher) Publish(ctx context.Context, packet domain.ChangePacket) error {
	if p.client == nil {
		return nil
	}

	client := p.client
	if binder, ok := p.client.(github.InstallationBinder); ok {
		if rawInstallationID := packet.Metadata["installation_id"]; rawInstallationID != "" {
			installationID, err := strconv.ParseInt(rawInstallationID, 10, 64)
			if err == nil {
				client = binder.ForInstallation(installationID)
			}
		}
	}

	status := "completed"
	conclusion := "neutral"
	switch packet.MergeLane {
	case domain.MergeLaneGreen:
		conclusion = "success"
	case domain.MergeLaneBlack:
		conclusion = "action_required"
	case domain.MergeLaneRed:
		conclusion = "neutral"
	}

	return client.PublishCheckRun(ctx, github.CheckRunInput{
		RepoOwner:  packet.Repo.Owner,
		RepoName:   packet.Repo.Name,
		HeadSHA:    packet.PR.HeadSHA,
		Name:       "merger/change-control",
		Status:     status,
		Conclusion: conclusion,
		Summary:    buildCheckSummary(packet),
	})
}

func buildCheckSummary(packet domain.ChangePacket) string {
	var summary strings.Builder
	fmt.Fprintln(&summary, "## Merger change control")
	fmt.Fprintln(&summary)
	fmt.Fprintln(&summary, "| Merge lane | Risk | Decision |")
	fmt.Fprintln(&summary, "| --- | ---: | --- |")
	fmt.Fprintf(&summary, "| **%s** | %d (%s) | %s |\n", packet.MergeLane, packet.RiskSummary.Score, packet.RiskSummary.Severity, packet.Decision.Status)

	if packet.Decision.Summary != "" {
		fmt.Fprintf(&summary, "\n**Why:** %s\n", packet.Decision.Summary)
	}
	if len(packet.Decision.AppliedPolicies) > 0 {
		fmt.Fprintf(&summary, "\n**Applied policies:** %s\n", strings.Join(packet.Decision.AppliedPolicies, ", "))
	}

	if len(packet.Mutations) > 0 {
		fmt.Fprintln(&summary, "\n### Detected mutations")
		for _, mutation := range packet.Mutations {
			fmt.Fprintf(&summary, "- **%s** `%s` — %s\n", mutation.Severity, mutation.Kind, mutation.Title)
		}
	}

	if len(packet.Risks) > 0 {
		fmt.Fprintln(&summary, "\n### Risk and mitigation")
		for _, risk := range packet.Risks {
			fmt.Fprintf(&summary, "- **%s (+%d):** %s\n", risk.Type, risk.Score, risk.Summary)
			for _, mitigation := range risk.Mitigations {
				fmt.Fprintf(&summary, "  - %s\n", mitigation)
			}
		}
	}

	if len(packet.Reviewers) > 0 {
		fmt.Fprintln(&summary, "\n### Required reviewers")
		for _, reviewer := range packet.Reviewers {
			marker := "recommended"
			if reviewer.Mandatory {
				marker = "required"
			}
			fmt.Fprintf(&summary, "- `%s` (%s): %s\n", reviewer.Team, marker, reviewer.Reason)
		}
	}

	if len(packet.Evidence) > 0 {
		fmt.Fprintln(&summary, "\n### Evidence checklist")
		for _, evidence := range packet.Evidence {
			requirement := "optional"
			if evidence.Required {
				requirement = "required"
			}
			fmt.Fprintf(&summary, "- [ ] `%s` (%s) — %s\n", evidence.Name, requirement, evidence.Reason)
		}
	}

	fmt.Fprintln(&summary, "\n### Deployment guidance")
	fmt.Fprintf(&summary, "- Strategy: **%s**\n", packet.Deployment.Strategy)
	if packet.Deployment.RequiresCanary {
		fmt.Fprintln(&summary, "- Canary rollout required")
	}
	if packet.Deployment.RequiresRollbackPlan {
		fmt.Fprintln(&summary, "- Rollback plan required")
	}
	if len(packet.Deployment.Environments) > 0 {
		fmt.Fprintf(&summary, "- Environments: %s\n", strings.Join(packet.Deployment.Environments, ", "))
	}

	return truncateSummary(summary.String(), 60000)
}

func truncateSummary(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes-1]) + "…"
}
