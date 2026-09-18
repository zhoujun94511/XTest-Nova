# M5.8C ATX 命令合同对齐验证

验证日期：2026-09-04。Agent：`xtest-nova-0.21.0-m5.8-contract77`。

全部 77 项的功能、输入输出、实现位置和限制统一记录在[完整 HTTP 合同](../../compliance/http-contract.md)；本文只保留本阶段验证证据。

## 对齐范围

- `GET /shell/background`：支持 `command`、`c` 查询参数并返回后台 PID。
- `POST /shell/background`：支持表单和 JSON 的 `command`、`c` 参数并返回后台 PID。
- `ANY /term`：普通请求返回内嵌终端页；WebSocket 支持 ATX 的输入和窗口尺寸二进制帧。
- 三项能力与 `/shell` 共用 `-legacy-unsafe-api`，默认均为 HTTP 403。

## 自动验证

- 全量 `go test ./agent/...` 通过。
- `go vet ./agent/...` 通过。
- Windows Go 官方竞态检测重复 3 次通过，无数据竞争报告。
- Nexus 合同检查：目标 77、实现 77、覆盖 77、缺失 0。
- Linux/Android arm64 交叉编译通过。

## Android 动态验证

| 设备                | 系统                  | 后台 GET/POST | PID 与落盘 | 终端页面     | PTY 输入/尺寸/回传 | 默认拒绝      |
|-------------------|---------------------|-------------|---------|----------|--------------|-----------|
| Xiaomi 2510DPC44G | Android 16 / SDK 36 | 通过          | 通过      | HTTP 200 | 通过           | 通过        |
| Samsung SM-G9860  | Android 13 / SDK 33 | 通过          | 通过      | HTTP 200 | 通过           | 单元与同构配置覆盖 |

验证使用 17912/18912/17890 隔离设备端口和独立主机转发，不覆盖 Nexus。测试 Agent、日志、输出文件和端口转发已定向清理，设备上未残留本轮进程。

## 结论

三个历史缺口已成为真实可执行实现，合同覆盖达到 77/77。默认部署的安全结论没有变化：未显式开启兼容开关时不提供任意命令执行能力。

## 发布制品

| 制品                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `D378D81937A053D9EB6116C9A3A230A3F0992843F1B95954FF7063C78C1F885E` |
| `xtest-nova-agent-armv7`   | `103E8ED8AEF4D1FB600B11F53A9C85A89529B57BB01726E9C19093D7EF75C3DE` |
| `xtest-nova-runner.jar`    | `394B1730EACD315352A6B2E7BA0A742E9D4E16F05850261C309EE35210FBEFCB` |
| `xtest-nova-companion.apk` | `0375884D3A09041FB6F6DAE6D516D470CDE9FFFC2001C1465EB8D71F77EB5B56` |

Companion 签名证书 SHA-256：`E49A7C6FB2EE0DBC4E6D3B6790D9DCF981D80F6B76628EB84755E7E1DA3A46DA`。签名口令和私钥未写入源码、文档或构建输出。
