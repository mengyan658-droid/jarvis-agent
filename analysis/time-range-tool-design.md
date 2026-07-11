# 时间工具设计分析

## 背景

在运维查询场景中，时间参数属于高风险参数。

如果让大模型直接生成时间戳，容易出现以下问题：

- 秒级和毫秒级混用。
- 时区理解错误。
- `start_time` / `end_time` 格式不一致。
- 相对时间计算错误，例如“最近 5 小时”被算成默认 1 小时。
- 明确起止时间被误解析成 `since`。
- 查询 Tool 实际执行时看不到模型到底传了什么时间。

因此当前设计的核心目标是：

```text
大模型只识别时间语义
Go 代码负责确定性计算起止时间
业务 Tool 只消费标准 TimeRange
Client 层再适配下游服务需要的秒、毫秒或 RFC3339
```

## 核心原则

### 1. 不让模型计算时间戳

不推荐：

```json
{
  "start_time_ms": 1782873600000,
  "end_time_ms": 1783305600000
}
```

推荐：

```json
{
  "kind": "absolute_range",
  "start_text": "7月1号",
  "end_text": "7月5号"
}
```

或者：

```json
{
  "kind": "relative",
  "amount": 5,
  "unit": "hour"
}
```

最终时间戳由本地 `resolve_time_range` Tool 生成。

### 2. 一个请求使用一个确定时间窗口

Workflow 开始时先解析时间窗口，然后后续查询复用同一个 `domain.TimeRange`。

这样可以避免：

```text
query_metrics 用一个 now
query_changes 用另一个 now
query_alarms 又用另一个 now
```

当前示例改造的是 `query_changes`。

### 3. 统一使用左闭右开区间

所有查询时间窗口统一为：

```text
[start, end)
```

即：

```text
包含 start
不包含 end
```

这样连续分页或连续时间窗口查询时不会重复命中边界数据。

## 当前实现链路

完整链路如下：

```text
用户输入
  -> LLM ParseIntent
  -> Runtime 本地兜底解析时间
  -> Workflow 生成 ResolveTimeRangeInput
  -> resolve_time_range Tool 计算 TimeRange
  -> query_changes Tool 使用 TimeRange
  -> ChangeClient 按 [start,end) 过滤
```

对应代码：

```text
internal/agent/runtime.go
  负责从原始用户消息兜底提取时间表达

internal/workflow/time_range.go
  负责把 intent parameters 转成 ResolveTimeRangeInput

internal/tool/resolve_time_range.go
  负责确定性计算 start/end

internal/domain/time_range.go
  定义标准 TimeRange 输出结构

internal/tool/tools.go
  QueryChangesInput 改为接收 domain.TimeRange

internal/client/change/mock.go
  ChangeClient 按 [start,end) 执行过滤

internal/tool/types.go
  记录 query_changes 的时间测试日志
```

## 分层职责

### Runtime 层

Runtime 负责从原始用户消息中做确定性兜底。

原因是 LLM 可能漏识别时间，比如用户说：

```text
查询华东生产环境最近5小时的故障机
```

LLM 如果没有输出：

```json
{
  "since": "5h"
}
```

Workflow 就会走默认最近 1 小时。

因此 Runtime 会在 LLM 解析之后补一层本地解析：

```text
明确起止时间优先：
  7月1号到7月5号 -> start_text=7月1号, end_text=7月5号

相对时间其次：
  最近5小时 -> since=5h
  最近30分钟 -> since=30m
  最近2天 -> since=2d
  近一周 -> since=1w

自然日：
  今天 -> since=today
  昨天 -> since=yesterday
```

优先级是：

```text
absolute_range > relative/since > default
```

也就是说，如果用户输入里有“7月1号到7月5号”，即使 LLM 误给了 `since=1h`，Runtime 也会用 `start_text/end_text` 覆盖掉 `since`。

### Workflow 层

Workflow 不直接计算时间。

它只负责把参数转换为 `ResolveTimeRangeInput`：

```text
start_text + end_text
  -> kind=absolute_range

since=5h
  -> kind=relative, amount=5, unit=hour

since=1w
  -> kind=relative, amount=1, unit=week

since=today
  -> kind=today

since=yesterday
  -> kind=yesterday

无时间参数
  -> kind=default
```

然后调用：

```text
resolve_time_range
```

### Tool 层

`resolve_time_range` 负责确定性计算。

支持的 `kind`：

```text
default
today
yesterday
relative
since
absolute_range
```

其中：

```text
default
  用户没指定时间，默认最近 1 小时

today
  今天 00:00 到 now

yesterday
  昨天 00:00 到今天 00:00

relative
  最近 N minute/hour/day/week 到 now

since
  从 start_text 到 now

absolute_range
  从 start_text 到 end_text
```

### Domain 层

标准输出是 `domain.TimeRange`：

```go
type TimeRange struct {
    RangeID      string
    Start        time.Time
    End          time.Time
    Now          time.Time
    Timezone     string
    Source       string
    Interval     string
    DurationMS   int64
    DurationSec  int64
    StartUnixSec int64
    EndUnixSec   int64
    StartUnixMS  int64
    EndUnixMS    int64
    IsDefault    bool
}
```

