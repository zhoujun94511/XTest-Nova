# Android 15/16 复测后安全与执行效率整改 Todo

日期：2026-09-15

## 审计结论

本轮依据 `tests/reports/android15-16-shortwave-watermelon-retest-20260915` 的四组 20 分钟真机证据整改，不扩大为完整游戏视觉探测工程。

代码根因有三项：

1. `NodeExplorer.ActResult.ERROR` 同时表示纯渲染页面需要坐标兜底和层级获取/解析失败，层级失败会错误取得随机坐标输入权限。
2. `CoordinateActionEvidence.begin` 会直接覆盖尚未结算的 pending，导致坐标动作数与结果数不一致。
3. `continuous` 只有输入次数授权，没有消费外跳结果；触发商店或 Resolver 的区域仍可能再次被选择。

普通有语义节点的 `NodeExplorer` 动作选择、支付/安装拦截和默认 `bounded` 策略保持不变。

## Plan 与 Todo

### P0：层级失败不得授权坐标输入

- [x] 将层级获取/解析失败拆为 `OBSERVATION_ERROR`，与纯渲染 `ERROR` 分离。
- [x] `renderSignal=none` 的层级失败输出 `coordinate_fallback_suppressed`，不再进入 tap/swipe 随机分支。
- [x] 已知广告 Activity 层级不可用时复用现有广告等待、Back 和重拉状态机。
- [x] 普通页面层级连续失败采用独立 `HierarchyFailurePolicy`：先等待，第 3 次 Back，第 6 次重拉；Activity 变化或成功观测后重置。
- [x] 增加组件自测，避免恢复计数继续堆入 `Main.java`。

### P0：坐标证据完整结算

- [x] 下一动作覆盖 pending 前先输出 `outcome=unobserved`、`reason=superseded_by_next_action` 和下一动作 ID。
- [x] 任务结束同时结算 pending 与尚未完成的 external recovery，不再二选一。
- [x] Agent 汇总增加 `coordinateTarget`、`coordinateUnobserved`，复测可直接校验 actions/results 是否守恒。

### P0：continuous 外跳反馈

- [x] 外跳结果立即封禁触发的 3×5 渲染上下文区域（Activity + render signal + zone）；后续 tap/swipe 候选跳过该上下文的已封禁区域，不污染其他 Activity。
- [x] 同一会话累计 3 次坐标外跳后，将显式 `continuous` 降级为有界模式，不取消原有安全策略。
- [x] 输出 `coordinate_zone_blocked` 与 `render_fallback_downgraded` 事件。
- [x] Agent 汇总增加区域封禁、降级和坐标兜底抑制次数。
- [x] 区域封禁与降级逻辑由 `CoordinateFallbackController` 持有，不按应用包名硬编码。

### P1：构建与回归

- [x] Runner 编译和全部自测通过。
- [x] Agent 全量 Go 测试通过。
- [x] 新 Runner 同步进入四组件 runtime bundle，并重建 ARM64/ARMv7 Agent。
- [x] 包体门禁通过；未增加 APK、SO、模型、模板或外部 Provider。
- [x] Android 15/16 使用相同目标、seed、Provider 和 20 分钟口径复测新制品；四组均自然结束，见 `tests/reports/android15-16-shortwave-watermelon-optimization-validation-20260915/REPORT.md`。
- [x] 对照复测完成：Shortswave 通过；结果守恒、区域封禁和降级事件通过；游戏外跳率及 Android 16 跨代际 ping-pong 不通过。

构建结果：Runner 52,319 B（上限 65,536 B），ARM64 Agent 13,152,792 B（相对基线 +336 B），ARMv7 Agent 13,409,707 B（相对基线 +332 B）。Runner SHA-256 为 `804FC5A182C45B2F7DCEE231B6C25A88E3B4692A9C74C4F53D14D07D0429DCE5`，内嵌副本哈希一致。

## 当前执行效率判定

### 机械响应是否正常

| 场景 | Android 16 | Android 15 | 判断 |
|---|---:|---:|---|
| Shortswave | 172 / 1203.287s，8.58 事件/分钟 | 199 / 1203.267s，9.92 事件/分钟 | 没有停死，但吞吐偏低 |
| 西瓜游戏 | 394 / 1204.017s，19.63 坐标动作/分钟 | 284 / 1203.144s，14.16 坐标动作/分钟 | 对 system 层级采集与 500ms throttle 组合属于可解释范围 |

四组最大事件间隔为 5.0～10.0 秒，均低于项目 30 秒卡住阈值；外跳恢复平均为 3.49～4.64 秒，也低于既有 30 秒恢复门槛。因此停止响应、循环调度和恢复速度正常。

按实际动作时间戳计算，西瓜游戏动作间隔 P50/P95 为 Android 16 2.89/3.18 秒、Android 15 3.04/7.66 秒，符合 system 层级采集、500ms throttle 和外跳恢复叠加后的量级。Shortswave 为 Android 16 4.35/21.39 秒、Android 15 3.48/22.09 秒；20 秒级长尾与广告软等待相符，不属于线程卡死，但会直接压低单位时间覆盖量。

### 是否符合需求

不能只按“动作发得快”判定符合需求：

- Shortswave 上一轮 30 分钟基线约为 Android 16 16.83、Android 15 17.34 事件/分钟；本轮分别低约 49.0% 和 42.8%。两轮 seed、构建和 Provider 口径并非严格 A/B，不能直接定性为性能回归，但已明显超过项目“相同口径吞吐降低不得超过 10%”的预警线，必须在新版本同口径复测。
- 游戏的原始坐标吞吐不慢，但 Android 16/15 外跳率为 1.52%/6.69%，且当前 `target` 只表示动作后仍在目标包，不等于游戏场景真正推进。因此游戏只满足机械执行速度，不满足安全有效效率要求。
- 新策略会在危险现场主动少点或降级，原始动作数可能下降。验收应优先看外跳率、结果完整率、有效场景推进和恢复时间，不能以更高点击数抵消误触。

> 2026-09-15 后续产品决策：有限系统外跳的验收线调整为约 2%，详细安全约束见 [`generation-pingpong-remediation-todo-20260915.md`](generation-pingpong-remediation-todo-20260915.md)。

最终门槛：零崩溃/ANR；最大无事件间隔小于 30 秒；坐标 results/actions 为 100%；已知广告层级失败随机坐标为 0；坐标外跳率不高于约 2%；相同 seed/Provider 下 Shortswave 事件吞吐相对基线下降不超过 10%。游戏场景推进率在没有可靠 before/after 渲染证据前标记为“未资格”，不得伪造通过。

## 暂不实施

- 不引入 OCR、OpenCV、模型、云端识别或主机 Sidecar。
- 不实现完整 SurfaceFlinger + 截图差分闭环；本次数据先验证轻量反馈是否足以把外跳率压到 1% 以下。
- 不按 Shortswave 或西瓜游戏包名写特例。
