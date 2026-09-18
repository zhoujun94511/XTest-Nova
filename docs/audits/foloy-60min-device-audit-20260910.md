# Foloy 60 分钟真机深度探索审计

## 结论

2026-09-10 在 Samsung SM-G9860（Android 13，序列号 `R5CN30EQKNM`）上，以 `tcg.scanner.value.app` 为目标执行了 60.21 分钟探索。前 34.07 分钟为首段，因 Java Runner 进入外部包而提前结束；随后补跑 26.15 分钟，并将 Java 阶段改为会话结束后自动恢复目标应用、继续下一轮，达到完整时间预算。

本次验证确认输入与滚动能力已经真实生效，但 Java Runner 仍存在跨场景、多轮 Ping-Pong 漏检；外部 Activity 恢复和运行中指标也需要优先完善。Foloy 未观察到崩溃或 ANR，退出记录均为测试编排主动 `force-stop`。

## 汇总数据

| 引擎 | 会话 | 动作/步骤 | 状态 | 输入 | 滚动 | 有效滚动 | 循环检测 | 阻断边 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Go Explorer | 41 | 408 | 231 | 69 | 46 | 39 | 36 | 55 |
| Java Runner | 6 | 171 | 49 | 25 | 24 | 0 | 0 | 0 |

输入探针在两套引擎均得到覆盖：

- Go：ASCII 17、中文 18、符号 17、Emoji 17。
- Java：ASCII 7、中文 6、符号 6、Emoji 6。

Go 的 41 个会话中，12 个正常 `state_exhausted`，28 个 `safety_stop`，1 个在阶段截止时主动停止。24 个零步骤会话均与外部任务栈残留有关，不是正常探索耗尽。

外部前台分布：三星浏览器 16 次、系统 Photo Picker 7 次、Launcher 2 次、Dialer 1 次、Yellow Pages 1 次；另有 1 次阶段切换时 hierarchy 请求被取消。

Java 的 6 个会话中，5 个因 `external_package` 结束，最后 1 个在总时间预算到期时停止。全部 24 次滚动都被判为 `scroll_stall`，全部会话的 `cycleDetections` 与 `blockedEdges` 都是 0。

## 真机现象

1. 搜索框能够依次接收 ASCII、中文、符号和 Emoji。开始截图也直接记录了搜索框与已拉起的键盘。
2. Go Explorer 能在可滚动页面继续向下探索，46 次尝试中有 39 次被识别为内容推进。
3. Java Runner 实际发出了滑动，但场景身份没有随可见内容变化，24 次全部记为停滞。
4. Java 多次重复如下路径，却从未计为循环：`主界面卡片 -> 详情页 -> 详情页底部控件自环 -> 返回主界面 -> 滚动 -> 另一卡片/同类详情`。
5. 进入浏览器或系统图片选择器后，安全边界会正确终止当前会话；首段测试仅 `force-stop + monkey launch` 无法稳定抢回前台，导致连续零步骤会话。有限退栈并重新从导出 Launcher 入口启动后可以恢复。
6. Java 每轮都点击过一个 bounds 为 `[0,0][0,0]` 的 `android.view.View`，共 6 次，属于无效候选动作。
7. Java 运行期间远端 `events.jsonl` 持续增长，但 `/v1/monkey/runs/current` 的 events 和 exploration 指标一直为 0，只在进程退出后回填。

## 代码审计

### P1：新 cycle scene 会清空循环历史，长环容易永久漏检

`runner/src/main/java/com/openatx/xtest/nova/runner/Main.java:313-317` 在遇到新 `cycleId` 时清空 `recentTransitions` 和 `semanticTransitionCounts`。Foloy 的动态节点、动作集合或滚动内容会持续制造新的 cycle scene，导致 A-B-C-A 或更长路径尚未达到重复阈值就丢失历史。虽然 `recordTransition` 已实现周期 1..N 检测，但上游清空策略使其在真机上没有机会触发，本次 171 个 Java 动作最终为 0 次循环检测。

建议：历史使用固定容量环形窗口，不因发现新场景全量清空；只在明确的新探索分支上降低旧样本权重。循环键至少包含 `{activity/semanticScene, semanticAction, destination}`，对周期 1..4 连续重复 2～3 次进行阻断，并为跨会话热点边保留短期惩罚。

### P1：外部 Activity 只停止，不承担有预算的恢复

Java 在特殊页面处理后的多个分支直接返回 `stop:external_package`；Go 在 `manager.go:489-491` 也直接 `safety_stop`。安全停止是正确边界，但全局测试编排没有恢复策略时，一个外部跳转会使剩余时间失效。

