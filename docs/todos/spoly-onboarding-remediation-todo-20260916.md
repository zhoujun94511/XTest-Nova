# Spoly 多页 Onboarding 整改 Todo

日期：2026-09-16

## P0：页面级前进动作

- [x] 为 onboarding 前进动作建立 `Activity + semanticScene + action` 页面级执行身份。
- [x] 同一页面继续防重复，不同页面复用相同 `Continue` 时释放动作。
- [x] 普通 generation 继承页面历史，recovery generation 释放直接动作预算。
- [x] 保持普通动作的 `cycleScene` 去重和全局输入去重不变。

## P0：耗尽恢复隔离

- [x] NodeExplorer 暴露“当前耗尽场景含 onboarding 前进动作”的结构化状态。
- [x] 前进动作后的同 Activity 空动作过渡帧有界等待，超时按 `transition_timeout` 原地停止。
- [x] Main 在该状态下输出 `onboarding_exhaustion_guard` 并以
  `onboarding_exhausted` 结束，不执行 Back、monkey launch 或 force-stop。
- [x] 保持 Unity、纯渲染、普通 DFS 和外部页面恢复路径不变。

## P1：可观测性

- [x] Agent 将 `onboarding_exhausted` 识别为终态。
- [x] ExplorationMetrics 增加 onboarding 耗尽计数并补解析测试。
- [x] API 文档说明 `completed` 与业务到达不同，新增可恢复停止原因。

## P0：测试与交付

- [x] Runner self-test 覆盖五页同 Activity/同按钮/同坐标的 Compose Pager。
- [x] 测试普通继承阻止同页重复、恢复继承允许从首屏重新前进。
- [x] 测试 onboarding 空过渡帧不会触发 DFS Back，等待耗尽后原地停止。
- [x] Runner 构建、自测、Agent 全量测试和根目录发布构建通过。
- [x] Android 15 当天首装、未完成 onboarding 的现场状态进入业务页面；无重拉，Crash/ANR/Native Crash 为 0。
- [x] Android 16 同版本隔离回归进入业务页面；无重拉，Crash/ANR/Native Crash 为 0。
- [x] 最终发布物已部署到 Android 15/16，设备端健康检查通过。
- [x] 记录构建哈希、事件摘要和未通过项；只有实际通过的 Todo 才勾选。
- [x] 清除 Spoly 数据后，用最终哈希发布物再次执行严格首装回归。
