# XTest Popup 对齐审计

审计日期：2026-09-04。交互基准为 Android 16 真机上的原 XTest Popup 10248，语义
基准补充采用恢复源码 `e1.a`、`e1.b`、`e1.d`；当前实现为 XTest Nexus Companion 30714。
结论依据代码、系统窗口、截图和真实副作用，不以路由存在或服务存活替代功能验收。

## 当前结论

Nova 已对齐第一层信息架构、目标应用性能六类入口、录制与 Monkey 第二层业务语义。
12 项全部通过。M5.8F2、M5.8F3 与 M5.8F 均已关闭。

| 对照项 | 原 XTest 10248 | XTest Nexus 30714 | 结论 |
| --- | --- | --- | --- |
| 无业务 Activity 遮挡 | 中转后保持原前台 | 无界面 Launcher，Android 13 前台不变 | 通过 |
| 主菜单信息架构 | 性能测试、录制回放、Monkey、最小化、退出 | 名称与顺序一致 | 通过 |
| 主菜单关键视觉结构 | 右上 82dp×136dp、深色矩形行、退出红字 | 112dp×220dp，名称、顺序、配色结构一致，五项触控区完整 | 通过；可用性优先，非逐像素复刻 |
| 应用选择 | 图标、名称、可滚动 2/3 屏列表；选择不启动应用 | 80% 屏宽、78% 可用屏高并设 360×640dp 上限；图标、名称、滚动可用，仅记录选择 | 通过 |
| 性能测试 | 82dp×126dp，持续显示、CSV 落盘、停止；CPU/内存/FPS/GPU/网络/电池六类入口 | 手机为 176dp×216dp；平板统一为 320dp×300dp、44dp 标题及关闭热区；标题、负号、单位和核心指标可读，网络、记录和文件可上滑查看；标题和目录使用目标应用 | 通过；SurfaceView/游戏按 W-003 限定 |
| 录制回放 | 任务、用例、触控/按键/最终文本/截图、播放 | 任务/用例层级、触控、返回键、截图断言、非密码聚焦输入框最终文本及播放已通 | 通过 |
| Monkey 配置 | 低电量退出、Activity 无/黑/白名单、目标页用例、控件黑名单 | 低电量阈值、Activity 黑/白名单、目标页观测、控件黑名单和 `Activity=任务/用例` 映射均进入配置 | 通过 |
| Monkey 执行 | UiAutomator 混合探索、名单约束、停止恢复 | Runner 执行低电量、Activity 与控件守卫；目标页命中时同步暂停随机事件，由 Agent 校验并执行已签名用例，完成后恢复 | 通过 |
| 最小化 | 20dp 橙色小窗口并可恢复 | 贴右边缘的半透明灰色窄把手；手机 12×32dp 可见、48×48dp 触摸，长按恢复 | 优化；降低自动探索误触风险 |
| 退出 | 结束 Popup，不停止 Agent | Popup 停止，Agent PID 不变 | 通过 |
| 状态切换 | 菜单、选择、配置、运行、停止、最小化 | 基础状态均闭环；回放完成自动返回列表 | 通过 |
| Nova Android 16 | 原 10248 基线通过 | 同字节码、不同包名的 30711 隔离包与 10248 并存；完整流程及撤权恢复通过，原包版本和更新时间未变化 | 通过 |

## 本轮新增修复

- 30704：持久化回放完成后自动返回任务列表，不再永久显示“回放中”。
- 30705：录制配置加入归一化悬浮控件排除区；从控制条区域开始的点击、滑动或多点动作
  不进入业务用例，避免用户点击“完成”被误录。
- 30706：录制控制条显示实时动作数，并接入返回键和截图断言；Android 13 真机从悬浮窗
  写入 2 个动作成功。
- 30707：删除可见配置 Activity，启动及选择目标均不改变原前台；主菜单、性能窗、最小化、
  应用选择、录制和 Monkey 配置尺寸分别对齐恢复源码的 82×136dp、82×126dp、20×20dp、
  2/3 屏、5/6 屏和 2/3 屏。
- 30707：性能窗改为应用名标题和可滚动布局，PID、CPU、内存、FPS、网络按单位格式化；
  修复轮询只刷新一次和 `total pss` 被误读为 0 KB 的问题。
- 30707：性能会话按 `/sdcard/XTest/<包名>/Perf/yyyyMMdd_HHmmss/perf.csv` 存储，目录名
  使用设备时区；停止后采样协程退出，CSV 行数不再增长。
- 30708：所有新设备产物统一迁移到 `/sdcard/xtest-nexus`；性能与回放的第二级目录均使用
  目标包名，性能窗标题使用目标应用 label。`XTest Nova Validation` 夹具改名为
  `目标应用样例`，避免验收素材把工具名称误认成被测应用名称。
- 30708：按原 XTest 的 `dumpsys gfxinfo <目标包>` 累计帧数思路实现帧差 FPS；首样本为
  `null`，静态页面为 `0`，动态页面返回真实差值。增加 KGSL/sysfs GPU 适配器和电池
  电流、百分比、温度采集；可选探针并发执行、单项 2 秒超时，失败不拖断 CPU/内存/网络。
- 30708：性能 CSV 增加 `fps`、`gpu_percent`、`battery_current_ma`、
  `battery_level_percent`、`battery_temperature_c`，并修复停止期间取消可选探针被误记为
  `signal: killed` 的问题。
- 30708：Companion 可见名称和通知名称统一为 `XTest Nexus`；内部 Go module 与构建目录
  暂保留 `xtest-nova`，它们不是设备产物或用户可见身份，避免在兼容收口期制造无关迁移。