建议：区分会话安全边界和全局探索预算。当前会话仍应停止并落盘；调度层随后执行最多 2～3 次可观测恢复：BACK/关闭已知系统控制器、校验前台包、解析导出的 Launcher Activity 重启。恢复失败后冷却并记为 blocked external edge，禁止立即重放触发动作。不要硬编码 Foloy 的未导出内部 Activity。

### P1：Java 滚动进度使用结构场景 ID，无法感知内容位移

`Main.java:419-421` 仅比较 `previousScene.equals(scene)` 判定滚动是否推进；物理 scene 由 `stableFields()` 构成，而 `Main.java:754` 只包含 index、resource-id、class 和少量布尔属性，不包含可见文本或滚动锚点。列表滚动前后结构相同就必然被判为 stall。本次 24/24 次全部误判，而 Go 同设备同应用为 39/46 次有效。

建议：为滚动单独计算 viewport signature，组合顶部/中部/底部稳定可见锚点、文本语义摘要和容器 scroll offset（若可用）；动作后等待 UI 稳定并比较 viewport signature。连续 2 次无变化才封禁该方向，同时支持反向滚动和每容器独立预算。

### P2：零面积节点仍进入点击候选

`Main.java:640-651` 收集候选时未验证面积和屏幕交集，`Node.eligible()` 也没有 bounds 校验。本次 6 个会话均点击 `[0,0][0,0]` 节点。

建议：候选进入动作池前要求 `right > left`、`bottom > top`、中心点位于有效显示区域，并裁剪到当前 window bounds；无效节点仅用于结构指纹，不生成输入动作。

### P2：运行中 Java 指标不可观测

HTTP 状态只在 Runner 退出并解析日志后回填。长时间运行时，控制端无法判断是正常推进、Ping-Pong 还是卡死，也无法自动基于速率触发恢复。

建议：增量 tail `events.jsonl` 或让 Runner 周期写原子 metrics snapshot；暴露 `lastEventAt`、最近场景/动作、动作速率、cycleDetections、blockedEdges、scrollProgress、externalPackage。若 `lastEventAt` 超过阈值，调度层执行诊断或终止当前会话。

### P2：场景身份仍受 index 影响，语义与物理身份不一致

`Main.java:754` 保留 `index`，动态列表插入和加载占位会制造新物理场景；另一方面正文变化未进入物理 scene，导致滚动内容被合并。当前三套 scene/semantic/cycle 指纹方向正确，但字段职责需要重新划分。

建议：物理 scene 去除裸 index，使用稳定资源 ID、类、可见语义锚点及相对区域；cycle scene 应对数字、时间、计数做归一化，但保留导航语义和动作身份；滚动 viewport 使用独立指纹，不与页面身份共用。

## 实施 Todo

1. **P1 / Java 循环窗口**：删除“发现新 cycle scene 即清空全部历史”，补充真实 A-B-A-B、A-B-C-A、动态指纹漂移、同动作多目标测试。
2. **P1 / 全局恢复器**：在会话调度层实现外部包分类、有限退栈、Launcher 入口解析、恢复校验、冷却与失败预算；触发外部跳转的边进入跨会话封禁表。
3. **P1 / 滚动指纹**：新增 viewport signature 和稳定等待，分别统计 attempt/progress/stall；用 Foloy 主界面、详情列表做真机回归。
4. **P2 / 候选过滤**：过滤零面积、屏外、被遮挡动作；增加 `[0,0][0,0]` 和部分屏外节点单测。
5. **P2 / 实时指标**：运行接口增量更新，增加 `lastEventAt` 与最近循环样本；长测控制端按事件活性判断卡死。
6. **P2 / 指纹收敛**：移除动态 index，加入稳定语义锚点，建立动态列表/骨架屏/异步文本回归集。
7. **验收门槛**：Foloy 连续 60 分钟；外部跳转后 30 秒内恢复或明确耗尽；零面积动作 0；Java 滚动有效率与 Go 差距不超过 15 个百分点；构造的周期 1～4 循环在第三次重复前阻断；运行指标延迟不超过 5 秒。

## 证据位置

- 首段：`tests/reports/foloy-long-audit-20260910-172950`
- 补跑：`tests/reports/foloy-long-audit-20260910-172950/supplemental-26m`
- Java 原始事件：各 `java/**/device-artifacts/events.jsonl`
- Go 每轮状态、步骤和图：各 `go/run-*/state.json`、`steps.json`、`graph.json`
- 崩溃与退出：两段目录内的 `logcat-crash.txt`、`exit-info.txt`
- 真机截图：两段目录内的 `start.png`、`finish.png`

