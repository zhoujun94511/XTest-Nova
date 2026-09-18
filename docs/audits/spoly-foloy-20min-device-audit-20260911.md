# Spoly / Foloy 真机长测与审计报告（2026-09-11）

## 1. 结论

在同一台 Samsung SM-G9860（Android 13 / API 33，序列号 `R5CN30EQKNM`）上，Spoly 与 Foloy 均完成 20 分钟目标应用测试。每款应用由 10 分钟 Go Explorer 和 10 分钟 Java Runner 组成，测试期间不清除应用数据，并启用购买、订阅、删除、卸载等高风险操作拦截。

本轮没有发现目标应用 Crash、ANR 或 Native Crash 的证据，也没有发生遍历死循环或无法停止。审计确认并修复了两项基础设施问题：长测脚本过早拉取尚在收尾的产物，以及长运行中普通 logcat 达到上限后可能漏判后段事故。

## 2. 测试对象与原始证据

| 应用 | 包名 | 版本 | 实测时长 | 原始报告 |
| --- | --- | --- | ---: | --- |
| Spoly | `sport.card.identifier.grade.app` | 1.1.2 (40) | 20.109 分钟 | `tests/reports/spoly-long-audit-20260911-134901` |
| Foloy | `tcg.scanner.value.app` | 1.3.0 (41) | 20.091 分钟 | `tests/reports/foloy-long-audit-20260911-140915` |

Java Runner 每次最终产物均为 12 个文件：事件、运行清单、活动覆盖、探索图、起止截图、普通日志、Crash、ANR、Native Crash 和诊断摘要。两组运行清单中的 11 个内容哈希全部复算一致。

## 3. 执行结果

### Spoly

| 引擎 | 运行数 | 步骤/事件 | 状态 | 边 | 输入 | 滚动进展/尝试 | 循环检测 | 阻断边 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Go Explorer | 5 | 142 | 95（各运行求和） | — | 0 | 25/31 | 23 | 27 |
| Java Runner | 1 | 76 | 46 | 75 | 0 | 4/7 | 31 | 48 |

- Go 阶段 4 次以 `state_exhausted` 正常结束，最后一次在阶段预算到达时停止；Java 阶段在预算到达时以 `stopped` 正常结束。
- 循环检测次数较高，但对应边被持续阻断，状态仍然增长，且运行能在预算内终止；这是防循环机制生效，不是死循环证据。
- 本次可达路径未暴露可安全输入的字段，因此输入数为 0，不能据此认定输入引擎失效。

### Foloy

| 引擎 | 运行数 | 步骤/事件 | 状态 | 边 | 输入 | 滚动进展/尝试 | 循环检测 | 阻断边 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Go Explorer | 7 | 116 | 84（各运行求和） | — | 36 | 2/2 | 4 | 12 |
| Java Runner | 1 | 72 | 26 | 66 | 18 | 1/6 | 1 | 16 |

- Java Runner 自然完成（`completed`），并执行 5 次已知路径重放。
- Java 的 5 次滚动停滞中，4 次前后可见内容指纹完全一致；详情页滚动则正确识别为进展。Go 的滚动样本来自不同入口，不能直接以 2/2 推断 Java 存在误判，因此本轮不修改滚动算法。
- Go 出现 1 次不稳定快照，但随后恢复并继续运行，没有升级为失败。

## 4. Crash / ANR 审计

两款应用的最终 `run.json`、`crash.json`、`anr.json` 和 `native_crash.txt` 均报告 0 个事故；设备 `exit-info` 与 crash buffer 也未发现本次会话对应的 Crash/ANR。普通诊断日志均标记为截断，因此结论应表述为“本次多源证据未发现事故”，而不是证明应用绝对不存在稳定性问题。

## 5. 已确认问题与优化

### P1：停止后过早拉取 Runner 产物（已修复）

两次 20 分钟运行都复现：DELETE 停止请求返回时 `running=false`，但 `finalizing=true`。旧脚本立即保存状态并拉取，Spoly 首次只得到 3 个文件，Foloy 首次只得到 6 个文件；收尾完成后设备端实际都有 12 个文件。因此 Stop 不会丢弃日志，但旧编排会制造“日志丢失”的假象，并把过期状态写入 `summary.json`。

修复内容：

- 等待 `running=false` 且 `finalizing=false` 后再保存状态和拉取；
- 设置 60 秒有界等待，超时显式失败，不再静默生成残缺报告；
- 按远端会话名隔离本地产物目录，避免重复 pull 形成不确定嵌套；
- 拉取后校验 11 个固定文件及 `finish.png`/`failure.png` 终态截图。

### P1：长运行后段 Crash/ANR 可能被普通日志上限遮蔽（已修复）

Spoly 和 Foloy 的 10 分钟 Java 运行都出现 `diagnosticsTruncated=true`。原实现对全量 logcat 保留前 4 MiB，若事故发生在更晚时段，结构化事故识别可能拿不到对应行。

修复内容：在普通日志之外，独立采集低噪声的 crash buffer，以及 events buffer 中的 `am_crash`/`am_anr`；分析阶段合并这些证据。普通上下文即使达到容量上限，后段关键事故仍有独立通道。异步收尾的诊断时限由 5 秒调整为 15 秒，以容纳新增数据源，同时不增加 Stop API 的响应等待。

### P2：Go 滚动计数语义可进一步澄清（后续优化项）

Spoly 的 `scrollAttempts=31`，而 `scrollProgress + scrollStalls=26`。差额来自停止、特殊场景恢复等情况下尚未形成下一稳定快照的滚动。当前数据不影响遍历决策，但报表容易被理解为计数丢失。建议后续新增 `scrollUnresolved`，并约束 `attempts = progress + stalls + unresolved`。

## 6. TODO 对齐

- [x] Spoly 真机运行 20 分钟并保存原始证据。
- [x] Foloy 真机运行 20 分钟并保存原始证据。
- [x] 交叉检查事件、探索图、截图、Crash、ANR、Native Crash、exit-info 和哈希。
- [x] 修复 Runner finalizing 阶段的产物拉取竞态。
- [x] 增加产物完整性门禁与确定性会话目录。
- [x] 增加独立 Crash/ANR 诊断通道与回归测试。
- [x] 通过 Agent 全量测试及 ARM64/ARMv7 构建。
- [x] 在同一真机部署新版 Agent，并完成 Foloy 短时诊断烟测：12 个产物、11 个哈希条目、无诊断错误。
- [ ] 后续版本增加 `scrollUnresolved` 指标并更新报告契约。

## 7. 验证记录

- PowerShell 长测脚本语法检查：通过。
- finalizing 轮询与完整产物校验的函数级回归：通过。
- `go test ./internal/runner`：通过。
- `go test ./...`：通过。
- Linux ARM64 / ARMv7 Agent 构建：通过。
- Foloy 新版真机诊断烟测：`completed`，12 个产物，Crash/ANR/Native Crash 均为 0，`diagnosticsAvailable=true`，无截断、无采集错误。
- `go test -race ./internal/runner`：未执行成功；当前 Windows Go 1.25 cgo 工具在构建 `runtime/cgo` 时退出，未进入项目测试断言。该环境限制不计为代码通过，需在可用的 race 工具链或 Linux CI 补跑。
