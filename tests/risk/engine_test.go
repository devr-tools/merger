package risk_test

import (
	"context"
	"testing"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/risk"
)

func TestDefaultEngineScoresDataAccessMutation(t *testing.T) {
	packet := domain.ChangePacket{
		Mutations: []domain.Mutation{{
			Kind:     domain.MutationDataAccess,
			Severity: domain.SeverityMedium,
			Files:    []string{"internal/store/orders.go"},
		}},
	}

	summary, risks, err := risk.DefaultEngine().Evaluate(context.Background(), packet)
	if err != nil {
		t.Fatalf("evaluate risk: %v", err)
	}

	if summary.Score != 24 {
		t.Fatalf("expected data-access risk score 24, got %d", summary.Score)
	}
	if len(summary.Contributors) != 1 || summary.Contributors[0] != domain.RiskRuntime {
		t.Fatalf("expected runtime contributor, got %#v", summary.Contributors)
	}
	if len(risks) != 1 {
		t.Fatalf("expected one risk, got %#v", risks)
	}
	if risks[0].Type != domain.RiskRuntime {
		t.Fatalf("expected runtime risk classification, got %q", risks[0].Type)
	}
	if risks[0].Summary != "data access behavior changed" {
		t.Fatalf("expected explicit data-access summary, got %q", risks[0].Summary)
	}
	if len(risks[0].Mitigations) != 2 || risks[0].Mitigations[1] != "data consistency validation" {
		t.Fatalf("expected data-access mitigations, got %#v", risks[0].Mitigations)
	}
}

func TestDefaultEngineIncludesConflictRisk(t *testing.T) {
	packet := domain.ChangePacket{Conflict: domain.ConflictAssessment{
		Score:       25,
		Findings:    []domain.ConflictFinding{{Kind: "base_drift", Paths: []string{".github/workflows/release.yml"}}},
		Mitigations: []string{"update the change"},
	}}

	summary, risks, err := risk.DefaultEngine().Evaluate(context.Background(), packet)
	if err != nil {
		t.Fatalf("evaluate risk: %v", err)
	}
	if summary.Score != 25 || len(risks) != 1 || risks[0].Type != domain.RiskConflict {
		t.Fatalf("expected conflict risk, got summary=%#v risks=%#v", summary, risks)
	}
}
