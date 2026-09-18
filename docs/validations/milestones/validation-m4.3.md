# M4.3 验证记录

验证日期：2026-09-03

版本：

- Agent：`xtest-nova-0.11.0-m4.3`
- Companion：`2.6.0-m4.3`（versionCode 20600）

## 自动检查

- `go vet ./agent/...`：通过；
- `go test ./agent/...`：通过；
- Nexus 合同检查：74/77，缺失项仍为有意禁用的 `GET/POST /shell/background` 与 `ANY /term`；
- PowerShell 验证器语法检查：通过；
- Companion、Runner、ARM64 Agent、ARMv7 Agent 构建：通过。

最终产物 SHA-256：

| 产物                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `BA0DFED2EDDD39A3ED1AC1212A2376D352DCF1DB53B823837D427D2F967E8358` |
| `xtest-nova-agent-armv7`   | `17DDAD3F558AA6DE5085AF57FED56DB35C901E0C0151BC2C7EA4AE506CB26670` |
| `xtest-nova-runner.jar`    | `FC317E1120C8FD5B0F06F58277973C1B5E958803A45EA8C0ED54ADC9F074EE11` |
| `xtest-nova-companion.apk` | `DBA802C9E4ADF1377ACF407C6FDB7D758C00131304BC72DFFCFAD31116BA12BE` |

Companion APK 使用 v3 签名方案验证通过，签名证书 SHA-256 为 `e49a7c6fb2ee0dbc4e6d3b6790d9dcf981d80f6b76628eb84755e7e1da3a46da`。仓库和构建日志均不包含签名口令。

## 真机矩阵

每台设备持续采样 30 秒，间隔 2 秒，共 15 个样本。测试只读取诊断与健康状态，不执行输入。

| 设备                |    Android SDK | ABI       | 健康失败 | 协程 初始/最大 |      堆增长 | 转发恢复 | 进程恢复 | panic |
|-------------------|---------------:|-----------|-----:|---------:|---------:|------|------|------:|
| Xiaomi 2510DPC44G | 36（Android 16） | arm64-v8a |    0 |      7/7 | 87,272 B | 通过   | 通过   |     0 |
| Samsung SM-G9860  | 33（Android 13） | arm64-v8a |    0 |      7/7 | 85,944 B | 通过   | 通过   |     0 |

两台设备采样期间 PID 均稳定，所有 Runner、遍历、录制、回放和录屏会话均保持空闲；Go 运行时系统内存增长均为 0。进程回收后 PID 与 `startedAt` 均变化，健康检查恢复。

## 结论与剩余边界

M4.3 的 Android 13/16 ARM64 短时稳定性、端口断开恢复和 Agent 进程回收门禁通过。尚未覆盖 Android 14/15、ARMv7 真机、数小时长稳、低电量、权限变化，以及持续业务负载下的资源趋势；这些项目不能由本次结果推断为已通过。
