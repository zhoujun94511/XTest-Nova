# GoLand 静态检查整改记录（2026-09-07）

## 已整改

- 使用官方 `gofmt` 格式化全部 Go 源码，并将 `device/service.go`、`httpapi/api.go`、
  `httpapi/api_test.go` 的标准库与项目导入显式分组。
- 删除未使用的 `artifacts.ScreenRecords`、`contract.SemanticSummary` 和 `runner.New`。
- 对 HTTP Body、文件、Socket、WebSocket、Multipart 临时文件和下载临时文件的关闭/清理结果
  进行显式处理；性能 CSV 文件关闭失败会写入最终运行错误。
- `evaluation.delta` 使用直接 nil 判断后解引用；探索分析失败时不再继续写入结果。
- 包装错误判断统一使用 `errors.Is` / `errors.As`。
- `exploration.Config` 与 `recordreplay.Case` 的接收器类型统一；Case 摘要计算通过副本归一化，
  不修改调用方对象。
- 录制与回放启动复用同一个输入互斥检查器，消除重复代码且保持原锁定范围。
- 修正 Shell 局部变量与 `command` 包重名、错误文本、取消响应语法、冗余转换及所列空切片写法。

## 不修改生产代码的 IDE 提示

- `AndroidManifest.xml` 中的 `http://schemas.android.com/apk/res/android` 是 Android 官方 XML
  命名空间。GoLand 的“URI 未注册”属于 IDE XML Catalog/Android 支持配置，不应改写 URI 或添加 DTD。
- `minitouch` 是 OpenATX 组件和兼容接口的正式名称，不属于拼写错误。
- 保留少量显式创建的非 nil 空切片，用于兼容 HTTP JSON 返回 `[]`，不能机械替换成 nil 后返回 `null`。

## 验证

- 全量 `gofmt`、`go vet ./...`、`go test ./...` 通过。
- Go 竞态检测通过。
- 签名 Agent、Runner、Companion 和验证 APK 构建通过。
- 77/77 HTTP 合同、首次提交内容检查与发布门禁通过。
