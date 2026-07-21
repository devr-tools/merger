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
	evidenceBindings := make(map[string]GitHubCheckBinding)
	checkBindings := make(map[githubCheckIdentity]string)
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
		for _, binding := range rule.Require.GitHubChecks {
			if existing, ok := evidenceBindings[binding.Evidence]; ok && existing != binding {
				return fmt.Errorf("evidence %q has conflicting GitHub check bindings", binding.Evidence)
			}
			evidenceBindings[binding.Evidence] = binding
			identity := githubCheckIdentity{Name: binding.Name, AppID: binding.AppID}
			if evidence, ok := checkBindings[identity]; ok && evidence != binding.Evidence {
				return fmt.Errorf("GitHub check %q from app %d is bound to both evidence %q and %q", binding.Name, binding.AppID, evidence, binding.Evidence)
			}
			checkBindings[identity] = binding.Evidence
		}
	}
	return nil
}

type githubCheckIdentity struct {
	Name  string
	AppID int64
}

func validateRule(label string, rule RuleConfig) error {
	if !hasCondition(rule.When) {
		return fmt.Errorf("%s must define at least one when condition", label)
	}
	if !hasEffect(rule) {
		return fmt.Errorf("%s must define at least one requirement or action", label)
	}

	if err := validateWhenClause(label, rule.When); err != nil {
		return err
	}
	if err := validateRequirements(label, rule.Require); err != nil {
		return err
	}
	if err := validateAction(label, rule.Action); err != nil {
		return err
	}
	return nil
}

func validateWhenClause(label string, when WhenClause) error {
	for _, kind := range when.Mutations {
		if _, ok := supportedMutationKinds[kind]; !ok {
			return fmt.Errorf("%s has unsupported mutation kind %q", label, kind)
		}
	}
	for _, criticality := range when.Criticalities {
		if _, ok := supportedCriticalities[criticality]; !ok {
			return fmt.Errorf("%s has unsupported criticality %q", label, criticality)
		}
	}
	if when.RiskScoreGTE < 0 || when.RiskScoreGTE > 100 {
		return fmt.Errorf("%s risk_score_gte must be between 0 and 100", label)
	}
	if err := requireNonEmpty(label, "path", when.Paths); err != nil {
		return err
	}
	return requireNonEmpty(label, "ownership team", when.OwnershipTeams)
}

func validateRequirements(label string, require RequirementClause) error {
	if err := requireNonEmpty(label, "reviewer", require.Reviewers); err != nil {
		return err
	}
	if err := requireNonEmpty(label, "evidence name", require.Evidence); err != nil {
		return err
	}
	declaredEvidence := make(map[string]struct{}, len(require.Evidence))
	for _, evidence := range require.Evidence {
		declaredEvidence[evidence] = struct{}{}
	}
	boundEvidence := make(map[string]struct{}, len(require.GitHubChecks))
	for index, binding := range require.GitHubChecks {
		bindingLabel := fmt.Sprintf("%s GitHub check binding at index %d", label, index)
		if strings.TrimSpace(binding.Evidence) == "" || strings.TrimSpace(binding.Name) == "" {
			return fmt.Errorf("%s must define evidence and name", bindingLabel)
		}
		if binding.AppID <= 0 {
			return fmt.Errorf("%s app_id must be a positive integer", bindingLabel)
		}
		if _, declared := declaredEvidence[binding.Evidence]; !declared {
			return fmt.Errorf("%s references undeclared evidence %q", bindingLabel, binding.Evidence)
		}
		if _, duplicate := boundEvidence[binding.Evidence]; duplicate {
			return fmt.Errorf("%s duplicates binding for evidence %q", label, binding.Evidence)
		}
		boundEvidence[binding.Evidence] = struct{}{}
	}
	if err := requireNonEmpty(label, "deployment environment", require.Deployment.Environments); err != nil {
		return err
	}
	strategy := require.Deployment.Strategy
	if strategy == "" {
		return nil
	}
	if _, ok := supportedDeploymentStrategies[strategy]; !ok {
		return fmt.Errorf("%s has unsupported deployment strategy %q", label, strategy)
	}
	return nil
}

func validateAction(label string, action ActionClause) error {
	lane := action.MinimumLane
	if lane == "" {
		return nil
	}
	if _, ok := supportedLanes[lane]; !ok {
		return fmt.Errorf("%s has unsupported minimum lane %q", label, lane)
	}
	return nil
}

func hasCondition(when WhenClause) bool {
	return len(when.Mutations) > 0 || len(when.Paths) > 0 || len(when.Criticalities) > 0 ||
		when.RiskScoreGTE != 0 || len(when.OwnershipTeams) > 0
}

func hasEffect(rule RuleConfig) bool {
	deployment := rule.Require.Deployment
	return len(rule.Require.Reviewers) > 0 || len(rule.Require.Evidence) > 0 || len(rule.Require.GitHubChecks) > 0 ||
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
