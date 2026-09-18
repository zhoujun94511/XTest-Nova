# M5.8G/F 语义合同与用户流程验证

验证日期：2026-09-04。Agent：`xtest-nexus-0.22.0-m5.9-compat`。

## M5.8G：77 项语义合同

`agent/internal/contract/semantics.go` 为每个方法路由登记：

- 请求编码，以及路径、查询和请求体字段；
- 成功状态和可能的错误状态；
- 响应载体及关键字段；
- 只读、配置、进程、文件、安装、输入、录屏等副作用分类；
- UiAutomator、暂存 APK、危险接口开关等运行资格。

自动检查确认目标 77、实现 77、路径覆盖 77、语义合同 77、文档差异 0。任何路由缺少
语义记录或关键类别为空都会导致单元测试和发布门禁失败。完整 JSON 可通过：

```powershell
go run -mod=vendor ./agent/cmd/contract-check -reference .\docs\compliance\reference-http-contract.md -semantics-json
```

危险 Shell、文件写入、应用安装、输入法、服务启停和媒体类合同继续由隔离夹具、
路径边界、所有权检查及真机回滚脚本约束，不在日常门禁中对用户数据执行任意副作用。

## M5.8F：统一用户任务流

`tests/e2e/validate-user-flows.ps1` 聚合既有 Runner 与 M5.6 门禁，并新增真实 Web/PTY 和
Companion UI 流程。早期 Android 13 与 Android 16 报告中的 Monkey、Popup Assistant、
录制回放、流式和终端流程均通过；悬浮窗部分的早期口径不足，见下方纠偏记录：

- 隔离夹具 Monkey 启动、幂等、并发拒绝、停止和日志停止门禁；
- Popup Assistant 唯一安全文本匹配和停止；
- 录制、完整性封装、截图断言、校验及 2/2 动作确定性回放；
- App Event、minicap 截图降级流和 7890 Monitor；
- Web 控制台和终端页加载；
- PTY WebSocket 二进制输入、100×30 resize、命令标记回显及会话退出；
- Android 13 的 Companion 安装、无界面入口、直接显示控制器、OverlayService 运行状态和卸载闭环。

Android 16 上已存在版本 10248 的 Popup。验收只读取状态，未覆盖或卸载用户安装。
Android 13 测试前没有 Popup，测试后已卸载。两台设备的临时 Agent、Runner、夹具、
用例、日志、PID、端口转发和前台应用均由脚本清理或恢复。

早期验收曾由脚本进入配置页并点击“显示悬浮控制器”，只能证明 UI 手动链路可用，
不能证明 `popup start` 与原 XTest 的直接启动体验一致。30702 起改用
`PopupLauncherActivity` 无界面入口，并将“无残留前台页面、控制器直接可见、
`running=true`”列为同一条门禁，原结论不再作为命令行直接启动的证据。

完成上述通过结果后再次尝试单命令全量复跑时，小米 Android 16 在短时间重复安装验证
夹具阶段返回 `INSTALL_FAILED_USER_RESTRICTED: Install canceled by user`；Android 13
仍完整通过。该限制属于 W-012 厂商 USB 安装确认策略，不是 Runner、录制回放或终端
回归。脚本现在先写 `.attempt` 报告，只有阶段成功才替换正式报告，失败证据保存为
`.failed`，避免后续环境失败覆盖最近一次通过记录。

完整执行：

```powershell
.\tests\e2e\validate-user-flows.ps1 -Serial <Android16序列号>,<Android13序列号>
```

调试新增的 Web/PTY/Companion 段时可复用刚通过的 Runner 和 M5.6 报告：

```powershell
.\tests\e2e\validate-user-flows.ps1 -Serial <序列号> -SkipEstablished
```

`-SkipEstablished` 只用于局部调试；正式发布证据必须执行完整命令。

## 2026-09-04 悬浮窗直启纠偏

发现 30701 的 `popup start` 实际打开 `PopupActivity` 配置页，必须由验收脚本代替用户
点击“显示悬浮控制器”才能启动服务。这不能证明命令行为与原 XTest 一致，因此原
“悬浮窗流程通过”结论撤回。

30702 已完成：

- Agent 改为启动 `PopupLauncherActivity` 无界面入口；
- Launcher 直接启动私有 `OverlayService` 并在 `onCreate` 结束，不进入最近任务；
- 首次无保存配置时传入启动前的前台应用作为默认 Monkey 目标，已有配置不覆盖；
- `popup start` 最多等待 3 秒确认服务真实运行，失败返回非零；
- `popup status` 新增 `running=true/false`；
- 门禁检查前台 Activity 不变、`APPLICATION_OVERLAY` 窗口存在、服务运行且版本为
  30702。

Android 13（Samsung，SDK 33）隔离真机结果：

- 启动前后前台均为 `com.xtest.nova.fixture/.ValidationActivity`；
- 命令输出 `popup overlay started`；
- 状态为 `installed=true running=true versionCode=30702`；
- 系统窗口为 `ty=APPLICATION_OVERLAY`；
- 点击“开始”后 Runner 进入运行态，进程参数目标为
  `-p com.xtest.nova.fixture`，随后立即停止；
- 测试 Popup、验证应用、Agent、Runner、转发和临时文件均已清理。

Android 16（Xiaomi，SDK 36）保留用户现有原 XTest Popup 10248，不用 Nova 30702
覆盖。只读/启动对照确认 10248 启动后原前台应用不变，并出现原五项悬浮菜单。这证明
目标交互基线有效，但不构成 Nova 30702 在 Android 16 上的安装证明。

上述 30702 结论是历史纠偏记录。30707 完成菜单、原版尺寸、前台保持、性能单位/滚动与
持续采样。30708 进一步统一 `/sdcard/xtest-nexus/<目标包名>` 产物根目录，并在 Android 13
真机验证普通 View FPS、KGSL GPU、电池电流/电量/温度和扩展 CSV；目标标题及目录均来自
被测应用，不再使用测试工具名称。详见 [`popup-alignment-audit.md`](../../audits/popup-alignment-audit.md)、[`validation-popup-30707.md`](../popup/validation-popup-30707.md)
和 [`validation-popup-30708.md`](../popup/validation-popup-30708.md)。30709 又补齐任务/用例目录、聚焦非密码输入框最终文本，
以及 Monkey 低电量、Activity 名单、目标页观测和控件黑名单，详见
[`validation-popup-30709.md`](../popup/validation-popup-30709.md)。30710 通过一次性令牌和同步回调完成目标页签名用例的暂停、
执行与恢复，并扩大性能窗、补充电池状态语义，详见 [`validation-popup-30710.md`](../popup/validation-popup-30710.md)。M5.8F2
已关闭。30711 随后以与原 10248 并存的同字节码隔离包完成 Android 16 完整任务流、
运行中撤权、无权限拒绝、恢复与服务重建，详见 [`validation-popup-30711.md`](../popup/validation-popup-30711.md)；M5.8F3
与 M5.8F 均已关闭。
