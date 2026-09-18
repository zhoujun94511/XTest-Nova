# 全链路审计整改 Todo（2026-09-11）

## 目标

依据全链路 Review 与 XTest 既有异常签名行为，修复异常退出漏报、重复异常膨胀、构建错误吞没及长测假成功，并收紧日志和产物生命周期边界。

## P1：结果可信度

- [x] A1 按 `ApplicationExitInfo` 记录块解析真机多行格式，同时兼容单行格式；解析不完整时不得声明诊断完整。
- [x] A2 对 Java Crash、Native Crash、ANR 建立稳定签名；首次保留完整详情，后续同签名只累计次数、首末时间和证据来源。
- [x] A3 Runner 的异常计数改为累计实际发生次数，报告条目保持按签名聚合，避免跨 logcat/events/exit-info 重复计数。
- [x] A4 为三星真机格式、解析失败、跨来源同次异常、跨时间重复异常增加回归测试。

## P1：构建与交付

- [x] B1 Runner、Companion、Validation Fixture、Game Demo 使用干净的 classes/dex 中间目录，避免已删除类残留。
- [x] B2 所有 `jar`、`zipalign`、`apksigner` 原生命令失败立即终止，禁止复用旧签名产物。
- [x] B3 CI 使用临时测试证书完成签名六产物构建、release manifest 和完整发布门禁。

## P2：编排与生命周期

- [x] C1 Foloy 长测要求 `0 < GoMinutes < TotalMinutes`，Go/Java 两阶段必须实际执行，Java 诊断和产物不完整时失败。
- [x] C2 目标日志 PID 在明确进程死亡后及时失效，避免 PID 重用导致日志串包。
- [x] C3 会话清理只删除合法时间戳会话目录，保留人工归档和未知目录。
- [x] C4 更新异常产物接口文档，明确“唯一签名数”和“实际发生次数”的口径。

## 验证

- [x] V1 Go 定向及全量测试、`go vet`、Race 检测通过。
- [x] V2 PowerShell 全量语法解析、首次提交候选门禁通过。
- [x] V3 干净正式构建、六产物 manifest、完整发布门禁通过。
- [x] V4 真机执行 Java Crash、ANR 与正常 Foloy 烟测，验证多行 Exit Info、聚合计数和 13 个产物。

## 外部交付项

- [ ] E1 创建并推送首次 Git 提交，使 CI、差异 Review 和版本回滚真正生效（需要仓库所有者决定提交边界）。

## 验证记录

- XTest 参考：读取 Nexus 恢复源码中的 Crash 堆栈签名和“首次详情、后续相同 crash 提示”语义；Nova 将该语义扩展到 Java Crash、Native Crash、ANR 和其他异常退出。
- 自动验证：Go 全量测试、`go vet`、完整 Race、PowerShell 全量解析和首次提交候选门禁通过；错误的 Foloy 时长拆分会在接触设备前被拒绝。
- 发布验证：干净正式构建成功，六个正式产物 manifest 已刷新，完整发布门禁通过。
- Java Crash 真机：三星 `R5CN30EQKNM` 的多行、嵌套括号 Exit Info 成功解析；general log、incident buffers、exit-info 三路证据折叠为一次，`crashCount=1`、`abnormalExitCount=1`、`diagnosticsComplete=true`。
- ANR 真机：`anrCount=1`、`failureType=app_anr`、`diagnosticsComplete=true`。
- Foloy 真机：正常完成，5 个动作、4 个状态，Crash/ANR/Native/异常退出均为 0，诊断完整。
- 三组真机运行均生成 13 个文件、12 个 manifest 哈希，复算无差异；证据位于 `tests/reports/full-chain-remediation-20260911`。
- E1 未自动执行：当前仓库仍无首次提交，创建提交会决定整个项目的版本边界，需要仓库所有者明确授权。
