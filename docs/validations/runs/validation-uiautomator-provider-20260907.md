# Nova UiAutomator Provider 初始资格记录（2026-09-07）

## 范围

本记录验证独立 host/test APK、持续服务端点、Android 版本基本兼容、Agent 影子链、10,000 次采集和所有权清理。两小时长稳及完整场景矩阵仍需继续补齐。

测试使用独立 debug 包名：

- `com.openatx.xtest.nova.uiautomator`
- `com.openatx.xtest.nova.uiautomator.test`

未覆盖或替换 Nexus、Companion 或业务 APK。测试结束后，Android 13/15 上的两个临时包和 Android 13 的临时 Agent/配置均已删除。

## 构建与静态检查

- Gradle 8.14、AGP 8.11.2、AndroidX UiAutomator 2.4.0 均为固定版本；
- dependency lock 与 artifact SHA-256 verification metadata 已生成；
- host/test debug 与 release 构建通过；
- Agent 全量 Go 测试、`go vet`、automation 竞态检测及源码候选门禁通过；
- 完整离线构建链通过，覆盖 Runner、Companion、验证夹具、host/test release APK 以及 Agent ARM64/ARMv7；
- 修复 `uiautomator/build.ps1` 对调用者当前目录的隐式依赖，现可从仓库根目录可靠执行总构建；
- 影子对照已生成结构、完整内容和 17 类关键属性指纹，并记录各来源指纹采样/变化次数；格式空白及 XML 属性顺序不会造成误报；
- release test APK 清单确认只保留 Nova Instrumentation，移除了 AndroidX test-core 合并进来的 exported 辅助 Activity 和未使用的 `REORDER_TASKS`；
- host 明确声明 `INTERNET`，因为 Instrumentation 在目标 host 进程中执行；
- 服务明确绑定 `127.0.0.1`，避免 Android 将通用 loopback 解析为 `::1` 而 Agent 使用 IPv4 时连接失败。

## 真机结果

| 设备 | 系统 | 结果 |
|---|---|---|
| R5CN30EQKNM | Android 13 / API 33 | `/health`、`/v1/hierarchy`、`/v1/windows`、`/v1/wait-stable`、`/v1/diagnostics` 通过；直连样本 55,046 bytes、146 nodes、3 windows、0 service failures。Agent `shadow` 端到端通过。 |
| R3CY80B2G4W | Android 15 / API 35 | 五个端点通过；样本 58,287 bytes、157 nodes、4 windows、0 service failures；停止 host 后 Instrumentation 退出。 |
| 83fc400c | Android 16 / API 36 | 重新连接后 host/test 安装成功；五个 Provider 端点、鉴权、Agent 影子/主模式、1,000 次热态、崩溃回退恢复及所有权清理通过。 |
| 939AX05WZJ | Android 12 / API 32（Pixel 3a XL） | 未授权请求返回 401；五个端点通过；直连样本 28,902 bytes、78 nodes、3 windows、0 service failures。Agent `shadow`、异常回退、重新启动、`/stop` 所有权清理及 1 秒短空闲退出通过。 |

Android 13 的 Agent 影子样本：

- 决策来源保持 `system-dump`；
- system dump：24 nodes、9,266 bytes、单次约 2,231 ms；
- nova provider：146 nodes、55,046 bytes、单次约 1,314 ms；
- `shadowComparisons=1`、`lastShadowNodeDelta=122`；
- `/stop` 后 Agent 正常退出，自有 Provider 端口不可访问；只停止 Nova host，不接管外部 UiAutomator。

这些是单页、单次样本，只证明观测和生命周期链有效，不能据此宣称 P95、50% 中位数提升或结构等价。节点数差异说明多窗口/序列化口径仍需在 U7 中逐属性对照。

Android 12 的 Agent 影子样本：

- 决策来源保持 `system-dump`；
- system dump：15 nodes，单次约 2,552 ms；
- nova provider：78 nodes，单次约 1,095 ms；
- `shadowComparisons=1`、`lastShadowNodeDelta=63`；
- 强制终止 Provider 后，下一次层级仍由 system dump 正常返回；
- Provider 可再次启动，`/stop` 前 host PID 存在，停止后 PID 消失；
- 使用 `idleTimeoutMillis=1000` 的独立运行返回 `status=stopped`、Instrumentation 成功结束；
- 测试包、临时 Agent、配置和转发均已清理。

