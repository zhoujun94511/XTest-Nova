# M2.6 验证记录

验证日期：2026-09-03。

## 实现范围

- `/touchreader` WebSocket、Android `getevent` 设备探测、Protocol-B 多点状态机和归一化触摸事件；
- `/jsonrpc/0` 固定代理至 `127.0.0.1:9008`，请求限制 4 MiB、响应限制 16 MiB；
- 内嵌 `/` 控制台、`/static/js/*`、`/static/css/*`、`/static/media/*`、`/assets/*` 和 SPA 回退；
- 控制台不引用 CDN，动态设备内容使用 `textContent` 构建，并启用 Content Security Policy；
- TouchReader WebSocket 关闭时终止它自己启动的 `getevent` 进程。

scrcpy 没有用低帧率截图接口伪装完成。下一阶段将使用官方 scrcpy 4.0 server、固定哈希和独立控制/视频通道实现。无认证终端与后台 Shell 仍保持禁用。

## 自动化结果

- `go test ./agent/...`：通过；
- `go vet ./agent/...`：通过；
- ARM64/ARMv7 Agent：构建通过；
- Runner Dex/JAR：构建通过；
- Companion APK：构建、对齐及维护证书签名通过；
- Nexus 合同覆盖：73/77 个“方法+路径”，剩余 4 个明确报告。

## Android 16 真机结果

测试设备：`83fc400c`，SDK 36，`arm64-v8a`。Nova 使用 17912/18912/17890 独立测试端口。

| 检查项            | 结果                                         |
|----------------|--------------------------------------------|
| Agent 版本       | `xtest-nova-0.4.0-m2.6`                    |
| 控制台 `/`        | HTTP 200，1,074 字节                          |
| JavaScript     | HTTP 200，1,756 字节                          |
| SPA 路由         | `/console/device` 返回控制台 HTTP 200           |
| 不存在的静态文件       | HTTP 404                                   |
| 浏览器安全头         | CSP 与 `X-Content-Type-Options: nosniff` 生效 |
| JSON-RPC       | 设备无 9008 服务，HTTP 502 如实报告                  |
| TouchReader    | WebSocket 保持连接，期间存在一个自有 `getevent` 进程      |
| TouchReader 断开 | `getevent` 进程在连接关闭后消失                      |

验证后已停止 Nova 测试进程并移除三个临时端口转发；没有注入触摸，没有安装 Nova Companion，也没有修改现有 Nexus 服务。

## 产物

| 产物                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `6D15E3504D41DC196DFA3EF2043410AAE686748D2CF46CAA2AE29A0C1A6D61D6` |
| `xtest-nova-agent-armv7`   | `14186B53A89BBC96E5C28C7D90C7E458ECBA249B0AE82D60C6D7625B0B9C31B7` |
| `xtest-nova-runner.jar`    | `8AB9EC2279750B2E71E617121109E56897029099C0ACD308A07CB78B6898AB82` |
| `xtest-nova-companion.apk` | `B461D1DCFEDF2A898D3C806903DB44E4CF103CDE5BD75FAD1EB9EC389210ED19` |

Companion 包名为 `com.openatx.xtest.popup`，`versionCode=20006`，`versionName=2.0.5-m2.6`。APK Signature Scheme v3 验证通过，签名证书 SHA-256 为：

```text
e49a7c6fb2ee0dbc4e6d3b6790d9dcf981d80f6b76628eb84755e7e1da3a46da
```
