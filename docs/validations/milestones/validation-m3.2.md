# M3.2 验证记录

验证日期：2026-09-03。

## 实现范围

- 从 UiAutomator `scrollable=true` 节点生成边界内的向前滑动动作；
- 新状态记录父状态及进入动作，形成可回溯的深度优先状态图；
- 点击进入的子状态使用受限 `BACK` 返回，滚动形成的子状态使用原坐标反向滑动恢复；
- 滚动、回溯、弹窗恢复均由独立配置显式启用；
- `maxBacktracks` 限制返回与反向滚动总次数；
- 安全恢复词只包括取消、稍后、以后再说、跳过、继续等待及对应英文，开启后优先于普通点击；
- 状态增加滚动、回溯和恢复计数，状态图节点公开父状态；
- 会话状态回显滚动、回溯、弹窗恢复和最大回溯配置；
- 保持 M3.1 的目标包、危险文本、密码框、最大步数和双引擎互斥边界。

M3.2 仍不确认系统授权、不操作其他应用、不把未知弹窗视为可恢复状态。目标包离开前台会立即安全停止。

## 自动化结果

- `go test ./agent/...`：通过；
- `go vet ./agent/...`：通过；
- `go test -count=20 ./internal/exploration`：20 轮通过；
- 滚动动作边界、滚动开关、安全弹窗优先级、点击回溯、反向滚动、DFS 图边和最大回溯配置测试：通过；
- ARM64/ARMv7 Agent：构建通过；
- Runner Dex/JAR：构建通过；
- Companion APK：构建、对齐及维护证书签名通过；
- Nexus 历史合同覆盖保持 74/77，三个危险入口继续禁用。

## Android 16 真机结果

测试设备：`83fc400c`，SDK 36，`arm64-v8a`。Nova 使用 17914/18914/17892 独立测试端口。

| 检查项      | 结果                                                            |
|----------|---------------------------------------------------------------|
| Agent 版本 | `xtest-nova-0.7.0-m3.2`                                       |
| 页面预览     | 成功解析真实页面的 57 个节点                                              |
| 点击候选     | 11 个                                                          |
| 滚动候选     | 2 个，证明 M3.2 模型已识别真实可滚动容器                                      |
| 安全恢复候选   | 当前页面 0 个，未误标普通操作                                              |
| 执行保护     | 同时启用 scroll/backtrack/recovery 但 `execute=false` 时返回 HTTP 409 |
| 动作副作用    | 会话未运行，steps、scrolls、backtracks、recoveries 均为 0                |

验证没有执行点击、滚动、返回、按键或文本输入。完成后已停止测试 Agent，删除 Nova 专属 Agent、日志和临时 XML，并移除三个端口转发。

## 产物

| 产物                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `7DE5C103FE433CA9787B517A7C65DFE4E85B920B5EB42A50CA43B7097A5118DB` |
| `xtest-nova-agent-armv7`   | `49F48C97C10A9856A430A8E69F74281C2EEB68D317B0295FDED56A6AF91CE758` |
| `xtest-nova-runner.jar`    | `81E6F1683892A3B41D8A63058207FA2EBB7D4CE0262E0846263501CE810605D9` |
| `xtest-nova-companion.apk` | `2AE80BBDE0456898B0B779739B29B5571568747034FD2F7BF0F75645D582DB16` |

Companion 包名为 `com.openatx.xtest.popup`，`versionCode=20200`，`versionName=2.2.0-m3.2`。APK Signature Scheme v3 验证通过，签名证书 SHA-256 为：

```text
e49a7c6fb2ee0dbc4e6d3b6790d9dcf981d80f6b76628eb84755e7e1da3a46da
```
