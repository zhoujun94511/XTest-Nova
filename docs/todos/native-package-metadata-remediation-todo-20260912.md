# APK 解析依赖清退计划与 Todo（2026-09-12）

## 目标

移除多年未维护的 `github.com/shogo82148/androidbinary`，由 XTest Nova 使用 Android
系统 `PackageManager` 实现当前真正需要的 Activity 清单与应用图标能力。不得新增用户安装项，
不得继续在 Agent 内解析不可信 APK 的二进制 Manifest 或 `resources.arsc`。

## Todo

- [x] 审计依赖使用面：仅 Runner Activity 覆盖率与 `/packages/{pkg}/icon` 两处。
- [x] 建立 `agent/internal/pkgmeta` 独立组件，统一包名校验、10 秒超时、1 MiB 响应上限、Activity 格式校验与 JPEG 签名校验。
- [x] 在 Companion 建立独立 `PackageMetadataProvider`，使用 Android `PackageManager` 获取完整 Activity 清单并渲染 192×192 JPEG 图标。
- [x] Provider 增加 `android.permission.DUMP` 读取权限和 shell/root UID 二次校验，只读管道禁止写操作。
- [x] Runner 覆盖率切换到 `android-package-manager` 口径；Provider 不可用时保留 `dumpsys package + observed` 降级，不阻断产物收尾。
- [x] 应用图标接口切换到自有元数据组件，保持 `image/jpeg` HTTP 合同。
- [x] 删除 `apksafe` 临时防崩层、`androidbinary` Go 模块声明、校验和与 vendor 源码。
- [x] 发布门禁增加依赖回归检查，禁止模块元数据或 vendor 目录重新引入该库。
- [x] Agent 升级为 `xtest-nova-0.23.2-m6.0-self-contained`；Companion 升级为 `30715 / 3.7.15-native-package-metadata`。
- [x] 全量 Go 单测、离线 vendor 测试、签名构建与发布门禁通过。
- [x] Android 16 最终构建真机验证：Settings 图标 6,923 字节有效 JPEG；Activity 594；覆盖率口径 `android-package-manager`；诊断产物完整；Agent 健康。
- [x] Android 13 最终构建真机验证：Settings 图标 6,046 字节有效 JPEG；Activity 518；覆盖率口径 `android-package-manager`；诊断产物完整；Agent 健康。
- [x] 最终二进制审计：源码引用 0、vendor 目录不存在、ARM64 二进制模块信息不含 `androidbinary`。

## 架构结果

正式交付仍是每个 CPU 架构一个 Agent 文件。Companion 本来就是完整运行时的必需组件，
本次只在其内部增加只读系统元数据桥，不增加 APK、JAR、端口或人工安装步骤。

Runner 收尾不再接触 APK 字节：Activity 成功时使用系统权威清单；系统桥异常时回退到
已有包解析器输出与实际观察集合。图标读取失败只影响单次图标请求，不影响 Agent 或测试任务。
