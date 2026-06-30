#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT_DIR/scripts/lib.sh"

RUNTIME_DIR="${RUNTIME_DIR:-$ROOT_DIR/.runtime}"
PID_FILE="${PID_FILE:-$RUNTIME_DIR/jarvis-agent.pid}"
STOP_TIMEOUT_SECONDS="${STOP_TIMEOUT_SECONDS:-10}"
APP_PORT="${APP_PORT:-8080}"

if [ ! -f "$PID_FILE" ]; then
  listen_pid=$(find_listen_pid "$APP_PORT")
  if [ -n "$listen_pid" ]; then
    echo "jarvis-agent pid file not found, but port $APP_PORT is in use by pid=$listen_pid"
    echo "stop that process manually, or set APP_PORT to the managed instance port"
    exit 1
  fi
  echo "jarvis-agent is not running: pid file not found"
  exit 0
fi

pid=$(cat "$PID_FILE")
if [ -z "$pid" ] || ! kill -0 "$pid" 2>/dev/null; then
  echo "jarvis-agent is not running: stale pid file removed"
  rm -f "$PID_FILE"
  listen_pid=$(find_listen_pid "$APP_PORT")
  if [ -n "$listen_pid" ]; then
    echo "port $APP_PORT is still in use by pid=$listen_pid"
    echo "stop that process manually, or set APP_PORT to the managed instance port"
    exit 1
  fi
  exit 0
fi

echo "stopping jarvis-agent: pid=$pid"
kill "$pid"

elapsed=0
while kill -0 "$pid" 2>/dev/null; do
  if [ "$elapsed" -ge "$STOP_TIMEOUT_SECONDS" ]; then
    echo "force stopping jarvis-agent: pid=$pid"
    kill -9 "$pid" 2>/dev/null || true
    break
  fi
  sleep 1
  elapsed=$((elapsed + 1))
done

rm -f "$PID_FILE"
echo "stopped jarvis-agent"
