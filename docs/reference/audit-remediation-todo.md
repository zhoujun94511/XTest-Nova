# XTest Nova 全链路审计整改 Todo

审计基线：XTest Nexus 10248、Nexus 77 项历史 HTTP 合同、Nova 当前源码与既有 Android 12/13/16 验证记录。

状态定义：`[x]` 已由源码和测试关闭；`[~]` 正在实施；`[ ]` 尚未关闭；`[!]` 需要外部设备或发布周期。

## R1 数据完整性与发布安全

- [x] R1.1 为 multipart 请求增加真正的总大小限制，删除上传双重截断并补充超限回归测试。
- [x] R1.2 修复首次提交门禁的内容扫描、无远端处理和可选远端校验。
- [x] R1.3 `/installAgentApk` 只在精确版本且 APK 字节一致时跳过；其他情况由 Package Manager 重装并执行签名校验。
- [x] R1.4 已使用恢复签名资料重新生成制品和发布清单，首次提交检查、测试、77 项合同和签名发布门禁全部通过；后续源码变更仍须在交付前重跑。

## R2 运行时就绪、停止与回滚

- [x] R2.1 Agent 同步绑定 7912/8912/7890，任一必需监听失败即退出；Android 13 已完成 7912/8912 冲突注入，并确认无残留进程或 PID 文件。
- [x] R2.2 健康接口暴露必需依赖状态，能力接口保留 supported 顶层字段并增加 readiness。
- [x] R2.3 `server -d` 等待 PID、精确版本响应头和健康检查；Android 13 已完成 daemon 启动、健康和停止复验。
- [x] R2.4 部署失败会恢复旧运行时并恢复或卸载 Companion；Android 13 已完成运行时切换后失败、Companion 安装后失败两种注入，旧 Agent 均恢复健康且临时 Companion 被移除。
- [x] R2.5 `/stop` 与执行入口共享生命周期互斥，强杀 Runner 后等待状态收敛；完整 Go 竞态门禁和 Android 13 动态停止均已通过。
- [x] R2.6 关闭流程为各组件提供独立期限，不复用已经耗尽的总 Context。

## R3 录制回放与长期资源边界

- [x] R3.1 回放前校验录制方向和宽高比；不兼容时明确拒绝。
- [x] R3.2 截图断言失败保存实际 PNG、期望/实际哈希、距离和动作索引。
- [x] R3.3 用例只扫描 Replay 目录，按目标包最多保留 200 个用例会话。
- [x] R3.4 Perf 最多保留 200 个会话，Agent daemon 日志达到 32 MiB 时保留一个轮转备份。

## R4 XTest/Nexus 行为对齐

- [~] R4.1 Runner 已优先使用目标包节点探索，层级失败或场景耗尽才降级为有界随机输入；最新 Android 13 双 Activity 样例得到 4 次点击、3 次长按、1 次滚动、4 次回退且随机降级为 0，仍待更多真实应用资格测试。
- [x] R4.2 已按原 DEX/JADX/Smali 重新核对 XTest 引擎并完成当前可验证行为整改，证据见 [[`xtest-monkey-bytecode-audit.md`](../audits/xtest-monkey-bytecode-audit.md)](../audits/xtest-monkey-bytecode-audit.md)。
  - [x] R4.2a 使用目标 APK 二进制 Manifest 解析 Activity 分母；无法读取时才明确降级为 Package Resolver + 运行时观测下界。
  - [x] R4.2b 节点稳定分类已向原默认字段 `activity,resource-id,class,clickable,enabled,checkable` 收敛；场景使用 Activity、树深度/索引和稳定字段生成 SHA-256 结构指纹。
  - [x] R4.2c 已具备 CLICK、LONG_CLICK、滚动、BACK、状态转移和已知图路径搜索；Android 15 专用非栈式夹具已产生 `knownPathReplays=1` 的真机证据。
  - [x] R4.2d 原字节码未发现 Compose 专用分支或优先级；Nova 继续以 Accessibility 通用树兼容。Foloy 已在 Android 13/15/16 完成真实 Compose 首启、父容器点击、懒加载列表、Onboarding 手势和多页面探索，未加入应用/设备特判；该证据只关闭真实 Compose 通用兼容性，不宣称“XTest Compose 优先对齐”。
  - [x] R4.2e Runner 已从全局节点去重升级为场景动作图：稳定结构指纹、场景内动作状态、转移边和已知路径回放均有 JVM 自测，Agent 解析和产物链有 Go 回归测试。
  - [x] R4.2f Android 13 fixture 已验证自环、跨页转移、耗尽回退和零层级降级；Android 15 非栈式夹具进一步验证 8 状态、14 边、16 事件及已知路径回放，指标均写入 `exploration_graph.json`/`run.json`。
