# 可信执行与性能基线整改 Todo（2026-09-12）

目标：把动作、证据、判定和性能结果绑定到同一次执行，阻止旧页面动作、重复触控、旧会话停止新任务以及缺少证据时误报通过。多设备任务租约不在本轮范围内。

## Todo List

- [x] T1 建立 `runId / attemptId / sessionId / ownerToken / generation` 执行身份；Runner、智能遍历、录制和回放的外部停止必须匹配会话与所有者，智能遍历、录制和回放共用互斥协调器。
- [x] T2 为普通探索、特殊场景恢复和确定性回放生成 `observationId + stepId`；执行前复核页面，旧观察动作只写拒绝回执，不触发输入。
- [x] T3 建立幂等动作回执；同一 `stepId` 重试返回原回执，不产生第二次触屏。
- [x] T4 建立 SHA-256 `evidence.json`；Runner、探索、性能、录制和回放的证据均关联同一 attempt，所有者令牌禁止落盘。
- [x] T5 增加独立三态 Result Judge：只有完成且检查点证据齐全才为 `passed`，明确失败为 `failed`，未执行完成或没有验收证据为 `not_tested`。
- [x] T6 将 Runner 与智能遍历报告接入 Result Judge，回放按截图检查点和动作回执确定性判定。
- [x] T7 固化录制用例 `caseFingerprint`；回放采用深拷贝快照并在启动前校验调用方预期指纹，内容变化时拒绝执行。
- [x] T8 增加目标应用冷/暖启动多轮测量，输出 min/max/mean/P50/P90/P95，并支持 P95 基线与最大退化比例判定。
- [x] T9 启动性能结果写入独立 Startup 产物目录并纳入证据索引；Web 性能页提供包名、模式、轮数、间隔和基线参数。
- [x] T10 增加所有权隔离、幂等回执、旧观察拒绝、证据路径/摘要、三态判定、用例篡改和启动统计回归测试。

## 关键语义

```text
runId / attemptId
        └─ sessionId + ownerToken + generation
              └─ observationId + stepId
                    └─ action receipt + evidence SHA-256
                          └─ passed / failed / not_tested
```

- `ownerToken` 是停止和修改活动会话的能力凭据，只在内存及实时状态中存在，不进入运行产物。
- 页面、Activity 或特殊场景发生变化后，旧观察对应动作必须返回 `stale_observation`。
- 执行完成不等于验证通过；没有验收检查点的成功执行为 `not_tested`。
- `caseFingerprint` 约束整份标准化用例内容，不能只校验用例名称或动作数量。
- 启动基线比较采用 P95 且至少需要 5 个有效样本；样本不足、系统命令缺字段或未提供有效基线时不能伪造通过结果。
- 冷启动统计采用 Android `TotalTime`；暖任务从桌面恢复时部分系统不返回 `TotalTime`，此时明确标记 `durationSource=wait_time` 并采用 `WaitTime`，不把缺失字段伪装成零耗时。

## 验证

- [x] Agent 全量 Go 测试通过。
- [x] Windows CGO 竞态检测门禁通过。
- [x] Web JavaScript 语法检查通过。
- [x] Linux/Android ARM64 与 ARMv7 Agent 交叉构建通过，临时二进制核验后已清理。
- [x] Android 13/15/16 真机冷/暖启动、基线判定和证据完整性验证；目标应用使用系统 Gallery。

真机证据见 [三设备启动性能验证](../validations/runs/validation-trusted-execution-startup-20260912.md)。
