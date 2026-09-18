# XTest Nova 文档索引

本目录保存产品真源、接口合同、实施计划和历史验证记录。文档按内容归入
固定分类目录，历史验证不因里程碑结束而删除；只有确认被后续文档完整替代、
没有剩余引用且不含独立结论的文件才会移除。

## 目录分类

- [`compliance/`](compliance/)：HTTP 合同、兼容矩阵、豁免和第三方 SBOM。
- [`reference/`](reference/)：架构、使用、发布、迁移和项目级权威说明。
- [`api/`](api/)：探索、Monkey 和录制回放接口。
- [`guides/`](guides/)：测试策略、设备矩阵、韧性和安全夹具指南。
- [`audits/`](audits/)：代码、设备、交互和对齐审计快照。
- [`plans/`](plans/)：设计方案及分阶段实施计划。
- [`todos/`](todos/)：仍需追踪或保留整改过程的任务清单。
- [`validations/milestones/`](validations/milestones/)：M2 至 M5 里程碑验证。
- [`validations/popup/`](validations/popup/)：Popup 与 Companion 专项验证。
- [`validations/runs/`](validations/runs/)：按设备、日期或场景执行的验证。
- [`adr/`](adr/)：架构决策记录。
- [`../tests/archive/`](../tests/archive/)：只用于追溯的历史测试场景和基线。

以下仓库级文件按生态约定保留在项目根目录，不参与 docs 分类迁移：

- [项目入口](../README.md)
- [English README](../README_en.md)
- [版本变更记录](../CHANGELOG.md)
- [项目权利与分发声明](../NOTICE.md)
- [第三方软件清单](../THIRD_PARTY_NOTICES.md)
- [MIT 许可证](../LICENSE)

## 状态约定

- **权威参考**：描述当前架构、接口、安全边界或发布规则，修改代码时应同步更新。
- **当前计划**：仍包含开放项、外部阻断或尚未通过的产品验收。
- **完成记录**：实施已结束，但保留决策、取舍和验证摘要。
- **历史验证**：特定版本、设备或日期的快照，不代表当前版本自动取得同等资格。
- **外部证据**：`tests/reports/` 中的真机原始数据默认不进入 Git；文档必须保留足以独立理解结论的摘要，不能把本地报告路径当作唯一证据。

日期后缀使用 `YYYYMMDD`。`validation-*` 表示验证快照，`*-todo-*` 或
`*-plan-*` 表示实施跟踪，`*-audit-*` 表示审计结论。

## 权威参考

