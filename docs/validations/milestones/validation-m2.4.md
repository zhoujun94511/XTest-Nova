# M2.4 验证记录

验证日期：2026-09-03。

## 实现范围

- 从设备已安装 APK 的资源表提取应用图标，并兼容输出 JPEG；
- `/newCommandTimeout` 动态设置 UiAutomator 空闲停止时间，范围限制为 1 秒至 24 小时；
- `/popupBoxAssistant` 配置驱动的层级扫描、上下文匹配、点击、输入及幂等启停；
- `GET /minitouch` WebSocket：服务启动、Unix socket 桥接、协议 banner 解析、触控参数校验、单连接保护和断线复位；
- `/stop` 同时收敛 AutoPopup 生命周期。

WebSocket 使用 `github.com/coder/websocket v1.8.15`；APK 资源解析使用与 Nexus 行为基线一致的 `github.com/shogo82148/androidbinary v1.0.3`。依赖版本由 Go module 固定。

## 自动化结果

- `go test ./agent/...`：通过；
- `go vet ./agent/...`：通过；
- ARM64/ARMv7 Agent：构建通过；
- Runner Dex/JAR：构建通过；
- Companion APK：构建、对齐及维护证书签名通过；
- Nexus 合同覆盖：58/77 个“方法+路径”，剩余 19 个明确报告。

## Android 16 真机结果

测试设备：`83fc400c`，SDK 36，`arm64-v8a`。Nova 使用 17912/18912 独立测试端口。

| 检查项                 | 结果                                                          |
|---------------------|-------------------------------------------------------------|
| Agent 版本            | `xtest-nova-0.3.0-m2.4`                                     |
| 命令空闲超时              | JSON 整数 `300` 更新为 5 分钟                                      |
| 应用图标                | `com.mi.globalbrowser` 返回 HTTP 200、1,979 字节、JPEG `FFD8` 文件头 |
| Popup 初始状态          | `running=false`、`stopping=false`                            |
| Popup 启停            | 空配置下 POST/DELETE 均成功，未触发任何点击或输入                             |
| minitouch WebSocket | 握手成功，并通过文本帧如实返回设备缺少二进制                                      |

设备上没有 `/data/local/tmp/minitouch`，因此没有伪造完整触控成功，也没有下载或安装旧二进制。协议 banner 和触控命令转换由单元测试覆盖。

验证后已停止 Nova 测试进程、删除测试文件并移除临时端口转发。Nova Companion 未安装，现有 Nexus Popup 未被替换。

## 产物

| 产物                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `69C9649C8A65056A6CFB871FC43129713F2DC150CE4501CFDD706AC3D9F0F8AC` |
| `xtest-nova-agent-armv7`   | `CBC5BC2AF1AE847DB80CA7006A421A7643C2F88D40CC91CC5F55458F87215429` |
| `xtest-nova-runner.jar`    | `F426C14790C1B3CB03DB07374826C1DED10C5B17B822CA942B80A4101631FCFC` |
| `xtest-nova-companion.apk` | `E618683871B9AB8FFF347C68C364F92F4FC71D6B70A9D3FF3E249945EAA9995C` |

Companion 包名为 `com.openatx.xtest.popup`，`versionCode=20004`，`versionName=2.0.3-m2.4`。APK Signature Scheme v3 验证通过，签名证书 SHA-256 为：

```text
e49a7c6fb2ee0dbc4e6d3b6790d9dcf981d80f6b76628eb84755e7e1da3a46da
```
