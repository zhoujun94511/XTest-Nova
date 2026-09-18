# M3.3 A/B 评估验证记录

验证日期：2026-09-03  
验证设备：Xiaomi 2510DPC44G，Android 16 / API 36，ARM64  
目标：建立 Nexus 与 Nova 可审计、可重复且不混淆指标口径的 A/B 评估链。

## 已完成

- 新增 `xtest-evaluation/v1` 统一报告，包含引擎、场景、种子、停止原因、崩溃、安全动作和探索指标；
- 支持从 Nexus 文本日志提取精确 Activity 覆盖率、场景状态发现数、motion、scroll 和自然结束标记；
- Nova 报告直接从会话状态、状态图和步骤生成，并对动作序列生成与时间戳无关的 SHA-256 摘要；
- 比较器对 Activity 覆盖率退化、候选崩溃、危险动作和非正常完成给出 `pass`、`needs-review` 或 `fail`；
- 相同引擎版本、场景和种子的多份报告可以进行动作序列确定性检查；
- 增加 Android 前台 Activity 的完整组件名观测，并把 Activity 写入只读预览和状态图；
- 只有场景提供同版本 APK manifest 的 `expectedActivities` 时才计算覆盖率百分比；状态指纹始终只作为发现数；
- 修复 `deploy.ps1` 在多设备环境下遗漏设备序列号，以及后台启动命令参数被拆分的问题。

## 自动化验证

- `go test ./agent/...`：全部通过；
- 场景文件验证：`android16-foloy-smoke.json` 通过；
- 比较命令完整路径：使用审核基线自比较，判定 `pass`，三个可比较指标差值均为 0；
- Nexus 历史合同：77 项，已覆盖 74 项；`/shell/background` 的 GET/POST 和 `/term` 继续因安全边界禁用。

历史 60 秒 Nexus 基线来自外部参考仓
`XTest-Nexus/docs/history/README_BEFORE_REORGANIZATION.md:102`，该来源不随本仓库
交付：Activity 5/17（29.411764%）、59 个场景状态、5 次 scroll、7 次 motion，
自然完成且无异常。本仓库保留的历史快照位于
[`tests/archive/scenarios/baselines/nexus-android16-foloy-60s.json`](../../../tests/archive/scenarios/baselines/nexus-android16-foloy-60s.json)。

## Android 16 只读验证

本轮没有启动 Nexus、随机 Runner 或 Nova 执行会话，也没有注入点击、滑动或返回事件。临时部署 Nova Agent 后仅执行健康检查、前台读取、页面预览和空会话报告导出：

| 项目          | 结果                                              |
|-------------|-------------------------------------------------|
| Agent 版本    | `xtest-nova-0.8.0-m3.3`                         |
| 前台包         | `tcg.scanner.value.app`                         |
| 前台 Activity | `tcg.scanner.value.app/com.bc.tcg.MainActivity` |
| 页面节点        | 57                                              |
| 安全候选动作      | 13                                              |
| 页面指纹        | `6b4486856df89dcbd6b8416b2214dfe0`              |
| 报告模型        | `xtest-evaluation/v1`                           |
| 空会话状态       | `completed=false`、`steps=0`                     |
| 清理          | `NOVA_M33_CLEAN`                                |

Activity 观测和报告接口均在 Android 16 实际运行通过。完整 A/B 数字仍需在固定目标 APK 版本、补齐 17 个 manifest Activity 清单后，有意识地运行 Nova 执行场景；本轮不以只读预览冒充执行结果。

## 交付物

| 文件                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `C90F63846FFF375BD6A1E6DE5EB66EB1954F05E5DAA077521A69AFD15AE43CC4` |
| `xtest-nova-agent-armv7`   | `3F532AB2BD99943B3C18840B13832BE5F5DEBEC68DD278B8229A4A8EC17F28DF` |
| `xtest-nova-runner.jar`    | `BD0FB6604A368BCB3CFC84C4752C4E26B877014924026BA30EC680DFF34D358A` |
| `xtest-nova-companion.apk` | `BB31ACD31395038924CF71519F5C1028EEFE5885C191244158587293453DCF8C` |

Companion 为 `versionCode=20300`、`versionName=2.3.0-m3.3`，APK Signature Scheme v3 验证通过。签名证书 SHA-256：

`e49a7c6fb2ee0dbc4e6d3b6790d9dcf981d80f6b76628eb84755e7e1da3a46da`

仓库不保存签名口令。最终扫描未发现自动化协作者署名或明文签名口令信息。
