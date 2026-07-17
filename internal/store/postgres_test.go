package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/devr-tools/merger/internal/domain"
)

type recordingExecutor struct {
	queries []string
	args    [][]any
	err     error
}

func (e *recordingExecutor) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	e.queries = append(e.queries, query)
	e.args = append(e.args, args)
	return stubResult{}, e.err
}

type stubResult struct{}

func (stubResult) LastInsertId() (int64, error) { return 0, nil }
func (stubResult) RowsAffected() (int64, error) { return 1, nil }

func TestSyncEvidenceRequirementsPreservesExecutionOwnedFields(t *testing.T) {
	executor := &recordingExecutor{}
	updatedAt := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	packet := domain.ChangePacket{
		ID:        "cp-1",
		UpdatedAt: updatedAt,
		Evidence: []domain.EvidenceRequirement{{
			Name:     "integration_tests",
			Type:     domain.EvidenceIntegrationTests,
			Required: true,
		}},
	}

	if err := syncEvidenceRequirements(context.Background(), executor, packet); err != nil {
		t.Fatalf("sync evidence requirements: %v", err)
	}
	if len(executor.queries) != 1 {
		t.Fatalf("expected one evidence upsert, got %d", len(executor.queries))
	}

	conflictClause := strings.SplitN(executor.queries[0], "do update set", 2)
	if len(conflictClause) != 2 {
		t.Fatalf("expected an on-conflict update clause, got %q", executor.queries[0])
	}
	for _, executionField := range []string{"status", "summary", "details_url", "updated_by", "metadata", "updated_at"} {
		if strings.Contains(conflictClause[1], executionField) {
			t.Fatalf("conflict update must preserve execution-owned field %q: %s", executionField, conflictClause[1])
		}
	}
	if !strings.Contains(conflictClause[1], "evidence_type") || !strings.Contains(conflictClause[1], "required") {
		t.Fatalf("conflict update must refresh requirement-owned fields: %s", conflictClause[1])
	}
}

func TestSyncEvidenceRequirementsReturnsExecutorError(t *testing.T) {
	want := errors.New("write failed")
	executor := &recordingExecutor{err: want}
	packet := domain.ChangePacket{
		ID: "cp-1",
		Evidence: []domain.EvidenceRequirement{{
			Name: "unit_tests",
			Type: domain.EvidenceUnitTests,
		}},
	}

	err := syncEvidenceRequirements(context.Background(), executor, packet)
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}
