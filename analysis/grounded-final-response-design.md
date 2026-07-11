# 大模型最终回复 Grounding 设计方案

## 背景

当前 Agent 已经通过 Tool 和 Workflow 获取了结构化运维证据，例如：

```text
Host
HostMetrics
Alarm
ChangeRecord
CMDB metadata
FaultEvidence
FaultAssessment
ToolObservation
```

但如果最终摘要直接交给大模型自由生成，即使 prompt 写了“只能基于输入”，模型仍可能出现：

- 杜撰不存在的故障原因。
- 引入工具结果里没有的组件，例如数据库、磁盘、网络抖动。
- 把普通变更说成高风险变更。
- 把未出现的告警、指标、服务名写进最终回答。
- 对 `FaultAnalyzer` 的确定性评分重新解释甚至改写。
- 在证据不足时给出过度确定的结论。

因此最终回复不能只靠 prompt 约束，而应该设计成：

```text
Tool Result
  -> Evidence Summary
  -> Grounded LLM Output
  -> Local Validator
  -> Final Response / Fallback Response
```

核心目标是：

```text
模型可以组织语言
但不能突破证据边界
```

## 设计目标

### 1. 事实由代码生成

以下内容必须由 Go 代码确定性生成，不交给大模型判断：

```text
host_id
故障分数
故障等级
是否故障
CPU / 内存指标
告警数量
变更数量
时间窗口
CMDB owner/service
FaultAnalyzer reasons
```

### 2. 模型只做语言组织

大模型只负责：

```text
把已给出的事实组织成自然语言
合并相近证据
生成简短可读的诊断描述
在证据不足时说明无法判断
```

### 3. 输出必须可校验

大模型最终输出不能是纯文本，而应该是结构化 JSON。

每个结论和建议都必须引用证据：

```text
evidence_refs
```

本地代码校验通过后，才把它转成最终响应。

### 4. 校验失败必须可回退

如果模型输出不合法、引用不存在的证据、出现未授权事实，本地直接丢弃模型结果，使用 Go 模板生成 fallback summary。

## 当前问题模式

当前常见链路：

```text
Workflow 获取 FaultAssessment
  -> LLM GenerateFaultSummary / GenerateHostDiagnosis
  -> 直接返回模型文本
```

问题在于：

```text
模型输入是事实
模型输出是自由文本
自由文本很难判断哪些内容来自事实，哪些内容来自模型猜测
```

例如输入只有：

```json
{
  "host_id": "host-001",
  "score": 40,
  "level": "degraded",
  "reasons": ["critical_alarms", "cpu_usage_critical"],
  "evidence": {
    "metrics": {
      "cpu_usage_percent": 96
    },
    "alarms": [
      {
        "severity": "critical",
        "message": "CPU saturation"
      }
    ]
  }
}
```

模型却可能输出：

```text
host-001 疑似由于数据库连接池耗尽导致接口超时。
```

这里的“数据库连接池耗尽”和“接口超时”都不在证据中，属于不可靠推断。

## 推荐链路

推荐改成：

```text
1. Workflow 完成 Tool 调用
2. FaultAnalyzer 生成确定性 FaultAssessment
3. EvidenceSummaryBuilder 抽取可引用事实
4. LLM 只基于 EvidenceSummary 输出 GroundedSummary JSON
5. GroundingValidator 校验引用和内容
6. 校验通过：渲染 GroundedSummary
7. 校验失败：渲染 fallback template
```

示意：

```text
FaultAssessment
  -> EvidenceSummary
    -> LLM GroundedSummary
      -> ValidateGroundedSummary
        -> FinalResponse
```

## Evidence Summary 设计

### EvidenceFact

建议定义事实单元：

```go
type EvidenceFact struct {
    ID       string         `json:"id"`
    Type     string         `json:"type"`
    Text     string         `json:"text"`
    Value    any            `json:"value,omitempty"`
    Severity string         `json:"severity,omitempty"`
    Source   string         `json:"source"`
    Metadata map[string]any `json:"metadata,omitempty"`
}
```

字段说明：

