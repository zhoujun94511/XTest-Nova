# 游戏跨代际 Ping-Pong 整改 Todo

日期：2026-09-15

## 决策

Android 16 西瓜游戏 20 分钟复测中，72 次探索代际耗尽/重启有 68 次发生在坐标结果后 500ms 内。现有 `cycleDetections=0`，根因是反循环状态只覆盖单个 `NodeExplorer`，而 `Main` 在 `EXHAUSTED` 后重建 Explorer，形成“坐标输入 → 短暂语义层 → 耗尽 → Back/重拉 → 纯渲染层”的跨代际循环。

本轮只修复该确定性问题，不引入视觉模型或外部资源。外跳验收线按产品定位从 1% 调整为约 2%；有限系统交互可以接受，但必须可恢复且不得完成支付、安装、订阅等副作用。

## Todo

### P0：跨代际循环守卫

- [x] 新增独立 `GenerationPingPongGuard`，不继续把状态机堆入 `Main.java`。
- [x] 仅显式 `continuous` 模式参与判定；普通 NodeExplorer 与 bounded/off 路径不变。
- [x] 使用单调时钟识别“坐标 target 结果后 1.5 秒内立即 EXHAUSTED”。
- [x] 同一 Activity 连续 3 次命中后阻止 Back/重拉，保留当前 Explorer 等待稳定。
- [x] 等待采用 30/60/120 秒封顶的递增退避；退避结束只放行一次有限恢复。
- [x] 退避门禁覆盖整个探索入口；等待期间禁止语义点击、随机坐标、Back 和重拉，但仍轮询前台/停止状态。
- [x] Activity 变化或真实语义动作成功后重置守卫，避免状态污染其他页面。
- [x] 每 10 秒输出等待心跳，停止信号和运行状态仍可观测。
- [x] 输出 `generation_pingpong_blocked`、`generation_pingpong_wait` 并加入 Agent 汇总。

### P0：验证

- [x] Runner 自测覆盖触发阈值、等待心跳、退避升级、Activity 隔离和语义进展重置。
- [x] Agent 日志汇总测试覆盖新指标。
- [x] Runner 全量构建、Agent 全量测试、runtime bundle 同步和包体门禁。
- [x] Android 16 西瓜游戏使用相同 seed 完成 20 分钟针对性真机回归；守卫真实命中，但代际重启只下降 61.1%、立即耗尽只下降 69.1%，未达到 90% 验收线。
- [x] Android 15 游戏与两机 Shortswave 完成隔离回归；Shortswave 吞吐相对基线约 +5.8%/-1.6%，没有进入游戏守卫。

最终补严构建结果：Runner 54,329 B（上限 65,536 B），ARM64/ARMv7 Agent 相对基线分别增加 192/252 B；runtime bundle Runner 哈希一致。Runner SHA-256 为 `EC20E8EA62B28B2D30E63C22D68A9036B06DFE42568B31E372B25D18E12D49E2`。

Android 16 已对守卫前一版完成 5 分钟同 seed 冒烟：87 个事件、76 个坐标动作、Crash/ANR/Native Crash 为 0。本轮没有进入此前“坐标后立即代际耗尽”的现场，故 `generationPingPongBlocks=0`，只能证明构建运行正常，不能将 20 分钟真机验收项勾选。随后补严了覆盖整个探索入口的退避门禁并通过本地全量门禁，但该最终构建尚未做新一轮真机验收。前述冒烟轮外跳 5/76（6.58%），仍高于约 2% 容忍线。

最终构建随后完成 Android 15/16 各应用 20 分钟真机复测，完整结论见 `tests/reports/android15-16-pingpong-final-validation-20260915/REPORT.md`。Android 16 守卫 9 次真实命中且退避窗口零违规，但总体降幅未达到 90%；两机游戏外跳率分别为 5.56%/3.64%，Android 16 另记录目标游戏进程一次 abnormal SIGKILL。因此本项完成了验证执行，但产品验收仍未通过。

## Phase 2（2026-09-15 代码落地）

