package controlplane_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devr-tools/merger/internal/access"
	"github.com/devr-tools/merger/internal/controlplane"
	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/store"
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

func TestServiceDerivesEvidenceActorFromAuthenticatedPrincipal(t *testing.T) {
	repo := seedRepository(t)
	service := controlplane.NewService(repo)
	ctx := access.WithPrincipal(context.Background(), access.Principal{
		Subject: "ci-workflow",
		Roles:   []access.Role{access.RoleEvidenceWriter},
	})

	execution, err := service.UpdateEvidenceExecution(ctx, domain.EvidenceExecution{
		ChangePacketID: "cp_1",
		Name:           "integration_tests",
		Status:         domain.EvidenceWaived,
		UpdatedBy:      "spoofed-actor",
	})
	if err != nil {
		t.Fatalf("update evidence with authenticated principal: %v", err)
	}
	if execution.UpdatedBy != "ci-workflow" {
		t.Fatalf("expected authenticated subject, got %q", execution.UpdatedBy)
	}

	executions, err := repo.ListEvidenceExecutions(context.Background(), "cp_1")
	if err != nil {
		t.Fatalf("list evidence executions: %v", err)
	}
	if len(executions) != 1 || executions[0].UpdatedBy != "ci-workflow" {
		t.Fatalf("expected persisted authenticated subject, got %+v", executions)
	}
}

func TestServiceReconcilesEvidenceDecisionLaneAndCheck(t *testing.T) {
	repo := reconciliationRepository(t, nil)
	assigner := &stubLaneAssigner{lane: domain.MergeLaneGreen}
	publisher := &stubEvidencePublisher{}
	service := controlplane.NewServiceWithOptions(
		repo,
		controlplane.WithLaneAssigner(assigner),
		controlplane.WithCheckPublisher(publisher),
	)

	before := time.Now().UTC()
	_, err := service.UpdateEvidenceExecution(context.Background(), domain.EvidenceExecution{
		ChangePacketID: "cp_reconcile",
		Name:           "integration_tests",
		Status:         domain.EvidenceSatisfied,
		UpdatedBy:      "ci",
	})
	if err != nil {
		t.Fatalf("update evidence execution: %v", err)
	}

	packet, err := repo.GetChangePacket(context.Background(), "cp_reconcile")
	if err != nil {
		t.Fatalf("get reconciled packet: %v", err)
	}
	if packet.Decision.Status != domain.DecisionApproved {
		t.Fatalf("expected approved decision, got %q", packet.Decision.Status)
	}
	if packet.MergeLane != domain.MergeLaneGreen {
		t.Fatalf("expected recomputed GREEN lane, got %q", packet.MergeLane)
	}
	if packet.Decision.DecidedAt.Before(before) || packet.UpdatedAt.Before(before) {
		t.Fatalf("expected reconciliation timestamps to advance: decided=%s updated=%s", packet.Decision.DecidedAt, packet.UpdatedAt)
	}
	if assigner.packet.Decision.Status != domain.DecisionApproved {
		t.Fatalf("expected assigner to receive approved packet, got %q", assigner.packet.Decision.Status)
	}
	if len(publisher.executions) != 1 || publisher.executions[0].Status != domain.EvidenceSatisfied {
		t.Fatalf("expected refreshed check with satisfied evidence, got %+v", publisher.executions)
	}
	if publisher.packet.MergeLane != domain.MergeLaneGreen {
		t.Fatalf("expected refreshed check to receive GREEN lane, got %q", publisher.packet.MergeLane)
	}
}

