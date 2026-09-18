# XTest Nova：历史 HTTP 兼容合同（77/77）

本文是 Nova 项目内历史 HTTP 兼容合同的**唯一主说明文档**，用于开发、联调、
回归和发布审查。它完整展开 77 个“HTTP 方法 + 路径”合同，并说明用途、主要输入
输出、Nova 实现位置和资格限制。

## 1. 范围与口径

- 仓内基线：[`reference-http-contract.md`](reference-http-contract.md)，由已审查的历史
  路由快照导入，共 72 个路由注册项；将多方法路由展开后为 77 个方法—路径合同。
- 当前实现：Agent `xtest-nova-0.26.0-m6.3-auto-popup`，合同检查结果为目标 77、覆盖 77、缺失 0。
- 本表描述设备侧 Agent 兼容端口，默认监听 `127.0.0.1:7912`。Companion 8912 接口和 Nova 新增的 `/v1/*` 接口不计入这 77 项。
- `ANY` 表示历史路由没有在注册层限制 HTTP 方法；它不是建议客户端任意选择方法。
  客户端应使用“输入/输出”列描述的常规方式。
- `77/77` 表示路径、方法和实际处理链均已存在，不表示所有实现都与旧二进制逐指令相同，也不取消设备、系统版本或安全资格限制。
- 每条方法路由另有机器校验的语义记录，包含请求编码、参数字段、成功/错误状态、响应类型/字段、副作用和运行资格；缺少任何一项都会使合同测试失败。
- Nova 是 clean-room 可维护实现：复用外部合同和运行原理，不把历史二进制恢复物
  作为生产源码。

### 双实现黄金检查

`agent/cmd/contract-golden` 可同时连接历史参考端和 Nova 候选端。默认夹具从 77 项语义表中筛选
无路径占位、只读、非流式接口，比较状态码、去除 charset 后的 Content-Type、正文类型以及去重后的
JSON 字段路径；动态值和数组条数不会造成字段形状误报。响应上限为 4 MiB，单请求超时 10 秒。

安装、文件写入、应用启停、设备输入、Monkey、录制和任意命令执行不属于安全自动探测集合，必须在
隔离设备上使用单独夹具，并带显式副作用授权、前后状态快照和清理断言。不能为了达到 77 个动态用例
而默认开放 `/shell/background` 或 `/term`。

## 2. 状态与通用约定

| 标记   | 含义                                                                                      |
|------|-----------------------------------------------------------------------------------------|
| 已实现  | 已具备可执行实现和自动或真机证据。                                                                       |
| 有限资格 | 主链路已实现，但受安全边界、设备能力或外部依赖限制；详情见[兼容矩阵](compatibility.md)和[豁免登记](compatibility-waivers.md)。 |
| 显式启用 | 默认返回 HTTP 403，只有 Agent 使用 `--legacy-unsafe-api` 启动时可用。                                  |

通用行为：

- 成功响应按历史合同返回文本、二进制、HTML 或 JSON；JSON 失败通常包含 `success: false` 和 `error`。
- 无效参数返回 400；LAN 认证缺失、无效、过期或来源检查失败返回 401；权限/安全
  开关拒绝返回 403；资源不存在返回 404，方法不允许返回 405，运行冲突返回 409，
  外部依赖不可用返回 5xx。
- 默认情况下 7912、8912 和 7890 都只监听回环地址，推荐通过可信 `adb forward`
  使用。LAN 模式只允许主 Agent 7912 非回环监听，8912 和 7890 始终回环。
  `--legacy-unsafe-api` 本身不放宽监听地址；与 LAN 模式组合时，三个危险接口与其他
  非回环 Primary 请求一样，必须先通过 Bearer 或浏览器会话认证。
- 文件接口限定在 `/data/local/tmp`、`/sdcard` 和 `/storage/emulated/0` 的受控解析范围；下载阻止回环、私网和非 HTTP(S) 目标。

## 3. 完整合同清单

“Nova 实现”列中的路径默认相对于 `agent/internal/`；例如 `httpapi/api.go` 对应仓库文件 `agent/internal/httpapi/api.go`。

### 3.1 基础、层级与进程性能

