# Popup 30710 可读性能窗与目标页用例验收

验证日期：2026-09-04。设备：Samsung Android 13（SDK 33）隔离夹具。

## 性能窗与电流语义

- 性能窗由机械复刻的 82dp×126dp 调整为 132dp×150dp，保留右上位置、深色结构、关闭
  按钮和纵向滚动，目标应用标题及 `-175.0 mA` 等完整单位不再被迫换行。
- API/CSV 保留厂商 `current_now/current_avg` 的原始有符号值，新增 `battery_status`；悬浮窗
  显示“电流原值”以及“充电中/放电中/未充电/已充满”。
- 截图采样时出现 `-175 mA`；复核时设备为 `USB powered=true`、`status=Full`、100%，
  瞬时值已变化为 `8`、平均值 `-2`。因此负号是厂商节点原始瞬时符号，不能脱离系统
  状态直接判定为放电。
- `tests/reports/popup-30710/performance-readable.png` 为 30710 真机证据：窗口实际为 371×422
  像素，目标应用标题、FPS、GPU、`电流原值: -13.0 mA`、`电池: 已充满 100%` 和温度均
  保持单行显示；纵向滚动继续提供网络、行数和文件路径。

## 目标页自动用例

- 创建并签名 `task=monkey`、`name=target-case` 用例，目标包为
  `com.xtest.nova.fixture`。
- Monkey 配置映射
  `com.xtest.nova.fixture/.ValidationActivity=monkey/target-case`。
- Agent 启动前解析最新匹配用例，验证完整性和目标包，为本次 Monkey 生成一次性 128 位
  随机令牌；重复映射和令牌重放会被拒绝。
- Runner 命中目标 Activity 后同步调用本机回环接口。调用未返回期间随机循环暂停，Agent
  执行 Replay；真机日志出现 `target_case_completed` 后 Monkey 恢复，并继续产生 6 个事件。
- 外部停止会同时终止 Runner 和正在执行的目标页 Replay；用例最长执行 5 分钟，超时会
  停止回放并使本次 Monkey 以明确失败结束。

白盒测试覆盖签名用例执行、完成动作数和一次性令牌防重放。Android 13 动态测试覆盖
任务用例落盘、目标页命中、Replay 完成和随机事件恢复。本报告关闭 M5.8F2e，M5.8F2
整体关闭；M5.8F3 的独立 Android 16 安装验收仍保持打开。
