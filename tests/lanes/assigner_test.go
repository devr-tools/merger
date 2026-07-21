package lanes_test

import (
	"context"
	"testing"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/lanes"
)

func TestAssignerReturnsGreenForLowRiskAutomatableChanges(t *testing.T) {
	assigner := lanes.NewAssigner(lanes.Config{GreenMax: 20, YellowMax: 55, RedMax: 85})

	lane, err := assigner.Assign(context.Background(), domain.ChangePacket{
		RiskSummary: domain.RiskSummary{Score: 10},
		Decision: domain.PolicyDecision{
			Status: domain.DecisionApproved,
		},
		Deployment: domain.DeploymentRequirement{
			Strategy: domain.DeployDirect,
		},
	})
	if err != nil {
		t.Fatalf("assign lane: %v", err)
	}
	if lane != domain.MergeLaneGreen {
		t.Fatalf("expected GREEN, got %s", lane)
	}
}

func TestAssignerEscalatesPendingDecisionToRed(t *testing.T) {
	assigner := lanes.NewAssigner(lanes.Config{GreenMax: 20, YellowMax: 55, RedMax: 85})

	lane, err := assigner.Assign(context.Background(), domain.ChangePacket{
		RiskSummary: domain.RiskSummary{Score: 10},
		Decision: domain.PolicyDecision{
			Status: domain.DecisionPending,
		},
		Deployment: domain.DeploymentRequirement{
			Strategy: domain.DeployDirect,
		},
	})
	if err != nil {
		t.Fatalf("assign lane: %v", err)
	}
	if lane != domain.MergeLaneRed {
		t.Fatalf("expected pending decision to produce RED, got %s", lane)
	}
}

func TestAssignerEscalatesEscalatedDecisionToRed(t *testing.T) {
	assigner := lanes.NewAssigner(lanes.Config{GreenMax: 20, YellowMax: 55, RedMax: 85})

	lane, err := assigner.Assign(context.Background(), domain.ChangePacket{
		RiskSummary: domain.RiskSummary{Score: 10},
		Decision: domain.PolicyDecision{
			Status: domain.DecisionEscalated,
		},
	})
	if err != nil {
		t.Fatalf("assign lane: %v", err)
	}
	if lane != domain.MergeLaneRed {
		t.Fatalf("expected escalated decision to produce RED, got %s", lane)
	}
}

func TestAssignerPreservesStricterPendingMinimumLane(t *testing.T) {
	assigner := lanes.NewAssigner(lanes.Config{GreenMax: 20, YellowMax: 55, RedMax: 85})

	lane, err := assigner.Assign(context.Background(), domain.ChangePacket{
		RiskSummary: domain.RiskSummary{Score: 10},
		Decision: domain.PolicyDecision{
			Status:      domain.DecisionPending,
			MinimumLane: domain.MergeLaneBlack,
		},
	})
	if err != nil {
		t.Fatalf("assign lane: %v", err)
	}
	if lane != domain.MergeLaneBlack {
		t.Fatalf("expected BLACK policy minimum lane to be preserved, got %s", lane)
	}
}
