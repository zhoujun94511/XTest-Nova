# XTest 定制 Monkey 字节码对齐审计

审计日期：2026-09-04。

## 事实源

本审计没有只引用 Nexus 状态结论，而是交叉读取外部 XTest-Nexus 参考仓中的
以下恢复材料。这些路径和文件不随本仓库交付，不是本地文档链接：

- 原始 DEX 的 SHA-256 为 `119A98D05C8116CBAC4557774D3C55E4B2E62B2A4FACE1093232228517F3E307`；
- `XTest-Nexus/monkey-engine-recovered/decompiled-reference/jadx-1.5.3/` 中的 67 个核心 Java 参考；
- `XTest-Nexus/monkey-engine-recovered/engine-java-reconstruction/`；
- `XTest-Nexus/popup-recovered/smali/` 权威反汇编；
- `XTest-Nexus/monkey-engine-recovered/CLASS_MAP.md` 和
  `XTest-Nexus/monkey-engine-recovered/SOURCE_COMPLETENESS_AUDIT.md` 的恢复边界。

JADX 输出仍有 17 个失败方法和大量结构警告，因此 Java 只能帮助理解；发生冲突时以 Smali 为准。Nova 不复制这些混淆实现，只依据可验证行为进行 clean-room 重写。

## 已由字节码确认的行为

| 行为           | 字节码证据                                                                                                                     | Nova 当前状态                              |
|--------------|---------------------------------------------------------------------------------------------------------------------------|----------------------------------------|
| 引擎模式         | `v0.d` 解析 `--uiautomatorapi`、`--uiautomatordfs`、`--uiautomatormix`、`--uiautomatortroy`                                    | Nova 采用可维护节点引擎，不复用原隐藏 API 实现           |
| 页面树          | `u2.a`、`x0.a` 从 AccessibilityNodeInfo 采集 class、text、resource-id、content-desc、clickable、long-clickable、scrollable、bounds 等 | 已采集主要字段                                |
| 稳定分类         | `u2.b` 默认 `max.widget.classification=activity,resource-id,class,clickable,enabled,checkable`，并支持树结构等可选字段                  | 已向默认字段收敛，并使用 Activity、树结构和稳定字段生成 SHA-256 指纹 |
| DFS 状态图      | `s2.e0` 维护页面状态、动作到状态集合，并在未访问转换中选择动作；耗尽后搜索到未知状态的路径                                                                         | 已实现场景动作状态、转移边和到未知状态的已知路径回放          |
| DFS 动作       | `s2.e0.b` 为 START、BACK、CLICK、LONG_CLICK；可滚动节点另外生成滚动输入                                                                     | 已实现 CLICK、LONG_CLICK、滚动、BACK 及图路径回放    |
| Activity 分母  | `v0.d` 使用 `PackageManager.getPackageInfo(package, GET_ACTIVITIES).activities`                                             | 已改为直接解析目标 APK 二进制 Manifest；失败时明确降级     |
| Activity 覆盖率 | `v0.e`、`s2.g0`、`s2.i0` 记录 tested activities 并输出覆盖率                                                                        | Nova 输出 JSON/文本覆盖报告，并标记分母来源            |
| 底层事件         | `s2.d` 定义 0–12：Key、Touch、Trackball、Rotation、Activity、Flip、Throttle、Noop、Permission、Command、Power、FrameRate、AppFrameRate   | 这是底层注入事件类型，不应误写成“13 类页面探索策略”           |

## Compose 结论纠偏

在保留的 67 个 Monkey 核心 Java、simple 关键类和权威 Smali 中搜索 `compose`、`androidx.compose`，没有找到专用类、参数、判断或优先级。原引擎读取通用 Accessibility 节点，因此部分正确暴露 semantics 的 Compose 界面可能可用，但这不等于存在“Compose 优先”实现。

因此维护文档必须使用以下口径：

> XTest 具有 Accessibility 通用节点探索能力；Compose 可用性取决于目标应用暴露的语义树。当前没有证据支持“原 XTest 对 Compose 节点专门加权或优先”。

## 对 Nova 的直接整改

1. Activity 分母从 `dumpsys package` 正则推测改成 APK 二进制 Manifest 解析，覆盖报告升级为 v4，并保留明确降级口径。
2. 节点标识优先使用原默认分类字段；仅在信息不足时使用标签或位置消歧，避免动态文本直接制造大量伪新控件。
3. 增加 LONG_CLICK 动作，不再把原 DFS 动作集合简化成 CLICK/BACK。
4. R4.2 已关闭：Nova 已实现可维护的场景动作图、转换集合和到未知状态的路径搜索，并由 JVM 自测、Go 回归及 Android 13/15 真机证据覆盖。
5. Compose 作为设备/应用资格项单独验证，不把 Nova 自定义优先规则冒充 XTest 行为。
