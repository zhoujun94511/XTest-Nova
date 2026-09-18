# XTest Nexus 对照审计整改计划与 Todo（2026-09-11）

## 目标

吸收旧项目中可复用的运维诊断能力，同时保持 Nova 已有的模块边界、安全约束、生命周期状态机和发布回滚机制。

## 实施清单

- [x] P0 修复真机发布介质校验仍只识别旧录屏目录的问题，并校验文件确实被删除。
- [x] P0 新增版本化组件健康矩阵，覆盖 Agent、Companion、Monitor、UIAutomator、Minitouch、AutoPopup、Scrcpy 与执行会话。
- [x] P0 新增受限的产物目录与守护日志查询 API，并接入 Web Console；禁止任意路径读取。
- [x] P0 将守护日志轮转抽成独立组件，支持多份备份、压缩和过期清理。
- [x] P1 将 Companion/Monitor 端口冲突处理抽成启动组件：仅在健康及版本握手通过时复用，否则显式降级，主 Agent 不被辅助端口拖垮。
- [x] P1 新增有界的设备端 soak 诊断任务，样本流式写入 JSONL，内存只保留最新窗口，支持停止和重启残留识别。
- [x] P1 增加 Go 离线依赖可复现性检查，确保无网络环境可执行测试和构建。
- [x] P2 整理新增 API、产物路径、降级语义和验收结果文档。
- [x] 验证：新增单元测试、Go vet、全量 Go 测试、PowerShell 解析和完整发布门禁全部通过。

## 约束

- 组件健康、产物索引、日志读取、日志轮转、辅助监听和 soak 分属独立模块，不合并为单一“诊断大文件”。
- 日志 API 仅允许固定别名，产物 API 仅允许 `/sdcard/xtest-nexus` 下经过格式校验的包/类型/会话。
- 辅助端口被占用不等于可信；必须完成协议与版本握手。
- soak 持久化采用逐行 JSON，禁止按运行时长无界累积内存样本。

## 实际落地

- 诊断拆分为 `componenthealth`、`artifactcatalog`、`logview`、`diagnosticsoak` 四个模块；HTTP 路由按组件拆为 `m44` 与 `m45`，未继续扩张原运行时诊断文件。
- 守护日志由 `logrotation` 独立管理：24 MiB 触发、3 份 gzip 备份、最长保留 72 小时，并支持进程运行期间轮转。
- Companion 使用 HTTP 服务身份和 Agent 精确版本握手；Monitor 使用 TCP `ping`、服务身份和协议版本握手。握手失败只记录组件降级，主端口保持可用。
- soak 时长限定 60 秒至 7 天、采样间隔 5 至 300 秒；每条样本立即写入 `soak.jsonl`，内存窗口固定为 120 条，异常重启遗留 `.active` 会转为 `.interrupted`。
- 保留 Companion 独立签名 APK 和现有六产物发布清单，没有采用旧项目的单二进制 APK 自举：当前部署回滚和签名校验链更完整，复制该设计会降低发布边界清晰度。

## 验收结果

- 全量 `go test ./agent/...`：通过。
- 全量竞态检测 `scripts/test-race.ps1`：通过。
- `go vet ./agent/...`：通过。
- Android/arm64 交叉编译：通过。
- PowerShell 脚本语法检查：通过。
- 完整发布门禁 `scripts/test-release.ps1`：通过，版本 `xtest-nexus-0.22.0-m5.9-compat`。