| # | 方法     | 路径                              | 功能及主要输入/输出                                             | 状态与限制                                                  | Nova 实现                   |
|--:|--------|---------------------------------|--------------------------------------------------------|--------------------------------------------------------|---------------------------|
| 1 | `ANY`  | `/version`                      | 返回 Agent 版本字符串。                                        | 已实现                                                    | `httpapi/api.go`          |
| 2 | `POST` | `/newCommandTimeout`            | 设置 UiAutomator 命令空闲超时；请求体为 1–86400 的 JSON 整数秒数，返回生效结果。 | 已实现；范围校验                                               | `httpapi/utility_handlers.go` |
| 3 | `ANY`  | `/dump/hierarchy`               | 获取当前窗口 XML 层级。                                         | 有限资格；依赖 UiAutomator，见 W-005                            | `httpapi/app_file_automation_handlers.go`  |
| 4 | `ANY`  | `/dump/hierarchyWithScreenshot` | 同时返回窗口层级和截图数据。                                         | 有限资格；依赖 UiAutomator，见 W-005                            | `httpapi/app_file_automation_handlers.go`  |
| 5 | `ANY`  | `/proc/list`                    | 返回设备进程列表及 PID/名称。                                      | 已实现                                                    | `httpapi/device_process_handlers.go` |
| 6 | `ANY`  | `/proc/{pkgname}/meminfo`       | 返回目标包主进程内存信息。                                          | 有限资格；依赖系统 `dumpsys` 输出                                 | `httpapi/device_process_handlers.go` |
| 7 | `ANY`  | `/proc/{pkgname}/meminfo/all`   | 聚合目标包及其冒号子进程内存信息。                                      | 有限资格；见 W-003                                           | `httpapi/device_process_handlers.go` |
| 8 | `ANY`  | `/proc/{pkgname}/cpuinfo`       | 返回目标包 CPU 使用信息。                                        | 有限资格；采样精度依赖 Android 版本                                 | `httpapi/device_process_handlers.go` |
| 9 | `ANY`  | `/proc/{pkgname}/perf`          | 返回目标应用多进程 CPU、内存、UID 网络及可用的 FPS、卡顿、GPU、电池聚合性能数据，并附数据来源、部分指标错误和采样耗时。 | 有限资格；普通 View FPS/卡顿已实现，SurfaceView 回退仅提供 FPS；首样本及不支持的设备来源明示为 `null`，见 W-003 | `httpapi/device_process_handlers.go` |

### 3.2 WebView、应用会话与命令执行

|  # | 方法     | 路径                    | 功能及主要输入/输出                                                                  | 状态与限制                           | Nova 实现                                |
|---:|--------|-----------------------|-----------------------------------------------------------------------------|---------------------------------|----------------------------------------|
| 10 | `ANY`  | `/webviews`           | 枚举设备上的 WebView 调试 Unix socket。                                              | 已实现；需应用开启 WebView 调试            | `httpapi/task_session_handlers.go`              |
| 11 | `ANY`  | `/webviews/{pkgname}` | 按包名过滤 WebView 调试端点。                                                         | 已实现；需目标进程存在                     | `httpapi/task_session_handlers.go`              |
| 12 | `ANY`  | `/pidof/{pkgname}`    | 返回目标包 PID；无存活进程时返回明确错误。                                                     | 已实现                             | `httpapi/device_process_handlers.go`              |
| 13 | `POST` | `/session/{pkgname}`  | 启动应用会话并返回包信息、启动结果。                                                          | 已实现；受 Android 启动策略影响            | `httpapi/task_session_handlers.go`              |
| 14 | `GET`  | `/shell`              | 从 `command`/`c` 查询参数执行同步 Shell；支持 `timeout`，返回 `output`、`exitCode`、`error`。 | **显式启用**；任意命令风险                 | `httpapi/api.go`                       |
| 15 | `POST` | `/shell`              | 接受表单或 JSON 的 `command`/`c` 与 `timeout`，返回旧合同三字段。                            | **显式启用**；任意命令风险                 | `httpapi/api.go`                       |
| 16 | `GET`  | `/shell/background`   | 从 `command`/`c` 查询参数启动后台 Shell，返回 `success`、`pid`、`description`。            | **显式启用**；命令 ≤16 KiB，最多 16 个并发任务 | `httpapi/device_process_handlers.go`、`legacyexec` |
| 17 | `POST` | `/shell/background`   | 从表单或 JSON `command`/`c` 启动后台 Shell并返回 PID。                                  | **显式启用**；子进程被持有，自然退出或 Agent 停止时回收 | `httpapi/device_process_handlers.go`、`legacyexec` |
| 18 | `ANY` | `/stop`               | 实际操作使用 `POST`；停止 Nova 自有会话和后台 Shell，等待 Runner 派生产物收尾后写回响应并优雅关闭 Agent HTTP 服务。                    | 要求 `X-XTest-Control: true`；其他方法返回 405；收尾超时拒绝退出；不干预其他进程 | `httpapi/task_session_handlers.go`              |

