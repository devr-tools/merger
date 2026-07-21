package controlplane

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/store"
)

const (
	DefaultListLimit           = 50
	MaxListLimit               = 200
	maxEvidenceRequestBodySize = 64 << 10
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
		methodNotAllowed(w, http.MethodGet)
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
			methodNotAllowed(w, http.MethodGet)
			return
		}
		h.getChangePacket(w, r, changePacketID)
		return
	}

	if len(parts) == 3 && parts[1] == "evidence" {
		if parts[2] == "audit" {
			if r.Method != http.MethodGet {
				methodNotAllowed(w, http.MethodGet)
				return
			}
			h.listEvidenceAudit(w, r, changePacketID)
			return
		}
		if r.Method != http.MethodPut {
			methodNotAllowed(w, http.MethodPut)
			return
		}
		h.updateEvidenceExecution(w, r, changePacketID, parts[2])
		return
	}

	http.NotFound(w, r)
}

func (h *HTTPHandler) listEvidenceAudit(w http.ResponseWriter, r *http.Request, changePacketID string) {
	limit, err := parseListLimit(r)
	if err != nil {
		writeLoggedHTTPError(w, r, http.StatusBadRequest, "invalid limit parameter", err)
		return
	}
	entries, err := h.service.ListEvidenceAuditEntries(r.Context(), changePacketID, limit)
	if err != nil {
		writeLoggedHTTPError(w, r, statusForError(err), publicErrorMessage(err), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": entries})
}

func (h *HTTPHandler) listChangePackets(w http.ResponseWriter, r *http.Request) {
	limit, err := parseListLimit(r)
	if err != nil {
		writeLoggedHTTPError(w, r, http.StatusBadRequest, "invalid limit parameter", err)
		return
	}

	packets, err := h.service.ListChangePackets(r.Context(), limit)
	if err != nil {
		writeLoggedHTTPError(w, r, http.StatusInternalServerError, "internal server error", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": packets})
}

func (h *HTTPHandler) getChangePacket(w http.ResponseWriter, r *http.Request, id string) {
	view, err := h.service.GetChangePacket(r.Context(), id)
	if err != nil {
		writeLoggedHTTPError(w, r, statusForError(err), publicErrorMessage(err), err)
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
	r.Body = http.MaxBytesReader(w, r.Body, maxEvidenceRequestBodySize)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		writeLoggedHTTPError(w, r, http.StatusBadRequest, "invalid request body", err)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = errors.New("request body must contain a single JSON value")
		}
		writeLoggedHTTPError(w, r, http.StatusBadRequest, "invalid request body", err)
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
		writeLoggedHTTPError(w, r, statusForError(err), publicErrorMessage(err), err)
		return
	}
	writeJSON(w, http.StatusAccepted, execution)
}

func parseListLimit(r *http.Request) (int, error) {
	values, present := r.URL.Query()["limit"]
	if !present {
		return DefaultListLimit, nil
	}
	if len(values) != 1 || values[0] == "" {
		return 0, errors.New("limit must be a positive integer")
	}

	limit, err := strconv.Atoi(values[0])
	if err != nil || limit <= 0 {
		return 0, errors.New("limit must be a positive integer")
	}
	if limit > MaxListLimit {
		limit = MaxListLimit
	}
	return limit, nil
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeLoggedHTTPError(w http.ResponseWriter, r *http.Request, status int, message string, err error) {
	if err != nil {
		log.Printf("controlplane http error: method=%s path=%s status=%d err=%v", r.Method, r.URL.Path, status, err)
	}
	http.Error(w, message, status)
}

func statusForError(err error) int {
	if errors.Is(err, store.ErrChangePacketNotFound) {
		return http.StatusNotFound
	}
	var validationError *EvidenceValidationError
	if errors.As(err, &validationError) {
		return http.StatusBadRequest
	}
	var notFoundError *EvidenceNotFoundError
	if errors.As(err, &notFoundError) {
		return http.StatusNotFound
	}
	var transitionError *EvidenceTransitionError
	if errors.As(err, &transitionError) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func publicErrorMessage(err error) string {
	if errors.Is(err, store.ErrChangePacketNotFound) {
		return "change packet not found"
	}
	var validationError *EvidenceValidationError
	if errors.As(err, &validationError) {
		return validationError.Error()
	}
	var notFoundError *EvidenceNotFoundError
	if errors.As(err, &notFoundError) {
		return notFoundError.Error()
	}
	var transitionError *EvidenceTransitionError
	if errors.As(err, &transitionError) {
		return transitionError.Error()
	}
	return "internal server error"
}
