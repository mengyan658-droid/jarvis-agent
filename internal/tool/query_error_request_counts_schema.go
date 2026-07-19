package tool

import "jarvis-agent/internal/client"

func QueryErrorRequestCountsFunctionTool() client.FunctionTool {
	return client.FunctionTool{
		Type: "function",
		Function: client.FunctionDefinition{
			Name:        QueryErrorRequestCountsToolName,
			Description: "Plan request error count query parameters. Time range is provided by the runtime and must not be generated.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"device_models": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Device model list from user input, for example iphone-15.",
					},
					"idcs": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "IDC list from user input, optional.",
					},
					"error_code": map[string]any{
						"type":        "string",
						"description": "Error code from user input, for example E500.",
					},
					"aggregation_value": map[string]any{
						"type":        "integer",
						"minimum":     1,
						"description": "Bucket size value, default 1.",
					},
					"aggregation_unit": map[string]any{
						"type":        "string",
						"enum":        []string{"m", "h", "d"},
						"description": "Bucket size unit, default h.",
					},
				},
				"required":             []string{"device_models", "error_code"},
				"additionalProperties": false,
			},
		},
	}
}