关键字段：

```text
source
  default / today / yesterday / relative / since / absolute_range

interval
  固定为 [start,end)

duration_sec
  方便验证“最近 5 小时”是否等于 18000 秒

start_time_sec / end_time_sec
  给需要 Unix 秒的系统使用

start_time_ms / end_time_ms
  给需要 Unix 毫秒的系统使用
```

## 时间问法覆盖方式

### 1. 相对时间

用户问法：

```text
最近5小时
近5小时
过去5小时
最近30分钟
近两小时
最近2天
近两天
过去7天
近一周
最近一周
```

会被归一化为：

```text
5h  -> relative, amount=5, unit=hour
30m -> relative, amount=30, unit=minute
2d  -> relative, amount=2, unit=day
1w  -> relative, amount=1, unit=week
```

计算方式：

```text
start = now - duration
end = now
```

示例，假设：

```text
now = 2026-07-11 10:30:00 +08:00
```

则：

```text
最近5小时:
start = 2026-07-11 05:30:00 +08:00
end   = 2026-07-11 10:30:00 +08:00
duration_sec = 18000

最近2天:
start = 2026-07-09 10:30:00 +08:00
end   = 2026-07-11 10:30:00 +08:00
duration_sec = 172800

近一周:
start = 2026-07-04 10:30:00 +08:00
end   = 2026-07-11 10:30:00 +08:00
duration_sec = 604800
```

### 2. 自然日

用户问法：

```text
今天
昨天
```

计算方式：

```text
今天:
start = 今天 00:00:00
end = now

昨天:
start = 昨天 00:00:00
end = 今天 00:00:00
```

注意：

```text
昨天 != 最近24小时
```

这是两个不同语义。

### 3. 明确日期范围

用户问法：

```text
7月1号到7月5号
7月1日到7月5日
2026年7月1日到2026年7月5日
7/1到7/5
2026-07-01 到 2026-07-05
2026/07/01 到 2026/07/05
```

如果没有写年份，默认使用请求发生时所在年份。

例如当前年份是 2026，则：

```text
7月1号到7月5号
```

会计算成：

```text
start = 2026-07-01 00:00:00 +08:00
end   = 2026-07-06 00:00:00 +08:00
```

原因是查询区间为 `[start,end)`，为了完整包含 7月5号 当天，日期型结束时间会自动推进到下一天 00:00。

### 4. 明确日期时间范围

用户问法：

```text
7月1号10点到7月5号18点
7月1号上午10点到7月5号下午6点
2026-07-01 10:00 到 2026-07-05 18:00
2026/07/01 10:00 到 2026/07/05 18:00
```

如果结束时间包含具体小时或分钟，则不会自动推进一天。

例如：

```text
7月1号10点到7月5号18点
```

会计算成：

```text
start = 2026-07-01 10:00:00 +08:00
end   = 2026-07-05 18:00:00 +08:00
```

## Query Tool 如何使用时间

当前示例改造的是 `query_changes`。

改造前：

```go
type QueryChangesInput struct {
    HostID string
    Since  time.Time
}
```

问题：

```text
只能表达开始时间
没有 end
各 workflow 可能自己 time.Now().Add(-1h)
无法验证下游真实查询窗口
```

改造后：

```go
type QueryChangesInput struct {
    HostID    string
    TimeRange domain.TimeRange
}
```

`query_changes` Tool 做强校验：

```text
host_id 必填
time_range.start/end 必填
start 必须早于 end
```

`ChangeClient` 按 `[start,end)` 过滤：

```text
record.CreatedAt >= start
record.CreatedAt < end
```

这样下游 Client 可以根据自己的协议转换：

```text
CMP logs -> start_time_sec / end_time_sec
Watchdog -> start_time_ms / end_time_ms
工单系统 -> RFC3339
Go client -> time.Time
```

## 日志验证

为了方便手工验证时间是否正确，当前临时增加了旁路日志：

```text
logger/time-test.log
```

它只记录 `query_changes` 的时间入参，不记录所有 Tool。

启动脚本默认设置：

```text
TIME_TEST_LOG=logger/time-test.log
```

查看方式：

```bash
grep "$REQ_ID" logger/time-test.log | jq .
```

实时观察：

```bash
tail -f logger/time-test.log | jq .
```

日志中重点看：

```text
input.host_id
input.time_range.source
input.time_range.start_time
input.time_range.end_time
input.time_range.duration_sec
input.time_range.start_time_sec
input.time_range.end_time_sec
input.time_range.start_time_ms
input.time_range.end_time_ms
```

## 测试问法样例

### 相对时间

```text
查询华东生产环境最近5小时的故障机
查询华东生产环境近5小时的故障机
查询华东生产环境过去5小时的故障机
查询华东生产环境最近30分钟的故障机
查询华东生产环境近两小时的故障机
查询华东生产环境最近2天的故障机
查询华东生产环境近两天的故障机
查询华东生产环境过去7天的故障机
查询华东生产环境近一周的故障机
查询华东生产环境最近一周的故障机
```

