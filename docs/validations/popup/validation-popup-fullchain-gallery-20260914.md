# 悬浮窗三机全链路真机验证（Gallery）

验证日期：2026-09-14

## 设备与目标应用

| 设备 | Android | 目标应用 |
| --- | ---: | --- |
| `R5CN30EQKNM` | 13 | Samsung Gallery `com.sec.android.gallery3d` |
| `R3CY80B2G4W` | 15 | Samsung Gallery `com.sec.android.gallery3d` |
| `83fc400c` | 16 | Xiaomi Gallery `com.miui.gallery` |

所有业务操作均从 Companion 悬浮窗进入。HTTP 状态接口仅用于核对悬浮窗动作是否真正到达 Agent、会话归属、最终状态和产物路径。

## 验证结论

### 性能测试

三台设备均从悬浮窗选择 Gallery、启动采集、展示实时指标并停止落盘。

| 设备 | 样本 | 失败 | 部分样本 | 结果 |
| --- | ---: | ---: | ---: | --- |
| Android 13 | 45 | 0 | 1 | 通过 |
| Android 15 | 25 | 0 | 0 | 通过 |
| Android 16 | 25 | 0 | 25 | 通过（GPU 指标降级） |

Android 16 的 `partialSamples=25` 不是整次采集失败。CPU、内存、FPS/Jank、电池、网络均已写入；该 ROM 没有暴露受支持的 GPU 利用率来源，每行明确记录 `supported GPU utilization source unavailable`，符合可用指标继续采集的降级策略。

性能摘要：

- Android 13：`/sdcard/xtest-nova/com.sec.android.gallery3d/Perf/20260914_100103/summary.json`
- Android 15：`/sdcard/xtest-nova/com.sec.android.gallery3d/Perf/20260914_100129/summary.json`
- Android 16：`/sdcard/xtest-nova/com.miui.gallery/Perf/20260914_100130/summary.json`

### 录制与回放

三台设备均完成“选择 Gallery → 新建录制 → 添加截图断言 → 完成保存 → 已保存用例 → 点击回放”。三次回放均为 `completedActions=1`、`stopReason=completed`、`verdict=passed`，判定理由为截图检查点和动作回执齐全。

| 设备 | 用例 | 指纹 | 回放产物 |
| --- | --- | --- | --- |
| Android 13 | `task-0914/case-0914-101811` | `sha256:4d0455b9…` | `/sdcard/xtest-nova/com.sec.android.gallery3d/ReplayRuns/20260914_102117` |
| Android 15 | `task-0914/case-0914-101810` | `sha256:bc790a5a…` | `/sdcard/xtest-nova/com.sec.android.gallery3d/ReplayRuns/20260914_102116` |
| Android 16 | `task-0914/case-0914-101948` | `sha256:4ed0b1b5…` | `/sdcard/xtest-nova/com.miui.gallery/ReplayRuns/20260914_102118` |

每个回放目录均实际存在 `replay.json`、`receipts.json`、`evidence.json`。

### Monkey

Gallery 探索链路在三台设备均启动、产生状态/动作并完成诊断收尾：

| 设备 | 事件 | 状态 | 结束原因 | Crash/ANR/Native Crash |
| --- | ---: | ---: | --- | --- |
| Android 13 | 36 | 19 | `graph_exhausted` | 0/0/0 |
| Android 15 | 51 | 18 | `external_package` | 0/0/0 |
| Android 16 | 2 | 1 | `external_package` | 0/0/0 |

Android 13/15 另以修复后的 Companion 在运行中点击“停止”，均得到 `stopReason=stopped`，诊断完整。Android 16 同样完成了修复后主动停止协议验证；该次控制回归使用系统 Game Center，Gallery 的正常探索与自动收尾由上表单独覆盖。

三台 Gallery 运行目录均实际包含：

- `events.jsonl`
- `run.json`
- `activity_coverage.json` / `activity_coverage.txt`
- `exploration_graph.json`
- `start.png` / `finish.png`
- `logcat.txt` / `diagnostics.txt`
- `crash.json` / `anr.json` / `native_crash.txt`
- `exit_info.json` / `evidence.json`

`crash.json` 和 `anr.json` 均为 `available=true`、`complete=true`、空 incidents；这表示异常采集链路完整且本轮未发生目标应用异常，不是未采集。

## 真机发现并修复的问题

可信执行所有权上线后，Companion 的录制 Stop、回放 Stop 和 Monkey Stop 没有携带启动响应中的 `sessionId/ownerToken`。结果是 Agent 正确拒绝旧式无归属停止请求，但悬浮窗无法结束自己创建的会话。录制界面的状态轮询还会覆盖短暂错误提示，使问题表现为“完成按钮无响应”。

修复内容：

1. Companion 从启动和状态响应中保存 recording/replay/monkey 的执行身份。
2. 三类 Stop 请求携带 `X-XTest-Session-Id` 与 `X-XTest-Owner-Token`。
3. 录制完成前推进 UI generation，终止旧轮询，失败提示不再被覆盖。
4. 新 Companion 已同步进 Agent runtime bundle，arm64/armv7 单文件 Agent 和发布清单已重新生成。

## 最终环境状态

- 三台 Agent 健康检查均为 `status=ok`。
- 三台悬浮窗均为 installed/running，versionCode `30715`。
- 三台当前无 Runner、探索、录制、回放、录屏或性能会话残留。
- 三台层级 Provider 均恢复为 `nova-provider`。
- `dist/xtest-nova-companion.apk` 与 Agent 内嵌 Companion SHA-256 一致：`48C01B2CD48F5EA85CFE98808BF77C75A8BE1C6493B6C6E61FA44DBCE4602018`。

## 本地证据

真机界面截图保存在 `tests/reports/popup-fullchain-20260914/`，包含性能选择/运行、录制运行/保存列表、Monkey 配置/运行及修复后复测画面。