### 3.3 UiAutomator 服务与受控文件

|  # | 方法       | 路径                    | 功能及主要输入/输出                                       | 状态与限制                        | Nova 实现                              |
|---:|----------|-----------------------|--------------------------------------------------|------------------------------|--------------------------------------|
| 19 | `GET`    | `/services/{name}`    | 查询命名服务状态；当前兼容 `uiautomator`。                     | 有限资格；见 W-005                 | `httpapi/device_process_handlers.go`            |
| 20 | `POST`   | `/services/{name}`    | 启动命名服务。                                          | 有限资格；仅管理 Nova 所有权范围          | `httpapi/device_process_handlers.go`            |
| 21 | `DELETE` | `/services/{name}`    | 停止命名服务。                                          | 有限资格；不误杀外部所有者                | `httpapi/device_process_handlers.go`            |
| 22 | `POST`   | `/uiautomator`        | 启动 UiAutomator 服务。                               | 有限资格；依赖 Companion/后端，见 W-005 | `httpapi/app_file_automation_handlers.go`             |
| 23 | `DELETE` | `/uiautomator`        | 停止 Nova 管理的 UiAutomator 服务。                      | 有限资格；外部所有权保护                 | `httpapi/app_file_automation_handlers.go`             |
| 24 | `GET`    | `/uiautomator`        | 查询 UiAutomator 运行状态。                             | 已实现                          | `httpapi/app_file_automation_handlers.go`             |
| 25 | `ANY`    | `/raw/{filepath:.*}`  | 下载受控路径文件，返回原始字节。                                 | 有限资格；路径白名单和符号链接越界保护，见 W-002  | `httpapi/app_file_automation_handlers.go`             |
| 26 | `ANY`    | `/finfo/{lpath:.*}`   | 返回受控路径文件/目录元信息。                                  | 有限资格；见 W-002                 | `httpapi/app_file_automation_handlers.go`             |
| 27 | `ANY`    | `/info`               | 返回旧合同关键设备字段，包括整数 SDK、版本、显示、密度、电量、内存、CPU、存储和构建信息。 | 已实现；扩展字段保持向后兼容               | `device/service.go`、`httpapi/api.go` |
| 28 | `ANY`    | `/upload/{target:.*}` | 上传文件到受控路径并返回文件信息。                                | 有限资格；最大 512 MiB、原子写入、路径保护    | `httpapi/app_file_automation_handlers.go`             |

### 3.4 APK、下载与安装任务

