# 最小化悬浮把手三机 20 分钟真机验证（2026-09-15）

## 结论

Companion 30718 的贴边最小化把手在 Android 13、15、16 三台手机上分别连续运行超过
20 分钟。三机合计向真实 48dp 触摸热区发送 373 次单击，完成 60 次
Agent/Companion/Monkey 状态检查和 30 次 UiAutomator 节点树检查；没有单击展开、节点泄漏、
Agent 重启、悬浮服务退出或 Monkey 意外启动。三台设备的长按恢复和再次最小化均通过。

## 测试前清理

测试前先停止既有 Agent，并只对项目拥有的资源执行清理：卸载
`com.openatx.xtest.popup`、Nova UiAutomator host/test 和已安装的
`com.xtest.nova.fixture`，删除已解析且通过前缀校验的
`/data/local/tmp/xtest-nova*` 文件，以及 `/sdcard/xtest-nova`、
`/sdcard/xtest-nova-screenrecords`、`/sdcard/xtest-nexus` 三个精确目录。

- Android 16 清理 8 个临时文件、3 个运行组件包和 626KB 旧产物目录；
- Android 15 清理 9 个临时文件、3 个运行组件包及 1 个验证夹具包；
- Android 13 清理 9 个临时文件和 3 个运行组件包；
- 三台清理后复查结果均为：残留临时文件 0、残留项目包 0、残留产物根目录 0。

清理完成后，三台均重新部署当前 ARM64 自包含 Agent，健康检查确认四个内嵌运行组件完整，
Companion 的已安装和期望版本均为 30718。

## 结果

| 设备 | Android | 有效时长 | 热区单击 | 状态检查 | 节点树检查 | 主菜单 | 最小化 | 结果 |
| --- | ---: | ---: | ---: | ---: | ---: | --- | --- | --- |
| 2510DPC44G (`83fc400c`) | 16 | 1203 秒 | 125 | 20 | 10 | 336×660px | 144×144px | 通过 |
| SM-S936U (`R3CY80B2G4W`) | 15 | 1202 秒 | 125 | 20 | 10 | 315×619px | 135×135px | 通过 |
| SM-G9860 (`R5CN30EQKNM`) | 13 | 1206 秒 | 123 | 20 | 10 | 315×619px | 135×135px | 通过 |

每次单击后都重新读取 Companion WindowManager 尺寸；尺寸必须保持最小化，否则立即失败。
每分钟核对 Agent PID、30718 版本一致性及 Monkey 空闲状态，每两分钟导出一次 UiAutomator
层级并确认不包含 Companion 包名或旧的“恢复 XTest Nova”入口。结束时对把手执行 900ms
长按，确认恢复原主菜单尺寸，再重新最小化。三台开始与结束前台包一致，失败列表均为空。

## 证据

结构化结果和起止截图位于 `tests/reports/minimized-overlay-20m-20260915/`。测试结束后三台保持
新版 Agent/Companion 运行、悬浮窗最小化，Monkey 保持 `running=false finalizing=false`。
