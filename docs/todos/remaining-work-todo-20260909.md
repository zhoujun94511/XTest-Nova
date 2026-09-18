# XTest Nova 剩余工作 Todo

日期：2026-09-09

状态：`[x]` 已完成；`[~]` 正在推进；`[ ]` 当前环境可执行；`[!]` 需要新设备、外部参考端、发布周期或用户授权。

## P0 状态与证据对齐

- [x] P0.1 将 Foloy 三机 Compose、系统弹窗和交易保护证据同步到总 Todo、Provider 计划与审计整改文档。
- [x] P0.2 全量 Go 测试、`go vet`、Runner 自检和 77 项合同通过。
- [x] P0.3 首次提交候选门禁通过，共 236 个候选文件；实际提交与远程上传仍等待用户明确授权。

## P0A 对齐审计整改（2026-09-09）

- [x] P0A.1 后台 Shell 建立精确进程所有权，`/stop` 与进程退出均终止并等待自有后台命令，补充幂等停止测试。
- [x] P0A.2 JSON-RPC 后端响应按 16 MiB+1 探测，超限明确返回 502，不再以成功状态静默截断。
- [x] P0A.3 发布门禁要求清单精确包含六个唯一制品，并校验源码版本、APK 元数据、包名、签名一致性、大小及哈希。
- [x] P0A.4 77 项声明合同增加真实 `http.ServeMux` 注册解析测试，防止声明存在但处理器漏注册。
- [x] P0A.5 统一 Monkey DFS、Activity 分母、Foloy/Nexus A/B 和统一停止的现行文档口径，并补充变更记录。
- [!] P0A.6 六制品源码与 unsigned 产物已重建；当前 Companion 证书为 `E49A7C…A46DA`，UiAutomator host/test 证书为 `8BB545…B7A65`，生成器按安全门禁拒绝混签清单。需使用同一维护者正式密钥重签三个 APK 后重新生成清单；真机与发布回归仍归 P2–P4。

## P1 Provider 资格自动化

- [x] P1.1 已建立 `tests/e2e/validate-uiautomator-longrun.ps1`：固定设备、目标应用、时长、采样间隔，输出成功/失败、延迟分位数、节点范围及全程/预热后 PSS 趋势。
- [x] P1.2 脚本具备唯一 run/session 标识、精确进程与安装所有权、失败收敛、前台恢复和设备隔离清理；拒绝覆盖非本次运行持有的 Agent/UIAutomator 实例。
- [x] P1.3 Android 13 短周期自检通过：3 个探索会话、30 步、10 个采样、56 次 Provider 捕获、零失败；Agent/测试包/端口转发全部清理，Foloy 保留。证据：`tests/reports/provider-longrun-smoke-a13-v3.json`。

## P2 当前设备矩阵补齐

- [ ] P2.1 Android 13 Foloy 真实分屏：system/Nova 隔离影子、业务节点匹配、50 次直连和恢复全屏。
- [ ] P2.2 Android 16 Foloy 真实分屏：system/Nova 隔离影子、业务节点匹配、50 次直连和恢复全屏。

> 当前设备说明：Android 16 可在线执行只读检查，但安装 Nova UIAutomator 测试包时由设备返回 `INSTALL_FAILED_USER_RESTRICTED`；在设备侧允许 USB 安装前，P2.2 的 Provider 实测不能完成。
- [ ] P2.3 Android 13/16 Provider 崩溃回退与恢复复验。
- [ ] P2.4 更新 U1/U7/U9 的设备证据和差异解释。

## P3 两小时长稳资格

- [ ] P3.1 Android 13 与 Android 16 并行执行两小时 Provider/探索长稳。
- [ ] P3.2 验证零死锁、零不可恢复超时、节点不退化、P95 门槛及 PSS 无持续线性增长。
- [ ] P3.3 执行结束后验证 Provider 降级恢复、Agent 停止和所有临时组件清理。
- [ ] P3.4 将原始报告和摘要写入 `tests/reports/` 与 Provider 资格记录。

## P4 发布回归

- [ ] P4.1 Monkey、AutoPopup、录制回放、性能、流式能力及 77 项 HTTP 合同完整回归。
- [ ] P4.2 完整构建、签名、发布清单、敏感信息、竞态和首次提交候选门禁。
- [ ] P4.3 根据所有量化证据决定 U8：保持 system 默认或切换为 `nova → system-dump → legacy-9008`。

## P5 仍可继续的软件工作

- [ ] P5.1 R4.1 Runner 在更多获准真实应用上的节点探索资格。
- [ ] P5.2 R5.2 Web 工作台补齐日志浏览、远控播放器和压力任务。
- [ ] P5.3 R5.5 首次远端 CI 运行及失败收敛。
- [ ] P5.4 清理文档中已被 Foloy Compose 真机证据替代的过期限制，但保留 SurfaceView/Canvas 无语义边界。

## 外部阻断项

- [!] E1 Android 14 真机。
- [!] E2 ARMv7 真机。
- [!] E3 TouchReader Protocol-A 输入设备样本。
- [!] E4 可信 minitouch 对应 ABI 二进制与完整触控资格。
- [!] E5 Mali/其他 GPU、商业游戏及 24 小时稳定性资格。
- [!] E6 可安全运行的 Nexus 参考端，用于 R4.3/R4.4 双实现黄金基线。
- [!] E7 首次 Git 提交与远程上传，需要用户明确授权；当前仓库文件尚未进入首次提交基线。

## U8 切主硬门槛

- API 33+ 热态采集 P95 ≤ 500 ms；API 32 大型虚拟树按版本化门槛执行。
- 相对 system dump 中位耗时改善至少 50%。
- 10,000 次采集无死锁、无进程泄漏。
- 两小时遍历无持续内存增长。
- Provider 崩溃后可降级并恢复。
- Android 12/13/15/16 可访问业务节点不比 system dump 退化。
- Monkey、Popup、录制回放及 77 项合同全部通过。
