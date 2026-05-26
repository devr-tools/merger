package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/mergerhq/merger/internal/domain"
	"github.com/mergerhq/merger/internal/store"
)

type HTTPHandler struct {
	service *Service
}

func NewHTTPHandler(service *Service) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/change-packets", h.handleChangePackets)
	mux.HandleFunc("/api/v1/change-packets/", h.handleChangePacketByID)
}

func (h *HTTPHandler) handleChangePackets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listChangePackets(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *HTTPHandler) handleChangePacketByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/change-packets/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing change packet id", http.StatusBadRequest)
		return
	}

	changePacketID := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.getChangePacket(w, r, changePacketID)
		return
	}

	if len(parts) == 3 && parts[1] == "evidence" && r.Method == http.MethodPut {
		h.updateEvidenceExecution(w, r, changePacketID, parts[2])
		return
	}

	http.NotFound(w, r)
}

func (h *HTTPHandler) listChangePackets(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	packets, err := h.service.ListChangePackets(r.Context(), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": packets})
}

func (h *HTTPHandler) getChangePacket(w http.ResponseWriter, r *http.Request, id string) {
	view, err := h.service.GetChangePacket(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), statusForError(err))
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *HTTPHandler) updateEvidenceExecution(w http.ResponseWriter, r *http.Request, changePacketID, evidenceName string) {
	var request struct {
		Status     domain.EvidenceStatus `json:"status"`
		Summary    string                `json:"summary"`
		DetailsURL string                `json:"detailsUrl"`
		UpdatedBy  string                `json:"updatedBy"`
		Type       domain.EvidenceType   `json:"type"`
		Required   bool                  `json:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	execution := domain.EvidenceExecution{
		ChangePacketID: changePacketID,
		Name:           evidenceName,
		Type:           request.Type,
		Status:         request.Status,
		Required:       request.Required,
		Summary:        request.Summary,
		DetailsURL:     request.DetailsURL,
		UpdatedBy:      request.UpdatedBy,
	}
	execution, err := h.service.UpdateEvidenceExecution(r.Context(), execution)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, execution)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func statusForError(err error) int {
	if errors.Is(err, store.ErrChangePacketNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
