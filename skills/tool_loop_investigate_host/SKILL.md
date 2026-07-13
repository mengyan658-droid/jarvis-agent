---
name: tool_loop_investigate_host
version: v1
description: 使用 function calling 工具循环排查单台主机根因。
intents:
  - tool_loop_investigate_host
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

