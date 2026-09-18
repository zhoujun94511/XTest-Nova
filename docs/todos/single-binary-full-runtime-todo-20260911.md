# 单文件完整运行时改造计划与 Todo（2026-09-11）

## 目标

每种 CPU 架构只交付一个 Agent 二进制。Agent 服务启动时必须自动释放 Runner，并校验、安装或升级 Companion、UiAutomator Host、UiAutomator Test，全部完成后才启动对外服务。目标样例应用 `com.xtest.nova.fixture` 明确不属于运行时，不嵌入、不自动安装、不进入正式发布清单。

## Todo

- [x] P0 建立独立 `runtimebundle` 模块，嵌入四个正式运行组件及可审计清单，启动时重新计算 SHA-256。
- [x] P0 建立独立 `runtimebootstrap` 模块，负责原子释放、安装一致性判断、事务回滚和启动状态。
- [x] P0 Agent 服务启动默认执行完整 bootstrap；任一必需组件失败时禁止进入 ready/监听状态。
- [x] P0 部署脚本改为每台设备只推送一个架构对应的 Agent，不再要求用户传安装开关或推送 JAR/APK。
- [x] P0 发布清单改为两个架构 Agent；Runner/Companion/UiAutomator 作为内嵌组件记录摘要、版本和签名证书。
- [x] P1 构建链在正式签名构建后同步内嵌资源，校验三个 APK 同证书；开发构建不得用未签名 APK覆盖正式内嵌资源。
- [x] P1 健康与组件诊断暴露完整运行时 bootstrap 状态和各组件结果。
- [x] P1 更新验证脚本中仍单独推送 Runner/Companion 的链路，统一验证单文件自举。
- [x] P2 更新 README、发布说明和架构说明，明确样例应用只用于测试，不属于交付物。
- [x] 验证单元测试、竞态检测、Android arm64/armv7 构建、脚本解析和完整发布门禁。

## 边界

- “单文件”指电脑到设备的交付文件只有一个架构 Agent；Android Instrumentation 仍要求 Host/Test 以独立包存在，但安装由 Agent 内部完成。
- 不嵌入 validation fixture、Foloy、Spoly 或其他目标应用。
- 不接受“缺组件但核心服务继续运行”的静默降级；完整运行时 bootstrap 失败即启动失败，并在守护日志保留具体原因。

## 对齐结果

- 发布版本已统一使用 `xtest-nova-*`；`xtest-nova-release/v2` 顶层仅登记 `xtest-nova-agent-arm64`、`xtest-nova-agent-armv7`。
- 内嵌清单精确包含 `runner`、`companion`、`uiautomatorHost`、`uiautomatorTest`，构建与发布门禁都会拒绝未声明文件及 validation fixture。
- 真机 `R5CN30EQKNM`（ARM64）仅推送一个 Agent 后启动成功；组件诊断中 bootstrap 与四个载荷全部为 `ready`，Nova Provider 层级采集可用。
- 样例应用默认不构建；仅夹具专项显式使用 `-BuildValidationFixture`。已清除 `dist` 中遗留的 validation APK，设备自举不会安装它。
- Go 单元测试、`go vet`、全包竞态检测、两个 Android 架构构建、PowerShell 解析、首次提交门禁和完整发布门禁均通过。