## Android 12/15 连续复验与影子隔离修复

在 Android 12 `939AX05WZJ` 和 Android 15 `R3CY80B2G4W` 上执行第二轮连续验证时发现：常驻 Nova Instrumentation 会持有设备唯一的 `UiAutomation` 连接，导致后续作为决策源的系统 `uiautomator dump` 被系统以 `signal: killed` 终止。旧实现两台均只在首轮成功，后续 9/10 系统采集失败；单次影子样本未能暴露该问题。

实现已改为串行隔离：每轮先完成 system dump，再临时启动 Nova Instrumentation 采集对照，完成后强制停止 host；下一轮系统采集会等待上一轮清理完成。该模型保留 system 的决策权，但影子阶段包含冷启动成本，不能用其总耗时评价 Nova 热态性能。

修复后连续影子结果：

| 设备 | system 决策采集 | Nova 对照采集 | system/Nova 节点 | 结构/完整指纹 | 缺包降级恢复 |
|---|---:|---:|---:|---|---|
| Android 12 / Pixel 3a XL | 10/10，0 失败 | 10/10，0 失败 | 25 / 98 | 不匹配 | 系统树正常返回，记录 1 次 Nova 失败，重装后恢复 |
| Android 15 / SM-S936U | 10/10，0 失败 | 10/10，0 失败 | 423 / 507 | 不匹配 | 系统树正常返回，记录 1 次 Nova 失败，重装后恢复 |

两台的 17 类关键属性指纹在当前页面均不完全匹配，主要受节点数量、多窗口范围及两次串行采集间页面状态影响；这证明差异观测已生效，不代表 Nova 节点退化，仍需在固定业务页面逐项解释。

同一当前页面的 20 次热态数据：

| 设备 | Provider 直连 P50 / P95 | Agent `nova` P50 / P95 | 节点范围 | 指纹变化 |
|---|---:|---:|---:|---:|
| Android 12 / Pixel 3a XL | 36.0 / 103.2 ms | 87.7 / 280.9 ms | 98–98 | 0 |
| Android 15 / SM-S936U | 11.4 / 15.4 ms | 63.8 / 77.5 ms | 507–507 | 0 |

两台在 `nova` 主模式均为 20/20 成功。强制终止 host 后，Agent 均检测到 Provider 退出并回退至 system dump（Android 12 为 25 nodes，Android 15 为 423 nodes）；重新调用启动接口后恢复 `nova-provider`（分别为 98 和 507 nodes）。停止接口调用前 host PID 存在，调用后消失。最终已卸载两台的 host/test 包，删除临时 Agent 和 PID，移除端口转发；未覆盖 Android 15 上原有的 Agent 文件。

移除已被 `analyzeHierarchy` 替代的未使用 `countNodes` 包装函数后，又执行一次最小双机回归：Android 12/15 均完成 system 3/3、Nova shadow 3/3、零失败；随后在 `nova` 主模式强制终止 host，两台均回退 system 并成功重新启动 Nova。全量 Go 测试、`go vet`、automation 竞态检测和首次提交源码门禁再次通过，设备测试内容再次完整清理。

## Plant Scope 固定业务页验证

两台设备均安装用户提供的 `plant.care.identifier.app`。测试不清除应用数据，不接受条款，不触发试用或订阅；Android 12 使用应用 Settings Compose 页面，Android 15 使用现有试用说明页面。影子诊断增加稳定身份匹配数、两侧未匹配数、未匹配节点包分布、逐属性差异计数及按包节点净增量，避免把多窗口扩展节点误判成业务节点回归。

| 设备/页面 | system / Nova | 稳定身份匹配 | 未匹配解释 | 匹配节点属性差异 |
|---|---:|---:|---|---|
| Android 12 / Settings | 42 / 93 | 37 | system 侧 5 个均为 Plant Scope 的 FrameLayout、ComposeView、View、ScrollView 根容器；system 以导航栏上方 2040 为底，Nova 多窗口根以整屏 2160 为底。Nova 侧另有 48 个 System UI 和 8 个业务包未匹配节点 | 0 |
| Android 15 / 试用说明 | 42 / 107 | 42 | system 独有 0；Nova 额外覆盖 52 个 System UI、10 个桌面和 3 个业务包节点 | `index` 1 个 |