```text
id
  稳定事实 ID，例如 fact.host.host-001.cpu_usage

type
  host / metrics / alarm / change / cmdb / assessment / time_range

text
  给模型看的短文本，必须由 Go 代码生成

value
  原始结构化值，用于本地校验

severity
  可选，critical / warning / info

source
  来自哪个 Tool 或本地组件，例如 query_metrics、query_alarms、fault_analyzer
```

### EvidenceSummary

建议定义：

```go
type EvidenceSummary struct {
    SubjectType string         `json:"subject_type"`
    SubjectID   string         `json:"subject_id"`
    TimeRange   domain.TimeRange `json:"time_range,omitempty"`
    Assessment  AssessmentFact `json:"assessment"`
    Facts       []EvidenceFact `json:"facts"`
    Constraints []string       `json:"constraints,omitempty"`
}
```

其中 `AssessmentFact` 可以包含：

```go
type AssessmentFact struct {
    HostID   string   `json:"host_id"`
    Score    int      `json:"score"`
    Level    string   `json:"level"`
    IsFaulty bool     `json:"is_faulty"`
    Reasons  []string `json:"reasons"`
}
```

## Fact ID 设计

Fact ID 要稳定、可校验。

示例：

```text
fact.assessment.host-001.score
fact.assessment.host-001.level
fact.host.host-001.reachable
fact.host.host-001.health_check
fact.metrics.host-001.cpu_usage
fact.metrics.host-001.memory_usage
fact.metrics.host-001.high_cpu_duration
fact.alarm.host-001.critical_count
fact.alarm.host-001.critical.cpu_saturation
fact.change.host-001.recent_count
fact.change.host-001.deploy
fact.cmdb.host-001.owner
fact.cmdb.host-001.service
fact.time_range.current
```

模型最终输出只能引用这些 ID。

## Evidence Summary 示例

以 `host-001` 为例：

```json
{
  "subject_type": "host",
  "subject_id": "host-001",
  "assessment": {
    "host_id": "host-001",
    "score": 40,
    "level": "degraded",
    "is_faulty": true,
    "reasons": [
      "critical_alarms",
      "cpu_usage_critical",
      "cpu_high_load_sustained"
    ]
  },
  "facts": [
    {
      "id": "fact.metrics.host-001.cpu_usage",
      "type": "metrics",
      "text": "CPU 使用率为 96%",
      "value": 96,
      "severity": "critical",
      "source": "query_metrics"
    },
    {
      "id": "fact.metrics.host-001.high_cpu_duration",
      "type": "metrics",
      "text": "CPU 高负载持续 12 分钟",
      "value": 12,
      "severity": "warning",
      "source": "query_metrics"
    },
    {
      "id": "fact.alarm.host-001.critical_count",
      "type": "alarm",
      "text": "当前存在 1 个 critical 告警",
      "value": 1,
      "severity": "critical",
      "source": "query_alarms"
    },
    {
      "id": "fact.cmdb.host-001.service",
      "type": "cmdb",
      "text": "服务名为 payment-api",
      "value": "payment-api",
      "source": "query_cmdb"
    }
  ],
  "constraints": [
    "Only use facts listed in facts.",
    "Do not infer root cause beyond evidence.",
    "Fault score and level must equal assessment."
  ]
}
```

## Grounded Summary 输出设计

大模型不直接输出最终中文文本，而是输出固定 JSON：

```go
type GroundedSummary struct {
    Summary         string              `json:"summary"`
    Conclusion      string              `json:"conclusion"`
    Level           string              `json:"level"`
    IsFaulty        bool                `json:"is_faulty"`
    EvidenceRefs    []string            `json:"evidence_refs"`
    KeyFindings     []GroundedFinding   `json:"key_findings"`
    Recommendations []GroundedRecommend `json:"recommendations,omitempty"`
    Uncertainties   []string            `json:"uncertainties,omitempty"`
}

type GroundedFinding struct {
    Text         string   `json:"text"`
    EvidenceRefs []string `json:"evidence_refs"`
}

type GroundedRecommend struct {
    Text         string   `json:"text"`
    EvidenceRefs []string `json:"evidence_refs"`
}
```

示例输出：

