package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"jarvis-agent/internal/config"
	"jarvis-agent/internal/service"
)

func TestAgentQueryResponseEnvelope(t *testing.T) {
	runtime := service.NewRuntime(config.Config{
		AgentTimeout:      5 * time.Second,
		AgentMaxSteps:     10,
		AgentMaxToolCalls: 20,
	}, slog.Default())
	router := NewRouter(runtime, slog.Default(), Timeout(5*time.Second))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/query", bytes.NewBufferString(`{"message":"查询华东生产环境最近一小时的故障机"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req-test")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", resp.Code, resp.Body.String())
	}
	var envelope ResponseEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != CodeOK || envelope.Msg != "ok" || envelope.Data == nil {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
}

func TestAgentQueryValidationErrorEnvelope(t *testing.T) {
	runtime := service.NewRuntime(config.Config{AgentTimeout: 5 * time.Second}, slog.Default())
	router := NewRouter(runtime, slog.Default(), Timeout(5*time.Second))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/query", bytes.NewBufferString(`{"message":""}`))
	req.Header.Set("X-Request-ID", "req-test")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status %d: %s", resp.Code, resp.Body.String())
	}
	var envelope ResponseEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != CodeBadRequest || envelope.Msg != "message is required" || envelope.Data == nil {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
}