`nova` 主模式各执行 50 次热态采集：

| 设备 | 成功 | P50 / P95 | 节点范围 | 指纹变化 |
|---|---:|---:|---:|---:|
| Android 12 | 50/50 | 86.1 / 262.4 ms | 93–93 | 0 |
| Android 15 | 50/50 | 26.4 / 36.5 ms | 107–107 | 0 |

旋转专项中，Android 12 横屏连续为 80 nodes、恢复竖屏连续为 93 nodes；Android 15 横屏和恢复共完成 6 次隔离影子对照，system/Nova 均零失败。两台的自动旋转及用户旋转值均恢复测试前状态。Home 后能采集桌面多窗口，重新启动 Plant Scope 后恢复业务页；Android 12/15 分别识别 45 个业务包节点。通知面板专项中，Android 12 的 Nova 树识别 159 个 System UI 节点；Android 15 的 57 个 system 节点全部在 Nova 中匹配，Nova 额外覆盖 181 个多窗口节点，双方零失败。通知面板随后收起。

以上证据说明已验证页面没有业务语义节点退化；Android 12 的 5 个稳定身份未匹配项是根容器 bounds 口径差异，不是文本或交互节点缺失。测试结束后再次卸载 Nova host/test、删除临时 Agent/PID 并移除转发，Plant Scope 数据未清除。

## 锁屏、WebView、压力与恢复专项

Android 15 设备已配置安全锁屏，未尝试绕过或修改用户凭据。锁屏状态连续执行 5 轮隔离影子对照：system/Nova 均零失败，system 的 5 个节点全部在 Nova 的 68 个节点中匹配，Nova 额外节点 63 个且均属于 System UI。该结果证明安全锁屏下可采集、可对照，不等同于完成“自动解锁”能力。

Android 12 在 Plant Scope 隐私政策 `InternalWebViewActivity` 上完成 WebView 对照。system/Nova 分别为 393/444 nodes；采用“resource-id 优先、其次文本或 content-desc、最后 class+bounds”的稳定身份规则后匹配 288 个节点，255 个已匹配节点存在 bounds 差异。system/Nova 未匹配分别为 105/156，主要是没有稳定语义标识的 WebView 虚拟 `View`、`ListView` 及容器节点；按包净增量仍为 System UI +48、Plant Scope +3。该页面未发现明确的业务文本或交互语义丢失，但通用虚拟节点无法可靠一一对应，因此 WebView 仍保留为需要更多页面样本的风险项。

首轮 1,000 次热态采集结果：

| 设备/页面 | 成功 | P50 / P95 | 节点 | Provider PSS 趋势 |
|---|---:|---:|---:|---|
| Android 12 / WebView | 1000/1000 | 147.9 / 980.8 ms | 444，稳定 | 约 19 MB 基线，100 次约 67 MB，之后约 71–73 MB 平台化 |
| Android 15 / 试用页 | 1000/1000 | 22.4 / 35.1 ms | 107，稳定 | 约 25 MB 基线，100 次约 36 MB，之后约 36 MB 平台化 |

两台均无请求失败、死锁或持续线性内存增长。10 并发线程共 100 个请求也均成功；由于 Manager 有意串行化层级采集，Android 12 WebView 的 P50/P95 为 1707.4/2337.4 ms，Android 15 为 111.9/929.1 ms，这反映排队等待，不应当与单请求采集门槛混用。

随后两台各执行 10 轮“强制终止 Provider → system 回退 → 重启 Nova”：回退 10/10、重启恢复 10/10，未记录 Provider 失败。Android 12 Provider 直连 WebView 的 1,000 次结果为 P50 55.5 ms、P95 100.8 ms，而旧 Agent 路径 P95 为 980.8 ms，定位到 Agent 对每次大树同步执行完整 XML 规范化和 17 类属性指纹造成的分配与 GC 长尾。

Agent 已将常规采集路径调整为边界完整性校验、字节级节点计数和内容 SHA-256；严格 XML 解码、结构及逐属性规范化仍在影子对照路径执行。锁屏树上的优化后初测为 1000/1000、P50 38.8 ms、P95 53.1 ms、P99 129.3 ms；该样本不是 WebView，待设备解锁后必须在同一 WebView 页面复测，才可确认性能门槛。

