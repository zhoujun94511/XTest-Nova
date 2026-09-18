# 测试机清理与最新构建验证（2026-09-12）

> 复核说明：第一次执行隐藏了 `adb uninstall` 输出，且没有在重新部署前逐包断言不存在，证据链不充分。以下内容以第二次完整重做的卸载输出、空环境断言和新安装时间为准。

## 清理范围

- 停止两台设备上的 Nova Agent。
- 卸载 `com.openatx.xtest.popup`、`com.openatx.xtest.nova.uiautomator`、`com.openatx.xtest.nova.uiautomator.test`。
- 删除明确列出的 `/data/local/tmp/xtest-nova-*` 运行文件。
- 第二次复核重做时删除 `/sdcard/xtest-nova` 结果：Android 16 约 2.5 MB，Android 13 约 390 KB。
- 未删除相册或其他用户数据，未卸载 Gallery 目标应用。

两台设备上的六次卸载命令均明确返回 `Success`。重新部署前逐台确认：Agent 进程为空，三个包的 `pm path` 均为空，Agent 文件与 `/sdcard/xtest-nova` 均不存在。

## 构建与门禁

- 复核中发现 Agent 停止后立即重启时，PackageManager 可能短暂查不到已安装包，旧判断会误触发整组重装；bootstrap 已增加有界重试和对应回归测试。
- 从修复后的当前源码重新完成正式签名构建并生成 `xtest-nova-0.25.1-m6.2-bootstrap-pm-retry` 发布清单。
- 全量 Go 测试、UiAutomator Host/Test 构建、402 项基础门禁及 77/77 发布合同门禁通过。
- ARM64 SHA-256：`EC78A8C13341F5558846525AEAD0E17D5154AB49811B9786C3C4EFC17F797043`。
- ARMv7 SHA-256：`E31A2CE001C91A9FDB568401995D0D1B9CE0C3BF42CFF16A50F5B938ADAF8F1C`。

## 空环境真机结果

| 设备 | 系统 | 首次安装 | 层级节点 | 截图 | Companion | Runner | 异常与诊断 |
| --- | --- | --- | ---: | ---: | --- | --- | --- |
| `83fc400c` | Android 16 / API 36 | 三个 APK 均 `installed-atomic`；安装时间 14:02:36 | 45 | 1,477,922 B | 启动并确认运行 | 完成 | 功能冒烟通过 |
| `R5CN30EQKNM` | Android 13 / API 33 | 三个 APK 均 `installed-atomic`；安装时间 14:02:37 | 29 | 341,339 B | 启动并确认运行 | 完成 | 功能冒烟通过 |

功能测试结束后不加人工等待立即停止并重启 Agent：两台设备均显示 Runner `reused`、三个 APK `installed-reused`，三个包的 `lastUpdateTime` 完全不变，组件诊断 `degraded=false`，未再次安装。

## 当前保留的新证据

- Android 16：`/sdcard/xtest-nova/com.miui.gallery/Monkey/20260912_140312`，13 个文件。
- Android 13：`/sdcard/xtest-nova/com.sec.android.gallery3d/Monkey/20260912_140326`，13 个文件。

每台设备目前只保留本轮最新的一个会话；历史构造产物未重新出现。
