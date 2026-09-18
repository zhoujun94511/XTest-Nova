# M2.3 验证记录

验证日期：2026-09-03。

## 实现范围

- 带任务 ID、状态、进度、取消和原子落盘的异步下载；
- 下载后安装 APK 的 `/packages` 与旧 `/install` 任务接口；
- 公网 HTTP(S) 白名单、DNS 解析后地址复核和重定向复核，阻止回环、私网、链路本地及非 HTTP(S) URL；
- `/session/{pkg}` 启动已解析的 Launcher Activity；
- `/webviews`、`/webviews/{pkg}` 与 `/wlan/ip`；
- `/screenshot/0` 截图别名；
- `/stop` 仅停止 Nova 自有服务；
- minitouch PUT/DELETE 进程生命周期。

`GET /minitouch` 所需 WebSocket 传输尚未实现，固定返回 501，因此没有计入合同覆盖。无认证的 `/shell/background` 仍按安全决策返回 403，也没有计入覆盖。

## 自动化结果

- `go test ./agent/...`：通过；
- `go vet ./agent/...`：通过；
- ARM64/ARMv7 Agent：构建通过；
- Runner Dex/JAR：构建通过；
- Companion APK：构建、对齐及维护证书签名通过；
- Nexus 合同覆盖：53/77 个“方法+路径”，剩余 24 个明确报告。

## Android 16 真机结果

测试设备：`83fc400c`，SDK 36，`arm64-v8a`。Nova 使用 17912/18912 独立测试端口。

| 检查项                             | 结果                               |
|---------------------------------|----------------------------------|
| Agent 版本                        | `xtest-nova-0.2.2-m2.3`          |
| WebView 枚举                      | 发现 2 个调试 socket，按包过滤正确           |
| WLAN IPv4                       | `172.16.1.211`                   |
| `/screenshot/0`                 | HTTP 200，PNG 17,208 字节           |
| 回环 URL 下载                       | HTTP 400，SSRF 防护生效               |
| 非法文件权限位                         | HTTP 400，仅接受 Unix 权限位            |
| 不存在的下载任务                        | HTTP 404                         |
| `/packages` 与 `/install` 回环 URL | HTTP 400                         |
| minitouch DELETE                | 幂等返回停止状态                         |
| minitouch PUT                   | 设备无二进制，HTTP 503 如实报告             |
| minitouch GET                   | HTTP 501，未伪装为已兼容                 |
| `/stop`                         | 返回 `Finished!`，随后健康检查仍为 HTTP 200 |

为避免改变共享设备前台状态，`/session/{pkg}` 只进行了自动化命令链测试，没有在真机启动或强制停止应用。没有调用远程 APK 安装，也没有安装 Nova Companion；现有 Nexus Popup 未被替换。

验证后已停止 Nova 测试进程、删除测试文件并移除临时端口转发。设备上没有残留 Nova M2.3 进程。

## 产物

| 产物                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `A156D7E908755751A9D358F997D8D8B5E8F61C949D142F7BDEC797175AFAB065` |
| `xtest-nova-agent-armv7`   | `E0A96C825496E2C492226130AD37CC014E3D2ECAF01B7C05C9DB1B587B1744DD` |
| `xtest-nova-runner.jar`    | `8CF0F8449A779BD1AE9A248E2E9BF3184DB7BD535342AC4E5C51D87E7D7F0397` |
| `xtest-nova-companion.apk` | `29845AA9815C22DA1E9ED890D59D4966850C7C2562C353C1DDE50AFF5FD78FF9` |

Companion 包名为 `com.openatx.xtest.popup`，`versionCode=20003`，`versionName=2.0.2-m2.3`。APK Signature Scheme v3 验证通过，签名证书 SHA-256 与 Nexus 维护证书一致：

```text
e49a7c6fb2ee0dbc4e6d3b6790d9dcf981d80f6b76628eb84755e7e1da3a46da
```
