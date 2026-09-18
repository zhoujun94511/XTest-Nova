# 游戏渲染探测与通用应用隔离整改 Todo

日期：2026-09-15

## 审计结论

游戏类应用需要专门的“能力探测”，但不应继续采用“命中 Unity 类名即自动无限随机点击”的执行逻辑。Activity 类名、SurfaceView 和 TextureView 只能证明页面可能缺少 Accessibility 语义，不能证明页面安全、活跃或适合持续坐标输入。因此采用分层方案：探测组件输出证据，策略组件负责授权，输入组件限制坐标范围。

暂不把 OCR、通用视觉模型或第三方游戏驱动直接引入核心运行时。它们会增加模型版本、设备算力、离线依赖和误识别风险；后续应作为可选 Provider，通过统一 Snapshot/Probe 接口接入。

## Plan 与 Todo

### P0：隔离探测与执行策略

- [x] 新增独立 `RenderSceneProbe`，输出 `none/render_surface/unity_activity`，不直接执行输入。
- [x] 新增独立 `CoordinateFallbackController`，提供 `off/bounded/continuous` 三档显式策略。
- [x] 默认策略为 `bounded`；Unity 特征不再自动获得持续输入权限。
- [x] 只有显式选择 `continuous` 时，纯渲染场景才持续 tap/swipe。
- [x] 存在可操作覆盖控件时继续优先层级动作。
- [x] 探测和回退事件记录 signal、mode、attempt 和 continuous，便于审计。
- [x] 新增独立 `SafetyActionPolicy`，默认阻止安装、下载和应用商店类 CTA；智能探索同步相同安全口径。

### P0：限制通用应用回归面

- [x] 非 Unity SurfaceView 保持最多 5 次有界兜底。
- [x] `off` 模式下纯渲染页面不执行坐标兜底。
- [x] 通用有界兜底移除 MENU，只保留 tap/swipe/back。
- [x] 新增 `SafeTouchRegion`，按设备 density 排除顶部/底部系统栏和左右边缘。
- [x] Agent、Web 工作台和 Companion 悬浮菜单贯通策略字段。
- [x] 西瓜制作机 2048 的 20 分钟模板显式声明 `continuous`，不依赖包名硬编码。

### P1：工程与发布一致性

- [x] 将广告、支付、权限和外部系统页处理从 `Main.java` 抽为独立 `SpecialHandler`；新增逻辑不再继续堆入主文件。
- [x] U8 资格未完成前，Agent 与部署脚本的层级 Provider 默认恢复为 `system`。
- [x] 更新架构和历史计划说明，消除默认值冲突。
- [x] Runner 自测覆盖普通静态页、SurfaceView、Unity、可操作覆盖层、三档策略和安全坐标。
- [x] Go 配置校验、命令透传和幂等配置指纹覆盖新字段。
- [ ] 使用正式签名重新构建完整运行时、同步 bundle 并生成新 `release-manifest.json`；开发构建不得覆盖正式清单。
- [ ] Android 15/16 使用显式 `continuous` 重新执行游戏快速回归；本项需要部署新签名构建。
- [x] Android 15/16 使用隔离临时测试签名完成 `off/bounded/continuous` 快速回归、Shortwave 隔离回归及游戏性能采样；该结果不替代上一项正式签名门禁。

### P2：Runner 大文件渐进拆分

- [x] 本轮先拆出特殊场景、渲染探测、回退控制和安全坐标四个边界明确、可独立自测的组件；`Main.java` 从约 105 KB 降至约 83 KB。
- [ ] 将剩余 `NodeExplorer` 依次拆为 hierarchy parser、scene model、action selector、graph state 四个组件；这是高回归面结构调整，不与本轮策略变更一次性混做。
- [ ] 将 Config、Guard、ShellInput 拆为独立文件并为命令超时、前台快照和参数校验增加直接单测。
- [ ] 增加源码体积趋势门禁：新业务类建议不超过 500 行，编排入口建议不超过 300 行；历史文件采用只降不升策略。

### P1：下一阶段探测 Provider

- [ ] 增加 SurfaceFlinger 图层活跃度探测：连续窗口无 presented frame 时暂停坐标输入，而不是继续盲点。
- [ ] 增加低成本截图变化率/黑屏/静帧探测，用于区分加载、暂停、广告遮挡和可交互游戏画面。
- [ ] 将渲染探测结果统一为带置信度、来源和时间戳的 `RenderProbeSnapshot`，供 Runner 与智能探索共用。
- [ ] 在相机、视频、地图、远程桌面、Canvas、非 Unity 游戏上建立隔离回归矩阵。
- [ ] OCR/视觉识别仅作为可选 Provider 评估，不进入默认发布链路，且不得绕过支付与外跳保护。

## 验收口径

1. 普通有语义节点应用的动作序列不因渲染探测改变。
2. 非 Unity 渲染页面默认最多 5 次兜底，不能持续输入。
3. Unity 页面未显式授权时同样最多 5 次；显式 continuous 才可持续。
4. 每次输入仍经过前台稳定快照、特殊页预处理和支付/广告保护。
5. 新签名制品与正式发布清单哈希一致后，才能标记设备复测和发布项完成。
