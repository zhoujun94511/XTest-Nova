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
  "durationSeconds": 60,
  "intervalMillis": 500,
  "seed": 20260903,
  "execute": true,
  "enableScroll": true,
  "enableBacktrack": true,
  "recoverPopups": true,
  "maxBacktracks": 20,
  "activityMode": "none",
  "activities": [],
  "controlBlacklist": [],
  "renderFallbackMode": "bounded",
  "lowBatteryExit": true,
  "minBatteryPercent": 15,
  "targetCases": [],
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

`execute=true` 是强制条件，避免把预览请求误当成执行请求。相同 `requestId` 且规范化配置完全一致时幂等返回原会话；同一 `requestId` 携带不同包、时长或策略时返回冲突。请求里的 `maxSteps` 在规范化时被清掉，不参与这次比较，也不会让会话停下来。已有遍历、随机运行、录制或回放处于运行、停止或收尾时同样返回冲突，后到的请求不替换当前会话。

`durationSeconds` 是启动必填的墙钟上限。省略、0 或小于 60 秒时按 60 秒兜底，上限 604800 秒。时间到达后以 `duration_elapsed` 结束。每一轮循环在采集前都检查这个墙钟。省略 `intervalMillis` 时接口默认 250 毫秒，合法范围是 100 到 60000，超出范围启动失败。命令行和悬浮窗会显式发送 500 毫秒，并带上活动范围、控件跳过、低电量、纯渲染策略和目标页用例。Web 智能探索发送页面上的动作间隔；未改时为 500 毫秒，表单最少 100 毫秒。低电量 15% 停止可勾选，勾选时带上 `lowBatteryExit=true` 和 `minBatteryPercent=15`。`targetCases` 非空时，启动前按 Monkey 的规则准备已签名用例；准备失败则会话不会开始。字段含义、片段匹配和同时启动的冲突规则见 [Monkey 高级策略 API](monkey-api.md)。

## 当前遍历链路

命令行、悬浮窗和不带 `engine` 的 Monkey POST 进入同一次 `exploration.Manager` 会话。Web 智能探索直接调用 `POST /v1/exploration/sessions`，同样进入这套会话。`engine=runner` 的随机遍历进入 Java Runner，不经过这里。`explorer` 不存在时，不带 `engine` 的 Monkey POST 才落到 Java Runner。一轮循环按下面的顺序处理，轮与轮之间再等待 `intervalMillis`。这个间隔只做节流，不代替动作后的稳定等待。

1. 采集前台包名、Activity、焦点窗口 token 和控件树。token 是 `dumpsys window windows` 里目标窗口的 `Window{token}`，不再读取 `mSeq`，也不调用不带参数的 `dumpsys window`。窗口列表里没有对应 token 时版本为空。采集期间 Activity 变化记为不稳定；连续 20 次不稳定以 `unstable_capture_limit` 停止。采集命令或控件树解析连续失败 5 次才以 `safety_stop` 停止，中间成功一次就重新计数。类名（不含包名）含 `share`、`chooser`、`resolver`、`sendintent` 的交接页先返回，并在 60 秒内等待，不把应用重新拉起。
2. 处理特殊场景。识别顺序是权限控制器、系统选择器、系统设置、系统解析器、评价、账单错误、购买确认、应用内购买确认、同意页、引导页、已知广告 Activity、充值或支付失败弹窗、付费墙，最后是带关闭控件的广告文案。充值或支付失败弹窗只点关闭或跳过，不点反馈或客服，并禁止再次选择打开它的那个点击。出现一次之后，本会话不再选择购买控件：资源名含 `sku`、`billing`、`purchase`、`subscribe`、`recharge`、`cashier`，或文案是订阅、确认购买、付款方式、充值、`subscribe`、`recharge`、`top up`。弹窗文案变化不重置尝试次数，预算按 Activity 计算。达到上限后走卡住页逃逸，且不因此打开四次手势阶梯；页面上只剩购买控件时同样离开，不再点下一个商品。
3. 前台已经离开目标包时做包外恢复。
4. 刚执行动作或刚重启后，最多等待 1.5 秒。Activity 变化后立即继续；同一个新的窗口 token 连续出现两次也继续。token 为空时不提前结束等待。同一窗口内的重排不再单独提前结束等待。
5. 解析普通可操作控件。
6. 启动页和空白页先等待，再决定是否写入状态图。
7. 执行会话守卫：时长、低电量、Activity 范围、目标页回放、控件黑名单、无控件时的坐标回退。
8. 写入状态图，选择一个普通动作并执行。

