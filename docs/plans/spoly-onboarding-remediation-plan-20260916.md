# Spoly 多页 Onboarding 整改计划（2026-09-16）

## 背景与决策

Android 15 真机上的 Spoly 1.3.0 首启链路已证明控件树和触控可用。Runner 能点击
`Get Started` 和首个 `Continue`，但后续页面复用了相同 Activity、按钮文案和坐标，
现有 `cycleScene + action id` 去重将它们误判成同一个已执行动作。动作耗尽后通用
Back/重拉又把应用带回引导起点。

本轮按通用 Compose/ViewPager 行为修复，不加入 Spoly 包名、页面文案之外的应用特判，
不放宽支付、订阅、安装和下载安全过滤。

## 实施范围

| ID | 优先级 | 任务 | 验收标准 |
|---|---|---|---|
| T1 | P0 | Onboarding 前进动作改为页面级去重 | 同一 Activity 连续页面复用 `Continue` 时每页均可执行；同一语义页不重复点击 |
| T2 | P0 | Onboarding 耗尽与通用恢复隔离 | 动作耗尽或前进后的空过渡帧超时时结构化停止，不执行 Back、force-stop 或重拉 |
| T3 | P1 | Agent 汇总新停止原因和指标 | `stopReason=onboarding_exhausted`，报告包含耗尽次数 |
| T4 | P0 | 回归覆盖 | 五页 Compose Pager、历史继承、恢复释放和安全过滤均有自动测试 |
| T5 | P0 | 构建与真机验证 | Runner/Agent 测试通过；Android 15/16 当天首装且仍停留 onboarding 的状态到达业务页面；最终构建部署并校验健康与哈希 |

## 设计约束

1. 普通动态页面继续使用粗粒度动作集合去重，避免价格、计时器等文本变化重新打开动作预算。
2. 仅 `Continue`、`Next`、`Get Started`、`Start Exploring` 等已识别前进动作使用
   `Activity + semanticScene + action` 页面级身份。
3. 普通代际刷新继承已完成 onboarding 页；明确重拉后的 recovery generation 可释放页面预算，
   但不得丢弃安全封禁和敏感输入历史。
4. 已识别 onboarding 页面在全部安全前进动作与左滑预算耗尽后停止并保留现场，禁止套用
   通用 DFS 的 Back/重拉。
5. onboarding 前进动作后的同 Activity 空动作过渡帧最多等待 20 秒；仍无可操作节点时
   以 `transition_timeout` 原地停止，禁止将渲染延迟误判为 DFS 回溯机会。
6. 修复不改变付费墙、购买确认、广告和外部系统页面策略。

## 验收证据

- Runner self-test 能复现“不同标题、相同 Continue、相同 cycleScene”的旧缺陷并通过修复。
- Agent 事件解析测试识别 `onboarding_exhausted` 和 `onboarding_exhaustion_guard`。
- 发布 Runner 与设备端哈希一致。
- Android 15/16 使用同一 Spoly 版本和相同 Runner 配置，在当天首装且尚未完成 onboarding 的
  现场状态验证；跨页前进成功、无重拉，无支付、订阅、安装副作用，Crash/ANR/Native Crash 为 0。
- 真机验证发现的空过渡帧回退已追加自动化回归并纳入最终发布物；不擅自清除应用数据，最终构建的
  再次首装验证单独列为待授权项。
