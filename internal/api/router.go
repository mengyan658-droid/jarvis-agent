package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"jarvis-agent/internal/agent"
)

type Handler struct {
	Runtime *agent.Runtime
	Logger  *slog.Logger
}

func NewRouter(runtime *agent.Runtime, logger *slog.Logger, timeoutMiddleware func(http.Handler) http.Handler) http.Handler {
	h := &Handler{Runtime: runtime, Logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/api/v1/agent/query", h.query)

	var handler http.Handler = mux
	handler = UserHeaders(handler)
	handler = timeoutMiddleware(handler)
	handler = AccessLog(logger)(handler)
	handler = Recovery(logger)(handler)
	handler = RequestID(handler)
	return handler
}

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "method not allowed")
		return
	}
	writeSuccess(w, r, HealthData{
		RequestID: RequestIDFromContext(r.Context()),
		Status:    "ok",
	})
}

func (h *Handler) query(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "method not allowed")
		return
	}
	var req AgentQueryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, CodeBadRequest, "invalid json request")
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		writeError(w, r, http.StatusBadRequest, CodeBadRequest, "message is required")
		return
	}
	result, err := h.Runtime.Query(r.Context(), RequestIDFromContext(r.Context()), req.Message)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			writeError(w, r, http.StatusGatewayTimeout, CodeTimeout, "request timeout")
			return
		}
		writeError(w, r, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	writeSuccess(w, r, AgentQueryData{
		RequestID:      result.RequestID,
		Intent:         result.Intent,
		Workflow:       result.Workflow,
		Summary:        result.Summary,
		Results:        result.Results,
		Warnings:       result.Warnings,
		ExecutionSteps: result.Steps,
		ToolCalls:      result.ToolCalls,
		DurationMS:     result.DurationMS,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
