# 三机真机场景 Monkey 验证（2026-09-15）

## 结论

- 新版 Agent 与 Companion 已完成正式签名构建、发布门禁和三机部署；Companion 版本为 `30718`。
- Android 16、Android 15 使用 ShortsWave，Android 13 使用 Samsung Gallery，各自执行不少于 20 分钟。
- 三台设备均未发现 Java Crash、Native Crash 或 ANR，诊断收尾完整且无诊断错误。
- Monkey 活跃期间，Companion 悬浮窗窗口出现次数为 0；结束后窗口与前台服务均恢复，版本仍为 `30718`。
- 本轮发现两个独立于悬浮窗优化的探索恢复缺口：激励广告二次关闭确认未自动处理，以及 Gallery 被 Back 带到桌面/重复工具栏路径后未及时重启目标。

## 测试结果

| 系统 / 设备 | 目标应用 | 墙钟时长 | 轮次 | 事件 | 场景 | 边 | Crash / ANR / Native | 悬浮窗抑制违规 | 结果 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- | ---: | --- |
| Android 16 / `83fc400c` | ShortsWave `2.65.00` | 1212.0 秒 | 1 | 372 | 256 | 366 | 0 / 0 / 0 | 0 | 通过 |
| Android 15 / `R3CY80B2G4W` | ShortsWave `2.65.00` | 1210.9 秒 | 1 | 269 | 186 | 266 | 0 / 0 / 0 | 0 | 通过，记录广告恢复缺口 |
| Android 13 / `R5CN30EQKNM` | Gallery `14.5.00.33` | 1212.5 秒 | 2 | 423 | 210 | 418 | 0 / 0 / 0 | 0 | 通过，记录目标逃逸/重复路径缺口 |

## 现场问题

### Android 15 激励广告二次确认

Runner 已进入 `com.google.android.gms.ads.AdActivity` 并触发退出，但停在 “Close Ad? / You will lose your reward / CLOSE / RESUME” 二次确认框。保存现场后选择安全的 `CLOSE`，恢复到 ShortsWave 主页面并继续原 20 分钟计时。中途再次复现中文版“关闭广告？/ 关闭 / 继续”，处理后继续推进。

建议后续将广告退出建模为多阶段状态：退出动作后再次取层级，优先匹配 `CLOSE`、`关闭`、`放弃奖励` 等明确退出语义；对 `RESUME`、`继续` 保持非首选；并加入有限次数、超时与截图证据。

### Android 13 Gallery 目标逃逸与重复路径

第一轮 166 个事件后，连续 Back 将 Gallery 带到桌面，但 Runner 仍保留最后一个 Gallery Activity。该轮按带身份令牌的正常停止流程结束并完成证据收尾，随后以剩余时间自动启动第二轮。第二轮末段在 `action_similar_on_toolbar` 上出现重复长按路径。

建议后续以实时前台包为准增加目标逃逸看门狗，并对同一场景/同一控件动作连续无进展设置阈值，触发安全回退、切换 Tab 或重启目标应用，而不是持续空转。

## 安全与收尾

- 使用控制黑名单避开购买、支付、订阅、充值、删除、卸载、分享、发送、设为等操作。
- 所有 Monkey 产物均已从设备拉回，`run.json` 中列出的 SHA-256 已逐项校验。
- 最终前台包保持为对应目标应用。
- 最终电量：Android 16 为 99%，Android 15/13 为 100%。
- 三台 Companion 均为 `installed=true running=true versionCode=30718 expectedVersionCode=30718`。

## 证据位置

- Android 16：`tests/reports/scenario-monkey-20m-20260915/android16-shortswave/`
- Android 15：`tests/reports/scenario-monkey-20m-20260915/android15-shortswave/`
- Android 13：`tests/reports/scenario-monkey-20m-20260915/android13-gallery/`

每个目录包含基线、前/中/后截图、最终摘要、Runner 状态、设备原始证据包、崩溃日志与应用退出信息。Android 15 额外包含广告卡住与恢复截图；Android 13 额外包含目标逃逸和重复路径截图。
