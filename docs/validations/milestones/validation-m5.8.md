# M5.8 首次提交审查

验证日期：2026-09-03

- Agent：`xtest-nova-0.20.0-m5.8`
- Companion：`3.7.0-m5.8`（versionCode 30700）

## 审查结论

- Nova 源码采用 MIT License，根目录包含 `LICENSE`；
- 根 `NOTICE.md` 说明 Nova 权利信息和第三方 scrcpy 资源；
- scrcpy 4.0 的 Apache-2.0 完整许可、来源、版本和固定哈希随资源保存；
- [`CHANGELOG.md`](../../../CHANGELOG.md) 记录首次交付范围与资格限制；
- 构建目录、`dist`、报告、APK、DEX、普通 JAR、密钥和私有凭据均被排除；
- 唯一允许的大型提交候选是官方 `scrcpy-server-v4.0.jar`，SHA-256 固定为 `84924BD564A1EB6089C872C7521F968058977F91F5FF02514A8C74AFF3210F3A`；
- 未发现私钥、口令、令牌、`Co-authored-by` 或 Codex 作者/协作者信息；
- `scripts/test-first-commit.ps1` 审核候选文件、必要文档、敏感模式、大文件和远程地址；
- `scripts/test-release.ps1 -RequireCleanGit` 保留为提交后的干净工作区门禁。

## 最终生产产物

| 产物                         | SHA-256                                                            |
|----------------------------|--------------------------------------------------------------------|
| `xtest-nova-agent-arm64`   | `4187C866C6F2DC9B6C0B43A718B1DA498FA04A371782B89E57FE8F0AC70F37AC` |
| `xtest-nova-agent-armv7`   | `127597DA84CAEF1B2A3B9673DAB4EF87331D45AD6FC501300693C8D6FE7B516B` |
| `xtest-nova-runner.jar`    | `33F93D7F10CCF9BA3F936CE5E1F457DBE99F1DDD7D30C99C4945124CEE599E8D` |
| `xtest-nova-companion.apk` | `29BC9D4F0C3A63995B448F1E5BE934D4C1272595140CB40C5C42908CE696EA2A` |

发布门禁、74/77 合同门禁和首次提交候选门禁全部通过。当前仍未创建提交或上传远程仓库；该动作保留给 M5.9，并要求用户明确授权。
