# 深度探索输入与滚动审计（2026-09-10）

## 结论

本次在 Samsung SM-G9860（Android 13，设备 `R5CN30EQKNM`）和 Foloy（`tcg.scanner.value.app`）上复现了两个覆盖缺口：

1. 探索器可以识别并点击 `android.widget.EditText`，但不会生成或执行文本输入动作。
2. 探索器可以识别可滚动容器，但点击动作的固定高优先级会使滚动长期饥饿；即使执行到滚动，同一稳定场景内的同一容器也只能向前滚动一次。

这两个问题同时存在于 Go 深度探索器和 Monkey Runner 内嵌的 Java `NodeExplorer` 中。它们不是设备能力问题，也不是简单增加运行时长可以可靠解决的问题。

## 真机证据

### Search 输入框

Foloy `SearchActivity` 的预览结果包含两个动作：返回按钮 `tap` 和 `android.widget.EditText` 的 `tap`，没有 `text/input` 动作。

使用固定种子执行 1 步后：

- 执行动作：`tap`，坐标 `(630, 177)`，控件类型 `android.widget.EditText`。
- 输入框状态：`focused=true`、`password=false`、`text=""`。
- 最终统计：`steps=1`、`scrolls=0`，停止原因 `max_steps`。

设备已安装三星/搜狗输入法、Appium UnicodeIME、Utf7Ime 和 AdbIME；项目的录制回放模块也已有基于 scrcpy clipboard/paste 的 UTF-8 `InjectText`。因此缺口在探索模型和调度链路，而不是设备无法输入中文或 Emoji。

### 首页滚动

Foloy `MainActivity` 的预览结果包含 13 个动作，其中 2 个是 `swipe-forward`，包括覆盖主要页面的 `[0,0][1080,2285]` 可滚动区域。

启用 `enableScroll=true`、固定种子执行 8 步后：

- `steps=8`
- `discoveredStates=5`
- `observedActivities=2`
- `scrolls=0`
- 8 个动作全部为 `tap`

这证明层级分析已经发现滚动能力，但选择器优先执行普通点击；点击引发状态或 Activity 跳转后，原页面的滚动候选没有得到公平执行机会。

## 代码审计发现

### P1：Go 探索器没有文本输入动作链路

- `exploration.Action` 虽然有 `Text` 字段，但 `Analyze` 只为 clickable 节点创建 `tap`，为 scrollable 节点创建 `swipe`。
- XML 模型没有采集 `focused`、`focusable`、`long-clickable` 或输入类型等字段，无法区分普通可点击节点与可编辑字段。
- `executeAction` 只支持 `tap`、`swipe`、`back`。
- `exploration.Manager` 只依赖 shell executor，没有接入已有的 scrcpy 文本控制器。

结果是 EditText 最多被聚焦，不可能覆盖 ASCII、中文、符号、空白、边界长度或 Emoji 输入。

### P1：Java NodeExplorer 同样没有文本输入动作

- `SceneSnapshot.parse` 只收集 `clicks`、`longClicks`、`scrolls`。
- 动作列表只构造 `tap`、`long`、`scroll`。
- `execute` 和 `ShellInput` 都没有文本输入分支。

因此 Monkey Runner 与 Go 引擎行为不一致的问题尚未出现，是因为二者目前都缺少输入能力。若只修一边，会立刻形成双引擎覆盖差异。

### P1：滚动被普通点击固定压后

Go `actionPriority` 将恢复动作设为 0、普通动作设为 1、滚动设为 2。只要当前场景仍有未尝试点击，滚动就不会被选中。

Java `SceneSnapshot` 更直接地把动作静态拼接为：全部点击、全部长按、最后全部滚动。任何前置点击导致的页面跳转、动态刷新或场景指纹变化，都可能让滚动永远无法执行。

这不是随机种子问题：种子只改变同优先级动作的次序，不能让滚动越过普通点击。

### P1：同一稳定页面只允许滚动一次

Go 和 Java 都把 `scroll(container)` 当作普通的一次性动作。动作 ID 不包含滚动游标或已展示内容，执行后即进入 `tried`。

若列表滚动后层级结构、稳定文本和边界没有产生新指纹（RecyclerView 复用、动态数字被归一化、WebView/Compose 语义稀疏时很常见），探索器不会生成第二次向前滚动。多屏内容因此仍无法完整探索。

### P1：缺少滚动进度与终点判定

当前只统计发出了多少次 swipe，没有比较滚动前后的内容窗口，也没有区分：

- 滚动成功并出现新内容；
- 手势落在错误容器；
- 已到列表底部；
- 页面拦截或没有移动；
- 键盘出现导致 viewport 改变。

Foloy 首页同时暴露全屏和底部区域两个 scrollable 节点，也说明还缺少嵌套容器去重、有效高度过滤和主滚动 viewport 选择。

### P2：已有 UTF-8 输入能力没有复用

