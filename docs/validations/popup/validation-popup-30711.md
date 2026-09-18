# Popup 30711 Android 16 独立验收

验证日期：2026-09-04。设备为 Xiaomi 2510DPC44G，Android 16 / SDK 36，ARM64。

## 隔离方式

设备上的原 XTest Popup `com.openatx.xtest.popup` 10248 必须保留。构建脚本新增可选的
`PackageName`、`ArtifactName` 和 `ApplicationLabel`，验收时从同一份 30711 Java 字节码生成
`com.openatx.xtest.popup.validation`，与原包并存。组件类名改为全限定名，正式包名及其外部
合同不变。验收结束只卸载隔离包。

原 10248 在验收前后均为 `versionCode=10248`、
`versionName=1.0.2-android16-unicode-record-replay-r7`，`lastUpdateTime` 保持
`2026-09-02 15:28:03`，未被覆盖、卸载或清除数据。

## 真机结果

完整业务任务流先由隔离包 30710 执行；专项中发现撤权残留窗口后，生产代码唯一增量为
`OverlayService` 的权限守卫及版本号。随后重建正式包与隔离包 30711，两者的
`classes.dex` SHA-256 均为
`4690B48A4D49215249D4CBE22946423D72EB7680BCEA2E846CF8993568AF4CF7`，并在 30711 上
重复执行完全停止后的启动、撤权、拒绝、恢复和退出回归。以下结果以完整业务流与该最小
增量回归共同构成最终验收证据，不把 30710 的发现阶段伪称为未经修改的 30711 全量复跑。

- 无界面启动：前台始终为 `com.xtest.nova.fixture/.ValidationActivity`；没有空白 Activity，
  服务窗口类型为 `APPLICATION_OVERLAY`。
- 主菜单：性能测试、录制回放、Monkey、最小化、退出五项名称与顺序正确。
- 性能：目标显示为“目标应用样例”，PID、CPU、系统 CPU、内存、普通 View FPS、电池状态、
  电量和温度持续刷新；该机不支持的 GPU 与瞬时电流显示 `--`，没有伪造数值。CSV 落入
  `/sdcard/xtest-nexus/com.xtest.nova.fixture/Perf/20260904_143459/perf.csv`。
- 录制回放：建立 `task-0904 / case-0904-143708`，自动采集聚焦非密码输入框最终文本并
  增加截图断言，共 2 个动作；签名用例保存到目标包的 `Replay` 目录，回放结果
  `actions=2`、`completedActions=2`、`stopReason=completed`。
- Monkey：只对 `com.xtest.nova.fixture` 运行，18 秒产生 23 个事件；主动停止后 API、进程
  和结构化日志状态一致。
- 最小化：Android 16 实测窗口由 `246x408` 像素变为 `60x60` 像素，点击气泡恢复为
  `246x408`。
- 退出：窗口和 `OverlayService` 均消失，Agent 与目标应用不受影响。

## 撤权缺陷与修复

初次专项测试发现，Android 16 通过 AppOps 在运行中撤销悬浮权限时，系统不会立即替应用
移除已经存在的窗口。30711 在服务主线程加入每秒一次的轻量权限守卫；撤权后 1 秒内调用
`stopSelf()`，由统一销毁路径移除窗口、取消轮询并关闭工作线程。

从完全停止状态重复验证结果：

- 授权启动：窗口与服务均存在；
- 运行中撤权：窗口移除、服务停止；
- 无权限启动：不创建服务，且不改变目标前台；
- 重新授权启动：窗口与服务均恢复；
- 用户点击退出：窗口与服务均无残留。

## 自动门禁

- 全量 Go 测试通过；
- Windows Go 官方竞态检测连续 3 次通过；
- 发布门禁通过，HTTP 合同仍为 77/77；
- 首次提交候选门禁通过，共 184 项（包含后续新增的 Android 12 验收报告）；
- 最终正式 APK 为 versionCode 30711、versionName
  `3.7.11-android16-permission-guard`，签名证书 SHA-256 保持
  `E49A7C6FB2EE0DBC4E6D3B6790D9DCF981D80F6B76628EB84755E7E1DA3A46DA`。
- 正式包与不同包名验收包的 `classes.dex` 摘要完全一致，隔离构建没有替换业务实现。

## 证据

- `tests/reports/popup-30710/android16-menu.png`；
- `tests/reports/popup-30710/android16-performance.png`；
- `tests/reports/popup-30710/android16-record-new.png`；
- `tests/reports/popup-30710/android16-record-running.png`；
- `tests/reports/popup-30710/android16-record-saved.png`；
- `tests/reports/popup-30710/android16-replay-complete.png`；
- `tests/reports/popup-30710/android16-monkey-form.png`；
- `tests/reports/popup-30710/android16-monkey-running.png`；
- `tests/reports/popup-30710/android16-minimized.png`。

M5.8F3 已关闭。Android 14/15、SurfaceView/游戏 FPS、ARMv7 等设备矩阵限制继续由
[`compatibility-waivers.md`](../../compliance/compatibility-waivers.md) 管理，不属于 M5.8F 未实现功能。
