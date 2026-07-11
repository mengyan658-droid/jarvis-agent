#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT_DIR/scripts/lib.sh"

ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"
if [ -f "$ENV_FILE" ]; then
  set -a
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  set +a
fi

RUNTIME_DIR="${RUNTIME_DIR:-$ROOT_DIR/.runtime}"
PID_FILE="${PID_FILE:-$RUNTIME_DIR/jarvis-agent.pid}"
LOG_DIR="${LOG_DIR:-$ROOT_DIR/logger}"
LOG_DATE="${LOG_DATE:-$(date +%F)}"
LOG_FILE="${LOG_FILE:-$LOG_DIR/jarvis-agent-$LOG_DATE.log}"
TIME_TEST_LOG="${TIME_TEST_LOG:-$LOG_DIR/time-test.log}"
BIN_FILE="${BIN_FILE:-$RUNTIME_DIR/jarvis-agent}"
APP_PORT="${APP_PORT:-8080}"
AGENT_MAX_STEPS="${AGENT_MAX_STEPS:-10}"
AGENT_MAX_TOOL_CALLS="${AGENT_MAX_TOOL_CALLS:-20}"
LLM_PROVIDER="${LLM_PROVIDER:-mock}"
LLM_API_BASE_URL="${LLM_API_BASE_URL:-}"
LLM_API_KEY="${LLM_API_KEY:-}"
LLM_MODEL="${LLM_MODEL:-}"
if [ -z "${AGENT_TIMEOUT:-}" ]; then
  if [ "$LLM_PROVIDER" = "mock" ]; then
    AGENT_TIMEOUT="5s"
  else
    AGENT_TIMEOUT="30s"
  fi
fi

mkdir -p "$RUNTIME_DIR" "$LOG_DIR"
if [ ! -f "$LOG_FILE" ]; then
  : >"$LOG_FILE"
fi
if [ ! -f "$TIME_TEST_LOG" ]; then
  : >"$TIME_TEST_LOG"
fi

if [ -f "$PID_FILE" ]; then
  old_pid=$(cat "$PID_FILE")
  if [ -n "$old_pid" ] && kill -0 "$old_pid" 2>/dev/null; then
    echo "jarvis-agent is already running: pid=$old_pid"
    exit 0
  fi
  rm -f "$PID_FILE"
fi

listen_pid=$(find_listen_pid "$APP_PORT")
if [ -n "$listen_pid" ]; then
  echo "cannot start jarvis-agent: port $APP_PORT is already in use by pid=$listen_pid"
  echo "stop that process first, or run with another APP_PORT"
  exit 1
fi

echo "starting jarvis-agent on port $APP_PORT"
echo "llm config: provider=$LLM_PROVIDER model=${LLM_MODEL:-<default>} base_url=${LLM_API_BASE_URL:-<default>}"
echo "agent timeout: $AGENT_TIMEOUT"
echo "time test log: $TIME_TEST_LOG"
(
  cd "$ROOT_DIR"
  go build -o "$BIN_FILE" ./cmd/server
  APP_PORT="$APP_PORT" \
  AGENT_TIMEOUT="$AGENT_TIMEOUT" \
  AGENT_MAX_STEPS="$AGENT_MAX_STEPS" \
  AGENT_MAX_TOOL_CALLS="$AGENT_MAX_TOOL_CALLS" \
  LLM_PROVIDER="$LLM_PROVIDER" \
  LLM_API_BASE_URL="$LLM_API_BASE_URL" \
  LLM_API_KEY="$LLM_API_KEY" \
  LLM_MODEL="$LLM_MODEL" \
  TIME_TEST_LOG="$TIME_TEST_LOG" \
  nohup "$BIN_FILE" >>"$LOG_FILE" 2>&1 &
  echo $! >"$PID_FILE"
)

pid=$(cat "$PID_FILE")
sleep 1
if ! kill -0 "$pid" 2>/dev/null; then
  echo "failed to start jarvis-agent: process exited, see log=$LOG_FILE"
  rm -f "$PID_FILE"
  exit 1
fi
echo "started jarvis-agent: pid=$pid log=$LOG_FILE"
