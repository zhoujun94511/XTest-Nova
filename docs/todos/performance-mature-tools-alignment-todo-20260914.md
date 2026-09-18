# 性能采集成熟工具对齐计划与完成记录

日期：2026-09-14

## 目标

参考 SoloPi、SoloX、Android `dumpsys` 与 Perfetto 的指标口径，优化 XTest Nova 的实时性能采集；只借鉴协议和统计方式，不引入第三方 APK、Python 服务或动态插件依赖。

## Todo List

- [x] 区分网络累计量与采样区间速率，避免把开机累计字节误解为实时流量。
- [x] 网络速率使用性能会话私有基线，不与监控接口或其他会话共享状态。
- [x] CSV 同时输出上下行累计字节、每秒速率和速率窗口。
- [x] Web 与悬浮窗优先展示上下行速率，并明确标注累计值。
- [x] 汇总文件增加每项指标的 `measured/idle/warmingUp/unsupported/failed` 质量计数。
- [x] 汇总增加上下行速率的 min/max/mean/P50/P90/P95。
- [x] 速率摘要按真实计数器窗口去重，避免页面轮询读取同一缓存值时重复加权。
- [x] 将 ROM 不支持与采集失败分离，不再把 GPU 能力缺失误计为部分失败样本。
- [x] Web 实时卡片和趋势图消费指标状态，预热、不支持、失败不再显示为数值 `0`。
- [x] 按指标解释空闲值：CPU/GPU/网络的真实 `0` 参与统计；FPS 静止窗口画断点并排除 P50/P95，避免把“无新帧”误作 0 FPS。
- [x] Web 增加网络上下行趋势、本次 P50/P95 摘要和逐指标质量统计，无需手工阅读 JSON。
- [x] Agent 后台/前台启动输出 Web 控制台的 ADB 转发与浏览器地址；部署脚本直接打印已转发的控制台和健康接口。
- [x] 增加网络预热、计数器回绕与质量摘要回归测试。
- [ ] 将 Perfetto FrameTimeline 作为可选离线深度分析器接入，不进入默认实时采样热路径。
- [ ] 增加性能历史基线的双设备/双版本可视化比较；当前已有阈值与摘要能力，后续独立组件化实现。

## 取舍

- 实时屏幕呈现帧率使用轻量 SurfaceFlinger timestats；`gfxinfo` 只保留为 View 渲染率与 jank 来源，不再作为屏幕 FPS。Perfetto 能给出更准确的 FrameTimeline 与 jank 类型，但持续抓 trace 会显著增加开销，不适合作为 1 秒级实时采样默认实现。
- CPU 保持包级多进程汇总，避免只看前台 Activity 主进程遗漏 `:service` 等子进程。
- ROM 不提供 GPU、电流等数据时保留 `unsupported`，不伪造为 0；静止页面的 FPS/网络速率使用 `idle`，与采集失败分开。

## 三机 Gallery 验证

基于最新自包含 arm64 Agent 和正式签名 Companion，三台设备均执行 12 秒、1 秒间隔采样，并在 Gallery 内持续滚动：

| 设备 | 目标包 | 样本 | 失败/部分失败 | 独立网络速率窗口 | 产物目录 |
| --- | --- | ---: | ---: | ---: | --- |
| Android 13 `R5CN30EQKNM` | `com.sec.android.gallery3d` | 11 | 0 / 0 | 2 | `/sdcard/xtest-nova/com.sec.android.gallery3d/Perf/20260914_113119` |
| Android 15 `R3CY80B2G4W` | `com.sec.android.gallery3d` | 12 | 0 / 0 | 2 | `/sdcard/xtest-nova/com.sec.android.gallery3d/Perf/20260914_113119` |
| Android 16 `83fc400c` | `com.miui.gallery` | 12 | 0 / 0 | 2 | `/sdcard/xtest-nova/com.miui.gallery/Perf/20260914_113120` |

Gallery 在测试窗口没有产生应用 UID 网络流量，所以两个窗口均被明确判定为 `idle`，而不是失败。Android 16 的 GPU 与电流为 `unsupported`，但修复后不再导致 `partialSamples` 增长。三台设备的 `summary.json` 均只统计 2 个真实速率窗口，没有把 1 秒轮询命中的缓存值重复计入百分位。

追加静止帧口径验证：Android 16 Gallery 产物 `/sdcard/xtest-nova/com.miui.gallery/Perf/20260914_114914` 共 7 行，帧率质量为 `warmingUp=2`、`idle=5`、`measured=0`，摘要没有生成虚假的 `fps=0` 统计项，`partialSamples=0`。Web 真机页面已验证实时指标、5 类趋势、本次摘要和质量卡片均正常渲染，无按钮异常放大或版面断裂。

验证门禁：Agent 全量 Go 测试通过、Web JavaScript 语法检查通过、完整发布门禁通过、正式签名全量构建通过。
