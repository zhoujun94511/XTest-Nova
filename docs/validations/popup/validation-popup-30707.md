# Popup 30707 对齐验收

> 历史快照：30708 已把新性能产物迁移到 `/sdcard/xtest-nexus/<目标包名>`，并补齐普通
> View FPS、GPU、电池与目标应用命名。当前结论见 [`validation-popup-30708.md`](validation-popup-30708.md)。

验收日期：2026-09-04。设备 `R5CN30EQKNM`，Android 13，1080×2400，密度约 2.8。
基准来自原 XTest Popup 10248 真机表现及恢复源码 `e1.a`、`e1.b`、`e1.d`。

## 已验证结果

- `popup start` 前后前台均为三星 Launcher；Manifest 不再注册可见配置 Activity，未出现标题页或空白页。
- 主菜单窗口 231×383 像素，对应 82×136dp；五项名称、顺序及退出红字与原版一致。
- 应用选择窗口 720×1523 像素，约为可用屏幕 2/3；图标、名称及滚动正常。
- 在夹具 `com.xtest.nova.fixture` 已强停时选择它，前台仍为 Launcher，性能窗显示“应用未启动”，证明选择行为不再隐式启动目标。
- 性能窗口 231×354 像素，对应 82×126dp；顶部显示应用名 `XTest Nova Validation`，而非 Popup 工具包名或目标包名。
- 性能内容使用 `ScrollView`；顶部和底部截图分别证明指标与结果路径均可查看。
- PID、CPU/系统 CPU、内存、FPS、接收/发送流量均带单位；修复后内存由设备 `total pss=14228 KB` 显示为 `13.89 MB`。
- 持续轮询从 5 行增长到 8 行，不再只刷新第一帧。
- CSV 写入 `/sdcard/XTest/com.xtest.nova.fixture/Perf/<设备本地 yyyyMMdd_HHmmss>/perf.csv`；表头固定，首条内存值非零。
- 点击关闭后状态变为 `running=false`；CSV 在停止时为 66 行，3 秒后仍为 66 行。
- 最小化窗口 56×56 像素，对应 20×20dp，可点击恢复。
- 录制根页/新建页/用例列表统一 5/6 屏；Monkey 配置和应用选择统一 2/3 屏。

## 证据文件

- `tests/reports/popup-30707/main.png`
- `tests/reports/popup-30707/minimized.png`
- `tests/reports/popup-30707/performance-top.png`
- `tests/reports/popup-30707/performance-bottom.png`

## 尚未关闭的差异

性能页仍缺原版 GPU、电池采集，FPS 在不支持当前 Surface 解析的设备上显示 `--`。因此本次关闭的是
启动遮挡、尺寸、单位、滚动、目标显示、持续刷新以及性能结果存储/命名问题，不将整个性能六类合同标为完成。
