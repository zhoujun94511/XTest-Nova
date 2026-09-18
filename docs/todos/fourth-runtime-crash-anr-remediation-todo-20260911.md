# 第四轮 Crash/ANR 证据链整改计划（2026-09-11）

## 现状

Monkey 会话已有事件流、清单、覆盖率、探索图和关键截图，但没有按本次会话和目标包归档 Logcat、Java Crash、Native Crash 或 ANR 证据。现有 `unexpected_exit`、外跳和 `failure.png` 只能说明执行异常，不能可靠给出故障类型与系统堆栈。

## 实施边界

- 以 Android shell 权限可访问的 Logcat 与 ActivityManager 为主，不要求 Root。
- 只保留本次会话时间范围及目标包相关行，设置命令超时、读取上限和落盘上限。
- `/data/anr`、Tombstone、DropBox 等受 ROM/权限限制的来源不强行读取；诊断元数据必须区分“未检测到”和“采集不可用”。
- Crash/ANR 证据属于 Runner `finalizing` 阶段，全局 `/stop` 必须等待其写入完成。

## Todo

- [x] R10 增加会话级 Logcat/ActivityManager 有界采集与目标包过滤。
- [x] R11 识别 Java Crash、Native Crash 和 ANR，生成结构化计数、故障类型与摘要。
- [x] R12 新增 `logcat.txt`、`crash.json`、`anr.json`、`native_crash.txt`、`diagnostics.txt` 产物，并纳入 `run.json` 哈希。
- [x] R13 Crash/ANR 更新 `stopReason/failureType` 并强制使用 `failure.png`，普通成功/手动停止语义保持不变。
- [x] T6 增加 Java Crash、Native Crash、ANR、时间/包过滤和不可用采集回归测试。
- [x] T7 扩展产物与清单测试，确认诊断文件在 `finalizing=false` 前完成写入。
- [x] V6 Runner 定向测试、Agent 全量测试、`go vet` 和完整项目构建通过。
- [x] V7 Android 真机验证诊断产物生成、全局停止收尾及清理完成。

## 验收标准

1. `run.json` 可直接判断本次会话是否发现 Crash/ANR、故障类型和诊断采集状态。
2. Java/Native Crash 与 ANR 至少保留目标包相关系统摘要；无 Root 权限时不会因受限文件导致整个测试失败。
3. 诊断采集不会无限阻塞或无限增长，也不会归档其他应用的大量日志。
4. 全局 `/stop` 返回成功后，诊断文件与最终清单已经落盘。

## 实施结果

- 新增会话起始时间过滤的 Logcat 全缓冲区快照和 ActivityManager `lastanr/processes` 摘要；每条命令总超时受 Runner 五秒诊断上下文约束，单命令最多保留 4 MiB，目标包文本产物最多 1 MiB。
- Java Crash、Native Crash、ANR 均有解析回归；会话前日志和其他应用日志不会进入本次诊断产物，采集命令不可用时通过 `diagnosticsAvailable/diagnosticErrors` 明示。
- 标准会话新增 `logcat.txt`、`crash.json`、`anr.json`、`native_crash.txt`、`diagnostics.txt`，并全部进入最终 `run.json` SHA-256 清单。
- Android 13 真机通过 `am crash com.xtest.nova.fixture` 触发受控 VM Crash：识别 `crashCount=1`、`failureType=app_crash`，保存 Java 完整堆栈及 `failure.png`。
- 真机全局 `/stop` 验证 12 类会话产物全部非空，最终 `run.json` 已记录 `finalizing=false`；Agent 进程、验证 APK 和 ADB forward 均清理完成。
- 真机专项报告：`tests/reports/fourth-runtime-crash-anr-runner-device-20260911.json`。
- 真机恢复力报告：`tests/reports/fourth-runtime-crash-anr-resilience-20260911.json`；48 次请求、0 失败、PID 稳定、panic 0。

## 能力边界

- 真机已验证 Java Crash；Native Crash 与 ANR 使用等价 Logcat/ActivityManager 样本完成解析测试。未使用会冻结整机的 `am hang` 制造 ANR。
- 非 Root 设备不保证可直接读取 Tombstone、`/data/anr` 或完整 DropBox；当前保存 shell 权限可获得的 debuggerd、Logcat 与 ActivityManager 摘要，并明确记录采集能力。
