# Skill 当前实现与规范流程对比

## 背景

当前项目已经实现了 Skill 的基础能力：

```text
SKILL.md 声明能力
  -> 启动时加载到 SkillRegistry
  -> LLM 通过 select_skill 选择 Skill
  -> Runtime 校验并规范化参数
  -> SkillExecutor 根据 executor 分发
  -> Workflow 或 Tool Loop 执行业务步骤
```

这套实现已经能支撑“能力路由”，但距离更标准的 Skill 运行体系还有一些差距，尤其是在参数 schema、执行器类型、触发方式、Skill 激活后的上下文注入、权限边界和输出约束方面。

本文对比当前已有实现和更规范的设计方式，并给出后续演进建议。

## 总体对比

| 环节 | 当前已有实现 | 更规范的设计 |
| --- | --- | --- |
| Skill 声明 | 使用 `skills/*/SKILL.md` 声明 name、version、description、executor、intents、triggers、workflow、tools、parameters、read_only、output_policy、output_schema、guardrails | 继续补充 steps、permissions 和更严格的 JSON Schema |
| Skill 注册 | 启动时加载 `skills` 目录，注册到 `SkillRegistry`，校验 executor、workflow target 和 tool allowlist | 注册时继续增强参数 schema、权限策略和 steps 结构校验 |
| 模型感知 | system prompt 写入 name、description、executor、workflow、intents、triggers、read_only、output_policy；function schema enum 限定 skill name | 大量 Skill 时先检索 top-K；必要时注入编译后的 runtime spec |
| Skill 触发 | 主要通过 LLM `select_skill`；失败后 fallback 到 `ParseIntent` | 支持显式触发、规则触发、LLM 触发、事件触发、fallback 澄清或默认执行 |
| 触发后行为 | Skill 被转成 Intent，进入 SkillExecutor，根据 executor 选择执行分支 | 增加独立 SkillValidator 和更完整的 runtime spec |
| 执行方式 | 已支持 `workflow`、`tool_loop` 和日报场景的 `guided_steps` executor，`sub_agent` 有声明和错误边界 | 完整泛化 `guided_steps`，并实现 `sub_agent` executor |
| select_skill tool result | 当前没有 `role=tool` result，路由结果只在本地使用 | 固定 Workflow 可不回传；guided/sub_agent 可返回压缩 runtime spec 给模型 |
| 参数规范化 | Skill 已声明 parameters；Runtime 对时间做兜底提取；Workflow 用 `resolve_time_range` | 按 parameters 做统一 required/enum/pattern 校验 |
| Tool 权限 | Skill 声明 tools，并用 `ValidateTools` 检查工具存在 | 每次执行时强制按 Skill allowlist 过滤 tools |
| 输出约束 | Workflow 最后调用 LLM summary，失败走 fallback | 最终回答必须基于 observation 和本地 assessment，可做 JSON schema 和 grounding 校验 |
| 可观测性 | 返回 execution_steps、tool_calls、warnings | 增加 skill_route、runtime_spec、step_trace、grounding_trace、parameter_trace |

## 1. 注册声明 Skill

### 当前已有

当前 Skill 文件位于：

```text
skills/query_faulty_hosts/SKILL.md
skills/diagnose_host/SKILL.md
skills/tool_loop_investigate_host/SKILL.md
```

当前 `SKILL.md` front matter 主要包含：

```yaml
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
output_schema:
  summary: string
  assessment: object
guardrails:
  - do_not_generate_timestamps
  - fault_score_must_use_fault_analyzer
---
```

启动时加载逻辑：

```text
service.NewRuntime
  -> loadSkills
  -> skill.LoadDir
  -> skill.NewRegistry
```

注册时已经有基础校验：

- `name` 不能为空。
- `executor` 必须是 `workflow`、`tool_loop`、`guided_steps`、`sub_agent` 之一。
- `executor=workflow` 时 `workflow` 不能为空。
- Skill name 不能重复。
- intent 不能重复映射到多个 Skill。
- Skill 声明的 tools 必须存在于 ToolRegistry。
- Skill 声明的 workflow target 必须存在于 WorkflowRegistry。

### 当前不足

当前 Skill 声明已经补齐了基础标准字段，但还缺少这些能力：

- `steps`：还不能声明步骤化 tool calling。
- `permissions`：还没有用户角色和动作权限声明。
- `parameters`：已有声明，但还没有统一的 required/enum/pattern 运行时校验。
- `output_schema`：已有声明，但还没有最终响应 schema 校验。

