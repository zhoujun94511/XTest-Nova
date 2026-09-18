# 设备矩阵验证

`tests/e2e/validate-device-matrix.ps1` 用于验证同一 Nova Agent 构建在多台 Android 设备上的空闲稳定性和恢复能力。验证过程不发送触摸、按键或文本输入。

## 覆盖内容

- 自动识别设备 ABI 并部署对应 Agent 和 Runner；
- 持续采样 `/v1/diagnostics/runtime` 的协程、堆和系统内存；
- 确认采样期间 PID 不变化，且 Runner、遍历、录制、回放、录屏会话均为空闲；
- 删除并重建 `adb forward` 后重新检查健康状态；
- 停止并重新启动脚本自己创建的 Agent，确认进程和启动时间都已更新；
- 扫描进程日志中的 panic；
- 无论成功或失败，清理脚本创建的 Agent 和端口转发。

脚本发现设备上已有 `xtest-nova-agent` 时会拒绝覆盖，避免干扰其他测试。

## 使用方式

```powershell
.\tests\e2e\validate-device-matrix.ps1 -Serial @('<serial-1>','<serial-2>') -DurationSeconds 60 -SampleIntervalSeconds 2
```

报告默认写入 `tests/reports/device-matrix-latest.json`，该目录已忽略，不进入版本管理。报告格式为 `xtest-device-matrix/v1`。

## 当前门禁

- 健康请求失败数必须为 0；
- PID 在持续采样期间必须稳定；
- 所有输入相关会话必须保持空闲；
- 端口转发和进程重启必须恢复；
- panic 数必须为 0；
- 最大协程数不得超过初始值 20；
- 堆内存和 Go 运行时系统内存增长分别不得达到 64 MiB。

此门禁用于发现明显回归，不替代数小时长稳、低电量、权限变化和具体业务场景测试。
