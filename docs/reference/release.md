# 发布与回滚

## 发布产物

构建工具链固定在 `tools/build-toolchain.psd1`。本地和 CI 必须使用其中记录的 Go、JDK、Android Platform 与 Build Tools；JAR/APK 内部归档时间固定，Go 构建关闭环境相关的 VCS 注入。发布清单同时记录工具链版本。

默认构建必须签名。开发时只能显式执行 `.\build.ps1 -Development`，该模式不
生成 Companion、UiAutomator 或 fixture，不覆盖正式 Agent 文件；它生成带
`development` 文件名的 Agent，并校验、复用现有 embedded runtime bundle。

完成签名构建后生成机器可读清单：

```powershell
.\new-release-manifest.ps1
```

`dist/release-manifest.json` 使用 `xtest-nova-release/v2` 格式。顶层正式交付物精确为两个架构 Agent：ARM64 与 ARMv7；实际部署时每台设备只使用其中一个。清单同时记录 Agent 内嵌 Runner、Companion、UiAutomator host/test 的大小、SHA-256、包名、版本和证书指纹。三个 APK 必须使用同一维护者证书签名；混签、缺项、重复项、源码版本不一致、大小或哈希不一致都会被门禁拒绝。

`generatedAt` 优先取 `SOURCE_DATE_EPOCH`，否则取当前 Git 提交时间；仓库尚无
提交时才取当前 UTC 时间。发布 JSON、SBOM 副本和第三方声明统一使用无 BOM
UTF-8 与 LF 换行，因此同一提交和同一产物可重复生成相同文本。尺寸基线和
runtime manifest 只能随对应制品实际重建后更新，单独生成清单不会改写它们。

`apksigner` 支持 `--ks-pass env:<name>` 与 `--key-pass env:<name>`。构建脚本
为每次签名创建随机环境变量并在 `finally` 中删除。CI 的 JDK `keytool` 同样
使用 `-storepass:env` 和 `-keypass:env`，口令不会出现在进程命令行或日志中。

`com.xtest.nova.fixture`、Foloy、Spoly 等目标或样例应用不属于运行时，不嵌入 Agent、不自动安装，也不进入正式发布清单。`dist` 不进入 Git，清单应与两个架构二进制一起归档。

发布前执行：

```powershell
.\scripts\test-release.ps1
```

门禁默认使用仓内 `docs/compliance/reference-http-contract.md`，重新计算所有产物哈希和大小，运行 Go 静态检查、单元测试、77 项合同检查，并解析全部 PowerShell 入口。只有对已审查的其他合同快照做显式验证时才传 `-ReferenceContractPath`；空值或不存在的文件会失败，不能跳过合同门禁。正式提交后可增加 `-RequireCleanGit`，要求工作区没有未提交修改。

## 安全部署

默认部署让 7912、8912 和 7890 全部保持设备回环监听，并建立 7912 的 ADB 转发：

```powershell
.\deploy.ps1 -Serial '<serial>' -StartServer
```

部署流程：

1. 只接受 ARM64 或 ARMv7，未知 ABI 直接拒绝；
2. 上传为暂存文件，并在设备端重新计算 SHA-256；
3. 将现有 Agent 复制为 `.previous`，切换暂存文件并启动；
4. Agent 在监听端口前校验并原子释放四个内嵌组件；需要同时安装或升级多个 APK 时，将 Companion、UiAutomator host/test 作为一个 Android 多包事务提交；
5. 不支持多包会话的旧设备安全回退逐包安装；安装前后均校验摘要，失败时放弃未提交会话或按逆序恢复旧版本；
6. 默认拉起 Companion 悬浮窗，并使用自动分配的本地端口检查健康状态、精确版本、完整组件状态及 `popupOverlayStartup`；
7. 部署失败时恢复 `.previous`，若原进程曾运行则重新启动旧版本。

设备上已有 Agent 运行时默认拒绝替换。确认后使用 `-ReplaceRunningAgent`。不存在 Companion 或 UiAutomator 的“按需安装”开关：它们属于完整功能的必需运行时，由 Agent 自动保持为内嵌版本。

受信任局域网部署必须同时传入 `-AllowLAN -ApiTokenFile <仓库外文件>`。令牌文件去除
首尾空白后至少 32 字节，部署脚本会校验文件、经哈希验证后以 `0600` 权限原子切换到
设备，且不会在启动参数或输出中显示令牌。该模式仅让主 Agent 7912 监听非回环地址并
启用 Bearer/Cookie 认证；Companion 8912 和 Monitor 7890 始终回环。
如同时传入 `-LegacyUnsafeAPI`，同步 Shell、后台 Shell 和 Web 终端也由同一 LAN
认证层保护；该组合仍只能用于受信任网络。

令牌轮换使用新文件重新执行 LAN 部署并传入 `-ReplaceRunningAgent`，重启会撤销旧令牌
和旧 Cookie 会话。切换失败时，部署事务同时恢复旧 Agent 和旧设备端令牌。恢复默认
模式时不带 LAN 参数重新部署；保留的设备端令牌文件本身不会开启认证或网络监听。
完整命令见[使用说明](usage.md)。

正常部署的 `-StartServer` 等价于设备端完整启动，悬浮窗会自动显示，不需要再执行 `popup start`。只有无界面门禁或维护任务才增加 `-NoPopup`；该开关不会减少已安装的必需组件。`popup start/status/uninstall` 继续作为独立恢复和诊断命令。

同一版本再次启动只复用已释放文件和已安装 APK，不创建 Package Installer 会话。多包事务可以把三个 APK 的安装/升级收敛为一次提交，但设备厂商仍可能对悬浮窗等特殊权限单独显示系统授权页；这类权限不能由普通 APK 安装事务代替。

`-HierarchyProvider system|shadow|nova` 只决定控件树采集策略，不决定组件是否安装。资格阶段使用 `shadow` 时，新旧层级按“system 决策采集 → Nova 临时对照采集 → 回收 Nova”的顺序隔离执行；满足 [`uiautomator-provider-plan.md`](../plans/uiautomator-provider-plan.md) 的门槛后可使用 `nova`。

旧端口 9008 UiAutomator 与 `/jsonrpc/0` 不属于默认完整功能链路。只有迁移旧客户端时才传入 `-LegacyUiAutomator`；未启用时 Agent 不探测 9008、不启动旧 Instrumentation，并对 `/jsonrpc/0` 返回 404。

## 显式回滚

```powershell
.\deploy.ps1 -Serial '<serial>' -Rollback -StartServer
```

回滚会保留当前 Agent 为 `.failed`，恢复 `.previous` 并重新执行健康检查。回滚副本是设备本地运维能力，不替代版本化发布包和外部制品归档。
