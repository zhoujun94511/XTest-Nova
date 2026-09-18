# Foloy 深度探索加固实施计划

目标：落实 [`foloy-60min-device-audit-20260910.md`](../audits/foloy-60min-device-audit-20260910.md) 的审计结论，并以现有 Go Explorer、Java Runner、Runner HTTP 状态和长测脚本为边界完成修复。

## Todo

- [x] P1：Java 循环历史改为固定窗口，不再因新场景清空；循环键纳入语义动作和目标。
- [x] P1：覆盖周期 1～4、动态物理指纹、跨 Activity 和同动作多目标的自检。
- [x] P1：新增滚动 viewport 指纹，连续停滞有界，内容位移可识别。
- [x] P1：对未知外部页面执行有限 BACK 恢复；失败仍安全停止，长测编排继续下一会话。
- [x] P2：过滤零面积、屏外和无屏幕交集的动作节点。
- [x] P2：物理场景身份移除裸 index，补入归一化语义与相对位置。
- [x] P2：Runner 运行状态增量暴露事件数、最后事件时间、当前 Activity/场景/动作和外部包。
- [x] 验证：Java self-test、Go 单元测试、静态检查和构建通过。
- [x] 真机：Foloy 定向验证输入、滚动、循环、外部恢复和实时指标。

## 完成标准

- 周期 1～4 在第三次重复前后被识别并阻断，不受物理指纹漂移影响。
- `[0,0][0,0]` 等无效节点不生成动作。
- 同结构但可见内容变化的滚动可判定为 progress，连续无变化才判定 stall/封禁。
- 外部页面恢复次数有上限；无法恢复时明确结束，不进入无界 BACK。
- 运行中状态接口不再长期保持全零，`lastEventAt` 随事件更新。

## 验证结果

- 10 分钟 Foloy 定向回归：Go 4 个会话共 62 步、36 状态、16 次输入、6 次循环检测；Java 5 分钟持续运行到预算停止，共 35 个动作、18 状态、4 次输入。
- Java 真机运行中指标从 0 实时增长到 33；最终记录 1 次 `cycle_detected`、4 条 `edge_blocked`，修复前 6 个会话均为 0。
- Java 真机事件中 `[0,0][0,0]` 动作数量为 0；系统 Photo Picker 通过有限 BACK 在同一会话恢复。
- Java 本轮 3 次滚动均为真实无内容变化的 stall；viewport 前后签名相同。viewport 内容变化的 progress 判定已由 self-test 覆盖，Go 真机本轮 2 次滚动有 1 次 progress。
- Go 初次真机恢复暴露 Android `monkey` 为 shell 包装脚本，直接执行会 `exec format error`；现已改为固定 `/system/bin/monkey` 经 `sh` 启动，并同步覆盖普通外跳与 DFS 恢复。
- 最终 Go 定向种子在同一会话完成 3 次外部恢复，阻断边增至 5，随后以 28 步、13 状态正常 `state_exhausted`，未再出现 exec format error。

证据：`tests/reports/foloy-hardening-validation-20260910`、`tests/reports/foloy-external-recovery-final.json`。

## 随机输入扩展 Todo

- [x] 用 seed 与字段身份生成可复现随机值，移除 4 个固定字符串语料。
- [x] 覆盖空值、空格、字符集、数值、结构化文本和 31/32/33/最大长度边界。
- [x] Go/Java 支持每字段用例数及最大长度配置，并设置硬上限。
- [x] 编辑框内容从场景身份中归一化，避免随机输入制造伪新场景。
- [x] 日志记录输入类别、Unicode 长度和 seed，不新增输入明文日志。
- [x] 在 Foloy 搜索框完成生成语料真机抽样验证：固定 seed `2026091003` 实际执行 18 次，18 个类别全部唯一，且 0/1/31/32/33/64 长度边界均命中并成功执行。
