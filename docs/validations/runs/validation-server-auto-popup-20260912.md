# Server 单命令启动悬浮窗验证（2026-09-12）

## 结论

`xtest-nova-0.26.0-m6.3-auto-popup` 已完成签名构建、发布门禁及 Android 16/13 空环境验证。正常使用只需：

```text
adb shell /data/local/tmp/xtest-nova-agent server -d
```

该命令会校验、释放并安装 Runner、Companion、UiAutomator host/test，随后自动显示 Companion 悬浮窗。无需再执行 `popup start`。

## 审计中发现并修复的问题

首次 Android 16 验证发现：包级 `SYSTEM_ALERT_WINDOW=allow` 被设备残留的 UID 级 `ignore` 覆盖，服务可瞬时启动但约 3 秒后退出；旧部署检查仍可能把该瞬时状态记为成功。

修复后统一通过独立 `overlaypermission` 组件设置包级和 UID 级 AppOps。部署门禁除检查 `popupOverlayStartup` 外，还要求 `popup status` 连续 4 秒保持 `installed=true running=true`，避免瞬时成功竞态。

## 空环境与部署结果

验证前，两台设备均停止旧 Agent，卸载以下三个 APK，并删除 `/data/local/tmp/xtest-nova-*` 运行文件及 `/sdcard/xtest-nova` 历史产物；随后逐项确认不存在：

- `com.openatx.xtest.popup`
- `com.openatx.xtest.nova.uiautomator`
- `com.openatx.xtest.nova.uiautomator.test`

| 设备 | 系统 | 默认启动 | 10 秒后悬浮服务/窗口 | 运行组件 |
|---|---:|---|---|---|
| `83fc400c` | Android 16 / SDK 36 | 一条 `server -d` 成功 | `running=true`，`APPLICATION_OVERLAY=true` | Runner 释放，三个 APK 原子安装 |
| `R5CN30EQKNM` | Android 13 / SDK 33 | 一条 `server -d` 成功 | `running=true`，`APPLICATION_OVERLAY=true` | Runner 释放，三个 APK 原子安装 |

两台设备的 `runtimeBootstrap`、四个 payload 和 `popupOverlayStartup` 均为 ready。Android 16 的有效 UID AppOps 为 allow，解决了包级显示 ignore 时的覆盖问题。

## Gallery 全链路结果

| 设备 | 目标应用 | 会话/前台 | 控件树 | 截图 | Runner | 产物 |
|---|---|---|---:|---:|---|---|
| Android 16 | `com.miui.gallery` | 成功 | 17,206 bytes | 1,439,746 bytes | `completed`, exit 0 | 13 个文件 |
| Android 13 | `com.sec.android.gallery3d` | 成功 | 12,053 bytes | 165,570 bytes | `completed`, exit 0 | 13 个文件 |

真机产物目录：

- `/sdcard/xtest-nova/com.miui.gallery/Monkey/20260912_143339`
- `/sdcard/xtest-nova/com.sec.android.gallery3d/Monkey/20260912_143352`

两次运行均生成 `events.jsonl`、`run.json`、覆盖率、探索图、首尾截图、全量 logcat、`crash.json`、`anr.json`、`native_crash.txt`、诊断和退出信息。诊断来源全部 available/complete；Crash、ANR、Native Crash、异常退出均为 0。

## 无界面边界

两台设备均复验 `server -d --no-popup`：运行组件保持安装，`popup status` 为 `installed=true running=false`，诊断为 `popupOverlayStartup=idle` 且明确记录 `disabled by --no-popup`。停止后不带该参数重启，悬浮窗均自动恢复。

## 构建与门禁

- 全量 Go 单元测试及竞态检测通过；新增默认启动、显式跳过、包级/UID 级授权回归测试。
- 全仓 PowerShell 语法检查通过；无界面验证脚本均显式使用 `--no-popup`，用户流程门禁改为验证自动启动。
- 签名构建通过，目标样例应用排除在正式运行时外。
- 77/77 路由、77/77 语义合同及完整发布门禁通过。
- ARM64 SHA-256：`F0ED25044DB70D63EE690F253786652B8B6805E27219A969A29FAAD6BB68604C`
- ARMv7 SHA-256：`9EDED5FA333F0F3D0646319EDFF7C6D9FF8011EAC6C1C8EC53ED3EF842585E95`
