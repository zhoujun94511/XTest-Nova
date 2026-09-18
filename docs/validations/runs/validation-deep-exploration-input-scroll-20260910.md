# 深度探索输入与滚动整改验证（2026-09-10）

## 环境

- 设备：Samsung SM-G9860，Android 13，序列号 `R5CN30EQKNM`
- 应用：Foloy，包名 `tcg.scanner.value.app`
- Agent：`xtest-nexus-0.22.0-m5.9-compat`
- 层级提供方：system

测试未清除 Foloy 数据，未触发购买、支付、密码或个人信息输入。

## 自动化验证

- `go test ./...`：通过。
- `go vet ./...`：通过。
- Runner Java 8 编译、D8 构建和 `NodeExplorer --self-test`：通过。
- 新增测试覆盖：EditText 语料生成、密码/OTP 排除、输入框宽度变化身份稳定、探针指纹归一、UTF-8 注入、输入转移不污染循环检测、滚动公平、稳定场景三次上限、伪滚动区域过滤和新指标解析。

## Go 深度探索真机结果

### Search 输入

固定 4 步执行结果：

- `inputs=4`
- `inputFields=1`
- `cycleDetections=0`
- 输入类别：ASCII `Nova123`、符号 `!@#_-`、Emoji `😀`、中文 `测试`
- 最终层级读回：`android.widget.EditText text="测试" focused="true" password="false"`

输入框在出现清除按钮后右边界从 990 缩小到 895，但字段仍只统计一次，四类语料 ID 没有因宽度变化重新获得预算。

### 滚动

固定 12 步执行结果：

- `scrolls=2`
- `scrollAttempts=2`
- `scrollProgress=2`
- `scrollStalls=0`
- `discoveredStates=7`
- `observedActivities=2`
- `cycleDetections=2`、`blockedEdges=2`

与整改前同类 8 步全部点击、`scrolls=0` 相比，滚动已在有限动作内获得执行机会；两个滚动均产生新场景，同时原有反 Ping-Pong 仍能封禁重复点击自环。

Foloy 首页原先同时暴露全屏和底部区域两个 scrollable 候选；整改后过矮底部伪滚动区域被过滤，仅保留主 viewport。

## Java Monkey Runner 真机结果

### Search 输入

35 秒运行结果：

- `inputs=4`
- `taps=2`
- `hierarchyFallback=0`
- `fallbackActions=0`
- `cycleDetections=0`
- 四条 `node_input` 事件分别为 ascii、cjk、symbols、emoji
- 无 `action_failed`

Runner 通过每次运行随机生成、仅运行期间有效的控制令牌调用 Agent 本机桥接，由 scrcpy 完成 UTF-8 注入。令牌不写入 manifest，Runner 停止后立即失效。

证据目录：`tests/reports/deep-exploration-input-scroll-20260910/java-input`。

### 滚动闭环

65 秒运行结果：

- `scrolls=1`
- `scrollAttempts=1`
- `scrollStalls=1`
- `inputs=4`
- `hierarchyFallback=0`
- `fallbackActions=0`
- `cycleDetections=0`

Runner 发出 `node_scroll` 后再次采集同一 scene，形成明确 `scroll_stall`，证明报告现在能够区分“发出手势”和“手势是否带来内容进展”。稳定场景的滚动重试最多三次，由 self-test 验证预算上限。

证据目录：`tests/reports/deep-exploration-input-scroll-20260910/java-scroll`。

## 验收结论

计划 Todo 全部完成。Go 与 Java 两条探索链路现在都具备：

1. 安全、有界、可追踪的 ASCII/中文/符号/Emoji 输入；
2. 密码和敏感字段隔离；
3. 不被普通点击永久饿死的滚动调度；
4. 稳定页面的有限重复滚动；
5. 滚动进展/停滞与输入覆盖指标；
6. 与现有反 Ping-Pong 机制兼容的独立输入预算。

