package controlplane_test

import (
	"context"
	"errors"
	"testing"

	"github.com/devr-tools/merger/internal/controlplane"
	"github.com/devr-tools/merger/internal/domain"
)

func TestServiceEnforcesEvidenceLifecycle(t *testing.T) {
	repo := seedRepository(t)
	service := controlplane.NewService(repo)
	ctx := context.Background()

	for _, status := range []domain.EvidenceStatus{
		domain.EvidenceRunning,
		domain.EvidenceFailed,
		domain.EvidenceRunning,
		domain.EvidenceSatisfied,
	} {
		_, err := service.UpdateEvidenceExecution(ctx, domain.EvidenceExecution{
			ChangePacketID: "cp_1",
			Name:           "integration_tests",
			Status:         status,
		})
		if err != nil {
			t.Fatalf("transition evidence to %q: %v", status, err)
		}
	}

	_, err := service.UpdateEvidenceExecution(ctx, domain.EvidenceExecution{
		ChangePacketID: "cp_1",
		Name:           "integration_tests",
		Status:         domain.EvidenceRunning,
	})
	var transitionError *controlplane.EvidenceTransitionError
	if !errors.As(err, &transitionError) {
		t.Fatalf("expected typed transition error, got %v", err)
	}
	if transitionError.From != domain.EvidenceSatisfied || transitionError.To != domain.EvidenceRunning {
		t.Fatalf("unexpected transition error: %+v", transitionError)
	}
}

func TestServiceRequiresAttributedWaiver(t *testing.T) {
	repo := seedRepository(t)
	service := controlplane.NewService(repo)

	_, err := service.UpdateEvidenceExecution(context.Background(), domain.EvidenceExecution{
		ChangePacketID: "cp_1",
		Name:           "integration_tests",
		Status:         domain.EvidenceWaived,
		UpdatedBy:      "   ",
	})
	var validationError *controlplane.EvidenceValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("expected typed validation error, got %v", err)
	}
}
