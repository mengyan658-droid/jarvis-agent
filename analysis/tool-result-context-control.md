# Tool Result 上下文控制分析

## 背景

在 function calling / tool loop 场景中，Tool 通常是对后端服务接口的封装。后端接口返回的数据面向程序消费，可能包含大量字段、分页列表、内部状态、历史明细、debug 字段等。

如果直接把后端 response 当作 `role=tool` 的 content 回传给大模型，会导致：

- 占用大量上下文窗口。
- 增加 token 成本和响应延迟。
- 模型注意力被无关字段分散。
- 最终回答容易复述原始字段。
- 多轮 tool loop 中更容易丢失核心状态。
- 长 response 会挤掉 system prompt、用户意图和前面的 observation。

因此，Tool Result 不应该等同于后端 API Response。

## 当前问题模式

当前常见实现是：

```text
LLM 返回 tool_calls
本地执行 Tool
Tool 直接调用后端服务
后端 response 直接作为 role=tool content 回传给 LLM
LLM 基于完整 response 继续调用工具或生成最终答案
```

问题在于后端 response 往往不是“给模型看的观察结果”，而是“给程序用的原始数据”。

例如告警接口可能返回：

```json
[
  {
    "id": "alm-001",
    "host_id": "host-001",
    "severity": "critical",
    "labels": {},
    "annotations": {},
    "raw_expression": "...",
    "dashboard_url": "...",
    "fingerprint": "...",
    "internal_rule_id": "...",
    "tenant_id": "...",
    "created_by": "...",
    "updated_at": "...",
    "history": []
  }
]
```

但模型真正需要的通常只是：

```json
{
  "summary": "1 critical alarm: CPU saturation, started 20 minutes ago",
  "counts": {
    "critical": 1,
    "warning": 0
  },
  "key_findings": [
    "CPU saturation alarm is active"
  ]
}
```

## 核心原则

应明确区分三层数据：

```text
Backend Response
  -> Tool Raw Result
    -> Compact Observation
      -> LLM role=tool content
```

也就是说：

```text
完整 response 留在本地
只把精简 observation 给模型
```

不要再使用：

```text
role=tool content = backend response
```

而应使用：

```json
{
  "status": "complete",
  "function": "query_metrics",
  "canonical_key": "query_metrics:host-001:last_1h",
  "summary": "CPU 96%, memory 76%, high CPU duration 12 minutes",
  "key_findings": [
    "CPU usage is critical",
    "High CPU load lasted 12 minutes"
  ],
  "next_allowed_functions": [
    "query_alarms",
    "query_changes",
    "query_cmdb"
  ]
}
```

## 推荐设计

### 1. 引入 Observation Builder

给每个 Tool 增加面向模型的结果压缩层。

推荐链路：

```text
Tool 执行
  -> 得到 raw result
  -> raw result 存入本地 state/evidence store
  -> ObservationBuilder 生成 compact observation
  -> compact observation 回传 LLM
```

示例结构：

```go
type ToolObservation struct {
    Status               string         `json:"status"`
    Function             string         `json:"function"`
    CanonicalKey         string         `json:"canonical_key"`
    Summary              string         `json:"summary"`
    KeyFindings          []string       `json:"key_findings,omitempty"`
    Counts               map[string]int `json:"counts,omitempty"`
    ResultRef            string         `json:"result_ref,omitempty"`
    Truncated            bool           `json:"truncated,omitempty"`
    NextAllowedFunctions []string       `json:"next_allowed_functions,omitempty"`
}
```

其中 `ResultRef` 用于引用本地保存的完整结果：

```text
obs-003
```

模型只看到：

```json
{
  "result_ref": "obs-003",
  "summary": "3 active alarms, 1 critical",
  "key_findings": [
    "CPU saturation alarm is active"
  ]
}
```

完整结果保存在本地：

```text
ObservationStore["obs-003"] = rawResult
```

### 2. 每个 Tool 定制摘要

不要主要依赖通用 JSON 截断。结构化运维数据应优先用规则化摘要。

建议各 Tool 的 observation 内容：

```text
get_host:
  host_id
  reachable
  health_check_passed
  region
  environment
  owner/service

query_metrics:
  cpu
  memory
  high_cpu_duration
  threshold findings

query_alarms:
  count by severity
  top critical/warning messages
  earliest/latest started time

query_changes:
  count
  recent high-risk changes
  deploy/config info
  time correlation hint

query_cmdb:
  owner
  service
  tier
  business metadata summary
```

