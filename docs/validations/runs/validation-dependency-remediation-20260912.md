# 依赖收口与双真机验证（2026-09-12）

## 构建和发布门禁

- Gradle Wrapper 8.14.5 的 host/test release 构建成功；两个应用的 `releaseRuntimeClasspath` 均为 `No dependencies`。
- 签名 UIAutomator host/test APK 分别为 8,783 和 25,167 字节，APK 二进制中未发现 AndroidX UIAutomator 类名。
- Agent 全量 Go 单测、离线 vet、首提交门禁及 release 门禁通过。
- 发布清单仍只登记两个架构 Agent 二进制；许可证与 SBOM 作为两个 `dependencyDocuments` 单独登记，不伪装成运行组件。

## Android 16（SDK 36，83fc400c）

- 部署版本：`xtest-nova-0.24.0-m6.1-dependency-hardened`；四个内嵌运行组件均通过 bootstrap 校验。
- Nova Provider `/v1/hierarchy/raw` 返回 HTTP 200、12,458 字节，证明移除 AndroidX 后的平台 API 实现可用。
- `/v1/capabilities` 报告 `jsonRPCProxy=false`，未开启兼容开关时 `/jsonrpc/0` 返回 404。
- 设备端 scrcpy server 4.1 大小 733,706 字节，SHA-256 为 `deacb991ed2509715160ffdc7907e47b4160eb30d1566217e9047fd5b8850cae`。
- scrcpy 4.1 真机通道累计读取 20,006 字节 H.264 数据，控制通道发送 HOME 键并收到 `accepted`；兼容截图流读取 2,631,284 字节 PNG。
- 停止后 Agent PID 和 scrcpy 子进程均不存在，本次 ADB 转发已删除。

## Android 13（SDK 33，R5CN30EQKNM）

- 部署版本及四组件 bootstrap 校验同上；host/test 均为 `versionCode=2`、`versionName=0.2.0-platform-only`。
- Nova Provider `/v1/hierarchy/raw` 返回 HTTP 200、7,991 字节。
- 默认 `jsonRPCProxy=false`，`/jsonrpc/0` 返回 404。
- 反向验证显式加入 `--legacy-uiautomator` 后 `jsonRPCProxy=true`；由于设备未运行旧 9008 后端，代理明确返回 502，证明兼容能力只受该开关控制且不会伪成功。
- scrcpy 4.1 累计读取 6,557 字节 H.264 数据，控制通道返回 `accepted`；兼容截图流读取 1,175,722 字节 PNG，设备端 JAR 哈希与固定值一致。
- 停止后 Agent PID 不存在，本次 ADB 转发已删除。

## 结论

本轮处理项在 Android 13/16 上均通过完整运行时安装、平台 UiAutomation 控件树、scrcpy 视频/控制及默认关闭旧 9008 的验证。minitouch 按用户要求保持原状，不纳入本轮结论。
