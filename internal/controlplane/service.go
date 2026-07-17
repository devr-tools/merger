package controlplane

import (
	"context"
	"strings"
	"time"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/store"
)

type Service struct {
	repository store.Repository
}

type ChangePacketView struct {
	Packet   domain.ChangePacket        `json:"packet"`
	Evidence []domain.EvidenceExecution `json:"evidence"`
}

func NewService(repository store.Repository) *Service {
	return &Service{repository: repository}
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
	return ChangePacketView{Packet: packet, Evidence: evidence}, nil
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
	if err := s.repository.UpsertEvidenceExecution(ctx, execution); err != nil {
		return domain.EvidenceExecution{}, err
	}
	return execution, nil
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