### 3. 保留硬截断兜底

定制摘要之外，还需要硬限制。

建议：

```text
单个 observation 最大 3000 chars
超过则 truncated=true
```

示例：

```go
func truncate(s string, max int) (string, bool) {
    if len(s) <= max {
        return s, false
    }
    return s[:max] + "...[truncated]", true
}
```

截断应作为最后保险，而不是主要摘要策略。

### 4. 对大结果工具增加参数限制

对于天然容易返回大量数据的工具，例如日志检索、工单列表、告警历史，应在 Tool 参数层做限制：

```text
limit
fields
time_range
severity
```

但这些参数不能完全交给模型自由填。

本地应设置默认值和上限：

```text
limit: default 20, max 50
time_range: default 1h, max 24h
fields: fixed allowlist
```

示例：

```go
if args.Limit <= 0 || args.Limit > 20 {
    args.Limit = 20
}
```

### 5. 最终总结避免塞完整 raw result

最终总结时，模型不应该再次看到完整后端 response。

推荐只给：

```json
{
  "assessment": {
    "score": 50,
    "level": "degraded",
    "is_faulty": true,
    "reasons": []
  },
  "key_findings": [],
  "tool_observations": []
}
```

在当前项目中，故障评分已经由 `FaultAnalyzer` 确定性完成，因此最终模型只需要基于 `assessment` 和 compact observations 生成自然语言结论。

## 当前项目落地建议

当前项目已经有：

```go
type ToolObservation struct {
    Status               string   `json:"status"`
    Function             string   `json:"function"`
    CanonicalKey         string   `json:"canonical_key"`
    Summary              string   `json:"summary"`
    NextAllowedFunctions []string `json:"next_allowed_functions,omitempty"`
}
```

这是正确方向。下一步建议扩展为：

```go
type ToolObservation struct {
    Status               string         `json:"status"`
    Function             string         `json:"function"`
    CanonicalKey         string         `json:"canonical_key"`
    Summary              string         `json:"summary"`
    KeyFindings          []string       `json:"key_findings,omitempty"`
    Counts               map[string]int `json:"counts,omitempty"`
    ResultRef            string         `json:"result_ref,omitempty"`
    Truncated            bool           `json:"truncated,omitempty"`
    NextAllowedFunctions []string       `json:"next_allowed_functions,omitempty"`
}
```

### 当前代码中的具体落点

当前 `tool_loop_investigate_host.go` 里已经不是直接把 raw result 回填给模型，而是先构造 `ToolObservation`：

```text
handleFunctionCall
  -> executeNormalizedFunctionCall
  -> summarizeToolResult
  -> client.MarshalToolResult(observation)
  -> append role=tool message
```

其中真正需要改造的是 `summarizeToolResult` 这一层。它现在只生成一个很短的 `summary`，例如：

```text
active_alarms=3
cpu=96 memory=76 high_cpu_minutes=12
```

这比直接塞完整 response 更安全，但仍然不够稳定，因为模型只能看到很粗的信息，无法区分关键证据和普通信息。

推荐改成：

```text
handleFunctionCall
  -> executeNormalizedFunctionCall
  -> raw result 写入 state.evidence 或 rawResults
  -> buildToolObservation
  -> role=tool content 只写 compact observation
```

也就是说，`summarizeToolResult` 不再只是返回字符串，而是升级为按 Tool 类型生成结构化 observation。

并在 `handleFunctionCall` 中调整为：

```text
rawResult := executeNormalizedFunctionCall(...)
state.rawResults[canonicalKey] = rawResult
observation := buildObservation(...)
role=tool content = observation
```

### 压缩流程

推荐压缩流程如下：

```text
1. Tool 执行，拿到完整 raw result
2. 完整 raw result 留在本地 state 中
3. 根据 Tool 类型提取关键字段
4. 生成 Summary、Counts、KeyFindings、ResultRef
5. 对 observation 做长度限制
6. 只把 observation 写入 role=tool
```

注意：不要让模型从完整 JSON 中自己找重点。运维场景里的重点信息，例如 CPU 是否超过阈值、告警数量、最近一小时是否有变更、故障评分，应该由 Go 代码确定性提取。

### 各 Tool 的压缩规则

建议每个 Tool 使用不同的压缩规则。