```json
{
  "summary": "host-001 当前评分为 40，等级 degraded，判定为故障。",
  "conclusion": "主要证据是 CPU 使用率达到 96%，且存在 critical 告警。",
  "level": "degraded",
  "is_faulty": true,
  "evidence_refs": [
    "fact.assessment.host-001.score",
    "fact.assessment.host-001.level",
    "fact.metrics.host-001.cpu_usage",
    "fact.alarm.host-001.critical_count"
  ],
  "key_findings": [
    {
      "text": "CPU 使用率达到 critical 阈值。",
      "evidence_refs": [
        "fact.metrics.host-001.cpu_usage"
      ]
    },
    {
      "text": "当前存在 critical 告警。",
      "evidence_refs": [
        "fact.alarm.host-001.critical_count"
      ]
    }
  ],
  "recommendations": [
    {
      "text": "优先检查 CPU 饱和相关进程和最近部署影响。",
      "evidence_refs": [
        "fact.metrics.host-001.cpu_usage"
      ]
    }
  ]
}
```

## Prompt 约束

Prompt 只作为第一层约束，不作为唯一约束。

示例：

```text
你是基础设施运维助手。

你只能基于输入 JSON 中的 facts 和 assessment 生成结论。

规则：
1. 每个 summary、conclusion、key_findings、recommendations 都必须引用 evidence_refs。
2. evidence_refs 必须来自输入 facts 或 assessment 对应的 fact id。
3. 不允许出现输入中不存在的组件、服务、告警、指标、错误码、故障原因。
4. 不允许修改 score、level、is_faulty。
5. 如果证据不足，只能输出“证据不足，无法判断根因”。
6. 输出必须是 JSON，不要输出 Markdown。
```

## 本地校验设计

### Validator 输入

```go
func ValidateGroundedSummary(summary GroundedSummary, evidence EvidenceSummary) error
```

### 校验规则

建议至少校验：

```text
1. JSON 必须能解析。
2. level 必须等于 evidence.assessment.level。
3. is_faulty 必须等于 evidence.assessment.is_faulty。
4. evidence_refs 必须全部存在。
5. key_findings 每一项必须至少引用一个 evidence_ref。
6. recommendations 每一项必须至少引用一个 evidence_ref。
7. summary/conclusion 中不能出现 evidence 中不存在的 host_id。
8. summary/conclusion 中不能出现未授权组件词。
9. 数字型事实不能被改写，例如 CPU 96% 不能写成 99%。
10. 如果 evidence 中没有 change fact，不能说“最近有变更”。
```

### 禁止词和白名单

可以维护一个领域词检查。

禁止模型无证据引入：

```text
数据库
Redis
磁盘
网络抖动
连接池
GC
OOM
丢包
DNS
机房故障
依赖服务异常
```

但如果对应事实中出现这些词，则允许。

实现方式：

```text
forbidden_term 出现在模型输出中
  -> 检查任意 EvidenceFact.Text 或 Value 是否包含该词
  -> 不存在则校验失败
```

### 数字一致性

可以从 evidence 中提取可引用数字：

```text
score=40
cpu=96
memory=76
critical_alarm_count=1
high_cpu_duration=12
```

模型输出中出现数字时，如果不是证据里的数字，则失败或降级。

## Fallback Summary 设计

当模型失败时，不要把失败暴露给用户，也不要返回未校验文本。

用 Go 模板生成：

```text
host-001 当前评分 40，等级 degraded，判定为故障。
关键证据：CPU 使用率 96%；存在 1 个 critical 告警；CPU 高负载持续 12 分钟。
```

模板字段全部来自 `EvidenceSummary`。

示例函数：

```go
func BuildFallbackSummary(e EvidenceSummary) string
```

Fallback 适用于：

```text
LLM 调用失败
LLM 返回非 JSON
LLM 引用不存在证据
LLM 输出出现未授权事实
LLM 修改评分结果
```

## 在当前项目中的落地方案

建议新增：

```text
internal/domain/evidence_summary.go
internal/service/evidence_summary_builder.go
internal/service/grounding_validator.go
internal/service/fallback_summary.go
```

也可以先放在 `internal/workflow` 内部实现，稳定后再抽到 `internal/service`。

### 新增 Domain 对象

