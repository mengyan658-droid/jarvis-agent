---
name: tool_loop_investigate_host
version: v1
description: 使用 function calling 工具循环排查单台主机根因。
executor: tool_loop
intents:
  - tool_loop_investigate_host
triggers:
  - 排查 host 根因
  - 看一下 host 根因
  - 调查单台主机异常
workflow: tool_loop_investigate_host
tools:
  - resolve_time_range
  - get_host
  - query_metrics
  - query_alarms
  - query_changes
  - query_cmdb
read_only: true
output_policy: grounded
parameters:
  - name: host_id
    type: string
    required: true
    pattern: "^host-[0-9]{3}$"
    description: 用户输入中的主机 ID，例如 host-001。
  - name: since
    type: string
    required: false
    description: 相对时间范围，例如 5h、30m、2d、1w。
  - name: start_text
    type: string
    required: false
    description: 明确起止时间的开始时间原文。
  - name: end_text
    type: string
    required: false
    description: 明确起止时间的结束时间原文。
output_schema:
  summary: string
  function_call_trace: array
  assessment: object
guardrails:
  - only_use_declared_read_only_tools
  - do_not_repeat_same_canonical_tool_key
  - do_not_generate_timestamps
  - fault_score_must_use_assess_fault
---

# When To Use

当用户说“排查 host-xxx”“看一下 host-xxx 根因”或固定 workflow 无法识别意图但包含 host_id 时使用。

# Parameters

- host_id: 必填，例如 host-001。
- since: 可选，例如 5h、30m、2d、1w。
- start_text/end_text: 可选，用于明确起止时间。

# Rules

- 只允许调用声明的只读查询工具。
- 不要重复调用同一个 canonical tool key。
- 不要生成时间戳。
- 最终评分必须以 assess_fault / FaultAnalyzer 为准。
