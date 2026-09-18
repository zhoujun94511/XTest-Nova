# Spoly 多页 Onboarding 整改验证（2026-09-16）

## 结论

根因是 Runner 的动作历史粒度与 Compose 多页 onboarding 不匹配，不是 Android 15 策略限制，
也不是 Spoly 控件不可访问。不同语义页复用同一个 `Continue`、Activity 和坐标时，旧实现按
`cycleScene + action` 将后续页误判为已执行；动作耗尽后通用恢复又可能 Back/重拉到引导起点。

整改后，onboarding 前进动作使用 `Activity + semanticScene + action` 页面级身份；同页仍防重，
不同页可继续前进。已识别 onboarding 的动作耗尽和前进后的空过渡帧超时均原地停止，不进入
通用 DFS Back、force-stop 或重拉。

## 自动验证

- Runner self-test：覆盖五页同 Activity、同 `Continue`、同坐标、相同 `cycleScene`；验证每个
  不同语义页可执行、同页不可重复、普通 generation 继承、recovery generation 释放预算。
- Runner self-test：覆盖 onboarding 空动作过渡帧的 20 秒时间型等待，以及随后
  `transition_timeout -> onboarding_exhausted` 原地停止。
- Agent：`go test ./...` 全量通过；事件解析测试覆盖 `onboarding_exhaustion_guard`、
  `onboardingExhaustions` 和 `onboarding_exhausted` 终态。
- 发布门禁：77/77 路由实现、覆盖和文档契约均通过；制品尺寸门禁通过。

## 真机结果

设备均运行 Spoly 1.3.0（versionCode 53），配置为 120 秒、300 ms throttle、seed 2026091635、
bounded 渲染回退。测试从当天首装且仍停留 onboarding 的真实现场状态开始，没有额外清除数据。
到达业务页后主动停止，避免随机探索产生无关副作用。

| 系统 / 设备 | requestId | 结果 | 事件摘要 | 稳定性 |
|---|---|---|---|---|
| Android 15 / SM-S936U (`R3CY80B2G4W`) | `spoly-onboarding-page-scope-a15` | 到达 `CategoryDetailActivity` | 35 events，29 taps，5 scrolls，0 relaunch | Crash/ANR/Native/abnormal 均 0 |
| Android 16 / 2510DPC44G (`83fc400c`) | `spoly-onboarding-page-scope-a16` | 到达 `CardDetailActivity` | 29 events，20 taps，5 scrolls，0 relaunch | Crash/ANR/Native/abnormal 均 0 |

两台设备日志均记录 `profile/v1/save_onboarding` 返回 HTTP 200，证明 onboarding 业务状态已保存。
完整证据分别位于：

- `tests/reports/spoly-onboarding-remediation-20260916/android15/20260916_221057/`
- `tests/reports/spoly-onboarding-remediation-20260916/android16/20260916_221059/`

### 最终哈希严格首装复测

用户清除双机应用数据后，第一次复测发现按 12 个观察周期计算的等待窗口只有约 5 秒：Android 16
刚好通过，Android 15 在首个 `Continue` 后按 `transition_timeout` 安全停止。该轮没有 Back、重拉
或崩溃，证明保护生效，但也证明次数型窗口对冷启动波动不稳健。最终实现改为明确的 20 秒时间窗口，
重新签名构建、部署并再次清除目标应用数据复测。

| 系统 / 设备 | requestId | 最终结果 | Onboarding 证据 | 恢复与稳定性 |
|---|---|---|---|---|
| Android 15 / SM-S936U | `spoly-final-clean2-a15-20260916` | 进入 `MainActivity` | 7 次 `Get Started/Continue`；11 次过渡等待；保存接口 HTTP 200 | 0 guard，0 relaunch，Crash/ANR/Native/abnormal 均 0 |
| Android 16 / 2510DPC44G | `spoly-final-clean2-a16-20260916` | 进入 `MainActivity` | 7 次 `Get Started/Continue`；11 次过渡等待；保存接口 HTTP 200 | 0 guard，0 relaunch，Crash/ANR/Native/abnormal 均 0 |

两端各有一次 onboarding 全部前进动作完成后的普通 DFS Back；它没有发生在空过渡等待或安全前进
动作耗尽阶段，也没有返回 onboarding 起点。Android 16 由该 Back 直接进入 `MainActivity`；Android 15
随后经过普通条款页面探索并进入 `MainActivity`。这与原缺陷中的“后续 Continue 被抑制后回到首屏”不同。

最终证据目录：

- `tests/reports/spoly-onboarding-remediation-20260916/android15-final-clean-2/20260916_222803/`
- `tests/reports/spoly-onboarding-remediation-20260916/android16-final-clean-2/20260916_222804/`

## 最终发布物

| 制品 | 字节数 | SHA-256 |
|---|---:|---|
| Runner | 62,485 | `0A6BEA3D5829AFCE0B337EDD9EE92B2A746C729B82807EF5CD373876D1F0118D` |
| Agent ARM64 | 13,255,420 | `94C02C016E6C7B08202EF7E720AB46106467CA94E71365DCEB1127D66CFB539F` |
| Agent ARMv7 | 13,576,486 | `18D56C445044F608F49CA8BF5CA0E9DE7AB15A14333E1F52D6B5364BC5EFB671` |

最终 ARM64 Agent 已部署到两台设备，健康检查确认四个内嵌运行时组件完整可用。

## 审计闭环

核心页面级去重版本的真机跑测还暴露过一次点击后的空动作过渡帧被 DFS 回退。该问题已在最终
发布物中增加 20 秒有界等待和超时原地停止，并由 Runner self-test 固化。最终哈希发布物已在
两台清数据设备上从首个 `Continue` 完整进入 `MainActivity`；Todo 已全部闭环，没有遗留未通过项。
