# M5.8F4 Monkey 证据归档与平板验收

验证日期：2026-09-04。Agent：`xtest-nexus-0.22.0-m5.9-compat`；Companion：
`3.7.14-monkey-artifacts-tablet`（30714）。本阶段不执行 Git 提交或远程上传。

## 实现结果

- Monkey 每次运行建立 `/sdcard/xtest-nexus/<目标包>/Monkey/<设备本地时间>/`，同秒会话使用数字后缀避免覆盖。
- 会话保存结构化事件、最终清单、JSON/文本 Activity 覆盖率，以及开始和结束截图；异常结束保存 `failure.png`。
- `run.json` 包含请求配置、最终状态和文件 SHA-256。目标页一次性令牌使用 `json:"-"` 排除，白盒测试确认不会写入清单。
- 兼容日志保留在 `/data/local/tmp/xtest-nova-monkey.log`，32 MiB 时替换一个 `.1` 备份。
- 录屏保存到 `/sdcard/xtest-nexus/<目标包>/ScreenRecord/<设备本地时间>/0.mp4`。调用未传包名时优先使用前台包，无法识别时使用 `device.unknown`。
- Monkey 和 ScreenRecord 分别按目标包/类型保留最近 200 个会话。保留操作只删除该类型最旧会话，不跨包、不跨类型清理。
- `smallestScreenWidthDp >= 600` 使用平板尺寸档。主菜单、紧凑面板和大面板由三个布局档统一派生，旋转时沿用 dp 规则而非设备像素硬编码。

## 白盒与资源边界

- 成功路径：`events.jsonl`、`run.json`、两种覆盖率、`start.png`、`finish.png` 均存在。
- 失败路径：非零退出保留停止原因、错误和 `failure.png`。
- 安全：调用令牌不落盘；截图必须通过 PNG 签名校验；清单使用原子替换写入。
- 并发：Runner 状态和会话所有权仍由单互斥状态机保护；三轮全量 `go test -race` 无数据竞争。
- 性能：每次 Monkey 固定增加两次关键截图和一次结束扫描；不逐事件截图。兼容日志上限 32 MiB，磁盘会话数量有界。
- 覆盖率语义：只统计 Runner 实际观察到的 Activity，不把未知 Activity 总数当作分母。

## Android 16 平板证据

设备：Samsung SM-X920，SDK 36，1848×2960，density 280。

1. 5 秒 Monkey 请求 `tablet-artifacts-localtime-30713` 正常完成，事件数 8、停止原因 `completed`、退出码 0、产物错误为空。
2. 会话目录为 `20260904_155506`，设备结束时钟为 `20260904_155513`，证实命名使用设备本地时间而非静态 Go 运行时的 UTC。
3. 会话生成 `events.jsonl`、`run.json`、`activity_coverage.json`、`activity_coverage.txt`、`start.png`、`finish.png`；清单包含五个可哈希证据文件的 SHA-256。
4. 4 秒录屏生成 `/sdcard/xtest-nexus/com.xtest.nova.fixture/ScreenRecord/20260904_155539/0.mp4`，大小 25,155 字节。
5. 30714 窗口实测：主菜单 266×455 px、应用选择 910×1260 px、性能窗 560×525 px。应用列表和性能数据均可滚动，红色关闭按钮完整可见。
6. 性能窗在竖屏和横屏均未越界；横屏后恢复 `accelerometer_rotation=1`、`user_rotation=0`。
7. 所有操作限定在序列号 `R52XA02NXAX`；另一台连接设备上的原 XTest 未被替换或调用。

本地截图及拉取产物保存在被 Git 忽略的 `tests/reports/tablet-artifacts-30713/`，用于本次人工复核，不进入首次提交候选。
