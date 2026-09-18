# M4.2 录制回放 API

M4.2 提供独立于随机 Runner 和智能遍历的版本化录制回放链。用例格式保持向后兼容的 `xtest-recording/v1`，触点坐标统一保存为 `0..1` 百分比，回放时按当前设备分辨率恢复。

新录制还会在触摸按下阶段保存可用的控件语义，包括 resource-id、文本、content-description、类名、边界和层级观察指纹。回放优先在当前页面重新定位该节点；未找到时使用原百分比坐标兼容旧页面与无层级场景。

## 能力边界

当前阶段支持：

- Protocol-B 最多 10 个触点录制；
- 点击、双击、长按、滑动和多指手势的语义归并；
- 通过明确接口记录输入法最终提交的 UTF-8 文本和返回键；
- 从当前聚焦的非密码输入框自动读取最终 UTF-8 文本及归一化中心坐标；
- 基于 64 位感知哈希的截图断言；
- 用例原子落盘与 SHA-256 完整性摘要；
- 点击、双击、长按、滑动、多指、UTF-8 文本和受限返回键回放；
- `0.1..4` 倍时间轴速度；
- 每个动作执行前检查目标应用仍在前台；
- 从 `resumeFrom` 动作序号进行中断续播；
- `loops`：默认 1 次，`2..50` 次有限循环，`-1` 直到手动停止；
- 与随机 Runner、智能遍历、其他录制或回放会话互斥。

本阶段仍不读取输入法 composing 候选态和视频断言。最终文本只在用户明确点击
“采集最终文本”后，从当前聚焦的非密码输入框读取；也可由控制端在输入法最终提交后显式
写入录制会话。密码字段及资源 ID、描述、输入类型或掩码文本显示为密码、PIN、OTP、银行卡、支付等敏感语义的字段会被保守拒绝；即使设备错误报告 `password=false` 也不会直接采集。未知动作和除返回键以外的按键不会被静默伪装为可用能力。

## 开始录制

`POST /v1/recordings`

```json
{
  "requestId": "record-detail-001",
  "package": "com.example.app",
  "task": "checkout",
  "name": "open-detail"
}
```

目标包必须位于前台。Agent 会读取实际触摸设备和当前显示尺寸，然后开始接收触点，但不会注入任何输入。

## 查询、停止和取得用例

- `GET /v1/recordings/current`：录制状态和已归并动作数；
- `GET /v1/recordings?package=<包名>`：列出已保存用例；
- `GET /v1/recordings/cases/{id}`：读取完整已保存用例；
- `DELETE /v1/recordings/cases/{id}`：携带 `X-XTest-Control: true` 删除指定已保存用例目录；
- `GET /v1/recordings/drafts?package=<包名>`：列出异常中断后保留的恢复草稿；
- `POST /v1/recordings/drafts/{id}/finalize`：携带 `X-XTest-Control: true` 验证并固化草稿；
- `DELETE /v1/recordings/drafts/{id}`：携带 `X-XTest-Control: true` 显式丢弃草稿；
- `DELETE /v1/recordings/current`：携带当前状态中的会话/所有者请求头后停止并原子保存用例；
- `GET /v1/recordings/current/case`：取得当前内存中的结构化用例。
- `POST /v1/recordings/current/text`：携带当前 `X-XTest-Session-Id`、`X-XTest-Owner-Token`，记录最终 UTF-8 文本，可带 `focus=true` 和百分比坐标；
- `POST /v1/recordings/current/focused-text`：携带当前 owner 凭证，自动读取当前聚焦的非密码输入框最终文本和中心坐标；
- `POST /v1/recordings/current/key`：携带当前 owner 凭证，记录白名单内的返回键（Android keyCode 4）；
- `POST /v1/recordings/current/assertions/screenshot`：携带当前 owner 凭证，记录当前屏幕感知哈希及允许距离。
- `PUT /v1/recordings/current/excluded-bounds`：携带当前 owner 凭证，更新悬浮窗排除区，避免移动控制条后把拖动或按钮点记录进用例。

