# 性能采集审计整改计划与 Todo（2026-09-12）

## 目标与边界

本轮以 Nova 现有性能链路为主体，对照 Fionna 的公开产品能力和实现思路进行审计。Fionna 采用 AGPL-3.0，且 Android 帧采集依赖仓库内未提供源码的 `PerfTool.jar`/原生库；本项目没有复制其代码或二进制，也没有新增对它的构建、运行或网络依赖。可借鉴内容仅限指标可配置、卡顿统计、会话汇总和可视化等通用产品思路。

状态：`[x]` 已实现并有自动验证；`[~]` 已实现、仍受设备能力限制；`[ ]` 后续增强。

## P0：修复数据语义和会话可靠性

- [x] P0-1 应用网络从 `/proc/<pid>/net/dev` 的设备命名空间总量改为应用 UID 口径；优先读取 `xt_qtaguid`，新系统回退 `dumpsys netstats detail`，不可用时明确写入指标错误，不伪造零值。
- [x] P0-2 每个性能会话独占 FPS 与网络缓存状态，兼容接口、Monitor 和控制台查询不再污染录制基线。
- [x] P0-3 CPU、内存、FPS、GPU、电池和网络并发采集；单个可选探针失败只形成部分样本，连续 5 次核心采集失败才终止会话。
- [x] P0-4 CPU 同时输出“整机容量百分比”和“单核为 100%”两种口径，并标明进程范围；CPU 汇总主进程及 `包名:*` 子进程，内存按包、网络按 UID 聚合。
- [x] P0-5 修正 Linux CPU 总量统计，避免 guest 重复计数，并将 iowait 按空闲时间处理。

## P1：补齐指标、产物与交互

- [x] P1-1 支持 1–60 秒采样间隔和最长 7 天的可选持续时间；下一次采样从本次完成后计时，避免慢探针造成请求堆积。
- [x] P1-2 CSV 增加 CPU 双口径、内存分类、卡顿帧/卡顿率、电池、应用 UID 网络、数据来源、采样耗时和指标错误。
- [x] P1-3 每次会话生成 `session.json`、`perf.csv` 和停止后原子写入的 `summary.json`；摘要记录有效/失败/部分样本以及主要指标的最小值、最大值和平均值。
- [~] P1-4 普通 View 使用 `gfxinfo` 统计 FPS、卡顿帧与卡顿率；SurfaceView/游戏 FPS 回退 SurfaceFlinger timestats。SurfaceFlinger 当前没有与 `gfxinfo` 等价的目标应用卡顿分类，因此该场景明确不提供卡顿率。
- [x] P1-5 Web 性能页展示实时指标卡、最近 120 个样本的 CPU/FPS/内存趋势、部分探针错误和会话产物入口；运行中不展示尚未生成的摘要链接。
- [x] P1-6 增加 UID 网络解析、会话 FPS 隔离、多进程识别、卡顿增量、暂态失败续采、产物结构和前端资源回归测试。

## P2：后续增强项

- [ ] P2-1 在取得可维护且许可兼容的数据源后，为 SurfaceView/Vulkan/商业游戏增加目标应用级帧耗时和卡顿分类；不得内嵌来源不明的 JAR/SO。
- [ ] P2-2 增加历史会话选择、两次运行对比和可配置阈值判定；当前 `summary.json` 已提供稳定输入格式。
- [ ] P2-3 在更多厂商 Android 14–16 设备上建立 UID 网络、GPU 节点、帧来源和采样开销基线。

## 验收门槛

- [x] 定向 Go 测试、静态检查和前端 JavaScript 语法检查通过。
- [x] Android 13/16 各完成一次“启动应用—定时采样—自动停止—三类产物可读”的真机闭环。
- [x] 发布清单与完整发布门禁通过。

## 真机验证记录

目标应用改为设备本地 Gallery；采样间隔 2 秒、自动停止时长 6 秒。Gallery 不依赖远端业务服务器，适合作为持续回归目标。

| 系统 | 设备 | 结果 | 关键数据 | 产物目录 |
|---|---|---|---|---|
| Android 16 / API 36 | 2510DPC44G (`83fc400c`) / `com.miui.gallery` | 3 样本，失败 0，部分样本 3 | 应用 UID 网络来源 `dumpsys-netstats`；GPU 无可信节点时保持空值，并逐样本记录明确错误 | `/sdcard/xtest-nova/com.miui.gallery/Perf/20260912_184026/` |
| Android 13 / API 33 | SM-G9860 (`R5CN30EQKNM`) / `com.sec.android.gallery3d` | 3 样本，失败 0，部分样本 0 | 应用 UID 网络来源 `dumpsys-netstats`；KGSL GPU 与电池电流可读 | `/sdcard/xtest-nova/com.sec.android.gallery3d/Perf/20260912_184026/` |

两台设备目录均包含 `perf.csv`、`session.json`、`summary.json`，摘要与 CSV 可直接读取。帧来源继续逐样本记录，不把 `surfaceflinger-timestats` 与 `gfxinfo` 混写为同一口径。
