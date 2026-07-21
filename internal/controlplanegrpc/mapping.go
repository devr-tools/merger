package controlplanegrpc

import (
	"time"

	"github.com/devr-tools/merger/internal/controlplane"
	"github.com/devr-tools/merger/internal/domain"
	mergerv1 "github.com/devr-tools/merger/proto/merger/v1"
)

func toGetChangePacketResponse(view controlplane.ChangePacketView) *mergerv1.GetChangePacketResponse {
	return &mergerv1.GetChangePacketResponse{
		Id:        view.Packet.ID,
		MergeLane: string(view.Packet.MergeLane),
		RiskScore: int32(view.Packet.RiskSummary.Score),
		Evidence:  toEvidenceExecutions(view.Evidence),
	}
}

func toListChangePacketsResponse(items []domain.ChangePacket) *mergerv1.ListChangePacketsResponse {
	response := &mergerv1.ListChangePacketsResponse{
		Items: make([]*mergerv1.ChangePacketSummary, 0, len(items)),
	}

	for _, item := range items {
		response.Items = append(response.Items, &mergerv1.ChangePacketSummary{
			Id:        item.ID,
			Repo:      item.Repo.FullName,
			PrNumber:  int32(item.PR.Number),
			MergeLane: string(item.MergeLane),
			RiskScore: int32(item.RiskSummary.Score),
		})
	}

	return response
}

func fromUpdateEvidenceExecutionRequest(request *mergerv1.UpdateEvidenceExecutionRequest) domain.EvidenceExecution {
	return domain.EvidenceExecution{
		ChangePacketID: request.GetChangePacketId(),
		Name:           request.GetName(),
		Type:           domain.EvidenceType(request.GetType()),
		Status:         domain.EvidenceStatus(request.GetStatus()),
		Required:       request.GetRequired(),
		Summary:        request.GetSummary(),
		DetailsURL:     request.GetDetailsUrl(),
		UpdatedBy:      request.GetUpdatedBy(),
	}
}

func toUpdateEvidenceExecutionResponse(execution domain.EvidenceExecution) *mergerv1.UpdateEvidenceExecutionResponse {
	return &mergerv1.UpdateEvidenceExecutionResponse{
		Evidence: toEvidenceExecution(execution),
	}
}

func toEvidenceExecutions(items []domain.EvidenceExecution) []*mergerv1.EvidenceExecution {
	result := make([]*mergerv1.EvidenceExecution, 0, len(items))
	for _, item := range items {
		result = append(result, toEvidenceExecution(item))
	}
	return result
}

func toEvidenceExecution(item domain.EvidenceExecution) *mergerv1.EvidenceExecution {
	return &mergerv1.EvidenceExecution{
		ChangePacketId: item.ChangePacketID,
		Name:           item.Name,
		Type:           string(item.Type),
		Status:         string(item.Status),
		Required:       item.Required,
		Summary:        item.Summary,
		DetailsUrl:     item.DetailsURL,
		UpdatedBy:      item.UpdatedBy,
		UpdatedAt:      formatTime(item.UpdatedAt),
	}
}

func toListEvidenceAuditEntriesResponse(items []domain.EvidenceAuditEntry) *mergerv1.ListEvidenceAuditEntriesResponse {
	response := &mergerv1.ListEvidenceAuditEntriesResponse{Items: make([]*mergerv1.EvidenceAuditEntry, 0, len(items))}
	for _, item := range items {
		response.Items = append(response.Items, toEvidenceAuditEntry(item))
	}
	return response
}

func toEvidenceAuditEntry(item domain.EvidenceAuditEntry) *mergerv1.EvidenceAuditEntry {
	return &mergerv1.EvidenceAuditEntry{
		Id:             item.ID,
		ChangePacketId: item.ChangePacketID,
		EvidenceName:   item.EvidenceName,
		FromStatus:     string(item.FromStatus),
		ToStatus:       string(item.ToStatus),
		Actor:          item.Actor,
		Summary:        item.Summary,
		DetailsUrl:     item.DetailsURL,
		Metadata:       item.Metadata,
		OccurredAt:     formatTime(item.OccurredAt),
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
