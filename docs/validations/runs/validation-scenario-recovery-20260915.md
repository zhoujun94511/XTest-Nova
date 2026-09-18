# 场景 Monkey 恢复整改验证（2026-09-15）

## 实现对齐

### 广告退出

- `close_ready` 与 `confirm_exit` 使用独立尝试键，广告主页面的尝试不会耗尽确认框额度。
- 确认框优先匹配 `CLOSE/关闭/放弃奖励/退出`，显式排除 `RESUME/继续/继续观看`。
- 补齐真机文案 `lose your reward`；已知广告 Activity 可解析广告 SDK 或系统对话框包提供的节点。
- 单阶段三次明确关闭失败后重拉目标应用，并一次性清理当前广告遭遇状态，不再把同一广告循环计为新遭遇。

### 目标恢复

- 保留原有前台看门狗：Launcher 立即重拉，普通外部包先 Back、最多三次后重拉。
- 无进展触发重拉时建立 recovery 代际：保留黑名单/循环封禁、输入去重和语义转移计数，刷新普通动作尝试额度。
- `exploration_cycle_restarted` 新增 `historyMode=inherit|recovery` 证据字段。

## 自动验证

- Runner Java 8 编译、D8 与内置自测通过。
- 英文 `CLOSE/RESUME`、中文 `关闭/继续`、`lose your reward` 分类、广告阶段键隔离和恢复状态清理自测通过。
- 恢复代际自测确认普通动作重新可选，同时危险边封禁与输入去重仍保留。
- 全量离线 Go 测试、Go Vet、双架构 Agent 构建和发布门禁通过。

## 三机真机回归

| 系统 | 目标 | 时长 | 事件 | 场景 | Crash / ANR / Native | 悬浮窗违规 | 关键恢复证据 |
| --- | --- | ---: | ---: | ---: | --- | ---: | --- |
| Android 16 | ShortsWave | 311.1 秒 | 117 | 80 | 0 / 0 / 0 | 0 | 广告 Activity 50 条事件、Launcher 恢复 2 次 |
| Android 15 | ShortsWave | 310.2 秒 | 147 | 103 | 0 / 0 / 0 | 0 | 广告 Activity 96 条事件、`historyMode=recovery` 1 次、Launcher 恢复 1 次 |
| Android 13 | Gallery | 315.4 秒 | 179 | 94 | 0 / 0 / 0 | 0 | Launcher 恢复 14 次且事件持续增长 |

三台诊断均完整、产物 SHA-256 校验通过，自动化期间 Companion 窗口计数为 0，结束后 Companion `30718` 窗口和服务均恢复。两台 ShortsWave 本轮虽进入真实广告 Activity，但投放素材未再次给出奖励退出二次确认，因此该分支的确定性验证来自结构化中英文自测；不把随机投放缺失虚报为真机覆盖。

## 证据

- `tests/reports/scenario-recovery-20260915/android16-shortswave/`
- `tests/reports/scenario-recovery-20260915/android15-shortswave/`
- `tests/reports/scenario-recovery-20260915/android13-gallery/`
