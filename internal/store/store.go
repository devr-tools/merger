package store

import (
	"context"
	"errors"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/events"
)

var ErrChangePacketNotFound = errors.New("change packet not found")

type ChangePacketStore interface {
	SaveChangePacket(context.Context, domain.ChangePacket) error
	GetChangePacket(context.Context, string) (domain.ChangePacket, error)
	FindLatestChangePacket(context.Context, string, int, string) (domain.ChangePacket, error)
	ListChangePackets(context.Context, int) ([]domain.ChangePacket, error)
}

type EventStore interface {
	SaveEvent(context.Context, events.Envelope) error
}

type EvidenceExecutionStore interface {
	UpsertEvidenceExecution(context.Context, domain.EvidenceExecution) error
	ListEvidenceExecutions(context.Context, string) ([]domain.EvidenceExecution, error)
	RecordEvidenceUpdate(context.Context, domain.EvidenceExecution, domain.EvidenceAuditEntry) error
	ListEvidenceAuditEntries(context.Context, string, int) ([]domain.EvidenceAuditEntry, error)
}

type DeploymentOutcomeStore interface {
	SaveDeploymentOutcome(context.Context, domain.DeploymentOutcome) error
	ListDeploymentOutcomes(context.Context, int) ([]domain.DeploymentOutcome, error)
}

type Repository interface {
	ChangePacketStore
	EventStore
	EvidenceExecutionStore
	DeploymentOutcomeStore
	Migrate(context.Context) error
	Close() error
}
