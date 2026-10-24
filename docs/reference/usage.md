# XTest Nova 使用说明

## 1. 项目边界

XTest Nova 使用可维护的 Agent、Runner、Companion 分层架构，对外统一使用
`/data/local/tmp/xtest-nova-agent`。

## 2. 设备端命令

| 操作    | 命令                             | 行为                                                                     |
|-------|--------------------------------|------------------------------------------------------------------------|
| 完整后台启动 | `xtest-nova-agent server -d`        | 校验并安装完整运行时，自动显示 Companion 悬浮窗；创建独立会话并写入 PID、日志 |
| 无界面启动 | `xtest-nova-agent server -d --no-popup` | 完整安装运行时但不拉起悬浮窗，供自动化、维护和诊断使用 |
| 停止    | `xtest-nova-agent server -d --stop` | 只停止 PID 文件指向的 Nova 进程；不存在时幂等成功                                         |
| 查看 Monkey | `xtest-nova-agent monkey status` | 查询当前 Monkey 的运行、收尾和停止原因，不改变任务 |
| 停止 Monkey | `xtest-nova-agent monkey stop` | 携带当前会话所有权正常停止 Monkey；保留 Agent 和产物，收尾后自动恢复悬浮窗 |
| 恢复悬浮窗 | `xtest-nova-agent popup start`      | 悬浮服务被强停或 Companion 被单独卸载时重新安装并显示控制器；正常启动无需执行                       |
| 查看悬浮窗 | `xtest-nova-agent popup status`     | 输出包名、安装状态、悬浮服务运行状态、暂存路径和安装路径                                           |
| 卸载悬浮窗 | `xtest-nova-agent popup uninstall`  | 卸载 `com.openatx.xtest.popup`；未安装时幂等成功                                  |
| 查看版本  | `xtest-nova-agent version`          | 输出与 `GET /version`、发布清单相同的版本                                           |
| 健康检查 | `xtest-nova-agent doctor` | 查询本机 `127.0.0.1:7912` 的 `/v1/health` |
| 设备信息 | `xtest-nova-agent device` | 输出品牌、型号和 SDK |
| 前台应用 | `xtest-nova-agent foreground` | 输出当前前台包名 |
| 应用 | `xtest-nova-agent apps list`、`info <包名>`、`launch <包名>` | 列出、查看或启动应用 |
| 截图 | `xtest-nova-agent screenshot <文件>` | 把当前屏幕 PNG 写到 `/data/local/tmp`、`/sdcard` 或 `/storage/emulated/0` |
| 启动 Monkey | `xtest-nova-agent monkey start --package <包名> --duration <秒>` | 启动同一次遍历会话。未写可选参数时，间隔 500 毫秒、纯渲染策略有界、低电量 15% 停止、活动范围不限制 |
| Monkey 结论 | `xtest-nova-agent monkey report` | 输出本次运行的通过 / 失败 / 未测结论 |
| 探索 | `xtest-nova-agent explore preview`、`start`、`status`、`stop` | `start` 必须带 `--package`；只有再带 `--execute` 才会点击。不带 `--execute` 时，`--json` 会带 `previewOnly: true`。执行时使用与 Monkey 相同的间隔、渲染、低电量和范围参数；不写 `--duration` 时按步数结束，默认 100 步 |
| 录制 | `xtest-nova-agent record start`、`status`、`stop`、`cases` | `start` 需要 `--package` 和 `--name` |
| 回放 | `xtest-nova-agent replay start`、`status`、`stop` | `start` 需要 `--package`、`--name` 和 `--execute` |
| 性能 | `xtest-nova-agent perf start`、`status`、`stop` | `start` 需要 `--package` |
| 产物 | `xtest-nova-agent artifacts [--package <包名>]` | 列出 `/sdcard/xtest-nova` 下的会话 |
| 归档 | `xtest-nova-agent pull <目录> <文件>` | 把允许路径下的目录打成 zip 再写到允许路径 |

这些命令都在设备上执行，并访问本机 `127.0.0.1:7912`。电脑上的调用方式是 `adb shell /data/local/tmp/xtest-nova-agent <子命令>`。停止 Monkey、探索、录制、回放和性能时会先读取当前会话身份，再带上会话头；输出里不包含 owner token。加上 `--json` 时输出去掉令牌后的接口 JSON。命令不提供远控画面，也不调用 `/shell` 或会关闭 Agent 的 `/stop`。

命令行、Web 控制台和悬浮窗启动的是同一次设备会话，不按入口分优先级。已经在运行、停止或收尾的会话保持不变；后到的启动返回冲突，不会排队，也不会把正在执行的遍历换掉。任一入口都可以查询当前会话并停止它。停止会等到当前目标页回放的输入结束，最长 120 秒。

`monkey start` 与带 `--execute` 的 `explore start` 还可使用：

