# Tool Loop 稳定性优化记录

## 背景

当前项目中的 `tool_loop_investigate_host` 使用原生 function calling：

```text
LLM -> tool_calls
本地执行 Tool
role=tool 结果回传 LLM
LLM 继续 tool_calls 或输出 final
```

这种模式比固定 Workflow 灵活，但如果完全相信模型返回的 `tool_calls`，容易出现重复调用、参数变体绕过去重、工具阶段迟迟不结束等问题。

## 改造前

改造前的 loop 行为是：

```text
1. 每一轮都把完整 tools 列表传给模型。
2. 模型返回什么 tool_calls，本地就直接执行什么。
3. 参数只解析 host_id，没有统一 canonical key。
4. 模型新增参数或修改参数后，可能绕过去重逻辑。
5. assess_fault 完成后，仍可能继续进入下一轮 tool calling。
6. tool result 直接回传给模型，缺少状态化 observation。
```

核心风险：

- 同一个 tool 被反复调用。
- 模型通过增加无关参数制造“新调用”。
- max loop 成为主要兜底机制，而不是最后保护。
- 调用成本和外部依赖压力不可控。

示例问题：

```json
{"name":"query_metrics","arguments":"{\"host_id\":\"host-001\"}"}
{"name":"query_metrics","arguments":"{\"host_id\":\"host-001\",\"reason\":\"double check\"}"}
```

语义上这两个调用是同一个查询，但原始参数不同。如果只按原始参数去重，就会被当成两次调用。

## 改造目标

本次优化目标是把 tool loop 从：

```text
LLM wants -> system executes
```

改为：

```text
LLM proposes -> system normalizes -> system applies policy -> system executes or skips
```

也就是模型只负责提出工具调用，本地负责裁决是否执行。

## 改造后

### 1. 分阶段暴露 Tools

改造后不再每轮传完整 tools。

当前阶段只暴露当前允许调用的工具：

```text
证据未收集完整:
  get_host
  query_metrics
  query_alarms
  query_changes
  query_cmdb

证据收集完整:
  assess_fault

assess_fault 完成:
  不再开放 tools，直接生成 final
```

这样模型即使想重复调用已经完成的工具，也不会在下一轮看到对应 schema。

### 2. 参数规范化

模型返回的 function arguments 不再直接使用。

本地会统一提取并规范化 `host_id`：

```text
host_id
hostID
host
id
```

都会被归一到：

```json
{"host_id":"host-001"}
```

同时会裁剪模型新增的无关字段：

```json
{
  "host_id": "host-001",
  "reason": "double check",
  "include_raw": true
}
```

规范化后仍然是：

```json
{"host_id":"host-001"}
```

### 3. Canonical Key 去重

每个 tool 调用都会生成语义级 canonical key。

当前规则：

```text
get_host:host-001
query_metrics:host-001:last_1h
query_alarms:host-001
query_changes:host-001:last_1h
query_cmdb:host-001
assess_fault:host-001:evidence_v1
```

去重不再依赖模型原始参数，而是依赖本地语义归一化后的 key。

### 4. 重复调用返回 skipped_duplicate

如果模型重复调用已执行过的 tool，本地不会再次执行外部 Tool。

而是返回一个稳定 observation：

```json
{
  "status": "skipped_duplicate",
  "function": "query_metrics",
  "canonical_key": "query_metrics:host-001:last_1h",
  "summary": "query_metrics was already executed; reused observation query_metrics:host-001:last_1h",
  "next_allowed_functions": ["query_alarms", "query_changes", "query_cmdb"]
}
```

这样既能让模型知道重复调用被跳过，也不会浪费外部调用。

### 5. assess_fault 前置条件

`assess_fault` 必须在证据工具完成后才能执行。

必需证据工具：

```text
get_host
query_metrics
query_alarms
query_changes
query_cmdb
```

如果模型提前调用 `assess_fault`，本地会返回：

```json
{
  "status": "blocked_prerequisite",
  "function": "assess_fault",
  "summary": "cannot assess fault before required evidence tools complete",
  "next_allowed_functions": [...]
}
```

### 6. 严格 Tool Schema

tool schema 增加：

```json
"additionalProperties": false
```

当前只允许模型填：

```json
{
  "host_id": "host-001"
}
```

注意：schema 只是提示和约束模型输出，本地仍然会做参数裁剪和规范化。

### 7. Observation 状态化

Tool result 回传给模型时，不再只是原始结果。

会包装成结构化 observation：

```json
{
  "status": "complete",
  "function": "query_metrics",
  "canonical_key": "query_metrics:host-001:last_1h",
  "summary": "cpu=96 memory=76 high_cpu_minutes=12",
  "next_allowed_functions": ["query_alarms", "query_changes", "query_cmdb"]
}
```

完整证据仍保留在本地 `state.evidence` 中，用于最终 `FaultAnalyzer` 确定性评分。

## 改造效果

改造后具备这些稳定性保障：

- 模型不能通过新增无关参数绕过去重。
- 同一语义 tool call 只会真实执行一次。
- 已完成阶段的工具不会继续暴露给模型。
- `assess_fault` 完成后不会继续 tool loop。
- max loop 从主要兜底变成最后保护。
- 响应里的 `function_call_trace` 会展示调用状态：

```json
{
  "function": "query_metrics",
  "canonical_key": "query_metrics:host-001:last_1h",
  "status": "skipped_duplicate",
  "observation": "query_metrics was already executed; reused observation query_metrics:host-001:last_1h"
}
```

## 测试覆盖

新增测试覆盖模型重复调用场景：

```text
模型第一轮同时返回：
  query_metrics({"host_id":"host-001"})
  query_metrics({"host_id":"host-001","reason":"double check","include_raw":true})

预期：
  query_metrics 真实 Tool 只执行一次
  第二次调用标记为 skipped_duplicate
```

测试文件：

```text
internal/workflow/tool_loop_investigate_host_test.go
```

## 涉及代码

主要文件：

```text
internal/workflow/tool_loop_investigate_host.go
internal/workflow/tool_loop_investigate_host_test.go
README.md
```

关键函数：

```text
availableInvestigationTools
handleFunctionCall
normalizeFunctionCall
canonicalFunctionKey
functionHostID
missingEvidenceFunctions
nextAllowedFunctions
```

## 后续可继续优化

后续还可以继续增强：

- 为所有 Tool 增加独立的参数 schema 和 validator。
- 把 canonical key 逻辑下沉到 ToolRegistry。
- 增加 Tool result cache，有效期内直接复用。
- 增加 per-tool 调用预算，例如每个 host 的 metrics 最多一次。
- 对 skipped/blocked observation 做更明确的 LLM 反馈模板。
- 将 evidence store 独立成结构，避免 Workflow 继续膨胀。
