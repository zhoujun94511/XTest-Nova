# 游戏 Demo 异常采集真机验证 TODO（2026-09-11）

> 历史验证记录：以下事项针对合并前的独立游戏包。场景现已迁入 `com.xtest.nova.fixture`，后续以 [`unified-validation-fixture-todo-20260911.md`](unified-validation-fixture-todo-20260911.md) 为准。

- [x] T1. 在游戏内增加可由通用探索器发现的“异常实验室”入口。
- [x] T2. 增加 Java Crash、主线程 ANR、Native Crash 和进程信号退出四个受控场景及稳定控件标识。
- [x] T3. 提供场景矩阵，明确预期分类和核心证据文件。
- [x] T4. 构建、签名并安装游戏 Demo 1.3（versionCode 4）。
- [x] T5. 真机从游戏首页盲探到异常实验室，实际点击 Java Crash、Native Crash、ANR 和信号退出控件。
- [x] T6. 真机独立验证 Java Crash，保存完整 Runner 产物。
- [x] T7. 真机独立验证主线程 ANR，保存完整 Runner 产物。
- [x] T8. 真机独立验证进程信号退出，保存完整 Runner 产物。
- [x] T9. 汇总设备、远端会话目录、本地证据目录、分类计数和异常日志摘录。
- [x] T10. 补充 Native Crash (SIGSEGV) 并在真机上验证致命信号采集；同时修复无 shell tombstone 权限时 `native_crash.txt` 只有文件头的缺口。
