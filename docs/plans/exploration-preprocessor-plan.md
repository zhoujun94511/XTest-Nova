# 新应用探索前置处理计划

日期：2026-09-09。

## 目标

在智能遍历执行普通页面动作前，先识别并处理首次条款、系统权限、付费墙、广告和临时系统内容。特殊页面必须得到明确结果：安全处理后继续、按显式策略同意，或带原因暂停；不允许静默卡住，也不允许随机点击购买或法律同意入口。

旧 XTest 的 `mainContentContains + clickTarget/resource-id` 上下文匹配、权限 Activity 放行和 WindowClose 用例作为行为参考。内部自动化场景保持其“同意、继续、允许”默认通过逻辑，但交易动作继续单独隔离。

## 策略约定

| 场景 | 默认策略 | 可配置策略 | 约束 |
|---|---|---|---|
| 条款/隐私同意 | `accept` | `pause`、`accept`、`decline` | 点击明确的同意或继续入口并记录 |
| 系统权限 | `allow` | `pause`、`allow`、`deny` | 只处理已知 Permission Controller 包 |
| 付费墙/试用 | `explore` | `pause`、`dismiss`、`explore` | 保留页面供探索，但不点击购买、支付、订阅确认或开始试用 |
| 广告/营销弹窗 | `dismiss` | `pause`、`dismiss` | 只点击关闭、跳过、稍后等安全入口 |
| 应用内评分 | `dismiss` | `pause`、`dismiss` | 仅识别系统托管评分窗口，不放行普通商店页面 |
| Onboarding 引导页 | `advance` | `pause`、`advance` | 优先把 Continue/Next 文本提升到最近可点击父容器；仍不可执行或点击无进展时从屏幕中部右向左滑动 |
| Google Play 评分/结算页 | `dismiss` | `pause`、`dismiss`（评分） | 评分和结算错误只关闭；购买确认始终只返回，绝不点击 Subscribe |
| 临时系统内容 | `ignore` | 固定 | 不进入目标应用页面指纹 |

每个特殊页面的自动动作有独立次数上限。超过上限后以结构化原因停止，不继续随机输入。

## Todo

- [x] P0：完成现有 Runner、智能遍历、AutoPopup 与旧 XTest 恢复逻辑审计。
- [x] P0：增加统一特殊场景分类、策略模型和结构化动作记录。
- [x] P0：在普通 DFS 动作前执行预处理，并实现有界重试及可恢复停止原因。
- [x] P0：排除其他包临时节点对目标页面指纹的污染。
- [x] P0：条款和权限按默认允许策略记录执行；购买、订阅确认和开始试用继续禁止随机或隐式执行。
- [x] P1：识别 Android/AOSP、Google、Samsung、MIUI 权限控制器并按策略处理。
- [x] P1：接入付费墙、广告、营销弹窗的安全关闭策略。
- [x] P1：复核 AutoPopup 边界；保留原 XTest 四字段配置、`mainContentContains` 上下文匹配和显式动作语义，不向兼容接口注入自主探索策略。
- [x] P1：保留原 XTest `WindowClose` 可伴随 Runner/用例执行的并行语义；互斥仅用于两个自主探索引擎之间。
- [x] P1：补齐条款、权限、付费墙、广告、评分、WebView 容器、Google Play 结算、临时系统节点和动作上限测试。
- [x] P1：更新 API、场景样例及兼容性口径。
- [x] V1：运行全部 Go 测试、Runner 构建与自检。
- [x] V2：在 Android 13/15/16 的 Foloy 清数据重装状态验证首次启动处理。
- [x] V2：确认三台设备无购买并记录页面覆盖和停止原因；测试进程在验收后清理。

## 验收门槛

1. 条款页面默认接受并形成结构化记录；配置为 `pause` 时返回 `policy_required`。
2. 权限控制器不再因为前台包变化直接触发通用 `safety_stop`。
3. 付费墙允许安全探索，广告默认关闭；购买类控件始终不进入候选动作。
4. 特殊动作失败或页面不变化时，在配置次数内停止并提供场景、策略和候选动作信息。
5. AutoPopup 继续按原 XTest 契约执行用户显式配置的动作，并可作为 `WindowClose` 能力伴随 Runner；自主随机 Runner 与智能遍历仍保持互斥。
6. 三台 Foloy 新用户链路至少到达可探索业务页，或返回可解释、可恢复的策略停止结果。

验收记录见 [`validation-exploration-preprocessor-20260909.md`](../validations/runs/validation-exploration-preprocessor-20260909.md)。
