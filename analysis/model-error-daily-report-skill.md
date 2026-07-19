# 机型错误码数量日报 Skill 实现现状

## 背景

当前项目新增了 `model_error_daily_report` Skill，用于生成指定机型和错误码的请求错误数量日报。

这个 Skill 的目标不是只返回原始 JSON，而是生成一份可供前端直接渲染的 Markdown 报告，同时保留结构化统计数据供前端绘制图表。

当前 Skill 文件：

```text
skills/model_error_daily_report/SKILL.md
```

核心 Workflow：

```text
internal/workflow/model_error_daily_report.go
```

底层查询 Tool：

```text
query_error_request_counts
```

## Skill 声明

当前声明：

```yaml
name: model_error_daily_report
version: v1
executor: guided_steps
workflow: model_error_daily_report
tools:
  - resolve_time_range
  - query_error_request_counts
read_only: true
output_policy: grounded
```

关键点：

- `executor=guided_steps`：表示 Skill 触发后，不是完全固定参数执行，而是让模型参与步骤内的 Tool 参数规划。
- `workflow=model_error_daily_report`：当前仍复用 Workflow 作为执行容器。
- `tools` 只允许时间解析和错误请求数量查询两个只读工具。
- `output_policy=grounded`：最终报告必须基于 Tool 查询结果和本地统计事实。

## 触发方式

当前主要通过 `select_skill` 触发。

请求进入 Runtime 后：

```text
Runtime.Query
  -> selectIntent
  -> LLM ChatWithTools(select_skill)
  -> skill=model_error_daily_report
  -> SkillExecutor
  -> executor=guided_steps
  -> ModelErrorDailyReportWorkflow
```

Mock LLM 中也加入了规则化识别：

```text
包含“日报”
或同时包含“错误码”和“机型”
  -> model_error_daily_report
```

同时会尝试提取：

```text
device_models
idcs
error_code
aggregation_value
aggregation_unit
```

真实 LLM 的 fallback `ParseIntent` prompt 也已加入该 intent。

## 执行链路

当前链路如下：

```text
用户输入
  -> select_skill 选择 model_error_daily_report
  -> SkillExecutor 根据 executor=guided_steps 分发
  -> Workflow 接收原始用户输入 message
  -> 模型规划时间参数
  -> 本地执行 resolve_time_range
  -> 模型规划查询参数
  -> 本地执行 query_error_request_counts
  -> Go 本地统计维度事实
  -> LLM 基于事实生成 Markdown 报告
  -> LLM 失败则使用 Go fallback Markdown
  -> API 返回 code/msg/data
```

## Guided Steps 当前实现

当前 `guided_steps` 已经抽象出代码级通用执行器 `GuidedStepExecutor`：

```text
internal/workflow/guided_step_executor.go
```

它负责：

- 统一记录 execution steps。
- 统一写入 workflow warnings。
- 统一通过 LLM function calling 规划某个指定 Tool 的参数。
- 统一执行本地 ToolRegistry 中的 Tool。
- 统一解码模型返回的 tool call arguments。

日报 Workflow 仍然保留业务编排定义，也就是由 `model_error_daily_report` 明确声明有哪些 guided steps、每个 step 的业务状态如何合并。

### 1. plan_report_time_range

作用：

```text
让模型根据用户原文规划 resolve_time_range 的参数。
```

模型只拿到一个允许调用的工具：

```text
resolve_time_range
```

模型需要返回类似：

```json
{
  "kind": "relative",
  "amount": 24,
  "unit": "hour"
}
```

或：

```json
{
  "kind": "relative",
  "amount": 1,
  "unit": "week"
}
```

本地强约束：

- 模型不能生成时间戳。
- 如果模型失败，回退到 Runtime/Intent 中已经规范化的参数。
- 如果用户没有明确时间，默认最近 24 小时。
- 最终时间戳由 `resolve_time_range` Tool 在本地计算。

### 2. resolve_report_time_range

作用：

```text
执行 resolve_time_range Tool，得到确定的 domain.TimeRange。
```

这一步是本地 Tool 调用，不由模型直接产生最终时间戳。

输出包含：

```text
start_time
end_time
timezone
duration_sec
start_time_sec
end_time_sec
start_time_ms
end_time_ms
```

### 3. plan_error_count_query

作用：

```text
让模型根据用户原文规划 query_error_request_counts 的业务查询参数。
```

模型只拿到一个允许调用的工具：

