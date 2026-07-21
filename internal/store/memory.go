package store

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/events"
)

type MemoryRepository struct {
	mu             sync.RWMutex
	changePackets  map[string]domain.ChangePacket
	events         map[string]events.Envelope
	evidenceByCPID map[string]map[string]domain.EvidenceExecution
	auditByCPID    map[string][]domain.EvidenceAuditEntry
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		changePackets:  make(map[string]domain.ChangePacket),
		events:         make(map[string]events.Envelope),
		evidenceByCPID: make(map[string]map[string]domain.EvidenceExecution),
		auditByCPID:    make(map[string][]domain.EvidenceAuditEntry),
	}
}

func (r *MemoryRepository) RecordEvidenceUpdate(_ context.Context, execution domain.EvidenceExecution, audit domain.EvidenceAuditEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.evidenceByCPID[execution.ChangePacketID] == nil {
		r.evidenceByCPID[execution.ChangePacketID] = make(map[string]domain.EvidenceExecution)
	}
	if execution.UpdatedAt.IsZero() {
		execution.UpdatedAt = time.Now().UTC()
	}
	r.evidenceByCPID[execution.ChangePacketID][execution.Name] = execution
	r.auditByCPID[audit.ChangePacketID] = append(r.auditByCPID[audit.ChangePacketID], cloneAuditEntry(audit))
	return nil
}

func (r *MemoryRepository) ListEvidenceAuditEntries(_ context.Context, changePacketID string, limit int) ([]domain.EvidenceAuditEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make([]domain.EvidenceAuditEntry, 0, len(r.auditByCPID[changePacketID]))
	for _, entry := range r.auditByCPID[changePacketID] {
		entries = append(entries, cloneAuditEntry(entry))
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].OccurredAt.After(entries[j].OccurredAt) })
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func cloneAuditEntry(entry domain.EvidenceAuditEntry) domain.EvidenceAuditEntry {
	if len(entry.Metadata) == 0 {
		return entry
	}
	entry.Metadata = make(map[string]string, len(entry.Metadata))
	for key, value := range entry.Metadata {
		entry.Metadata[key] = value
	}
	return entry
}

func (r *MemoryRepository) SaveChangePacket(_ context.Context, packet domain.ChangePacket) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.changePackets[packet.ID] = packet
	r.ensureEvidenceLocked(packet)
	return nil
}

func (r *MemoryRepository) GetChangePacket(_ context.Context, id string) (domain.ChangePacket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	packet, ok := r.changePackets[id]
	if !ok {
		return domain.ChangePacket{}, ErrChangePacketNotFound
	}
	return packet, nil
}

func (r *MemoryRepository) FindLatestChangePacket(_ context.Context, repoFullName string, prNumber int, headSHA string) (domain.ChangePacket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var latest domain.ChangePacket
	found := false
	for _, packet := range r.changePackets {
		if packet.Repo.FullName != repoFullName || packet.PR.Number != prNumber || packet.PR.HeadSHA != headSHA {
			continue
		}
		if !found || packet.UpdatedAt.After(latest.UpdatedAt) {
			latest, found = packet, true
		}
	}
	if !found {
		return domain.ChangePacket{}, ErrChangePacketNotFound
	}
	return latest, nil
}

func (r *MemoryRepository) ListChangePackets(_ context.Context, limit int) ([]domain.ChangePacket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	packets := make([]domain.ChangePacket, 0, len(r.changePackets))
	for _, packet := range r.changePackets {
		packets = append(packets, packet)
	}

	sort.Slice(packets, func(i, j int) bool {
		return packets[i].UpdatedAt.After(packets[j].UpdatedAt)
	})
	if limit > 0 && len(packets) > limit {
		packets = packets[:limit]
	}

	return packets, nil
}

func (r *MemoryRepository) SaveEvent(_ context.Context, event events.Envelope) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[event.ID] = event
	return nil
}

func (r *MemoryRepository) UpsertEvidenceExecution(_ context.Context, execution domain.EvidenceExecution) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.evidenceByCPID[execution.ChangePacketID] == nil {
		r.evidenceByCPID[execution.ChangePacketID] = make(map[string]domain.EvidenceExecution)
	}
	if execution.UpdatedAt.IsZero() {
		execution.UpdatedAt = time.Now().UTC()
	}
	r.evidenceByCPID[execution.ChangePacketID][execution.Name] = execution
	return nil
}

func (r *MemoryRepository) ListEvidenceExecutions(_ context.Context, changePacketID string) ([]domain.EvidenceExecution, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	values := r.evidenceByCPID[changePacketID]
	executions := make([]domain.EvidenceExecution, 0, len(values))
	for _, execution := range values {
		executions = append(executions, execution)
	}
	sort.Slice(executions, func(i, j int) bool {
		return executions[i].Name < executions[j].Name
	})
	return executions, nil
}

func (r *MemoryRepository) Migrate(context.Context) error { return nil }
func (r *MemoryRepository) Close() error                  { return nil }

func (r *MemoryRepository) ensureEvidenceLocked(packet domain.ChangePacket) {
	if r.evidenceByCPID[packet.ID] == nil {
		r.evidenceByCPID[packet.ID] = make(map[string]domain.EvidenceExecution)
	}
	for _, requirement := range packet.Evidence {
		current, ok := r.evidenceByCPID[packet.ID][requirement.Name]
		if ok {
			current.Type = requirement.Type
			current.Required = requirement.Required
			r.evidenceByCPID[packet.ID][requirement.Name] = current
			continue
		}

		r.evidenceByCPID[packet.ID][requirement.Name] = domain.EvidenceExecution{
			ChangePacketID: packet.ID,
			Name:           requirement.Name,
			Type:           requirement.Type,
			Status:         domain.EvidencePending,
			Required:       requirement.Required,
			UpdatedBy:      "merger",
			UpdatedAt:      time.Now().UTC(),
		}
	}
}
