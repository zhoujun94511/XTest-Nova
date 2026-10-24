# Monkey 高级策略 API

命令行 `monkey start`、Web 控制台的遍历和悬浮窗都向 Monkey POST 提交同一次遍历。
Web 控制台填写运行时长和动作间隔。间隔默认 500 毫秒，范围 50–60000；低电量 15% 停止默认勾选，取消后不因电量停止。页面同时固定发送有界渲染和不限制活动范围。
Agent 存在探索引擎时，这次请求由探索会话执行，不再另起一套随机过程。基础字段为目标包、
运行秒数、事件间隔和可选随机种子。三个入口未改参数时都发送：间隔 500 毫秒、
`renderFallbackMode=bounded`、`lowBatteryExit=true`、`minBatteryPercent=15`、
`activityMode=none`。接口本身省略这些字段时，低电量退出为关闭，空的渲染策略按有界处理。

高级字段如下：

```json
{
  "package": "com.example.app",
  "durationSeconds": 600,
  "throttleMillis": 500,
  "renderFallbackMode": "bounded",
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

`activityMode` 可为 `none`、`allowlist` 或 `blocklist`。当前前台不在允许范围内，或落在排除范围内时，
会话按一次返回离开该页。控件黑名单按文本、描述和资源 ID 的片段匹配，只从候选控件里去掉命中项，
不因此整页返回。广告、权限、付费墙和评分这些特殊页先处理，再应用黑名单。
`renderFallbackMode` 可为 `off`、`bounded` 或 `continuous`。仅当页面没有可执行控件，
且层级里有 `SurfaceView` 或 `UnityPlayer` 时才使用坐标点击。`bounded` 每个 Activity 最多 5 次，
`continuous` 不设这个次数。

设置 `durationSeconds` 后，会话只在时长到达时以 `duration_elapsed` 结束，步数不是停止条件。
低电量每 30 秒读取一次 `dumpsys battery`，电量不高于 `minBatteryPercent` 时以 `low_battery` 结束；
正在充电也不豁免。读数失败不停止。

命令行、Web 和悬浮窗没有入口优先级。设备上已有遍历、录制或回放处于运行、停止或收尾时，
新的启动返回 HTTP 409，正在执行的会话和它的目标页计划都保持不变。相同 `requestId`
只有在规范化配置一致时才作为幂等重试返回原状态；同键异参同样返回 HTTP 409。
三个入口通常各自生成新的请求，因此第二次点击是冲突，不是续跑，也不会排队。
任一入口都可以读取当前会话身份并停止它。停止会等到正在执行的目标页回放不再占用输入，
Agent 与悬浮窗这条停止请求最长等待 120 秒。

目标 Activity 首次命中会进入已准备的签名用例。用例失败或单次回放超过五分钟时，
该次不记为已完成，10 秒后可以再试；取消停止时不再改选另一个动作。

`targetCases` 使用任务名和用例名引用 `/sdcard/xtest-nova/<目标包>/Replay` 中的录制用例。
若存在多个同名版本，启动时选择最新的完整性有效用例。启动前必须同时满足：

- 用例 SHA-256 完整性验证通过；
- 用例目标包与 Monkey 目标包一致；
- Activity、任务名和用例名格式合法；
- 同一映射没有重复；
- 当前没有录制、回放或智能遍历占用输入会话。

Agent 为每次运行生成一次性 128 位随机令牌。探索会话命中目标页时，以当前会话为宿主执行
对应的已签名用例；回放占用输入期间，遍历不再选择另一个动作。用例成功后会话继续。
令牌重放、非当前运行令牌及未登记映射均被拒绝。外部停止会结束遍历，并等到正在执行的
目标页回放退出。父会话释放时，托管中的回放所有权一并释放。

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
