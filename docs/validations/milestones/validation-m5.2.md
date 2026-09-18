# M5.2 验证记录

验证日期：2026-09-03

版本：

- Agent：`xtest-nova-0.14.0-m5.2`
- Companion：`3.1.0-m5.2`（versionCode 30100）

## 双机只读兼容门禁

`tests/e2e/validate-compatibility.ps1` 不发送输入、不启动或停止第三方应用、不切换输入法，也不执行安装卸载。每台设备执行 18 项检查，结果全部通过。

| 能力                 | Xiaomi 2510DPC44G / SDK 36 | Samsung SM-G9860 / SDK 33      |
|--------------------|----------------------------|--------------------------------|
| 前台应用               | `com.miui.securitycenter`  | `com.sec.android.app.launcher` |
| 应用列表               | 470 个                      | 442 个                          |
| 进程列表               | 1,054 个                    | 844 个                          |
| SystemUI PID       | 5,067                      | 2,480                          |
| SystemUI Total PSS | 340,255 KB                 | 192,180 KB                     |
| CPU 核数             | 8                          | 6                              |
| WLAN IPv4          | `172.16.1.211`             | `172.16.1.72`                  |
| WebView 调试 socket  | 1 个                        | 1 个                            |
| PNG 截图             | 126,557 B                  | 1,260,987 B                    |
| 层级 XML             | 23,504 字符                  | 36,607 字符                      |

其余通过项包括设备型号/SDK/ABI、Settings 包详情、聚合性能、设备内存、网络连接状态、数据分区容量、当前及已启用输入法、UiAutomator 只读状态。

## 发布门禁与产物

- Go 静态检查、全部单元测试和 PowerShell 入口语法检查通过；
- Nexus 合同保持 74/77，三个危险入口继续有意禁用；
- 发布清单格式为 `xtest-nova-release/v1`；
- APK v3 签名及恢复证书指纹验证通过。

| 产物                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `8E62110F5AC5AC0F7FD8CD97D76A38476C1ADFBCCD9A7C81141B187D1A74998E` |
| `xtest-nova-agent-armv7`   | `07048EB4C21A4AD353FF788D24AB0C9F99857039C9F705CFDFDF8D69E8680776` |
| `xtest-nova-runner.jar`    | `F9C7019748DF2F265DC4CDDE4F9CDF4A976BE7987232857872736D8B19C45000` |
| `xtest-nova-companion.apk` | `F0D4161BD2F32612745E6C486174865695BF1BCEE54C2A2E85CCDCF353DAC273` |

## 状态调整

依据现有单元测试、历史 Android 16 记录及本次 Android 13/16 自动门禁，设备信息、前台应用、截图和 WebView/WLAN 调整为 `implemented`。

应用管理、输入法、UiAutomator、录屏、触控及其他会改变设备状态或依赖缺失二进制的组合项继续保持 `partial`，不由只读结果推断为完成。
