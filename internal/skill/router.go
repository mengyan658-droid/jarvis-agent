package skill

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"jarvis-agent/internal/client"
)

const SelectSkillFunctionName = "select_skill"

func SelectSkillFunctionTool(registry *Registry) client.FunctionTool {
	return client.FunctionTool{
		Type: "function",
		Function: client.FunctionDefinition{
			Name:        SelectSkillFunctionName,
			Description: "Select exactly one Jarvis Agent skill for the user request. This is a routing tool only; it does not execute the skill.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"skill": map[string]any{
						"type":        "string",
						"enum":        registry.Names(),
						"description": "Selected skill name. Must be one of the enum values.",
					},
					"parameters": map[string]any{
						"type":                 "object",
						"description":          "Normalized skill parameters. Use strings for values. Do not generate timestamps.",
						"additionalProperties": map[string]any{"type": "string"},
					},
					"confidence": map[string]any{
						"type":        "number",
						"minimum":     0,
						"maximum":     1,
						"description": "Confidence score from 0 to 1.",
					},
				},
				"required":             []string{"skill", "parameters"},
				"additionalProperties": false,
			},
		},
	}
}

func RouterSystemPrompt(registry *Registry) string {
	var b strings.Builder
	b.WriteString("你是 Jarvis Agent 的 Skill Router。你必须根据用户输入选择一个 skill，并调用 select_skill。\n")
	b.WriteString("不要执行业务查询，不要调用业务工具，不要生成最终答案。\n")
	b.WriteString("参数规则：不要生成时间戳；相对时间使用 since，例如 5h、30m、2d、1w；明确起止时间使用 start_text/end_text 原文；数组参数使用英文逗号分隔字符串。\n")
	b.WriteString("可用 skills：\n")
	for _, summary := range registry.Summaries() {
		b.WriteString("- name: ")
		b.WriteString(summary.Name)
		b.WriteString("\n  description: ")
		b.WriteString(summary.Description)
		b.WriteString("\n  executor: ")
		b.WriteString(summary.Executor)
		if summary.Workflow != "" {
			b.WriteString("\n  workflow: ")
			b.WriteString(summary.Workflow)
		}
		if len(summary.Intents) > 0 {
			b.WriteString("\n  intents: ")
			b.WriteString(strings.Join(summary.Intents, ", "))
		}
		if len(summary.Triggers) > 0 {
			b.WriteString("\n  triggers: ")
			b.WriteString(strings.Join(summary.Triggers, ", "))
		}
		if summary.ReadOnly {
			b.WriteString("\n  read_only: true")
		}
		if summary.OutputPolicy != "" {
			b.WriteString("\n  output_policy: ")
			b.WriteString(summary.OutputPolicy)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func DecodeSelection(arguments string) (Selection, error) {
	var raw struct {
		Skill      string         `json:"skill"`
		Parameters map[string]any `json:"parameters"`
		Confidence float64        `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
		return Selection{}, fmt.Errorf("decode select_skill arguments: %w", err)
	}
	params := map[string]string{}
	for key, value := range raw.Parameters {
		params[key] = stringifyParameter(value)
	}
	return Selection{
		Skill:      strings.TrimSpace(raw.Skill),
		Parameters: params,
		Confidence: raw.Confidence,
	}, nil
}

func stringifyParameter(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case nil:
		return ""
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(data)
	}
}
