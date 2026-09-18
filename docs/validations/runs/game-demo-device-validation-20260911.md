# Nova 追星小游戏真机验证（2026-09-11）

## 目标

> 历史验证记录：本页描述的是合并前的独立包。2026-09-11 起这些场景已迁入 `com.xtest.nova.fixture`，不再维护独立 `game-demo` 工程；历史包名和产物路径保留用于证据追溯。

新增独立 Android 测试应用 `com.xtest.nova.gamedemo`，通过游戏式动态状态验证 XTest-Nova 的输入、点击、弹窗、循环阻断、跨 Activity、滚动、安全过滤以及 Runner 诊断产物。

## 游戏场景

- 玩家昵称输入和 8 类边界输入语料；
- 简单/困难模式切换；
- 九宫格动态星星、命中、失误、生命耗尽、重新开始；
- 过关奖励和游戏结束弹窗；
- 排行榜 Activity 与 40 行滚动列表；
- 帮助、重置以及动态状态文本；
- “购买金币”安全诱饵，应用内部记录是否被实际点击。

## 首轮测试发现的问题

1. 同一 EditText 滚动后坐标变化，两套引擎将其识别为新的输入字段，重复执行输入语料。Java Runner 因输入和滚动公平调度，在 120 秒内执行 8 次输入、4 次滚动，却没有执行任何点击。
2. Java `controlBlacklist` 是页面级守卫；长测脚本又重复传入系统已经内置的购买、支付等危险词。当页面出现购买文案时，Runner 会连续返回，而不是只过滤对应节点。

## 修复

- Go 和 Java 输入字段身份优先使用 resource-id，其次使用 content-desc；仅在两者均缺失时才使用坐标。输入框跨滚动后保持同一逻辑身份。
- 增加跨 viewport 位移的 Go 回归测试和 Java Runner 自检。
- 通用长测脚本不再把内置危险词重复配置为页面级 `controlBlacklist`；两套引擎既有的节点级中英文危险词过滤继续生效。
- 小游戏将核心玩法放在固定区域，帮助、排行榜、训练提示和购买诱饵放入独立滚动区域，使动态玩法和滚动场景可以在同一轮覆盖。

## 最终真机结果

设备：Samsung SM-G9860，Android 13 / API 33，序列号 `R5CN30EQKNM`。小游戏版本 1.1 (2)。

| 指标 | Go Explorer（120 秒预算） | Java Runner（90 秒） |
| --- | ---: | ---: |
| 结束原因 | `stopped`（预算到达） | `completed` |
| 步骤/事件 | 39 | 16 |
| 状态 | 18 | 5 |
| 边 | 38 | 15 |
| 输入 | 8 | 8 |
| 输入字段 | 1 | 1（由输入语料证明） |
| 点击 | 27 | 8 |
| 滚动进展/尝试 | 3/3 | 0/0 |
| 循环检测 | 3 | 0 |
| 阻断边 | 4 | 0 |
| Activity 覆盖 | 2/2，100% | 1/2 |

应用内部审计：

- Go：`starts=4`、`cellClicks=8`、`misses=8`、`resets=2`；
- Java：`starts=3`、`cellClicks=2`、`misses=2`；
- 两轮均不存在 `unsafeClicks`，证明“购买金币”诱饵没有被执行。

Java Runner 最终生成 12 个产物，11 个清单哈希复算全部一致，`diagnosticErrors` 为空，Crash、ANR、Native Crash 均为 0。

## 证据

- Go 最终结果：`tests/reports/game-demo-device-test-20260911/go-final/`
- Java 最终结果：`tests/reports/game-demo-device-test-20260911/java-final/`
- Java 完整产物：`tests/reports/game-demo-device-test-20260911/java-final/device-artifacts/20260911_151210/`

## 验证状态

- [x] 小游戏独立 APK 构建、签名和 Android 13 真机安装。
- [x] 页面结构和启动截图检查。
- [x] Go Explorer 真机执行及应用内部动作核验。
- [x] Java Runner 真机执行、诊断产物和哈希核验。
- [x] 输入字段跨滚动身份回归。
- [x] Runner 自检。
- [x] Agent 全量 Go 测试。
- [x] ARM64 / ARMv7 Agent 构建及 ARM64 真机部署。
