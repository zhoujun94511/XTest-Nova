# 第三方依赖收口 Todo（2026-09-12）

范围依据本轮全仓依赖审计；用户明确要求第 2 项 minitouch 暂不处理。

- [x] D1 移除 AndroidX UIAutomator 及其传递运行依赖，改用 Android 平台 API。
- [x] D2 将 UIAutomator Gradle Wrapper 固定到 8.14.5 并重新生成依赖锁与 SHA-256 校验元数据。
- [ ] D3 minitouch 能力声明和运行时断链修复——本轮明确延期，不改代码、载荷或声明。
- [x] D4 将旧 9008/旧 UiAutomator 启动、探测及 JSON-RPC 暴露统一放到 `--legacy-uiautomator` 显式开关后。
- [x] D5 将官方未修改 scrcpy server 升级到 4.1，并固定版本、设备路径和 SHA-256。
- [x] D6 增加第三方清单、SBOM、完整许可证发布副本及一致性门禁。
- [x] D7 完成定向单测、离线依赖检查和 Gradle release 构建。
- [x] D8 完成自包含签名发布构建、全量发布门禁及 Android 13/16 真机验证。

验收规则：D8 只有在两个设备均验证运行时四组件、Nova 控件树、scrcpy 4.1 视频通道、默认 9008 关闭及停止清理后才能勾选。

验收证据见 [`validation-dependency-remediation-20260912.md`](../validations/runs/validation-dependency-remediation-20260912.md)。