|  # | 方法       | 路径                               | 功能及主要输入/输出                                   | 状态与限制                           | Nova 实现                           |
|---:|----------|----------------------------------|----------------------------------------------|---------------------------------|-----------------------------------|
| 29 | `ANY`    | `/installLocalApk/{apkfilename}` | 安装 `/data/local/tmp/apk` 中的指定 APK，完成后清理暂存文件。 | 有限资格；仅接受安全基名和 `.apk`            | `httpapi/device_process_handlers.go`         |
| 30 | `ANY`    | `/installAgentApk`               | 检查并安装随 Agent 暂存的 Nova Companion APK。         | 有限资格；Android 16 USB 安装策略见 W-012 | `httpapi/device_process_handlers.go`         |
| 31 | `ANY`    | `/installApk`                    | 接收 APK 内容并执行安装。                              | 有限资格；安装权限和设备策略见 W-001           | `httpapi/app_file_automation_handlers.go`          |
| 32 | `POST`   | `/download`                      | 创建公开 HTTP(S) 下载任务，返回任务 ID。                   | 已实现；阻止回环/私网/协议绕过                | `httpapi/task_session_handlers.go`、`tasks` |
| 33 | `ANY`    | `/download/{key}`                | 查询下载任务状态、进度、字节数和错误。                          | 已实现；完成历史有界保留                    | `httpapi/task_session_handlers.go`、`tasks` |
| 34 | `POST`   | `/packages`                      | 从表单 `url` 创建异步 APK 下载并安装任务，返回任务 ID。          | 有限资格；仅公开 HTTP(S)，受安装策略限制        | `httpapi/task_session_handlers.go`         |
| 35 | `GET`    | `/packages`                      | 查询已安装应用列表；可按系统应用条件过滤。                        | 已实现                             | `httpapi/app_file_automation_handlers.go`          |
| 36 | `ANY`    | `/packages/{id}`                 | 查询异步包安装任务的状态和进度；任务不存在时返回 404。                    | 有限资格；ID 必须属于本进程任务               | `httpapi/task_session_handlers.go`         |
| 37 | `ANY`    | `/packages/{pkgname}/info`       | 返回应用版本、路径、系统属性等包信息。                          | 已实现                             | `httpapi/app_file_automation_handlers.go`          |
| 38 | `ANY`    | `/packages/{pkgname}/icon`       | 从 APK 提取并返回应用图标。                             | 已实现；图标缺失返回明确错误                  | `httpapi/utility_handlers.go`         |
| 39 | `POST`   | `/install`                       | 创建旧协议兼容安装任务。                                 | 有限资格；URL/文件来源受控                 | `httpapi/task_session_handlers.go`         |
| 40 | `GET`    | `/install/{id}`                  | 查询旧协议安装任务状态。                                 | 已实现                             | `httpapi/task_session_handlers.go`         |
| 41 | `DELETE` | `/install/{id}`                  | 取消/移除旧协议安装任务。                                | 已实现；仅作用于 Nova 自有任务              | `httpapi/task_session_handlers.go`         |

### 3.5 触控、事件、监控与媒体

|  # | 方法       | 路径                            | 功能及主要输入/输出                            | 状态与限制                                  | Nova 实现                                  |
|---:|----------|-------------------------------|---------------------------------------|----------------------------------------|------------------------------------------|
| 42 | `PUT`    | `/minitouch`                  | 修复/启动 minitouch 进程。                   | 有限资格；缺可信 ABI 二进制时不可执行，见 W-006          | `httpapi/task_session_handlers.go`、`minitouch`    |
| 43 | `DELETE` | `/minitouch`                  | 停止 Nova 管理的 minitouch。                | 已实现；仅停止自有进程                            | `httpapi/task_session_handlers.go`、`minitouch`    |
| 44 | `GET`    | `/minitouch`                  | 将 WebSocket 与 minitouch socket 双向桥接。  | 有限资格；协议和并发已测，真机触控见 W-006               | `httpapi/task_session_handlers.go`、`minitouch`    |
| 45 | `ANY`    | `/touchreader`                | WebSocket 输出触摸输入事件。                   | 有限资格；Protocol-B 已实现，Protocol-A 见 W-008 | `httpapi/web_console_handlers.go`、`touchreader`  |
| 46 | `ANY`    | `/appevent/info`              | 返回 App Event 能力和当前状态。                 | 已实现                                    | `httpapi/streaming_handlers.go`                |
| 47 | `ANY`    | `/appeventmonitor`            | WebSocket 发布应用事件文本流。                  | 已实现；有界队列，慢消费者不阻塞发布者                    | `httpapi/streaming_handlers.go`、`events`       |
| 48 | `PUT`    | `/monitor`                    | WebSocket 桥接 7890 性能监控行式 JSON 流。      | 有限资格；FPS 资格见 W-003                     | `httpapi/streaming_handlers.go`、`monitor`      |
| 49 | `GET`    | `/minicap/broadcast`          | 返回旋转信息并持续输出 PNG 截图帧。                  | 有限资格；系统截图降级，不伪装原生高帧率，见 W-007           | `httpapi/streaming_handlers.go`                |
| 50 | `GET`    | `/minicap`                    | 提供 minicap 兼容 WebSocket 截图流。          | 有限资格；见 W-007                           | `httpapi/streaming_handlers.go`                |
| 51 | `ANY`    | `/scrcpy/{type}/{definition}` | 提供 scrcpy 画面或控制 WebSocket；路径指定通道和清晰度。 | 有限资格；固定官方 server 哈希，矩阵见 W-010          | `httpapi/scrcpy_handlers.go`、`scrcpy`       |
| 52 | `POST`   | `/screenrecord`               | 启动受控屏幕录制并返回会话信息。                      | 已实现；单实例、目录隔离                           | `httpapi/streaming_handlers.go`、`screenrecord` |
| 53 | `PUT`    | `/screenrecord`               | 停止录屏并返回 MP4 结果信息。                     | 已实现；停止期间拒绝重入                           | `httpapi/streaming_handlers.go`、`screenrecord` |

