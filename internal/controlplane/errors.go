package controlplane

import (
	"fmt"

	"github.com/devr-tools/merger/internal/domain"
)

type EvidenceValidationError struct {
	Field   string
	Message string
}

func (e *EvidenceValidationError) Error() string {
	return fmt.Sprintf("invalid evidence %s: %s", e.Field, e.Message)
}

type EvidenceNotFoundError struct {
	ChangePacketID string
	Name           string
}

func (e *EvidenceNotFoundError) Error() string {
	return fmt.Sprintf("evidence %q is not required by change packet %q", e.Name, e.ChangePacketID)
}

type EvidenceTransitionError struct {
	Name string
	From domain.EvidenceStatus
	To   domain.EvidenceStatus
}

func (e *EvidenceTransitionError) Error() string {
	return fmt.Sprintf("evidence %q cannot transition from %q to %q", e.Name, e.From, e.To)
}
