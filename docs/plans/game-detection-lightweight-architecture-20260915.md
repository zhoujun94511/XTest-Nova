# 游戏探测与执行方案设计（纯本地、零外部依赖）

日期：2026-09-15

## 1. 审核修订结论

Nova 游戏探测必须保持自包含、离线可用，不依赖云服务、主机 Sidecar、游戏源码、被测包插桩或第三方 SDK。

推荐方案只使用项目已经具备的 Agent、Runner、系统命令、层级、SurfaceFlinger 和截图通道：

1. 本地采集 Activity、层级、Surface 帧增量和缩略图差异。
2. 使用确定性规则判断语义页、活跃渲染页、静帧、过渡页和特殊页。
3. 坐标输入改为安全网格候选，并验证每次动作前后的真实变化。
4. 广告、支付、安装、外跳、静帧及无效区域全部在设备端熔断。

成熟项目仅用于理解行业问题，不成为 Nova 的依赖、协议或后续必选路线。禁止引入 Firebase、Airtest、AltTester、OpenCV、OCR、通用视觉模型、远程识别服务、新增 APK 或新守护进程。

## 2. 项目实际基线

### 2.1 当前制品

| 制品 | 当前测试构建 | 本方案门禁 |
|---|---:|---:|
| Agent ARM64 | 13,152,456 B（约 12.54 MiB） | P0 增量不超过 256 KiB |
| Runner JAR | 46,450 B（约 45.4 KiB） | 不超过 64 KiB |
| Companion APK | 53,657 B（约 52.4 KiB） | 不增长或最多增加 4 KiB |

发布检查必须同时覆盖单文件和内嵌 runtime bundle，不能只看 Companion APK。

### 2.2 可以直接复用的代码

- `RenderSceneProbe` 已能区分普通页面、Surface 渲染页面和 Unity Activity。
- `CoordinateFallbackController` 已隔离 `off / bounded / continuous` 授权。
- `SafeTouchRegion` 已排除系统栏和边缘手势。
- `SpecialHandler`、`AdScenePolicy`、`SafetyActionPolicy` 已承担广告、支付、外跳及安装下载保护。
- Agent `system.Service` 已有 SurfaceFlinger timestats 和目标 Layer 帧计数解析。
- Agent 已有截图、启动首帧、性能采集、层级 Provider 和结构化产物能力。
- Runner 已通过本机回环调用 Agent 层级接口，不需要新增通信进程。

### 2.3 真正缺口

1. Unity Activity 或 SurfaceView 只能说明页面可能缺少语义节点，不能证明它可交互。
2. 当前坐标动作缺少统一 before/after 验证，无法判断动作是否推进场景。
3. Runner 平铺 XML 标签，容易丢失游戏内广告父容器上下文。
4. `continuous` 仍缺少区域级失败预算和外跳区域封禁。
5. Unity/OpenGL/Vulkan 不能使用 View `gfxinfo` 冒充完整游戏帧数据。

## 3. 对成熟方案的正确借鉴方式

以下项目仅证明问题边界，不进入产品架构：

- Firebase Game Loop 说明跨引擎游戏无法只靠标准 UI 层级可靠操作。Nova 只借鉴“场景可复现、执行有超时、结果结构化”，不接入 Firebase，也不要求游戏修改 Manifest。参考：https://firebase.google.com/docs/test-lab/android/game-loop
- Airtest 说明图像识别必须带阈值、位置和失败语义，而且存在误匹配。Nova 只借鉴“低置信度不执行”，不接入 Airtest、Python、OpenCV 或模板资产。参考：https://airtest.doc.io.netease.com/en/IDEdocs/airtest_framework/3_airtest_image/
- AltTester 说明 Unity 对象级识别依赖插桩测试包，因此不符合 Nova 的第三方 APK 黑盒目标。Nova 不实现 Unity Driver。参考：https://alttester.com/docs/sdk/latest/home.html
- Android 官方说明 Unity、Unreal、OpenGL/Vulkan 绕过常规 View 渲染路径。Nova 继续使用设备本地 SurfaceFlinger 帧增量，不把 View 指标混入游戏口径。参考：https://developer.android.com/topic/performance/vitals/render

## 4. 纯本地目标架构