当前 `tools` 更多是文档和启动校验用途，还没有在所有执行路径里强制作为运行时 allowlist 使用。

### 规范设计

更完整的 Skill 声明建议如下：

```yaml
---
name: investigate_host_step_agent
version: v1
description: 分步骤排查单台主机根因
intents:
  - investigate_host
triggers:
  - 排查 host
  - 根因分析
executor: guided_steps
workflow: ""
parameters:
  host_id:
    type: string
    required: true
    pattern: "^host-[0-9]{3}$"
  since:
    type: string
    required: false
  start_text:
    type: string
    required: false
  end_text:
    type: string
    required: false
tools:
  - get_host
  - query_metrics
  - query_alarms
  - query_changes
  - query_cmdb
  - resolve_time_range
read_only: true
output_policy: grounded
steps:
  - id: collect_host
    goal: 查询主机基础信息
    allowed_tools:
      - get_host
    required_outputs:
      - host
  - id: collect_signals
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
output_schema:
  summary: string
  assessment: object
  evidence_refs: array
guardrails:
  - do_not_generate_timestamps
  - do_not_fabricate_tool_results
  - fault_score_must_use_fault_analyzer
---
```

核心变化是：Skill 从“路由文档”升级为“可执行能力说明”。

## 2. 让模型感知 Skill

### 当前已有

当前模型通过两种信息感知 Skill。

第一种是 system prompt。

`RouterSystemPrompt` 会把 Skill 摘要写入 prompt：

```text
name
description
workflow
intents
```

第二种是 function schema。

`select_skill` 的 `skill` 字段使用 enum：

```json
{
  "skill": {
    "type": "string",
    "enum": [
      "diagnose_host",
      "query_faulty_hosts",
      "tool_loop_investigate_host"
    ]
  }
}
```

这比让模型自由输出 intent name 更稳定，因为模型只能在已注册 Skill 中选择。

### 当前不足

当前模型已经感知到 Skill 的路由摘要，包括：

- Skill name。
- Skill description。
- Skill executor。
- Skill workflow。
- Skill intents。
- Skill triggers。
- Skill 是否 read-only。
- Skill 的 output_policy。

当前模型还没有在路由阶段感知：

- Skill 的完整参数 schema。
- Skill 的完整 guardrails。
- Skill 的执行步骤。

另外，当前所有 Skill 摘要直接放进 prompt。Skill 数量少时没问题，数量多之后会出现：

- prompt 过长。
- 模型选择困难。
- 相似 Skill 混淆。
- enum 过大导致选择不稳定。

### 规范设计

更规范的方式是分两层感知。

第一层是路由感知：

```text
给模型 Skill 摘要
只用于选择 Skill
不用于执行业务
```

第二层是执行感知：

```text
Skill 被选中后
给模型注入选中 Skill 的 runtime spec
只包含执行当前 Skill 所需的最小信息
```

少量 Skill 可以直接把全部摘要放进 prompt。

大量 Skill 应改为：

```text
用户输入
  -> 本地召回 top-K Skill
  -> system prompt 只包含 top-K
  -> select_skill enum 只包含 top-K name
```

不要把完整 `SKILL.md` 原文直接塞给模型，推荐把它编译成结构化 runtime spec。

## 3. Skill 触发逻辑

### 当前已有

当前有两种真正的 Skill 触发路径。

#### 3.1 LLM select_skill 触发

这是主路径：

```text
用户输入
  -> Runtime.selectIntent
  -> LLM ChatWithTools
  -> select_skill
  -> DecodeSelection
  -> 校验 Skill 存在
  -> 返回 Intent
```

如果模型返回：

```json
{
  "skill": "diagnose_host",
  "parameters": {
    "host_id": "host-001"
  },
  "confidence": 0.9
}
```

Runtime 会转换为：

```go
client.Intent{
    Name: "diagnose_host",
    Parameters: map[string]string{
        "host_id": "host-001",
    },
}
```

#### 3.2 ParseIntent fallback 触发

如果 `select_skill` 不可用或失败，会 fallback 到老的 `ParseIntent`：

```text
没有加载 Skill
LLM 不支持 function calling
select_skill 调用失败
select_skill 参数非法
select_skill 返回未知 Skill
select_skill 没有返回 tool call
```

然后通过：

```text
skill name 直接匹配
或 skill intents 匹配
```

找到 Skill。

### 当前还有一种 Workflow fallback

