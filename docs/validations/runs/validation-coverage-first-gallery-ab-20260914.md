# 覆盖率优先探索 Gallery A/B 真机验证

日期：2026-09-14（Asia/Shanghai）

## 目标与口径

对 Gallery 使用相同随机种子 `20260914`、最大 1000 步和最大 200 次回溯执行两组策略：

- A（旧策略代理）：system dump、严格二次观测、每输入框 18 个用例、750 ms 动作间隔。
- B（新策略）：Nova Provider、快速前台新鲜度校验、每输入框 1 个正常值、250 ms 动作间隔。

20 分钟是单次上限，不强行让已耗尽状态图的任务空转，因此以下以动作/分钟和新状态/分钟比较。有效动作率按 `有效/(有效+无效+跨应用)` 计算；Activity 数是实际观测值，不是 manifest 覆盖率。

## 探索结果

| 设备 | 组别 | 运行时间 | 首次动作 | 步骤 | 状态 | Activity | 动作/分钟 | 状态/分钟 | Activity/分钟 | 有效动作率 | 跨应用率 | 结束原因 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Android 13 `R5CN30EQKNM` | A | 158.63 s | 4.790 s | 26 | 8 | 1 | 9.83 | 3.03 | 0.38 | 48.00% | 4.00% | `state_exhausted` |
| Android 13 `R5CN30EQKNM` | B | 27.82 s | 0.284 s | 34 | 16 | 1 | 73.33 | 34.51 | 2.16 | 52.94% | 2.94% | `state_exhausted` |
| Android 15 `R3CY80B2G4W` | A | 433.23 s | 4.189 s | 81 | 34 | 1 | 11.22 | 4.71 | 0.14 | 73.42% | 6.33% | `state_exhausted` |
| Android 15 `R3CY80B2G4W` | B | 51.24 s | 0.116 s | 103 | 66 | 2 | 120.61 | 77.28 | 2.34 | 88.89% | 2.02% | `state_exhausted` |
| Android 16 `83fc400c` | A | 61.55 s | 4.784 s | 11 | 4 | 2 | 10.72 | 3.90 | 1.95 | 70.00% | 0.00% | `special_unhandled`，停在系统权限页 |
| Android 16 `83fc400c` | B（修复后） | 53.34 s | 0.324 s | 91 | 41 | 6 | 102.36 | 46.12 | 6.75 | 57.14% | 3.30% | `state_exhausted` |

Android 13 新策略的动作吞吐提升 7.46 倍、状态发现速率提升 11.39 倍。Android 15 分别提升 10.75 倍和 16.41 倍，同时有效动作率从 73.42% 提高到 88.89%。Android 16 分别提升 9.55 倍和 11.83 倍；但旧组被系统权限页提前截断，其覆盖数字仅可作为保守参考，不是完整耗尽基线。

全部跨应用率都低于 10%。Android 15 新策略有效动作率达到 88.89%；Android 13、16 分别为 52.94% 和 57.14%，仍未达到计划中的 60% 目标。三机吞吐和状态发现门禁通过，但有效动作率门禁只在 Android 15 通过，不能据此宣称全部质量门禁完成。

## 同页 Provider 延迟

两台设备均重新启动到 Gallery 首页；保持页面不变，Nova 与 system 各采样 20 次：

| 设备 | Provider | 平均 | P50 | P95 | 最小 | 最大 |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| Android 13 | Nova | 32.97 ms | 26.63 ms | 41.89 ms | 17.31 ms | 158.28 ms |
| Android 13 | system | 2193.70 ms | 2199.19 ms | 2223.29 ms | 2142.95 ms | 2225.45 ms |
| Android 15 | Nova | 21.62 ms | 15.38 ms | 42.71 ms | 6.35 ms | 104.17 ms |
| Android 15 | system | 2229.62 ms | 2039.92 ms | 2818.59 ms | 2014.18 ms | 3125.03 ms |
| Android 16 | Nova | 21.35 ms | 14.57 ms | 31.96 ms | 10.64 ms | 131.32 ms |
| Android 16 | system | 2054.37 ms | 2051.43 ms | 2072.16 ms | 2033.40 ms | 2074.79 ms |

Android 13/15/16 上 Nova 的 P95 分别快 53.1、66.0 和 64.8 倍，均远低于 1 秒门禁。测试结束后三台 Agent 均已复核：健康接口为 `ok`、`lastSource=nova-provider`、`fallbacks=0`。

## 测试发现并修复的竞态缺陷

Android 16 首轮 B 组在 45 步后遇到 Activity 切换。新鲜度检查正确拒绝了旧动作，但旧实现使用 `len(successfulSteps)+1` 生成 `stepId`：被拒绝的动作不进入成功步骤，下一次选择便复用相同 `stepId`，回执存储将其视为失败请求重试并错误终止任务。

修复后使用独立的动作尝试序号生成 `stepId`。回归测试覆盖“第一次陈旧拒绝、第二次稳定执行”的连续场景；Android 16 真机复跑产生 3 次 `staleActions`，对应成功步骤最后为 91、最后回执编号为 `step-000094`，证明三个拒绝编号没有被复用，任务继续运行直至自然耗尽。

## 产物

- Android 13 A：`/sdcard/xtest-nova/com.sec.android.gallery3d/Exploration/20260914_092307`
- Android 13 B：`/sdcard/xtest-nova/com.sec.android.gallery3d/Exploration/20260914_092442`
- Android 15 A：`/sdcard/xtest-nova/com.sec.android.gallery3d/Exploration/20260914_095112`
- Android 15 B：`/sdcard/xtest-nova/com.sec.android.gallery3d/Exploration/20260914_095314`
- Android 16 A：`/sdcard/xtest-nova/com.miui.gallery/Exploration/20260914_092237`
- Android 16 B 首轮（用于复现缺陷）：`/sdcard/xtest-nova/com.miui.gallery/Exploration/20260914_092437`
- Android 16 B 修复后：`/sdcard/xtest-nova/com.miui.gallery/Exploration/20260914_093151`

每个目录均保留 `steps.json`、`receipts.json`、`graph.json` 和 `evidence.json`。

## 限制与结论

Android 13/15/16 三系统 G1 已全部完成。三台设备的结果确认此前“规则增多导致吞吐下降”的判断成立，也确认本轮 Nova 快速观测与覆盖调度显著扭转了该问题。下一步不是恢复严格双采集，而是提高低收益动作识别，争取在维持当前状态发现速率的前提下让 Android 13/16 的有效动作率稳定超过 60%。
