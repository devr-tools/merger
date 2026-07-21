package policy

import (
	"fmt"
	"strings"

	"github.com/devr-tools/merger/internal/domain"
)

func applyRule(evaluation *Evaluation, rule RuleConfig) {
	evaluation.Decision.AppliedPolicies = append(evaluation.Decision.AppliedPolicies, rule.Name)
	if rule.Description != "" {
		evaluation.Decision.Reasons = append(evaluation.Decision.Reasons, rule.Description)
	}

	applyReviewerRequirements(evaluation, rule)
	applyEvidenceRequirements(evaluation, rule)
	applyDeploymentRequirements(evaluation, rule)
	applyPolicyAction(evaluation, rule)
}

func applyReviewerRequirements(evaluation *Evaluation, rule RuleConfig) {
	for _, reviewer := range rule.Require.Reviewers {
		evaluation.Reviewers = appendUniqueReviewer(evaluation.Reviewers, domain.ReviewerRequirement{
			Team:      reviewer,
			Reason:    fmt.Sprintf("required by policy %s", rule.Name),
			Mandatory: true,
		})
	}
}

func applyEvidenceRequirements(evaluation *Evaluation, rule RuleConfig) {
	for _, evidence := range rule.Require.Evidence {
		var githubCheck *domain.GitHubCheckBinding
		for _, binding := range rule.Require.GitHubChecks {
			if binding.Evidence == evidence {
				githubCheck = &domain.GitHubCheckBinding{Name: binding.Name, AppID: binding.AppID}
				break
			}
		}
		evaluation.Evidence = appendUniqueEvidence(evaluation.Evidence, domain.EvidenceRequirement{
			Type:        domain.EvidenceType(evidence),
			Name:        evidence,
			Required:    true,
			Reason:      fmt.Sprintf("required by policy %s", rule.Name),
			Producer:    "policy-engine",
			GitHubCheck: githubCheck,
		})
	}
}

func applyDeploymentRequirements(evaluation *Evaluation, rule RuleConfig) {
	if rule.Require.Deployment.Strategy != "" {
		evaluation.Deployment.Strategy = domain.DeploymentStrategy(rule.Require.Deployment.Strategy)
	}
	if len(rule.Require.Deployment.Environments) > 0 {
		evaluation.Deployment.Environments = append(evaluation.Deployment.Environments, rule.Require.Deployment.Environments...)
	}
	evaluation.Deployment.RequiresCanary = evaluation.Deployment.RequiresCanary || rule.Require.Deployment.RequiresCanary
	evaluation.Deployment.RequiresRollbackPlan = evaluation.Deployment.RequiresRollbackPlan || rule.Require.Deployment.RequiresRollbackPlan
}

func applyPolicyAction(evaluation *Evaluation, rule RuleConfig) {
	if rule.Action.MinimumLane != "" {
		evaluation.Decision.MinimumLane = maxLane(evaluation.Decision.MinimumLane, rule.Action.MinimumLane)
	}

	if rule.Action.Block {
		evaluation.Decision.Status = domain.DecisionBlocked
		evaluation.Decision.Violations = append(evaluation.Decision.Violations, domain.PolicyViolation{
			Policy:   rule.Name,
			Reason:   "policy blocked change propagation",
			Severity: domain.SeverityCritical,
		})
		return
	}

	if len(evaluation.Reviewers) > 0 || len(evaluation.Evidence) > 0 {
		evaluation.Decision.Status = domain.DecisionPending
	}
}

func finalizeDecision(evaluation *Evaluation) {
	evaluation.Decision.Summary = strings.Join(evaluation.Decision.Reasons, "; ")
	if evaluation.Decision.Summary == "" {
		evaluation.Decision.Summary = "no blocking policy constraints"
	}
}
