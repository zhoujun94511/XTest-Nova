# Monkey 高级策略 API

Nova 与 Companion 的 Monkey POST 接口接受同一结构化配置。基础字段为目标包、运行秒数、
事件间隔和可选随机种子；高级字段如下：

```json
{
  "package": "com.example.app",
  "durationSeconds": 600,
  "throttleMillis": 500,
  "lowBatteryExit": true,
  "minBatteryPercent": 15,
  "activityMode": "blocklist",
  "activities": ["com.example.app/.DangerActivity"],
  "targetActivities": ["com.example.app/.CheckoutActivity"],
  "controlBlacklist": ["立即支付", "com.example.app:id/delete"],
  "targetCases": [
    {
      "activity": "com.example.app/.CheckoutActivity",
      "task": "checkout",
      "case": "verify-order"
    }
  ]
}
```

`activityMode` 可为 `none`、`allowlist` 或 `blocklist`。Activity 守卫或控件黑名单命中时，
Runner 执行返回并跳过本轮随机事件。目标 Activity 首次命中会记录结构化日志。
相同 `requestId` 只有在规范化配置一致时才作为幂等重试返回原状态；同键异参返回 HTTP 409，且不会替换正在运行的目标页令牌或用例计划。

`targetCases` 使用任务名和用例名引用 `/sdcard/xtest-nova/<目标包>/Replay` 中的录制用例。
若存在多个同名版本，启动时选择最新的完整性有效用例。启动前必须同时满足：

- 用例 SHA-256 完整性验证通过；
- 用例目标包与 Monkey 目标包一致；
- Activity、任务名和用例名格式合法；
- 同一映射没有重复；
- 当前没有录制、回放或智能遍历占用输入会话。

Agent 为每次运行生成一次性 128 位随机令牌，只通过 Runner 进程参数传递。Runner 命中目标
页面后以同步本机回环请求触发用例；请求返回前随机循环处于暂停状态。Replay 完成后 Monkey
继续，失败或超过五分钟则以 `target_case_failed` 明确结束。令牌重放、非当前运行令牌及未
登记映射均被拒绝。外部停止会终止 Runner 和正在执行的目标页 Replay。
目标页授权、Replay 启动和 Runner 停止共享同一执行边界，因此 Stop 返回后不会再由已授权的旧请求启动 Replay；Runner UTF-8 文本注入也遵循同一边界。

`GET /v1/monkey/runs/current` 返回本次运行的 `identity.sessionId` 与临时
`identity.ownerToken`。外部调用 `DELETE /v1/monkey/runs/current` 时必须将两者分别放入
`X-XTest-Session-Id` 和 `X-XTest-Owner-Token` 请求头；旧页面的迟到 Stop 返回 HTTP 409，
不能停止新一代 Runner。最终 `run.json`、`evidence.json` 等产物不会保存 owner token。
`GET /v1/monkey/report` 返回独立 Result Judge 的三态结论。

Companion 文本格式为：

```text
Activity=任务/用例；Activity=任务/用例
```

Activity 本身允许包含 `/`，等号右侧最后一个 `/` 用于分隔任务与用例。

## 运行状态与证据包

每次运行使用设备本地时间建立独立目录：

```text
/sdcard/xtest-nova/<目标包>/Monkey/yyyyMMdd_HHmmss/
  events.jsonl
  run.json
  activity_coverage.json
  activity_coverage.txt
  exploration_graph.json
  logcat.txt
  crash.json
  anr.json
  native_crash.txt
  diagnostics.txt
  exit_info.json
  start.png
  finish.png 或 failure.png
```

同一秒启动的会话自动追加 `_2`、`_3`，不会覆盖已有结果。`run.json` 记录请求配置、最终状态及
产物 SHA-256，但不会保存目标页调用令牌。Activity 覆盖率只报告本次实际观测到的 Activity，
没有可证明分母时不伪造百分比。

`GET /monkey` 和 `GET /v1/monkey/runs/current` 返回 `package`、`running`、`finalizing`、`events`、`stopReason`、
`exitCode`、`artifactDir`、日志/清单/覆盖率/探索图路径、探索计数、截图路径和非致命产物错误。
`running=false, finalizing=true` 表示 Runner 子进程已经退出但派生产物仍在收尾；只有
`running=false, finalizing=false` 才表示最终清单、覆盖率、探索图和结束截图均已完成写入。
`completed` 只表示遍历预算正常结束，不代表已经到达某个业务页面。已识别的 Onboarding 在
所有安全前进点击和左滑预算耗尽后返回 `stopReason=onboarding_exhausted`，并保留当前页面供
诊断；该终态不会执行通用 Back、force-stop 或重新拉起应用。探索指标中的
`onboardingExhaustions` 记录该保护门禁的命中次数。
诊断产物使用本次会话起始时间和目标包过滤，`run.json` 同步记录 `crashCount`、`anrCount`、
`nativeCrashCount`、`failureType`、采集可用性及错误。检测到 Java/Native Crash 或 ANR 时，
`stopReason` 分别归一为 `app_crash` 或 `app_anr`，结束截图使用 `failure.png`。非 Root 设备无法读取的
Tombstone、`/data/anr` 或 DropBox 不作为成功前提，`diagnosticsAvailable=false` 与
`diagnosticErrors` 用于区分“未发现故障”和“系统诊断不可用”。普通日志在 Runner
会话期间持续采集，先按目标包与已知 PID 过滤，再使用有界头尾保留策略；常见凭据、用户标识和
设备标识会脱敏。`diagnosticSources` 分别报告 general log、crash buffer、events、lastanr、
processes 与 exit-info 的可用性和截断状态，`diagnosticsComplete` 表示关键事故源完整。
`exit_info.json` 保存当前会话内的 Android `ApplicationExitInfo`，用于补充低内存、信号、
系统杀进程及未形成典型 logcat 事件的异常退出。

Crash 与 ANR 报告按稳定异常签名聚合。每个 `incidents[]` 条目只保留首次出现的详情，
`fingerprint` 标识异常签名，`occurrences` 表示实际发生次数，`firstSeenAt`/`lastSeenAt`
记录首末时间；重复发生时 `repeatSummary` 提示同类异常累计次数，`sources` 记录参与判定的
诊断来源。同一次异常在 general log、crash/events buffer 和 exit-info 中的多份证据会先折叠，
不会增加发生次数。`crashCount`、`anrCount`、`nativeCrashCount` 是累计发生次数，而不是
`incidents[]` 的唯一签名条目数。Exit Info 存在无法解析的记录块时，`diagnosticsComplete=false`
并在 `diagnosticErrors`/来源状态中报告解析缺口。
`exploration_graph.json` 记录稳定场景数、转移边数、点击/长按/滚动/回退、已知路径回放和层级降级次数；
逐条转移仍保留在 `events.jsonl`，摘要不能替代原始证据。历史兼容日志仍写入
`/data/local/tmp/xtest-nova-monkey.log`，达到 32 MiB 时轮转为一个 `.1` 备份。

每个目标包的 Monkey 会话最多保留 200 个，创建新会话时删除最旧目录。截图 HTTP 接口仍是
即时流，不会因一次查看而无限落盘；需要可追溯证据的 Monkey 关键节点由上述会话自动保存。
