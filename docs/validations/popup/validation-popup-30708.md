# Popup 30708 性能与命名对齐验收

> 历史性能快照：录制与 Monkey 的当前增量结论见 [`validation-popup-30709.md`](validation-popup-30709.md)。

验证日期：2026-09-04。设备：Samsung Android 13（SDK 33，序列号仅在本地报告保留）。
Android 16 设备继续保留原 XTest Popup 10248，本轮未覆盖安装。

## 验收结果

- Companion `versionCode=30708`，可见应用名、前台服务通知名称均为 `XTest Nexus`。
- 启动和选择目标后，前台保持验证夹具；悬浮性能窗标题为目标应用 label
  `目标应用样例`，不是 Companion 或 Agent 名称。
- 性能会话路径为
  `/sdcard/xtest-nexus/com.xtest.nova.fixture/Perf/20260904_134017/perf.csv`：根目录表示工具，
  第二级目录表示目标包，目录时间使用设备本地时区。
- 动态点击目标应用期间，悬浮窗观测到 FPS 5.1（另一短会话最大 5.83）、GPU 12.0%
  （短会话 10%–21%）、电流 -175.0 mA、电量 100%、温度 31.8°C。
- 最终会话正常停止，`running=false`、`rows=148`、`error` 为空；最后静止样本 FPS 为 0，
  没有把旧帧率或固定常量继续展示为实时结果。
- CSV 表头包含 PID、应用/系统 CPU、内存、FPS、GPU、电池电流/电量/温度和网络收发。
- 新装设备上 Companion 包不存在时，部署脚本不再因可选的旧版本查询失败而中断。

## 采样语义

FPS 使用 `dumpsys gfxinfo <目标包>` 的 `Total frames rendered` 累计值，以同一目标包相邻
样本的帧差除以设备单调时钟差。首个样本没有基线，返回 `null`；有基线但页面无新帧时
返回 `0`。这适合普通 View 渲染链，但不承诺覆盖 SurfaceView、游戏或所有厂商合成路径。

GPU 优先读取 KGSL 百分比节点，其次尝试通用 busy 节点；电池电流读取标准 power_supply
节点并统一换算为 mA，电量和温度来自 `dumpsys battery`。不可访问的可选字段保持空值，
CPU、内存和网络基础采样不会因此失败。可选探针并行执行并各自设置 2 秒上限。

## 证据

- `tests/reports/popup-30708/performance.png`：目标应用标题及动态 FPS/GPU/电流；
- `tests/reports/popup-30708/performance-bottom.png`：电量、温度、网络、记录行数和目标包路径；
- `agent/internal/system/service_test.go`：帧差、GPU、电池解析与单位换算；
- `agent/internal/perflog/manager_test.go`：扩展 CSV 与协作停止取消语义。

本报告关闭 M5.8F2c 的普通应用性能采集范围。SurfaceView/游戏及 Android 14–16 厂商
节点矩阵继续由 W-003 跟踪；M5.8F2d（录制任务/最终文本）、M5.8F2e（Monkey 高级策略）
及 M5.8F3（独立 Android 16 安装验收）在本历史快照时仍未关闭；后续已由
[`validation-popup-30711.md`](validation-popup-30711.md) 完成。
