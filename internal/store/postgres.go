package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/events"
)

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(dsn string) (*PostgresRepository, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetMaxIdleConns(10)
	db.SetMaxOpenConns(25)

	return &PostgresRepository{db: db}, nil
}

func (r *PostgresRepository) Migrate(ctx context.Context) error {
	for _, statement := range postgresMigrations() {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresRepository) SaveChangePacket(ctx context.Context, packet domain.ChangePacket) error {
	payload, err := json.Marshal(packet)
	if err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	query := `
insert into merger_change_packets (
  id, repo_full_name, pr_number, head_sha, author_login, merge_lane, risk_score, decision_status, payload, created_at, updated_at
)
values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
on conflict (id) do update set
  head_sha = excluded.head_sha,
  merge_lane = excluded.merge_lane,
  risk_score = excluded.risk_score,
  decision_status = excluded.decision_status,
  payload = excluded.payload,
  updated_at = excluded.updated_at`

	_, err = tx.ExecContext(
		ctx,
		query,
		packet.ID,
		packet.Repo.FullName,
		packet.PR.Number,
		packet.PR.HeadSHA,
		packet.Author.Login,
		packet.MergeLane,
		packet.RiskSummary.Score,
		packet.Decision.Status,
		payload,
		packet.CreatedAt,
		packet.UpdatedAt,
	)
	if err != nil {
		return err
	}

	if err := syncEvidenceRequirements(ctx, tx, packet); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PostgresRepository) FindLatestChangePacket(ctx context.Context, repoFullName string, prNumber int, headSHA string) (domain.ChangePacket, error) {
	var payload []byte
	err := r.db.QueryRowContext(ctx, `
select payload from merger_change_packets
where repo_full_name = $1 and pr_number = $2 and head_sha = $3
order by updated_at desc
limit 1`, repoFullName, prNumber, headSHA).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ChangePacket{}, ErrChangePacketNotFound
	}
	if err != nil {
		return domain.ChangePacket{}, err
	}
	var packet domain.ChangePacket
	if err := json.Unmarshal(payload, &packet); err != nil {
		return domain.ChangePacket{}, err
	}
	return packet, nil
}

func (r *PostgresRepository) SaveEvent(ctx context.Context, event events.Envelope) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(
		ctx,
		`insert into merger_event_log (id, event_type, source, correlation_id, causation_id, change_packet_id, payload, created_at)
		 values ($1,$2,$3,$4,$5,$6,$7,$8)
		 on conflict (id) do nothing`,
		event.ID,
		event.Type,
		event.Source,
		event.CorrelationID,
		event.CausationID,
		event.ChangePacketID,
		payload,
		time.Now().UTC(),
	)

	return err
}

func (r *PostgresRepository) GetChangePacket(ctx context.Context, id string) (domain.ChangePacket, error) {
	var payload []byte
	if err := r.db.QueryRowContext(ctx, `select payload from merger_change_packets where id = $1`, id).Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ChangePacket{}, ErrChangePacketNotFound
		}
		return domain.ChangePacket{}, err
	}

	var packet domain.ChangePacket
	if err := json.Unmarshal(payload, &packet); err != nil {
		return domain.ChangePacket{}, err
	}
	return packet, nil
}