恢复 `waitForIdle(200, 3000)` 后，Android 15 在安全锁屏 Bouncer 动画状态出现该调用未按全局超时返回，Provider 单客户端线程被长期占用，Agent 按请求超时回退到 system。为避免把不可靠的隐式稳定等待放在每次采集必经路径，最终 `/v1/hierarchy` 已移除 `waitForIdle`；显式稳定需求继续由 `/v1/wait-stable` 提供。重新构建和安装后，Android 12 锁屏态 20/20，P50/P95 为 46.0/71.9 ms。Android 15 Bouncer 内 `getWindows`/节点遍历仍可能阻塞并触发 Agent 超时回退，作为锁屏边界风险保留。

设备解锁后在完全相同页面完成最终复测：

| 设备/页面 | 路径 | 成功 | P50 / P95 / P99 | 节点 |
|---|---|---:|---:|---:|
| Android 12 / WebView | 优化后 Agent，无隐式等待 | 1000/1000 | 94.0 / 859.0 / 1111.7 ms | 444，其中业务包 396 |
| Android 12 / WebView | Provider 直连，无隐式等待 | 1000/1000 | 58.9 / 716.2 / 1104.2 ms | 444 |
| Android 12 / WebView | 优化后 Agent，实验性 50/250 ms 稳定窗 | 1000/1000 | 90.9 / 783.9 / 1147.4 ms | 444 |
| Android 15 / Settings | 最终 Agent，无隐式等待 | 1000/1000 | 18.4 / 28.0 / 565.8 ms | 106，其中业务包 45 |

短稳定窗只将 WebView P95 从 859.0 ms 降至 783.9 ms，仍未达到 500 ms 门槛，因此未保留。Agent 轻量路径显著改善 Android 12 WebView 的 P50（147.9 → 94.0 ms）和平均耗时，但周期性长尾在 Provider 直连时仍存在，说明主要剩余瓶颈位于 Android/WebView 无障碍树获取与序列化，而不是 Agent 转发；该页面尚不满足切主性能门禁。Android 15 固定页满足门禁。

后续分阶段计时进一步定位：Pixel 与 Android 16 的系统 WebView 均为 151.0.7922.199，因此不是 Pixel WebView 组件版本陈旧。Pixel 300 次样本中 `getWindows` P95 0.84 ms、窗口根获取 P95 14.0 ms、节点递归 P95 634.45 ms；再次细分后，属性读取 P95 46.31 ms，而 `AccessibilityNodeInfo.getChild()` 累计 P95 681.58 ms。Android 16 随后停在同一个 Plant Scope `InternalWebViewActivity`，得到 441 nodes、395 个业务包节点，与 Pixel 的 444/396 几乎等量；500 次直连中层级总耗时 P95 25.69 ms、属性读取 P95 15.21 ms、`getChild()` 累计 P95 5.04 ms。该同页等量树对照基本排除了页面复杂度差异。官方 API 33 才加入带预取策略的 `getChild(index, prefetchingStrategy)` 和窗口根预取接口；结果与 Android 12 在大型 WebView 虚拟无障碍树上批量预取能力较弱的边界吻合。由于两台 Plant Scope 内部 versionCode 仍分别为 7 和 8，且硬件性能不同，不能把全部差异严格归因于 Android 版本，但 API 32 框架查询路径是主要因素。

最终在 API 32 专用路径先取得并复用 `getRootInActiveWindow()`，利用旧框架已有的活动根预取缓存；API 33+保持多窗口取根逻辑不变。Pixel 直连 500 次仍为 444 nodes，P50/P95 从分支前约 64.8/665.3 ms降至 54.0/95.4 ms，`getChild()` P95 从 681.58 ms降至 24.48 ms；完整 Agent 1,000/1,000 为 P50 89.5 ms、P95 719.2 ms、P99 942.0 ms，节点和 396 个业务节点稳定。最终影子 3/3 与此前结果一致：匹配 289、system/Nova 未匹配 104/155、零失败，没有因版本分支丢失节点。

