# M4.2 高级录制回放验证记录

验证日期：2026-09-03  
验证设备：Xiaomi 2510DPC44G，Android 16 / API 36，ARM64  
目标：在 M4.1 安全边界上补齐 UTF-8 最终文本、双击、多指、截图断言和中断续播。

## 已完成

- 相邻 350 ms、距离不超过 3% 屏幕对角比例的两次点击合并为 `double_tap`；
- Protocol-B 同期活跃的 2 至 10 个触点归并为 `multi_touch`，保留各触点索引、起止百分比坐标和总时长；
- `POST /v1/recordings/current/text` 记录输入法最终提交的 UTF-8 文本，可携带焦点坐标；
- `POST /v1/recordings/current/key` 仅接受返回键；
- `POST /v1/recordings/current/assertions/screenshot` 记录当前屏幕的 64 位感知哈希；
- 截图断言允许 `0..16` 汉明距离，失败以独立的 `assertion_failed` 停止；
- 文本回放先聚焦、全选、清空，再使用官方 scrcpy 4.0 UTF-8 剪贴板粘贴；
- 新增隔离的 scrcpy 控制会话：关闭视频且不替换现有画面流；
- 多指回放在同一 scrcpy 控制批次发送按下、移动和抬起帧；
- `resumeFrom` 支持从已确认动作序号续播，并重新建立相对时间轴；
- 用例格式保持 `xtest-recording/v1`，旧 M4.1 用例继续有效。

## 自动化验证

- `go test ./agent/...`：全部通过；
- `go vet ./agent/...`：通过；
- scrcpy 官方 server 固定 SHA-256 和既有协议帧测试继续通过；
- 控制专用 server 参数确认 `video=false`、`control=true`；
- 双击合并：2 次点击归并为 1 个双击动作；
- 双指归并：两个触点生成 1 个多指动作；
- 多指回放：生成 2 个按下、2 个移动、2 个抬起，共 6 帧；
- UTF-8 文本：中文、空格和 Emoji 原样交给控制接口；
- 截图哈希：相同画面通过，相反画面进入 `assertion_failed`；
- 中断续播：首动作完成后协作停止，再从 `resumeFrom=1` 完成第二动作；
- Nexus 历史合同维持 74/77，两个后台 Shell 方法和终端继续禁用。

Go 竞态检测仍因本机缺少 C 编译器无法启动，该项没有标记为通过。并发状态继续由互斥锁、可取消上下文、完成通道及停止/续播测试覆盖。

## Android 16 非破坏验证

本轮只录制并回放截图断言，没有录入或执行文本、点击、滑动、多指和按键：

| 项目        | 结果                       |
|-----------|--------------------------|
| Agent 版本  | `xtest-nova-0.10.0-m4.2` |
| 前台包       | `tcg.scanner.value.app`  |
| 用例动作数     | 1                        |
| 动作类型      | `assert_screenshot`      |
| 动作时间点     | 785 ms                   |
| 感知哈希      | `ff81d7e700000000`       |
| 最大允许距离    | 16                       |
| 跨客户端完整性校验 | 通过                       |
| 回放停止原因    | `completed`              |
| 已完成动作     | 1                        |
| 输入类动作     | 0                        |
| 清理        | `NOVA_M42_FINAL_CLEAN`   |

第一次状态查询发生在原始动作时间点之前，因此显示仍在运行；改为轮询明确终态后，同一只读链自然完成。该结果不冒充 Unicode 或多指真机注入通过：这些能力已有协议与状态机自动化验证，但仍需用户选择无破坏测试页面后再做端到端动作验证。

## 当前边界

- 文本语义由 Companion 或控制端在输入法最终提交后显式写入，不记录 composing 候选态；
- 不采集或恢复密码字段明文；
- 暂不支持视频断言、条件分支和循环策略；
- 截图感知哈希适合页面整体回归，不替代区域控件断言；
- 多指真机注入和 Unicode 真机写入需在专用测试页面专项验证。

## 交付物

| 文件                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `E53C29042C767489A09263A7C3EAF328244998BDCBC6D8076C62C2B85F37B4B5` |
| `xtest-nova-agent-armv7`   | `BCDDE3F8A5215C732A47BD8820080FF86C13CAF62111356E2906E0D7A2438C58` |
| `xtest-nova-runner.jar`    | `2A693DFA97A3BFF9F77AE6AD84EFD903F74D99C06417323428985B1D6D92246E` |
| `xtest-nova-companion.apk` | `752B021893F26ADF30EE5387955A881CE2FEA5874E65C30B813FAC1CB1FD973C` |

Companion 为 `versionCode=20500`、`versionName=2.5.0-m4.2`。APK Signature Scheme v3 验证通过，签名证书 SHA-256：

`e49a7c6fb2ee0dbc4e6d3b6790d9dcf981d80f6b76628eb84755e7e1da3a46da`

仓库不保存签名口令或自动化协作者署名。
