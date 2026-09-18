# 第三轮 Runner 产物收尾整改计划（2026-09-11）

## 问题边界

单独停止 Runner 后，原始事件日志已关闭并保留，派生产物会在 `finalizing` 阶段继续生成；但全局 Agent 关闭路径尚未等待该阶段完成，进程退出可能截断覆盖率、探索图、最终截图或最终清单。

## Todo

- [x] R7 为 Runner 增加可取消、可超时的产物收尾等待接口，不改变普通 `Stop` 快速返回语义。
- [x] R8 全局 `/stop` 在停止 Runner 后等待 `finalizing=false`，失败时拒绝退出 Agent 并返回明确错误。
- [x] R9 信号关闭路径在销毁截图/自动化依赖前等待 Runner 收尾，并在超时时记录错误。
- [x] T4 增加收尾等待的阻塞、超时和成功完成回归测试。
- [x] T5 增加全局关闭必须调用 Runner 收尾等待的 HTTP 回归测试。
- [x] V4 定向测试、Agent 全量测试和 `go vet` 通过。
- [x] V5 完整项目构建和 Android 真机恢复力验证通过，退出后进程及端口转发清理完成。

## 验收标准

1. 普通 Runner Stop 仍在子进程退出后快速返回，并通过 `finalizing` 暴露收尾状态。
2. 全局 Agent Stop 不会在 Runner 派生产物完成前退出。
3. 收尾等待支持上下文超时，不产生无限等待；超时不得被静默吞掉。
4. `finalizing=false` 后，最终清单、覆盖率、探索图和最终截图的写入流程已经结束。

## 实施结果

- Runner 使用每次运行独立的完成信号实现 `WaitFinalized(ctx)`；等待支持取消和超时，不采用轮询，也不改变普通 Stop 的快速返回语义。
- 全局 `/stop` 最多等待 10 秒完成 Runner 派生产物；等待失败会返回错误并取消本次 Agent 退出。系统信号关闭也会在销毁截图/自动化依赖前执行同样的有界等待并记录超时。
- Agent 全量测试、`go vet ./agent/...`、Runner/Companion/UiAutomator 与 ARM64/ARMv7 完整构建通过。
- Android 13 真机在 Runner 运行中调用全局 `/stop` 后，Agent 正常退出，`events.jsonl`、`run.json`、`activity_coverage.json`、`exploration_graph.json`、`finish.png` 均完整存在。
- 真机 Runner 报告：`tests/reports/third-runtime-finalization-runner-device-20260911.json`。
- 真机恢复力报告：`tests/reports/third-runtime-finalization-resilience-20260911.json`；48 次请求、0 失败、PID 稳定、panic 0。
