package api

import "context"

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	userKey      contextKey = "user"
)

type UserContext struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	SessionID string `json:"session_id"`
}

func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}
