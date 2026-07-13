# Skill 设计方式分析

## 背景

当前项目已经引入 Skill 概念，用于描述 Agent 可以处理的能力。现有实现中，Skill 主要承担“能力声明”和“路由依据”的职责：

```text
用户输入
  -> LLM 通过 select_skill 选择 Skill
  -> Runtime 校验 Skill
  -> 映射到 Workflow
  -> Workflow 执行业务步骤
  -> 返回统一响应
```

当前 Skill 文件位于 `skills/*/SKILL.md`，例如：

```text
skills/query_faulty_hosts/SKILL.md
skills/diagnose_host/SKILL.md
skills/tool_loop_investigate_host/SKILL.md
```

代码中的核心组件包括：

```text
internal/skill/loader.go     加载 SKILL.md
internal/skill/registry.go   注册和查询 Skill
internal/skill/router.go     构造 select_skill function tool
internal/agent/runtime.go    调用 select_skill 并路由到 Workflow
```

## Skill 不只是 Markdown

`SKILL.md` 是 Skill 的声明载体，但 Skill 不应只被理解为一段 Markdown 文档。

更合理的定义是：

```text
Skill = 能力声明 + 参数规范 + 工具权限 + 执行策略 + 输出约束
```

其中 `SKILL.md` 适合承载：

- Skill 名称、版本、描述。
- 适用意图。
- 对应 executor 或 workflow。
- 允许使用的 tools。
- 是否只读。
- 输出策略。
- 使用规则和约束。

运行时不建议直接把完整 Markdown 原文交给模型执行，而应加载并编译成结构化的 runtime spec。

## 当前实现方式

当前项目采用的是“Skill Router + Workflow Executor”模式。

### 1. 启动时加载 Skill

服务启动时从 `skills` 目录加载 `SKILL.md`，注册到 `SkillRegistry`。

```text
service.NewRuntime
  -> loadSkills
  -> skill.LoadDir
  -> skill.Registry
```

加载后的 Skill 会参与工具校验：

```text
Skill 声明的 tools
  -> 与 ToolRegistry 中的工具对比
  -> 缺失则记录 warning
```

### 2. LLM 如何感知 Skill

当前不是把所有 Skill 全文塞给模型，而是通过两部分让模型感知：

```text
system prompt:
  告诉模型它是 Skill Router
  列出可用 Skill 摘要

function tool:
  select_skill
  skill 字段使用 enum 限定可选 Skill 名称
```

`select_skill` 的 schema 约束：

```json
{
  "skill": "必须是已注册 Skill enum 之一",
  "parameters": "规范化参数，值统一使用字符串",
  "confidence": "0 到 1 的置信度"
}
```

这种方式比让模型自由返回 intent name 更稳定，因为：

- Skill 名称来自 enum。
- 参数集中在一个结构化对象中。
- 本地代码会校验 Skill 是否存在。
- 未知 Skill 可以 fallback 到默认 loop workflow。

### 3. select_skill 当前没有 tool result

当前 `select_skill` 是一次路由型 tool call。

它不是完整的：

```text
assistant tool_call
  -> role=tool result
  -> assistant final
```

而是：

```text
assistant 返回 select_skill tool_call
  -> Runtime 解析 arguments
  -> 得到 SkillSelection
  -> 转成 Intent
  -> 映射 Workflow
```

也就是说，`select_skill` 的“结果”当前是本地内部对象：

```go
client.Intent{
    Name:       "diagnose_host",
    Parameters: map[string]string{"host_id": "host-001"},
}
```

它不会再以 `role=tool` 的形式回传给模型。

这适合当前固定 Workflow 模式，因为后续执行不依赖模型继续理解 Skill 内容。

## 三种推荐 Skill 执行方式

后续 Skill 可以按执行方式分成三类。

### 1. Fixed Workflow Skill

适用于步骤明确、业务规则稳定、需要强确定性的场景。

示例：

```text
query_faulty_hosts
diagnose_host
```

流程：

```text
select_skill
  -> 固定 Workflow
  -> Go 代码编排 Tool 调用
  -> FaultAnalyzer 本地评分
  -> LLM 只做最终摘要
```

