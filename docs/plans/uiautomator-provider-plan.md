# Nova 自有 UiAutomator Provider 实施计划

日期：2026-09-07  
决策：长期必要性高，短期发布阻断程度中等；分期实施，不一次性替换全部执行底座。

## 范围与边界

本计划只收口“控件树如何产生”。普通点击、滑动和按键继续使用现有 `input` 链，多指和精确轨迹继续使用 scrcpy。首期不合并 Runner、策略引擎和触控注入，以便把采集回归与输入回归分开定位。

自有 Provider 仍依赖 Android Accessibility 节点。SurfaceView、OpenGL/Vulkan、Canvas 自绘和未暴露语义的 Compose/WebView 不会因为更换 Provider 自动获得节点；这些场景继续使用截图、视觉识别和坐标触控，并明确返回能力不足。

2026-09-12 起已移除 AndroidX UIAutomator 运行依赖。Nova Provider 只使用 Android 平台 `Instrumentation`、`UiAutomation`、`AccessibilityNodeInfo` 与 `DisplayManager` API，两个 APK 的 `releaseRuntimeClasspath` 均为空，以缩小供应链和运行体积。

## 目标架构

```text
Nova Agent
  └─ HierarchyProvider
       ├─ nova-provider（资格通过后主用）
       ├─ system-dump（第一回退）
       └─ legacy-9008（仅 --legacy-uiautomator 显式启用）

Nova UiAutomator 工程
  ├─ xtest-nova-uiautomator-host.apk
  └─ xtest-nova-uiautomator-test.apk
       └─ loopback HTTP + 平台 UiAutomation
```

外部 `/dump/hierarchy`、`/dump/hierarchyWithScreenshot`、`/v1/hierarchy/raw`、`/jsonrpc/0` 合同保持不变。首期 Provider 输出兼容 XML，统一 Snapshot 与 Java/Go 页面模型属于第二期。

## Todo 与交付状态

| ID | 工作项 | 当前状态 | 完成证据/剩余工作 |
|---|---|---|---|
| U1 | 现状观测基线 | 进行中 | 指标已接入运行诊断；Android 12/15/16 已完成固定页、WebView/锁屏专项或热态采样；Android 13/15/16 又完成 Foloy 多页面、完整首启和特殊系统场景基线。更多获准应用与完整设备基线待补。 |
| U2 | 独立 host/test 工程 | 已完成（源码/构建） | `uiautomator/` 使用独立 Gradle Wrapper 8.14.5 和 AGP 8.11.2，生成两个无第三方运行依赖 APK；依赖锁和 SHA-256 校验元数据已生成。 |
| U3 | 持续层级服务 | 已完成（源码/构建） | 已提供 `/health`、`/v1/hierarchy`、`/v1/windows`、`/v1/wait-stable`、`/v1/diagnostics`；真机资格待 U9。 |
| U4 | 安全与生命周期 | 已完成（源码/单测） | 已有回环监听、每次启动 256-bit 随机令牌、请求头/响应大小限制、串行采集、请求超时、空闲退出、Agent 所有权启停、异常降级和退出清理；真机资格待 U9。 |
| U5 | Provider 链 | 已完成（源码/单测） | `HierarchyProvider`、Snapshot、`system/shadow/nova` 显式配置和 Nova 生命周期已接入；默认只有 `system-dump`，旧 9008 必须通过兼容开关显式加入。 |
| U6 | 兼容合同不变 | 已完成（静态/单测） | 现有 handler 签名和返回 XML 保持不变，automation/httpapi 测试通过。 |
| U7 | 影子对照 | 功能已验证/场景待扩展 | 已使用串行隔离模型规避 `UiAutomation` 竞争；结构/完整指纹、17 类属性、稳定身份匹配及按包差异均已落地。Plant Scope 固定页及 Android 13/15/16 Foloy 多页面、Compose 首启、权限和外部系统页通过；已验证页面无业务语义节点退化，更多获准应用样本待扩展。 |
| U8 | 资格后切主 | 未开始 | 只有达到门槛后才改为 `nova → system → legacy`；输入链不变。 |
| U9 | 设备矩阵与长稳 | 进行中 | Android 12/15/16 已通过隔离影子、主模式和崩溃恢复；Pixel/Android 16 同一 WebView 各完成 10,000/10,000 次、零失败。Android 15 Foloy 真实分屏完成 system/Nova 影子及 50 次直连，P95 318.99 ms、节点稳定；Android 13/16 真实分屏和两小时长稳待补。证据见 [`validation-uiautomator-provider-20260907.md`](../validations/runs/validation-uiautomator-provider-20260907.md)。 |
| U10 | 文档与发布门禁 | 进行中 | 本计划、架构、兼容矩阵和 NOTICE 已更新；W-005/W-009 只能在真机证据完成后关闭。 |

## 分期与进入条件

### 第一期：可影子验证

交付 U1–U7。默认仍使用 system dump。Agent 只启动自己安装、自己生成令牌的 Instrumentation；不得依据包名停止外部实例。服务不可用时必须自动降级，不能让层级接口伪成功。

### 第二期：有条件切主

满足以下全部条件后执行 U8：

- API 33+ 热态层级采集 P95 不高于 500 ms；API 32 及以下的大型虚拟节点树允许 P95 不高于 1 秒，但必须零超时且相对 system dump 的 P95 至少降低 50%；
- 相对 system dump 的中位耗时至少降低 50%；
- 10,000 次采集无死锁、无进程泄漏；
- 两小时遍历无持续内存增长；
- 服务崩溃后能降级并恢复；
- Android 12/13/15/16 的可访问节点不比 system dump 退化；
- 现有 Monkey、Popup、录制回放和 77 项 HTTP 合同全部通过。

门槛是发布目标，不是当前测量结果。未取得证据前，W-005/W-009 保持开放。

## 参考边界

- `android-uiautomator-server-jar`：只参考服务启动和 UiAutomation 生命周期；
- `uiautomator2`：只参考健康检查、恢复和层级接口语义；
- Appium UiAutomator2 Server：只参考会话、节点失效与异常分类；
- DroidBot：只参考状态图，不引入运行时依赖；
- scrcpy：继续负责画面与复杂触控，当前锁定并校验官方 4.1 server；
- Poco/Airtest/STF：首期不引入，避免扩大运行链路和回归面。
