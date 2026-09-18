# M2.1 验证记录

验证日期：2026-09-03。

## 自动化检查

- `go test ./agent/...`：通过；
- `go vet ./agent/...`：通过；
- ARM64 Agent：构建通过；
- ARMv7 Agent：构建通过；
- Runner Java、D8 Dex/JAR：构建通过；
- Companion Java、D8、AAPT2、zipalign、APK 签名：通过。

Nexus 路由表解析得到 77 个“方法+路径”合同，对应文档中的 72 个注册项。Nova M2.1 精确覆盖 18 个方法合同，剩余 59 个由检查工具明确报告为待迁移。

## Android 16 真机

设备序列号：`83fc400c`；SDK：36；ABI：`arm64-v8a`。

Nova 使用设备端 17912/18912 独立测试端口，不覆盖 Nexus 服务：

| 检查                   | 结果                        |
|----------------------|---------------------------|
| Agent 版本             | `xtest-nova-0.2.0-m2.1`   |
| 健康检查                 | 通过                        |
| 第三方应用列表              | 62 个                      |
| 旧 Popup 包信息读取        | 通过                        |
| PNG 截图               | 17,208 字节                 |
| 窗口层级                 | 486 字符                    |
| 文件上传/查询/下载           | 内容一致，临时文件已删除              |
| UiAutomator 缺少服务 APK | 正确拒绝并返回 HTTP 500，没有虚报启动成功 |
| 8912 健康检查            | 通过                        |

为避免未经确认覆盖设备上正在使用的 Nexus Popup，本轮只验证了 Nova Companion 的静态构建、包信息和签名，没有安装到真机。

## 交付校验

| 产物                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `4B04BC9B5F302BF3F1B1E7A2C436576EF1EE734405AE856ECF6D8BD2E182AAAD` |
| `xtest-nova-agent-armv7`   | `39920297BDB0F9F8328B0494111092113D4E3A6D9ED45D2FC5256154ECA158D9` |
| `xtest-nova-runner.jar`    | `25E301D145AF86CAC3C00A574190A94FE5C5AFD92BC1A8C3C53ABEA67C9DE8B8` |
| `xtest-nova-companion.apk` | `53B98FCAE69019BA447B0AE34F1B6BB6573CE008119F98BAD5E40AC5EE6A8CAC` |

Companion 包名为 `com.openatx.xtest.popup`，`versionCode=20001`，`versionName=2.0.0-m2.1`。签名证书 SHA-256 为：

```text
e49a7c6fb2ee0dbc4e6d3b6790d9dcf981d80f6b76628eb84755e7e1da3a46da
```

该证书与 Nexus 当前维护 APK 一致，可以执行同签名升级。
