# 第二轮运行时审计整改计划（2026-09-11）

## 实施原则

- 保持现有 Agent、Runner、Companion 和 Manager/Service 边界，不引入平行框架。
- 跨组件数据以 Agent 实际 JSON 合同为准，并为客户端消费方式增加回归约束。
- 区分进程运行、产物收尾和服务关闭三个生命周期，避免复用单一布尔状态。
- HTTP 长连接继续支持 WebSocket；普通请求通过独立正文上限和读取期限防御慢请求。

## Todo

- [x] R1 修复 Companion 保存用例列表对 `{"cases": [...]}` 响应的解析。
- [x] R2 修复 Companion 性能会话对 `last.cpuinfo`、`last.memoinfo` 等嵌套字段的解析。
- [x] R3 调整 Agent 关闭顺序：停止 HTTP 接入并等待普通在途请求后，再清理运行时资源。
- [x] R4 Runner 增加 `finalizing` 状态；子进程退出后立即清除 `running/PID/控制令牌`，产物处理完成后退出 finalizing。
- [x] R5 为普通 HTTP 请求统一增加结构化正文上限和读取期限，并保留 multipart/WebSocket 的独立预算。
- [x] R6 `wm density` 同时存在 Physical/Override 时选择最后一个有效值。
- [x] T1 增加 Companion–Agent JSON 合同回归测试。
- [x] T2 增加 Runner 产物收尾期间状态与 Stop 行为测试。
- [x] T3 增加 HTTP 超大正文和 density Override 回归测试。
- [x] V1 Go 全量测试及 `go vet` 通过。
- [x] V2 Runner 自测、Companion 编译和完整项目构建通过。
- [x] V3 已连接 Android 设备执行恢复力冒烟验证并确认清理完成。

## 验收标准

1. 保存用例与性能悬浮窗能够消费 Agent 当前响应，不再回落到解析错误或全零指标。
2. Agent 进入关闭状态后不再接收新的普通 HTTP 请求，且清理快照不会漏掉新启动资源。
3. Runner 进程退出后 `running=false`，产物生成期间 `finalizing=true`，Stop 不发生假超时。
4. 普通结构化请求不能无限读取或无限增长；WebSocket 与大文件上传仍按各自合同工作。
5. Density 与 Display 一致遵循 Android 最后一个 Override 输出。

## 实施结果

- Agent 全量测试与 `go vet ./agent/...` 通过；新增合同、Runner 状态、HTTP 边界和 density 用例均通过。
- Runner 自测、Companion 编译和根目录完整构建通过，ARM64/ARMv7 Agent 及 Android 工件已重新生成。
- Samsung SM-G9860（Android 13）完成 30 秒设备恢复力验证：48 次请求、0 失败、PID 稳定、goroutine 峰值 7、panic 0。
- 设备验证结束后 `xtest-nexus` 进程与 ADB forward 均已清理。
- 设备报告：`tests/reports/second-runtime-audit-remediation-device-20260911.json`。

## 已知验证环境提示

- JDK 21 在关闭 Android SDK `android.jar` 的 zip 文件系统时仍会输出既有的 `AccessDeniedException` 警告；构建脚本退出码为 0，APK 与完整构建均成功产出。
- Windows 当前 Go/CGO 环境未纳入 race detector 验收；竞态风险由锁边界审阅、状态机回归测试和设备恢复力冒烟覆盖，后续仍建议在 Linux CI 中补跑 `go test -race ./agent/...`。
