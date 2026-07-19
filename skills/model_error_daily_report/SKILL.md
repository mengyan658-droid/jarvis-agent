---
name: model_error_daily_report
version: v1
description: 生成最近 24 小时内指定机型和错误码的请求错误数量日报，并从机型和时间维度分析。
executor: guided_steps
intents:
  - model_error_daily_report
triggers:
  - 生成机型错误码日报
  - 查询指定机型错误码数量日报
  - 统计最近24小时错误码请求数量
workflow: model_error_daily_report
tools:
  - resolve_time_range
  - query_error_request_counts
read_only: true
output_policy: grounded
parameters:
  - name: device_models
    type: string
    required: true
    description: 机型列表，多个机型使用逗号分隔，例如 iphone-15,iphone-14。
  - name: idcs
    type: string
    required: false
    description: IDC 列表，多个 IDC 使用逗号分隔，例如 shanghai-a,beijing-a。
  - name: error_code
    type: string
    required: true
    description: 错误码，例如 E500。
  - name: since
    type: string
    required: false
    description: 时间范围，默认最近 24 小时，也可传 24h。
  - name: aggregation_value
    type: string
    required: false
    description: 时间聚合粒度数值，默认 1。
  - name: aggregation_unit
    type: string
    required: false
    enum: m,h,d
    description: 时间聚合单位，默认 h。
output_schema:
  summary: string
  total_count: integer
  by_device_model: array
  by_time: array
  records: array
guardrails:
  - use_resolve_time_range_for_time_window
  - do_not_generate_timestamps
  - query_read_only_counts_only
  - final_report_must_use_tool_results
---

# When To Use

当用户要求生成、查看、统计“最近 24 小时”内某些机型在某个错误码上的请求错误数量日报时使用。

# Parameters

- device_models: 必填，机型列表，例如 iphone-15 或 iphone-15,iphone-14。
- error_code: 必填，错误码，例如 E500。
- idcs: 可选，IDC 列表，例如 shanghai-a,beijing-a。
- since: 可选，默认最近 24 小时。
- aggregation_value/aggregation_unit: 可选，默认按 1 小时聚合。

# Rules

- 不要生成时间戳。
- 如果用户没有明确时间范围，使用最近 24 小时。
- 机型和错误码必须来自用户输入，不要编造。
- 日报至少包含机型维度和时间维度分析。
- 只读查询，不允许执行写操作。