### 3.6 终端、截图、输入法、系统与配置

|  # | 方法       | 路径                   | 功能及主要输入/输出                                                            | 状态与限制                                    | Nova 实现                                    |
|---:|----------|----------------------|-----------------------------------------------------------------------|------------------------------------------|--------------------------------------------|
| 54 | `ANY`    | `/term`              | 普通请求返回内嵌终端页；WebSocket 使用 PTY。二进制首字节 `0` 为输入，`1` 为 `{cols,rows}` 调整尺寸。 | **显式启用**；帧 ≤64 KiB，最多 4 会话，断线终止并回收 shell | `httpapi/terminal_handler.go`、`legacyterm` |
| 55 | `ANY`    | `/screenshot`        | 捕获并返回当前屏幕 PNG。                                                        | 已实现                                      | `httpapi/app_file_automation_handlers.go`                   |
| 56 | `ANY`    | `/u2packages`        | 返回 UiAutomator/Companion 相关包及服务状态。                                    | 已实现；结果取决于设备安装状态                          | `httpapi/device_process_handlers.go`                  |
| 57 | `ANY`    | `/imeStatus`         | 返回当前输入法和可用输入法状态。                                                      | 已实现                                      | `httpapi/device_process_handlers.go`                  |
| 58 | `ANY`    | `/setIme`            | 使用 `ime`/`id`/`inputMethod` 切换输入法。                                    | 有限资格；需要设备已安装并允许目标 IME，见 W-004            | `httpapi/device_process_handlers.go`                  |
| 59 | `ANY`    | `/network/info`      | 返回网络接口、地址和连接状态。                                                       | 已实现                                      | `httpapi/device_process_handlers.go`、`system`         |
| 60 | `ANY`    | `/disk/info`         | 返回设备存储容量和使用信息。                                                        | 已实现                                      | `httpapi/device_process_handlers.go`、`system`         |
| 61 | `POST`   | `/popupBoxAssistant` | 启动配置驱动的 AutoPopup 助手。                                                 | 已实现；保持原四字段规则、上下文匹配及 WindowClose 伴随执行语义 | `httpapi/utility_handlers.go`、`autopopup`      |
| 62 | `DELETE` | `/popupBoxAssistant` | 停止 AutoPopup 助手。                                                      | 已实现；幂等停止                                 | `httpapi/utility_handlers.go`、`autopopup`      |
| 63 | `POST`   | `/pushConfig`        | 原子替换并持久化 Companion/Popup 配置 JSON。                                     | 要求 `X-XTest-Control: true`                 | `httpapi/api.go`、`configstore`             |
| 64 | `GET`    | `/pullConfig`        | 返回当前持久化配置 JSON。                                                       | 已实现                                      | `httpapi/api.go`、`configstore`             |

### 3.7 静态资源、设备辅助与控制台

