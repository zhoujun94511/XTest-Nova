# ShortsWave 自适应随机探索真机验证（2026-09-14）

## 验证目标

验证默认探索不再依赖固定业务控件优先级，并确认支付、积分和会员入口可参与探索但不会挤占整个任务；同时覆盖全屏广告恢复、系统结算页保护、息屏唤醒、异常采集和悬浮窗恢复。

## 环境与前置清理

- Android 16：`83fc400c`，SDK 36。
- Android 15：`R3CY80B2G4W`，SDK 35。
- 目标应用：`com.shorts.wave.drama`。
- 每台设备均先停止旧 Agent、卸载 Companion/UIAutomator Host/UIAutomator Test、删除 `/data/local/tmp/xtest-nova-*`、`/sdcard/xtest-nova` 和 `/sdcard/XTestNova`，再部署最新签名 Agent，由 Agent 重新释放并安装运行组件。
- 每台设备运行 1,800 秒，`throttleMillis=500`，使用不同且可复现的 seed。

## 最终结果

| 指标 | Android 16 | Android 15 |
| --- | ---: | ---: |
| 实际持续时间 | 1,803.6 秒 | 1,802.8 秒 |
| 结束原因 / 退出码 | `completed` / 0 | `completed` / 0 |
| 事件 | 506 | 521 |
| 状态 | 363 | 418 |
| 边 | 490 | 516 |
| 点击 | 348 | 354 |
| 滚动 | 130 | 140 |
| 有进展滚动 | 104 | 122 |
| 输入 | 1 | 2 |
| 层级回退 | 0 | 0 |
| Crash / ANR / Native Crash | 0 / 0 / 0 | 0 / 0 / 0 |
| 目标应用 Activity | 15 | 18 |

两台设备合计覆盖 20 个不同目标应用 Activity。除首页、详情、搜索、充值、VIP 和积分外，还进入了排行榜及详情、规则、登录、下载、Web、设置、关于、删除历史、删除账号，以及 Google、AppLovin、Facebook 三类广告 Activity。

## 与旧策略的等时长结果对比

| 设备 | 旧事件 → 新事件 | 旧状态 → 新状态 | 旧 Activity → 新 Activity |
| --- | ---: | ---: | ---: |
| Android 16 | 336 → 506（+50.6%） | 158 → 363（+129.7%） | 12 → 15（+25.0%） |
| Android 15 | 465 → 521（+12.0%） | 301 → 418（+38.9%） | 15 → 18（+20.0%） |

Android 16 新增覆盖删除历史、登录、排行榜详情；Android 15 新增覆盖删除账号、关于、设置、排行榜及详情、Facebook 广告。不同 seed 未保证重复同一页面集合，这是随机覆盖探索的预期表现；两台新结果的并集用于衡量多样性。

## 动态调度与特殊场景证据

- `enter_pointswall_ic` 在两台设备上均实际点击 3 次；旧结果为 Android 16 7 次、Android 15 16 次。限制来自同一动作的反馈衰减和有限重复保护，不是把积分模块设置为固定低优先级。
- 支付、充值、会员、积分节点和普通节点位于同一候选池，使用 seed 化加权随机抽样。权重由尝试次数、新状态、新 Activity 和无进展结果更新。
- Android 16 记录 37 条、Android 15 记录 13 条购买确认保护事件；允许进入应用内购买入口，但进入 Google Play 最终结算页后执行 Back。
- Android 16 记录 135 次、Android 15 记录 178 次广告关闭动作。广告无法立即关闭时等待可关闭阶段，超时后返回或重拉目标应用，不终止任务。
- Android 16 20 次、Android 15 23 次目标应用重拉，均属于广告/外部应用/探索耗尽恢复；任务最终只因时长到达而结束。
- Runner 首次启动、探索周期重启和目标应用重拉前均执行唤醒及非安全锁屏解除；本轮未再出现 Doze 导致的 0 动作空转。
- 测试结束后两台设备的 Companion 均为 `installed=true running=true`，悬浮窗抑制状态已恢复为 false。

## 产物与已知限制

- Android 16：`tests/reports/shortswave-adaptive-30min-validation-20260914/android16/20260914_180653`。
- Android 15：`tests/reports/shortswave-adaptive-30min-validation-20260914/android15/20260914_180654`。
- 两套目录均包含 14 个文件：运行清单、事件时间线、Activity 覆盖、图指标、证据索引、起止截图、logcat、Crash、ANR、Native Crash、进程退出信息和诊断摘要。
- 全量 logcat 达到上限，`diagnosticsTruncated=true`；独立 Crash/ANR/Native Crash 数据源均为 available/complete，未发现异常事件。
- 两张结束截图为黑色视频/渲染画面（Android 15 仍可见系统栏），不能单独证明最后业务页面；最终 Activity、动作和覆盖判断以同时间线、Activity 记录和层级数据为准。
- `exploration_graph.json` 当前只保存汇总指标，不保存完整节点/边明细；完整转移证据仍在 `events.jsonl`。这不影响本轮结论，但后续若要在 Web 中可视化路径，应扩展图产物结构。

## 结论

本轮达到“动态随机、覆盖收益反馈、支付入口可探测、特殊页面不终止任务”的目标。相较旧策略，两台设备在等时长下的事件、状态和 Activity 覆盖均提高，并且没有再被积分页长期占用。应用类型识别没有作为硬规则接入；现阶段以运行反馈覆盖弱先验，避免把通用探索器重新固化成按业务类型编写的脚本。