```text
Activity / Hierarchy ─┐
Surface frame delta ──┼─> LocalRenderProbe ─> RenderProbeSnapshot
32×18 visual delta ───┘          │
                                 v
Special/Safety policy ──> GameActionPolicy
                                 │
                                 v
                         ZoneActionPlanner
                                 │
                                 v
                      ActionResultVerifier
                                 │
                       reward / block zone
```

以上模块全部运行在现有 Agent 和 Runner 中，只使用现有 7912 回环服务，不增加端口、进程或网络依赖。

### 4.1 RenderProbeSnapshot

```json
{
  "schemaVersion": "xtest-render-probe/v1",
  "sceneType": "semantic_ui|render_active|render_static|transition|special|unknown",
  "confidence": 0.86,
  "signals": ["unity_activity", "surface_layer", "frame_delta", "visual_delta"],
  "frameDelta": 12,
  "visualChangePermille": 184,
  "blackPermille": 3,
  "foregroundStable": true,
  "sampledAtElapsedMillis": 12345678,
  "validForMillis": 1500
}
```

规则：

- 置信度只是观测结果，不能直接授权输入。
- 使用单调时钟关联动作，禁止用墙上时钟判断先后。
- 每个信号必须标记 `measured / warming_up / unsupported / failed`，缺失不能当 0。
- `special`、前台不稳定或快照过期时，坐标动作无权执行。

### 4.2 三个编译期本地 Provider

#### HierarchySignalProvider

- 复用现有 hierarchy Provider。
- 输出语义动作数、Surface、WebView、广告父上下文和危险 CTA 区域。
- XML 改为栈式解析，广告父容器内只允许明确关闭控件。
- 不保存完整 XML 历史，只保留摘要和本轮区域掩码。

#### SurfaceFrameSignalProvider

- 复用 `ParseSurfaceFrames` 与现有命令执行器。
- 两个短窗口间目标 Layer 帧增量大于 0，才判定渲染活跃。
- Layer 必须绑定目标包/PID，系统层、广告 Activity 和多窗口层不计入目标帧。
- 与性能采样共享一份缓存，禁止同一秒重复查询 SurfaceFlinger。

#### VisualDeltaSignalProvider

- 复用现有截图通道，在 Agent 内立即缩小为 32×18 灰度网格。
- 只计算亮度、黑屏率、差分率、简单边缘密度和分区变化率。
- 使用 Go 标准库解码，常驻数据控制在 16 KiB 内，只保留 before/after 两份。
- 不做 OCR、模板匹配、特征点、对象检测或远程识别。

## 5. 本地动作闭环

每次坐标动作必须经过：

1. `observe_before`：确认目标前台稳定，特殊页预处理，取得探测快照。
2. `propose`：从安全网格选择候选，不全屏随机。
3. `authorize`：检查场景、区域、危险 CTA、广告上下文、模式和冷却预算。
4. `execute`：只执行一次动作，记录坐标和区域 ID。
5. `observe_after`：重新采集 Activity、特殊页、帧增量和画面差异。
6. `verify`：目标前台保持且出现合理变化才记成功；外跳、支付、广告点击、静帧均惩罚并封禁区域。

屏幕划分为 3×5 逻辑网格。顶部、底部、系统栏、边缘手势和广告区域不进入候选集。同一 scene-zone 连续 2 次无进展后封禁，整个静态场景最多失败 5 次。

`continuous` 只取消任务级固定 5 次限制，不取消 scene-zone 熔断、安全策略和静帧暂停，因此应逐步在界面上更名为“自适应持续探索”。

### 场景决策表

| 状态 | 默认行为 |
|---|---|
| `semantic_ui` | 使用 NodeExplorer，不启用坐标兜底 |
| `render_active` | bounded 可尝试；持续模式按区域反馈继续 |
| `render_static` | 等待一个窗口，仍静止则 Back 或重新启动 |
| `transition` | 不输入，等待稳定 |
| `special` | 只交给 SpecialHandler |
| `unknown` | 默认不坐标点击，最多一次 Back 后重新观察 |

## 6. 组件拆分

```text
agent/internal/renderprobe/
  model.go
  coordinator.go
  hierarchy_provider.go
  surface_provider.go
  visual_delta_provider.go
  cache.go

runner/.../runner/
  RenderProbeClient.java
  GameActionPolicy.java
  ZoneActionPlanner.java
  ActionResultVerifier.java
  HierarchyParser.java
```

`Main.java` 只负责编排。新类建议不超过 300 行，硬上限 500 行；识别、策略、执行和证据分别自测。