```text
get_host:
  保留 host_id、region、environment、reachable、health_check_passed
  key_findings 只记录不可达、健康检查失败等异常

query_metrics:
  保留 cpu、memory、high_cpu_duration_minutes
  key_findings 记录 CPU >= 95%、CPU >= 85%、内存 >= 95%、高负载持续 >= 10 分钟

query_alarms:
  保留 severity 计数
  critical 告警优先写入 key_findings
  warning 告警最多保留前 3 条摘要
  不把 labels、annotations、history、dashboard_url 等完整字段交给模型

query_changes:
  保留最近变更数量、最近一条变更、变更类型、是否在最近一小时
  key_findings 只记录和故障时间窗口相关的变更

query_cmdb:
  保留 owner、service、tier、重要业务标签
  不把全部 CMDB 元数据透传给模型

assess_fault:
  保留 score、level、is_faulty、reasons
  评分结果必须来自 FaultAnalyzer，不能让模型重新评分
```

### 推荐的 role=tool 内容

以 `query_alarms` 为例，模型看到的内容应该接近：

```json
{
  "status": "complete",
  "function": "query_alarms",
  "canonical_key": "query_alarms:host-001",
  "summary": "active alarms: 4, critical: 1, warning: 3",
  "counts": {
    "critical": 1,
    "warning": 3
  },
  "key_findings": [
    "critical alarm: CPU saturation",
    "multiple warning alarms are active"
  ],
  "result_ref": "query_alarms:host-001",
  "next_allowed_functions": [
    "query_changes",
    "query_cmdb"
  ]
}
```

完整告警列表仍然保存在本地 `state.evidence.Alarms` 中，后续 `assess_fault` 可以继续使用完整结构化数据，但模型不需要看到所有原始字段。

### 长度限制

结构化压缩之外，还要加硬限制兜底。

建议：

```text
summary 最多 300 字符
key_findings 最多 5 条
单条 key_finding 最多 200 字符
整个 observation 最多 2000 到 3000 字符
超过限制时设置 truncated=true
```

这层限制是最后保险，不是主要压缩策略。主要压缩仍然应该靠每个 Tool 的规则化提取。

### 示例：Metrics Observation

```go
func buildMetricsObservation(key string, metrics domain.HostMetrics) ToolObservation {
    findings := []string{}
    if metrics.CPUUsagePercent >= 95 {
        findings = append(findings, "CPU usage is critical")
    }
    if metrics.HighCPUDurationMinutes >= 10 {
        findings = append(findings, "CPU high load is sustained")
    }
    return ToolObservation{
        Status:       "complete",
        Function:     "query_metrics",
        CanonicalKey: key,
        Summary: fmt.Sprintf(
            "cpu=%.0f memory=%.0f high_cpu_minutes=%d",
            metrics.CPUUsagePercent,
            metrics.MemoryUsagePercent,
            metrics.HighCPUDurationMinutes,
        ),
        KeyFindings: findings,
    }
}
```

### 示例：Alarms Observation

```go
func buildAlarmsObservation(key string, alarms []domain.Alarm) ToolObservation {
    counts := map[string]int{}
    findings := []string{}
    for _, alarm := range alarms {
        counts[alarm.Severity]++
        if alarm.Severity == "critical" {
            findings = append(findings, alarm.Message)
        }
    }
    return ToolObservation{
        Status:       "complete",
        Function:     "query_alarms",
        CanonicalKey: key,
        Summary:      fmt.Sprintf("active_alarms=%d critical=%d warning=%d", len(alarms), counts["critical"], counts["warning"]),
        Counts:       counts,
        KeyFindings:  findings,
    }
}
```

## 是否需要 LLM 二次摘要

一般不建议每个 tool result 都再调用一次 LLM 摘要，因为会增加成本和延迟。

优先级建议：

```text
1. 结构化字段摘要
2. 本地规则压缩
3. 字符长度截断
4. 只有非结构化长文本时，才考虑 LLM summarizer
```

例如日志检索这种非结构化文本，可以考虑：

```text
raw logs -> local topK/filter -> LLM summarize -> compact observation
```

## 结论

Tool result 的设计目标不是“完整”，而是“足够模型继续推理”。

推荐原则：

```text
后端完整 response 留本地
LLM 只看 compact observation
最终评分由确定性代码完成
最终总结基于 assessment + key findings
```

这能同时降低上下文占用、减少成本、提升最终回答稳定性，并避免模型被后端 response 的噪声字段污染。
