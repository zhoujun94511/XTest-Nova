# M2.5 验证记录

验证日期：2026-09-03。

## 实现范围

- `/appevent/info` 有界消息发布与 `/appeventmonitor` WebSocket 实时订阅；
- 独立 7890 行式 JSON 性能服务及 `PUT /monitor` WebSocket 双向桥；
- `/screenrecord` 专属进程启停与每次独立会话目录；
- `/minicap`、`/minicap/broadcast` 使用系统截图提供 PNG 二进制帧和真实方向信息；
- Agent 退出及 `/stop` 时收敛自有录屏、Runner、UiAutomator、minitouch、AutoPopup 和下载任务。

Nova 不自动下载旧 minicap/minitouch 二进制。minicap 兼容流重视 Android 16 可用性和生命周期安全，目前为每秒一帧，不宣称达到原生 minicap 帧率。Monitor 暂未加入可靠 FPS 采样，响应以 `fps:null` 明确表示，而不是填入伪造值。

## 自动化结果

- `go test ./agent/...`：通过；
- `go vet ./agent/...`：通过；
- ARM64/ARMv7 Agent：构建通过；
- Runner Dex/JAR：构建通过；
- Companion APK：构建、对齐及维护证书签名通过；
- Nexus 合同覆盖：65/77 个“方法+路径”，剩余 12 个明确报告。

## Android 16 真机结果

测试设备：`83fc400c`，SDK 36，`arm64-v8a`。Nova 使用 17912/18912/17890 独立测试端口，避免占用 Nexus 的 7912/8912/7890。

| 检查项            | 结果                                   |
|----------------|--------------------------------------|
| Agent 版本       | `xtest-nova-0.3.1-m2.5`              |
| App Event      | 发布 JSON 后 WebSocket 收到完全一致文本         |
| minicap 控制帧    | `rotation 0`，来自设备真实方向                |
| minicap 图像帧    | Binary，375,301 字节，PNG 文件头 `89504E47` |
| 17890 Monitor  | 返回 SystemUI CPU、内存和网络数据，`fps:null`   |
| `PUT /monitor` | HTTP 101 WebSocket 升级成功              |
| 录屏启动           | HTTP 200，返回 `screenrecord started`   |
| 录屏停止           | 生成 154,254 字节 MP4                    |

本次测试录屏文件及其会话目录已明确删除。验证后已停止 Nova 测试进程并移除三个临时端口转发；Nova Companion 未安装，现有 Nexus 服务和 Popup 未被替换。

## 产物

| 产物                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `4AC8143CBDD900A66C099118E207DCD3F24A926FBFB57A03D2407C9B2AD64E88` |
| `xtest-nova-agent-armv7`   | `419CF9865B7FB5F573472265E5BEECA24768FCC5B0B25EB3C2764A023265EFD6` |
| `xtest-nova-runner.jar`    | `65F864A5C985013A855FFC9056A01FB1762BB3F375035B6501ADFA03B3BD5E62` |
| `xtest-nova-companion.apk` | `22E411244D1EE1DE8F8E4995DEC6B888317270C7CCD2C08118FE78853A1861F5` |

Companion 包名为 `com.openatx.xtest.popup`，`versionCode=20005`，`versionName=2.0.4-m2.5`。APK Signature Scheme v3 验证通过，签名证书 SHA-256 为：

```text
e49a7c6fb2ee0dbc4e6d3b6790d9dcf981d80f6b76628eb84755e7e1da3a46da
```
