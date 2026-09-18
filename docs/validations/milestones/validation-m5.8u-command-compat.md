# M5.8U 命令与使用兼容验证

验证日期：2026-09-04。Agent：`xtest-nexus-0.22.0-m5.9-compat`。

## 实现范围

- `server -d`、`server -d --stop`；
- `popup start`、`popup status`、`popup uninstall`；
- `version`；
- `/shell` 查询、表单、JSON 与旧响应三字段；
- `/stop` 响应后优雅退出；
- `/info` 关键历史字段及整数 SDK；
- `xtest-nexus` 构建、部署、回滚、发布清单及设备验证脚本。

## 自动门禁

- `go test ./...`：通过；
- `go vet ./...`：通过；
- Go 官方竞态检测：通过；
- Nexus 方法路径合同：77/77，文档差异 0；
- 发布门禁：通过；
- PowerShell 构建、部署和设备验证脚本语法检查：通过。

## 真机结果

Android 16（SDK 36）和 Android 13（SDK 33）均通过 18 项只读资格检查，并实际完成
`version → server -d → PID 检查 → server -d --stop`。

在原本未安装 Companion 的 Android 13 设备上额外完成：

`popup status(false) → popup start → status(true) → popup uninstall → status(false)`。

Android 13 还验证了表单 `POST /shell` 的 `c` 和 `timeout`、扩展 `/info`，
以及 `GET /stop` 返回 `Finished!` 后进程退出。测试结束后临时二进制、APK、PID、
日志、端口转发和临时安装均已清理。

## 发布制品

| 制品                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nexus-arm64`        | `41E45FB3551F1CA2C25C9F2DA614DAD7D92EC8534F941F286E6581D3C4B31432` |
| `xtest-nexus-armv7`        | `09A9CA242ED716E55BDA8A1EDCD523DDD5E9232592080B3D5F7C1B3B6679733B` |
| `xtest-nova-companion.apk` | `0375884D3A09041FB6F6DAE6D516D470CDE9FFFC2001C1465EB8D71F77EB5B56` |

ARMv7 已交叉编译并通过静态门禁，但当前没有 ARMv7 真机，因此维持既有资格限制。