```go
type EvidenceFact struct {}
type EvidenceSummary struct {}
type GroundedSummary struct {}
type GroundedFinding struct {}
type GroundedRecommend struct {}
```

### 修改 LLM 接口

当前：

```go
GenerateFaultSummary(ctx context.Context, assessments []domain.FaultAssessment) (string, error)
GenerateHostDiagnosis(ctx context.Context, assessment domain.FaultAssessment) (string, error)
```

建议新增，不直接替换：

```go
GenerateGroundedSummary(ctx context.Context, evidence domain.EvidenceSummary) (domain.GroundedSummary, error)
```

保留旧接口，便于灰度。

### Workflow 接入点

`QueryFaultyHostsWorkflow`：

```text
FaultAssessment[]
  -> BuildEvidenceSummaryList
  -> GenerateGroundedSummary
  -> ValidateGroundedSummary
  -> fallback if invalid
```

`DiagnoseHostWorkflow`：

```text
FaultAssessment
  -> BuildEvidenceSummary
  -> GenerateGroundedSummary
  -> ValidateGroundedSummary
  -> fallback if invalid
```

`ToolLoopInvestigateHostWorkflow`：

```text
ToolObservation + FaultAssessment
  -> BuildEvidenceSummary
  -> GenerateGroundedSummary
  -> ValidateGroundedSummary
  -> fallback if invalid
```

## 与 Tool Result 压缩的关系

Tool Result 压缩解决的是：

```text
不要把很长的后端 response 直接塞进模型上下文
```

Grounded Final Response 解决的是：

```text
即使模型拿到精简证据，也不能自由杜撰最终结论
```

两者关系：

```text
Raw Tool Result
  -> Compact ToolObservation
  -> EvidenceSummary
  -> GroundedSummary
  -> Validated Final Response
```

## 推荐实现顺序

### 阶段 1：只做 EvidenceSummary 和 fallback

先不接 LLM。

```text
FaultAssessment -> EvidenceSummary -> fallback summary
```

目标：

```text
把事实模型稳定下来
```

### 阶段 2：接入 LLM JSON 输出

让模型输出 `GroundedSummary`。

校验失败时 fallback。

### 阶段 3：增强 Validator

增加：

```text
证据引用校验
数字一致性校验
未授权领域词校验
建议动作白名单
```

### 阶段 4：扩展到所有 Workflow

先从 `DiagnoseHostWorkflow` 做，因为单机诊断证据最集中。

再扩展到：

```text
QueryFaultyHostsWorkflow
ToolLoopInvestigateHostWorkflow
```

## 失败策略

建议明确失败策略：

```text
LLM 失败
  -> fallback summary

LLM 非 JSON
  -> fallback summary

LLM JSON schema 不合法
  -> fallback summary

evidence_refs 不存在
  -> fallback summary

出现未授权事实
  -> fallback summary

只是一两个 recommendation 不合法
  -> 删除不合法 recommendation，其余保留
```

保守做法：

```text
任何校验失败都整体 fallback
```

后续可以再做局部降级。

## 示例：杜撰内容如何被拦截

### 输入证据

```json
{
  "facts": [
    {
      "id": "fact.metrics.host-001.cpu_usage",
      "text": "CPU 使用率为 96%"
    },
    {
      "id": "fact.alarm.host-001.critical_count",
      "text": "当前存在 1 个 critical 告警"
    }
  ]
}
```

### 模型输出

```json
{
  "summary": "host-001 可能由于数据库连接池耗尽导致接口超时。",
  "evidence_refs": [
    "fact.metrics.host-001.cpu_usage"
  ]
}
```

### 校验结果

```text
失败
```

原因：

```text
数据库连接池 不在任何 EvidenceFact 中
接口超时 不在任何 EvidenceFact 中
```

返回 fallback：

```text
host-001 当前 CPU 使用率 96%，并存在 1 个 critical 告警。评分结果以 FaultAnalyzer 为准。
```

## 结论

限制大模型最终回复的关键不是写更强的 prompt，而是改变数据流：

```text
事实由代码生成
模型引用事实
本地校验引用
失败使用模板
```

最终目标：

```text
LLM 负责表达
Go 负责事实和边界
```

