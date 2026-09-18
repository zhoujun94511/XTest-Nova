# M2.7 验证记录

验证日期：2026-09-03。

## 实现范围

- `/scrcpy/{type}/{definition}` WebSocket 兼容入口；
- 官方 Genymobile scrcpy server 4.0 作为未修改资源内嵌，Agent 启动前固定校验 SHA-256；
- `screen` 通道启动独立 `app_process` 会话并直接转发原始 H.264 二进制流；
- `control` 通道实现触摸、按键、文本、旋转、剪贴板与滚动消息的 scrcpy 4.0 帧编码；
- 优先使用浏览器上报的已解码视频尺寸，旧客户端回退至设备 `wm size`，避免缩放后的位置事件被 server 拒绝；
- 画面断开、进程退出或 Agent 停止时关闭视频/控制 Socket 并回收自有 server 进程；
- JAR 使用 `/data/local/tmp/xtest-nova-scrcpy-server-v4.0.jar`，不覆盖 Nexus 或其他 scrcpy 客户端的载荷。

官方依赖来源为 [Genymobile/scrcpy v4.0](https://github.com/Genymobile/scrcpy/releases/tag/v4.0)，开发协议参考其 [v4.0 开发文档](https://github.com/Genymobile/scrcpy/blob/v4.0/doc/develop.md)。JAR 大小为 732,226 字节，SHA-256 为：

```text
84924bd564a1eb6089c872c7521f968058977f91f5ff02514a8c74aff3210f3a
```

上游采用 Apache License 2.0；完整许可证和第三方说明随 JAR 保存在 `agent/internal/scrcpy`。

## 自动化结果

- `go test ./agent/...`：通过；
- `go vet ./agent/...`：通过；
- scrcpy 载荷哈希、清晰度参数、启动参数、触摸/按键/剪贴板帧和边界输入测试：通过；
- ARM64/ARMv7 Agent：构建通过；
- Runner Dex/JAR：构建通过；
- Companion APK：构建、对齐及维护证书签名通过；
- Nexus 合同覆盖：74/77 个“方法+路径”，剩余 `/shell/background` 的 GET/POST 和 `/term` 明确保持禁用。

## Android 16 真机结果

测试设备：`83fc400c`，SDK 36，`arm64-v8a`。Nova 使用 17912/18912/17890 独立测试端口。

| 检查项          | 结果                                    |
|--------------|---------------------------------------|
| Agent 版本     | `xtest-nova-0.5.0-m2.7`               |
| 能力声明         | `scrcpy=true`，不再列入 planned            |
| 画面 WebSocket | `screen/low` 成功连接并收到 Binary H.264 数据  |
| 控制 WebSocket | 与画面通道同时保持连接；无副作用的非法事件返回结构化错误          |
| JAR 缓存       | 设备侧 SHA-256 与内嵌官方载荷一致                 |
| 断线回收         | 浏览器连接关闭后三秒内不再存在 Nova scrcpy server 进程 |
| 隔离性          | 测试前已存在的其他 scrcpy 4.0 会话未被停止或覆盖        |

验证没有注入触摸、按键或文本，没有安装 Nova Companion。验证后已停止 Nova 测试 Agent，删除 Nova 专属 Agent、日志和 scrcpy JAR，并移除三个临时端口转发。

## 产物

| 产物                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `0971F60E8CF15C7B2DDA9FA4A7635FAFC5C64AE540556F3B2ABA82881904FC8B` |
| `xtest-nova-agent-armv7`   | `C1870EDDDDCD001E8DE9C712C12BC9C5459979E868B83393CDD43963032A6D1A` |
| `xtest-nova-runner.jar`    | `24CCF9020779ED5A66C74145110F29927E06D11AA7AB2FF166DFBC8A498D9B96` |
| `xtest-nova-companion.apk` | `3042DE370B3FB03BD40DD597ABE5767449634341784C1FB401FEF0137995ADB2` |

Companion 包名为 `com.openatx.xtest.popup`，`versionCode=20007`，`versionName=2.0.6-m2.7`。APK Signature Scheme v3 验证通过，签名证书 SHA-256 为：

```text
e49a7c6fb2ee0dbc4e6d3b6790d9dcf981d80f6b76628eb84755e7e1da3a46da
```
