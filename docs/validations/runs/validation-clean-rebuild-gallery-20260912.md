# 清机、重构建与 Gallery 真机验证（2026-09-12）

## 清理与重建

- Android 16（`83fc400c`）和 Android 13（`R5CN30EQKNM`）上的旧 `XTest`、`XTestNova`、`xtest-nova`、`xtest-nexus` 输出目录、根目录历史截图/XML、Nova/Nexus 临时载荷、旧 Agent/Runner 回滚副本及 scrcpy 4.0 均已清理。
- 清理前已停止 Agent；Companion、UiAutomator host/test 均卸载。Foloy、Gallery、系统应用和个人媒体未删除。
- 当前源码重新完成正式签名构建，重新生成 release manifest，并通过全量离线 release gate。
- 两台设备的首次 bootstrap 均报告 Runner 为 `extracted`，三个 APK 均为 `extracted,installed`，证明本次没有复用旧安装。

## Gallery 正常流程

| 设备 | 目标应用 | 实际动作 | 页面结果 | 异常 |
|---|---|---:|---|---|
| Android 16 | `com.miui.gallery` | 4 | 进入首页菜单、回收站页面并返回首页，随后切换 Collections | Crash 0、ANR 0、Native Crash 0 |
| Android 13 | `com.sec.android.gallery3d` | 4 | 进入创建相册流程并覆盖输入框边界，但未确认创建 | Crash 0、ANR 0、Native Crash 0 |

两次执行均生成 13 个标准产物：`events.jsonl`、`run.json`、`activity_coverage.json/.txt`、`exploration_graph.json`、`start.png`、`finish.png`、`logcat.txt`、`diagnostics.txt`、`crash.json`、`anr.json`、`native_crash.txt` 和 `exit_info.json`。诊断均为 available/complete，无采集错误或截断。

产物目录：

- Android 16：`/sdcard/xtest-nova/com.miui.gallery/Monkey/20260912_124117`
- Android 13：`/sdcard/xtest-nova/com.sec.android.gallery3d/Monkey/20260912_124143`

## 其他功能链路

- Nova 控件树：两台设备 HTTP 200。
- 默认旧 9008：`jsonRPCProxy=false`，`/jsonrpc/0` 返回 404。
- scrcpy 4.1：两台设备均取得 H.264 视频数据，HOME 控制消息返回 `accepted`。
- Companion 与两套 UiAutomator APK 均由最新自包含 Agent 自动安装。

## 收尾状态

- Foloy 验证目录已删除；其服务器关闭导致应用独立启动 20 秒仍停在 `SplashActivity`，因此不作为本轮探索结论。
- 未使用的 Fixture 只在本机构建过，未安装到设备，随后已从 `dist` 删除。
- Agent 已停止，ADB 转发全部清除。设备上仅保留本轮 Gallery 产物以及最新正式运行组件。

Android 16 Gallery 首次层级读取出现一次 `Read timed out`，系统安全回退后继续完成 4 个动作；未导致断链或产物缺失。复查发现失败的系统 dump 曾留下一个临时窗口 XML，因此已修复错误分支清理并补充回归测试。基于修复后重新构建的 Agent，Android 16 system Provider 连续三次返回 12,458 字节且临时文件为 0；Android 13 Nova Provider 连续三次返回 9,266 字节且临时文件为 0。最终重新执行的 release gate 通过。

剩余观察项：Android 16 的匿名菜单节点能够进入回收站页面，虽然本次立即返回且未执行删除动作，但后续安全策略可考虑结合目标 Activity 名称增加二次限制。
