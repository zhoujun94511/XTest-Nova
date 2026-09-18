# Gallery 可信执行全链路三设备验证（2026-09-12）

本轮以 Android 16 小米相册、Android 15/13 三星相册为真实目标，验证 Runner 所有权、正常
探索、诊断证据、三态判定、用例指纹、截图检查点回放以及 Web 启动性能入口。

最终三台设备运行同一 ARM64 Agent，SHA-256 为
`B60B3E7A40BCC75274ABB958F48BD7A6D7762E3D29B6CCF4E0A26513B96AF0F6`。

## 场景结果

| 场景 | Android 16 | Android 15 | Android 13 |
| --- | --- | --- | --- |
| 错误所有者停止运行中 Runner | HTTP 409，继续运行 | HTTP 409，继续运行 | HTTP 409，继续运行 |
| 正确所有者停止 | `stopped` / exit 0 | `stopped` / exit 0 | `stopped` / exit 0 |
| Gallery 正常 Runner | completed / 2 events | completed / 1 event | completed / 2 events |
| Crash / ANR / Native Crash | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| 诊断完整性 | complete | complete | complete |
| 无业务检查点的 Runner 判定 | `not_tested` | `not_tested` | `not_tested` |
| Gallery 截图检查点回放 | passed / 1 receipt | passed / 1 receipt | passed / 1 receipt |
| 伪造 caseFingerprint | HTTP 409 | HTTP 409 | HTTP 409 |

正常 Runner 没有业务验收断言，因此即使 `completed` 也只能得到 `not_tested`；截图检查点回放
同时具备完成状态、动作回执和截图证据，才得到 `passed`。

## 真机发现与修复

正确主动停止在层级采集阻塞超过五秒时会强制回收 Runner 进程，旧逻辑把这一内部回收细节
误记为 `unexpected_exit / signal: killed`。现已增加当前 generation 的停止意图，只有当前所有者
能够设置；无真实目标应用 Crash/ANR 时，强制回收统一归类为 `stopped / exitCode=0`。三台复测
均生成 `finish.png`，不再生成误导性的 `failure.png`。

## 用例指纹

- Android 16：`sha256:4df07586253c8adacfd5200f25702fba84db19daf6017ae309eb1979b2d2e5dc`
- Android 15：`sha256:826d045201aeae9c35b5aefa1d1e4ea93c976d653e69c19542ef1b3463273594`
- Android 13：`sha256:3148dd405979343f00860689e336f730e98690e86b7b9bd8294d96e6e711727c`

三份用例均包含一个真实 Gallery 截图检查点，validate 返回相同指纹，回放完成并生成一份
executed 动作回执。错误预期指纹全部被拒绝。

## Web 入口

冷热启动需要保留 UI 入口，但无需增加新的一级导航。当前入口位于“性能采集”页面下的
“冷启动与暖启动基线”独立卡片，与持续性能采集共用目标应用选择器。三台设备实际返回的
Web 资源均包含：目标包选择、冷/暖类型、轮数、间隔、P95 基线、允许退化幅度和开始测量；
按钮已绑定 `POST /v1/performance/startup`。

原始文件已拉回 `tests/reports/gallery-trusted-execution-validation-20260912/`，按设备分为 stopped、
normal、recording 和 replay 四类。持久化文件均未包含 ownerToken。
