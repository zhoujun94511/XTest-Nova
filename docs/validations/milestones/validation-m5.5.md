# M5.5 验证记录

验证日期：2026-09-03

版本：

- Agent：`xtest-nova-0.17.0-m5.5`
- Companion：`3.4.0-m5.5`（versionCode 30400）
- 验证夹具：`com.xtest.nova.fixture` 1.0（versionCode 1）

## 安全边界

`tests/e2e/validate-runner.ps1` 发现已有 Nova Agent 或同名夹具时拒绝覆盖。它只安装无权限的独立夹具，并只推送本轮 Agent、Runner、日志与停止标记。夹具拦截返回键，使随机返回事件不能退出隔离 Activity；Runner 不生成 Home 或应用切换事件。

无论成功或失败，验证器都会停止 Runner、卸载夹具、恢复原前台、停止并删除本轮 Agent/Runner、删除日志与停止标记，并移除端口转发。

## 双机结果

Android 16（Xiaomi 2510DPC44G，SDK 36，ARM64）：

- Runner 启动 PID 7717；
- 重复 requestId 保持同一进程；
- 不同 requestId 的并发启动返回 HTTP 409；
- 主动停止后状态收敛；
- 结构化日志记录 8 个事件，日志 206 字节，停止后一秒不再增长。

Android 13（Samsung SM-G9860，SDK 33，ARM64）：

- Runner 启动 PID 20746；
- 重复 requestId 保持同一进程；
- 不同 requestId 的并发启动返回 HTTP 409；
- 主动停止后状态收敛；
- 结构化日志记录 7 个事件，日志 206 字节，停止后一秒不再增长。

全部步骤通过，报告 schema 为 `xtest-runner-validation/v1`。

## 构建与产物

| 产物                          | SHA-256                                                            |
|-----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`    | `2D9204D1C5D20E5D04473284393711059C1302710C3595CCEEF7AE7A85BD71EA` |
| `xtest-nova-agent-armv7`    | `9591F0366E63CDE856EDBEDD6C8C1E848AB136477232D6056A8AA4F5B71FEB78` |
| `xtest-nova-runner.jar`     | `FF64C50BDC5D50819C73CA234E21F45909D808512E9AF8329854879A91532885` |
| `xtest-nova-companion.apk`  | `D273B300585E70FA46DA302B4617E13E5BEF3179F90E3A085DFED80D6A1A9DDA` |
| `xtest-nova-validation.apk` | `1691D21F04D69DE5F199A3EA43DF4A21AD3BE5D9C0818906CBE82E491DFD5196` |

完整发布门禁通过；验证夹具与报告不进入生产发布清单。