- [总体架构](reference/architecture.md)
- [使用说明](reference/usage.md)
- [LAN 认证、令牌轮换与恢复默认](reference/usage.md#32-生成-lan-令牌文件)
- [危险兼容 API 安全说明](guides/legacy-unsafe-api.md)
- [发布与回滚](reference/release.md)
- [完整 HTTP 合同](compliance/http-contract.md)
- [兼容矩阵](compliance/compatibility.md)
- [兼容资格与豁免](compliance/compatibility-waivers.md)
- [迁移计划](reference/migration-plan.md)
- [发布前审查](reference/pre-release-review.md)
- [主里程碑 Todo](reference/todo.md)
- [全链路审计整改](reference/audit-remediation-todo.md)
- [第三方 SBOM](compliance/third-party-sbom.json)
- [架构决策记录](adr/)

上述合同、兼容矩阵、豁免和 SBOM 被首次提交或发布门禁读取，不得随意移动。

## API 与测试策略

- [智能遍历 API](api/exploration-api.md)
- [Monkey 高级策略 API](api/monkey-api.md)
- [录制回放 API](api/record-replay-api.md)
- [生成输入策略](guides/generated-input-strategy.md)
- [设备矩阵验证](guides/device-matrix.md)
- [韧性与长稳验证](guides/resilience-testing.md)
- [副作用隔离夹具](guides/side-effect-fixtures.md)
- [`--legacy-unsafe-api` 安全使用说明](guides/legacy-unsafe-api.md)
- [UiAutomator Provider 计划](plans/uiautomator-provider-plan.md)
- [图内 Anti-Ping-Pong 设计](guides/anti-pingpong-design.md)
- [XTest Monkey 字节码行为审计](audits/xtest-monkey-bytecode-audit.md)
- [Popup 对齐审计](audits/popup-alignment-audit.md)

## 当前计划与开放验收

- [剩余工作 Todo](todos/remaining-work-todo-20260909.md)
- [覆盖优先探索计划](plans/coverage-first-exploration-plan-20260912.md)
- [性能采集整改](todos/performance-audit-remediation-todo-20260912.md)
- [成熟性能工具对齐](todos/performance-mature-tools-alignment-todo-20260914.md)
- [广告特殊场景整改](todos/ad-special-scene-remediation-todo-20260914.md)
- [依赖整改](todos/dependency-remediation-todo-20260912.md)
- [运行时并发审计](todos/runtime-concurrency-audit-todo-20260911.md)
- [游戏跨代际 Ping-Pong 整改](todos/generation-pingpong-remediation-todo-20260915.md)
- [游戏轻量探测架构](plans/game-detection-lightweight-architecture-20260915.md)
- [游戏渲染探测整改](todos/game-render-detection-remediation-todo-20260915.md)
- [SurfaceView FPS 整改](todos/game-surfaceview-fps-remediation-todo-20260915.md)

Todo 中的 `[x]` 只表示对应实施或验证步骤完成。文档明确写有“产品验收未通过”
或仍含 `[ ]`、`[~]`、`[!]` 时，不得将整个主题视为关闭。

## 2026-09-15 游戏与场景专项

- [Unity Phase 2 修复计划](plans/unity-pingpong-remediation-plan-20260915.md)
- [跨代际 Ping-Pong 整改](todos/generation-pingpong-remediation-todo-20260915.md)
- [安全与效率复测整改](todos/post-retest-safety-efficiency-remediation-todo-20260915.md)
- [游戏证据与包体门禁](todos/game-evidence-size-gate-todo-20260915.md)
- [场景 Monkey 20 分钟验证](validations/runs/validation-scenario-monkey-20m-20260915.md)
- [场景恢复 Todo](todos/todo-scenario-recovery-20260915.md)
- [场景恢复验证](validations/runs/validation-scenario-recovery-20260915.md)
- [最小化悬浮窗 20 分钟验证](validations/runs/validation-minimized-overlay-20m-20260915.md)

## 历史验证

以下文档按文件名前缀和日期保留在对应验证目录：

- `validation-m*.md`：M2 至 M5 的里程碑验证。
- `validation-popup-*.md`：Companion/Popup 分版本验证，汇总结论以
  [Popup 对齐审计](audits/popup-alignment-audit.md)为准。
- `validation-*-YYYYMMDD.md`：设备、场景或能力专项验证。
- `*-audit-YYYYMMDD.md`：设备或实现审计快照。
- 已完成的 `*-todo-*`、`*-plan-*`：保留实施原因和替代方案，不作为当前
  backlog；当前状态以本索引的“当前计划”和[主 Todo](reference/todo.md)为准。

历史验证中的设备型号、版本、指标和限制只适用于文档记录的运行条件。引用
被 Git 忽略的 `tests/reports/` 时，应优先阅读文档内摘要；需要原始证据时重新运行
对应脚本生成。

## 历史夹具与数据

- [`tests/archive/scenarios/`](../tests/archive/scenarios/)：历史场景和基线 JSON。
- [`tests/scenarios/`](../tests/scenarios/)：当前可执行场景与黄金基线约定。
- [第三方 SBOM](compliance/third-party-sbom.json)：虽为机器可读数据，但属于发布真源，
  不是可清理的临时生成物。

## 维护规则

1. 新增能力先更新权威参考，再增加实施计划或验证记录。
2. 验证文档必须写明设备、版本、参数、结果和未通过项。
3. 完成计划后保留决策摘要，并从“当前计划”移出；不要只改标题掩盖开放项。
4. 删除文档前检查 README、CHANGELOG、PLAN、脚本和其他文档的引用。
5. 不提交 `tests/reports/`、`dist/`、签名材料、设备日志或可重新生成的构建产物。
