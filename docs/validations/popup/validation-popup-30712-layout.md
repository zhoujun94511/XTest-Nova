# Popup 30712 统一尺寸体系与 Android 12 验收

验收日期：2026-09-04。设备为 Pixel 3a XL（Android 12L / SDK 32，1080×2160，
400dpi），正式签名包为 `com.openatx.xtest.popup` 30712。本轮针对性能窗比例、关闭控件
裁切和各状态各自硬编码尺寸的问题做专项审计，不改动 Agent 业务合同。

## 统一布局规则

| 语义层级  | 设计尺寸                        | Android 12 实测                 | 用途                       |
|-------|-----------------------------|-------------------------------|--------------------------|
| 主导航   | 112×220dp                   | 280×550px                     | 五项一级菜单                   |
| 紧凑状态  | 176dp 宽                     | 性能 440×540px                  | 性能、录制中、回放中、Monkey 运行中和消息 |
| 内容面板  | 360dp 上限、80% 屏宽；高度随内容       | 录制菜单 864×524px，新建录制 864×677px | 短菜单和短表单                  |
| 可滚动面板 | 同一宽度；高度为 640dp 上限或 78% 可用屏高 | 864×1591px                    | 应用、用例和 Monkey 高级策略列表     |
| 最小化   | 28×28dp                     | 70×70px                       | 可点击恢复气泡                  |

大标题栏统一为 48dp，紧凑标题栏统一为 36dp。所有标题栏关闭入口由无明确热区的红点
改为红色 `×`，并分别占满 48×48dp 或 36×36dp 的触控区域。紧凑浮动窗统一使用
20dp 右边距和 52dp 底部偏移。

## 修复结果

- 性能窗从 132×150dp 调整为 176×216dp；应用名、负号和单位不换行，红色关闭按钮不
  再贴边裁切。首屏显示 PID、应用/系统 CPU、内存、FPS、GPU、电流、电池与温度；上滑
  可查看接收、发送、记录行数和目标包名目录。
- 录制回放短菜单不再强制占满 78% 屏高，消除了大面积空白；新建录制表单同样按内容
  自适应。应用列表、用例列表和 Monkey 高级表单继续使用统一可滚动面板。
- 录制输入排除区不再使用旧的 150dp 宽度和裸像素偏移，直接复用 176dp 录制窗、20dp
  边距、52dp 底部偏移与 280dp 录制态高度，避免可见窗口与触摸过滤区域错位。
- 性能页上滑后完整看到 `/sdcard/xtest-nexus/com.xtest.nova.fixture/Perf/.../perf.csv`；
  点击 `×` 后性能会话状态为 `running=false` 并返回主菜单。
- 最小化窗口实测 70×70px，点击后恢复 280×550px 主菜单。

## 证据

- `tests/reports/popup-30712/android12-main.png`；
- `tests/reports/popup-30712/android12-app-picker.png`；
- `tests/reports/popup-30712/android12-performance-final.png`；
- `tests/reports/popup-30712/android12-performance-scrolled.png`；
- `tests/reports/popup-30712/android12-record-menu-final.png`；
- `tests/reports/popup-30712/android12-record-form.png`；
- `tests/reports/popup-30712/android12-monkey-form.png`；
- `tests/reports/popup-30712/android12-minimized.png`。

验收结束后停止性能采样、停止 Popup、卸载正式 Companion 与验证夹具、移除本轮端口转发
和临时设备文件，并恢复 Pixel Launcher。另一台 Android 16 设备上的原 Popup 10248 未被
覆盖或操作。
