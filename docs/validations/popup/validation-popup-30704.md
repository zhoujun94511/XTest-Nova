# Nova Popup 30704/30706 真机验证

验证日期：2026-09-04。设备：Samsung Android 13（SDK 33），验证包：
`com.xtest.nova.fixture`。原 XTest 10248 的 Android 16 截图作为交互基线，未被覆盖。

## 验证结果

- `popup start` 直接出现五项悬浮菜单，不留下可见 Activity；
- 三个业务入口均先进入带图标、名称和滚动能力的应用列表；
- 性能页持续取得所选验证应用的真实进程指标并可停止返回；
- 从悬浮窗新建录制，加入一个安全返回键动作后保存；任务列表显示动作数 1；
- 重启 Agent 与 Popup 后用例仍可见；从悬浮窗加载并回放，结果为 1/1 完成；
- 后台回放完成后界面自动返回任务列表，不停留在伪运行态；
- Monkey 启动进程包含 `-p com.xtest.nova.fixture`，停止后 API 和进程均为非运行态；
- 最小化窗口为 79×79 像素橙色气泡，点击后恢复完整五项菜单；
- 点击退出后 Popup 报告 `running=false`，Agent PID 未变化；
- Companion APK 使用项目专用证书签名，APK Signature Scheme v3 校验通过；
- Agent 全量 Go 测试、Companion/Runner/验证夹具构建通过。

30706 追加验证：录制控制条显示实时动作数，并提供“记录返回键”和“截图断言”；两项
均从悬浮窗成功写入，界面显示 2 个动作。录制请求携带归一化悬浮控件排除区，白盒测试
确认从该区域开始的点击和滑动不会进入业务用例。

## 边界

ADB 的 `input tap` 位于 Linux 输入设备事件层之上，不会被基于 `getevent` 的
TouchReader 当作物理触摸采集。因此自动验证通过录制 API 加入白名单返回键动作，
用于证明悬浮窗启动会话、非空动作持久化、列表加载、真实注入回放及完成态回收。
Protocol-B 物理触摸采集能力由既有 TouchReader 真机资格覆盖；本报告不把 ADB 模拟
点击错误表述为物理触摸录制证据。

独立 Android 16 环境仍未提供；为保护当前设备上的原 XTest 10248，本轮没有覆盖安装。

本报告只证明上述基础链路，不代表原 Popup 第二层语义已全部对齐。原源码还有性能 CSV、
录制任务/最终文本，以及 Monkey 的低电量、Activity 黑白名单、目标页用例和控件黑名单，
这些差距由 [`popup-alignment-audit.md`](../../audits/popup-alignment-audit.md) 与 [`todo.md`](../../reference/todo.md) 继续跟踪。
