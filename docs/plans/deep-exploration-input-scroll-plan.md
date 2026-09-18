# 深度探索输入与多屏滚动实施计划

日期：2026-09-10。

## 目标

基于 [`deep-exploration-input-scroll-audit-20260910.md`](../audits/deep-exploration-input-scroll-audit-20260910.md) 的真机和代码审计结论，补齐 Go 深度探索器与 Java Monkey Runner 的安全文本输入、多屏滚动、公平调度和可观测指标，并在 Foloy 真机上完成回归。

## 约束

- 密码、PIN、OTP、支付、银行卡、账号等敏感字段不得自动输入。
- 默认输入仅使用固定测试探针，不使用真实个人信息，也不自动提交表单。
- 中文、符号和 Emoji 必须走 scrcpy UTF-8 clipboard/paste；不得以 `adb shell input text` 作为正式实现。
- 滚动必须有动作预算、进展/停滞判断，并继续受全局步数、反 Ping-Pong 和安全策略约束。
- Go 与 Java 两条探索链路保持相同的动作语义和统计口径。

## Plan

1. 扩展层级模型，识别安全可编辑字段并生成有限输入语料动作。
2. 将现有 scrcpy UTF-8 注入能力接入 Go 探索器；为 Java Runner 建立运行期令牌保护的本机注入桥接。
3. 将点击优先的静态调度改为有界公平调度；同一稳定场景允许有限次重复滚动。
4. 增加输入、滚动尝试、滚动进展和滚动停滞指标，并同步公共报告。
5. 补齐 Go 单元/集成测试与 Java 自检，再执行仓库级验证。
6. 构建、部署到已连接设备，使用 Foloy Search 与首页/列表执行真机验收。

## Todo

- [x] T1：Go 层级分析识别非敏感 EditText，生成 ASCII、中文、符号、Emoji 四类输入动作。
- [x] T2：探针文本在指纹中归一化；同一 Activity/字段/语料不因页面文本变化被重复执行。
- [x] T3：Go Manager 接入 scrcpy TextInjector，实现聚焦、清空、UTF-8 输入和结构化计数。
- [x] T4：Go 调度实现“最多两个非滚动动作后给滚动机会”，同一稳定场景允许最多三次受控滚动。
- [x] T5：过滤过矮伪滚动区域，保留主要可滚动 viewport。
- [x] T6：增加 inputs、inputFields、scrollAttempts、scrollProgress、scrollStalls 指标并同步 evaluation report。
- [x] T7：Java NodeExplorer 生成相同输入语料动作，并通过运行期本机桥接调用 scrcpy 注入。
- [x] T8：Java NodeExplorer 同步公平滚动、有限重复滚动和事件指标；Agent Runner 解析新事件。
- [x] T9：新增输入安全、Unicode、探针指纹、滚动公平、多次滚动和指标测试；扩展 Java self-test。
- [x] V1：执行 gofmt、全部 Go tests、go vet、Runner 构建与 self-test。
- [x] V2：构建并部署 Agent/Runner 到 `R5CN30EQKNM`。
- [x] V3：Foloy Search 真机验证 ASCII、中文、符号、Emoji 输入；密码/敏感字段保护由自动化测试覆盖。
- [x] V4：Foloy 可滚动页面真机验证有限步内发生滚动，并记录滚动进展/停滞与反 Ping-Pong 指标。
- [x] D1：更新审计文档、Todo 状态和真机验证证据。

完成证据见 [`validation-deep-exploration-input-scroll-20260910.md`](../validations/runs/validation-deep-exploration-input-scroll-20260910.md)。

## 验收门槛

1. Foloy Search 的 EditText 不再只有 tap，至少生成四类受控输入动作；真机层级可读回对应 UTF-8 文本。
2. 同一字段的同类语料在一个探索会话内最多执行一次，探针值不制造无限新场景。
3. 页面同时存在点击和滚动时，至多两个普通动作后滚动获得执行机会。
4. 稳定指纹页面允许继续滚动，但最多三次；无进展不会形成无限 swipe。
5. 报告能区分输入次数/字段数、滚动尝试、有效进展和停滞。
6. 两条引擎通过自动测试；真机运行无购买、支付、密码或隐私数据输入。
