# 真实场景运行时健壮性整改计划

## 原则

- Foloy 只作为复现与验收样本，生产实现不得使用其包名、Activity、文案、坐标或页面结构。
- 修复落在 scrcpy 协议语义和探索会话生命周期，适用于所有 Android 应用。
- 随机输入仍受 seed、等价类、字段去重和全局步骤预算约束。

## Todo

- [x] R1：剪贴板注入等待指定 sequence 的 `clipboardAck`，忽略 clipboard change、UHID 等无关设备消息和其他 sequence 的 ACK。
- [x] R2：WebSocket 控制通道复用相同的设备响应匹配逻辑，避免两条输入路径行为分叉。
- [x] R3：探索 Manager 增加显式停止意图；停止请求到达后，后续采集/动作错误不得覆盖 `stopped`。
- [x] T1：增加无关事件、错误 sequence、正确 ACK、超时和取消场景的 scrcpy 单元测试。
- [x] T2：增加采集阻塞期间显式停止的并发回归测试，断言 `stopReason=stopped` 且无错误文本。
- [x] V1：Go 全量测试、静态检查和 Java Runner 自检通过。
- [x] V2：真机连续执行多轮 ASCII/CJK/符号/Emoji 剪贴板输入，确认无 `unexpected scrcpy clipboard response`。
- [x] V3：滚动只按 viewport 变化判定进展；保留 Compose、WebView、原生列表跨样本验证项，不加入 Foloy 特判。

## 验收标准

1. 无关设备消息不能中断文本输入，只有匹配 sequence 的 ACK 才确认成功。
2. ACK 超时、控制连接关闭和调用方取消仍返回可区分错误。
3. API 显式停止得到稳定的 `stopped`，不再因 `context canceled` 误报 `safety_stop`。
4. 运行时源码不出现 Foloy 标识或领域规则。

## 实施与验证结果

- scrcpy 响应等待现在按事件类型与 64 位 sequence 匹配；无关消息和其他请求的 ACK 被消费并忽略，连接关闭、超时和调用取消保持独立错误。
- `Manager.Stop` 在取消上下文前记录停止意图；运行协程之后收到的采集或动作取消错误会统一收敛为 `stopped`。
- Go 全量测试、`go vet` 与 Java `NodeExplorer self-test` 通过。
- 设备 `R5CN30EQKNM` 上连续执行 3 轮，每轮 18 个唯一输入类别，共 54 次真实注入；剪贴板响应错误为 0。三轮主动停止均为 `stopReason=stopped` 且 `error` 为空。
- 真机证据：`tests/reports/runtime-robustness-device-validation.json`、`tests/reports/runtime-robustness-device-validation-repeated.json`。