优点：

- 稳定。
- 可测试。
- 容易观测。
- 不容易重复调用 Tool。
- 评分和关键判断不受模型影响。

适合：

- 查询故障机。
- 诊断单台机器。
- 标准化巡检。
- 固定报表查询。

不建议为了“更 Agent 化”而把这些场景改成开放式 ReAct。

### 2. Tool Loop / ReAct Skill

适用于路径不完全固定，需要模型根据中间观察结果决定下一步的场景。

当前项目中的示例：

```text
tool_loop_investigate_host
```

流程：

```text
select_skill
  -> tool_loop_investigate_host workflow
  -> LLM ChatWithTools
  -> 本地执行 Tool
  -> role=tool observation 回填
  -> LLM 继续选择下一步
  -> assess_fault
  -> 最终诊断
```

这种模式要做强约束：

- 每轮只暴露当前允许调用的 tools。
- 已完成的 tool 不再暴露。
- 使用 canonical key 做重复检测。
- tool result 压缩成 observation，不直接塞完整后端 response。
- 评分必须使用本地 FaultAnalyzer。
- 限制 max steps、max tool calls、timeout。

适合：

- 根因排查。
- 多系统交叉调查。
- 中间结果会影响下一步查询的场景。

风险：

- 模型可能重复调用 Tool。
- 模型可能参数漂移。
- 成本和延迟更高。
- 如果 observation 过长，会影响最终回答质量。

### 3. Guided Step Skill

这是更推荐的中间形态，介于固定 Workflow 和开放 ReAct 之间。

核心思想：

```text
Go 控制步骤边界
LLM 在每个步骤内选择或填充 tool call
```

示例 Skill 结构：

```yaml
---
name: investigate_host_step_agent
version: v1
description: 分步骤排查单台主机根因
executor: guided_steps
tools:
  - get_host
  - query_metrics
  - query_alarms
  - query_changes
  - query_cmdb
read_only: true
output_policy: grounded
steps:
  - id: collect_host
    goal: 查询主机基础信息
    allowed_tools:
      - get_host
    required_outputs:
      - host

  - id: collect_runtime_signal
    goal: 查询指标和告警
    allowed_tools:
      - query_metrics
      - query_alarms
    required_outputs:
      - metrics
      - alarms

  - id: collect_context
    goal: 查询变更和 CMDB
    allowed_tools:
      - query_changes
      - query_cmdb
    required_outputs:
      - changes
      - cmdb

  - id: assess
    local_action: fault_analyzer

  - id: summarize
    local_action: grounded_summary
---
```

执行逻辑：

```text
for each step:
  构造 step prompt
  只暴露 step.allowed_tools
  调用 LLM ChatWithTools
  校验 tool call 是否允许
  执行 tool
  生成 compact observation
  校验 required_outputs
```

优点：

- 比固定 Workflow 灵活。
- 比开放 ReAct 稳定。
- Tool 权限更容易收敛。
- 日志和调试更清晰。
- 更适合生产环境逐步落地。

## 是否应该把 Skill 内容作为 select_skill 的 tool result

这取决于后续执行方式。

### 固定 Workflow 不建议

如果后续仍然是：

```text
select_skill
  -> Go Runtime 路由
  -> 固定 Workflow 执行
```

则不建议把 Skill 内容作为 `select_skill` 的 tool result 塞回 messages。

原因：

- 后续执行不依赖模型继续理解 Skill。
- 增加 token 成本。
- 增加 prompt injection 风险。
- 可能让模型误以为它要自己执行 Workflow。

### Step Agent / SubAgent 可以使用

如果后续是：

```text
select_skill
  -> 返回选中 Skill 的 runtime spec
  -> LLM 根据 spec 分步骤调用 tools
```

则可以把 Skill 内容作为 `select_skill` 的 tool result。

但不建议塞完整 `SKILL.md` 原文，而应塞编译后的结构化最小 spec：