特殊场景默认安全策略：权限点允许，广告和评价点关闭，引导页向前，付费墙交给普通探索。类名含 `language` 或 `locale` 的页面不按引导页向前。首次启动的语言页不看类名：同一页上至少有两个整段文案就是语言名的控件时，不按引导页向前。没有文案的控件，以及整段文案是「选择语言」这类设置标题的控件，不把这一页拆成普通页。有 English 就点 English，否则点中文或简体中文；这一下成功之后才点继续或确定。点击失败时不确认。跳过不点。类名含 setting、language 或 locale 的页面不替用户改语言。旁边还有播放这类普通内容控件时，只避开语言名和继续、确定。资源名含 language 的语言名只允许点启动页上被选中的那一个。单独一个「中文」或 English 不会把整页判成语言页。策略为 `pause` 时以 `policy_required` 停止，计时会话也不绕过。

- 购买确认、系统选择器和系统设置直接发送系统 `BACK`。
- 已知全屏广告优先点击描述中含 `interstitial close` 的控件，其次是关闭文案或资源名，再次是角落里已经暴露的关闭控件。这些都没有时，立刻点右上角关闭位置，随后是系统 `BACK`。不等待广告片长，开屏、插屏、试玩和激励视频使用同一套关闭顺序。同一次广告遭遇共用尝试次数。
- 评价找不到关闭控件时直接 `BACK`。权限找不到按钮时最多等 5 秒。
- 同一特殊场景默认最多 3 次。广告、未处理的特殊页和其他非支付失败场景达到次数后，允许冷启动并走下面的四次手势阶梯，壳页面也不例外；时长已经用完则停在 `duration_elapsed`。没有可执行动作的 `special_unhandled` 同样允许冷启动。`policy_required` 仍会停止。加载失败单独按同一 Activity 重试 2 次，之后去掉滑动、继续普通探索，不走逃逸。支付失败达到上限后走逃逸，但不把壳页面送进四次手势阶梯。

离开目标包时，刚执行过回溯只重新拉起目标。桌面是包名最后一段以 `launcher` 结尾或只在它后面跟数字，包名以 `.home` 结尾，类名正好是 `Launcher`、`LauncherActivity`、`Home`、`HomeActivity`，或类名以 `Launcher` 结尾。类名同时含 `share`、`chooser`、`resolver`、`sendintent` 时按交接页返回，不按桌面拉起。另外按整段包名识别各厂商系统桌面进程，包括 AOSP、Pixel、三星、小米、POCO、华为、荣耀、OPPO、一加、vivo、传音、摩托罗拉、Nothing、LG、华硕、索尼、HTC、魅族、努比亚、中兴、联想、锤子和黑鲨。包名中间出现 `launcher` 不算。落在桌面时只把目标应用带回前台，不计入外部失败，也不 `force-stop`。其他外部包先发送系统 `BACK`，再以 Launcher 类别拉起目标包。同一个外部包最多 3 次，回到目标包后清零。第 3 次之后 `force-stop` 并冷启动，然后重新开始最多 8 秒的启动等待。时长已经用完则停在 `duration_elapsed`。`BACK` 或拉起命令本身失败，直接 `safety_stop`。前台包名读空时本轮等待，不发送返回键，也不计入外部恢复。

类名（不含包名和类路径）包含 splash、startup、launch、loading 或 routing 时，最多按住 8 秒，不把空白启动页写入状态图。包名或中间包路径里的这些片段不算启动页。冷启动会重新开始这 8 秒。按住期间仍检查时长和低电量。没有可点控件时先累计 3 次空采集。页面已经没有可选动作后，再确认 2 次才视为耗尽。同一语义状态连续两次没有新边，或引导页用完次数仍停在原页，走同一次卡住离开：还有未执行的安全动作就继续点，没有则返回一次，返回后仍是这个状态就冷启动。这段离开共用 30 秒冷却，会话继续直到时长结束。语言选择和桌面拉回不负责这次离开。

