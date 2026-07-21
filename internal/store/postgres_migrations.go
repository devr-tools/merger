package store

func postgresMigrations() []string {
	return []string{
		`create table if not exists merger_change_packets (
			id text primary key,
			repo_full_name text not null,
			pr_number integer not null,
			author_login text not null,
			merge_lane text not null,
			risk_score integer not null,
			decision_status text not null,
			payload jsonb not null,
			created_at timestamptz not null,
			updated_at timestamptz not null
		)`,
		`alter table merger_change_packets add column if not exists head_sha text`,
		`create index if not exists idx_merger_change_packets_repo_pr on merger_change_packets (repo_full_name, pr_number)`,
		`create index if not exists idx_merger_change_packets_exact_head on merger_change_packets (repo_full_name, pr_number, head_sha, updated_at desc)`,
		`create table if not exists merger_event_log (
			id text primary key,
			event_type text not null,
			source text not null,
			correlation_id text,
			causation_id text,
			change_packet_id text,
			payload jsonb not null,
			created_at timestamptz not null
		)`,
		`create index if not exists idx_merger_event_log_change_packet_id on merger_event_log (change_packet_id)`,
		`create index if not exists idx_merger_event_log_event_type on merger_event_log (event_type)`,
		`create table if not exists merger_evidence_executions (
			change_packet_id text not null,
			evidence_name text not null,
			evidence_type text not null,
			status text not null,
			required boolean not null,
			summary text,
			details_url text,
			updated_by text,
			metadata jsonb,
			updated_at timestamptz not null,
			primary key (change_packet_id, evidence_name)
		)`,
		`create index if not exists idx_merger_evidence_executions_change_packet_id on merger_evidence_executions (change_packet_id)`,
	}
}
