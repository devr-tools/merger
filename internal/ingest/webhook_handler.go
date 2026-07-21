package ingest

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/devr-tools/merger/internal/github"
	"github.com/devr-tools/merger/internal/telemetry"
	"github.com/devr-tools/merger/pkg/identity"
)

type WebhookHandler struct {
	processor *Processor
	decoder   github.WebhookDecoder
}

func NewWebhookHandler(processor *Processor, decoder github.WebhookDecoder) *WebhookHandler {
	return &WebhookHandler{processor: processor, decoder: decoder}
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := telemetry.WithRequestID(r.Context(), identity.New("req"))

	webhook, err := h.decoder.Decode(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx = telemetry.WithCorrelationID(ctx, webhook.DeliveryID)
	if webhook.Event == "check_run" {
		if webhook.CheckRun == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if err := h.processor.ProcessCheckRun(ctx, *webhook.CheckRun); err != nil {
			log.Printf("reconcile GitHub check run: %v", err)
			http.Error(w, "unable to reconcile check run", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if webhook.Event != "pull_request" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch webhook.Payload.Action {
	case "opened", "reopened", "synchronize":
	default:
		w.WriteHeader(http.StatusAccepted)
		return
	}

	packet, err := h.processor.ProcessPROpened(ctx, webhook.Payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"changePacketId": packet.ID,
		"mergeLane":      packet.MergeLane,
		"riskScore":      packet.RiskSummary.Score,
	})
}