| 参数 | 含义 | 缺省 |
|------|------|------|
| `--throttle <毫秒>` | 两次动作之间的固定间隔，范围 100–60000 | 500 |
| `--render off\|bounded\|continuous` | 没有控件的 Surface/Unity 页面是否做坐标点击。`bounded` 每个 Activity 最多 5 次，`continuous` 不设这个次数 | `bounded` |
| `--min-battery <百分比>` | 电量不高于该值时停止。`0` 关闭低电量停止 | 15，且启用低电量停止 |
| `--activity-mode none\|allowlist\|blocklist` | 活动范围。后两种必须再写 `--activities` | `none` |
| `--activities <列表>` | Activity 列表，逗号分隔。悬浮窗选择器里的 `.类名` 可以直接使用 | 空 |
| `--control-blacklist <列表>` | 跳过文本、描述或资源 ID 中包含这些片段的候选控件，逗号分隔 | 空 |
| `--target-cases <映射>` | `Activity=任务/用例`，多条用分号或中文分号分隔。用例必须已经录制并签名 | 空 |
| `--duration <秒>` | 墙钟时长，范围 1–604800。设置后步数到上限也不会提前结束 | Monkey 必填；探索可不写 |

探索不写 `--duration` 时，`maxSteps` 为 0 会按 100 步结束，允许范围 1–10000。Web 控制台只保留一个遍历入口。页面上填写运行时长和动作间隔，间隔默认 500 毫秒、范围 50–60000；低电量 15% 停止可以勾选。有界渲染和空的活动范围仍随请求发出。命令行和悬浮窗仍可填写这些约束。接口省略间隔时仍默认 250 毫秒。

`server -d --stop` 明确定义为停止，不隐式重启。需要重启时先执行停止命令，再执行
`server -d`，避免升级脚本出现双进程竞态。只推送二进制文件不会安装或启动任何 APK；执行
Server 后才会进行 bootstrap。正常启动默认显示悬浮窗，只有显式 `--no-popup` 才跳过显示。
悬浮窗依赖系统「显示在其他应用上层」权限；部分厂商不允许 shell 静默授权，启动后会打开
系统列表，选择 XTest Nova 并打开开关即可，不必重新部署。系统族差异见
[悬浮窗权限](overlay-permission.md)。

## 3. 部署

### 3.1 默认回环与 ADB 转发

```powershell
.\deploy.ps1 -Serial <serial> -StartServer
```

默认情况下，Agent 7912、Companion 8912 和 Monitor 7890 都只监听设备回环地址。
部署脚本只转发 Agent 7912，并优先分配空闲本机端口。旧工具固定使用 7912 时传入
`-LocalAgentPort 7912`。可按需增加：

- `-ForwardCompanion`：本机 8912 → 设备 8912；
- `-ForwardMonitor`：本机 7890 → 设备 7890；
- `-NoPopup`：完整安装运行时但不自动显示悬浮窗。

即使启用 LAN 模式，8912 和 7890 也始终保持回环监听，只能按上述方式通过 ADB
转发访问。

### 3.2 生成 LAN 令牌文件

LAN 令牌不是下载或预置的固定文件，应由设备管理员在主机上自行生成。项目提供安全
生成脚本：

```powershell
$tokenPath = Join-Path $HOME '.xtest-nova-api-token'
.\scripts\new-lan-token.ps1 -OutputPath $tokenPath
```

脚本使用密码学安全随机数生成 32 字节数据，以 Base64、无 BOM UTF-8 写入文件；
已存在文件默认拒绝覆盖，轮换时必须显式增加 `-Force`。
把该文件保存在仓库外并限制为当前管理员可读，绝对不要提交令牌文件。不要在命令行
参数中直接传入令牌，也不要把令牌放在 URL、查询参数、日志或截图中。

### 3.3 LAN 部署与 API 调用

只有主 Agent 7912 可以在显式授权后监听局域网地址：

```powershell
.\deploy.ps1 -Serial <serial> -StartServer -AllowLAN -ApiTokenFile $tokenPath
```

`-AllowLAN` 必须与 `-ApiTokenFile` 配套。需要从局域网使用 Shell 或 Web 终端时可以
同时增加 `-LegacyUnsafeAPI`；LAN 认证中间件仍会保护这些危险接口。部署脚本将令牌
以权限 `0600` 写入设备，启动参数和输出都不会包含令牌。LAN 只适合受信任且有访问
控制的网络，不应直接暴露到公网。

脚本或 API 客户端使用 `Authorization: Bearer` 请求头：

```powershell
$baseUrl = 'http://<设备WLAN-IP>:7912'
$token = [IO.File]::ReadAllText($tokenPath).Trim()
$headers = @{ Authorization = "Bearer $token" }
Invoke-RestMethod "$baseUrl/v1/health" -Headers $headers
```

查询参数名 `token`、`api_token` 或 `access_token` 会被拒绝，即使请求同时携带有效
Bearer 头也返回 HTTP 401。

### 3.4 浏览器登录与退出

