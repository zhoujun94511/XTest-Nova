# Gallery 目标应用切换与真机烟测（2026-09-12）

## 调整结果

- 当前可执行长测入口改为 `tests/e2e/run-gallery-long-audit.ps1`，默认目标名为 `gallery`。
- 脚本按顺序识别 `com.miui.gallery`、`com.sec.android.gallery3d`、`com.android.gallery3d`，最后才回退 Google Photos；也允许显式传入其他 Gallery 包名。
- `tests/scenarios/` 提供 Android 16 MIUI Gallery 和 Android 13 Samsung
  Gallery 两个安全场景。
- 删除、回收站、编辑、分享、发送、设为等中英文动作进入拒绝列表。
- 历史 Foloy 数据不改名冒充 Gallery 黄金基线；Gallery 基线必须在相同设备、应用版本和场景下重新采集。
- 每个 Go run 必须达到 `MinimumGoStepsPerRun`，终态稳定，评估报告完整，
  且拉取的步骤、图、回执和证据索引均为非空并通过哈希校验。每个 Java run
  必须达到 `MinimumJavaEventsPerRun`，正常退出，诊断完整，且完整非空产物集
  通过 manifest 哈希校验。任一 run 失败都会使 `summary.json` 的 `passed`
  为 `false` 并令脚本失败。
- 真机长测只通过手动触发的 self-hosted Windows CI job 执行，调用方必须明确
  提供 runner 标签和 adb serial；设备缺失或未授权会失败，不使用 hosted runner
  冒充真机结果。两个最低工作量阈值也可由手动触发输入覆盖。

## 真机结果

| 设备 | 场景 | 结果 |
|---|---|---|
| 2510DPC44G / Android 16 | `com.miui.gallery` | 执行 3 步、发现 2 状态；进入 MIUI Security Center 权限页后没有匹配到安全动作，按设计以 `special_unhandled` 停止，未执行删除、编辑或分享。 |
| SM-G9860 / Android 13 | `com.sec.android.gallery3d` | 执行 5 步、发现 4 状态和 5 条边，以 `max_steps` 正常结束，无运行错误。 |

同一轮性能采集均按 2 秒间隔、6 秒预算自动停止：Android 16 和 Android 13 各写入 3 个样本、0 个失败样本，应用网络均为 `package_uid` / `dumpsys-netstats` 口径。产物目录分别为：

- `/sdcard/xtest-nova/com.miui.gallery/Perf/20260912_184026/`
- `/sdcard/xtest-nova/com.sec.android.gallery3d/Perf/20260912_184026/`