预期：

```text
最近5小时 -> source=relative, duration_sec=18000
最近30分钟 -> source=relative, duration_sec=1800
最近2天 -> source=relative, duration_sec=172800
近一周 -> source=relative, duration_sec=604800
```

### 自然日

```text
查询华东生产环境今天的故障机
查询华东生产环境昨天的故障机
```

预期：

```text
今天 -> source=today, start=今天 00:00, end=now
昨天 -> source=yesterday, start=昨天 00:00, end=今天 00:00
```

### 明确日期范围

```text
查询华东生产环境7月1号到7月5号的故障机
查询华东生产环境7月1日到7月5日的故障机
查询华东生产环境2026年7月1日到2026年7月5日的故障机
查询华东生产环境7/1到7/5的故障机
查询华东生产环境2026-07-01 到 2026-07-05的故障机
查询华东生产环境2026/07/01 到 2026/07/05的故障机
```

预期：

```text
source=absolute_range
start_time=2026-07-01T00:00:00+08:00
end_time=2026-07-06T00:00:00+08:00
```

如果写了具体年份，以用户给出的年份为准。

如果没有写年份，以请求发生时所在年份为准。

### 明确日期时间范围

```text
查询华东生产环境7月1号10点到7月5号18点的故障机
查询华东生产环境7月1号上午10点到7月5号下午6点的故障机
查询华东生产环境2026-07-01 10:00 到 2026-07-05 18:00的故障机
查询华东生产环境2026/07/01 10:00 到 2026/07/05 18:00的故障机
```

预期：

```text
source=absolute_range
start_time=2026-07-01T10:00:00+08:00
end_time=2026-07-05T18:00:00+08:00
```

### 简写时间

```text
查询华东生产环境5h的故障机
查询华东生产环境30m的故障机
查询华东生产环境2d的故障机
查询华东生产环境1w的故障机
```

预期：

```text
5h -> source=relative, duration_sec=18000
30m -> source=relative, duration_sec=1800
2d -> source=relative, duration_sec=172800
1w -> source=relative, duration_sec=604800
```

## Curl 验证模板

```bash
make restart

REQ_ID=req-time-test-001

curl -s -X POST http://localhost:8080/api/v1/agent/query \
  -H 'Content-Type: application/json' \
  -H 'X-Request-ID: '"$REQ_ID" \
  -H 'X-User-ID: u-001' \
  -H 'X-User-Role: sre' \
  -H 'X-Session-ID: s-001' \
  -d '{"message":"查询华东生产环境最近5小时的故障机"}'

grep "$REQ_ID" logger/time-test.log | jq .
```

## 单元测试覆盖

当前测试覆盖了以下核心点：

```text
internal/tool/resolve_time_range_test.go
  default 最近 1 小时
  today
  yesterday
  relative hour/day/week
  since 今天10点
  absolute_range 标准日期时间
  absolute_range 中文日期范围
  日期型结束时间包含结束日
  带具体结束小时不自动推进
  schema 只暴露规范枚举

internal/workflow/time_range_test.go
  since=2d -> relative day
  since=1w -> relative week
  最近2天 -> relative day
  近一周 -> relative week
  start_text/end_text 优先于 since

internal/agent/time_range_test.go
  Runtime 从原始用户输入兜底提取最近5小时
  Runtime 提取最近30分钟、最近2天、近一周
  Runtime 提取 7月1号到7月5号、7/1至7/5、2026-07-01 到 2026-07-05

internal/tool/tools_test.go
  query_changes 使用 TimeRange 查询
  query_changes 按 [start,end) 过滤
  query_changes 写入 logger/time-test.log 格式的旁路时间日志
```

运行：

```bash
go test ./...
```

## 当前边界和后续建议

当前已经支持大部分常见查询问法，但还有一些边界可以后续增强。

暂未重点支持：

```text
本周
上周
本月
上个月
昨天晚上
今天上午
故障发生前30分钟到故障后10分钟
最近三个工作日
跨年但用户不写年份的日期范围
```

建议后续扩展：

```text
1. 增加 natural_week / natural_month 语义。
2. 增加 day_window，例如 今天上午、昨天晚上。
3. 对无年份日期在跨年场景下增加歧义处理。
4. 把 TimeRange 接入 query_metrics、query_alarms、日志查询、工单查询。
5. 将 logger/time-test.log 变成可配置的调试开关，避免长期保留临时测试日志。
```

## 结论

当前时间工具的稳定性来自三层约束：

```text
LLM 只做语义识别
Runtime 做本地兜底提取
resolve_time_range 做确定性计算和校验
```

业务查询 Tool 不再接收模型生成的时间戳，而是接收标准 `domain.TimeRange`。

这样可以同时解决：

```text
时间戳格式错误
秒/毫秒混用
模型漏识别时间
明确起止日期被误判成默认时间
多个 Tool 使用不同 now
查询边界重复或遗漏
```

