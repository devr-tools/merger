package controlplane

import (
	"context"
	"time"

	"github.com/mergerhq/merger/internal/domain"
	"github.com/mergerhq/merger/internal/store"
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
	if execution.UpdatedAt.IsZero() {
		execution.UpdatedAt = time.Now().UTC()
	}
	if err := s.repository.UpsertEvidenceExecution(ctx, execution); err != nil {
		return domain.EvidenceExecution{}, err
	}
	return execution, nil
}
