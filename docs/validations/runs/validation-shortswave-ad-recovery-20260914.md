# ShortsWave 全屏广告恢复真机验证（2026-09-14）

目标包：`com.shorts.wave.drama`。设备为 Android 16 `83fc400c` 与 Android 15 `R3CY80B2G4W`，均使用自包含 ARM64 Agent、Nova Provider 和签名 Companion `3.7.17-automation-overlay-lifecycle`。

## 结论

- 两台设备均真实进入 `com.google.android.gms.ads.AdActivity`，识别到 Google 全屏可试玩/开屏广告的非 clickable `close-button`，没有点击 `Play Now`、安装或继续观看素材。
- Android 16 最终冷启动闭环先对真实通知权限弹窗执行 1 次 `permission/allow`，随后识别 1 次 Google 全屏广告、执行 3 次受冷却和遭遇级上限约束的关闭动作、恢复 1 次，广告累计等待 15685ms；继续执行到 30 个动作、22 个业务状态并以 `state_exhausted` 正常结束。
- Android 15 的首次闭环识别 1 次广告、恢复 1 次，之后进入 ShortsWave 主页面；最终生命周期回归又识别 3 次广告并恢复 2 次，第 3 次由测试端主动停止，没有伪记为恢复。
- 自动化运行时两台设备的 Companion WindowManager 记录均为 0，未再抢占广告右上角关闭位置；结束后 `overlaySuppressed=false` 并恢复 315×619/336×660 的主菜单，不再显示 28dp 橙色大球。
- Android 16 清数据冷启动暴露的权限弹窗首帧空节点问题已改为 5 秒有界等待，并在最终包中实际完成授权。充值入口 `Top Up/top_up` 与 `cl_stripe` 已加入不可覆盖的金融动作黑名单；最终 30 个动作中金融入口命中数为 0。

## 证据目录

- `tests/reports/shortswave-ad-validation-20260914/v3-overlay-suppression`：两机首次广告关闭、业务页面恢复、步骤和回执。
- `tests/reports/shortswave-ad-validation-20260914/v4-final-lifecycle`：运行中零悬浮窗口、结束后恢复主菜单及 Companion 3.7.17 版本证据。
- `tests/reports/shortswave-ad-validation-20260914/v5-android16-final-ad`：Android 16 冷启动权限首帧缺陷现场和原始层级。
- `tests/reports/shortswave-ad-validation-20260914/v6-android16-permission-ad`：Android 16 最终广告恢复、19 个动作、12 个业务状态、步骤与幂等回执。
- `tests/reports/shortswave-ad-validation-20260914/v7-final-package`：最终发布 Agent 的权限、广告、金融入口拦截、悬浮窗生命周期和正常耗尽闭环。

构建执行全部 Go 测试、Runner 自检、Android APK 构建与签名校验；`scripts/test-release.ps1` 通过。发布清单仍为六项交付：双架构 Agent、四个内嵌运行组件及第三方声明/SBOM 门禁。
