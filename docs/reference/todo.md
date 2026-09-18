# XTest Nova 功能对齐 Todo

> 2026-09-04 全链路复审纠偏：本文件的历史阶段勾选只表示当时定义的阶段已执行，不再代表与 Nexus/XTest 整体产品等价或已具备发布资格。Monkey 行为、运行时黄金合同和完整 Web 控制台仍未闭合；M5.9 由 [全链路审计整改 Todo](audit-remediation-todo.md) 阻断。

状态：`[x]` 已完成并有证据；`[ ]` 可继续执行；`[!]` 需要当前环境之外的设备、二进制或发布周期。

## 已完成阶段

- [x] M1：Agent、Runner、Companion 可维护架构及构建部署链路；
- [x] M2：Nexus 历史合同基础实现 74/77，3 个任意命令入口默认禁用；
- [x] M3：智能遍历、状态图、可逆 DFS 和 Nexus/Nova A/B 评估；
- [x] M4.1–M4.2：录制回放、UTF-8 文本、双击、多指、截图断言和断点续播；
- [x] M4.3–M4.4：运行诊断、设备矩阵、断线/进程恢复、短稳、权限和低电量专项；
- [x] M5.1：发布清单、发布门禁、版本健康检查及自动/显式回滚；
- [x] M5.2：Android 13/16 双机只读兼容门禁；
- [x] M5.3：独立夹具的安装、查询、会话启动、卸载、前台与输入法恢复。
- [x] M5.4：multipart APK 安装、公开 HTTPS 下载任务、录屏生命周期及产物清理。
- [x] M5.5：隔离夹具上的 Monkey/Runner 生命周期、停止门禁和运行日志。
- [x] M5.6：Popup Assistant 安全规则样本、录制回放端到端及流式能力回归。
- [x] M5.7：逐项处理兼容矩阵原 `partial`，建立资格状态、书面豁免与发布门禁。
- [x] M5.8：首次 Git 提交前文件审查、许可证/NOTICE、版本说明和提交候选门禁。
- [x] M5.8R：需求—实现追踪、核心状态机白盒审查、Go 官方竞态检测、Android 13/16 动态回归、短稳与并发资源门禁；审查结论和限制见 [`pre-release-review.md`](pre-release-review.md)。
- [x] M5.8C：按 ATX 协议补齐后台 Shell GET/POST 与 PTY 终端，在显式兼容开关下达到 Nexus 合同 77/77；默认拒绝及 Android 13/16 真机链路通过。
- [x] M5.8U：统一 `xtest-nexus` 设备入口和原 XTest 命令结构；对齐 `/shell`、`/stop`、`/info` 关键报文，更新部署/验证/发布链并完成 Android 13/16 真机闭环。

## 当前可执行

- [x] 性能采集语义、可靠性、产物和 Web 工作台已按[性能采集审计整改计划与 Todo](../todos/performance-audit-remediation-todo-20260912.md)完成代码整改、Android 13/16 真机闭环和发布门禁；SurfaceView/商业游戏卡顿分类与历史会话对比保留为文档中的 P2 增强项。

- [~] M5.8H：按[自有 UiAutomator Provider 实施计划](../plans/uiautomator-provider-plan.md)分期收口控件树采集底座。
  - [~] U1：观测指标已接入 `/v1/diagnostics/runtime`；Android 12/15 完成原有热态与影子采样，Android 13/15/16 已完成 Foloy 多页面及完整首启特殊场景基线，更多获准真实应用及完整设备基线待补。
  - [x] U2：新增独立 `uiautomator/host` 与 `uiautomator/test` 工程，固定 Gradle/AGP/AndroidX 版本、依赖锁和哈希校验，构建两个 APK。
  - [x] U3：持续 Instrumentation 服务已实现健康、层级、多窗口、稳定等待和诊断端点；真机资格归 U9。
  - [x] U4：回环监听、随机令牌、大小限制、串行请求、超时、空闲回收、Agent 所有权和退出/异常降级已实现并通过单测；真机资格归 U9。
  - [x] U5：Provider 接口、Snapshot、三段回退和 `system/shadow/nova` 显式配置已接入；默认保持系统 dump。
  - [x] U6：现有层级、截图和 JSON-RPC 外部合同保持不变。
  - [~] U7：串行隔离影子、结构/属性指纹、稳定身份匹配和按包差异已实现；Plant Scope 固定页及 Android 13/15/16 的 Foloy 多页面、Compose 首启和系统场景没有明确业务语义节点退化，更多获准应用样本待扩展。
  - [ ] U8：达到量化门槛后切换 `nova → system-dump → legacy-9008`。
  - [~] U9：Android 12/15/16 已通过原有固定页、热态、影子和崩溃恢复；Pixel/Android 16 同一 WebView 各完成 10,000/10,000 次、零失败。Android 15 Foloy 真实分屏影子和 50 次直连通过，P95 318.99 ms、161 nodes稳定；Android 13/16 真实分屏和两小时长稳待补，见 [`validation-uiautomator-provider-20260907.md`](../validations/runs/validation-uiautomator-provider-20260907.md)。
  - [~] U10：架构、兼容、构建与维护文档已开始更新；W-005/W-009 待真机证据关闭。

