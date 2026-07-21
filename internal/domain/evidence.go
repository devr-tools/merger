package domain

import "time"

type EvidenceStatus string

const (
	EvidencePending   EvidenceStatus = "pending"
	EvidenceRunning   EvidenceStatus = "running"
	EvidenceSatisfied EvidenceStatus = "satisfied"
	EvidenceFailed    EvidenceStatus = "failed"
	EvidenceWaived    EvidenceStatus = "waived"
)

type EvidenceExecution struct {
	ChangePacketID string            `json:"changePacketId"`
	Name           string            `json:"name"`
	Type           EvidenceType      `json:"type"`
	Status         EvidenceStatus    `json:"status"`
	Required       bool              `json:"required"`
	Summary        string            `json:"summary,omitempty"`
	DetailsURL     string            `json:"detailsUrl,omitempty"`
	UpdatedBy      string            `json:"updatedBy,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	UpdatedAt      time.Time         `json:"updatedAt"`
}

// EvidenceAuditEntry is an immutable record of an evidence state transition.
// The execution remains the current snapshot; audit entries retain the actor,
// provenance, and context that produced every historical state.
type EvidenceAuditEntry struct {
	ID             string            `json:"id"`
	ChangePacketID string            `json:"changePacketId"`
	EvidenceName   string            `json:"evidenceName"`
	FromStatus     EvidenceStatus    `json:"fromStatus"`
	ToStatus       EvidenceStatus    `json:"toStatus"`
	Actor          string            `json:"actor,omitempty"`
	Summary        string            `json:"summary,omitempty"`
	DetailsURL     string            `json:"detailsUrl,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	OccurredAt     time.Time         `json:"occurredAt"`
}
