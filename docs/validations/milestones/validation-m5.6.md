# M5.6 验证记录

验证日期：2026-09-03

版本：

- Agent：`xtest-nova-0.18.0-m5.6`
- Companion：`3.5.0-m5.6`（versionCode 30500）
- 验证夹具：`com.xtest.nova.fixture` 1.0（versionCode 1）

## 安全边界

验证器只安装无权限隔离夹具。Popup 规则同时要求页面包含唯一夹具文本且节点文本精确匹配，点击后夹具把文本改为 `Popup handled` 作为可观察证据。录制阶段只显式加入白名单返回键和截图断言；夹具拦截返回键，因此回放不会离开隔离 Activity。每个动作前仍由 Agent 校验前台包。

验证器发现已有 Nova Agent、同名夹具或既有 Nova 配置文件时拒绝覆盖。结束时停止 Popup、录制与回放，卸载夹具，恢复原前台，并删除本次 Agent、配置、用例文件及端口转发。

## 双机结果

| 检查         | Android 16 / Xiaomi 2510DPC44G   | Android 13 / Samsung SM-G9860    |
|------------|----------------------------------|----------------------------------|
| Popup 唯一规则 | 点击并观察到 `Popup handled`，停止成功      | 点击并观察到 `Popup handled`，停止成功      |
| 录制与完整性     | 2 个动作，封装和摘要校验通过                  | 2 个动作，封装和摘要校验通过                  |
| 回放         | 2/2，`completed`                  | 2/2，`completed`                  |
| App Event  | WebSocket 原样收到 40 字节事件           | WebSocket 原样收到 40 字节事件           |
| minicap    | `rotation 0`，PNG 41,730 字节       | `rotation 0`，PNG 46,402 字节       |
| Monitor    | TCP 7890 返回 `xtest-nova-monitor` | TCP 7890 返回 `xtest-nova-monitor` |

全部步骤通过，报告 schema 为 `xtest-m56-validation/v1`。流式校验客户端把 minicap 单帧读取上限限定为 16 MiB；Agent 侧的输入限制没有放宽。

## 构建与产物

| 产物                          | SHA-256                                                            |
|-----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`    | `B1BAFE1816326055186B4E0A8C3F1C738C73A4773DBE895F55D4ED777137B1A4` |
| `xtest-nova-agent-armv7`    | `15A8FAA83737C888ED06FADAF87F4B292B2D0A9890D3AE1BA794A8184BE7D023` |
| `xtest-nova-runner.jar`     | `71F6C0169314D594EB6E7F8E943ADCFD28153F725ED8F6520D96BE4A671048D4` |
| `xtest-nova-companion.apk`  | `61BB9EDF7DD98C591D8B830BDA9E47FA0939539FBFDC74893324C79F018DD393` |
| `xtest-nova-validation.apk` | `0D7D543829E6D728809224B78DFC99D28B50C105E2CC19EF5DFA7A05DC420DE9` |

完整发布门禁通过；恢复证书口令、验证报告和夹具 APK 均不进入生产发布清单。
