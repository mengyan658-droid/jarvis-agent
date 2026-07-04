# jarvis-agent

`jarvis-agent` 是一个面向基础设施运维场景的 Go Agent 示例项目。当前版本仍使用 Mock Jarvis/Monitor/Change/CMDB Client，但 LLM 已支持 Mock、智谱 GLM 和 OpenAI-compatible API。

## 架构

- `AgentRuntime`：只负责意图解析、工作流路由和调度；未知 intent 会兜底进入 tool loop。
- `Workflow`：负责编排业务步骤，当前实现固定 Workflow 和原生 function calling tool loop。
- `Tool`：统一封装外部服务调用，并记录 tool call。
- `Client`：定义外部服务接口，并提供 Mock 实现。
- `Domain`：保存领域对象和确定性故障评分逻辑，不依赖 HTTP、LLM SDK 或具体 Client。

核心调用链：

```text
HTTP API
  -> AgentRuntime
    -> LLM ParseIntent
    -> WorkflowRegistry
      -> 固定 Workflow
        -> ToolRegistry -> Tool -> Client
        -> FaultAnalyzer
      -> Tool Loop Workflow
        -> LLM tools/tool_calls loop
        -> ToolPolicy/Normalizer
        -> ToolRegistry -> Tool -> Client
        -> FaultAnalyzer
```

## 已实现场景

- 查询故障机：固定 Workflow，根据区域、环境和最近一小时窗口查询主机，拉取指标、告警、变更、CMDB 信息，执行故障评分并默认只返回故障机器。
- 诊断单台机器：固定 Workflow，提取 `host-001` 形式的 Host ID，查询相关证据后生成单机诊断。
- 排查单台机器根因：原生 function calling tool loop，由模型提出 `tool_calls`，本地规范化、去重、执行工具，并用 `FaultAnalyzer` 做确定性评分。

## Mock 数据

- `host-001`：华东 production，CPU 96%，有 critical 告警，最近有部署变更，判定为故障。
- `host-002`：华东 production，健康机器。
- `host-003`：华北 staging，不可达且健康检查失败，判定为 critical。
- `host-004`：华东 production，CPU 87%，有多个 warning 告警。
- `host-005`：华南 production，只有近期普通变更，不判定为故障。

## 配置

通过环境变量读取：

- `APP_PORT`：默认 `8080`
- `AGENT_TIMEOUT`：`LLM_PROVIDER=mock` 时默认 `5s`，真实 LLM provider 默认 `30s`
- `AGENT_MAX_STEPS`：默认 `10`
- `AGENT_MAX_TOOL_CALLS`：默认 `20`
- `LLM_PROVIDER`：默认 `mock`，可设置为 `glm` 或 `openai-compatible`
- `LLM_API_BASE_URL`：真实模型 API Base URL
- `LLM_API_KEY`：真实模型 API Key
- `LLM_MODEL`：模型名
- LLM 调用会始终记录请求体和响应体，日志不包含 Authorization header。

使用智谱 GLM API：

```bash
export LLM_PROVIDER=glm
export LLM_API_KEY=your-glm-api-key
export LLM_MODEL=glm-5.1
export AGENT_TIMEOUT=30s
go run ./cmd/server
```

使用脚本启动时，也可以在项目根目录创建 `.env`：

```bash
cp .env.example .env
```

然后编辑 `.env`：

```bash
LLM_PROVIDER=glm
LLM_API_KEY=your-glm-api-key
LLM_MODEL=glm-5.1
AGENT_TIMEOUT=30s
```

再执行：

```bash
make restart
```

启动脚本会打印当前使用的 provider/model，服务日志也会记录最终配置。

`LLM_PROVIDER=glm` 时，如果不显式设置 `LLM_API_BASE_URL`，默认使用：

```text
https://open.bigmodel.cn/api/paas/v4
```

使用其他 OpenAI-compatible API：

```bash
export LLM_PROVIDER=openai-compatible
export LLM_API_BASE_URL=https://api.openai.com/v1
export LLM_API_KEY=your-api-key
export LLM_MODEL=gpt-4o-mini
export AGENT_TIMEOUT=30s
go run ./cmd/server
```

如果你提供的是兼容 OpenAI Chat Completions 的私有网关，只需要替换：

```bash
export LLM_API_BASE_URL=https://your-llm-gateway.example.com/v1
export LLM_API_KEY=your-api-key
export LLM_MODEL=your-model-name
```

## 运行

```bash
go mod tidy
go fmt ./...
go test ./...
go run ./cmd/server
```

或使用 Makefile：

```bash
make tidy
make fmt
make test
make run
```

后台启动：

```bash
./scripts/start.sh
```

