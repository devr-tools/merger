package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mergerhq/merger/internal/controlplane"
	"github.com/mergerhq/merger/internal/domain"
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
}
