package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/semmidev/batch-processing/internal/domain"
	"github.com/semmidev/batch-processing/internal/observability"
	"github.com/semmidev/batch-processing/internal/service"
	"go.uber.org/zap"
)

type Handler struct {
	batchService service.BatchService
	validator    *validator.Validate
}

func NewHandler(batchService service.BatchService) *Handler {
	return &Handler{
		batchService: batchService,
		validator:    validator.New(),
	}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (h *Handler) SubmitBatch(w http.ResponseWriter, r *http.Request) {
	var req domain.SubmitBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	if err := h.validator.Struct(req); err != nil {
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	batchID, err := h.batchService.SubmitBatch(r.Context(), idempotencyKey, req)
	if err != nil {
		observability.Log.Error("failed to submit batch", zap.Error(err))
		h.respondError(w, http.StatusInternalServerError, "failed to submit batch")
		return
	}

	res := domain.SubmitBatchResponse{
		BatchID: batchID.String(),
		Message: "Batch accepted for processing",
	}

	h.respondJSON(w, http.StatusAccepted, res)
}

func (h *Handler) GetBatchStatus(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	batchID, err := uuid.Parse(idStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid batch id")
		return
	}

	status, err := h.batchService.GetBatchStatus(r.Context(), batchID)
	if err != nil {
		observability.Log.Error("failed to get batch status", zap.Error(err))
		h.respondError(w, http.StatusInternalServerError, "failed to get batch status")
		return
	}

	if status == nil {
		h.respondError(w, http.StatusNotFound, "batch not found")
		return
	}

	h.respondJSON(w, http.StatusOK, status)
}

func (h *Handler) CancelBatch(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	batchID, err := uuid.Parse(idStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid batch id")
		return
	}

	err = h.batchService.CancelBatch(r.Context(), batchID)
	if err != nil {
		observability.Log.Error("failed to cancel batch", zap.Error(err))
		if err.Error() == "batch not found" {
			h.respondError(w, http.StatusNotFound, err.Error())
		} else {
			h.respondError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]string{"message": "batch cancelled successfully"})
}

func (h *Handler) respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func (h *Handler) respondError(w http.ResponseWriter, status int, message string) {
	h.respondJSON(w, status, map[string]string{"error": message})
}