```json
{
  "skill": "investigate_host_step_agent",
  "executor": "guided_steps",
  "parameters": {
    "host_id": "host-001",
    "since": "2d"
  },
  "allowed_tools": [
    "get_host",
    "query_metrics",
    "query_alarms",
    "query_changes",
    "query_cmdb"
  ],
  "steps": [
    {
      "id": "collect_host",
      "allowed_tools": ["get_host"],
      "required_outputs": ["host"]
    },
    {
      "id": "collect_runtime_signal",
      "allowed_tools": ["query_metrics", "query_alarms"],
      "required_outputs": ["metrics", "alarms"]
    }
  ],
  "rules": [
    "do_not_generate_timestamps",
    "do_not_fabricate_tool_results",
    "fault_score_must_use_fault_analyzer"
  ]
}
```

这样模型拿到的是可执行约束，而不是自由文本说明。

## 子 Agent 是否适合执行 Skill

子 Agent 适合探索型 Skill，不适合所有 Skill。

推荐判断：

```text
确定性强、步骤固定
  -> Fixed Workflow

路径不固定、需要根据中间结果选择下一步
  -> SubAgent 或 Tool Loop

需要有限步骤、每步由模型填参或选择工具
  -> Guided Step Executor
```

在当前项目中：

```text
query_faulty_hosts
  -> 保持 Fixed Workflow

diagnose_host
  -> 保持 Fixed Workflow

tool_loop_investigate_host
  -> 可以抽象为 SubAgentExecutor
```

子 Agent 必须有硬边界：

- 只允许访问当前 Skill 声明的 tools。
- 只接收当前任务必要上下文。
- 不能访问全局工具池。
- 不能修改全局状态。
- 必须有 timeout、max steps、max tool calls。
- 必须返回结构化结果。
- 最终评分必须使用本地确定性代码。

推荐架构：

```text
AgentRuntime
  -> SkillRouter
  -> SkillExecutor
      -> WorkflowExecutor
      -> GuidedStepExecutor
      -> SubAgentExecutor
```

## 参数规范化设计

Skill 不应该只声明“能做什么”，还应该声明“参数如何产生”。

### 时间参数

时间类参数不要让模型直接生成时间戳。

推荐方式：

```text
用户自然语言
  -> select_skill 提取 since/start_text/end_text
  -> resolve_time_range tool 统一解析
  -> 下游 query tool 使用 TimeRange
```

模型只负责：

```text
最近 5 小时 -> since=5h
7月1号到7月5号 -> start_text=7月1号, end_text=7月5号
今天 -> since=today
```

本地负责：

- 时区。
- 起止时间计算。
- 秒级/毫秒级转换。
- 合法性校验。
- 默认时间范围。

### 枚举参数

对于 provider、region、environment 这类有限集合，应在 schema 中使用 enum。

例如：

```json
{
  "region": {
    "type": "string",
    "enum": ["east-china", "north-china", "south-china"]
  }
}
```

### 用户原文参数

对于 path、接口名、资源 ID 这类依赖用户原文的参数，不建议让模型自由改写。

推荐策略：

- 保留 raw_text。
- 尽量使用精确提取。
- 不让模型补全用户未提供的关键字段。
- 对格式做严格校验。
- 缺失时返回澄清问题，而不是猜测。

## Tool Result 设计

Skill 执行过程中，Tool Result 不应等同于后端 Response。

推荐链路：

```text
Backend Response
  -> Tool Raw Result
  -> 本地 Evidence State
  -> Compact Observation
  -> role=tool content
```

模型只看到压缩后的 observation：

```json
{
  "status": "complete",
  "function": "query_metrics",
  "canonical_key": "query_metrics:host-001:last_1h",
  "summary": "cpu=96 memory=76 high_cpu_minutes=12",
  "next_allowed_functions": ["query_alarms", "query_changes"]
}
```

完整结果保留在本地 state 或 evidence 中，用于：

- FaultAnalyzer 评分。
- API response 返回。
- 审计日志。
- 后续 deterministic 判断。

这样可以降低：

- 上下文占用。
- 模型复述无关字段。
- 最终回答杜撰。
- 多轮对话遗忘关键证据。

## 输出约束

Skill 应明确声明输出策略。

推荐策略：

```text
output_policy: grounded
```

含义：