```text
query_error_request_counts
```

模型需要返回：

```json
{
  "device_models": ["iphone-15"],
  "idcs": ["shanghai-a"],
  "error_code": "E500",
  "aggregation_value": 1,
  "aggregation_unit": "h"
}
```

本地会继续校验：

- `device_models` 必填。
- `error_code` 必填。
- `aggregation` 必须合法。
- `idcs` 可选。

如果模型规划失败，会使用 `select_skill` 阶段抽取出的参数作为 fallback。

### 4. query_error_request_counts

作用：

```text
执行错误请求数量查询。
```

查询条件：

```text
time_range
device_models
idcs
error_code
aggregation
```

底层 mock 数据结构：

```go
ErrorRequestCountSample{
    Timestamp,
    DeviceModel,
    IDC,
    ErrorCode,
    Count,
}
```

Mock 查询会：

- 按时间范围过滤。
- 按机型数组过滤。
- 按 IDC 数组过滤。
- 按错误码过滤。
- 按聚合粒度切时间桶。
- 按 `时间桶 + 机型 + IDC + 错误码` 聚合 count。

输出记录：

```json
{
  "bucket_start": "...",
  "bucket_end": "...",
  "device_model": "iphone-15",
  "idc": "shanghai-a",
  "error_code": "E500",
  "count": 30
}
```

### 5. analyze_model_dimension

作用：

```text
Go 本地统计机型维度、IDC 维度和总数。
```

输出：

```text
total_count
by_device_model
by_idc
records
```

这里不依赖 LLM，保证数字确定。

### 6. analyze_time_dimension

作用：

```text
Go 本地按时间桶汇总趋势数据。
```

输出：

```text
by_time
```

当前时间趋势只做了时间桶求和和排序，峰值、低谷等分析在 Markdown 生成阶段使用这些确定性数据完成。

### 7. generate_daily_report_summary

作用：

```text
让 LLM 基于本地统计事实生成 Markdown 报告。
```

传给 LLM 的不是原始后端大响应，而是 `ModelErrorDailyReportResult` 这种结构化事实。

LLM prompt 要求：

- 只输出 Markdown。
- 不输出 JSON。
- 不包裹代码块。
- 必须使用固定标题。
- 数字、错误码、机型、IDC、时间范围必须以输入为准。
- 不允许编造。
- 明细表最多展示 20 行。

如果 LLM 失败：

```text
fallbackModelErrorDailyReportMarkdown
```

会用 Go 模板生成确定性 Markdown。

## 返回给前端的数据

HTTP API 外层仍然是统一响应：

```json
{
  "code": "OK",
  "msg": "ok",
  "data": {}
}
```

日报结果在：

```text
data.results
```

核心字段：

```text
data.summary
data.results.report_markdown
data.results.total_count
data.results.by_device_model
data.results.by_idc
data.results.by_time
data.results.records
data.results.query
```

其中：

- `data.summary` 当前也是 Markdown 报告正文。
- `data.results.report_markdown` 是推荐给前端渲染的 Markdown 报告。
- `by_device_model` 可用于机型维度柱状图。
- `by_time` 可用于时间趋势折线图。
- `by_idc` 可用于 IDC 维度柱状图。
- `records` 可用于明细表格。

## Markdown 报告结构

当前要求报告包含固定标题：

```markdown
# 机型错误码数量日报

## 概览
## 查询条件
## 机型维度分析
## 时间趋势分析
## IDC 维度分析
## 明细数据
## 结论
```

前端可以直接将：

```text
data.results.report_markdown
```

交给 Markdown 渲染组件。

## 当前稳定性设计

当前已经做了以下约束：

### 1. Skill 工具边界

`model_error_daily_report` 只声明：

```text
resolve_time_range
query_error_request_counts
```

### 2. 步骤内工具限制

`plan_report_time_range` 只暴露：

```text
resolve_time_range
```

`plan_error_count_query` 只暴露：

```text
query_error_request_counts
```

模型不能在这两个步骤里自由调用其他工具。

### 3. 时间戳本地计算

模型只生成：

```text
kind
amount
unit
start_text
end_text
```

真正的时间戳由本地 `resolve_time_range` 生成。

### 4. 统计事实本地计算

这些数据由 Go 代码确定性计算：

```text
total_count
by_device_model
by_idc
by_time
records
```

LLM 不参与求和、排序和聚合。

### 5. Markdown 失败兜底