如果最终 routeName 是空、unknown 或 workflow 不存在，当前会 fallback 到：

```text
tool_loop_investigate_host workflow
```

这不是严格意义上的 Skill 触发，因为没有重新选择 `tool_loop_investigate_host` Skill，而是直接走 Workflow fallback。

### 当前不足

当前没有实现：

- API 显式指定 Skill。
- 前端按钮或菜单触发 Skill。
- 本地规则优先触发 Skill。
- 事件触发 Skill。
- 用户角色过滤 Skill。
- 参数不足时返回澄清问题。
- 多 Skill 组合或链式触发。

### 规范设计

推荐支持以下触发方式。

#### 显式触发

由 API、前端按钮、菜单或自动化系统直接指定：

```json
{
  "skill": "diagnose_host",
  "parameters": {
    "host_id": "host-001"
  }
}
```

适合高确定性入口。

#### 规则触发

用本地规则优先处理高频确定场景：

```text
包含 "故障机" -> query_faulty_hosts
包含 "诊断" 且包含 host_id -> diagnose_host
包含 "排查" 或 "根因" 且包含 host_id -> tool_loop_investigate_host
```

规则触发适合确定性强、误判成本高的入口。

#### LLM 触发

自然语言入口交给 LLM：

```text
用户输入
  -> select_skill
  -> 本地校验
  -> 执行 Skill
```

这是通用能力最强的触发方式。

#### Fallback 触发

当无法识别时，不一定总是进入 tool loop。

更规范的 fallback 可以是：

```text
参数不足 -> 返回澄清问题
意图不明确 -> 返回可选 Skill 列表
包含 host_id 但意图不明确 -> 进入受限调查 Skill
风险较高 -> 拒绝执行或转人工
```

#### 事件触发

由系统事件触发 Skill：

```text
监控告警
变更完成
工单创建
定时巡检
CMDB 资源变更
```

这种触发不依赖用户 message，但仍应走 SkillValidator 和 SkillExecutor。

## 4. Skill 触发后的逻辑

### 当前已有

当前 `select_skill` 成功后，Runtime 做的是：

```text
Skill name
  -> skillForIntent
  -> spec.Workflow
  -> WorkflowRegistry.Get
  -> wf.Run
```

固定 Workflow 的执行方式：

```text
Workflow 固定编排 Tool
  -> Tool 调 Client
  -> Domain FaultAnalyzer 评分
  -> LLM 生成 summary
  -> fallback summary
```

`tool_loop_investigate_host` 的执行方式：

```text
Workflow 内部重新构造 messages
  -> LLM ChatWithTools
  -> 本地执行 Tool
  -> role=tool observation 回填
  -> 多轮循环
  -> assess_fault
  -> 最终诊断
```

当前 `select_skill` 没有对应 `role=tool` result。

也就是说：

```text
select_skill 的输出只作为本地路由结果
不会继续把 Skill 内容塞回同一个 messages 中
```

### 当前不足

当前已经有明确的 SkillExecutor 分发层：

```text
Runtime
  -> SkillRouter
  -> SkillExecutor
      -> executor=workflow
      -> executor=tool_loop
```

当前没有统一处理：

- 独立 SkillValidator。
- Skill 参数 required/enum/pattern 校验。
- Skill 权限校验。
- Skill tool allowlist 强制过滤。
- Skill runtime spec 构造。
- Skill 执行 trace。
- Skill 输出 schema 校验。

### 规范设计

触发后推荐流程：

```text
SkillSelection
  -> SkillValidator
      -> skill exists
      -> user role allowed
      -> read_only allowed
      -> parameters schema valid
      -> required parameters present
      -> tools available
  -> ParameterNormalizer
      -> time range
      -> enum mapping
      -> host_id/path/resource_id
  -> SkillExecutor
      -> executor=workflow
      -> executor=guided_steps
      -> executor=tool_loop
      -> executor=sub_agent
  -> OutputGrounder
  -> Unified Response
```

## 5. 触发后的处理方式

### 5.1 Workflow Executor

适合固定步骤。

```text
Skill
  -> WorkflowExecutor
  -> 固定 Workflow
  -> Tool
  -> Client
  -> Domain
  -> Summary
```

优点：

- 稳定。
- 可测试。
- 成本低。
- 不容易重复调用 Tool。

适合当前：

```text
query_faulty_hosts
diagnose_host
```

### 5.2 Guided Step Executor

适合步骤固定但每步需要模型理解参数或选择工具的场景。

