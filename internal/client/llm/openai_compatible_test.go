package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jarvis-agent/internal/client"
)

func TestOpenAICompatibleClientSendsGLMCompatibleRequest(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"name\":\"unknown\",\"parameters\":{}}"}}]}`))
	}))
	defer server.Close()

	c := NewOpenAICompatibleClient(server.URL, "token", "glm-5.1")
	if _, err := c.ParseIntent(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}

	if gotPath != "/chat/completions" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotAuth != "Bearer token" {
		t.Fatalf("unexpected auth header: %s", gotAuth)
	}
	if gotBody["model"] != "glm-5.1" {
		t.Fatalf("unexpected model: %#v", gotBody["model"])
	}
	if gotBody["stream"] != false {
		t.Fatalf("unexpected stream: %#v", gotBody["stream"])
	}
	if _, ok := gotBody["messages"].([]any); !ok {
		t.Fatalf("messages missing: %#v", gotBody["messages"])
	}
}

func TestOpenAICompatibleClientAcceptsFullChatCompletionsURL(t *testing.T) {
	c := NewOpenAICompatibleClient("https://open.bigmodel.cn/api/paas/v4/chat/completions", "token", "glm-5.1")
	if got := c.chatCompletionsURL(); got != "https://open.bigmodel.cn/api/paas/v4/chat/completions" {
		t.Fatalf("unexpected url: %s", got)
	}
}

func TestOpenAICompatibleClientSendsFunctionCallingTools(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"get_host","arguments":"{\"host_id\":\"host-001\"}"}}]}}]}`))
	}))
	defer server.Close()

	c := NewOpenAICompatibleClient(server.URL, "token", "glm-5.1")
	_, err := c.ChatWithTools(context.Background(), []client.ToolChatMessage{{Role: "user", Content: "排查 host-001"}}, []client.FunctionTool{{
		Type: "function",
		Function: client.FunctionDefinition{
			Name:        "get_host",
			Description: "查询主机",
			Parameters:  map[string]any{"type": "object"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["tool_choice"] != "auto" {
		t.Fatalf("unexpected tool_choice: %#v", gotBody["tool_choice"])
	}
	tools, ok := gotBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("unexpected tools: %#v", gotBody["tools"])
	}
}