- 最终回答只能基于 tool observation 和本地 assessment。
- 不允许引用未查询到的数据。
- 不允许编造变更、告警、根因。
- 不允许把推测写成事实。
- 评分、等级、是否故障必须来自 FaultAnalyzer。

更严格的做法是让模型输出结构化 JSON：

```json
{
  "summary": "string",
  "findings": [
    {
      "claim": "string",
      "source": "tool_observation_id"
    }
  ],
  "uncertainties": ["string"],
  "recommended_next_steps": ["string"]
}
```

然后本地校验：

- 每个 finding 是否有 source。
- source 是否来自本次 tool result。
- 是否引用了不存在的字段。
- 是否出现禁止动作，例如重启、删除、隔离。

## 可观测性

Skill 执行需要完整 trace。

建议每次请求记录：

- selected_skill。
- skill_selection_confidence。
- selected_workflow 或 executor。
- normalized_parameters。
- time_range。
- allowed_tools。
- 每次 tool call 的 canonical key。
- tool observation 摘要。
- skipped duplicate。
- blocked prerequisite。
- warnings。
- final output source。

API 响应中可以继续保留：

```text
execution_steps
tool_calls
warnings
duration_ms
```

日志中建议额外记录：

```text
skill_route
skill_runtime_spec
step_trace
grounding_trace
```

## 版本和兼容性

Skill 应该有版本。

```yaml
name: diagnose_host
version: v1
```

后续发生这些变化时应升级版本：

- 参数语义变化。
- tool allowlist 变化。
- 输出格式变化。
- 执行器变化。
- 步骤顺序变化。

不要在不改版本的情况下改变生产行为。

## 推荐落地路径

### 阶段 1：保持当前 Skill Router

当前实现已经具备：

- `SKILL.md` 声明。
- SkillRegistry。
- `select_skill` function calling。
- 固定 Workflow 路由。
- loop workflow 示例。

短期可以继续保持。

### 阶段 2：补充 executor 字段

在 Skill front matter 中增加：

```yaml
executor: workflow
```

可选值：

```text
workflow
guided_steps
sub_agent
```

这样 Runtime 不再只根据 workflow 字段执行，而是根据 executor 分发。

### 阶段 3：实现 GuidedStepExecutor

新增一个示例：

```text
skills/investigate_host_step_agent/SKILL.md
```

用于对比：

```text
diagnose_host                固定 Workflow
tool_loop_investigate_host   开放 Tool Loop
investigate_host_step_agent  步骤化 Tool Calling
```

### 阶段 4：引入 Skill Runtime Spec

将 `SKILL.md` 编译为结构化 spec：

```text
Markdown front matter + body
  -> skill.Spec
  -> skill.RuntimeSpec
  -> LLM 可消费的最小 JSON
```

只有 Step Agent 和 SubAgent 需要把 runtime spec 注入 messages。

### 阶段 5：强化 Grounding 校验

最终回答阶段增加：

- 引用来源校验。
- 禁止杜撰校验。
- 输出 JSON schema 校验。
- fallback summary。

## 当前项目的建议结论

对当前 Jarvis Agent 来说，推荐保持以下原则：

```text
1. Skill 是能力声明，不是自由 prompt。
2. 固定流程优先使用 Workflow。
3. 探索式排查才使用 Tool Loop 或 SubAgent。
4. 更推荐新增 Guided Step Skill，对比固定 Workflow 和开放 ReAct。
5. select_skill 用于路由，除非进入 Step Agent，否则不必返回 tool result。
6. 不直接把完整 SKILL.md 塞给模型，应使用编译后的 runtime spec。
7. Tool Result 必须压缩成 observation。
8. 时间、枚举、path 等参数要分类型做规范化和校验。
9. 故障评分和关键业务判断必须留在本地确定性代码中。
10. 每个 Skill 必须有工具权限边界、执行边界和输出边界。
```

这套设计可以同时满足：

- Workflow 优先。
- 可控使用 function calling。
- 支持 ReAct 对比实验。
- 降低 Tool 重复调用。
- 降低参数漂移。
- 降低模型杜撰最终答案的概率。
- 为后续接入真实 Jarvis、监控、CMDB、变更和工单服务保留扩展空间。