页面耗尽时，如果页面上还有返回控件（`navigation_iv`、`back_iv` 或文案“返回 / Back”），先点它，并封掉刚才那个没有离开页面的入口。主页面、播放页和路由页按类名判断，包名和类路径不算。这些页面在普通耗尽和支付失败逃逸时先发一次系统 `BACK`。页面仍然停在这个 Activity 时，`force-stop` 并冷启动，然后 30 秒内不再杀进程，也不再发返回。这些路径不做左右缘滑动。广告或未处理特殊页把冷启动打开后，壳页面改走下面的四次手势。

其他 Activity 在点过页面内返回后，按同一 Activity 依次做 4 次。手势发出后的 1.5 秒稳定等待与普通动作相同，放在特殊页和离包恢复之后；这段时间仍处理权限、广告和离开目标包，只推迟写入状态图：

1. 系统 `BACK`。
2. 左缘向内滑动。
3. 右缘向内滑动。
4. 自上而下的内容滑动，起点在屏幕高度 15%，终点在 55%，避开状态栏和底部 Home 区域。

这 4 次之后 `force-stop` 并冷启动。普通耗尽和支付失败逃逸里，主页面、播放页和路由页不走这 4 次滑动，返回一次后页面不变再冷启动，并间隔 30 秒。时长用完则停在 `duration_elapsed`。落在桌面时只把目标应用带回前台，两次拉起至少间隔 8 秒，不 `force-stop`。

会话守卫发生在普通选动作之前。低电量每 30 秒读取一次 `dumpsys battery`，低于阈值停在 `low_battery`；正在充电不豁免，读数失败则本轮继续。Activity 不在允许范围或落在排除范围时先发送 `BACK`。前台 Activity 命中目标页用例时，回放作为本会话的子任务；失败 10 秒后可再试，成功后不再重复。黑名单只去掉同名控件。页面是 `SurfaceView` 或 `UnityPlayer` 且没有可操作控件时，`bounded` 每个 Activity 最多 5 次安全区坐标动作，`continuous` 持续执行。

可点父节点没有自身文案时，使用第一个子节点文案作为语义键，最多取 32 个字符，避免一页上的空容器被当成同一个动作。同一 Activity 里已经执行过的点击不会因为画面换成新指纹而再次入选。登录、分享这类外部账号动作只在没有其他未执行点击时才进入候选。出现过支付失败后，购买控件直接退出候选，不参与这次降权。普通候选会去掉内置拒绝词，包括卸载、安装、下载、清除数据，以及客服、联系我们、反馈和帮助中心。语言切换不进入候选。控件自身整段文案是 Language、语言这类设置项时排除。资源 id 的控件名（不含包名）含 language、locale 时，设置入口仍然排除；整段文案本身就是语言名的控件留给页面规则决定。类名（不含包名）含 language 或 locale 的页面不点击。设置页上整段文案就是一种语言名的控件也不点。首次启动页同样：至少两个控件的整段文案是语言名时，有 English 就点 English，否则点中文或简体中文，这一下成功之后再点继续或确定。没有文案的控件和语言设置标题不拆散这个页面。旁边还有普通内容控件时，避开语言名以及继续、确定。资源名含 language 的语言名只在这个启动页上允许点被选中的那一个。普通标题和分类名即使正好是「中文」或 English，仍可点。密码、验证码和支付类输入框不进入候选。取消、稍后、跳过等恢复控件在 `recoverPopups=true` 时优先选择。其余动作按权重抽样：点击 100，滑动 80，输入 45，返回 25；连续两次非滑动后滑动升到 220；登录、分享、云同步等外部风险词降到四分之一。