如果 LLM 报告生成失败：

```text
warnings += llm daily report failed; used fallback markdown report
```

并使用 Go 模板生成 Markdown。

## 当前限制

### 1. guided_steps 还不是完全数据驱动执行器

现在已经有代码级通用 `GuidedStepExecutor`，但 step 列表仍由日报 Workflow 在 Go 代码中声明。

也就是说：

```text
executor=guided_steps
  -> SkillExecutor
  -> model_error_daily_report workflow
  -> workflow 使用 GuidedStepExecutor.Run(...) 执行受控步骤
```

还没有实现完全数据驱动的：

```text
Skill steps
  -> Generic GuidedStepExecutor
  -> 根据 SKILL.md steps 自动执行
```

这是刻意保留的边界：当前阶段先抽出可复用执行机制，但业务参数合并、校验、聚合和报告事实结构仍放在 Go 代码中，避免过早把复杂业务逻辑放进 `SKILL.md`。

### 2. 参数 schema 未统一校验

`SKILL.md` 已经声明 parameters，但运行时还没有通用 `SkillValidator` 按 schema 做校验。

当前日报 Workflow 内部手写校验：

```text
device_models required
error_code required
aggregation valid
```

### 3. LLM 规划参数仍可能出错

模型可能：

- 漏提机型。
- 漏提错误码。
- 把 IDC 识别错。
- 聚合单位传错。

当前本地有 fallback 和部分校验，但还没有完整的参数纠错或澄清问题机制。

### 4. Markdown 报告仍可能不完全稳定

最终 Markdown 如果由真实 LLM 生成，即使输入事实相同，也可能出现措辞、侧重点不一致。

当前控制方式是：

- temperature 使用较低值。
- prompt 要求基于输入事实。
- LLM 失败时 fallback。

但还没有做：

- Markdown 输出 schema 校验。
- 数字一致性校验。
- 标题完整性校验。
- 引用来源校验。

如果要求同一查询每次输出完全一致，应优先使用 Go fallback 模板作为主报告，LLM 只做可选润色。

### 5. 大数据量压缩策略还需要增强

当前传给 LLM 的 facts 包含 `records`。

如果查询结果非常大，后续应该改为：

```text
raw records 留在本地
只传 report_facts 给 LLM
records 最多传 topN 或 sample
```

推荐传给 LLM 的结构：

```json
{
  "total_count": 12345,
  "top_models": [],
  "top_idcs": [],
  "time_trend": {
    "peak_bucket": {},
    "low_bucket": {},
    "trend": "increasing"
  },
  "truncated": true,
  "record_count": 50000
}
```

## 示例请求

```bash
curl -s -X POST http://localhost:8080/api/v1/agent/query \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: u-001' \
  -H 'X-User-Role: sre' \
  -H 'X-Session-ID: s-001' \
  -d '{"message":"生成最近24小时 iphone-15 错误码 E500 的数量日报"}'
```

也支持更灵活的自然语言：

```text
生成最近一周 iphone-15,iphone-14 错误码 E500 的数量日报
生成昨天 shanghai-a 的 iphone-15 错误码 E500 日报
统计最近24小时 shanghai-a,beijing-a 机房 iphone-15 错误码 E500 的日报
```

## 推荐下一步

建议按优先级继续改造：

1. 引入通用 `SkillValidator`，按 `SKILL.md parameters` 做 required、enum、pattern 校验。
2. 把日报的 guided steps 抽象成通用 `GuidedStepExecutor`。
3. 引入 `ReportFacts`，限制传给 LLM 的 facts 大小。
4. 对 LLM Markdown 做标题、数字和事实一致性校验。
5. 增加参数不足时的澄清响应，例如缺少机型或错误码时不直接报错，而是返回“请提供机型和错误码”。
6. 支持前端视图模型，例如 `summary_cards`、`charts`、`tables`、`insights`。

## 结论

当前日报 Skill 已经不是纯固定 Workflow，也不是开放式 ReAct。

它是一个受控的 guided steps 实现：

```text
模型负责理解用户输入并规划 Tool 参数
Go 负责执行 Tool、计算事实、校验参数和兜底
LLM 负责把事实转成 Markdown 报告
前端负责渲染 Markdown 和结构化图表数据
```

这个形态比固定 Workflow 灵活，也比无限 loop ReAct 稳定，适合当前“查询参数依赖用户自然语言，但业务分析需要确定性”的日报场景。
