package tool

import "jarvis-agent/internal/client"

func ResolveTimeRangeFunctionTool() client.FunctionTool {
	return client.FunctionTool{
		Type: "function",
		Function: client.FunctionDefinition{
			Name:        ResolveTimeRangeToolName,
			Description: resolveTimeRangeToolDescription(),
			Parameters:  ResolveTimeRangeJSONSchema(),
		},
	}
}

func ResolveTimeRangeJSONSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kind": map[string]any{
				"type":        "string",
				"enum":        []string{"default", "today", "yesterday", "relative", "since", "absolute_range"},
				"description": "Time range type. Use default only when the user did not specify a time range.",
			},
			"amount": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"description": "Positive integer for kind=relative, for example 30 for last 30 minutes.",
			},
			"unit": map[string]any{
				"type":        "string",
				"enum":        []string{"minute", "hour", "day"},
				"description": "Unit for kind=relative. Do not use seconds or milliseconds.",
			},
			"start_text": map[string]any{
				"type":        "string",
				"description": "Original start time text from the user. Required for since and absolute_range. Do not convert it to a timestamp.",
			},
			"end_text": map[string]any{
				"type":        "string",
				"description": "Original end time text from the user. Required only for absolute_range. Use now only if the user explicitly says until now.",
			},
			"timezone": map[string]any{
				"type":        "string",
				"description": "Timezone name. Use Asia/Shanghai unless the user explicitly specifies another timezone.",
			},
		},
		"required":             []string{"kind"},
		"additionalProperties": false,
	}
}

func resolveTimeRangeToolDescription() string {
	return "Resolve a user time expression into a validated [start,end) time range. " +
		"Do not calculate timestamps yourself. This tool returns start/end in both Unix seconds and milliseconds. " +
		"Use kind=default when the user does not specify time; it means the last 1 hour. " +
		"Use kind=today for today 00:00 to now in the selected timezone. " +
		"Use kind=yesterday for yesterday natural day, not last 24 hours. " +
		"Use kind=relative for last N minutes/hours/days. " +
		"Use kind=since for from a user-provided start time to now. " +
		"Use kind=absolute_range only when the user provides both start and end time."
}
