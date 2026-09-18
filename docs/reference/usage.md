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

`server -d --stop` 明确定义为停止，不隐式重启。需要重启时先执行停止命令，再执行
`server -d`，避免升级脚本出现双进程竞态。只推送二进制文件不会安装或启动任何 APK；执行
Server 后才会进行 bootstrap。正常启动默认显示悬浮窗，只有显式 `--no-popup` 才跳过显示。

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
