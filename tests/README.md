# XTest Nova 测试目录

本目录集中保存跨组件、真机和端到端测试资产。Go 单元测试仍以 `_test.go`
形式与被测包同目录存放，这是 Go 工具链访问包内实现和执行 `go test ./...`
所要求的标准布局。

## 目录

- [`e2e/`](e2e/)：真机、兼容性、长稳、录制回放和探索验证脚本。
- [`e2e/lib/`](e2e/lib/)：真机测试共享的设备安全与清理函数。
- [`scenarios/`](scenarios/)：当前可执行的探索、游戏和验证夹具场景。
- [`scenarios/baselines/`](scenarios/baselines/)：黄金基线命名与采集约定。
- [`archive/`](archive/)：仅用于历史结论追溯的旧场景和基线。
- `reports/`：本地测试输出，受 `.gitignore` 排除，不进入版本库。

## 执行方式

所有命令均从仓库根目录执行。例如：

```powershell
.\tests\e2e\validate-runner.ps1 -Serial <设备序列号>
.\tests\e2e\validate-device-matrix.ps1 -Serial @('<设备1>','<设备2>')
.\tests\e2e\validate-exploration-golden.ps1 -Baseline <基线报告> -Candidate <候选报告>
```

仓库级源码、竞态和发布门禁仍位于 [`scripts/`](../scripts/README.md)：

```powershell
go test ./agent/...
.\scripts\test-race.ps1
.\scripts\test-first-commit.ps1
```

真机脚本默认从根目录 `dist/` 读取已构建产物，并将输出写入
`tests/reports/`。测试退出时必须清理自身创建的设备文件、端口转发、进程和
临时应用，不得覆盖用户已有组件。
