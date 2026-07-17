package policy

import (
	"context"
	"time"

	"github.com/devr-tools/merger/internal/domain"
)

type Evaluation struct {
	Decision   domain.PolicyDecision
	Evidence   []domain.EvidenceRequirement
	Reviewers  []domain.ReviewerRequirement
	Deployment domain.DeploymentRequirement
}

type Engine interface {
	Evaluate(context.Context, domain.ChangePacket) (Evaluation, error)
}

type RuleEngine struct {
	config Config
}

func NewRuleEngine(config Config) *RuleEngine {
	return &RuleEngine{config: config}
}

func (e *RuleEngine) Evaluate(_ context.Context, packet domain.ChangePacket) (Evaluation, error) {
	if err := Validate(e.config); err != nil {
		return Evaluation{}, err
	}

	evaluation := Evaluation{
		Decision: domain.PolicyDecision{
			Status:    domain.DecisionApproved,
			DecidedAt: time.Now().UTC(),
		},
		Deployment: domain.DeploymentRequirement{
			Strategy: domain.DeployDirect,
		},
	}

	for _, rule := range e.config.Policies {
		if !matchesRule(rule, packet) {
			continue
		}
		applyRule(&evaluation, rule)
	}

	finalizeDecision(&evaluation)
	return evaluation, nil
}
