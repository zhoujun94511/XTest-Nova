# Foloy 10 分钟真实场景验证

## 范围

- 时间：2026-09-10 20:04:02～20:14:09（Asia/Shanghai），实际 10.12 分钟。
- 设备：`R5CN30EQKNM`。
- 样本应用：Foloy（`tcg.scanner.value.app`）。包名只由测试编排显式传入，不参与核心策略判断。
- 引擎分配：Go Explorer 5 分钟，Java Runner 5 分钟。
- Agent：`xtest-nexus-0.22.0-m5.9-compat`。

## 结果

| 引擎 | 会话/动作 | 状态 | 输入 | 滚动 | 循环与封禁 |
| --- | ---: | ---: | ---: | ---: | ---: |
| Go | 4 个会话 / 60 步 | 48 | 22 | 4 次，2 次产生进展 | 7 次检测，8 条边封禁 |
| Java | 1 个会话 / 35 动作 | 12 | 18 | 2 次，均为 stall | 0 次检测，0 条边封禁 |

Java 在同一编辑框实际执行了全部 18 个输入等价类，长度包含 0、1、31、32、33 和 64。输入完成后计数保持为 18，没有因编辑框内容变化重新生成语料。Java 的 33 条转移中未出现连续三轮的 1～8 阶重复周期，因此本轮 `cycleDetections=0` 与原始序列一致。

Go 的 4 个会话中，2 个以 `state_exhausted` 自然结束，证明图耗尽后没有进入无限随机输入；循环检测和边封禁均真实触发。最后一个 Go 会话处于阶段预算边界，被编排器取消采集后记录为 `safety_stop`。

测试结束时 Foloy 仍为前台应用，Agent 健康。Crash buffer 为空；应用退出记录只有测试编排主动执行的 `force-stop`，没有观察到应用崩溃或 ANR。

## 审计项

1. Go 第 3 个会话以 `input_failed` 停止，错误为 `unexpected scrcpy clipboard response`。此前已有 4 次输入成功；问题来自 scrcpy 控制通道把非预期设备事件直接当作剪贴板确认失败，属于输入通道健壮性问题，不是 Foloy 规则问题。
2. 阶段结束时的主动取消可能被层级采集包装成 `safety_stop`，错误中实际为 `context canceled`。需要让显式停止优先归类为 `stopped`，避免报告误报安全故障。
3. Java 两次滚动均为真实 viewport stall；Go 同轮滚动进展率为 2/4。样本量较小，后续应在更多通用列表、WebView 和 Compose 样本上继续比较。

## 证据

完整原始状态、步骤、图、事件、截图、Crash buffer 和退出记录位于 `tests/reports/foloy-real-10min-20260910-200402`。