|  # | 方法    | 路径                     | 功能及主要输入/输出                          | 状态与限制                               | Nova 实现                                 |
|---:|-------|------------------------|-------------------------------------|-------------------------------------|-----------------------------------------|
| 65 | `ANY` | `/assets/{(.*)}`       | 返回内嵌兼容 Web 资源。                      | 已实现；无 CDN，未知资源 404                  | `httpapi/web_console_handlers.go`、`httpapi/web` |
| 66 | `ANY` | `/screenshot/0`        | `/screenshot` 的历史兼容别名，返回 PNG。       | 已实现                                 | `httpapi/task_session_handlers.go`               |
| 67 | `ANY` | `/wlan/ip`             | 返回 wlan0 IPv4 地址。                   | 已实现；无地址时明确失败                        | `httpapi/task_session_handlers.go`               |
| 68 | `ANY` | `/device/memory`       | 返回设备总体内存信息。                         | 已实现                                 | `httpapi/device_process_handlers.go`、`system`      |
| 69 | `GET` | `/foregroundPkg`       | 返回当前前台包名。                           | 已实现；兼容 Android 13/16 dumpsys 格式     | `httpapi/api.go`、`device`               |
| 70 | `GET` | `/wakeupScreen`        | 发送幂等 `KEYCODE_WAKEUP` 唤醒屏幕。         | 已实现；不会关闭已点亮屏幕                       | `httpapi/api.go`、`device`               |
| 71 | `ANY` | `/jsonrpc/0`           | 兼容代理到固定回环 `127.0.0.1:9008/jsonrpc/0`。 | 默认 404；仅显式传入 `--legacy-uiautomator` 后启用，请求 ≤4 MiB、响应 ≤16 MiB | `httpapi/web_console_handlers.go`               |
| 72 | `ANY` | `/ping`                | 返回服务存活 JSON。                        | 已实现                                 | `httpapi/api.go`                        |
| 73 | `ANY` | `/static/js/{(.*)}`    | 返回内嵌 JavaScript 资源。                 | 已实现；白名单映射、CSP                       | `httpapi/web_console_handlers.go`、`httpapi/web` |
| 74 | `ANY` | `/static/css/{(.*)}`   | 返回内嵌 CSS 资源。                        | 已实现；白名单映射、CSP                       | `httpapi/web_console_handlers.go`、`httpapi/web` |
| 75 | `ANY` | `/static/media/{(.*)}` | 返回内嵌媒体资源。                           | 已实现；未知资源 404                        | `httpapi/web_console_handlers.go`、`httpapi/web` |
| 76 | `ANY` | `/{(.*)}`              | Web 控制台 SPA 回退；包含扩展名的未知资源不回退。       | 已实现                                 | `httpapi/web_console_handlers.go`               |
| 77 | `ANY` | `/`                    | 返回 Nova 内嵌 Web 控制台首页。               | 已实现；无 CDN，CSP 限制                    | `httpapi/web_console_handlers.go`、`httpapi/web` |

## 4. 三个任意命令入口

`GET/POST /shell/background` 和 `ANY /term` 属于历史兼容合同。Nova 完整实现协议，
但与 `/shell` 一起默认关闭：

```text
默认启动                         -> /shell、/shell/background、/term 返回 403
增加 --legacy-unsafe-api         -> 三类任意命令能力启用
```

这个开关不是认证或授权机制。启用后，能够访问端口的程序可以设备 shell 身份读写文件、管理应用和启动进程。详细决策见 [ADR-0003](../adr/0003-gated-atx-command-compatibility.md)。

## 4.1 导出机器可读语义合同

```powershell
go run -mod=vendor ./agent/cmd/contract-check -reference .\docs\compliance\reference-http-contract.md -semantics-json
```

输出固定为 77 个对象。实现新增、删除或遗漏请求/响应语义时，`contract.Semantics`
和发布门禁会直接失败。

## 5. 77 项之外的认证与扩展接口

LAN 认证新增以下 Primary 路由：

- `POST /v1/auth/lan/session`：提交 JSON `{"token":"..."}`；成功返回 201，并设置
  `xtest_nova_lan_session` Cookie。Cookie 为 HttpOnly、SameSite=Strict、路径 `/`，
  默认有效期 12 小时；请求体只允许一个 JSON 值，重新登录会轮换并撤销旧会话，
  进程内会话上限为 128，满额时淘汰最早会话；
- `DELETE /v1/auth/lan/session`：撤销当前浏览器会话、删除 Cookie 并返回 200；
- `GET /v1/auth/lan/status`：返回 `enabled` 和 `authenticated`，用于登录页判断状态。

