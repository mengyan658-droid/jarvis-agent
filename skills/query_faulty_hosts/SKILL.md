---
name: query_faulty_hosts
version: v1
description: 查询指定区域、环境、时间范围内的故障机器列表。
intents:
  - query_faulty_hosts
workflow: query_faulty_hosts
tools:
  - resolve_time_range
  - query_hosts
  - query_metrics
  - query_alarms
  - query_changes
  - query_cmdb
read_only: true
output_policy: grounded
---

# When To Use

当用户想查询故障机、异常机器、故障主机列表时使用。

# Parameters

- region: 可选，支持 east-china、north-china、south-china。
- environment: 可选，支持 production、staging。
- since: 可选，例如 5h、30m、2d、1w。
- start_text/end_text: 可选，用于明确起止时间。

# Rules

- 不要生成时间戳。
- 相对时间使用 since。
- 明确起止时间保留用户原文到 start_text/end_text。
- 只读查询，不允许重启、删除、隔离等写操作。

