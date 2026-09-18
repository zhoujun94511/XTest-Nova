# Gallery 黄金基线

当前可执行场景使用系统 Gallery，不再使用 Foloy。Gallery 会随设备厂商和系统版本变化，因此黄金报告必须在同一设备、同一 Gallery 版本和同一安全场景上实际采集，不能把历史 Foloy 数字改名复用。

建议文件名：

- `nexus-android16-miui-gallery-60s.json`
- `nexus-android13-samsung-gallery-60s.json`

取得真实参考报告后，再将其传给
`tests/e2e/validate-exploration-golden.ps1`。旧 Foloy 运行事实仅保留在历史
验证文档中，不再属于 `tests/scenarios/` 下的可执行测试对象或黄金基线。