## 7. 包体与性能门禁

### 构建门禁

- Runner JAR 不超过 65,536 B。
- Agent ARM64 相对当前基线增量不超过 262,144 B；ARMv7 同步检查。
- Companion 相对当前基线增量不超过 4,096 B。
- runtime bundle 不得新增 `.so`、模型、字库、模板集、第五个 APK 或外部 Provider 配置。
- CI 输出各制品大小、增量、SHA-256，超限直接失败。

### 运行门禁

- 探测平均额外 CPU 目标小于 2% 单核。
- 截图差分默认 1.5–2 秒最多一次，静态期退避到 3–5 秒。
- SurfaceFlinger 查询与性能模块共享采样结果。
- 探测超时 800ms 后降级，不能阻塞停止信号和支付保护。
- 每会话只保留两份缩略图和有限区域历史，任务结束释放。

## 8. 落地 Todo

2026-09-15 收益复核后，完整探测闭环保留为触发式候选方案；当前只落地低风险的坐标动作证据和包体积门禁，执行记录见 [`game-evidence-size-gate-todo-20260915.md`](../todos/game-evidence-size-gate-todo-20260915.md)。下列未勾选项不得被视为已经实现。

### P0：闭环与安全

- [ ] 定义 `RenderProbeSnapshot v1` 与质量状态，增加合同测试。
- [ ] 抽取共享 Surface frame sampler，供性能与探测复用。
- [ ] 实现 32×18 标准库画面差分，覆盖黑屏、静帧、微动画和大跳变测试。
- [ ] 将 Runner XML 解析改为保留父子上下文，广告容器子节点默认过滤。
- [ ] 新增 `GameActionPolicy`、`ZoneActionPlanner`、`ActionResultVerifier`。
- [ ] 保持 `continuous` API 兼容，将执行语义升级为区域有预算的自适应持续探索。
- [x] 增加低风险的 `coordinate_action`、`coordinate_action_result`、`coordinate_action_recovery` 证据事件。
- [ ] 随完整探测闭环增加 `render_probe`、`zone_blocked`、`render_stalled` 事件。
- [x] 增加制品大小和 runtime bundle 内容门禁。

### P1：本地真机矩阵

- [ ] Unity：西瓜制作机 2048，覆盖游戏、插屏广告、商店预览和支付页。
- [ ] 非 Unity：至少一款 Unreal/OpenGL/Vulkan 游戏。
- [ ] 反例：相机、视频、地图、Canvas 和远程桌面。
- [ ] Android 13/15/16 各执行 bounded 与自适应持续模式 20 分钟。
- [ ] 记录有效动作率、静帧动作数、区域封禁数、外跳率、恢复耗时、崩溃/ANR 和包体增量。

### P2：继续增强本地启发式

- [ ] 增加分区变化热图，只保存数值，不保存模板图片。
- [ ] 增加会话内有效区域自动加权，任务结束只保留统计摘要。
- [ ] 增加加载、黑屏、暂停和循环场景的组合启发式。
- [ ] 增加确定性 seed 回放，复现网格动作与熔断路径。

## 9. 验收标准

1. 普通应用动作序列与当前基线一致，游戏探测不改变普通 NodeExplorer 默认路径。
2. 未显式授权的 Unity/Surface 页面不会持续坐标输入。
3. 同一 scene-zone 连续两次无进展后本场景封禁；外跳后立即封禁触发区域。
4. 支付确认、安装下载、系统设置和非目标包不能被任何坐标策略绕过。
5. 所有探测在断网、无控制端服务的设备上正常运行。
6. FPS/jank 继续按来源和质量状态输出，View `gfxinfo` 不冒充 Unity 完整帧统计。
7. 满足第 7 节全部包体和运行开销门禁。

## 10. 明确不做

- 不依赖 Firebase、Airtest、AltTester、云真机、主机 Sidecar 或游戏源码改造。
- 不提供动态 Provider 下载、插件加载或网络识别接口。
- 不内置 OpenCV、Tesseract、ONNX Runtime、OCR 或视觉模型。
- 不按包名硬编码“游戏 = continuous”。
- 不使用 Unity Activity 或 SurfaceView 单一信号授权输入。
- 不把截图变化直接等同于业务成功；广告视频和加载动画同样会变化。
- 不对第三方 APK反编译改包或运行时注入。
