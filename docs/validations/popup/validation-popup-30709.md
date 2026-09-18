# Popup 30709 录制与 Monkey 高级策略验收

> 历史快照：目标页签名用例调度与可读性能窗已由 30710 完成，见
> [`validation-popup-30710.md`](validation-popup-30710.md)。

验证日期：2026-09-04。设备：Samsung Android 13（SDK 33）隔离夹具。

## 录制任务与最终文本

- 新录制接受 `task=checkout`、`name=focused-text`，状态和最终 Case 均保留任务层级。
- 自动最终文本接口从当前聚焦的非密码 `EditText` 读取 `最终文本-杭州`，记录
  `focus=true` 和归一化中心坐标 `(0.5, 0.954791...)`。
- 停止后原子保存至
  `/sdcard/xtest-nexus/com.xtest.nova.fixture/Replay/checkout/20260904T060131.443732435Z/case.json`，
  文件包含 SHA-256 完整性摘要。
- 白盒测试确认没有显式坐标时可继承最近一次真实目标点击；密码输入框会被拒绝。

## Monkey 高级策略

- 电量阈值设为 100% 时，Runner 以 `low_battery` 正常结束，事件数为 0。
- 目标页 `com.xtest.nova.fixture/.ValidationActivity` 首次出现时写入 `target_page` 日志。
- 控件黑名单命中 `目标应用样例` 时连续写入 `guarded`，随机事件数保持 0。
- Activity 黑名单命中夹具 Activity 时同样写入 `guarded`，事件数保持 0。
- 控件层级复用 Agent 已有层级采集链，不再由 Runner 每轮启动新的 UiAutomator 进程；
  Activity、电量和控件探针分别限频，避免高级策略放大 CPU 与进程开销。
- 30709 签名包真机状态为 `installed=true running=true versionCode=30709`，系统窗口类型为
  `APPLICATION_OVERLAY`，前台仍为 `com.xtest.nova.fixture/.ValidationActivity`。
- `tests/reports/popup-30709/monkey-advanced.png` 保存高级配置页真机截图；层级断言确认低电量、
  Activity 白名单/黑名单、目标页面和控件黑名单字段均真实存在。

本报告关闭 M5.8F2d，并完成 M5.8F2e 的低电量、Activity 名单、目标页观测和控件黑名单。
“目标页命中后自动执行指定录制用例”仍需解决 Runner/Replay 输入会话互斥，M5.8F2e 暂不关闭。