```text
Skill steps
  -> step 1 只暴露 allowed_tools
  -> LLM tool_calls
  -> 本地执行
  -> 校验 required_outputs
  -> step 2
```

这是建议下一步重点实现的模式。

它比固定 Workflow 灵活，比开放 ReAct 稳定。

### 5.3 Tool Loop Executor

适合探索型排查。

```text
LLM
  -> tool_calls
  -> tool observation
  -> LLM
  -> tool_calls
  -> ...
```

必须配套：

- allowed tools。
- max steps。
- max tool calls。
- timeout。
- duplicate detection。
- canonical key。
- prerequisite check。
- compact observation。

当前 `tool_loop_investigate_host` 已经具备部分能力。

### 5.4 SubAgent Executor

适合复杂任务封装。

```text
父 Agent 选择 Skill
  -> 子 Agent 在 Skill 边界内执行
  -> 返回结构化结果
```

子 Agent 不能拿到全局工具池，只能拿到当前 Skill 的 tools allowlist。

## 6. select_skill tool result 是否需要

### 当前做法

当前不返回 `select_skill` tool result。

这是合理的，因为当前多数 Skill 被映射到固定 Workflow，Workflow 不依赖模型继续读取 Skill 内容。

### 规范建议

是否需要 `select_skill` tool result，取决于 executor。

```text
executor=workflow
  -> 不需要。Runtime 本地路由即可。

executor=guided_steps
  -> 可以需要。返回编译后的 runtime spec。

executor=tool_loop
  -> 可选。若 loop prompt 已经足够，可以不需要。

executor=sub_agent
  -> 建议需要。子 Agent 需要明确 Skill 边界。
```

如果返回，不建议塞完整 `SKILL.md`，而应返回结构化最小 spec：

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
    "query_alarms"
  ],
  "steps": [
    {
      "id": "collect_host",
      "allowed_tools": ["get_host"]
    }
  ],
  "rules": [
    "do_not_generate_timestamps",
    "do_not_fabricate_tool_results"
  ]
}
```

## 7. 当前到规范的演进建议

建议按下面顺序演进。

### 第一阶段：补齐 Skill 元数据

已完成基础字段：

```yaml
executor: workflow
parameters: {}
triggers: []
output_schema: {}
guardrails: []
```

后续还需要补：

```yaml
permissions: {}
steps: []
```

### 第二阶段：引入 SkillExecutor

已将当前 Runtime 抽成：

```text
Skill -> SkillExecutor -> WorkflowExecutor / ToolLoopExecutor
```

后续需要补 `GuidedStepExecutor` 和 `SubAgentExecutor`。

### 第三阶段：实现 Guided Step Executor

新增一个对比 Skill：

```text
investigate_host_step_agent
```

用于对比三种方式：

```text
diagnose_host                固定 Workflow
tool_loop_investigate_host   开放 Tool Loop
investigate_host_step_agent  步骤化 Tool Calling
```

### 第四阶段：强化参数规范化

重点处理：

- 时间范围。
- 枚举字段。
- host_id。
- path。
- 资源 ID。

对于时间参数，继续保持：

```text
模型只提取 since/start_text/end_text
本地 resolve_time_range 计算真实时间戳
```

### 第五阶段：强化输出 grounding

最终回答阶段引入：

- output_schema。
- source refs。
- tool observation 引用。
- 禁止杜撰校验。
- fallback summary。

## 结论

当前项目的 Skill 已经完成了“路由层”的核心闭环：

```text
声明 Skill
  -> 加载注册
  -> prompt + enum 让模型选择
  -> 本地校验
  -> 映射 Workflow
```

但更规范的 Skill 应该进一步覆盖：

```text
参数 schema
执行器类型
触发策略
权限边界
步骤定义
运行时 tool allowlist
输出 schema
grounding 校验
执行 trace
```

短期最合理的方向不是把所有 Skill 都改成 ReAct，而是保留当前稳定的固定 Workflow，同时新增 `Guided Step Executor` 做对比实验。

推荐目标架构：

```text
AgentRuntime
  -> SkillRouter
      -> explicit trigger
      -> rule trigger
      -> LLM select_skill
      -> fallback
  -> SkillValidator
  -> SkillExecutor
      -> WorkflowExecutor
      -> GuidedStepExecutor
      -> ToolLoopExecutor
      -> SubAgentExecutor
  -> ToolRegistry
  -> Client
  -> Domain Analyzer
  -> Grounded Response
```
