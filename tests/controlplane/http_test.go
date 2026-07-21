package controlplane_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devr-tools/merger/internal/controlplane"
	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/store"
)

func TestHTTPHandlerReturnsChangePacketView(t *testing.T) {
	repo := seedRepository(t)
	handler := controlplane.NewHTTPHandler(controlplane.NewService(repo))
	mux := http.NewServeMux()
	handler.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/change-packets/cp_1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	packet := payload["packet"].(map[string]any)
	if packet["id"] != "cp_1" {
		t.Fatalf("unexpected packet payload: %#v", payload)
	}
}

func TestHTTPHandlerUpdatesEvidenceExecution(t *testing.T) {
	repo := seedRepository(t)
	handler := controlplane.NewHTTPHandler(controlplane.NewService(repo))
	mux := http.NewServeMux()
	handler.Register(mux)

	body := bytes.NewBufferString(`{"status":"satisfied","summary":"tests passed","updatedBy":"ci","type":"security_review","required":false}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/change-packets/cp_1/evidence/integration_tests", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}

	executions, err := repo.ListEvidenceExecutions(context.Background(), "cp_1")
	if err != nil {
		t.Fatalf("list executions: %v", err)
	}
	if executions[0].Status != domain.EvidenceSatisfied {
		t.Fatalf("expected satisfied evidence, got %s", executions[0].Status)
	}
	if executions[0].Type != domain.EvidenceIntegrationTests || !executions[0].Required {
		t.Fatalf("expected policy-owned evidence fields, got type=%q required=%t", executions[0].Type, executions[0].Required)
	}
}

func TestHTTPHandlerListsImmutableEvidenceAuditHistory(t *testing.T) {
	repo := seedRepository(t)
	handler := controlplane.NewHTTPHandler(controlplane.NewService(repo))
	mux := http.NewServeMux()
	handler.Register(mux)

	for _, body := range []string{
		`{"status":"running","summary":"started","updatedBy":"ci"}`,
		`{"status":"satisfied","summary":"passed","updatedBy":"ci"}`,
	} {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/change-packets/cp_1/evidence/integration_tests", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("update evidence: got %d", rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/change-packets/cp_1/evidence/audit", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Items []domain.EvidenceAuditEntry `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 2 {
		t.Fatalf("expected two immutable audit entries, got %#v", response.Items)
	}
	if response.Items[0].FromStatus != domain.EvidenceRunning || response.Items[0].ToStatus != domain.EvidenceSatisfied || response.Items[0].Actor != "ci" {
		t.Fatalf("unexpected latest audit entry: %#v", response.Items[0])
	}
	if response.Items[1].FromStatus != domain.EvidencePending || response.Items[1].ToStatus != domain.EvidenceRunning {
		t.Fatalf("unexpected original audit entry: %#v", response.Items[1])
	}
	if response.Items[0].ID == "" || response.Items[0].OccurredAt.IsZero() {
		t.Fatalf("expected immutable audit identity and time: %#v", response.Items[0])
	}
}

func TestHTTPHandlerRejectsInvalidEvidenceUpdates(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		body       string
		prepare    func(t *testing.T, mux *http.ServeMux)
		wantStatus int
	}{
		{
			name:       "invalid status",
			path:       "/api/v1/change-packets/cp_1/evidence/integration_tests",
			body:       `{"status":"unknown"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty status",
			path:       "/api/v1/change-packets/cp_1/evidence/integration_tests",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing packet",
			path:       "/api/v1/change-packets/missing/evidence/integration_tests",
			body:       `{"status":"satisfied"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "undeclared evidence",
			path:       "/api/v1/change-packets/cp_1/evidence/security_review",
			body:       `{"status":"satisfied"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "unattributed waiver",
			path:       "/api/v1/change-packets/cp_1/evidence/integration_tests",
			body:       `{"status":"waived"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "invalid transition",
			path: "/api/v1/change-packets/cp_1/evidence/integration_tests",
			body: `{"status":"running"}`,
			prepare: func(t *testing.T, mux *http.ServeMux) {
				req := httptest.NewRequest(http.MethodPut, "/api/v1/change-packets/cp_1/evidence/integration_tests", bytes.NewBufferString(`{"status":"satisfied"}`))
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				if rec.Code != http.StatusAccepted {
					t.Fatalf("prepare satisfied evidence: got status %d", rec.Code)
				}
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := seedRepository(t)
			handler := controlplane.NewHTTPHandler(controlplane.NewService(repo))
			mux := http.NewServeMux()
			handler.Register(mux)
			if test.prepare != nil {
				test.prepare(t, mux)
			}

			req := httptest.NewRequest(http.MethodPut, test.path, bytes.NewBufferString(test.body))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != test.wantStatus {
				t.Fatalf("expected %d, got %d: %s", test.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHTTPHandlerRejectsTrailingEvidenceJSON(t *testing.T) {
	repo := seedRepository(t)
	handler := controlplane.NewHTTPHandler(controlplane.NewService(repo))
	mux := http.NewServeMux()
	handler.Register(mux)

	body := bytes.NewBufferString(`{"status":"satisfied"} {"status":"pending"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/change-packets/cp_1/evidence/integration_tests", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHTTPHandlerRejectsOversizedEvidenceRequest(t *testing.T) {
	repo := seedRepository(t)
	handler := controlplane.NewHTTPHandler(controlplane.NewService(repo))
	mux := http.NewServeMux()
	handler.Register(mux)

	body := bytes.NewBufferString(`{"summary":"` + strings.Repeat("x", 65<<10) + `"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/change-packets/cp_1/evidence/integration_tests", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func seedRepository(t *testing.T) *store.MemoryRepository {
	t.Helper()

	repo := store.NewMemoryRepository()
	packet := domain.ChangePacket{
		ID:        "cp_1",
		Repo:      domain.RepoRef{FullName: "acme/repo"},
		PR:        domain.PullRequestRef{Number: 42},
		MergeLane: domain.MergeLaneYellow,
		RiskSummary: domain.RiskSummary{
			Score: 30,
		},
		UpdatedAt: time.Now().UTC(),
		Evidence: []domain.EvidenceRequirement{
			{Name: "integration_tests", Type: domain.EvidenceIntegrationTests, Required: true},
		},
	}
	if err := repo.SaveChangePacket(context.Background(), packet); err != nil {
		t.Fatalf("seed change packet: %v", err)
	}
	return repo
}
