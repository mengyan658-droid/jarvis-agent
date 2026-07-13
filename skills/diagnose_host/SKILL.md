---
name: diagnose_host
version: v1
description: 诊断单台主机的故障状态和关键证据。
intents:
  - diagnose_host
workflow: diagnose_host
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

当用户说“诊断 host-xxx”“分析 host-xxx”或希望查看单台主机状态时使用。

# Parameters

- host_id: 必填，例如 host-001。
- since: 可选，例如 5h、30m、2d、1w。
- start_text/end_text: 可选，用于明确起止时间。

# Rules

- host_id 必须来自用户输入，不要编造。
- 不要生成时间戳。
- 最终评分必须以 FaultAnalyzer 为准。
- 只读查询，不允许重启、删除、隔离等写操作。

