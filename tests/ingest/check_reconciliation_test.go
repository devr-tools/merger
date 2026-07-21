package ingest_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/devr-tools/merger/internal/controlplane"
	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/events"
	"github.com/devr-tools/merger/internal/github"
	"github.com/devr-tools/merger/internal/ingest"
	"github.com/devr-tools/merger/internal/lanes"
	"github.com/devr-tools/merger/internal/mutations"
	"github.com/devr-tools/merger/internal/policy"
	"github.com/devr-tools/merger/internal/risk"
	"github.com/devr-tools/merger/internal/runtimegraph"
	"github.com/devr-tools/merger/internal/store"
	"github.com/devr-tools/merger/internal/telemetry"
)

func TestProcessCheckRunReconcilesOnlyExplicitBindingForLatestMatchingPacket(t *testing.T) {
	repository := store.NewMemoryRepository()
	older := reconciliationPacket("cp_old", time.Now().UTC().Add(-time.Minute))
	latest := reconciliationPacket("cp_latest", time.Now().UTC())
	if err := repository.SaveChangePacket(context.Background(), older); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveChangePacket(context.Background(), latest); err != nil {
		t.Fatal(err)
	}
	updater := &recordingEvidenceUpdater{}
	processor := newReconciliationProcessor(repository, updater)

	requireNoError(t, processor.ProcessCheckRun(context.Background(), checkRunPayload(t, "CI / integration", 123, "success", "head-sha")))
	if len(updater.executions) != 1 {
		t.Fatalf("expected one evidence update, got %d", len(updater.executions))
	}
	execution := updater.executions[0]
	if execution.ChangePacketID != latest.ID || execution.Name != "integration_tests" || execution.Status != domain.EvidenceSatisfied {
		t.Fatalf("unexpected reconciliation: %#v", execution)
	}
	if execution.Metadata["github_check_run_id"] != "99" || execution.Metadata["github_app_id"] != "123" {
		t.Fatalf("expected GitHub provenance metadata, got %#v", execution.Metadata)
	}
}

func TestProcessCheckRunRejectsUnboundOrStaleChecks(t *testing.T) {
	repository := store.NewMemoryRepository()
	packet := reconciliationPacket("cp_1", time.Now().UTC())
	if err := repository.SaveChangePacket(context.Background(), packet); err != nil {
		t.Fatal(err)
	}
	updater := &recordingEvidenceUpdater{}
	processor := newReconciliationProcessor(repository, updater)

	for _, payload := range []github.CheckRunWebhookPayload{
		checkRunPayload(t, "CI / integration", 999, "success", "head-sha"), // wrong app
		checkRunPayload(t, "another check", 123, "success", "head-sha"),    // wrong name
		checkRunPayload(t, "CI / integration", 123, "success", "old-sha"),  // stale head
	} {
		if err := processor.ProcessCheckRun(context.Background(), payload); err != nil {
			t.Fatalf("process rejected check: %v", err)
		}
	}
	if len(updater.executions) != 0 {
		t.Fatalf("expected rejected checks not to update evidence, got %#v", updater.executions)
	}
}

func TestProcessCheckRunMarksNonSuccessfulCompletionFailed(t *testing.T) {
	repository := store.NewMemoryRepository()
	packet := reconciliationPacket("cp_1", time.Now().UTC())
	if err := repository.SaveChangePacket(context.Background(), packet); err != nil {
		t.Fatal(err)
	}
	updater := &recordingEvidenceUpdater{}
	processor := newReconciliationProcessor(repository, updater)
	requireNoError(t, processor.ProcessCheckRun(context.Background(), checkRunPayload(t, "CI / integration", 123, "neutral", "head-sha")))
	if len(updater.executions) != 1 || updater.executions[0].Status != domain.EvidenceFailed {
		t.Fatalf("expected neutral check to fail evidence, got %#v", updater.executions)
	}
}

func TestProcessCheckRunUsesControlPlaneToPersistEvidence(t *testing.T) {
	repository := store.NewMemoryRepository()
	packet := reconciliationPacket("cp_1", time.Now().UTC())
	if err := repository.SaveChangePacket(context.Background(), packet); err != nil {
		t.Fatal(err)
	}
	processor := newReconciliationProcessor(repository, controlplane.NewService(repository))
	requireNoError(t, processor.ProcessCheckRun(context.Background(), checkRunPayload(t, "CI / integration", 123, "success", "head-sha")))
	executions, err := repository.ListEvidenceExecutions(context.Background(), packet.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(executions) != 1 || executions[0].Status != domain.EvidenceSatisfied {
		t.Fatalf("expected persisted satisfied evidence, got %#v", executions)
	}
	if executions[0].UpdatedBy != "github-app:123" || executions[0].DetailsURL != "" {
		t.Fatalf("expected GitHub provenance, got %#v", executions[0])
	}
}

func reconciliationPacket(id string, updatedAt time.Time) domain.ChangePacket {
	return domain.ChangePacket{
		ID: id, Repo: domain.RepoRef{FullName: "acme/repo"}, PR: domain.PullRequestRef{Number: 42, HeadSHA: "head-sha"}, UpdatedAt: updatedAt,
		Evidence: []domain.EvidenceRequirement{{Name: "integration_tests", Required: true, GitHubCheck: &domain.GitHubCheckBinding{Name: "CI / integration", AppID: 123}}},
	}
}

func checkRunPayload(t *testing.T, name string, appID int64, conclusion, headSHA string) github.CheckRunWebhookPayload {
	t.Helper()
	var payload github.CheckRunWebhookPayload
	body := map[string]any{
		"action": "completed", "repository": map[string]any{"full_name": "acme/repo"},
		"check_run": map[string]any{"id": 99, "name": name, "head_sha": headSHA, "status": "completed", "conclusion": conclusion,
			"app": map[string]any{"id": appID}, "pull_requests": []map[string]any{{"number": 42}}},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func newReconciliationProcessor(repository *store.MemoryRepository, updater ingest.EvidenceUpdater) *ingest.Processor {
	return ingest.NewProcessor(telemetry.NewLogger("error"), telemetry.NewTracer(), events.NewMemoryBus(), stubGitHubService{}, mutations.DefaultEngine(), risk.DefaultEngine(), policy.NewRuleEngine(policy.Config{}), lanes.NewAssigner(lanes.Config{}), stubCheckPublisher{}, runtimegraph.NewResolver(runtimegraph.Options{}), repository, updater)
}

type recordingEvidenceUpdater struct{ executions []domain.EvidenceExecution }

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (u *recordingEvidenceUpdater) UpdateEvidenceExecution(_ context.Context, execution domain.EvidenceExecution) (domain.EvidenceExecution, error) {
	u.executions = append(u.executions, execution)
	return execution, nil
}
