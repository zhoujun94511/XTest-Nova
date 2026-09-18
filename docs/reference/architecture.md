# XTest Nova 总体架构

## 目标

Nova 保持 XTest 已验证的控制方式、端口和关键数据合同，同时把可维护性作为首要约束：

1. 新生产源码不得依赖反编译类名和不可解释控制流；
2. Android 普通应用身份与 shell 权限能力必须隔离；
3. 外部兼容协议与内部领域模型必须隔离；
4. 每项兼容能力必须有自动化合同，不能靠返回固定成功值占位。

## 运行结构

```text
控制端 / 浏览器
        |
        | 默认：ADB forward -> 回环 HTTP 7912
        | 可选：受认证 LAN -> 非回环 HTTP 7912
        v
Nova Agent (Go)
  |-- runtime bundle Runner/Companion/UiAutomator 内嵌载荷与摘要
  |-- runtime bootstrap 原子释放、安装校验和失败回滚
  |-- /v1/* 现代、版本化 API
  |-- legacy adapter XTest 兼容入口
  |-- device service 设备查询与命令边界
  |-- runner manager 生命周期、幂等和日志
  |-- scrcpy manager 官方载荷校验、会话和协议桥
  |-- exploration 页面状态图、安全规则和确定性策略
  |-- recordreplay 触点归并、用例完整性和受限时间轴回放
  |-- diagnostics 运行时资源与会话状态观测
  |-- pkgmeta 调用 Companion 的系统 PackageManager 元数据桥，不解析 APK
  |-- hierarchy providers 控件树来源、回退与采集指标
        |
        | app_process + CLASSPATH
        v
Nova Runner (Java/Dex, shell UID)
  |-- 目标包启动
  |-- 点击/滑动/按键事件
  |-- 停止标记与结构化日志

Nova UiAutomator（独立 Instrumentation 生命周期）
  |-- host/test 两个 APK，与 Companion 生命周期隔离
  |-- 仅回环 HTTP、随机令牌、响应限制和空闲退出
  |-- 首期只读取 Accessibility 控件树，不接管输入

Nova Companion (Android application UID)
  |-- 悬浮窗权限与界面
  |-- 权限缺失时安全停止，不保留失效前台服务
  |-- 目标包、时长、节流配置
  |-- HTTP 8912 -> Nova Agent
  |-- 受 DUMP 权限与 shell/root UID 双重限制的只读包元数据 Provider
```

## 模块边界

### Agent

- `cmd/xtest-nova-agent`：进程入口和信号处理；
- `internal/config`：启动配置；
- `internal/device`：Android 命令与设备信息；
- `internal/pkgmeta`：通过系统 `content` 命令读取 Companion 提供的 Activity 清单和有界 JPEG 图标；
- `internal/runner`：Runner 状态机；
- `internal/scrcpy`：官方 scrcpy server 固定哈希、独立进程/Socket 生命周期和控制帧编码；
- `internal/exploration`：UiAutomator XML 领域模型、状态指纹、候选动作、状态图和遍历生命周期；
- `internal/recordreplay`：标准化动作模型、用例落盘、完整性校验和受限回放生命周期；
- `internal/automation`：Provider 中立 Snapshot、`system-dump`/自有服务，以及仅在 `--legacy-uiautomator` 开启时可用的旧 9008 兼容 Provider；
- `internal/httpapi`：版本化 API，以及只读运行时诊断；
- `internal/compat`：历史协议适配，不承载业务规则；
- `internal/configstore`：8912 配置持久化。
- `internal/runtimebundle`：内嵌四个正式运行组件并验证清单、大小和 SHA-256；
- `internal/runtimebootstrap`：以事务方式释放 Runner；三个 APK 同时变化时使用 Android 多包原子安装会话，旧设备不支持时回退逐包安装，并维护组件状态。

### Runner

Runner 不依赖 Companion，也不提供 UI。它以受控参数启动，并以 shell UID 执行最小权限集合。未来的 UiAutomation/控件树能力也必须通过 `InputDevice` 接口接入，避免污染事件策略。

### Companion

Companion 管理 Android 应用生命周期、悬浮权限和展示，并以 Android `PackageManager` 提供只读包元数据。元数据 Provider 只接受系统 `DUMP` 权限且再次校验调用 UID 为 shell/root；它不执行长任务、不开放写入，也不直接解析 APK。所有长任务由 Agent/Runner 持有，Activity 或 Service 重建不会让测试任务失控。悬浮权限在服务创建和再次启动时都要复核；权限缺失或窗口令牌失效时安全停止服务。

