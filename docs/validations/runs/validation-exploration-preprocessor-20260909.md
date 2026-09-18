# Foloy 新用户特殊场景验收记录

日期：2026-09-09

## 验收环境

| 设备 | Android | SDK | 应用 |
|---|---:|---:|---|
| 83fc400c / 2510DPC44G | 16 | 36 | Foloy 1.3.0 (41) |
| R3CY80B2G4W / SM-S936U | 15 | 35 | Foloy 1.3.0 (41) |
| R5CN30EQKNM / SM-G9860 | 13 | 33 | Foloy 1.3.0 (41) |

三台设备均通过清除 Foloy 数据构造首次启动状态，策略使用默认值：条款接受、权限允许、广告关闭、付费墙安全探索、评分关闭、Onboarding 前进，特殊页面最多尝试三次。

## 结果

- 三台首次条款页均识别为 `consent/accept`，不可点击的 Continue 文本成功提升到最近可点击 Compose 父容器，三台各记录 1 次特殊动作。
- 条款后的短暂空语义树不再立即触发 `graph_exhausted`；连续空采样保护使三台均进入 Onboarding。
- 三台 Onboarding 均完成 3 个 `onboarding/advance` 特殊动作并继续进入普通探索；每个事件同时保存从屏幕中部右向左滑动的回退动作。独立单元测试覆盖无可点击父容器时选择 `swipe left`；Android 13 另以相同坐标实机执行右→左滑动，页面切换到 `Everything about your card`，确认 Foloy 支持该手势。
- Android 13 到达付费墙并以 `paywall/explore` 继续，未把 Terms of Use / Privacy Policy / Subscription Terms 误判成首次条款。
- Android 16 的法律条款 WebView 暴露了根容器误点击问题。最终实现不保留应用页面特判，而是在通用节点模型中排除 WebView 根容器点击，内部可访问节点仍按统一规则处理。
- Android 15/13 整链路均以 `max_steps` 正常结束；Android 16 在法律正文修复后的恢复链同样以 `max_steps` 结束。
- 验证过程中没有执行 Subscribe、购买、支付、开始试用或恢复购买。Google Play 购买确认使用强制 Back，结算错误只允许 OK/Got it/知道了/确定。

## 自动化回归

- `go test ./...`：通过，包含 automation、exploration、autopopup、httpapi 等全部 Agent 包。
- Runner 构建及 `NodeExplorer --self-test`：通过。
- 新增回归覆盖：默认条款接受、策略暂停、系统权限允许、付费墙上下文过滤、Google Play 评分/购买/结算错误、WebView 容器过滤、Onboarding 左滑、跨包临时节点指纹、特殊场景次数上限。

## 原 XTest 契约复核

- Foloy 只作为验收样本，生产代码不包含 Foloy 包名、产品文案、页面坐标或设备型号。
- AutoPopup 保持 `autoClickByText`、`autoClickByResourceId`、`autoInputByHint`、`autoInputByResourceId` 四字段及 `mainContentContains` 语义；用户显式配置仍按旧 XTest 执行。
- `WindowClose`/AutoPopup 可继续伴随 Runner 工作，不再以 Nova 自主探索的互斥策略改变兼容接口行为。
- WebView 修复落在通用节点资格层：只过滤 WebView 根容器点击，不识别具体应用的“法律正文页”，内部可访问控件仍可探索。

## 停止原因口径

本轮短会话使用有限步数验证预处理链路，因此 `max_steps` 是预期完成状态，不代表页面异常。中途暴露的误判和过渡帧问题均在同轮修复并复测；Android 14 未纳入本轮真机矩阵。
