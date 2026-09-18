# 性能零值与录制保存整改计划（2026-09-14）

## 目标

修复性能数据将预热、静止、不支持和失败混写为数值 `0` 的问题；在保留 Nova 执行身份、用例指纹、动作回执和 Result Judge 的基础上，选择性吸收 SoloPi 的语义录制、可恢复草稿和完整用例包设计。

## Todo 与完成状态

- [x] P0-1 性能会话使用相邻采样周期的 `/proc` 增量计算应用 CPU；第一条样本标记为 `warming_up`，PID 集合变化时重新预热。
- [x] P0-2 CPU、FPS、Jank、GPU、电池电流、内存和网络增加 `metricStates`，区分 `warming_up / measured / idle / unsupported / failed`。
- [x] P0-3 CSV 的预热 CPU、空缺内存子项和不可计算 Jank 保持空单元格，并增加 `metric_states` 列；摘要不再把预热样本和无渲染帧的 Jank 当作有效零值。
- [x] P0-4 Companion 使用结构化 JSON 读取性能值，显示“预热中、静止、空闲、不支持、失败”，不再用统一默认值 `0` 掩盖缺失字段。
- [x] R0-1 录制开始即创建原子更新的 `draft.json`；每次归并动作后更新草稿，截图参考图片同步写入草稿目录。
- [x] R0-2 Agent 重启后可列出遗留草稿，通过 API 或悬浮窗“恢复并保存”；另提供显式删除草稿 API。
- [x] R0-3 完成录制后生成 `case.json`、`manifest.json`、`evidence.json` 和 `screenshots/`，manifest 记录用例指纹、显示尺寸、动作数以及文件 SHA-256/大小。
- [x] R0-4 截图断言保留参考 PNG 和相对路径，不再只有不可查看的感知哈希。
- [x] R0-5 触摸按下时获取当前层级并保存控件 resource-id、文本、content-description、类名、归一化边界和观察指纹；回放优先按语义重新定位，失败时保留原坐标兜底。
- [x] R0-6 任务名和用例名支持中文，同时拒绝路径分隔符、控制字符、`.`、`..` 和超过 80 字符的名称。
- [x] V0-1 增加 CPU 预热/静止/有效增量、静止 Jank、CSV 空值与状态、语义节点移动、草稿恢复、中文名称、截图参考和 manifest 回归测试。
- [x] V0-2 完整发布门禁通过；签名 Companion、UIAutomator、运行时 Bundle 及 arm64/armv7 Agent 重新构建并生成发布清单。
- [x] V0-3 Android 13/15/16 Gallery 性能短会话验证首行 CPU 为空且状态为预热；后续分别输出 measured 或 idle；Android 16 GPU/电流明确为 unsupported。
- [x] V0-4 Android 15 完成中文录制名、截图断言和四文件用例包验证；Android 13 使用 `kill -9` 模拟 Agent 异常中断，重启后发现一动作草稿并成功恢复为正式用例。

## 真机证据

- Android 13 性能：`/sdcard/xtest-nova/com.sec.android.gallery3d/Perf/20260914_111334/`
- Android 15 性能：`/sdcard/xtest-nova/com.sec.android.gallery3d/Perf/20260914_111334/`
- Android 16 性能：`/sdcard/xtest-nova/com.miui.gallery/Perf/20260914_111335/`
- Android 15 中文截图用例：`/sdcard/xtest-nova/com.sec.android.gallery3d/Replay/相册流程/20260914T031401.776281494Z/`
- Android 13 中断恢复用例：`/sdcard/xtest-nova/com.sec.android.gallery3d/Replay/恢复流程/20260914T031422.602924161Z/`

## 保留边界

- 未引入 SoloPi APK、动态插件或运行依赖，也未复制其实现代码。
- IF/WHILE、复杂业务编排和批量调度不属于本轮重点。
- 语义定位失败时使用录制坐标是兼容兜底；后续可按业务需要增加“严格语义定位失败即停止”策略。
