# M3.1 验证记录

验证日期：2026-09-03。

## 实现范围

- UiAutomator XML 页面树解析及节点领域模型；
- 对动态数字归一化的 128 位页面状态指纹；
- 可点击、启用、可见、非密码节点筛选，以及目标包边界检查；
- 内置中英文危险动作拒绝词，并支持附加拒绝词和文本/资源白名单；
- 根据固定 `seed` 和动作 ID 进行可复现的候选排序；
- 有界点击会话、状态图、动作轨迹、幂等 `requestId` 和协作式停止；
- Android 16 前台应用识别增加 `dumpsys activity` 回退；
- `/v1/exploration/preview` 只读预览，以及会话、状态图和步骤查询 API；
- 执行会话强制要求 `execute=true`，目标包离开前台时立即停止。
- 智能遍历与随机 Runner 互斥，固定 UiAutomator XML 文件的读写由独立锁串行化。

M3.1 不自动发送返回键、滚动、授权确认或跨应用恢复。以上行为进入 M3.2 前仍保持关闭。

## 自动化结果

- `go test ./agent/...`：通过；
- `go vet ./agent/...`：通过；
- `go test -count=50 ./internal/exploration`：50 轮通过；
- 页面过滤、动态指纹、规则白名单、前台边界、显式执行、单步上限、幂等请求、双引擎互斥和快速重启测试：通过；
- ARM64/ARMv7 Agent：构建通过；
- Runner Dex/JAR：构建通过；
- Companion APK：构建、对齐及维护证书签名通过；
- Nexus 历史合同覆盖保持 74/77，M3.1 使用独立版本化 API，不伪装为历史接口。

## Android 16 真机结果

测试设备：`83fc400c`，SDK 36，`arm64-v8a`。Nova 使用 17913/18913/17891 独立测试端口。

| 检查项      | 结果                                      |
|----------|-----------------------------------------|
| Agent 版本 | `xtest-nova-0.6.0-m3.1`                 |
| 前台包回退    | `dumpsys window` 空焦点时从 Activity 正确识别前台包 |
| 页面预览     | 成功解析真实页面，13 个节点、2 个安全候选、32 字符状态指纹       |
| 执行保护     | `execute=false` 返回 HTTP 409，会话保持未运行     |
| 目标包保护    | 非前台目标返回 HTTP 409                        |
| 动作副作用    | 会话步数为 0，没有注入点击、按键或文本                    |
| 状态查询     | 能力、会话、空状态图与空步骤接口均正常                     |

验证后已停止 Nova 测试 Agent，删除 Nova 专属 Agent 和日志，并移除三个临时端口转发；没有安装 Companion，也没有修改当前前台应用状态。

## 产物

| 产物                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `BAB1E88DA74B91D5978AFA8330A6AD6CF0A1D06E5B1447F0F4D744BDF2A5F9D4` |
| `xtest-nova-agent-armv7`   | `775C74CB534A06B54B8CDC0EEBDE78FDE554F22AEE130FF848F00B95C49773AB` |
| `xtest-nova-runner.jar`    | `79E318C74AA15829AFB0D7A3A51ADC2A422F24D59F32913D4152C9AD26BC997A` |
| `xtest-nova-companion.apk` | `01DCF75B37135E92652181E1320FFDBC99C5B2EBEDE58E534EEC395F2EE26D5C` |

Companion 包名为 `com.openatx.xtest.popup`，`versionCode=20100`，`versionName=2.1.0-m3.1`。APK Signature Scheme v3 验证通过，签名证书 SHA-256 为：

```text
e49a7c6fb2ee0dbc4e6d3b6790d9dcf981d80f6b76628eb84755e7e1da3a46da
```
