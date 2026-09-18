# 深度探索反 Ping-Pong 验证记录

日期：2026-09-10

设备：Samsung SM-G9860，Android 13 / API 33，序列号 `R5CN30EQKNM`。

目标应用：Foloy，包名 `tcg.scanner.value.app`。验证过程没有清除应用数据。

## 复现与修正

第一轮 Go 探索在前 10 步稳定复现 `SplashActivity → Privacy WebView → SplashActivity` 多次往返。精确状态因 WebView 加载内容变化产生不同指纹，初版内容语义检测没有识别该环。由此将状态身份拆分为精确状态、内容语义状态和粗粒度导航循环状态，并在确认周期后熔断整个周期，而非只关闭最后一条边。

真机同时暴露了采集一致性问题：一次层级采集期间 Activity 已从 Splash 切换到 Main，旧逻辑会形成“Splash Activity + Main 页面节点”的混合状态。现已增加采集前后 Activity 双读门禁。

## Go 探索验证

循环专项 `pingpong-fix-go-a13-r2-20260910`：

- 19 步、11 个精确状态、19 条边；
- 检测 1 个重复周期；
- 熔断 3 条边；
- 识别并熔断一次未返回父状态的 BACK；
- 最终以 `state_exhausted` 自然收敛，没有消耗完 30 步预算。

最终发布烟测 `pingpong-fix-release-smoke-a13-20260910`：

- 12 步、8 个状态、12 条边、2 个 Activity；
- 1 次正常滚动；
- `cycleDetections=0`、`blockedEdges=0`；
- 按 `max_steps` 正常结束，未误判正常前向探索。

另一次最终长路径运行在 15 步后识别一次 BACK 目标不匹配并熔断，随后以 `state_exhausted` 收敛。

## Runner 验证

最终二进制运行 `pingpong-fix-release-runner-a13-20260910`：

- 90 秒配置，实际 11 个节点动作；
- 3 个状态、10 条转移、1 次已知路径回放；
- `hierarchyFallback=0`、`fallbackActions=0`；
- 没有无条件 DFS BACK；
- 日志中的 Activity 与页面节点一致，没有再次出现 Splash/Main 混合场景；
- 本次自然路径未达到三次重复阈值，因此 `cycleDetections=0`，符合避免误报的设计。

完整 Runner 证据保存在 [`tests/reports/pingpong-fix-20260910/runner-final`](../../../tests/reports/pingpong-fix-20260910/runner-final)。

## 自动化结果

- `go test ./agent/...`：通过；
- Runner Java 8 编译：通过；
- `NodeExplorer --self-test`：通过；
- 自检覆盖二环、三环、物理指纹漂移、跨 Activity 内容漂移和间歇重复转移；
- ARM64 Agent 与 Runner 已重新构建、校验并部署，健康检查通过。