- 30709：录制用例增加可选任务元数据并落入 `Replay/<任务>/<时间>/case.json`；悬浮窗可
  自动读取当前聚焦的非密码输入框，将最终 UTF-8 文本和输入框中心坐标写入完整性签名。
- 30709：Monkey 高级参数从悬浮窗经 8912 传入 Runner。低电量每 30 秒检查，Activity
  每 500 毫秒检查，控件层级最多每 3 秒检查，避免按 50 毫秒事件间隔重复启动重型探针。
  黑/白名单及控件命中会执行返回并跳过随机事件，目标 Activity 首次命中写入结构化日志。
- 30710：性能窗扩大为 132dp×150dp，标题、负号和单位不再被迫换行；电流明确标为
  `电流原值`，并单独显示系统电池状态，避免把厂商节点符号直接解释为充电或放电方向。
- 30710：目标页映射采用 `Activity=任务/用例`。Agent 在启动前查找最新匹配用例并验证
  SHA-256、目标包和一次性 128 位令牌；Runner 命中页面后同步等待 Replay 完成，因此等待
  期间不生成随机事件。真机日志顺序为 `target_page`、`target_case_completed`，之后继续
  产生 6 个随机事件，证明暂停—执行—恢复闭环成立。
- 30711：Android 16 运行中撤销悬浮权限后，权限守卫在 1 秒内停止服务并移除窗口；重新
  授权可恢复。使用不同包名的同字节码验收包完成性能、2/2 录制回放、23 事件 Monkey、
  60×60 像素最小化、恢复和退出，且原 10248 未发生版本、更新时间或数据变化。
- 30712：统一主导航、紧凑状态、内容面板、可滚动面板和最小化五类布局令牌；全部标题栏
  关闭入口改为带 36/48dp 固定热区的红色 `×`。性能窗扩大为 176×216dp，短录制菜单和
  新建录制表单按内容自适应；录制排除区复用实际窗口宽高及 dp 边距。Android 12 真机逐页
  验证关闭、性能滚动、录制、Monkey 和最小化恢复。
- 30714：`smallestScreenWidthDp >= 600` 使用平板档位；在 SM-X920 上将紧凑面板统一为
  320dp 宽，性能页为 300dp 高、44dp 标题/关闭热区，正文 14sp。竖屏、横屏和向下滚动
  均实测通过，解决固定手机档在大屏上比例过小的问题。
- 当前优化：最小化态不再使用高显著红橙圆点，改为贴边中性把手并从自动探索节点树中
  排除；单击仅安全吸收，长按才恢复菜单。Monkey/智能探索运行态继续完全移除窗口。
- 一动作持久化用例回放结果为 `actions=1`、`completedActions=1`、
  `stopReason=completed`，完成后 UI 正确返回列表。
- Monkey 基础运行时进程参数包含 `-p com.xtest.nova.fixture`，停止后 API 与进程均退出。
- 最小化窗口实测 79×79；退出前后 Agent PID 保持不变。

## 证据

原 XTest：

- `D:\projectx\XTest-Nexus\evidence\xtest-popup-10248-android16.png`；
- `D:\projectx\XTest-Nexus\evidence\popup-audit\performance.png`；
- `D:\projectx\XTest-Nexus\evidence\popup-audit\record-replay.png`；
- `D:\projectx\XTest-Nexus\evidence\popup-audit\monkey.png`；
- `D:\projectx\XTest-Nexus\evidence\popup-audit\minimize.png`；
- 恢复源码：`monkey-engine-recovered/SOURCE_CODE/02-original-engine-java-reference/e1/`。

Nova：

- `tests/reports/popup-30703/main.png`、`app-picker.png`、`performance-fixture.png`；
- `tests/reports/popup-30703/record-root2.png`、`new-record.png`、`record-running.png`、
  `case-list.png`；
- `tests/reports/popup-30704-minimized.png`；
- `tests/reports/popup-30707/main.png`、`minimized.png`、`performance-top.png`、
  `performance-bottom.png`；
- `tests/reports/popup-30708/performance.png`、`performance-bottom.png`；
- `docs/validations/popup/validation-popup-30707.md`；
- `docs/validations/popup/validation-popup-30708.md`；
- `docs/validations/popup/validation-popup-30709.md`；
- `docs/validations/popup/validation-popup-30710.md`；
- `docs/validations/popup/validation-popup-30711.md`；
- `docs/validations/popup/validation-popup-30712-layout.md`；
- `docs/validations/popup/validation-monkey-artifacts-30714-tablet.md`；
- `docs/validations/popup/validation-popup-30704.md`（包含 30706 增量结果）。

## 剩余实现顺序

1. 性能扩展：为 SurfaceView、游戏、高刷新率与受限厂商 GPU 节点增加经过校准的适配器；
   在此之前保持 `null`/缺省，不把普通 View FPS 宣称为全局帧率。

## FPS 方案依据

- 原 XTest 恢复代码以目标包调用 `dumpsys gfxinfo` 并读取累计渲染帧；30708 使用相同的
  包级累计帧差策略，而不是从悬浮工具自身取样。
- SoloPi 的性能说明同样将 FPS 定义为应用级指标，并指出静态页面可能产生不准确结果，
  建议在动态页面切换或滚动中测量。参见 <https://github.com/alipay/SoloPi/wiki/Performance>。
- ATX 的 SurfaceFlinger 脚本提供了 Surface 延迟时间戳方案，但当前 Android 13 设备只返回
  刷新周期、没有有效帧时间戳，因此尚未把它作为伪兜底接入。参考实现：
  <https://github.com/NetEaseGame/ATX/blob/master/scripts/surfaceflinger-fps.py>。
