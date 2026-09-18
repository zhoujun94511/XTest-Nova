# Popup 30711 Android 12 真机验收

验证日期：2026-09-04。设备为 Google Pixel 3a XL，Android 12 / SDK 32，ARM64，
设备序列号 `939AX05WZJ`。设备验收前没有安装 XTest Popup 或验证夹具，因此直接安装正式
签名 APK `com.openatx.xtest.popup` 30711；结束后卸载测试产物并恢复原前台。

## 自动化链路

- Runner：启动、同请求幂等、并发请求 HTTP 409、主动停止及停止后日志稳定均通过；
  测试运行产生 3 个隔离事件。
- Popup Assistant：当前中文夹具的唯一文本规则命中、点击并停止通过。
- 录制回放 API：返回键与截图断言共 2 个动作，完整性签名校验通过，回放 2/2 完成。
- 流服务：App Event 收到 40 字节事件；minicap 返回 55197 字节有效画面；Monitor 与
  rotation 0 正常。

首次 M5.6 回归暴露的是验证脚本仍引用旧英文夹具文案及旧 `/sdcard/XTestNova` 路径。
脚本已改为当前“目标应用样例”唯一文本和
`/sdcard/xtest-nexus/<目标包>/Replay/<任务>/<时间>/case.json`，重跑通过。

## 正式 Popup 用户流程

- 安装和状态：`versionCode=30711`、SDK 32、`installed=true`、`running=true`。
- 无空白页：启动后前台保持 `com.xtest.nova.fixture/.ValidationActivity`，Popup 仅创建
  `APPLICATION_OVERLAY` 窗口。
- 主菜单：性能测试、录制回放、Monkey、最小化、退出五项正确显示。
- 性能：目标标题为“目标应用样例”；PID、CPU、系统 CPU、内存、普通 View FPS、网络、
  电池原值/状态/电量/温度持续刷新。样本 FPS 约 1.95，电流约 +209～259 mA，系统状态为
  充电中 89%，不支持的 GPU 显示 `--`。67 行会话正常停止，CSV 位于
  `/sdcard/xtest-nexus/com.xtest.nova.fixture/Perf/20260904_145809/perf.csv`。
- 悬浮录制：通过 UI 建立 `task-0904 / case-0904-150000`，最终文本和截图断言共 2 个
  动作；从悬浮用例列表启动后回放 `completedActions=2`、`stopReason=completed`。
- 悬浮 Monkey：从高级表单启动，只作用于测试夹具；19 秒产生 24 个事件，停止后 API
  与结构化日志一致。
- 最小化：主窗口 `205x340` 像素，最小化气泡 `50x50` 像素，点击后恢复。
- 权限：运行中撤权后窗口移除且服务停止；无权限启动不创建窗口并保持目标前台；重新
  授权后窗口和服务恢复。
- 退出：窗口与服务均无残留，Agent 不被 Popup 退出操作连带停止。

## 证据与结论

- `tests/reports/android12-runner-30711.json`；
- `tests/reports/android12-m56-30711.json`；
- `tests/reports/popup-30711/android12-menu.png`；
- `tests/reports/popup-30711/android12-performance.png`；
- `tests/reports/popup-30711/android12-record-menu.png`；
- `tests/reports/popup-30711/android12-record-running.png`；
- `tests/reports/popup-30711/android12-record-saved.png`；
- `tests/reports/popup-30711/android12-replay-complete.png`；
- `tests/reports/popup-30711/android12-monkey-form.png`；
- `tests/reports/popup-30711/android12-monkey-running.png`；
- `tests/reports/popup-30711/android12-minimized.png`。

Android 12 正式 Popup 完整任务流通过。GPU 缺省是设备节点不支持时的预期行为，继续由
W-003 管理；不是使用零值伪造成功。
