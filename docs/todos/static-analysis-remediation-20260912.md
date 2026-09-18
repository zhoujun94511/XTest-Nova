# Go 静态检查整改记录（2026-09-12）

## 结论

本轮列出的 IDE 告警已全部按源码语义处理，没有增加规则抑制。涉及的生产代码、测试代码和重复逻辑均已重新格式化并通过全量测试。

## 处理项

| 类别 | 处理结果 |
|---|---|
| 未处理的 Close/Serve/Fprintf 错误 | HTTP 响应、监听器、文件、压缩输入、日志读取和 Monitor 连接均显式关闭并传播错误；测试服务使用受控清理与 Serve 结果回收 |
| 潜在资源泄漏 | Diagnostics Soak 在启动协程前显式转移文件所有权；协程退出时关闭文件、删除活动标记并把清理失败写回状态 |
| 包装错误类型断言 | Monitor 改用 `errors.As`，可识别包装后的 `net.Error` |
| error 与结果使用顺序 | Runner 事件日志和相关测试先处理 error，再读取结果；运行时 APK 安装变更改为显式可空结果，失败时仍保留回滚信息 |
| 重复代码 | multipart 接收/清理、安装任务启动和 WebSocket 断连监听分别收敛为共享函数 |
| 无用代码 | 移除未使用的探索优先级函数和 onboarding 变量；默认运行时路径只保留单一实际定义 |
| 命名与风格 | 避免 `copy`、`device` 遮蔽 builtin 或包名；错误字符串统一小写开头；空切片使用零值声明 |
| 正则与转换 | 删除冗余转义，将非空白匹配改为 `\S`，删除截图灰度值的冗余类型转换 |
| 测试健壮性 | 未使用形参改为 `_`，多返回值测试拆分 error 与内容断言，避免错误路径读取不可信结果 |
| Multipart 生命周期 | 上传文件、临时表单统一封装为一个可关闭资源；打开失败立即清理，两个上传入口在所有返回路径均执行 Close/RemoveAll |
| Web 资源解析 | HTML 改用带根基准的同目录资源引用；增加明确的根资源路由并保留原 `/static` 路由，补齐截图占位资源和 `img src` |
| JavaScript 检查 | 动态 JSON 字段使用显式键访问，启动 Promise 用 `void` 标明有意异步执行；两个脚本均通过语法检查 |
| 失败状态验证 | Bootstrap 安装失败后从 Manager 查询回滚后的权威状态，不再读取与 error 同时返回的结果值 |

## 验证

- `gofmt`：通过。
- `go vet ./...`：通过。
- `go test ./...`：通过。
- 全量 `go test -race ./...` 门禁：通过。
- 签名构建：通过，验证夹具仍排除在正式运行时外。
- 77/77 路由、77/77 语义合同及发布门禁：通过。
- Android 16（`83fc400c`）与 Android 13（`R5CN30EQKNM`）均覆盖部署最新二进制并通过 Gallery 会话、Nova 控件树、2 秒 Runner 收尾及悬浮窗存活冒烟；两次运行均 `completed`、退出码 0、诊断完整且无 Crash/ANR/Native Crash。
- 第二批整改后，两台设备均重新覆盖部署最新 ARM64 Agent；`/`、四个同目录 CSS/JS、占位 SVG 及兼容 `/static` 资源均返回 HTTP 200，健康状态为 `ok`，13 个运行组件就绪。

真机产物：

- `/sdcard/xtest-nova/com.miui.gallery/Monkey/20260912_145410`
- `/sdcard/xtest-nova/com.sec.android.gallery3d/Monkey/20260912_145423`

最终发布版本仍为 `xtest-nova-0.26.0-m6.3-auto-popup`；本轮只修正实现质量，不改变外部 API 合同。

- ARM64 SHA-256：`62A471FCF1DBEFCC772588D43744A51B856D50AE1ACB3E23157F540096D9C3DD`
- ARMv7 SHA-256：`D374466D84CC26504EEE8042010CDE6F79BA72458588AB5C90A6D881C5B55E5A`
