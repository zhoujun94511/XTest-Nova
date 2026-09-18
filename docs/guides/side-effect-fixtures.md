# 副作用 HTTP 隔离夹具

运行入口：

```powershell
.\tests\e2e\validate-side-effect-fixtures.ps1 -Serial <设备序列号> -AgentPort <本机转发端口>
```

夹具只在 Agent 健康、没有 Runner、探索、录制、回放、录屏或性能会话时执行。测试目标默认是
`com.xtest.nova.fixture`，不应替换为保存真实业务数据的应用。

| 分组 | 接口/动作 | 隔离与恢复 |
|---|---|---|
| 文件 | `/upload`、`/raw`、`/finfo` | GUID 文件名；仅允许清理夹具根；删除后验证不存在 |
| 配置 | `/pushConfig`、`/pullConfig` | 保存原 JSON；使用无点击规则；最终重新读取核对 |
| 应用 | `/session/{pkg}` | 仅启动专用夹具 |
| 任务 | `/v1/performance/sessions` | 自有启动/停止；CSV 精确删除 |
| 媒体 | `/screenrecord` | 三秒短录制；验证非空 MP4 后精确删除 |
| 服务 | `/popupBoxAssistant`、`/minitouch` | 只停止夹具自有实例；缺可信二进制记资格跳过 |
| 设备 | `/wakeupScreen` | 验证幂等；原为休眠时尽力恢复休眠 |

以下能力不并入默认夹具：APK 安装/卸载由 `tests/e2e/validate-stateful.ps1` 独立执行；下载需要受控公网服务；
`/stop` 会结束被测 Agent；任意 Shell/终端默认禁止。这样可避免为了合同数量扩大设备风险。
