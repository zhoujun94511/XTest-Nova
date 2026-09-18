# 覆盖率优先探索 Gallery 三机烟测

日期：2026-09-12（Asia/Shanghai）

本次使用最新 ARM64 Agent、内嵌 Runner，以及 `.private/signing` 中的项目证书重新签名的运行组件，在三个在线设备上先卸载旧 UIAutomator Host/Test，再从空安装状态执行 Gallery 短时智能探索。目标是验证本轮状态机调整没有造成执行断链，并检查 Nova 主路径与 Nova→system 互斥回退。本次不是等时长 A/B 性能门禁。

| 系统 | 设备 | 目标应用 | 步骤 | 状态 | 边 | 有效/无效/跨应用 | 结果 |
| --- | --- | --- | ---: | ---: | ---: | --- | --- |
| Android 16 | `83fc400c` | `com.miui.gallery` | 8 | 7 | 8 | 5 / 3 / 0 | `max_steps`，Nova 主路径，无错误 |
| Android 15 | `R3CY80B2G4W` | `com.sec.android.gallery3d` | 8 | 6 | 8 | 5 / 3 / 0 | `max_steps`，Nova 首帧重试后成功，无错误 |
| Android 13 | `R5CN30EQKNM` | `com.sec.android.gallery3d` | 8 | 4 | 8 | 2 / 6 / 0 | `max_steps`，Nova 主路径，无错误 |

设备端探索产物目录：

- Android 16：`/sdcard/xtest-nova/com.miui.gallery/Exploration/20260912_205140`
- Android 15：`/sdcard/xtest-nova/com.sec.android.gallery3d/Exploration/20260912_205140`
- Android 13：`/sdcard/xtest-nova/com.sec.android.gallery3d/Exploration/20260912_205104`

首次运行发现 Nova Provider 返回无节点控件树后，仍持有 `UiAutomation`，导致 system dump 被系统终止，三机均 0 步退出。修复为互斥串行回退后复测，三机均完成任务；`lastSource=system-dump`、`fallbacks=1`，证明回退不再是假成功。

进一步定位到自有 UIAutomator 在部分设备 `getWindows()` 返回空列表时没有调用活动窗口根节点兜底；Android 15 还存在首次查询仅预热活动窗口缓存的现象。源码分别增加活动根兜底和同一 Instrumentation 内的首帧有界重试。项目签名材料实际位于 `.private/signing`，使用该证书重新签名 Host/Test 后，三台设备均明确完成旧包卸载（两包均返回 `Success`、残留包列表为空）和新包安装，设备 APK SHA-256 与本地构建一致。

最终控件树资格结果：Android 16 为 48 nodes / 17,160 bytes，Android 15 为 138 nodes / 51,272 bytes，Android 13 为 108 nodes / 40,722 bytes；三台 `lastSource` 均为 `nova-provider`、`fallbacks=0`。Android 15 首帧一次无节点，第二次在同一 Nova 所有者内成功，没有启动 system dump。

结论：探索状态机、指标、Nova 首帧兼容和串行回退已经形成真机可运行闭环；项目签名的新 UIAutomator 包已在 Android 13/15/16 完成 Nova 主路径验证。完整旧策略/新策略等时长 A/B 性能门禁仍按计划单独执行。