LAN 模式下，登录页所需的固定静态资源和以上认证端点可在认证前访问；除此以外，
来自非回环地址的 Primary 请求，包括兼容路由、`/v1/*` API、业务资源和所有
WebSocket，都需要有效的 `Authorization: Bearer <令牌>` 或会话 Cookie。设备内部
Companion 与可信 ADB 转发形成的 TCP 回环连接豁免 LAN 凭据，服务端不读取
`X-Forwarded-For` 等代理头建立信任。WebSocket 还执行同源检查。认证失败
统一返回 401 和 `WWW-Authenticate: Bearer`。任何名为 `token`、`api_token` 或
`access_token` 的 URL 查询参数都会被拒绝，客户端不得把令牌放入 URL。
Bearer 头必须唯一且只包含一个凭据；浏览器 Origin 按方案、规范化主机（含 IPv6）
和有效端口比较。登录页公开面只包含首页及其固定 CSS、JavaScript、图标和占位图，
文件、截图、归档、终端、远控媒体及其他 Primary 资源均不在公开面内。

Nova 还提供版本化的 `/v1/*` 能力，例如健康检查、结构化 Monkey、智能遍历、录制
回放和运行时诊断；Companion 在 8912 提供 Monkey、配置、层级与截图接口。这些认证
端点、版本化扩展和其他端口合同都在 77 项历史方法—路径合同之外，**不得计入 77**。

新增或修改历史兼容接口时，必须同步更新：

1. 本文对应条目；
2. `agent/internal/contract/contract.go` 的机器清单；
3. 相关处理器与自动测试；
4. `docs/compliance/compatibility.md` 的资格结论；
5. 最新验证报告和发布清单。

## 6. 验证方式与证据

```powershell
go run -mod=vendor ./agent/cmd/contract-check -reference .\docs\compliance\reference-http-contract.md -document .\docs\compliance\http-contract.md
go vet ./agent/...
go test ./agent/...
.\\scripts\\test-race.ps1 -Count 3
.\scripts\test-release.ps1
```

当前证据：

- 合同检查：目标 77、实现 77、覆盖 77、缺失 0；
- 全量 Go 测试、静态检查和竞态检测通过；
- Android 13 与 Android 16 双机兼容验证；
- 后台 Shell GET/POST、PID、落盘、PTY 输入、窗口尺寸、回传和断线清理通过；
- 默认模式下 `/shell`、`/shell/background`、`/term` 返回 403；
- 详细记录见 [M5.8C 验证报告](../validations/milestones/validation-m5.8-contract77.md)。

## 7. 历史文档说明

`validation-m2.*`、`validation-m3.*`、`validation-m4.*` 和早期 `validation-m5.*` 中的 18/77、38/77、74/77 等数字是当时里程碑的真实快照，为审计目的保留。它们不代表当前状态；当前合同结论以本文、兼容矩阵和最新 M5.8C 验证报告为准。
# Nova diagnostics extensions

The following Nova-only endpoints are versioned separately from legacy compatibility routes:

- `GET /v1/diagnostics/runtime` returns process memory, goroutine, session, and hierarchy-provider counters.
- `GET /v1/diagnostics/components` returns `xtest-nova-components/v1`, including supported, ready, running, state, version, and degradation detail per component.
- `GET /v1/diagnostics/artifacts?package=&kind=&limit=` lists validated sessions below `/sdcard/xtest-nova`; `limit` is restricted to 1-200 and path traversal is rejected.
- `GET /v1/diagnostics/logs/{name}?lines=&contains=` tails only fixed log aliases (`agent`, `minitouch`); it does not accept a filesystem path.
- `POST /v1/diagnostics/soak` starts a 60-second to 7-day bounded runtime sampling session. The interval is 5-300 seconds.
- `GET|DELETE /v1/diagnostics/soak/current` reads or stops the current sampling session. Samples are streamed to `/sdcard/xtest-nova/device.runtime/Diagnostics/<timestamp>/soak.jsonl`; only 120 recent samples remain in memory.

Companion and Monitor are auxiliary listeners. A bind conflict is reusable only after service identity and exact protocol/version handshake; an incompatible occupant degrades that component without terminating the primary Agent listener.
