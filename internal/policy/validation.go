package policy

import (
	"fmt"
	"strings"

	"github.com/devr-tools/merger/internal/domain"
)

var supportedMutationKinds = map[domain.MutationKind]struct{}{
	domain.MutationUnknown:               {},
	domain.MutationAuthBehaviorChange:    {},
	domain.MutationDatabaseSchema:        {},
	domain.MutationRuntimeConfig:         {},
	domain.MutationAPIContract:           {},
	domain.MutationDependency:            {},
	domain.MutationInfrastructure:        {},
	domain.MutationDataAccess:            {},
	domain.MutationDeploymentWorkflow:    {},
	domain.MutationObservabilityContract: {},
}

var supportedCriticalities = map[domain.Criticality]struct{}{
	domain.CriticalityLow:    {},
	domain.CriticalityNormal: {},
	domain.CriticalityHigh:   {},
	domain.CriticalityTier0:  {},
}

var supportedLanes = map[domain.MergeLane]struct{}{
	domain.MergeLaneGreen:  {},
	domain.MergeLaneYellow: {},
	domain.MergeLaneRed:    {},
	domain.MergeLaneBlack:  {},
}

var supportedDeploymentStrategies = map[string]struct{}{
	string(domain.DeployDirect):         {},
	string(domain.DeployCanary):         {},
	string(domain.DeployPhased):         {},
	string(domain.DeployManualApproval): {},
}

// Validate rejects policy definitions that cannot match predictably or cannot
// produce a meaningful requirement or decision.
func Validate(config Config) error {
	names := make(map[string]struct{}, len(config.Policies))
	for index, rule := range config.Policies {
		label := fmt.Sprintf("policy[%d]", index)
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			return fmt.Errorf("%s name must not be empty", label)
		}
		normalizedName := strings.ToLower(name)
		if _, duplicate := names[normalizedName]; duplicate {
			return fmt.Errorf("duplicate policy name %q", name)
		}
		names[normalizedName] = struct{}{}

		if err := validateRule(label+" ("+name+")", rule); err != nil {
			return err
		}
	}
	return nil
}

func validateRule(label string, rule RuleConfig) error {
	if !hasCondition(rule.When) {
		return fmt.Errorf("%s must define at least one when condition", label)
	}
	if !hasEffect(rule) {
		return fmt.Errorf("%s must define at least one requirement or action", label)
	}

	for _, kind := range rule.When.Mutations {
		if _, ok := supportedMutationKinds[kind]; !ok {
			return fmt.Errorf("%s has unsupported mutation kind %q", label, kind)
		}
	}
	for _, criticality := range rule.When.Criticalities {
		if _, ok := supportedCriticalities[criticality]; !ok {
			return fmt.Errorf("%s has unsupported criticality %q", label, criticality)
		}
	}
	if rule.When.RiskScoreGTE < 0 || rule.When.RiskScoreGTE > 100 {
		return fmt.Errorf("%s risk_score_gte must be between 0 and 100", label)
	}

	if err := requireNonEmpty(label, "path", rule.When.Paths); err != nil {
		return err
	}
	if err := requireNonEmpty(label, "ownership team", rule.When.OwnershipTeams); err != nil {
		return err
	}
	if err := requireNonEmpty(label, "reviewer", rule.Require.Reviewers); err != nil {
		return err
	}
	if err := requireNonEmpty(label, "evidence name", rule.Require.Evidence); err != nil {
		return err
	}
	if err := requireNonEmpty(label, "deployment environment", rule.Require.Deployment.Environments); err != nil {
		return err
	}

	strategy := rule.Require.Deployment.Strategy
	if strategy != "" {
		if _, ok := supportedDeploymentStrategies[strategy]; !ok {
			return fmt.Errorf("%s has unsupported deployment strategy %q", label, strategy)
		}
	}
	if lane := rule.Action.MinimumLane; lane != "" {
		if _, ok := supportedLanes[lane]; !ok {
			return fmt.Errorf("%s has unsupported minimum lane %q", label, lane)
		}
	}

	return nil
}

func hasCondition(when WhenClause) bool {
	return len(when.Mutations) > 0 || len(when.Paths) > 0 || len(when.Criticalities) > 0 ||
		when.RiskScoreGTE != 0 || len(when.OwnershipTeams) > 0
}

func hasEffect(rule RuleConfig) bool {
	deployment := rule.Require.Deployment
	return len(rule.Require.Reviewers) > 0 || len(rule.Require.Evidence) > 0 ||
		deployment.Strategy != "" || deployment.RequiresCanary || deployment.RequiresRollbackPlan ||
		len(deployment.Environments) > 0 || rule.Action.Block || rule.Action.MinimumLane != ""
}

func requireNonEmpty(label, field string, values []string) error {
	for index, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s %s at index %d must not be empty", label, field, index)
		}
	}
	return nil
}
