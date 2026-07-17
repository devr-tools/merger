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

type EvidencePublisher interface {
	PublishWithEvidence(context.Context, domain.ChangePacket, []domain.EvidenceExecution) error
}

type GitHubCheckPublisher struct {
	client github.CheckRunPublisher
}

func NewGitHubCheckPublisher(client github.CheckRunPublisher) *GitHubCheckPublisher {
	return &GitHubCheckPublisher{client: client}
}

func (p *GitHubCheckPublisher) Publish(ctx context.Context, packet domain.ChangePacket) error {
	return p.PublishWithEvidence(ctx, packet, nil)
}

func (p *GitHubCheckPublisher) PublishWithEvidence(ctx context.Context, packet domain.ChangePacket, executions []domain.EvidenceExecution) error {
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
		Summary:    buildCheckSummary(packet, executions),
	})
}

func buildCheckSummary(packet domain.ChangePacket, executions []domain.EvidenceExecution) string {
	var summary strings.Builder
	fmt.Fprintln(&summary, "## Merger change control")
	fmt.Fprintln(&summary)
	fmt.Fprintln(&summary, "| Merge lane | Risk | Decision |")
	fmt.Fprintln(&summary, "| --- | ---: | --- |")
	fmt.Fprintf(&summary, "| **%s** | %d (%s) | %s |\n", packet.MergeLane, packet.RiskSummary.Score, packet.RiskSummary.Severity, packet.Decision.Status)

	writeDecisionSummary(&summary, packet)
	writeMutationsSummary(&summary, packet)
	writeRiskSummary(&summary, packet)
	writeReviewerSummary(&summary, packet)
	writeEvidenceSummary(&summary, packet, executions)
	writeDeploymentSummary(&summary, packet)

	return truncateSummary(summary.String(), 60000)
}

func writeDecisionSummary(summary *strings.Builder, packet domain.ChangePacket) {
	if packet.Decision.Summary != "" {
		fmt.Fprintf(summary, "\n**Why:** %s\n", packet.Decision.Summary)
	}
	if len(packet.Decision.AppliedPolicies) > 0 {
		fmt.Fprintf(summary, "\n**Applied policies:** %s\n", strings.Join(packet.Decision.AppliedPolicies, ", "))
	}
}

func writeMutationsSummary(summary *strings.Builder, packet domain.ChangePacket) {
	if len(packet.Mutations) == 0 {
		return
	}
	fmt.Fprintln(summary, "\n### Detected mutations")
	for _, mutation := range packet.Mutations {
		fmt.Fprintf(summary, "- **%s** `%s` — %s\n", mutation.Severity, mutation.Kind, mutation.Title)
	}
}

func writeRiskSummary(summary *strings.Builder, packet domain.ChangePacket) {
	if len(packet.Risks) == 0 {
		return
	}
	fmt.Fprintln(summary, "\n### Risk and mitigation")
	for _, risk := range packet.Risks {
		fmt.Fprintf(summary, "- **%s (+%d):** %s\n", risk.Type, risk.Score, risk.Summary)
		for _, mitigation := range risk.Mitigations {
			fmt.Fprintf(summary, "  - %s\n", mitigation)
		}
	}
}

func writeReviewerSummary(summary *strings.Builder, packet domain.ChangePacket) {
	if len(packet.Reviewers) == 0 {
		return
	}
	fmt.Fprintln(summary, "\n### Required reviewers")
	for _, reviewer := range packet.Reviewers {
		fmt.Fprintf(summary, "- `%s` (%s): %s\n", reviewer.Team, reviewerRequirement(reviewer), reviewer.Reason)
	}
}

func reviewerRequirement(reviewer domain.ReviewerRequirement) string {
	if reviewer.Mandatory {
		return "required"
	}
	return "recommended"
}

func writeEvidenceSummary(summary *strings.Builder, packet domain.ChangePacket, executions []domain.EvidenceExecution) {
	if len(packet.Evidence) == 0 {
		return
	}
	fmt.Fprintln(summary, "\n### Evidence checklist")
	statusByName := evidenceStatuses(executions)
	for _, evidence := range packet.Evidence {
		fmt.Fprintln(summary, evidenceSummaryLine(evidence, statusByName[evidence.Name]))
	}
}

func evidenceSummaryLine(evidence domain.EvidenceRequirement, status domain.EvidenceStatus) string {
	requirement := "optional"
	if evidence.Required {
		requirement = "required"
	}
	if status == "" {
		status = domain.EvidencePending
	}
	marker := " "
	if status == domain.EvidenceSatisfied || status == domain.EvidenceWaived {
		marker = "x"
	}
	line := fmt.Sprintf("- [%s] `%s` (%s) — **%s**", marker, evidence.Name, requirement, status)
	if evidence.Reason != "" {
		line += ": " + evidence.Reason
	}
	return line
}

func writeDeploymentSummary(summary *strings.Builder, packet domain.ChangePacket) {
	fmt.Fprintln(summary, "\n### Deployment guidance")
	fmt.Fprintf(summary, "- Strategy: **%s**\n", packet.Deployment.Strategy)
	if packet.Deployment.RequiresCanary {
		fmt.Fprintln(summary, "- Canary rollout required")
	}
	if packet.Deployment.RequiresRollbackPlan {
		fmt.Fprintln(summary, "- Rollback plan required")
	}
	if len(packet.Deployment.Environments) > 0 {
		fmt.Fprintf(summary, "- Environments: %s\n", strings.Join(packet.Deployment.Environments, ", "))
	}
}

func evidenceStatuses(executions []domain.EvidenceExecution) map[string]domain.EvidenceStatus {
	statuses := make(map[string]domain.EvidenceStatus, len(executions))
	for _, execution := range executions {
		statuses[execution.Name] = execution.Status
	}
	return statuses
}

func truncateSummary(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes-1]) + "…"
}
