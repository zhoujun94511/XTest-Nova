# 运行组件合并实施 Todo（2026-09-12）

目标：保持电脑端单 Agent 交付和完整功能，在不牺牲控件树能力的前提下减少新设备上的安装确认次数。Companion 与 UiAutomation 继续隔离，避免 Instrumentation 生命周期中断悬浮窗。

- [x] C1 真机验证 UiAutomator host/test 合并可行性：Instrumentation 能启动，但 Android 16 返回 0 个层级节点，判定为不可交付并恢复标准双包结构。
- [x] C2 基于验证结果保留 Runner、Companion、UiAutomator host/test 四个逻辑组件；其中只有三个是 Android 安装包。
- [x] C3 保持 Provider、bootstrap、发布清单、部署与验证脚本的四组件合同，避免以减少包数换取功能退化。
- [x] C4 对同时需要安装或升级的 APK 使用 Android 多包原子安装会话；设备不支持创建会话时安全回退逐包安装。
- [x] C5 相同版本二次启动不创建安装会话；原子事务提交前失败会放弃会话并清理备份，提交后校验失败仍按原版本回滚。
- [x] C6 完成离线单测、签名构建、发布门禁和 Android 13/16 空环境真机验证；实测记录见 [`validation-runtime-atomic-bootstrap-20260912.md`](../validations/runs/validation-runtime-atomic-bootstrap-20260912.md)。

不在本轮范围：将 Companion 与 UiAutomation 合为同一 APK；移除 Companion；改变 minitouch。

## 决策说明

Android Instrumentation 的被测应用与测试应用边界不仅是构建形式。合并原型在 Android 16 上通过了进程启动、鉴权和健康检查，但窗口枚举为 0、层级结果为空，因此不能用作“减少一个 APK”的依据。最终优化点改为：电脑端仍只部署一个 Agent；设备端三个必需 APK 在一次 Package Installer 原子事务中完成安装或升级。
