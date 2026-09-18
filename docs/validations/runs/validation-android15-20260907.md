# Android 15 图探索、Surface 与隔离副作用资格（2026-09-07）

## 环境与口径

- 设备：Samsung SM-S936U，Android 15 / API 35，ARM64。
- Nova：`xtest-nexus-0.22.0-m5.9-compat`。
- 目标夹具：`com.xtest.nova.fixture`。
- 显示：物理 1440×3120，测试覆盖尺寸 1080×2340；支持 10/24/30/48/60/80/120 Hz。
- 本文把“应用实际提交 FPS”和“显示面板刷新率”分开记录。两者无需相等。

## 非栈式已知图路径回放

夹具增加 `ValidationActivity -> NonStackActivity -> TransientActivity` 路径。最终 40 秒运行使用
seed `20260907`、350 ms 节奏，结果为：

| 指标 | 结果 |
|---|---:|
| 事件 | 16 |
| 状态 | 8 |
| 边 | 14 |
| 点击 / 长按 / 滑动 / 回退 | 9 / 1 / 2 / 4 |
| 已知路径回放 | 1 |
| 层级随机降级 | 0 |

原始证据位于 `tests/reports/android15-qualification-20260907/graph-path/`。`run.json`、事件流、图、
覆盖率及首尾截图同时保留，证明不是仅靠单元测试命中代码分支。

## SurfaceView、高刷新率与 FPS

`SurfaceLoopActivity` 使用独立 `SurfaceView` 绘制循环并请求 120 Hz。设备显示状态确认目标 UID 的
`frameRateOverride=120.00001`、`mActiveRenderFrameRate=120.00001`。

旧采集只读取 `dumpsys gfxinfo` 的 View 帧计数，独立 Surface 因此持续返回 0。现在仅在目标
`gfxinfo` 无增量时启用 `SurfaceFlinger --timestats`，按包含目标包名且图层 ID 最新的活动图层
累计计数计算应用提交 FPS；不使用系统总帧数，也不把历史图层相加。该选择规则修复了 Activity
重启后旧图层累计帧数更大、导致 FPS 被错误钉在 0 的问题。资格脚本同时要求至少 3 个不低于
20 FPS 的样本，避免“只有非空值”造成假阳性。最终制品 10 秒复测得到 7/7 个合格样本，范围
78.27–88.32 FPS，平均 81.63 FPS；屏幕为 120 Hz，应用提交 FPS 与面板刷新率分别记录。

证据：`surface-perf-final.csv`、`surface.png`、`surface-record.mp4`。SHA-256 分别为：

- `E46BF9F1DD83C28FF4D6C714EB8A4EEA7E0A17393FF5E1711F38C5AB209C12C9`
- `EEEB65EEB6A74BEB2C2486CAC14DA0B19906701E2BBB46CD943BF0A175104C9C`
- `4EDF58F578B6CB8B33D6A80860846ABFC4C6DB243C3E7901E6FB78C8F6B82453`

夹具另增独立 OpenGL ES 2.0 连续渲染循环，避免用 CPU Canvas 负载错误判断 GPU 采集能力。
Android 15 实测 `kgsl` 8 个样本，峰值 87%，平均 79.88%；此前 Canvas 场景的 0% 是没有
GPU 负载的真实结果，不是解析失败。`tests/e2e/validate-rendering-qualification.ps1` 已将 Surface FPS 与
GPU 真负载固化为门禁，并分别标记 Compose 与商业游戏状态。

本轮可重复证据及 SHA-256：

- `rendering-qualification.json`：`26657A682DE5DAB08FF22841B76F088415C24A2528D68C8A07B2222679C70C09`
- `surface-perf-latest.csv`：`DEF1BA0C1B97A61371BEE294402E48D007931DE7AFFC21BF2EC6D99620203B7D`
- `gpu-perf-final.csv`：`B52649A964E71648524B8FE420BF8239F8E819B51B18861926C9C47D00E3A2EE`

## Nexus/Nova 同机双运行

两侧使用同一目标 APK、seed `20260907`、350 ms 节奏和 40 秒墙钟预算，并严格串行运行。
首次 Nexus 运行暴露其 Agent UiAutomator 与恢复 Monkey 争用系统唯一 UiAutomation 注册；错误
路径中的 shell Toast 又产生了误导性的包名/UID `SecurityException`。Nexus 增加独占租约、暂停/
恢复自身 UiAutomator 并保留真实错误后，重新对照已有效完成：Nexus 无崩溃，覆盖 4/6 Activity、
发现 53 个场景状态并记录 13 次 motion；Nova 完成 16 步、8 状态、14 边、2 次滚动和 1 次
已知路径回放。结构化比较由 `blocked` 更新为 `pass`。

两套引擎对“状态”的定义不同，不能直接用 53 与 8 判断优劣；Nova 当前结构化报告没有输出
与 Nexus 同口径的 Activity 覆盖字段，因此该项继续标记为不可比较。

最终证据位于 `tests/reports/android15-qualification-20260907/dual-run-final/`。两侧严格串行切换，
结束后均按自身生命周期停止，设备上没有同时保留两份输入引擎。

## 副作用 HTTP 隔离夹具

`tests/e2e/validate-side-effect-fixtures.ps1` 在执行前要求 Agent 无活动会话，随后验证：

1. 受控文件上传、读取、元信息与精确删除；
2. 配置替换、空规则 AutoPopup 启停及原配置恢复；
3. 目标应用启动、性能会话、录屏会话及产物精确删除；
4. minitouch 自有生命周期或明确的能力跳过；
5. 屏幕唤醒幂等与最终零活动会话。

Android 15 报告通过；原配置重新读取一致，3 个临时文件验证不存在。minitouch 因无可信设备
二进制记为 `qualified-skip`。安装/卸载继续由 `tests/e2e/validate-stateful.ps1` 在独立设备状态下覆盖；
`/shell`、`/shell/background` 与 `/term` 不会为追求覆盖数字而开启。

## 尚未取得的资格

- Android 14：当前连接设备为 Android 13、15、16，没有 Android 14，仍待真机。
- Compose：当前 Android 15 没有受控 Compose 目标 APK，不能用普通 View 结果替代。
- 真实游戏：独立 Surface 与 OpenGL ES 游戏式循环已通过，但真实商业游戏尚未提供，不能宣称商业游戏兼容。
- 跨厂商 GPU：当前 Adreno/KGSL 已有真负载数值，尚不能据此证明 Mali 及其他厂商驱动矩阵。