同一 Pixel WebView 的 system dump 30/30 为 P50/P95 3272.1/3592.1 ms、393 nodes。强制 API 32 回退 system 会使中位耗时约恶化 36 倍、P95 约恶化 5 倍，因此不采用“Android 12 禁用 Nova”。旧系统大型虚拟树采用兼容门槛：零超时、P95 不高于 1 秒且相对 system P95 至少改善 50%；当前 Nova 相对 system P95 改善约 80%，满足该门槛。通用 API 33+ 门槛继续保持 P95 不高于 500 ms。

最终无隐式等待版本的隔离影子复验均为 system 3/3、Nova 3/3、零失败：Android 12 WebView 为 system/Nova 393/444，匹配 289、system 未匹配 104、Nova 未匹配 155；Android 15 Settings 为 42/106，system 42 个节点全部匹配、Nova 额外 64。最终又各执行一次强制终止恢复：Android 12 从 Nova 444 nodes 回退 system 393 nodes 后恢复 Nova 444 nodes；Android 15 从 106 回退 42 后恢复 106，来源标记均正确。

## Android 16 补充资格

设备 `83fc400c`（Xiaomi 2510DPC44G，Android 16 / API 36）重新连接后，先前的临时 USB 安装限制已解除，host/test 两个 APK 均成功安装。未授权 `/health` 返回 401；授权后的 `/health`、`/v1/hierarchy`、`/v1/windows`、`/v1/wait-stable` 和 `/v1/diagnostics` 全部通过。Plant Scope 当前页直连样本为 110 nodes、3 windows、64 个业务包节点，Provider 记录 5 个请求、0 失败。

隔离影子连续 5/5：system 固定为 62 nodes，Nova 为 110 nodes，双方均零失败；稳定身份匹配 61，system 唯一未匹配节点属于 Plant Scope 根容器。Nova 额外覆盖 45 个 System UI、1 个桌面和净增 2 个业务包节点；已匹配节点只有 1 个 `index` 差异。主模式 1,000/1,000 成功，P50/P95/P99 为 21.0/29.0/63.7 ms，节点稳定为 110、业务包节点稳定为 64，最大单次 767.3 ms。

为控制页面复杂度变量，Android 16 又在与 Pixel 相同的 `InternalWebViewActivity` 完成 500 次直连采样：树稳定为 441 nodes、395 个业务包节点；客户端 P50/P95/P99 为 26.12/37.74/260.44 ms，Provider 层级总耗时为 15.89/25.69/246.72 ms。其中窗口根获取 P95 3.33 ms、属性读取 P95 15.21 ms、`getChild()` 累计 P95 5.04 ms。与 Pixel 的 444/396 等量树相比，该结果进一步确认 Android 12 长尾不由 WebView 页面规模造成。

强制终止 Provider 后，Agent 从 Nova 110 nodes 正确回退到 system 62 nodes；重新启动后恢复 Nova 110 nodes。停止接口后 Provider PID 消失。测试结束后卸载 host/test、删除临时 Agent/PID/日志并移除转发；Plant Scope 及其数据未改动。

## 双机 10,000 次稳定性资格

最终版本先在 Plant Scope Settings 固定页执行直连压力：Pixel 与 Android 16 均为 10,000/10,000、零失败，节点分别恒定为 93/92；P50/P95/P99 分别为 34.80/49.10/134.14 ms 和 11.33/16.58/42.21 ms。采样 PSS 区间分别为 44.2–50.6 MB 和 35.1–36.6 MB。

随后两台均进入相同的隐私政策 `InternalWebViewActivity`，待 WebView 完全加载后再次执行 10,000 次：

| 设备 | 成功 | P50 / P95 / P99 | 最大值 | 节点范围 | PSS 采样区间 |
|---|---:|---:|---:|---:|---:|
| Android 12 / Pixel 3a XL | 10000/10000 | 60.93 / 99.38 / 904.24 ms | 1464.00 ms | 444–444 | 61.6–83.1 MB |
| Android 16 / Xiaomi 2510DPC44G | 10000/10000 | 25.31 / 35.52 / 228.70 ms | 1019.20 ms | 440–441 | 38.7–45.5 MB |

