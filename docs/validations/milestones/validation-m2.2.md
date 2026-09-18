# M2.2 验证记录

验证日期：2026-09-03。

## 实现范围

- 进程列表和包名 PID；
- 应用单进程及多进程内存；
- 进程 CPU、系统 CPU 和网络统计；
- 聚合性能响应；
- 设备内存、数据分区容量和网络连接状态；
- 当前及已启用输入法、受校验的输入法切换；
- UiAutomator 包探测和服务管理；
- Companion APK 版本检查、暂存路径检查和安装入口；
- 本地暂存 APK 安装入口；
- 7912 与 8912 的 M2.2 兼容路由。

## 自动化结果

- `go test ./agent/...`：通过；
- `go vet ./agent/...`：通过；
- ARM64/ARMv7 Agent：构建通过；
- Runner Dex/JAR：构建通过；
- Companion APK：构建、对齐及维护证书签名通过；
- Nexus 合同覆盖：38/77 个“方法+路径”，剩余 39 个明确报告。

## Android 16 真机结果

测试设备：`83fc400c`，SDK 36，`arm64-v8a`。Nova 使用 17912/18912 独立测试端口。

| 检查项                        | 结果                      |
|----------------------------|-------------------------|
| Agent 版本                   | `xtest-nova-0.2.1-m2.2` |
| 进程列表                       | 1,021 项                 |
| `com.android.systemui` PID | 5,067                   |
| SystemUI 内存                | Total PSS 525,438 KB    |
| CPU 核数                     | 8                       |
| 网络统计                       | 205,675,677 字节          |
| 设备内存                       | 11,388,472 KB           |
| 数据分区容量                     | 247,902,220,288 字节      |
| 网络状态                       | 已连接                     |
| 当前输入法                      | Google LatinIME         |
| UiAutomator 服务             | 未安装，状态正确显示为停止           |
| 后台 Shell                   | HTTP 403，按安全设计禁用        |
| 未暂存 Companion 的安装请求        | HTTP 503，没有覆盖旧 APK      |
| 8912 网络信息                  | 通过                      |

验证后已经停止 Nova 测试进程并移除临时端口转发。Nova Companion 仍未安装到设备，现有 Nexus Popup 未被替换。

## 产物

| 产物                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `6B18651D69804F38E147EA99BE6F569C46A2DCD6E8058F33AAEEE189F697A99F` |
| `xtest-nova-agent-armv7`   | `3F722CF957589AABAEBAFACD5DEAC255BF56B66DD195A0495DA0EDBF1A7074FB` |
| `xtest-nova-runner.jar`    | `E0EF6B466DF7EDDD18A08E64167060CF574773FCAC0BB360371C550EA5C6A1DF` |
| `xtest-nova-companion.apk` | `0FE8324E11FB4525EEA1334287F4FE4AC6EE9B22C8326F68FD782B93424CA472` |

Companion 版本为 `2.0.1-m2.2`，`versionCode=20002`；证书 SHA-256 与 Nexus 维护证书一致：

```text
e49a7c6fb2ee0dbc4e6d3b6790d9dcf981d80f6b76628eb84755e7e1da3a46da
```