- [~] R4.3 建立 Nexus/Nova 同应用、同配置、同种子、同时间的行为黄金报告。
  - [x] R4.3a 已固化可复现黄金视图；时间、时长和证据位置保留在原始报告中，但不进入字节级黄金结果。
  - [~] R4.3b 已提供场景校验、报告归一化、Nexus 日志导入、Nova `run.json` 导入、指标比较和多次确定性检查入口；Android 15 的早期 `SecurityException` 已修复并完成一次同机串行双运行，结构化结果为 `pass`，但状态与 Activity 指标仍有不可比较项，仍需补齐可重复且指标同口径的业务基线。
  - [x] R4.3c Android 15 已验证两套运行时串行切换和精确 PID 停止，未并行注入；参考侧故障与 Nova 恢复健康状态均保留证据。
- [~] R4.4 建立 77 项实际请求、状态码、字段、错误和副作用的双实现黄金测试。
  - [x] R4.4a 已实现 19 个无路径占位、只读、非流式请求的双端执行器，比较状态码、正文类型和 JSON 字段形状；Android 13 同端自校验为 19/19、0 差异。
  - [x] R4.4b 已建立副作用隔离夹具：文件写读、配置恢复、应用启动、AutoPopup、性能、录屏、minitouch 能力分支及唤醒均有前后状态和精确清理；安装/卸载由既有 `tests/e2e/validate-stateful.ps1` 独立覆盖，任意命令接口继续默认禁用。
  - [!] R4.4c 最终双实现运行需要可安全部署的 Nexus 参考端；危险接口不得为了 77/77 数字而默认启用。

## R5 完整产品流程与工程化

- [x] R5.1 将 Web 控制台状态从“完整实现”纠正为当前最小控制台。
- [x] R5.2 Web 工作台已补齐应用、性能、Monkey、受控文件、录制回放、运行诊断、守护日志、scrcpy 远控播放器和长稳采样入口；设备资格限制继续按 R6 管理。
- [x] R5.3 Companion 使用 Android `org.json` 正式解析，停止操作使用独立 15 秒期限。
- [x] R5.4 锁定 Go、JDK、Android Platform/Build Tools；五个 unsigned 产物连续构建哈希一致。
- [~] R5.5 已增加 CI 定义：静态检查、单测、竞态、敏感信息和完整构建；待首次远端运行。
- [x] R5.6 README、兼容矩阵、旧 Todo 和发布审查均已增加资格口径；本轮同步修正 `/stop` 与 Web 工作台描述。

## R6 设备资格

- [~] R6.1 Android 15/API 35 通用兼容矩阵 18/18；Android 14 设备仍未连接。
- [!] R6.2 ARMv7 真机。
- [~] R6.3 Android 15 SurfaceView 独立循环已确认 120 Hz，最新活动图层回退采到 78.27–88.32 应用 FPS；OpenGL ES 真负载下 Adreno/KGSL 峰值 87%。西瓜制作机 2048 已在 Android 15/16 完成新版各 20 分钟商业游戏复测；Android 14 与 Mali/其他厂商 GPU 仍待资格设备。
- [!] R6.4 数小时业务负载与 24 小时稳定性。
- [!] R6.5 平板回归：当前重新连接平板后复跑竖屏、横屏、滚动、Monkey 与回放。
  - [~] R6.6 Android 15 / API 35（SM-S936U）资格：基础部署、健康、CLI、Monkey、录制回放、性能、截图、录屏、WebSocket、停止和清理。
  - [x] R6.6a 专用非栈式 fixture 已产生 `graph_path` 真机事件，`knownPathReplays=1`。
  - [x] R6.6b 已在同一设备按相同目标、seed、节奏和 40 秒预算串行切换 Nexus/Nova；修复 Nexus 自身 UiAutomator 与恢复 Monkey 的独占租约后，两侧均完成且无崩溃，结构化报告为 `pass`。状态与 Activity 指标口径差异保持不可比较标记。
  - [x] R6.6c 副作用 HTTP 已按文件、应用、任务/媒体和服务生命周期建立隔离夹具；原配置恢复、3 个精确产物删除及零活动会话均已验证。
  - [x] R6.6d SurfaceView 与 OpenGL ES 独立游戏式循环、120 Hz、Adreno/KGSL 真负载已通过；真实 Compose 已由 Foloy 三机闭环，西瓜制作机 2048 已补齐 Android 15/16 商业游戏各 20 分钟证据。
  - [x] R6.6e Android 15 SurfaceView/游戏类应用在 `gfxinfo` 无增量时回退到目标包 SurfaceFlinger 图层计数；暖机阶段返回空值而非伪造 0。
  - [x] R6.6f SurfaceFlinger 历史图层按最新单调图层 ID 排除，资格门禁要求至少 3 个不低于 20 FPS 的样本；已用 Activity 重启回归证明不会再被销毁图层钉成 0。

M5.9 首次提交的进入条件：R1–R5 不存在未关闭高优先级项，所有文档状态与实际证据一致，R6 未完成项均有明确资格限制或维护者批准。
