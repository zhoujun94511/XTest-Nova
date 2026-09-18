# 覆盖率优先探索整改计划与完成清单

> 2026-09-15 审计纠偏：本文记录的“默认切换 Nova Provider”属于阶段性实现结果；因 U8 资格门槛、跨版本对照和两小时长稳尚未全部关闭，运行与部署默认值已恢复为 `system`。`shadow`/`nova` 继续通过显式参数用于资格验证。

目标：Nova 是面向通用应用的智能探索工具。默认策略以单位时间新增页面、Activity 和有效状态为首要目标，同时保留支付、密码、真实删除、跨应用逃逸、会话所有权和异常证据等安全底线。

## 本轮 Todo List

- [x] P0 单次观测复用：Java Runner 每轮只读取一次控件树，由特殊页面识别和普通动作生成共同消费。
- [x] P0 快速动作新鲜度：Go 探索默认只复核前台包名与 Activity；`freshnessMode=strict` 保留完整二次控件树复核。
- [x] P0 正确动作生命周期：普通动作执行成功后才进入 `tried`、输入去重和滚动次数，过期动作不再永久丢失。
- [x] P0 拒绝动作幂等编号：`stepId` 使用独立动作尝试序号，不再因陈旧动作未进入成功步骤而复用编号并误终止后续探索。
- [x] P0 快速 Provider 主路径：Agent 默认配置改为 Nova Provider，启动时主动拉起；失败时保留 system dump 安全回退。
- [x] P0 Provider 互斥回退：Nova 首次采集失败后先释放 Instrumentation，再调用 system dump，避免两个 `UiAutomation` 所有者竞争导致回退进程被杀。
- [x] P0 空窗口兼容：Nova Provider 在 Android 13+ 的窗口列表为空时补读活动窗口根节点，避免返回无节点控件树。
- [x] P0 首帧预热兼容：Android 15/16 首次无节点读取在同一 Instrumentation 所有者内短暂等待并重试，重试仍失败才释放 Nova 并进入 system 回退。
- [x] P1 广度输入策略：默认从每个输入框 18 类缩减为 1 个正常短文本，完整 18 类仍可显式配置。
- [x] P1 覆盖收益调度：导航点击优先于输入，周期性强制滚动，云同步、登录、分享、备份等高跨应用风险入口延后。
- [x] P1 自适应等待第一阶段：动作节流 750ms→250ms、特殊页面稳定等待 5s→1.5s、空页面确认 16→3、耗尽确认 8→2。
- [x] P1 探索质量指标：增加有效动作、无效动作、跨应用动作、过期动作，并在 Web 探索状态中展示。
- [x] P1 组件化：动作新鲜度与覆盖调度分别放入 `freshness.go`、`scheduler.go`，不继续堆入 Manager。
- [x] 回归：Go 全仓测试、Runner 编译与自测通过，Runner 与两个架构 Agent 产物已重建，内嵌运行包已同步。

## 后续门禁 Todo List

- [x] G1 在 Android 13/15/16 的 Gallery 上各运行同等上限 A/B：旧 system 严格策略与新 Nova 快速策略。三机任务均执行至自然结束，结果见 [`validation-coverage-first-gallery-ab-20260914.md`](../validations/runs/validation-coverage-first-gallery-ab-20260914.md)。
- [x] G2 固化指标：首次有效动作耗时、动作/分钟、新状态/分钟、新 Activity/分钟、有效动作率、外部绕行率、控件树 P50/P95。Android 13/15/16 已形成同页 Provider 延迟与探索速率基线。
- [x] G3 Nova 空树失败后验证互斥 system 回退、任务继续和报告可追溯；项目签名新包部署后再验证三机 Nova 主路径。
- [ ] G4 将固定 1.5 秒稳定窗口继续替换为基于 Activity/窗口 generation 的动态稳定检测。
- [ ] G5 为 Java Runner 补齐与 Go 调度器一致的跨应用风险评分，并逐步收敛为共享探索策略合同。
- [ ] G6 建立跨运行状态模型复用、低收益边衰减和稀有页面奖励；持久化模型必须按应用版本隔离并可清空。
- [ ] G7 仅在控件树不可用或长期无进展时启用视觉/坐标候选，所有动作继续受安全边界和预算约束。

## 验收阈值

- Gallery 首次普通动作目标：Nova Provider P95 小于 1 秒；system 回退路径单独统计，不混入主路径。
- 相同时长下，新策略动作/分钟与新状态/分钟均不得低于旧策略；任一降低超过 10% 即回滚对应策略。
- 有效动作率不低于 60%，跨应用动作占比不高于 10%，Crash/ANR/Native Crash 采集不得因提速丢失。
- Stop、过期 observation、旧 ownerToken 和重复 stepId 的安全回归必须持续通过。
