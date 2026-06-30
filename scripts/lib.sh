#!/usr/bin/env sh

find_listen_pid() {
  port="$1"
  if command -v lsof >/dev/null 2>&1; then
    lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null | head -n 1
    return 0
  fi
  return 0
}