Companion 有意保留唯一导出的 `PopupLauncherActivity`。它是供 Agent 和受控
`adb shell am start` 调用的无界面启动入口，只负责校验目标包参数、启动未导出的
`OverlayService` 并立即结束，不是可浏览的配置页面。validation fixture 中
`ValidationActivity` 及故障、生命周期、WebView、权限、对话框、表单和图形场景的导出
Activity 也有意保留，供外部回归编排精确启动测试场景；fixture 不属于产品运行时或发布
清单，不能据此放宽 Companion 或正式组件的导出边界。

### UiAutomator

自有 UiAutomator 使用独立 host/test APK，不绑定 Companion。U8 资格门槛完成前默认使用 system dump；`shadow` 用于串行对照，`nova` 仅供显式资格验证。Nova 主模式启动失败或运行时读取失败时仍回退到 system dump。影子模式用于结构比对，不与 Nova 主模式同时运行。完整演进记录见 [[`uiautomator-provider-plan.md`](../plans/uiautomator-provider-plan.md)](../plans/uiautomator-provider-plan.md)。

## 单文件交付与启动边界

- ARM64 与 ARMv7 各构建一个自包含 Agent；控制端向单台设备只推送与其 ABI 对应的一个文件。
- Android 仍要求 Companion、UiAutomator host/test 作为独立 APK 包安装，但该过程完全由 Agent 启动 bootstrap 执行；首次安装或整组升级只提交一个多包安装事务。
- 内嵌 APK 摘要与已安装 APK 一致时不创建安装会话，也不会重复触发安装确认。
- bootstrap 完成后，Server 默认拉起 Companion 悬浮服务，再开放 HTTP 监听；启动结果写入 `popupOverlayStartup` 组件诊断。
- `--no-popup` 只跳过悬浮服务拉起，不跳过 Runner、Companion 或 UiAutomator 的校验和安装；用于无界面门禁和维护任务。
- bootstrap 完成前不开放 HTTP 监听；任何内嵌哈希、安装或回滚失败都会终止启动并写入守护日志。悬浮服务启动失败会以降级组件状态暴露，不导致 Agent 进程消失，便于诊断和恢复。
- Runner/Companion/UiAutomator 是产品运行时；validation fixture、Foloy、Spoly 和其他目标应用仅是测试输入，始终排除在运行时包与发布清单之外。

## 安全边界

- 默认模式下，Agent 7912、Companion 8912 和 Monitor 7890 都只监听回环地址，
  控制端通过可信 `adb forward` 使用；
- 只有主 Agent 7912 可以通过 `--allow-lan --listen 0.0.0.0:7912
  --api-token-file <文件>` 显式开放到受信任局域网；令牌至少 32 字节，所有 Primary
  API、业务资源和 WebSocket 对非回环客户端要求 Bearer 或浏览器会话 Cookie，登录页
  静态资源与认证端点除外；设备内部组件与可信 ADB 转发按真实 TCP 回环地址豁免，
  `X-Forwarded-For` 不参与信任判断；
- Companion 8912 和 Monitor 7890 无论何种模式都保持回环监听；
- 任意 Shell 接口默认关闭，兼容环境必须显式传入 `--legacy-unsafe-api`；该危险模式
  本身不提供认证且不放宽监听；与 LAN 模式组合时，非回环访问仍必须通过统一的
  Bearer 或浏览器会话认证；
- 旧 9008 JSON-RPC 与旧 UiAutomator Instrumentation 默认关闭，兼容环境必须显式传入 `--legacy-uiautomator`；
- 包名、请求 ID、事件参数均在进入进程边界前验证；
- 目标 APK 视为不可信输入；Activity 与图标均通过 Android `PackageManager` 获取，Agent 不解析二进制 Manifest 或 `resources.arsc`；
- 签名证书和口令不得进入 Git；
- 旧 DEX 仅允许放在仓库外的迁移证据目录，不得成为 Nova 构建依赖。
- scrcpy JAR 仅使用官方未修改发布载荷，版本、哈希与 Apache-2.0 许可证随源码固定记录。
- 智能遍历默认只预览；执行模式必须显式启用，且目标应用离开前台时立即停止。
- 回放用例必须通过 SHA-256 完整性校验并显式设置 `execute=true`；每个动作执行前重新确认目标应用前台状态。
