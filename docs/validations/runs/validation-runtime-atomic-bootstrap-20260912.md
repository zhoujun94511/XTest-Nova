# 运行组件原子安装验证（2026-09-12）

## 结论

- 发布版本：`xtest-nova-0.25.0-m6.2-atomic-bootstrap`。
- 电脑端仍只需部署与设备 ABI 对应的一个 Agent 文件。
- Runner JAR 只释放不安装；Companion、UiAutomator host/test 三个 APK 在首次安装或整组升级时通过一个 Android 多包事务提交。
- 相同版本二次启动全部显示 `reused` / `installed-reused`，不创建安装会话。
- UiAutomator host/test 单 APK 原型虽能启动，但 Android 16 窗口与节点均为 0，已判定不可交付并恢复标准双包边界。

## 自动验证

- Agent 全包单元测试：通过。
- `go vet ./...`：通过。
- 原子安装、相同版本复用、安装会话建立失败回退用例：通过。
- UiAutomator Host/Test Gradle 8.14.5 离线构建及依赖锁：通过。
- ARM64、ARMv7 自包含 Agent 构建：通过。
- 首次提交门禁：通过，共检查 401 个候选文件。
- 发布门禁：77/77 路由实现、覆盖、语义合同和文档均通过。

## 真机空环境验证

验证前仅卸载三个 Nova 运行 APK，并删除明确列出的 `/data/local/tmp/xtest-nova-*` 运行文件；未安装目标样例应用。

| 设备 | Android API | 首次 bootstrap | 已安装 APK | Gallery 层级 | 二次启动 |
| --- | ---: | --- | ---: | ---: | --- |
| `83fc400c` | 36（Android 16） | 三个 APK 均为 `extracted,installed-atomic` | 3 | 48 节点 | 均为 `reused,installed-reused` |
| `R5CN30EQKNM` | 33（Android 13） | 三个 APK 均为 `extracted,installed-atomic` | 3 | 8 节点，最终复验 9 节点 | 均为 `reused,installed-reused` |

两台设备的组件诊断均为 `degraded=false`。Companion 的 `popup start` 与 `popup status` 均确认悬浮服务真实运行；最终二进制上的 Gallery 3 秒 Runner 冒烟任务均正常启动和收尾，各生成 13 个产物文件。此前 5 秒诊断任务均以 `exitCode=0`、`stopReason=completed` 完成，诊断采集完整。

## 真机产物

- Android 16：`/sdcard/xtest-nova/com.miui.gallery/Monkey/20260912_132016`
  - 2 个动作、2 个状态、1 条图边；Crash/ANR/Native Crash/异常退出均为 0。
- Android 13：`/sdcard/xtest-nova/com.sec.android.gallery3d/Monkey/20260912_132029`
  - 1 个动作、1 个状态；Crash/ANR/Native Crash/异常退出均为 0。

最终二进制复验目录：

- Android 16：`/sdcard/xtest-nova/com.miui.gallery/Monkey/20260912_132543`
- Android 13：`/sdcard/xtest-nova/com.sec.android.gallery3d/Monkey/20260912_132552`

最终发布 Agent SHA-256：

- ARM64：`24366221C0B183B9F4683D562347D4273478D4B4FBBAF2BC2D260213CEFF0EBC`
- ARMv7：`7466F1D9E04B381B5A3BFE2733D7332F7CB1503E6BDFBFF33CE1AC9A352BDF62`

两个目录均包含 `run.json`、`events.jsonl`、`activity_coverage.json/.txt`、`exploration_graph.json`、`start.png`、`finish.png`、`logcat.txt`、`crash.json`、`anr.json`、`native_crash.txt`、`diagnostics.txt` 与 `exit_info.json`，且 `run.json` 记录各文件 SHA-256。

## 权限边界

多包安装将三个 APK 收敛为一个 Package Installer 提交事务，可减少新设备首次安装的确认次数。悬浮窗等 Android 特殊权限仍由系统独立管理，不能与 APK 安装授权合法合并；bootstrap 会尝试以 shell 能力设置权限，厂商系统仍可按自身策略显示单独授权页。