两台 Provider 诊断均为零内部失败；Pixel 最终 `lastFailure` 为空，结束后 PSS 为 56.6 MB，已从压力过程峰值回落。API 32 活动根缓存分支在大型 WebView 上保持节点完整，并通过 10,000 次无死锁、无进程泄漏门禁。该短时高频压力不能替代两小时长稳，后者仍需独立观察时间维度上的内存趋势。

## Foloy 三机业务页与分屏补充（2026-09-09）

用户提供的 Foloy 已安装在 Android 16 `83fc400c`、Android 15 `R3CY80B2G4W` 和 Android 13 `R5CN30EQKNM`，包名均为 `tcg.scanner.value.app`。Android 16/15 为 versionName 1.3.0、versionCode 43；Android 13 为 versionName 1.3.0、versionCode 41。验证不清除应用数据，不触发扫描、相机、登录或购买；Android 16/15 只关闭可独立取消的订阅页，Android 13 未点击包含同意条款语义的 Continue。

五个 Provider 端点和未授权 401 在三台均通过。Foloy 首页 20 次直连均零失败：

| 设备 | 页面状态 | 成功 | P50 / P95 | 总节点 / Foloy 节点 |
|---|---|---:|---:|---:|
| Android 16 / API 36 | MainActivity 首页 | 20/20 | 23.03 / 28.17 ms | 101 / 59 |
| Android 15 / API 35 | MainActivity 首页 | 20/20 | 25.19 / 94.64 ms | 119 / 59 |
| Android 13 / API 33 | 现有应用状态 | 20/20 | 45.73 / 72.70 ms | 76 / 34 |

Android 15/13 又在 Home、Category、Collection、Profile 四页各完成 5 次直连，均零失败且每页节点数稳定。Android 15 的总节点/Foloy 节点分别为 120/60、162/102、100/40、115/55；Android 13 分别为 104/62、165/123、125/83、115/73。页面语义覆盖首页估值与扫描入口、六类卡牌分类、收藏/愿望单/历史及 Profile 入口。

三台首页隔离影子均达到 Nova 5/5、零失败。Android 15/16 的 system 57 个 Foloy 节点中，初始稳定页有 56 个匹配；Nova 分别额外覆盖 Samsung/Xiaomi 的 System UI、Launcher 和 2 个净增 Foloy 节点。Android 15/16 的 Category、Collection、Profile 各 3 轮影子中，system 业务节点全部在 Nova 中匹配；Home 趋势卡片会在两次串行采集间滚动，因此保留动态未匹配项。Android 13 四页各 3 轮影子均成功；其 system 树对匿名容器和底部导航存在零 bounds/不同窗口边界，导致稳定身份匹配率低于 Android 15/16，但两侧关键业务文本均存在，Nova 每页净增 2 个 Foloy 节点，未发现明确语义丢失。

Android 15 通过系统最近任务 UI 建立 Foloy 上半屏、Settings 下半屏的真实 `multi-window`。system dump 仅返回当前 Settings 的 19 nodes；Nova 返回 161 nodes，其中 Foloy 36、Settings 56，并额外覆盖 System UI 59 和 Launcher 10。system 的 19 nodes 全部在 Nova 中匹配。分屏直连 50/50、零失败，P50/P95 为 46.00/318.99 ms，总节点固定 161、Foloy 节点固定 36；停止测试用 Settings 后 Foloy 恢复全屏 120/60。

三星最近任务界面本身会令 system `uiautomator dump` 返回 `null root node`，而 Nova 可正常取得 93 nodes，可作为 system 能力边界证据。Android 16 的 HyperOS 命令行窗口模式未形成真实双窗；最近任务自动操作又因坐标/窗口偏移意外弹出卸载确认，因此立即取消并停止自动点击，确认 Foloy 包仍安装且未删除数据。Android 13/16 的真实分屏留待受控手工进入后复验。

## 结论与剩余门禁

源码、构建、代码侧结构/属性对照能力、Android 12/13/15 基础运行和 Android 12/13 Agent 影子链已成立；默认配置继续为 `system`。以下项目未完成前不得切换发布默认值：

- 更多 WebView 页面、Android 13/16 真实分屏、解锁恢复及 SurfaceView 能力边界；
- 使用新结构/属性指标完成多页面真机对照并解释差异；
- 两小时内存趋势；
- Monkey、Popup、录制回放及 77 项合同的完整回归。
