# Server 单命令启动悬浮窗 Todo（2026-09-12）

目标：对齐 Nexus 的默认完整功能启动体验，使 `xtest-nova-agent server -d` 自动完成运行组件安装并拉起 Companion 悬浮窗，不再要求新人追加第二条 `popup start`。

- [x] A1 增加默认启用的 Server 启动悬浮窗生命周期，并确保 bootstrap 安装完成后才执行。
- [x] A2 增加 `--no-popup` 显式无界面开关；默认路径不允许静默跳过悬浮窗。
- [x] A3 保留 `popup start/status/uninstall` 作为独立维护、恢复和诊断命令。
- [x] A4 将启动结果写入组件诊断，部署健康检查默认要求悬浮窗已运行。
- [x] A5 更新 README、使用说明和架构说明，明确“推送文件不启动、server 默认完整启动”的边界。
- [x] A6 完成单测、签名构建、发布门禁以及 Android 13/16 空环境真机验证。

参考结论：Nexus 的 `server` 默认调用 `startPopup(false)`，只有 `--nopopup` 才跳过；其部署脚本还会在启动 Server 前显式调用 `popup start`。Nova 采用同一默认体验，但使用自身可维护的 Companion、bootstrap 和诊断边界。

完成证据见 [[`validation-server-auto-popup-20260912.md`](../validations/runs/validation-server-auto-popup-20260912.md)](../validations/runs/validation-server-auto-popup-20260912.md)。
