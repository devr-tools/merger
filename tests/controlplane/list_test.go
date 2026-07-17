package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devr-tools/merger/internal/controlplane"
	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/store"
)

func TestHTTPHandlerListsChangePackets(t *testing.T) {
	repo := seedRepository(t)
	err := repo.SaveChangePacket(context.Background(), domain.ChangePacket{
		ID:        "cp_2",
		Repo:      domain.RepoRef{FullName: "acme/repo"},
		PR:        domain.PullRequestRef{Number: 43},
		MergeLane: domain.MergeLaneGreen,
		UpdatedAt: time.Now().UTC().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("seed second packet: %v", err)
	}

	handler := controlplane.NewHTTPHandler(controlplane.NewService(repo))
	mux := http.NewServeMux()
	handler.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/change-packets?limit=1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		Items []domain.ChangePacket `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected one item, got %d", len(payload.Items))
	}
	if payload.Items[0].ID != "cp_2" {
		t.Fatalf("expected newest packet first, got %q", payload.Items[0].ID)
	}
}

func TestHTTPHandlerReturnsNotFoundForMissingPacket(t *testing.T) {
	repo := seedRepository(t)
	handler := controlplane.NewHTTPHandler(controlplane.NewService(repo))
	mux := http.NewServeMux()
	handler.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/change-packets/missing", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHTTPHandlerValidatesAndCapsListLimit(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantLimit  int
	}{
		{name: "default", query: "", wantStatus: http.StatusOK, wantLimit: controlplane.DefaultListLimit},
		{name: "valid", query: "?limit=17", wantStatus: http.StatusOK, wantLimit: 17},
		{name: "capped", query: "?limit=999", wantStatus: http.StatusOK, wantLimit: controlplane.MaxListLimit},
		{name: "empty", query: "?limit=", wantStatus: http.StatusBadRequest},
		{name: "zero", query: "?limit=0", wantStatus: http.StatusBadRequest},
		{name: "negative", query: "?limit=-1", wantStatus: http.StatusBadRequest},
		{name: "malformed", query: "?limit=many", wantStatus: http.StatusBadRequest},
		{name: "repeated", query: "?limit=1&limit=2", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &limitRecordingRepository{MemoryRepository: seedRepository(t)}
			handler := controlplane.NewHTTPHandler(controlplane.NewService(repo))
			mux := http.NewServeMux()
			handler.Register(mux)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/change-packets"+tt.query, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
			if tt.wantStatus == http.StatusOK && repo.limit != tt.wantLimit {
				t.Fatalf("expected repository limit %d, got %d", tt.wantLimit, repo.limit)
			}
		})
	}
}

func TestHTTPHandlerRejectsUnsupportedMethods(t *testing.T) {
	repo := seedRepository(t)
	handler := controlplane.NewHTTPHandler(controlplane.NewService(repo))
	mux := http.NewServeMux()
	handler.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/change-packets", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow %q, got %q", http.MethodGet, allow)
	}
}

type limitRecordingRepository struct {
	*store.MemoryRepository
	limit int
}

func (r *limitRecordingRepository) ListChangePackets(ctx context.Context, limit int) ([]domain.ChangePacket, error) {
	r.limit = limit
	return r.MemoryRepository.ListChangePackets(ctx, limit)
}