重启：

```bash
./scripts/restart.sh
```

停止：

```bash
./scripts/stop.sh
```

脚本默认写入：

- PID：`.runtime/jarvis-agent.pid`
- 日志：`logger/jarvis-agent-YYYY-MM-DD.log`
- 二进制：`.runtime/jarvis-agent`

可覆盖的环境变量：

- `APP_PORT`
- `AGENT_TIMEOUT`
- `AGENT_MAX_STEPS`
- `AGENT_MAX_TOOL_CALLS`
- `RUNTIME_DIR`
- `PID_FILE`
- `LOG_DIR`
- `LOG_DATE`
- `LOG_FILE`
- `BIN_FILE`
- `STOP_TIMEOUT_SECONDS`

如果 PID 文件丢失或过期，但端口仍被旧进程占用，脚本会直接失败并打印占用端口的 PID，避免新版本启动失败后请求仍落到旧服务上。

查看当天日志：

```bash
tail -f logger/jarvis-agent-$(date +%F).log
```

生成便于阅读的格式化日志：

```bash
./scripts/pretty-log.sh
```

默认输出：

```text
logger/jarvis-agent-YYYY-MM-DD.pretty.log
```

## API

所有 API 响应顶层统一使用 `code`、`msg`、`data`：

```json
{
  "code": "OK",
  "msg": "ok",
  "data": {
    "request_id": "req-xxx"
  }
}
```

错误响应：

```json
{
  "code": "BAD_REQUEST",
  "msg": "message is required",
  "data": {
    "request_id": "req-xxx"
  }
}
```

健康检查：

```bash
curl -s http://localhost:8080/healthz
```

查询华东生产环境最近一小时的故障机：

```bash
curl -s -X POST http://localhost:8080/api/v1/agent/query \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: u-001' \
  -H 'X-User-Role: sre' \
  -H 'X-Session-ID: s-001' \
  -d '{"message":"查询华东生产环境最近一小时的故障机"}'
```

诊断单台机器：

```bash
curl -s -X POST http://localhost:8080/api/v1/agent/query \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: u-001' \
  -H 'X-User-Role: sre' \
  -H 'X-Session-ID: s-001' \
  -d '{"message":"诊断 host-001"}'
```

原生 function calling tool loop 排查单台机器：

```bash
curl -s -X POST http://localhost:8080/api/v1/agent/query \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: u-001' \
  -H 'X-User-Role: sre' \
  -H 'X-Session-ID: s-001' \
  -d '{"message":"排查 host-001 的根因"}'
```

这条请求会路由到 `tool_loop_investigate_host`。

Loop 流程：

```text
1. Runtime 解析 intent，路由到 tool_loop_investigate_host。
2. Workflow 将当前阶段允许的 tools 传给 LLM。
3. LLM 返回 tool_calls。
4. 本地规范化参数并生成 canonical key。
5. 本地策略判断是否允许执行、是否重复、是否缺少前置证据。
6. 允许执行时通过 ToolRegistry 调用 Tool。
7. Tool observation 以 role=tool 回传给 LLM。
8. assess_fault 完成后不再开放 tools，生成最终诊断摘要。
```

Function calling 交互方式：

- 请求模型时传入 `tools`
- 模型返回 `tool_calls`
- 服务端执行对应 Tool
- Tool 结果以 `role=tool` 消息回传模型
- 模型生成最终结论

响应的 `results.function_call_trace` 会展示每次函数调用和观测结果；原来的 `诊断 host-001` 仍然走固定 Workflow。

如果 LLM 返回了 `unknown` 或未注册的 intent name，Runtime 会兜底路由到 `tool_loop_investigate_host`。兜底时会优先复用 LLM 解析出的 `host_id`，如果没有则从用户原始消息里提取 `host-001` 这类 Host ID。

Tool loop 带本地稳定性控制，不是模型想调什么就直接执行什么：

- 每轮只向模型暴露当前阶段允许调用的 tools。
- 工具参数会被本地规范化，只保留 `host_id`。
- 重复 tool call 会按 canonical key 去重，例如 `query_metrics:host-001:last_1h`。
- 模型新增无关参数不会绕过去重。
- 重复调用不会再次打外部 Tool，而是返回 `skipped_duplicate` observation。
- `assess_fault` 只会在证据工具完成后开放，完成后不再继续开放工具。

当前 canonical key：

```text
get_host:host-001
query_metrics:host-001:last_1h
query_alarms:host-001
query_changes:host-001:last_1h
query_cmdb:host-001
assess_fault:host-001:evidence_v1
```

稳定性优化详细记录见：

```text
improve/tool-loop-stability.md
```
