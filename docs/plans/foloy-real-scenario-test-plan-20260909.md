# Foloy 真实场景探索测试计划

日期：2026-09-09

## 原则与边界

- Foloy 仅作为真实应用样本；生产修复必须依据 Android/Accessibility/Compose/WebView 的通用结构或原 XTest 契约，不使用 Foloy 包名、文案、坐标和设备型号分支。
- 条款默认允许、系统权限默认允许、广告默认关闭、付费墙允许安全探索。
- 禁止提交购买、订阅、试用和支付；进入系统购买确认页时只验证识别和返回。
- AutoPopup 保持原 XTest 四字段配置和 WindowClose 伴随执行语义。
- 每轮限制步数、记录随机种子、特殊动作、停止原因和最终前台包；异常先复现，再判断是否属于通用缺陷。

## 场景矩阵

| 编号 | 设备 | 场景 | 关键断言 |
|---|---|---|---|
| S1 | Android 16 | 标准首启完整链路 | 条款父容器可执行；过渡帧不误停；Onboarding、业务选择和付费墙可继续 |
| S2 | Android 15 | 首启中的系统弹窗与权限 | 已知 Permission Controller 可跨包处理；返回目标应用后继续探索 |
| S3 | Android 13 | Onboarding 手势与旧系统兼容 | Continue 不可用时左滑有效；坐标始终位于当前屏幕有效区域 |
| S4 | Android 16 | 付费墙、法律链接和 WebView | WebView 根容器不点击；内部链接可探索；退出后图关系可恢复 |
| S5 | Android 16（Android 15 锁屏后替代） | Google Play 评分、结算错误和购买确认 | 评分/错误可关闭；购买确认只返回；无 Subscribe 动作 |
| S6 | Android 13 | 广告/营销/Paywall 安全探索 | 关闭类控件优先；通用 Continue 在金融上下文不进入交易 |
| S7 | 三台 | 稳定性与重复页面 | 动画空树不提前结束；相同特殊页面最多处理三次；无死循环 |
| S8 | Android 13（Android 15 锁屏后替代） | AutoPopup 伴随探索 | `/popupBoxAssistant` 与探索可同时运行；显式上下文规则生效；停止后无残留 |

## Todo

- [x] 建立场景矩阵、统一安全边界和证据字段。
- [x] 构建并部署当前 Nova Agent 与 UiAutomator Provider。
- [x] 执行 S1-S3 首启、Onboarding 与系统权限基线（权限实景改由 Android 13 触发）。
- [x] 执行 S4-S6 特殊页面和交易保护验证（购买确认实景改由 Android 16 执行）。
- [x] 执行 S7 三台设备稳定性、次数上限和退出原因检查。
- [x] 执行 S8 原 XTest AutoPopup 伴随执行契约检查（Android 13）。
- [x] 对发现项进行通用性审计、修复与回归。
- [x] 清理临时进程、测试包、文件和端口转发，保留 Foloy。

## 证据字段

每次会话至少保存：设备序列号、Android/SDK、Foloy 版本、requestId、seed、步数、发现状态数、特殊动作数、最后特殊场景、停止原因、最终前台包，以及是否观察到购买/订阅动作。
