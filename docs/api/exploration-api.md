# M3 智能遍历 API

所有接口仅绑定在 Nova Agent 的回环监听地址。M3.1 将页面分析与动作执行明确分开，客户端应先预览，再决定是否启动执行会话。

## 页面预览

`POST /v1/exploration/preview`

```json
{
  "package": "com.example.app",
  "rules": {
    "denyText": ["退出登录"],
    "allowText": ["下一步", "继续"],
    "allowResourcePrefixes": ["com.example.app:id/"]
  }
}
```

返回页面状态指纹、节点数、被过滤数量及候选点击动作。预览只读取当前前台包和 UiAutomator XML，不执行输入。

## 启动会话

`POST /v1/exploration/sessions`

```json
{
  "requestId": "regression-001",
  "package": "com.example.app",
  "maxSteps": 100,
  "intervalMillis": 750,
  "seed": 20260903,
  "execute": true,
  "enableScroll": true,
  "enableBacktrack": true,
  "recoverPopups": true,
  "maxBacktracks": 20,
  "specialHandling": {
    "mode": "safe",
    "consentPolicy": "accept",
    "permissionPolicy": "allow",
    "paywallPolicy": "explore",
    "adPolicy": "dismiss",
    "reviewPolicy": "dismiss",
    "onboardingPolicy": "advance",
    "maxAttempts": 3
  },
  "expectedActivities": [
    "com.example.app/com.example.app.MainActivity",
    "com.example.app/com.example.app.DetailActivity"
  ],
  "rules": {
    "denyText": ["退出登录"]
  }
}
```

`execute=true` 是强制条件，避免把预览请求误当成执行请求。相同 `requestId` 且规范化配置完全一致时幂等返回原会话；同一 `requestId` 携带不同包、步数或策略时返回冲突。已有其他会话运行时同样返回冲突。

## 查询与停止

- `GET /v1/exploration/sessions/current`：当前会话、停止原因和统计；
- `DELETE /v1/exploration/sessions/current`：携带当前状态中的 `identity.sessionId` 和 `identity.ownerToken`，分别作为 `X-XTest-Session-Id`、`X-XTest-Owner-Token` 请求头后协作式停止；
- `GET /v1/exploration/graph`：已发现状态及有向边；
- `GET /v1/exploration/steps`：按时间排列的动作记录。
- `GET /v1/exploration/receipts`：查看 stepId 幂等动作回执和旧 observation 拒绝记录；
- `GET /v1/exploration/report?scenario=<名称>`：导出 `xtest-evaluation/v1` 统一评估报告。

`expectedActivities` 必须来自本次被测 APK 的 manifest，采用完整的 `包名/类名` 格式。提供它时报告会给出精确 Activity 覆盖率；未提供时只记录实际观察数，不推测分母，也不输出覆盖率百分比。

## M3.1 安全规则

- 只接受合法 Android 包名，且目标包必须处于前台；
- 只选择可见、启用、非密码输入且有有效边界的可点击节点；
- 系统界面和其他包节点不会进入候选集；
- 内置过滤卸载、删除、付款、购买、清除数据、恢复出厂及格式化等中英文危险词；
- 客户端可以增加拒绝词或配置允许文本/资源前缀，但不能关闭内置危险词；
- 会话具有最大步数和最小动作间隔，离开目标包或输入失败时立即停止；
- 智能遍历与随机 Runner 互斥，UiAutomator 页面抓取在 Agent 内串行化；
- 滚动和回溯均需显式启用；向前滚动使用同坐标反向滑动恢复；
- 系统返回键只用于由已记录点击边进入的子状态，且受 `maxBacktracks` 限制；
- `recoverPopups=true` 仍优先处理“取消、稍后、以后再说、跳过、继续等待”等明确安全动作；
- `specialHandling` 在普通 DFS 前处理首次条款、Onboarding、权限控制器、付费墙、广告和系统评分。默认同意条款、允许系统权限、保留付费墙供安全探索并关闭广告；
- Compose 文本节点不可点击时，只提升到包含该文本的最近可点击父容器；Onboarding 仍无法前进时，执行屏幕中部从右向左滑动；
- 条款识别同时要求条款语义、同意意图和可执行决策控件；WebView 根容器不作为点击目标，内部可访问节点仍按通用规则探索；
- Google Play 购买确认页始终执行返回，结算错误只点击明确的确认关闭控件；
- 购买、支付、订阅确认、开始试用和恢复购买始终从候选动作中过滤；
- 系统权限只允许已知 AOSP、Google、Samsung 和 MIUI 权限控制器跨包处理，其他跨应用跳转仍立即停止；
- 外部应用恢复预算按连续失败计算；每次稳定回到目标包后重置，不会因多次合法进入同一系统控制器而累计误停；
- 特殊页面连续处理后未变化时受 `maxAttempts` 限制，并返回 `special_handling_failed`；策略设为 `pause` 时返回 `policy_required`。
- 全屏广告按开屏、插屏、激励视频和可试玩广告使用独立等待预算；显式关闭节点优先，随后才使用已知广告 Activity 的角落关闭候选与有界 Back。
- 广告 WebView 即使持续刷新文本和页面指纹，关闭尝试仍绑定同一次广告遭遇，不会借动态内容重置上限。
- 智能探索和 Runner 运行期间 Companion 触摸层会被移除，避免覆盖右上角广告关闭键；会话完成后恢复主菜单。空闲时手动最小化为贴右边缘的低显著窄把手，不进入 UiAutomator/无障碍探索树，普通单击不会展开，需长按恢复菜单。
- 状态接口通过 `adEncounters`、`adCloseActions`、`adBackFallbacks`、`adExternalRecoveries`、`adRecoveries`、`adWaitMillis`、`lastAdType` 和 `lastAdPhase` 记录广告恢复过程。

## M3.3 A/B 评估

统一报告区分两种指标：Activity 覆盖率有 manifest 分母，可以进行百分比回归判断；页面状态是运行期间发现的指纹数量，只比较发现数，绝不换算成覆盖率。报告还记录崩溃、停止原因、点击、滚动、回溯、恢复动作及与时间无关的动作序列摘要。

验证场景文件：

```powershell
go run ./agent/cmd/exploration-compare -scenario-file ./tests/scenarios/android16-gallery-smoke.json
```

完成 Nova 会话后导出报告：

```powershell
Invoke-WebRequest 'http://127.0.0.1:7912/v1/exploration/report?scenario=android16-gallery-smoke' -OutFile ./nova-report.json
```

与已审核的 Nexus 基线比较：

```powershell
go run ./agent/cmd/exploration-compare -baseline ./tests/reports/baselines/gallery-same-device-version-60s.json -candidate ./nova-report.json -out ./comparison.json
```

示例中的 `gallery-same-device-version-60s.json` 是采集后传入的报告占位路径，
仓库不提供可跨设备复用的 Gallery 黄金基线。Gallery 的包名和能力会随设备厂商
变化。仓库提供 Android 16 MIUI Gallery 与 Android 13 Samsung Gallery 两个
安全场景；黄金基线必须在相同设备、Gallery 版本和场景下真实采集，不能复用
历史 Foloy 指标。

检查相同引擎版本、场景和种子的两次动作序列是否一致：

```powershell
go run ./agent/cmd/exploration-compare -determinism './run-1.json,./run-2.json'
```

旧 Nexus 原始日志也可以导入统一格式：

```powershell
go run ./agent/cmd/exploration-compare -nexus-log ./nexus.log -scenario android16-gallery-smoke -package com.miui.gallery -out ./nexus-report.json
```
