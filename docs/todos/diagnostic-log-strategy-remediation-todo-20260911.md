# 异常日志采集策略整改 Todo（2026-09-11）

## 目标

基于 Foloy 10 分钟正常流程的真实日志证据，消除“先截断全局日志、再按目标过滤”造成的会话尾部丢失，并确保 Crash、ANR、Native Crash 与非崩溃异常退出的采集不受普通日志体量影响。

## P1

- [x] D1 将 crash buffer、events、lastanr、processes、exit-info 改为独立预算并发采集，关键事故源不再排在全量 logcat 之后。
- [x] D2 Runner 会话启动时同步启动目标范围 logcat 流，先按包名/PID过滤，再进入有界头尾缓冲；停止时封存完整会话尾部。
- [x] D3 将全局 `diagnosticsTruncated` 细分为来源级 available/truncated/error，并增加事故证据完整性状态。
- [x] D4 新增结构化 `exit_info.json`，保存会话内系统杀进程、低内存、信号、Crash、ANR 等退出原因。

## P2

- [x] D5 对诊断日志中的常见 token、用户/设备标识字段执行脱敏，并在产物元数据中声明脱敏状态。
- [x] D6 保留兼容字段和现有 Crash/ANR 判定；更新 Runner 产物清单、长测脚本及接口文档。

## 验证

- [x] V1 新增流式过滤、头尾截断、脱敏、来源状态和 exit-info 解析单元测试。
- [x] V2 Go 全量测试、`go vet` 与 Race 检测通过。
- [x] V3 PowerShell 全量语法解析及首次提交门禁通过。
- [x] V4 重建正式 Agent、刷新六产物 manifest 并通过完整发布门禁。
- [x] V5 Foloy 真机短时 Runner 验证：生成 13 个产物，普通日志覆盖运行尾部，事故源完整且无采集错误。

## 验证记录

- 单元与静态门禁：`go test ./...`、`go vet ./...`、完整 Race、PowerShell 全量语法解析、首次提交门禁均通过。
- 发布门禁：正式六产物完成重建，`dist/release-manifest.json` 已刷新，完整发布校验通过。
- Foloy 真机：设备 `R5CN30EQKNM`，45 秒 Runner 正常完成；产生 13 个文件、12 个内容哈希且复算无差异。
- 日志质量：`diagnosticsComplete=true`，7 个来源均可用，无来源错误；普通日志与事故日志均未截断，日志尾部距 `endedAt` 约 1.325 秒。
- 异常结论：Java Crash、ANR、Native Crash、Application Exit Info 异常退出均为 0；`exit_info.json` 标记 `complete=true`、`redacted=true`。
- 真机证据目录：`tests/reports/foloy-diagnostic-log-v2-20260911/20260911_190442`。
