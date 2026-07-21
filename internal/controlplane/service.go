package controlplane

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devr-tools/merger/internal/access"
	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/lanes"
	"github.com/devr-tools/merger/internal/store"
	"github.com/devr-tools/merger/pkg/identity"
)

type CheckPublisher interface {
	PublishWithEvidence(context.Context, domain.ChangePacket, []domain.EvidenceExecution) error
}

type Option func(*Service)

type Service struct {
	repository     store.Repository
	laneAssigner   lanes.Assigner
	checkPublisher CheckPublisher
}

type ChangePacketView struct {
	Packet   domain.ChangePacket         `json:"packet"`
	Evidence []domain.EvidenceExecution  `json:"evidence"`
	Audit    []domain.EvidenceAuditEntry `json:"audit"`
}

func NewService(repository store.Repository) *Service {
	return NewServiceWithOptions(repository)
}

func NewServiceWithOptions(repository store.Repository, options ...Option) *Service {
	service := &Service{repository: repository}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func WithLaneAssigner(assigner lanes.Assigner) Option {
	return func(service *Service) {
		service.laneAssigner = assigner
	}
}

func WithCheckPublisher(publisher CheckPublisher) Option {
	return func(service *Service) {
		service.checkPublisher = publisher
	}
}

func (s *Service) GetChangePacket(ctx context.Context, id string) (ChangePacketView, error) {
	packet, err := s.repository.GetChangePacket(ctx, id)
	if err != nil {
		return ChangePacketView{}, err
	}
	evidence, err := s.repository.ListEvidenceExecutions(ctx, id)
	if err != nil {
		return ChangePacketView{}, err
	}
	audit, err := s.repository.ListEvidenceAuditEntries(ctx, id, DefaultListLimit)
	if err != nil {
		return ChangePacketView{}, err
	}
	return ChangePacketView{Packet: packet, Evidence: evidence, Audit: audit}, nil
}

func (s *Service) ListEvidenceAuditEntries(ctx context.Context, changePacketID string, limit int) ([]domain.EvidenceAuditEntry, error) {
	if _, err := s.repository.GetChangePacket(ctx, changePacketID); err != nil {
		return nil, err
	}
	return s.repository.ListEvidenceAuditEntries(ctx, changePacketID, limit)
}

func (s *Service) ListChangePackets(ctx context.Context, limit int) ([]domain.ChangePacket, error) {
	return s.repository.ListChangePackets(ctx, limit)
}

func (s *Service) UpdateEvidenceExecution(ctx context.Context, execution domain.EvidenceExecution) (domain.EvidenceExecution, error) {
	if strings.TrimSpace(execution.ChangePacketID) == "" {
		return domain.EvidenceExecution{}, &EvidenceValidationError{Field: "changePacketId", Message: "is required"}
	}
	if strings.TrimSpace(execution.Name) == "" {
		return domain.EvidenceExecution{}, &EvidenceValidationError{Field: "name", Message: "is required"}
	}
	if !validEvidenceStatus(execution.Status) {
		return domain.EvidenceExecution{}, &EvidenceValidationError{Field: "status", Message: "must be one of pending, running, satisfied, failed, or waived"}
	}
	if principal, ok := access.PrincipalFromContext(ctx); ok {
		execution.UpdatedBy = principal.Subject
	}
	if execution.Status == domain.EvidenceWaived && strings.TrimSpace(execution.UpdatedBy) == "" {
		return domain.EvidenceExecution{}, &EvidenceValidationError{Field: "updatedBy", Message: "is required when waiving evidence"}
	}

	packet, err := s.repository.GetChangePacket(ctx, execution.ChangePacketID)
	if err != nil {
		return domain.EvidenceExecution{}, err
	}

	requirement, ok := findEvidenceRequirement(packet.Evidence, execution.Name)
	if !ok {
		return domain.EvidenceExecution{}, &EvidenceNotFoundError{
			ChangePacketID: execution.ChangePacketID,
			Name:           execution.Name,
		}
	}

	currentStatus, err := s.currentEvidenceStatus(ctx, execution.ChangePacketID, execution.Name)
	if err != nil {
		return domain.EvidenceExecution{}, err
	}
	if !canTransitionEvidence(currentStatus, execution.Status) {
		return domain.EvidenceExecution{}, &EvidenceTransitionError{
			Name: execution.Name,
			From: currentStatus,
			To:   execution.Status,
		}
	}

	execution.Type = requirement.Type
	execution.Required = requirement.Required
	execution.UpdatedAt = time.Now().UTC()
	audit := domain.EvidenceAuditEntry{
		ID:             identity.New("evidence_audit"),
		ChangePacketID: execution.ChangePacketID,
		EvidenceName:   execution.Name,
		FromStatus:     currentStatus,
		ToStatus:       execution.Status,
		Actor:          execution.UpdatedBy,
		Summary:        execution.Summary,
		DetailsURL:     execution.DetailsURL,
		Metadata:       execution.Metadata,
		OccurredAt:     execution.UpdatedAt,
	}
	if err := s.repository.RecordEvidenceUpdate(ctx, execution, audit); err != nil {
		return domain.EvidenceExecution{}, err
	}
	if err := s.reconcileDecision(ctx, &packet); err != nil {
		return domain.EvidenceExecution{}, err
	}
	return execution, nil
}

func (s *Service) reconcileDecision(ctx context.Context, packet *domain.ChangePacket) error {
	executions, err := s.repository.ListEvidenceExecutions(ctx, packet.ID)
	if err != nil {
		return fmt.Errorf("reload evidence executions: %w", err)
	}

	now := time.Now().UTC()
	if packet.Decision.Status != domain.DecisionBlocked {
		packet.Decision.Status = domain.DecisionPending
		if requiredEvidenceComplete(packet.Evidence, executions) && !hasMandatoryReviewers(packet.Reviewers) {
			packet.Decision.Status = domain.DecisionApproved
		}
		packet.Decision.DecidedAt = now
	}
	packet.UpdatedAt = now

	if s.laneAssigner != nil {
		lane, err := s.laneAssigner.Assign(ctx, *packet)
		if err != nil {
			return fmt.Errorf("recompute merge lane: %w", err)
		}
		packet.MergeLane = lane
	}

	if err := s.repository.SaveChangePacket(ctx, *packet); err != nil {
		return fmt.Errorf("persist reconciled change packet: %w", err)
	}
	if s.checkPublisher != nil {
		if err := s.checkPublisher.PublishWithEvidence(ctx, *packet, executions); err != nil {
			return fmt.Errorf("publish refreshed check: %w", err)
		}
	}
	return nil
}

func requiredEvidenceComplete(requirements []domain.EvidenceRequirement, executions []domain.EvidenceExecution) bool {
	statusByName := make(map[string]domain.EvidenceStatus, len(executions))
	for _, execution := range executions {
		statusByName[execution.Name] = execution.Status
	}
	for _, requirement := range requirements {
		if !requirement.Required {
			continue
		}
		status := statusByName[requirement.Name]
		if status != domain.EvidenceSatisfied && status != domain.EvidenceWaived {
			return false
		}
	}
	return true
}

func hasMandatoryReviewers(reviewers []domain.ReviewerRequirement) bool {
	for _, reviewer := range reviewers {
		if reviewer.Mandatory {
			return true
		}
	}
	return false
}

func findEvidenceRequirement(requirements []domain.EvidenceRequirement, name string) (domain.EvidenceRequirement, bool) {
	for _, requirement := range requirements {
		if requirement.Name == name {
			return requirement, true
		}
	}
	return domain.EvidenceRequirement{}, false
}

func (s *Service) currentEvidenceStatus(ctx context.Context, changePacketID, name string) (domain.EvidenceStatus, error) {
	executions, err := s.repository.ListEvidenceExecutions(ctx, changePacketID)
	if err != nil {
		return "", err
	}
	for _, execution := range executions {
		if execution.Name == name {
			return execution.Status, nil
		}
	}
	return domain.EvidencePending, nil
}

func validEvidenceStatus(status domain.EvidenceStatus) bool {
	switch status {
	case domain.EvidencePending,
		domain.EvidenceRunning,
		domain.EvidenceSatisfied,
		domain.EvidenceFailed,
		domain.EvidenceWaived:
		return true
	default:
		return false
	}
}

func canTransitionEvidence(from, to domain.EvidenceStatus) bool {
	if from == to {
		return true
	}

	switch from {
	case domain.EvidencePending:
		return to == domain.EvidenceRunning ||
			to == domain.EvidenceSatisfied ||
			to == domain.EvidenceFailed ||
			to == domain.EvidenceWaived
	case domain.EvidenceRunning:
		return to == domain.EvidenceSatisfied ||
			to == domain.EvidenceFailed ||
			to == domain.EvidenceWaived
	case domain.EvidenceFailed:
		return to == domain.EvidenceRunning || to == domain.EvidenceWaived
	case domain.EvidenceSatisfied, domain.EvidenceWaived:
		return false
	default:
		return false
	}
}