- [x] M5.8G：77 项均具备可执行语义合同，覆盖请求编码与字段、成功/失败状态、响应类型与字段、副作用及运行资格；高风险接口使用隔离夹具和可回滚真机门禁。
- [x] M5.8F：Monkey、Popup Assistant、录制回放、流式能力和 Web/PTY 终端已通过；M5.8F2 与 Android 16 独立安装复验均已关闭。
  - [x] M5.8F1：修复 `popup start` 空白/配置页问题；Android 13 验证无界面直启、真实悬浮窗口、状态反馈、前台目标传递及 Runner 启停闭环；Android 16 原 10248 行为完成对照。
  - [x] M5.8F2：XTest Nexus 30710 已完成菜单与可读尺寸、目标应用性能六类采集、录制任务/文本链，以及 Monkey 低电量、Activity 名单、控件黑名单和目标页签名用例调度。
    - [x] M5.8F2a：五项菜单、关键视觉结构、图标应用列表、最小化恢复、退出保留 Agent。
    - [x] M5.8F2b：修复回放完成伪运行态；增加录制控件排除区、动作计数、返回键和截图断言，非空持久化回放 2/2 通过。
    - [x] M5.8F2c：完成目标应用名标题、单位换算、滚动、持续刷新、会话停止，以及 `/sdcard/xtest-nexus/<目标包名>/Perf/yyyyMMdd_HHmmss/perf.csv`（设备时区）落盘；Android 13 真机完成 `gfxinfo` 帧差 FPS、KGSL GPU 与电池电流/电量/温度采集。首个 FPS 样本和不支持来源的设备明确返回 `null`；游戏/SurfaceView 与受限厂商节点继续按 W-003 限定，不伪造数据。
    - [x] M5.8F2d：录制任务模型、任务/用例目录层级、非密码聚焦输入框最终 UTF-8 文本和焦点坐标自动采集已接入；手工文本接口继续保留，密码字段明确拒绝。
    - [x] M5.8F2e：低电量退出、Activity 黑/白名单、目标页命中观测、控件文本/资源 ID 黑名单及“目标页→最新匹配的已签名任务/用例”已接入 Runner；同步令牌回调在随机事件暂停期间执行 Replay，完成后恢复 Monkey，Android 13 真机验证用例完成后继续产生 6 个随机事件。
  - [x] M5.8F3：以不同包名的同字节码隔离验收包与用户 10248 并存，在 Android 16 完成无空白页、五项菜单、性能、2/2 录制回放、Monkey、最小化、退出及撤权恢复；原 10248 未被覆盖。
- [x] M5.8F4：Monkey 证据归档与平板适配收口；设计、资源边界与 SM-X920 真机证据见 [`validation-monkey-artifacts-30714-tablet.md`](../validations/popup/validation-monkey-artifacts-30714-tablet.md)。
  - [x] M5.8F4a：按 `/sdcard/xtest-nexus/<目标包>/Monkey/yyyyMMdd_HHmmss/` 建立会话，保存 `events.jsonl`、`run.json`、Activity 覆盖率和开始/结束/失败关键截图。
  - [x] M5.8F4b：状态接口补齐目标包、事件数、停止原因、退出码及产物路径；固定兼容日志达到 32 MiB 时轮转一个备份。
  - [x] M5.8F4c：录屏归入目标包 `ScreenRecord` 目录；Monkey 和录屏各目标包/类型最多保留 200 个会话。
  - [x] M5.8F4d：为 `smallestScreenWidthDp >= 600` 增加统一平板尺寸档位，完成竖屏、横屏、滚动和旋转恢复验证。
  - [x] M5.8F4e：补齐成功/失败产物、敏感字段、轮转与保留策略单元测试，三轮竞态、发布门禁和平板端到端证据，确认清理与其他设备隔离。
- [ ] M5.9：首次提交与远程上传；除用户明确授权外，还必须满足 [`audit-remediation-todo.md`](audit-remediation-todo.md) 的进入条件。

## 外部依赖

- [~] Android 15/API 35 真机已完成基础、图探索、Surface FPS 与 Adreno/KGSL 真负载资格；Android 14 真机仍未连接；
- [!] 小米 Android 16 无人值守重复 APK 安装：短时间重复安装会触发 `INSTALL_FAILED_USER_RESTRICTED`，需要设备侧确认或等待厂商限制窗口恢复（W-012）；
- [!] ARMv7 真机覆盖：当前两台设备均为 ARM64；
- [!] TouchReader Protocol-A：需要提供 Protocol-A 输入设备样本；
- [!] minitouch 完整真机触控：设备缺少对应 ABI 的可信 minitouch 二进制；
- [!] 数小时真实业务负载长稳：当前按要求先执行短周期趋势门禁，正式发布前仍需后台运行窗口；
- [!] 旧引擎降级发布周期：需要真实发布流量和观察周期。

## 最终完成条件

- 所有可执行项完成；
- 所有外部依赖项取得证据或由维护者批准豁免；
- 兼容矩阵无未解释的 `partial`，资格限制均关联书面豁免；
- 发布清单、签名、回滚、敏感信息和许可证门禁全部通过；
- 完成用户授权的首次提交与远程上传。
