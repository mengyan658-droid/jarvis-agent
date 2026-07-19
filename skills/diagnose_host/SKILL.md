---
name: diagnose_host
version: v1
description: 诊断单台主机的故障状态和关键证据。
executor: workflow
intents:
  - diagnose_host
triggers:
  - 诊断 host
  - 分析 host
  - 查看单台主机状态
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
  assessment: object
  evidence: object
guardrails:
  - host_id_must_come_from_user_input
  - do_not_generate_timestamps
  - fault_score_must_use_fault_analyzer
  - read_only_queries_only
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
