# M5.3 验证记录

验证日期：2026-09-03

版本：

- Agent：`xtest-nova-0.15.0-m5.3`
- Companion：`3.2.0-m5.3`（versionCode 30200）
- 验证夹具：`com.xtest.nova.fixture` 1.0（versionCode 1）

## 安全边界

状态专项只操作唯一测试包，不对设备现有业务应用执行安装、停止或卸载。发现同名夹具或已有 Nova Agent 时拒绝覆盖。脚本在异常路径恢复原输入法、卸载夹具、恢复原前台、删除暂存 APK，并清理 Agent 和端口转发。

## 双机结果

Android 16（Xiaomi 2510DPC44G，SDK 36）：

- 通过 `/installLocalApk` 安装签名夹具；
- 包详情与 versionCode 校验通过；
- `/session` 启动 `.ValidationActivity`，前台包确认正确；
- `/v1/apps` 卸载后确认包不存在；
- 前台恢复为 `com.mi.android.globallauncher`；
- 设备没有批准列表内的测试 IME，因此未改变输入法；
- 既有外部 UiAutomator 服务在 Nova POST/DELETE 后仍保持运行，所有权保护通过。

Android 13（Samsung SM-G9860，SDK 33）：

- 夹具安装、详情、会话启动、前台确认和卸载全部通过；
- 前台恢复为 `com.sec.android.app.launcher`；
- 输入法从 `io.appium.settings/.UnicodeIME` 切换后恢复为 `com.samsung.android.honeyboard/.service.HoneyBoardService`；
- 设备未安装 UiAutomator server 包，Nova 返回明确错误并保持未运行，没有伪装启动成功。

最终确认两台设备都没有残留夹具；默认输入法分别为原 Google LatinIME 和 Samsung HoneyBoard。

## 构建与产物

- Go 静态检查、全部单元测试和发布门禁通过；
- 验证夹具使用同一恢复证书签名，不包含网络、存储或输入权限；
- 夹具只用于验收，不进入生产发布清单。

| 产物                          | SHA-256                                                            |
|-----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`    | `EC3197B26A394D04A321CAD5FFA0A1B703F98988312B2A8700345BDFD646CF83` |
| `xtest-nova-agent-armv7`    | `77BEA9289310D52DF5F014C938663037A958AAE597D6D3E84DF07F040B8A5351` |
| `xtest-nova-runner.jar`     | `EE617E27FF5128FB181C02B0707F0EECC60716895F4D00A5F1B76DDC342C5AFB` |
| `xtest-nova-companion.apk`  | `73C7F4B14EA03EB85E59C9CC8809EF675D87DDA004B106B61001250DBA48E97D` |
| `xtest-nova-validation.apk` | `BA3A4CFE5E74280D347ABBDFBFD3E1FE2C183F23E4AA73149721ADFE32BAF98D` |

## 剩余边界

公网异步安装、multipart 上传安装、Android 16 输入法切换、UiAutomator 自有服务启停和空闲回收仍未获得双机真机证据。录屏、触控及缺少相应设备/二进制的能力继续保持 `partial`。
