package api

import (
	"encoding/json"
	"io"
	"net/http"
)

const (
	CodeOK               = "OK"
	CodeBadRequest       = "BAD_REQUEST"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	CodeInternalError    = "INTERNAL_ERROR"
	CodeTimeout          = "TIMEOUT"
)

type AgentQueryRequest struct {
	Message string `json:"message"`
}

type ResponseEnvelope struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

type HealthData struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
}

type ErrorData struct {
	RequestID string `json:"request_id"`
}

type AgentQueryData struct {
	RequestID      string `json:"request_id"`
	Intent         string `json:"intent"`
	Skill          string `json:"skill,omitempty"`
	Workflow       string `json:"workflow"`
	Summary        string `json:"summary"`
	Results        any    `json:"results"`
	Warnings       any    `json:"warnings,omitempty"`
	ExecutionSteps any    `json:"execution_steps"`
	ToolCalls      any    `json:"tool_calls"`
	DurationMS     int64  `json:"duration_ms"`
}

func decodeJSON(r *http.Request, v any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

func writeSuccess(w http.ResponseWriter, r *http.Request, data any) {
	writeJSON(w, http.StatusOK, ResponseEnvelope{
		Code: CodeOK,
		Msg:  "ok",
		Data: data,
	})
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code string, message string) {
	writeJSON(w, status, ResponseEnvelope{
		Code: code,
		Msg:  message,
		Data: ErrorData{RequestID: RequestIDFromContext(r.Context())},
	})
}
