# 游戏 SurfaceView 探索与帧率口径整改计划

日期：2026-09-15

## 审计结论

1. Unity 游戏只暴露 SurfaceView、没有可操作控件时，层级探索会把“仍有少量节点”误判为可继续探索，导致反复拉起应用而没有有效输入。
2. 旧实现把包级 `gfxinfo Total frames rendered` 差值直接命名为 FPS。游戏主 Surface 与广告/WebView 同时渲染时，该计数可能汇总多个 ViewRoot，所以可出现 `155.78` 等高于屏幕刷新率的值。
3. jank 只在少量有效 `gfxinfo` 窗口内产生。Android 16 为 49/1094（4.48%），Android 15 为 9/1074（0.84%）；直接展示百分位会掩盖覆盖率不足。

## Plan 与 ToDo

### P0：恢复游戏场景探索能力

- [x] 识别目标包的 SurfaceView、GLSurfaceView 与 TextureView。
- [x] 渲染表面存在且没有可操作节点时，向上层返回明确的层级回退信号，进入既有的受控坐标探索。
- [x] 保留“存在可操作覆盖控件时优先层级动作”的行为，避免无条件随机点击。
- [x] 增加 Runner 自测，覆盖纯 SurfaceView 与 SurfaceView + Button 两种场景。
- [x] 坐标回退复用既有悬浮层抑制、外跳恢复和支付敏感界面保护，不增加旁路输入通道。

### P0：修正 FPS 指标语义

- [x] SurfaceFlinger timestats 可用时，以目标最新图层的 presented-frame 差值作为 `fps`。
- [x] `gfxinfo` 仅作为 View 渲染吞吐率，输出为 `renderFps` / `render_fps`，不再冒充屏幕 FPS。
- [x] 两类计数器均在采集完成后记录时间戳，避免把命令耗时从分母扣掉而放大速率。
- [x] 增加“`gfxinfo` 为 156、SurfaceFlinger 为 120 时最终 FPS 必须为 120”的回归测试。
- [x] 性能会话与摘要 schema 分别升级至 v4、v3；CSV 新增渲染率值与来源列。

### P1：让低覆盖率数据不可被误读

- [x] 摘要分别统计 `fps`、`render_fps` 与 jank。
- [x] 对 FPS、jank 的有效窗口少于 30 或覆盖率低于 10% 时输出结构化 warning。
- [x] Web 页面分开显示“屏幕呈现帧率”和“View 渲染率”；命中 warning 时显示“样本不足”，不再只突出百分位数值。
- [x] 保留历史原始 CSV，不回写或伪造旧测试数据；修正报告中的旧口径说明。

### 后续真机验收

- [x] 构建并部署包含本次修复的新版本，在 Android 15/16 的西瓜制作机 2048 上各复测至少 20 分钟。
- [x] 验证 Runner 在 120 秒内产生坐标回退动作且不再只循环 relaunch；确认支付、商店、安装和订阅界面仍被保护。
- [x] 同时记录设备刷新率、`fps` 来源与 `render_fps` 来源；确认呈现 FPS 不超过设备物理刷新上限的合理容差。
- [x] 报告 measured/idle/failed、有效覆盖率与 warnings；低覆盖率 jank 不形成流畅度结论。
- [x] 增加 SurfaceFlinger 首个已观察内容帧时间上界，并提供带墙钟约束、前中后截图、外跳和支付安全审计要求的游戏/广告场景模板。
- [ ] 后续独立增加“内容稳定”和“可交互”启动指标；它们不在本次用户指定的首内容帧与场景模板范围内。

真机复测报告与证据见 `tests/reports/watermelon-maker-2048-20260915-rerun/REPORT.md`。

## 验收门禁

- Runner 自测与构建通过。
- Agent 的 system、perflog 及全量 Go 测试通过。
- Web 脚本语法检查通过。
- 新版真机结果必须使用新 schema；旧版 `gfxinfo` 的 `fps` 数据只作为历史渲染吞吐率参考，不与新版呈现 FPS 合并比较。