默认保存路径：

```text
/sdcard/xtest-nova/<目标包名>/Replay/<任务名>/<UTC时间>/
  case.json
  manifest.json
  evidence.json
  screenshots/
```

保存后的 `integrity` 覆盖用例元数据和全部动作。任何字段被修改后，都必须重新经过可信工具审核并生成摘要；回放入口拒绝摘要不匹配的用例。

录制进行中会写入 `Replay/.drafts/<requestId>/draft.json`，截图参考文件同步位于草稿目录。正常完成后草稿被删除；Agent 或设备异常终止时草稿保留，可从 Companion 的“可恢复草稿”入口固化。`manifest.json` 记录用例指纹、录制尺寸、动作数和用例包文件摘要。
`task` 可省略；省略时继续使用 `Replay/<UTC时间>` 旧层级，既有 `xtest-recording/v1`
用例的摘要和加载保持兼容。

`POST /v1/replays/validate` 可以只校验完整用例并返回动作数，不创建回放会话，也不要求 `execute=true`。

## 开始回放

`POST /v1/replays`

```json
{
  "requestId": "replay-detail-001",
  "execute": true,
  "speed": 1,
  "resumeFrom": 0,
  "loops": 1,
  "case": {
    "schemaVersion": "xtest-recording/v1",
    "name": "open-detail",
    "package": "com.example.app",
    "recordedAt": "2026-09-03T08:00:00Z",
    "recordedWidth": 1080,
    "recordedHeight": 2400,
    "actions": [],
    "integrity": "sha256:<摘要>"
  }
}
```

`execute=true` 是强制门禁。服务先校验 schema、动作上限、时间顺序、坐标、按键白名单和完整性摘要，再确认目标包位于前台。回放期间每个动作前都会再次检查前台包；系统权限框（PermissionController / 安装器）会先尝试点「允许」并等待目标回到前台，其它包离开目标应用立即以 `safety_stop` 停止。

Unicode 文本会先聚焦可选坐标、全选并清空旧值，再通过隔离的官方 scrcpy 4.1 控制会话粘贴。该控制会话关闭视频，不会替换正在使用的画面流。`resumeFrom` 必须位于 `0..actions.length`，续播时以该动作作为新的相对时间起点。

- `GET /v1/replays/current`：查看动作总数、`completedActions` 和停止原因；
- `DELETE /v1/replays/current`：携带 `X-XTest-Session-Id` 与 `X-XTest-Owner-Token` 协作式停止当前回放。

录制完成返回 `caseFingerprint`；启动回放可同时提交该值，服务会对深拷贝后的完整用例重新
计算指纹，不一致即拒绝执行。回放完成后生成动作回执、`replay.json` 和 `evidence.json`，
只有截图检查点及其本次回执证据完整时才为 `passed`，否则按实际情况输出 `failed` 或
`not_tested`。

## 用例动作

```json
{
  "type": "swipe",
  "offsetMillis": 1200,
  "durationMillis": 350,
  "start": {"x": 0.5, "y": 0.75},
  "end": {"x": 0.5, "y": 0.25}
}
```

`offsetMillis` 表示相对录制起点的动作开始时间。回放使用绝对时间轴调度，前一个长动作的执行时间不会被重复叠加到下一动作延迟中。

双指动作示例：

```json
{
  "type": "multi_touch",
  "offsetMillis": 1800,
  "durationMillis": 300,
  "contacts": [
    {"index": 0, "start": {"x": 0.4, "y": 0.5}, "end": {"x": 0.2, "y": 0.5}},
    {"index": 1, "start": {"x": 0.6, "y": 0.5}, "end": {"x": 0.8, "y": 0.5}}
  ]
}
```

截图断言使用 16 位十六进制的 64 位感知哈希，`maxHashDistance` 范围为 `0..16`。超过阈值时回放以 `assertion_failed` 停止，而不是归类为输入失败。
