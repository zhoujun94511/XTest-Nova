# 可信执行与启动性能三设备验证（2026-09-12）

## 环境

| 设备 | Android | 目标应用 |
| --- | ---: | --- |
| `83fc400c` / 2510DPC44G | 16 / API 36 | `com.miui.gallery` |
| `R3CY80B2G4W` / SM-S936U | 15 / API 35 | `com.sec.android.gallery3d` |
| `R5CN30EQKNM` / SM-G9860 | 13 / API 33 | `com.sec.android.gallery3d` |

三台均部署同一 ARM64 Agent，SHA-256 为
`C7731EDBF30E44D0665BE3483BE8BC7F1A9DB73141B25812188C079A1FBE8D3D`。服务版本均为
`xtest-nova-0.26.0-m6.3-auto-popup`，组件诊断 `degraded=false`。

## 结果

每组包含 5 个有效样本，单位为毫秒。

| 设备 | 场景 | P50 | P90/P95 | 平均值 | 判定 |
| --- | --- | ---: | ---: | ---: | --- |
| Android 16 | 首轮冷启动 | 264 | 283 / 283 | 265.2 | `not_tested`，未配置基线 |
| Android 16 | 暖任务恢复 | 16 | 24 / 24 | 17.6 | `not_tested`，未配置基线 |
| Android 16 | 冷启动基线复测 | 216 | 254 / 254 | - | `passed`，基线 283、上限 566 |
| Android 15 | 首轮冷启动 | 89 | 90 / 90 | 88.4 | `not_tested`，未配置基线 |
| Android 15 | 暖任务恢复 | 11 | 18 / 18 | 12.0 | `not_tested`，未配置基线 |
| Android 15 | 冷启动基线复测 | 86 | 99 / 99 | - | `passed`，基线 90、上限 180 |
| Android 13 | 首轮冷启动 | 152 | 178 / 178 | 152.6 | `not_tested`，未配置基线 |
| Android 13 | 暖任务恢复 | 33 | 38 / 38 | 29.0 | `not_tested`，未配置基线 |
| Android 13 | 冷启动基线复测 | 139 | 143 / 143 | - | `passed`，基线 178、上限 356 |

首次暖启动暴露了真实兼容缺陷：三台设备在从桌面恢复已预热任务时均只返回 `WaitTime`，没有
`TotalTime`。实现已修正为冷启动严格使用 `TotalTime`；暖任务恢复缺少该字段时使用
`WaitTime`，并在每个样本记录 `durationSource=wait_time`。修正后三台均为 5/5 成功。

## 产物与完整性

设备端每台保留三组独立 `Startup/<设备本地时间>/` 目录，每组包含 `startup.json` 和
`evidence.json`。九组产物已拉回 `tests/reports/trusted-execution-device-validation-20260912/`。

- Android 16：`20260912_193807`、`20260912_194009`、`20260912_194023`。
- Android 15：`20260912_193806`、`20260912_194009`、`20260912_194022`。
- Android 13：`20260912_193807`、`20260912_194009`、`20260912_194023`。

九份证据索引均满足：`startup.json` SHA-256 匹配、attemptId 非空、没有 `ownerToken`、没有越界
证据路径。Android 16 和 Android 13 的替换前 Agent 分别备份为
`/data/local/tmp/xtest-nova-agent.pre-trusted-execution-20260912`；Android 15 原先没有标准 Agent。

