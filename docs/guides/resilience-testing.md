# 韧性与长稳验证

`tests/e2e/validate-resilience.ps1` 对同一构建执行持续请求、资源趋势、电量和温度观测。默认不改变设备系统状态，最长可配置为 12 小时。

## 默认门禁

每个采样周期请求健康、运行时诊断、能力和设备信息，并检查：

- HTTP 请求失败数为 0；
- Agent PID 稳定，日志中没有 panic；
- Runner、遍历、录制、回放和录屏会话保持空闲；
- 最大协程数不超过初始值 20；
- 堆内存和 Go 运行时系统内存增长分别小于 64 MiB；
- 记录电量变化与最高电池温度，但不把设备自然充放电作为失败条件。

```powershell
.\tests\e2e\validate-resilience.ps1 -Serial @('<serial-1>','<serial-2>') -DurationSeconds 3600 -SampleIntervalSeconds 10
```

报告默认写入已忽略的 `tests/reports/resilience-latest.json`，格式为 `xtest-resilience/v1`。

## 系统状态专项

以下开关会临时改变设备状态，因此默认关闭：

```powershell
.\tests\e2e\validate-resilience.ps1 -Serial '<serial>' -DurationSeconds 30 -ExerciseOverlayPermission -SimulateLowBattery -LowBatteryLevel 5
```

- `ExerciseOverlayPermission`：仅当 Companion 已安装且进程未运行时，将悬浮权限切到相反状态，确认 Agent 不受影响，再恢复原模式；
- `SimulateLowBattery`：通过 Android 电池服务模拟指定电量，确认 Agent 健康，再执行 `reset` 并核对实际电量恢复；
- 成功、失败或中断都会进入清理逻辑，恢复已改变的权限和电池服务；
- 脚本拒绝覆盖设备上已有的 Nova Agent。

专项验证不等价于真实低电量下的数小时业务负载测试，也不证明所有厂商 ROM 对运行中撤销悬浮权限的行为一致。
