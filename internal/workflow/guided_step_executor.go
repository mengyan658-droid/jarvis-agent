package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"jarvis-agent/internal/client"
)

type GuidedStepExecutor struct {
	ctx      context.Context
	wfctx    Context
	steps    *[]Step
	warnings *[]string
}

type GuidedStep struct {
	Name string
	Run  func(*GuidedStepExecutor) error
}

type GuidedToolPlanRequest struct {
	SystemPrompt     string
	UserMessage      string
	Messages         []client.ToolChatMessage
	Tool             client.FunctionTool
	RequiredToolName string
}

func NewGuidedStepExecutor(ctx context.Context, wfctx Context, steps *[]Step, warnings *[]string) *GuidedStepExecutor {
	return &GuidedStepExecutor{
		ctx:      ctx,
		wfctx:    wfctx,
		steps:    steps,
		warnings: warnings,
	}
}

func (e *GuidedStepExecutor) Run(steps ...GuidedStep) error {
	for _, step := range steps {
		if strings.TrimSpace(step.Name) == "" {
			return fmt.Errorf("guided step name is required")
		}
		if step.Run == nil {
			return fmt.Errorf("guided step %q handler is required", step.Name)
		}
		if err := runStep(e.steps, step.Name, func() error {
			return step.Run(e)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *GuidedStepExecutor) Warn(message string) {
	message = strings.TrimSpace(message)
	if message == "" || e.warnings == nil {
		return
	}
	*e.warnings = append(*e.warnings, message)
}

func (e *GuidedStepExecutor) PlanToolCall(req GuidedToolPlanRequest) (client.ToolCall, error) {
	functionLLM, ok := e.wfctx.LLM.(client.FunctionCallingClient)
	if !ok {
		return client.ToolCall{}, fmt.Errorf("llm client does not support function calling")
	}

	toolName := strings.TrimSpace(req.RequiredToolName)
	if toolName == "" {
		toolName = req.Tool.Function.Name
	}
	if toolName == "" {
		return client.ToolCall{}, fmt.Errorf("required tool name is required")
	}

	userMessage := strings.TrimSpace(req.UserMessage)
	if userMessage == "" {
		userMessage = strings.TrimSpace(e.wfctx.Message)
	}
	if userMessage == "" {
		return client.ToolCall{}, fmt.Errorf("original user message is required")
	}

	messages := make([]client.ToolChatMessage, 0, 2+len(req.Messages))
	if strings.TrimSpace(req.SystemPrompt) != "" {
		messages = append(messages, client.ToolChatMessage{Role: "system", Content: req.SystemPrompt})
	}
	messages = append(messages, client.ToolChatMessage{Role: "user", Content: userMessage})
	messages = append(messages, req.Messages...)

	assistant, err := functionLLM.ChatWithTools(e.ctx, messages, []client.FunctionTool{req.Tool})
	if err != nil {
		return client.ToolCall{}, err
	}
	for _, call := range assistant.ToolCalls {
		if call.Function.Name == toolName {
			return call, nil
		}
	}
	return client.ToolCall{}, fmt.Errorf("%s tool call is required", toolName)
}

func (e *GuidedStepExecutor) ExecuteTool(name string, input any) (any, error) {
	if e.wfctx.Tools == nil {
		return nil, fmt.Errorf("tool registry is required")
	}
	return e.wfctx.Tools.Execute(e.ctx, name, input, e.wfctx.ToolRecorder)
}

func DecodeGuidedToolArguments[T any](call client.ToolCall) (T, error) {
	var out T
	if err := json.Unmarshal([]byte(call.Function.Arguments), &out); err != nil {
		return out, fmt.Errorf("decode %s arguments: %w", call.Function.Name, err)
	}
	return out, nil
}
