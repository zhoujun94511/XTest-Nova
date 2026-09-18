# M5.7 验证记录

验证日期：2026-09-03

版本：

- Agent：`xtest-nova-0.19.0-m5.7`
- Companion：`3.6.0-m5.7`（versionCode 30600）

## 收口结果

逐项审查 M5.6 兼容矩阵中的 15 个 `partial` 后：

- 唤醒屏幕收口为 `implemented`：使用幂等 `KEYCODE_WAKEUP`，不会关闭已点亮屏幕；
- 13 类安全子集或兼容降级改为 `qualified`；
- minitouch 因缺少可信 ABI 二进制改为 `external-blocked`；
- 所有限制关联 W-001–W-012，并记录原因、风险控制和解除条件；
- 3 个无认证后台 Shell/终端合同继续为 `intentionally-disabled`，不通过豁免伪装成兼容。

发布门禁新增两项强制检查：兼容矩阵不得出现表格状态 `partial`，兼容豁免登记必须存在。矩阵引用的 W-001–W-012 与登记项逐一对应，无悬空编号。

## 资格原则

- 路径白名单、固定回环代理和禁用无认证命令入口属于安全设计，不以完全复制 Nexus 风险行为为目标；
- FPS 不可靠时返回 `null`，minicap 降级明确为 1 FPS PNG，不伪造指标或性能；
- 缺少真实设备、业务包、后端或可信二进制时标记外部条件，不把单元测试冒充真机完成；
- 解除豁免必须补充对应设备矩阵和可复现证据。

## 构建与门禁

- `go vet ./agent/...` 与 `go test ./agent/...` 通过；
- PowerShell 验证脚本解析通过；
- Nexus 合同保持 74/77，缺少的 3 项均为既定危险入口；
- Companion 证书 SHA-256：`E49A7C6FB2EE0DBC4E6D3B6790D9DCF981D80F6B76628EB84755E7E1DA3A46DA`。

| 产物                          | SHA-256                                                            |
|-----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`    | `A73A1F97354109D20F29301D0CA8FEF650B4BF3F6E4846D92AEA1C68657E20EF` |
| `xtest-nova-agent-armv7`    | `AEB2909FAB0097C74B5C25C421E0249231E82B00AFB016DD3A9D3C2505BF29BE` |
| `xtest-nova-runner.jar`     | `59A45C253A64D68690F8149CDCACE1225ABCA5BBBEA8E2049091D4D5F1C01F6F` |
| `xtest-nova-companion.apk`  | `879E28B7D9BA4EF2FB3E758D3CE89CD0B266093869D2C09D719A02FFAEF89CD5` |
| `xtest-nova-validation.apk` | `6F7EF00E6E4CBF734C34C57FA198EA89EF2ABB5D13EFC4029F0E0DAA399DDB7A` |

M5.7 不执行新的设备状态变化；资格结论引用 M2.4–M5.6 已完成的自动和 Android 13/16 真机证据。验证夹具不进入生产发布清单。
