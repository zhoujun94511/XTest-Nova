# 单一测试夹具合并 Todo（2026-09-11）

目标：将重复维护的 `com.xtest.nova.fixture` 与 `com.xtest.nova.gamedemo` 合并为一个测试项目和一个 APK，同时保留通用探索、长列表、动态状态及四类受控异常的验证能力。

## 实施清单

- [x] U1：以 `com.xtest.nova.fixture` 作为唯一测试包，版本提升为 `3.0-unified`。
- [x] U2：迁入游戏首页、九宫格动态状态、排行榜长列表和安全拦截样例。
- [x] U3：将 Java Crash、ANR、Native SIGSEGV、进程信号退出合并到统一异常实验室。
- [x] U4：兼容既有 `mode=crash|anr` 定向调用，并为 Native/Signal 保留 `auto=true` 定向调用。
- [x] U5：统一 `validation-fixture-matrix.json`，保留 F1–F9 编号并追加 F10–F12。
- [x] U6：删除第二套 Manifest、源码、构建脚本和独立场景矩阵。
- [x] U7：从总构建与发布脚本语法门禁中移除 `game-demo` 构建入口。
- [x] U8：完成统一夹具编译、签名、APK 元数据、场景唯一性和静态引用验证；产物为 `dist/xtest-nova-validation.apk`，包名 `com.xtest.nova.fixture`、版本 `3.0-unified`。
- [x] U9：在 Android 13 真机验证统一入口、游戏正常流程及四类异常产物。

## 真机证据

- 统一入口可见“游戏探索场景”和“异常实验室”，游戏页可见动态九宫格、排行榜及测试专用异常入口。
- 正常流程实际点击“开始游戏”及当前星星格，状态推进为“命中星星｜得分=1”。
- Java Crash：`/sdcard/xtest-nexus/com.xtest.nova.fixture/Monkey/20260911_205051`，`failureType=app_crash`、`crashCount=1`。
- Native SIGSEGV：`/sdcard/xtest-nexus/com.xtest.nova.fixture/Monkey/20260911_205145`，`failureType=app_crash`、`nativeCrashCount=1`。
- 进程信号退出：`/sdcard/xtest-nexus/com.xtest.nova.fixture/Monkey/20260911_205157`，`failureType=abnormal_exit`、`abnormalExitCount=1`。
- ANR：`/sdcard/xtest-nexus/com.xtest.nova.fixture/Monkey/20260911_205312`，按既有输入分发门禁触发，`failureType=app_anr`、`anrCount=1`。
- 完整发布门禁通过：`xtest-nexus-0.22.0-m5.9-compat`；真机已卸载旧 `com.xtest.nova.gamedemo`，仅保留统一夹具 `3.0-unified`。

## 边界

- 历史验证报告及 `/sdcard/xtest-nexus/com.xtest.nova.gamedemo` 目录继续作为旧版本证据，不改写为新包名。
- 多包隔离不再由完整游戏 Demo 承担；需要时应新增最小第二包夹具，避免恢复第二套重复场景。
