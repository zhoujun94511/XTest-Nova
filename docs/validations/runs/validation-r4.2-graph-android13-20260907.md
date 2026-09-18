# R4.2 场景动作图 Android 13 验证

日期：2026-09-07  
设备：Samsung SM-G9860，Android 13 / API 33  
目标：`com.xtest.nova.fixture`  
配置：20 秒、300 ms、固定种子 `20260907`

## 结果

- Runner JVM 自测通过，包括动态文本/坐标不改变稳定场景 ID，以及已知图路径首步选择。
- 运行正常完成，二进制 Manifest 声明 2 个 Activity，覆盖 2/2（100%）。
- 发现 3 个场景、6 条有向转移边。
- 执行 3 次点击、1 次长按、2 次滚动、1 次回退。
- 层级获取降级 0 次；本次没有触发已知路径回放。
- 事件链包含主界面到详情页、详情页长按自环、滚动产生新场景、BACK 返回主界面。

原始证据保存在本机被 `.gitignore` 排除的 `tests/reports/android13-graph-20260907/`，不会混入首次源码提交。
安全 HTTP 黄金执行器在同一 Nova 端点完成 19 个夹具的采集自校验，差异为 0。证据 SHA-256：

- `events.jsonl`: `17D24B9D094A80CE9F669DF88AEEB1BFBF6BB2D5D32BB5D226749A866C704602`
- `exploration_graph.json`: `22C8495AAC8E08B222E31B13670648D363DB3D690E8F3DE81DAA72959EF4D5F8`
- `activity_coverage.json`: `59F53E9C4DEEDC754E58866D091E6B31196A23470F7BEF6AC52C224106410033`
- `contract-golden-selfcheck.json`: `47416BF61F356121D21FB0FECCF47D3AB7C91B61BA3F6349B70B6147ECB7509C`

## 资格边界

本次证明场景指纹、动作执行、转移采集、摘要产物和 Activity 分母链路有效，但不能证明：

- Nexus 与 Nova 的行为等价；
- 非栈式导航下的已知图路径回放已经过真机验证；
- Compose、SurfaceView、游戏或 Android 14/15 设备适配。