Activity 记忆按包名和 `versionCode` 落盘，只记录语义动作到达过哪些 Activity，不保存 Q 值。没见过的动作加权，还能到达新 Activity 的动作加权，只去过已知页面的动作降权。选不出动作时，沿进入当前页的反向边回溯，默认最多 20 次。连续 3 次重复的语义周期，或同一语义转移累计 4 次，会封掉对应边并允许一次回溯升级。同一页面的这次升级冷却 30 秒，按墙钟计算。同一页面的滑动不算导航循环；同一语义页上同一条滑动连续没有改变页面 3 次后，封掉这条滑动，避免列表停住时一直滑。输入重复由字段尝试记录单独限制。

动作发出前再次核对页面指纹。点按和滑动超时 5 秒，输入超时 12 秒。命令失败的点击仍算本页已执行，不会再发同一下。连续 5 次命令失败才停在 `input_failed`，中间成功一次就重新计数。下一步落到同一语义页时记为无效动作，落到不同语义页时记为有效动作。语义页只含 Activity 和排序后的控件身份，不含坐标和被动文案。特殊场景动作没有目的页，不进入这一对计数。

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
- 内置过滤卸载、删除、安装、下载、清除数据、恢复出厂、格式化、客服和反馈等中英文危险词；
- 客户端可以增加拒绝词或配置允许文本/资源前缀，但不能关闭内置危险词；
- 每次启动都有墙钟上限。省略或不足 60 秒时按 60 秒执行，到点停在 `duration_elapsed`。动作间隔有下限。离开目标包时按恢复预算处理，输入命令连续失败 5 次才停止；
- 智能遍历与随机 Runner 互斥，UiAutomator 页面抓取在 Agent 内串行化；
- 滚动和回溯均需显式启用；向前滚动使用同坐标反向滑动恢复；
- 状态图回溯只沿已记录的进入边返回，且受 `maxBacktracks` 限制；主页面、播放页和路由页耗尽后先返回一次，页面不变再冷启动，其他页面的卡住逃逸见上文；
- `recoverPopups=true` 仍优先处理“取消、稍后、以后再说、跳过、继续等待”等明确安全动作；
- `specialHandling` 在普通 DFS 前处理首次条款、Onboarding、权限控制器、付费墙、广告和系统评分。默认同意条款、允许系统权限、保留付费墙供安全探索并关闭广告；
- Compose 文本节点不可点击时，只提升到包含该文本的最近可点击父容器；Onboarding 仍无法前进时，执行屏幕中部从右向左滑动；
- 条款识别同时要求条款语义、同意意图和可执行决策控件；WebView 根容器不作为点击目标，内部可访问节点仍按通用规则探索；
- Google Play 购买确认页始终执行返回，结算错误只点击明确的确认关闭控件；
- 付费墙默认交给普通探索。还没出现支付失败时，订阅、更多权益和恢复购买可以点击；出现一次支付失败后，这些购买控件退出候选。购买确认页本身走特殊场景返回；
- 系统权限只允许已知 AOSP、Google、Samsung 和 MIUI 权限控制器跨包处理；其他跨应用前台按包外恢复预算处理，见上文；
- 外部应用恢复预算按连续失败计算；每次稳定回到目标包后重置，不会因多次合法进入同一系统控制器而累计误停；
- 特殊页面连续处理后未变化时受 `maxAttempts` 限制。广告和未处理特殊页随后允许冷启动并走四次手势阶梯；支付失败走卡住页逃逸，壳页面只返回一次再冷启动。加载失败重试 2 次后去掉滑动并留在普通探索。策略设为 `pause` 时返回 `policy_required`。
- 全屏广告不按片长挂起。显式关闭节点优先；没有节点时点右上角关闭位置，再用有界 Back。`adMaxWaitSeconds` 仍接受原范围，但不再把会话停在广告上。
- 广告 WebView 即使持续刷新文本和页面指纹，关闭尝试仍绑定同一次广告遭遇，不会借动态内容重置上限。
- 遍历运行期间，悬浮窗收成边缘手柄，避免挡住页面控件；任务结束后从手柄恢复主菜单。点击手柄没有动作，长按手柄区域才展开。空闲时手动最小化同样贴在右边缘，不进入 UiAutomator/无障碍探索树。录制仍使用自己的录制表面。
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