- [x] 守卫 CONFIRMED 态：退避结束后 `exploration_cycle_refreshed` 软重建 Explorer，不 Back/重拉。
- [x] 已确认态下一次「坐标 target → 立即 EXHAUSTED」即再拦截；120s 无进展才 `pingpong_confirmed_stall` 硬重拉。
- [x] 进展解除：连续 2 个不同 semanticScene、60s 无立即耗尽、Activity 切换；不再用单次 EXECUTED 清空守卫。
- [x] `RenderContextQuarantine`：外跳后场景级 tap 冷却/暂停/会话禁 tap（保留 swipe）。
- [x] `relaunch_requested` / `relaunch_completed` + Agent exit-info 关联 FORCE STOP。
- [x] `ParseSurfaceFrames` 优先 SurfaceView/BLASTBufferQueue 图层。

待办：同 seed Android 15/16 各 20 分钟真机复测验收。

## Unity release 专项补强（2026-09-15）

Android 16 定向复现确认了另一条确定性循环：Unity hierarchy overlay 执行完
全部 action 后，`releaseContinuousRenderExhaustion()` 会反复清空同一语义
场景的预算。首版将 release 直接接入 generation hold，真机虽无重启，但
30/60/120 秒内没有探索输入，属于冻结而非有效修复。

- [x] 同一 Unity 语义 overlay 只允许一次 action budget replay；第二次耗尽
  主动执行 Back，第三次起进入原代际恢复，不再用长 hold 处理 release 循环。
- [x] Unity recovery budget 随 Explorer history 继承，代际重建不能重置
  replay 上限。
- [x] 30/60/120 秒 hold 期间不执行 60 秒稳定清除；稳定观察窗口从 hold
  释放并 soft refresh 后重新计时。
- [x] CONFIRMED 达到 120 秒无有效进展后放行 `pingpong_confirmed_stall`
  hard recovery，避免 120 秒 hold 无限续期。
- [x] 自测覆盖 Unity replay/Back/fallthrough、预算继承、二三级 hold、
  稳定解除、hard recovery，以及 bounded/off 隔离。
- [x] Runner、根目录全量构建、Agent 测试与 `go vet ./...` 通过。

最终 Runner SHA-256：
`8DDE68ED62CA5D8B0A365CC5BB98FD87DA5A503DDC1246E8900B82BB73B2BB49`；
ARM64 Agent SHA-256：
`4EFAD87AE4FEC44C5F9F0DC8860A46BC509FD8047968948634C52B81C0218D57`。
新 Runner 已同步到 runtime bundle，完整 Agent 二进制已部署，设备释放出的
Runner 哈希与构建产物一致。

Android 16 三分钟 Unity 主动恢复回归持续产生 59 个坐标动作，generation
block/wait、软刷新、重拉、外跳及 Crash/ANR/Native/abnormal 均为 0，没有
再通过 hold 让界面静止。三分钟 Shortswave bounded 隔离烟测为 34 events，
坐标动作、generation guard、soft refresh 均为 0，稳定性指标全为 0。
证据分别位于 `tests/reports/unity-active-recovery-a16-20260915-223412/` 与
`tests/reports/shortswave-unity-isolation-a16-20260915-221627/`。

上述短时回归证明 Unity 专属门禁和非 Unity 隔离按预期工作，但不替代同 seed
Android 15/16 各 20 分钟产品验收；90% 降幅、约 2% 外跳率和长期 hard
recovery 归因仍保持待验收。

## 外跳新验收口径

- 坐标外跳率目标不高于约 2%，按完整会话动作数计算，不用短窗口小分母误判。
- 外跳 100% 在 30 秒内恢复或明确耗尽；恢复 P95 单独记录。
- 支付确认、安装执行、订阅确认及不可逆系统操作仍为 0。
- 约 2% 是验收容忍线，不是鼓励探索器主动点击广告或商店入口；区域封禁和第三次外跳降级继续保留。

## 验收

1. Android 16 同类 20 分钟运行中，`coordinate target → immediate exhaustion → generation restart` 高频序列下降至少 90%。
2. `generation_pingpong_blocked` 能命中现场，退避期间不产生新的 Back/重拉或坐标动作。
3. 游戏最大结构化事件间隔小于 30 秒；退避心跳保证运行可观测。
4. Shortswave 动作吞吐相对相同 seed/Provider 基线下降不超过 10%。
5. 游戏坐标外跳率不高于约 2%，所有外跳安全恢复，Crash/ANR/Native Crash 为 0。
