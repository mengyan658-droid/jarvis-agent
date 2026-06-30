#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT_DIR/scripts/lib.sh"

RUNTIME_DIR="${RUNTIME_DIR:-$ROOT_DIR/.runtime}"
PID_FILE="${PID_FILE:-$RUNTIME_DIR/jarvis-agent.pid}"
LOG_FILE="${LOG_FILE:-$RUNTIME_DIR/jarvis-agent.log}"
BIN_FILE="${BIN_FILE:-$RUNTIME_DIR/jarvis-agent}"
APP_PORT="${APP_PORT:-8080}"

mkdir -p "$RUNTIME_DIR"

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
(
  cd "$ROOT_DIR"
  go build -o "$BIN_FILE" ./cmd/server
  APP_PORT="$APP_PORT" \
  AGENT_TIMEOUT="${AGENT_TIMEOUT:-5s}" \
  AGENT_MAX_STEPS="${AGENT_MAX_STEPS:-10}" \
  AGENT_MAX_TOOL_CALLS="${AGENT_MAX_TOOL_CALLS:-20}" \
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
