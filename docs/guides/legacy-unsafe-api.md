# `--legacy-unsafe-api` 使用说明

## 功能

`--legacy-unsafe-api` 是高风险兼容开关。启用后，Agent 允许客户端以设备
shell 身份执行任意命令，并开放以下接口：

- `GET/POST /shell`：同步执行命令并返回输出、退出码和错误；
- `GET/POST /shell/background`：启动后台命令并返回进程 PID；
- `GET /term`：打开 Web 终端，终端交互使用同路径的 WebSocket。

默认不开启时，三个接口均返回 HTTP 403。该开关只改变危险接口是否可用，不放宽任何
监听地址。单独使用时 Agent 7912、Companion 8912 和 Monitor 7890 都必须保持回环
监听；它可以与 `--allow-lan` 或部署脚本的 `-AllowLAN` 组合，此时 LAN 认证中间件
同样保护三个危险接口。

## 安全边界

该开关不提供认证、授权或命令隔离。能够访问 Agent 端口的客户端将获得与 Agent
进程相同的设备 shell 权限，可能读取设备数据、安装或删除应用、结束进程以及修改系统
设置。

仅在以下条件全部满足时使用：

1. 设备由当前操作者控制；
2. 使用回环监听与可信 USB/ADB 转发，或者显式启用带令牌认证的 LAN 模式；
3. LAN 模式仅连接受信任且有访问控制的局域网；
4. 使用结束后立即停止并以默认配置重启 Agent。

禁止将未认证且启用该开关的端口暴露到局域网、公网、反向代理或共享开发环境。
需要在可信局域网使用这些接口时，必须同时使用带令牌认证的
[`-AllowLAN` 模式](../reference/usage.md)。

## 启用

本机开发运行：

```powershell
go run ./agent/cmd/xtest-nova-agent server --legacy-unsafe-api
```

部署并启动设备端 Agent：

```powershell
.\deploy.ps1 -Serial <设备序列号> -StartServer -LegacyUnsafeAPI
```

在可信局域网启用，并让危险接口接受同一套 Bearer/Cookie 认证：

```powershell
$tokenPath = Join-Path $HOME '.xtest-nova-api-token'
.\scripts\new-lan-token.ps1 -OutputPath $tokenPath
.\deploy.ps1 -Serial <设备序列号> -StartServer -AllowLAN `
  -LegacyUnsafeAPI -ApiTokenFile $tokenPath
```

直接在设备端启动：

```text
adb shell /data/local/tmp/xtest-nova-agent server -d --legacy-unsafe-api
```

`deploy.ps1` 默认只把设备端 7912 转发到本机端口，不需要把监听地址改为
`0.0.0.0`。如同时连接多台设备，请使用部署命令输出的独立本机端口。

## 调用示例

先按部署输出建立或确认 ADB 转发，再调用相应本机地址：

```powershell
$baseUrl = 'http://127.0.0.1:7912'

Invoke-RestMethod "$baseUrl/shell" -Method Post `
  -ContentType 'application/json' `
  -Body '{"command":"id","timeout":15}'

Invoke-RestMethod "$baseUrl/shell/background" -Method Post `
  -ContentType 'application/json' `
  -Body '{"command":"logcat -d > /data/local/tmp/xtest-logcat.txt"}'

Start-Process "$baseUrl/term"
```

`/shell` 的 `command` 可简写为 `c`，`timeout` 范围为 1 至 3600 秒，默认 60
秒。后台命令最大 16 KiB，同时最多运行 16 个；达到上限时返回 HTTP 429。

## 验证与关闭

未启用时，可用无副作用命令确认接口返回 HTTP 403。启用后，只使用 `id` 等只读命令
进行验证：

```powershell
Invoke-WebRequest 'http://127.0.0.1:7912/shell?command=id'
```

关闭步骤：

```powershell
.\deploy.ps1 -Serial <设备序列号> -StartServer -ReplaceRunningAgent
```

`-ReplaceRunningAgent` 明确授权部署脚本替换正在运行的 Agent。新进程未携带
`-LegacyUnsafeAPI`，危险接口恢复为 HTTP 403。也可先执行设备端停止命令，再不带该
参数启动：

```text
adb shell /data/local/tmp/xtest-nova-agent server -d --stop
adb shell /data/local/tmp/xtest-nova-agent server -d
```
