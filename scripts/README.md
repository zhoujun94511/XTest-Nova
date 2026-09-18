# 构建与发布门禁脚本

此目录保存仓库级源码、构建和发布门禁。真机及跨组件测试已统一迁入
[`tests/e2e/`](../tests/e2e/)，场景和报告约定见
[`tests/README.md`](../tests/README.md)。

## 基础与发布门禁

- `test-first-commit.ps1`：提交前的源文件、敏感内容和大文件检查。
- `test-race.ps1`：Go Agent 竞态检测。
- `test-build-release.ps1`：全部 PowerShell 语法、可复现文本和 runtime bundle
  元数据测试，不执行签名构建。
- `test-release.ps1`：构建、测试、脚本语法、发布清单和跨项目合同的综合发布门禁。
- `new-lan-token.ps1`：生成仓库外的密码学随机 LAN API 令牌文件，默认拒绝覆盖。
- `validate-artifact-size.ps1`：发布载荷大小预算检查。
- `validate-runtime-bundle.ps1`：校验现有内嵌组件的集合、哈希、APK 元数据与签名。
- `sync-runtime-bundle.ps1`：同步并校验 Agent 内嵌运行组件。

## 调用约定

```powershell
.\scripts\test-first-commit.ps1
.\scripts\test-build-release.ps1
.\scripts\test-race.ps1
.\scripts\new-lan-token.ps1
.\tests\e2e\validate-runner.ps1 -Serial <设备序列号>
```

构建产物仍写入根目录 `dist/`；本地测试报告统一写入
`tests/reports/`。

根目录 `build.ps1` 默认执行需要签名的正式构建；无签名开发构建必须显式使用
`-Development`。发布清单优先读取 `SOURCE_DATE_EPOCH`，未设置时使用当前 Git
提交时间，无提交时才使用当前 UTC 时间。清单、SBOM 副本和
`THIRD_PARTY_NOTICES.txt` 均写为无 BOM UTF-8 和 LF 换行。
