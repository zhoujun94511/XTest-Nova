# M5.4 验证记录

验证日期：2026-09-03

版本：

- Agent：`xtest-nova-0.16.0-m5.4`
- Companion：`3.3.0-m5.4`（versionCode 30300）
- 验证夹具：`com.xtest.nova.fixture` 1.0（versionCode 1）

## 验证范围与安全边界

`tests/e2e/validate-delivery-media.ps1` 只操作独立验证夹具、固定路径 `/data/local/tmp/xtest-nova-public-download.html` 和本次新建的录屏会话目录。脚本发现同名夹具或已有 Nova Agent 时拒绝覆盖，并在成功或异常退出时卸载夹具、删除下载文件、停止并删除本次 Agent 与日志、移除本次端口转发。

公开下载使用 `https://example.com/` 验证 TLS、域名解析、任务状态、写入字节数与原子落盘，不安装下载内容。下载客户端继续拒绝回环、私网、链路本地、未指定地址和非 HTTP(S) 协议，TLS 最低版本为 1.2，未关闭证书校验。

## 兼容修复

- Android shell 的默认 Go DNS 解析器指向不可用的 `[::1]:53`，现保留系统解析优先，并在失败时依次使用受限的公共 DNS 回退；连接前仍逐个拒绝私网或本地解析结果。
- 通用 Linux 交叉编译产物不会自动发现 Android CA，现加载 Conscrypt APEX 与系统 CA 目录；证书链仍按系统信任根严格验证。
- 验证器按 Agent 实际任务状态 `success`、`failure`、`canceled` 收敛，不再依赖错误的状态名称。

## 双机结果

Android 16（Xiaomi 2510DPC44G，SDK 36，ARM64）：

- multipart 安装并卸载 8,601 字节签名夹具；
- 公开 HTTPS 下载 559 字节，远端文件与任务计数一致；
- 录制约 5 秒 MP4，大小 132,976 字节；
- 删除本次视频及会话目录。

Android 13（Samsung SM-G9860，SDK 33，ARM64）：

- multipart 安装并卸载 8,601 字节签名夹具；
- 公开 HTTPS 下载 559 字节，远端文件与任务计数一致；
- 录制约 5 秒 MP4，大小 53,995 字节；
- 删除本次视频及会话目录。

两台设备的全部步骤通过，验证报告 schema 为 `xtest-delivery-media/v1`。

## 构建与产物

- `go test ./agent/...`、`go vet ./agent/...`、PowerShell 解析和 Nexus 合同门禁通过；
- Nexus 历史合同覆盖 74/77，3 个无认证危险入口继续显式禁用；
- Companion 使用恢复证书签名，证书 SHA-256 为 `E49A7C6FB2EE0DBC4E6D3B6790D9DCF981D80F6B76628EB84755E7E1DA3A46DA`；
- 验证夹具不进入生产发布清单。

| 产物                          | SHA-256                                                            |
|-----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`    | `CD0CB044FFD5BE3F4C9746FBD06526D8CE5592F73F3F1CC62132BBE4319AD595` |
| `xtest-nova-agent-armv7`    | `114A4DE1CA4FF6295C77BD8C2A6B7ABA2C98771D6F379970126C327819BD6913` |
| `xtest-nova-runner.jar`     | `0C7736994C9106799ED4CA65239B7B59E65D70C57927744A7D86710B22BC4834` |
| `xtest-nova-companion.apk`  | `3CCFDFF3AF4C71C29E3A55E1B246D6F182D6E7512EC2FF0B5C7CCF26400ACAFA` |
| `xtest-nova-validation.apk` | `9D53BAC2DF8A9C835C09B26E8DAFBB1C7895F0E9C820047240FF0A1D3FD7DEED` |

## 下一阶段

M5.5 在同一隔离夹具上验证 Monkey/Runner 的启动、运行、停止、停止后无事件和日志证据；不会把随机事件发送到设备现有业务应用。
