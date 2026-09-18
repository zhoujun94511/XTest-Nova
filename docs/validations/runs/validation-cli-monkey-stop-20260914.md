# Monkey 设备端停止命令验证（2026-09-14）

## 目标

补回独立于网页和悬浮窗的设备端 Monkey 停止入口。该入口只停止当前 Monkey，不关闭 Agent、不卸载运行组件、不删除测试产物，并继续完成产物收尾和悬浮窗恢复。

## 命令

```sh
adb shell /data/local/tmp/xtest-nova-agent monkey status
adb shell /data/local/tmp/xtest-nova-agent monkey stop
```

关闭整个 Agent 仍使用：

```sh
adb shell /data/local/tmp/xtest-nova-agent server -d --stop
```

## 真机结果

| 设备 | 目标应用 | 停止结果 | Agent | 悬浮窗 | 产物 |
| --- | --- | --- | --- | --- | --- |
| Android 16 / `83fc400c` | `com.miui.gallery` | `stopReason=stopped` | 保持健康 | 自动恢复 | 14 个文件，无产物错误 |
| Android 15 / `R3CY80B2G4W` | `com.sec.android.gallery3d` | `stopReason=stopped` | 保持健康 | 自动恢复 | 14 个文件，无产物错误 |

两台设备均在测试前卸载全部 Nova APK、删除 Nova 运行文件及历史产物，再由最新自包含 Agent 重新部署。停止命令先读取当前 `sessionId/ownerToken`，再调用所有权保护的正常 Stop 接口，避免迟到命令停止新会话。

验证中同时修正 Companion 期望版本从 `30715` 落后于实际 `30717` 的问题。最终两台设备均报告 `versionCode=30717 expectedVersionCode=30717`，不会因版本常量断链而重复安装 Companion。

本地证据位于 `tests/reports/cli-monkey-stop-validation-20260914`。
