# Foloy 真实场景探索验证记录

日期：2026-09-09

## 结论

Foloy 仅作为真实应用样本。本轮新增和调整均基于 Android、Accessibility、Compose、WebView 与图探索的通用行为，未在生产代码中加入 Foloy 包名、文案、坐标、设备型号或 Android 版本特判。

Android 13、Android 15 与 Android 16 已完成首启、条款、Onboarding、动态列表、系统权限、付费墙、法律 WebView、Google Play 购买确认、外部系统页面、稳定性以及 AutoPopup 伴随执行验证。

## 实景结果

| 场景 | requestId | 结果 |
|---|---|---|
| Android 13 完整首启 | `foloy-s1-s3-a13-dynamic-r4` | 40 步达到上限；21 状态、35 边、5 Activity、5 次特殊动作、2 次滚动、5 次回退；最终停因 `max_steps` |
| Android 13 权限 | `foloy-s2-a13-permission` | 识别三星 Permission Controller，点击 `While using the app`；返回 Foloy 后继续，4 步达到上限 |
| Android 16 购买保护 | `foloy-s5-a16-purchase-guard` | Google Play 最终确认页第一步执行 `purchase_confirmation/dismiss`；无 `Subscribe`、购买、试用提交 |
| Android 16 稳定性 | `foloy-s7-a16-r1` | 30 步达到上限；15 状态、30 边、12 次 DFS 回退；无越界停止 |
| Android 13 稳定性 | `foloy-s7-a13-r1` | 40 步达到上限；32 状态、40 边、2 次 DFS 回退；无越界停止 |
| Android 15 完整首启 | `foloy-s7-a15-complete-r1` | 首轮真实触发并允许系统权限；进入媒体选择器时暴露安全停止边界 |
| Android 15 系统页面恢复 | `foloy-s7-a15-picker-recovery`、`foloy-s7-a15-system-recovery-r2`、`foloy-s7-a15-resolver-recovery-r3` | Photo Picker、系统 Settings 和 Android Resolver 均被逐项复现并改为有证据的安全返回 |
| Android 15 最终稳定性 | `foloy-s7-a15-final-r4` | 40 步达到上限；16 状态、39 边、6 Activity、2 次 DFS 回退；无越界停止 |
| AutoPopup 伴随探索 | `foloy-s8-a13-popup-coexist` | AutoPopup 运行时探索请求被接受并完成；停止后恢复原配置 `{}` |

购买确认页由人工只推进到最终确认边界，页面显示测试卡；自动探索没有点击最终订阅按钮。Android 13 相机入口触发的权限弹窗中，`CAMERA` 在触发前为未授权，探索器按默认允许策略选择前台使用期间允许。

## 发现并修复的通用问题

1. 系统层级转储偶发报告成功但结果文件缺失：转储文件改为进程和时间唯一命名，缺失时有界重试并返回明确错误。
2. 法律文档正文出现 advertising 等词时误判为广告：只有同时存在安全关闭目标时才归类为广告。
3. 启动动画/过渡帧空树过早导致图穷尽：增加有上限的空捕获容忍。
4. DFS 回退可能退出到桌面：仅当引擎自己的待处理动作明确为 Backtrack 时重新拉起目标包；任意其他跨包仍安全停止。
5. 特殊场景跳转后可能对异步半成品页面建图：特殊动作和受控拉起后，普通图探索等待过渡窗口结束，期间仍可处理系统/特殊弹窗。
6. Compose/RecyclerView 懒加载在首次滚动后补充节点：已知动作耗尽时进行有界多帧复核，指纹变化后继续建图，连续稳定才结束。
7. Android Photo Picker、系统 Settings 和 Resolver/Chooser 原先被视为未知跨包：仅对白名单系统包或具有明确 Resolver 文案的页面执行返回，不选择用户媒体、不修改系统设置。
8. Resolver 返回动画会短暂保留 `android` 前台包但丢失文案：只有上一帧已明确识别的同一控制器可在 5 秒过渡窗口内继续等待，其他 `android` 页面仍安全停止。

## 边界对齐

- AutoPopup 仍是原 XTest 的四字段配置：`autoClickByText`、`autoClickByResourceId`、`autoInputByHint`、`autoInputByResourceId`。
- `mainContentContains` 与显式目标语义保持不变；AutoPopup 可与 Runner/探索并行。
- WebView 根容器不作为点击目标，内部可访问节点仍参与探索。
- 默认条款允许、权限允许、广告关闭、付费墙安全探索；购买确认始终返回。

## 回归结果

- Agent 全量 Go 测试：通过。
- HTTP 契约：目标 77、实现 77、覆盖 77、缺失 0、文档差异 0。
- Android 13 最新实景首启：由修复前 6 步 `graph_exhausted` 改善为 40 步 `max_steps`，动态列表和后续业务页均被探索。
- Android 15 最新稳定性：由连续暴露 Photo Picker、Settings、Resolver 三个跨包边界，收敛为 40 步 `max_steps`。

## 收尾状态

- 三台设备的 Nova Agent 与 UiAutomator 临时进程已停止。
- UiAutomator host/test 测试包、明确的临时运行文件及 17912/27912/37912 端口转发已清理。
- 三台设备均保留 Foloy；Android 15 上原有的 `xtest-nexus-reference` 未改动。
- 无功能性未完成项。