录制回放模块已有 `scrcpy.Manager.InjectText`，通过 clipboard/paste 注入 UTF-8，并已覆盖 `"杭州 😀"` 的单元测试。但探索器仍使用仅支持 shell tap/swipe/back 的执行接口。

直接改用 `adb shell input text` 只能覆盖有限字符，对中文、Emoji、空格和部分符号不可靠，不应成为正式方案。

### P2：报告无法判断输入和有效滚动覆盖

当前报告有 taps/longPresses/scrolls，但没有：

- 发现的输入框数、已覆盖输入框数；
- 输入用例数、字符类别、注入失败数；
- scroll attempts、scroll progressed、scroll stalled、end reached；
- 单个容器的最大滚动深度与新内容增量。

因此即使未来增加动作，也无法从报告判断是否真正产生了输入效果或发现了折叠在下方的新控件。

## 建议的修复设计

### 1. 将文本输入建模为一等动作

新增 `input` 动作，至少携带字段稳定身份、语料类别、文本值和是否需要聚焦/清空。识别条件优先使用 `EditText`/可编辑语义，继续强制排除 `password=true`，并默认排除 PIN、OTP、支付、账号和隐私敏感字段。

建议安全、可控的默认语料预算：

- ASCII：`Nova123`
- 中文：`测试`
- 符号：`!@#_-`
- 空白/边界：按字段约束有限启用
- Emoji：`😀`，仅在 UTF-8 注入能力可用时启用

每个字段、每类语料只尝试一次。默认只输入并观察，不自动提交；搜索提交、IME action 和清空应作为独立、受预算和安全规则控制的动作。

将已注入的探针文本在场景指纹中归一为统一占位符，否则不同语料会不断制造“新场景”并重新获得输入预算。

### 2. 复用 scrcpy UTF-8 注入能力

为 Go 探索器注入一个窄接口，例如 `TextInjector.InjectText(context.Context, string)`，复用现有 scrcpy 实现。执行序列应为：聚焦、全选/清空、UTF-8 注入、等待稳定、读取层级验证实际文本或状态变化。

Java Runner 不应另造一套低质量 `input text`。优先方案是让 Runner 复用 Agent 的统一探索执行服务；过渡方案可以提供仅本机、短期令牌保护的文本注入桥接。最终应收敛为一个动作模型和一套调度器。

### 3. 用阶段化公平调度替代“点击全部结束才滚动”

对每个页面 viewport 采用阶段或配额调度，例如：

1. 处理安全恢复动作；
2. 探索顶部 2～3 个高价值控件；
3. 若存在未探测的主滚动容器，执行一次向前滚动；
4. 优先探索新出现的控件；
5. 重复滚动，直到终点、连续无进展或达到预算。

滚动不能靠提高随机概率解决；它需要防饥饿保证，例如“未滚动页面最多连续执行 3 个普通动作后必须给滚动一次机会”。

### 4. 为滚动建立游标和进度模型

以 `container identity + direction + visible-content fingerprint` 形成滚动动作身份。每次滚动后比较前后可见内容：

- 有新内容：推进游标，允许下一次滚动；
- 连续 2 次无变化：标记 `end_reached` 或 `stalled`；
- Activity/页面跳转：记录普通转移，不误判为滚动进度；
- 回退需要恢复父 viewport 时，使用校验后的反向滚动或图路径，而不是无条件反向 swipe。

默认可设置每个容器最多 8～12 次向前滚动、连续 2 次无进展停止，并由全局步数预算兜底。

### 5. 选择真正可滚动的主容器

过滤过矮区域和底部导航栏，合并同轴嵌套 scrollable，优先选择可见高度足够且覆盖主要内容的最深有效容器。手势起止点应避开状态栏、导航栏、固定底栏和系统 gesture inset。

### 6. 补齐回归测试与真机验收

最低测试集：

- EditText 生成输入动作，密码/PIN/OTP 不生成；
- ASCII、中文、符号和 Emoji 经 UTF-8 注入后可从层级读回；
- 探针文本不会制造无限新状态；
- 有点击和滚动同时存在时，滚动在有限动作内必定执行；
- RecyclerView 结构稳定时可连续滚动多屏；
- 到底部后两次无进展停止；
- 嵌套/伪 scrollable 容器选择正确；
- Foloy Search 输入覆盖和首页/列表多屏滚动作为真机回归场景。

## 推荐实施顺序

1. 先统一 Go 动作模型、文本注入接口、滚动游标和指标。
2. 为 Go 引擎补齐单元/集成测试并在 Foloy 真机验证。
3. 让 Java Runner 复用统一能力，至少保证动作语义、预算和报告一致。
4. 最后做 10～20 分钟长稳探索，联合验证输入覆盖、滚动深度和反 Ping-Pong 指标。

## 整改状态

2026-09-10 已按 [`deep-exploration-input-scroll-plan.md`](../plans/deep-exploration-input-scroll-plan.md) 完成代码整改和 Foloy 真机回归。实现结果与证据见 [`validation-deep-exploration-input-scroll-20260910.md`](../validations/runs/validation-deep-exploration-input-scroll-20260910.md)。
