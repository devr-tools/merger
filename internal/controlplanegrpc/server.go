package controlplanegrpc

import (
	"context"
	"errors"

	"github.com/devr-tools/merger/internal/controlplane"
	"github.com/devr-tools/merger/internal/store"
	mergerv1 "github.com/devr-tools/merger/proto/merger/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	mergerv1.UnimplementedChangeControlServiceServer
	service *controlplane.Service
}

func NewServer(service *controlplane.Service) *Server {
	return &Server{service: service}
}

func (s *Server) GetChangePacket(ctx context.Context, request *mergerv1.GetChangePacketRequest) (*mergerv1.GetChangePacketResponse, error) {
	if request.GetRef().GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "change packet id is required")
	}

	view, err := s.service.GetChangePacket(ctx, request.GetRef().GetId())
	if err != nil {
		return nil, statusForError(err)
	}

	return toGetChangePacketResponse(view), nil
}

func (s *Server) ListChangePackets(ctx context.Context, request *mergerv1.ListChangePacketsRequest) (*mergerv1.ListChangePacketsResponse, error) {
	limit := int(request.GetLimit())
	if limit <= 0 {
		limit = controlplane.DefaultListLimit
	}
	if limit > controlplane.MaxListLimit {
		limit = controlplane.MaxListLimit
	}

	items, err := s.service.ListChangePackets(ctx, limit)
	if err != nil {
		return nil, statusForError(err)
	}

	return toListChangePacketsResponse(items), nil
}

func (s *Server) UpdateEvidenceExecution(ctx context.Context, request *mergerv1.UpdateEvidenceExecutionRequest) (*mergerv1.UpdateEvidenceExecutionResponse, error) {
	if request.GetChangePacketId() == "" || request.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "change packet id and evidence name are required")
	}

	execution, err := s.service.UpdateEvidenceExecution(ctx, fromUpdateEvidenceExecutionRequest(request))
	if err != nil {
		return nil, statusForError(err)
	}

	return toUpdateEvidenceExecutionResponse(execution), nil
}

func statusForError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrChangePacketNotFound) {
		return status.Error(codes.NotFound, err.Error())
	}
	var validationError *controlplane.EvidenceValidationError
	if errors.As(err, &validationError) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	var notFoundError *controlplane.EvidenceNotFoundError
	if errors.As(err, &notFoundError) {
		return status.Error(codes.NotFound, err.Error())
	}
	var transitionError *controlplane.EvidenceTransitionError
	if errors.As(err, &transitionError) {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}
