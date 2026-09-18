# Phase 2 修复计划（2026-09-15）

基于真机复测与代码审计，在既有 `GenerationPingPongGuard` 之上做四项针对性调整，不引入视觉模型。

## 背景

- 守卫退避有效，但 hold 结束后仍走 Back/monkey/重拉，且 EXECUTED 清空守卫，导致代际重启仅降 ~69%。
- 外跳仅格点封禁，bounded 降级后仍有 tap 预算。
- force-stop 无因果证据，SIGKILL 与工具杀进程不可区分。
- Samsung SF FPS 仅包名+layerId 匹配，游戏层常失败。

## 实施 Todo

| ID | 优先级 | 任务 | 状态 |
|----|--------|------|------|
| T1 | P0 | `GenerationPingPongGuard`：CONFIRMED、pendingSoftRefresh、进展解除 | done |
| T2 | P0 | `Main`：软代际刷新、120s 硬恢复、进展挂钩 | done |
| T3 | P0 | `RenderContextQuarantine` + 坐标链路 | done |
| T4 | P1 | `relaunch_requested/completed` + launcher 先 monkey | done |
| T5 | P1 | Agent 指标与 exit-info 重拉关联 | done |
| T6 | P1 | `ParseSurfaceFrames` 图层 scoring | done |
| T7 | — | self-test、`go test`、文档更新 | done |

## Unity 专项补强（2026-09-15）

真机审计发现 `UnityPlayerActivity` 的可操作 overlay 会反复耗尽并进入
`unity_cycle_released`。首版把它接入 30/60/120 秒 generation hold，虽然
停止了重启，却让游戏长期无输入，不能视为有效探索。

- [x] Unity continuous 同一语义 overlay 只允许一次 action budget replay；
  第二次耗尽执行主动 Back 恢复，后续交给原代际恢复，避免无限重放或长时间
  冻结。
- [x] Unity recovery budget 跨 Explorer 继承，不能通过重建 Explorer 重新
  获得无限 replay。
- [x] 无立即耗尽的 60 秒稳定窗口从 hold 释放后开始计算，等待时间不再被误判
  为应用稳定；60/120 秒 hold 不会提前清除 CONFIRMED。
- [x] CONFIRMED 超过 120 秒无有效进展时返回 hard recovery 决策，避免在
  120 秒封顶 hold 中无限循环。
- [x] bounded/off、非 Unity continuous 与原 generation candidate 不变。
- [x] Runner self-test、项目全量构建和 Agent `go vet ./...` 通过。

Agent runtime bundle 已同步新 Runner 并整体部署。Android 16 三分钟专项
回归持续产生 59 个坐标动作，generation block/wait、软刷新、重拉、外跳及
Crash/ANR/Native/abnormal 均为 0，未再出现“靠 hold 静止”的结果。证据位于
`tests/reports/unity-active-recovery-a16-20260915-223412/`。

Shortswave bounded 3 分钟隔离烟测：34 events、坐标动作 0、守卫/软刷新 0、
Crash/ANR/Native/abnormal 均为 0。证据位于
`tests/reports/shortswave-unity-isolation-a16-20260915-221627/`。

## 验收

1. 代际 Back/force-stop 重启相对基线 ≥90% 降幅（soft refresh 替代）。
2. 游戏外跳率 ≤2%。
3. 工具 force-stop 可标为预期退出；未知 SIGKILL 仍为 abnormal。
4. Android 15 SF FPS 有效窗口恢复或输出候选层摘要。