func (r *PostgresRepository) ListChangePackets(ctx context.Context, limit int) ([]domain.ChangePacket, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.db.QueryContext(ctx, `select payload from merger_change_packets order by updated_at desc limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	packets := make([]domain.ChangePacket, 0, limit)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}

		var packet domain.ChangePacket
		if err := json.Unmarshal(payload, &packet); err != nil {
			return nil, err
		}
		packets = append(packets, packet)
	}

	return packets, rows.Err()
}

func (r *PostgresRepository) UpsertEvidenceExecution(ctx context.Context, execution domain.EvidenceExecution) error {
	return upsertEvidenceExecution(ctx, r.db, execution)
}

func (r *PostgresRepository) RecordEvidenceUpdate(ctx context.Context, execution domain.EvidenceExecution, audit domain.EvidenceAuditEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertEvidenceExecution(ctx, tx, execution); err != nil {
		return err
	}
	metadata, err := json.Marshal(audit.Metadata)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
insert into merger_evidence_audit_entries (
	id, change_packet_id, evidence_name, from_status, to_status, actor, summary, details_url, metadata, occurred_at
) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		audit.ID, audit.ChangePacketID, audit.EvidenceName, audit.FromStatus, audit.ToStatus,
		audit.Actor, audit.Summary, audit.DetailsURL, string(metadata), audit.OccurredAt,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func upsertEvidenceExecution(ctx context.Context, executor statementExecutor, execution domain.EvidenceExecution) error {
	metadata, err := json.Marshal(execution.Metadata)
	if err != nil {
		return err
	}
	if execution.UpdatedAt.IsZero() {
		execution.UpdatedAt = time.Now().UTC()
	}

	_, err = executor.ExecContext(ctx, `
insert into merger_evidence_executions (
	change_packet_id, evidence_name, evidence_type, status, required, summary, details_url, updated_by, metadata, updated_at
)
values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
on conflict (change_packet_id, evidence_name) do update set
	evidence_type = excluded.evidence_type,
	status = excluded.status,
	required = excluded.required,
	summary = excluded.summary,
	details_url = excluded.details_url,
	updated_by = excluded.updated_by,
	metadata = excluded.metadata,
	updated_at = excluded.updated_at`,
		execution.ChangePacketID,
		execution.Name,
		execution.Type,
		execution.Status,
		execution.Required,
		execution.Summary,
		execution.DetailsURL,
		execution.UpdatedBy,
		string(metadata),
		execution.UpdatedAt,
	)
	return err
}

func (r *PostgresRepository) ListEvidenceAuditEntries(ctx context.Context, changePacketID string, limit int) ([]domain.EvidenceAuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
select id, change_packet_id, evidence_name, from_status, to_status, coalesce(actor,''), coalesce(summary,''), coalesce(details_url,''), metadata, occurred_at
from merger_evidence_audit_entries where change_packet_id = $1 order by occurred_at desc limit $2`, changePacketID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]domain.EvidenceAuditEntry, 0, limit)
	for rows.Next() {
		var entry domain.EvidenceAuditEntry
		var metadata []byte
		if err := rows.Scan(&entry.ID, &entry.ChangePacketID, &entry.EvidenceName, &entry.FromStatus, &entry.ToStatus, &entry.Actor, &entry.Summary, &entry.DetailsURL, &metadata, &entry.OccurredAt); err != nil {
			return nil, err
		}
		if len(metadata) > 0 {
			_ = json.Unmarshal(metadata, &entry.Metadata)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (r *PostgresRepository) ListEvidenceExecutions(ctx context.Context, changePacketID string) ([]domain.EvidenceExecution, error) {
	rows, err := r.db.QueryContext(ctx, `
select change_packet_id, evidence_name, evidence_type, status, required, coalesce(summary,''), coalesce(details_url,''), coalesce(updated_by,''), metadata, updated_at
from merger_evidence_executions
where change_packet_id = $1
order by evidence_name asc`, changePacketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	executions := make([]domain.EvidenceExecution, 0)
	for rows.Next() {
		var execution domain.EvidenceExecution
		var metadata []byte
		if err := rows.Scan(
			&execution.ChangePacketID,
			&execution.Name,
			&execution.Type,
			&execution.Status,
			&execution.Required,
			&execution.Summary,
			&execution.DetailsURL,
			&execution.UpdatedBy,
			&metadata,
			&execution.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if len(metadata) > 0 {
			_ = json.Unmarshal(metadata, &execution.Metadata)
		}
		executions = append(executions, execution)
	}

	return executions, rows.Err()
}

func (r *PostgresRepository) Close() error {
	if r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *PostgresRepository) Ping(ctx context.Context) error {
	if r.db == nil {
		return fmt.Errorf("postgres repository not initialized")
	}
	return r.db.PingContext(ctx)
}

type statementExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func syncEvidenceRequirements(ctx context.Context, executor statementExecutor, packet domain.ChangePacket) error {
	for _, requirement := range packet.Evidence {
		_, err := executor.ExecContext(ctx, `
insert into merger_evidence_executions (
	change_packet_id, evidence_name, evidence_type, status, required, updated_by, metadata, updated_at
)
values ($1,$2,$3,$4,$5,$6,$7,$8)
on conflict (change_packet_id, evidence_name) do update set
	evidence_type = excluded.evidence_type,
	required = excluded.required`,
			packet.ID,
			requirement.Name,
			requirement.Type,
			domain.EvidencePending,
			requirement.Required,
			"merger",
			"null",
			packet.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
