#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
LOG_DIR="${LOG_DIR:-$ROOT_DIR/logger}"
LOG_DATE="${LOG_DATE:-$(date +%F)}"
INPUT_FILE="${1:-$LOG_DIR/jarvis-agent-$LOG_DATE.log}"
OUTPUT_FILE="${2:-$LOG_DIR/jarvis-agent-$LOG_DATE.pretty.log}"

if [ ! -f "$INPUT_FILE" ]; then
  echo "log file not found: $INPUT_FILE" >&2
  exit 1
fi

python3 - "$INPUT_FILE" "$OUTPUT_FILE" <<'PY'
import json
import sys

input_file, output_file = sys.argv[1], sys.argv[2]

def expand_nested_json(value):
    if isinstance(value, dict):
        return {k: expand_nested_json(v) for k, v in value.items()}
    if isinstance(value, list):
        return [expand_nested_json(v) for v in value]
    if isinstance(value, str):
        stripped = value.strip()
        if (stripped.startswith("{") and stripped.endswith("}")) or (stripped.startswith("[") and stripped.endswith("]")):
            try:
                return expand_nested_json(json.loads(stripped))
            except json.JSONDecodeError:
                return value
    return value

with open(input_file, "r", encoding="utf-8") as src, open(output_file, "w", encoding="utf-8") as dst:
    for line_no, line in enumerate(src, 1):
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
            event = expand_nested_json(event)
            dst.write(json.dumps(event, ensure_ascii=False, indent=2))
            dst.write("\n\n")
        except json.JSONDecodeError:
            dst.write(f"# line {line_no}: non-json log\n")
            dst.write(line)
            dst.write("\n\n")
PY
echo "pretty log written: $OUTPUT_FILE"