浏览器打开 `http://<设备WLAN-IP>:7912/`，输入**由管理员提供的令牌**。登录请求为
`POST /v1/auth/lan/session`；成功后服务设置路径为 `/` 的 HttpOnly、
SameSite=Strict Cookie，有效期 12 小时。令牌本身不会保存在浏览器中。页面资源可以
在登录前加载，但 Primary API、业务资源和 WebSocket 都需要有效 Bearer 或会话 Cookie。
该要求针对来自 WLAN 的非回环连接；设备内部 Companion 与可信 `adb forward` 的回环
连接不要求 LAN 凭据，以保持本机组件和恢复通道可用。服务端只检查真实 TCP 对端地址，
不会信任 `X-Forwarded-For` 等代理头。

在控制台点击“退出登录”会调用 `DELETE /v1/auth/lan/session`，立即撤销当前会话并
删除 Cookie。`GET /v1/auth/lan/status` 可返回 LAN 认证是否启用以及当前浏览器是否已
认证。认证缺失、无效、过期或 WebSocket 来源不匹配时返回 HTTP 401。

### 3.5 轮换令牌与恢复默认

用 3.2 的流程生成新文件，然后重新部署并替换正在运行的 Agent：

```powershell
.\deploy.ps1 -Serial <serial> -StartServer -ReplaceRunningAgent `
  -AllowLAN -ApiTokenFile $newTokenPath
```

重启会使旧 Bearer 令牌和全部旧浏览器会话立即失效。确认新令牌可用后，安全删除旧
文件。部署切换失败时，脚本会恢复先前 Agent 和设备端令牌。

恢复默认回环模式时，不带 LAN 参数重新启动：

```powershell
.\deploy.ps1 -Serial <serial> -StartServer -ReplaceRunningAgent
```

设备端既有令牌文件可能被保留用于回滚，但不带 `--allow-lan` 时不会启用认证或非回环
监听。继续使用部署输出的 ADB 转发 URL；确认无需回滚后，可通过受控 ADB 会话删除
`/data/local/tmp/.xtest-nova-api-token`。

## 4. `--legacy-unsafe-api`

该开关开放同步 Shell、后台 Shell 和 Web 终端。部署示例：

```powershell
.\deploy.ps1 -Serial <serial> -StartServer -LegacyUnsafeAPI
.\deploy.ps1 -Serial <serial> -StartServer -AllowLAN `
  -LegacyUnsafeAPI -ApiTokenFile $tokenPath
```

直接启动设备端 Agent：

```text
adb shell /data/local/tmp/xtest-nova-agent server -d --legacy-unsafe-api
```

使用结束后，应停止 Agent，并在不携带该参数的情况下重新启动。不要把启用该开关的
未认证端口暴露到局域网、公网、反向代理或共享环境。单独启用时该模式仍只允许回环
监听；与 `--allow-lan` 或部署脚本的 `-AllowLAN` 组合时，非回环请求必须先通过 LAN
Bearer 或浏览器会话认证。完整接口说明、调用示例和关闭步骤见
[`--legacy-unsafe-api` 使用说明](../guides/legacy-unsafe-api.md)。

## 5. HTTP 兼容边界

- 77/77 表示全部方法和路径均有处理链。
- `/shell` 同时接受查询、表单和 JSON，兼容 `command`、`c`、`timeout`。
- `/shell` 固定返回 `output`、`exitCode`、`error`。
- `/stop` 会先等待 Runner 派生产物收尾，再写回 `Finished!` 并关闭 Agent；收尾超时会返回错误且不会退出 Agent。
- `/info` 保留原合同关键字段及整数 `sdk`。

Nova 的 `/v1/*`、安全路径校验、会话互斥和诊断字段属于扩展，不要求旧客户端使用。
当前已为 77 项建立可执行语义合同及高风险关键接口报文测试。77/77 表示方法、路径、
请求编码、关键字段、状态码和副作用均有对应验证。

## 6. 安全与维护边界

- 默认仅监听回环地址，不恢复 `:7912` 全网卡暴露。
- 非回环 7912 必须显式启用 LAN 模式和令牌认证；8912、7890 始终回环。
- 任意命令接口不默认启用。
- 不使用 `xtest-agent` 文件名，避免与旧 XTest/ATX 进程碰撞。
- Agent、Runner、Companion 不合并成不可维护的单体文件。
- Web 不承诺逐像素复刻旧界面；悬浮窗已对齐关键结构，30710 补齐目标应用性能、录制任务/最终文本、Monkey 高级守卫及目标页签名用例调度，30711 增加运行中撤销悬浮权限后的自动关闭与恢复；30712 建立主导航、紧凑状态、内容面板、可滚动面板和最小化五类统一尺寸规则；30714 增加平板档，紧凑面板为 320dp、性能页为 320dp×300dp，并放大标题、正文和关闭热区。手机性能窗继续保持 176dp×216dp。

Companion 不再注册可见配置 Activity；唯一导出的 `PopupLauncherActivity` 使用
`Theme.NoDisplay`，只中转启动私有悬浮服务。发布门禁要求启动前后前台应用不变、系统中
出现五项 `APPLICATION_OVERLAY` 菜单，且 `popup status` 报告 `running=true`。目标应用
只在用户从可滚动列表中选择后保存，选择动作不会隐式启动目标。
