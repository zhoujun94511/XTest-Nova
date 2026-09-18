# 运行时断链与并发审计整改计划（2026-09-11）

## 目标与边界

本轮将上一轮审计结论映射到现有 `Manager / Service / Runner / Companion` 边界内处理，不新增平行框架。整改优先级为：生命周期阻塞、并发所有权、循环收敛、客户端断链恢复、输入边界防御。

## Todo 与落实状态

- [x] R1 Runner 命令超时：stdout/stderr 改为独立线程并发排空，主线程先执行 10 秒超时等待；输出限制为 4 MiB，超时或超限时强制结束子进程。
- [x] R2 Monitor 连接回收：Service 跟踪所有已接入 TCP 连接，关闭服务时同时关闭监听器与存量连接；WebSocket 读端断开时主动关闭后端 TCP，使阻塞中的 Scanner 立即退出；补 accept/close 交错保护。
- [x] R3 scrcpy 响应所有权：共享画面会话只允许一个控制 WebSocket 持有设备响应队列，第二个控制端得到明确拒绝，消除 ACK/剪贴板事件被竞争消费者取走的风险。独立 `InjectEvents` 会话不受影响。
- [x] R4 智能遍历收敛：增加连续 20 次不稳定采集预算；成功获得稳定快照后重置连续计数，超过预算以 `unstable_capture_limit` 可诊断原因结束，避免前台持续切换导致无限循环。
- [x] R5 下载/安装任务背压：Manager 最多允许 8 个并发活动任务，超过上限返回稳定错误；HTTP 下载、包安装和兼容安装入口统一映射为 429。
- [x] R6 Companion 瞬时断链恢复：性能、录制、回放和 Monkey 状态轮询在网络异常时显示“暂时不可达”并以 2 秒间隔重试，不再把一次失败误判为会话结束；页面 generation 变化后自动停止旧轮询。
- [x] R7 录制追加代际校验：每次录制启动生成新 generation；文本、返回键、截图断言以及触摸回调在真正写入前再次校验 generation 和 Running 状态，阻断 stop/start 之间的 TOCTOU 串写。
- [x] R8 设备信息边界：`df` 成功但空输出时不再索引空切片；`wm size` 同时出现 Physical/Override 时采用最后一个有效值，以当前覆盖尺寸为准。
- [x] T1 Runner 增加真实休眠子进程超时自测，要求 250 ms 预算在 5 秒内完成清理。
- [x] T2 Monitor 增加服务关闭回收活动连接、WebSocket 断开解除后端阻塞测试；断开回归连续执行 10 轮。
- [x] T3 增加任务并发上限、scrcpy 单控制端、不稳定采集预算、录制 generation 串写、空存储输出和 Override 尺寸回归测试。
- [x] V1 `go test ./agent/... -count=1` 全量通过。
- [x] V2 `go vet ./agent/...` 通过。
- [x] V3 完整 `build.ps1` 通过，产出 Runner、Companion、UiAutomator 与 ARM64/ARMv7 Agent；Runner 两项自测通过。
- [x] V4 Samsung SM-G9860 / Android 13 执行 30 秒恢复力冒烟：48 次请求零失败、PID 稳定、goroutine 7→7、panic 0，且脚本完成 Agent 与端口转发清理。
- [ ] V5 Go Race：当前 Windows Go/CGO 环境在进入项目测试前由 `runtime/cgo` 失败，不能据此声明竞态检测通过；需在可用的 Windows race 工具链或 Linux CI 上补跑 `scripts/test-race.ps1` / `go test -race ./agent/...`。

## 验收结果

源代码整改与自动化回归均已完成，八项审计问题已有对应实现和防回归测试。真机基础生命周期验证通过；唯一未关闭项是受本机 Go/CGO 工具链限制的 Race Detector 资格项，不影响普通测试、静态检查或发布产物构建结果。

真机证据：`tests/reports/runtime-concurrency-remediation-device-20260911.json`。

## 后续门禁建议

1. 将 Monitor 断开测试、录制 generation 测试和不稳定采集预算测试保留在 Agent 常规测试集。
2. 发布 CI 必须在至少一个支持 Race Detector 的环境执行 V5；失败时不得把本清单升级为“竞态资格完成”。
3. 若未来需要多控制端 scrcpy，需引入按请求 ID/sequence 分派的单读取器，而不能移除当前单所有者约束后直接共享事件 channel。