func TestServiceKeepsDecisionPendingForOutstandingRequirementsOrMandatoryReviewers(t *testing.T) {
	tests := []struct {
		name      string
		evidence  []domain.EvidenceRequirement
		reviewers []domain.ReviewerRequirement
	}{
		{
			name: "outstanding evidence",
			evidence: []domain.EvidenceRequirement{
				{Name: "integration_tests", Type: domain.EvidenceIntegrationTests, Required: true},
				{Name: "security_review", Type: domain.EvidenceSecurityReview, Required: true},
			},
		},
		{
			name:      "mandatory reviewer",
			evidence:  []domain.EvidenceRequirement{{Name: "integration_tests", Type: domain.EvidenceIntegrationTests, Required: true}},
			reviewers: []domain.ReviewerRequirement{{Team: "security", Mandatory: true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := reconciliationRepository(t, func(packet *domain.ChangePacket) {
				packet.Evidence = tt.evidence
				packet.Reviewers = tt.reviewers
			})
			service := controlplane.NewService(repo)

			_, err := service.UpdateEvidenceExecution(context.Background(), domain.EvidenceExecution{
				ChangePacketID: "cp_reconcile",
				Name:           "integration_tests",
				Status:         domain.EvidenceSatisfied,
			})
			if err != nil {
				t.Fatalf("update evidence execution: %v", err)
			}

			packet, err := repo.GetChangePacket(context.Background(), "cp_reconcile")
			if err != nil {
				t.Fatalf("get reconciled packet: %v", err)
			}
			if packet.Decision.Status != domain.DecisionPending {
				t.Fatalf("expected pending decision, got %q", packet.Decision.Status)
			}
		})
	}
}

func TestServiceReconcilesRequiredEvidenceStatuses(t *testing.T) {
	tests := []struct {
		status       domain.EvidenceStatus
		updatedBy    string
		wantDecision domain.DecisionStatus
	}{
		{status: domain.EvidencePending, wantDecision: domain.DecisionPending},
		{status: domain.EvidenceRunning, wantDecision: domain.DecisionPending},
		{status: domain.EvidenceFailed, wantDecision: domain.DecisionPending},
		{status: domain.EvidenceSatisfied, wantDecision: domain.DecisionApproved},
		{status: domain.EvidenceWaived, updatedBy: "release-manager", wantDecision: domain.DecisionApproved},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			repo := reconciliationRepository(t, nil)
			service := controlplane.NewService(repo)

			_, err := service.UpdateEvidenceExecution(context.Background(), domain.EvidenceExecution{
				ChangePacketID: "cp_reconcile",
				Name:           "integration_tests",
				Status:         tt.status,
				UpdatedBy:      tt.updatedBy,
			})
			if err != nil {
				t.Fatalf("update evidence execution: %v", err)
			}

			packet, err := repo.GetChangePacket(context.Background(), "cp_reconcile")
			if err != nil {
				t.Fatalf("get reconciled packet: %v", err)
			}
			if packet.Decision.Status != tt.wantDecision {
				t.Fatalf("expected %q decision, got %q", tt.wantDecision, packet.Decision.Status)
			}
		})
	}
}

func TestServicePreservesBlockedDecisionDuringEvidenceReconciliation(t *testing.T) {
	repo := reconciliationRepository(t, func(packet *domain.ChangePacket) {
		packet.Decision.Status = domain.DecisionBlocked
		packet.Decision.DecidedAt = time.Unix(100, 0).UTC()
	})
	service := controlplane.NewService(repo)

	_, err := service.UpdateEvidenceExecution(context.Background(), domain.EvidenceExecution{
		ChangePacketID: "cp_reconcile",
		Name:           "integration_tests",
		Status:         domain.EvidenceSatisfied,
	})
	if err != nil {
		t.Fatalf("update evidence execution: %v", err)
	}

	packet, err := repo.GetChangePacket(context.Background(), "cp_reconcile")
	if err != nil {
		t.Fatalf("get reconciled packet: %v", err)
	}
	if packet.Decision.Status != domain.DecisionBlocked {
		t.Fatalf("expected blocked decision to be preserved, got %q", packet.Decision.Status)
	}
	if !packet.Decision.DecidedAt.Equal(time.Unix(100, 0).UTC()) {
		t.Fatalf("expected blocked decision timestamp to be preserved, got %s", packet.Decision.DecidedAt)
	}
}

func reconciliationRepository(t *testing.T, customize func(*domain.ChangePacket)) *store.MemoryRepository {
	t.Helper()
	repo := store.NewMemoryRepository()
	packet := domain.ChangePacket{
		ID:        "cp_reconcile",
		Repo:      domain.RepoRef{Owner: "acme", Name: "repo", FullName: "acme/repo"},
		PR:        domain.PullRequestRef{Number: 42, HeadSHA: "abc123"},
		MergeLane: domain.MergeLaneYellow,
		Decision:  domain.PolicyDecision{Status: domain.DecisionPending},
		Evidence: []domain.EvidenceRequirement{
			{Name: "integration_tests", Type: domain.EvidenceIntegrationTests, Required: true},
		},
		UpdatedAt: time.Unix(1, 0).UTC(),
	}
	if customize != nil {
		customize(&packet)
	}
	if err := repo.SaveChangePacket(context.Background(), packet); err != nil {
		t.Fatalf("seed reconciliation packet: %v", err)
	}
	return repo
}

type stubLaneAssigner struct {
	lane   domain.MergeLane
	packet domain.ChangePacket
}

func (s *stubLaneAssigner) Assign(_ context.Context, packet domain.ChangePacket) (domain.MergeLane, error) {
	s.packet = packet
	return s.lane, nil
}

type stubEvidencePublisher struct {
	packet     domain.ChangePacket
	executions []domain.EvidenceExecution
}

func (s *stubEvidencePublisher) PublishWithEvidence(_ context.Context, packet domain.ChangePacket, executions []domain.EvidenceExecution) error {
	s.packet = packet
	s.executions = append([]domain.EvidenceExecution(nil), executions...)
	return nil
}
