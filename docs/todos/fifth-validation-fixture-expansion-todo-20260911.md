# 第五轮目标样例应用与场景验证计划（2026-09-11）

## 目标

将 `com.xtest.nova.fixture` 从基础点击/长列表/渲染负载夹具扩展为分层、可重复、可清理的功能与故障场景库，用同一个隔离应用验证 Agent、Runner、UiAutomator、特殊页处理、WebView、录制回放和诊断收尾，并通过真机运行进一步发现实现缺口。2026-09-11 起游戏探索和四类异常场景也已并入该单一夹具，迁移清单见 [`unified-validation-fixture-todo-20260911.md`](unified-validation-fixture-todo-20260911.md)。

## 场景矩阵

| 场景 | 夹具入口 | 主要覆盖 | 核心断言 |
|---|---|---|---|
| F1 基础启动与导航 | `ValidationActivity` | `/session`、前台包、层级树、滚动入口 | 主页面及五个场景入口可被层级采集识别 |
| F2 复杂表单 | `FormScenarioActivity` | 文本/数字/邮箱/密码、重复文本、禁用控件、动态节点、长页滚动 | 聚焦普通输入可记录；密码输入受保护；禁用控件不可执行 |
| F3 异步弹窗 | `DialogScenarioActivity` | AlertDialog、延迟 UI、唯一文案、确认/取消 | 层级能观测弹窗；AutoPopup 可按精确规则处理 |
| F4 运行时权限 | `PermissionScenarioActivity` | Android Permission Controller、允许/拒绝策略、回到目标包 | 未授权时出现系统权限页，处理后恢复目标应用 |
| F5 本地 WebView | `WebViewScenarioActivity` | 调试 WebView socket、HTML 输入/脚本节点 | `/webviews/{pkg}` 能发现目标进程 WebView |
| F6 生命周期与外跳 | `LifecycleScenarioActivity` | 重建、持久计数、系统设置外跳、返回恢复 | 重建状态递增；外跳包可识别并能返回目标包 |
| F7 受控 Java Crash | `FaultScenarioActivity(mode=crash)` | Crash 识别、失败截图、Logcat/清单 | `failureType=app_crash` 且证据文件完整 |
| F8 受控 ANR | `FaultScenarioActivity(mode=anr)` | ANR 识别、超时恢复、诊断收尾 | `failureType=app_anr`，验证后强制恢复且不留冻结进程 |
| F9 图/渲染负载 | 既有 NonStack/Surface/GLES | 图路径、循环检测、FPS/GPU/录屏 | 既有资格验证继续通过，无回归 |

## Todo

- [x] E1 新增复杂表单、异步弹窗、权限、本地 WebView、生命周期和受控故障 Activity。
- [x] E2 主入口增加五个安全场景导航，故障 Activity 不接入随机探索入口。
- [x] E3 建立机器可读场景目录，声明入口、风险、预期能力和清理动作。
- [x] E4 建立真机场景验证脚本，覆盖启动、层级、表单保护、弹窗、权限、WebView 和生命周期。
- [x] E5 扩展 Runner 真机脚本，覆盖夹具触发的 Java Crash/ANR 及完整诊断产物。
- [x] T8 夹具编译、Manifest/场景目录静态检查及脚本语法检查通过。
- [x] V8 Android 真机场景矩阵通过，验证期间发现的问题形成明确结论并修复或登记。
- [x] V9 Agent 全量测试、`go vet`、完整构建、竞态检测和恢复力验证通过。

## 安全约束

- Fault Activity 只能由验证脚本显式启动；普通 Runner 不展示 Crash/ANR 导航。
- ANR 只冻结验证应用主线程，验证脚本设置总期限并在 finally 中 `force-stop`/卸载夹具。
- 权限场景记录并恢复验证应用原权限，不改变其他应用权限。
- WebView 只加载内置 HTML，不访问公网。
- 每次设备验证必须确认 Agent、夹具进程和 ADB forward 均已清理。

## 实施与审计结论

- 场景目录为 `tests/scenarios/validation-fixture-matrix.json`，当前包含 12 个唯一场景、14 个已声明 Activity；夹具版本提升为 `3.0-unified`。
- 新增 `tests/e2e/validate-fixture-scenarios.ps1`。真机覆盖了可见层级滚动、普通文本采集、密码拒采、延迟弹窗、三星权限控制器、权限页返回、本地 WebView socket、进程重启计数、系统设置外跳与返回。
- `tests/e2e/validate-fixture-scenarios.ps1` 要求显式传入正式签名后的扩展 APK，避免误用仓库中尚未重签的旧 APK；临时测试证书没有覆盖正式产物。
- `tests/e2e/validate-runner.ps1` 支持显式传入夹具 APK，并使用隔离故障入口验证真实 Java Crash、真实 input-dispatch ANR、故障后恢复和全局 Stop 的 12 项产物；对旧版夹具保留兼容并明确标记跳过受控 ANR。
- 发现并修复跨运行污染：`dumpsys activity lastanr` 是系统持久全局状态，旧 ANR 曾被无条件计入后续正常运行，使其错误生成 `failure.png`。现仅使用按当前 `startedAt` 过滤的 logcat 事件分类；`lastanr` 仍保留在 `diagnostics.txt` 中作为上下文证据。
- 发现并修复样例密码控件的平台差异：仅设置 `inputType` 时，目标三星设备层级仍报告 `password=false` 并暴露文本；现显式设置密码转换，Agent 已实测返回 HTTP 409 拒绝采集。
- 验证器兼容三星 Android 13：权限状态改从 `dumpsys package` 读取，并按“当前可见层级”语义分阶段滚动断言，避免测试误报。
- 竞态检测发现回归测试直接无锁读取 Runner 私有控制令牌；测试改为在同一互斥锁下读取。全量 `go test -race` 复测通过，生产状态机未发现对应竞争。

## 验证结果

| 验证 | 结果 |
|---|---|
| 场景目录、Manifest 和 PowerShell 语法静态检查 | 通过：9 场景、12 Activity、版本 `2.0-expanded` |
| Android 13 / Samsung SM-G9860 安全场景 | 通过：F1-F6 |
| Runner 状态机、真实 Crash/ANR、故障恢复、全局 Stop | 通过；12 项收尾产物完整 |
| 旧版 v1 签名夹具兼容 | 通过；明确跳过 v2 受控 ANR，其余 Runner/Crash/Stop 与 12 项产物通过 |
| Agent 全量单元测试与 `go vet ./...` | 通过 |
| 完整构建 | 通过：Runner、Companion、扩展夹具、UiAutomator、ARM64/ARMv7 Agent |
| `go test -race ./agent/...` | 通过 |
| 30 秒恢复力验证 | 通过：48 次请求零失败、PID 稳定、goroutine 7→7、无 panic |
| 设备清理 | 通过：Agent 不在运行、夹具已卸载、ADB forward 为 0 |

机器验证报告：

- `tests/reports/fifth-fixture-scenarios-20260911.json`
- `tests/reports/fifth-fixture-runner-faults-20260911.json`
- `tests/reports/fifth-fixture-resilience-20260911.json`
- `tests/reports/fifth-runner-legacy-fixture-compat-20260911.json`
