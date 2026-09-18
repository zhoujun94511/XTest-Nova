<!--
GENERATED REFERENCE BASELINE. DO NOT EDIT BY HAND.
Source: XTest-Nexus/docs/reference/TARGET_HTTP_CONTRACT.md
Imported: 2026-09-16
Source version (SHA-256): 2F9F7AFB2597FA05279F2B6A8EA336DB4F049F3939F825C749E2A34AA464D847
Update this file only by re-importing a reviewed upstream contract snapshot.
-->

# Nexus HTTP route contract baseline

This repository-local snapshot is the immutable input for Nova contract checks.
The source registration addresses are retained for traceability.

| # | Method | Route | Handler | Registration site |
|---:|---|---|---|---|
| 1 | `ANY` | `/version` | `main.handleVersion` | `0xaf6ca4` |
| 2 | `POST` | `/newCommandTimeout` | `main.handleSetCommandTimeout` | `0xaf6cd4` |
| 3 | `ANY` | `/dump/hierarchy` | `main.handleHierarchyDump` | `0xaf6d54` |
| 4 | `ANY` | `/dump/hierarchyWithScreenshot` | `main.handleHierarchyDumpWithScreenshot` | `0xaf6d88` |
| 5 | `ANY` | `/proc/list` | `main.handleListProc` | `0xaf6dbc` |
| 6 | `ANY` | `/proc/{pkgname}/meminfo` | `main.handlePkgMemInfo` | `0xaf6df0` |
| 7 | `ANY` | `/proc/{pkgname}/meminfo/all` | `main.handlePkgAllMemInfo` | `0xaf6e24` |
| 8 | `ANY` | `/proc/{pkgname}/cpuinfo` | `main.handlePkgCpuInfo` | `0xaf6e58` |
| 9 | `ANY` | `/proc/{pkgname}/perf` | `main.handlePkgPerf` | `0xaf6e8c` |
| 10 | `ANY` | `/webviews` | `main.handleWebViews` | `0xaf6ec0` |
| 11 | `ANY` | `/webviews/{pkgname}` | `main.handlePkgWebView` | `0xaf6ef4` |
| 12 | `ANY` | `/pidof/{pkgname}` | `main.hanldPkgPID` | `0xaf6f28` |
| 13 | `POST` | `/session/{pkgname}` | `main.handlePkgSession` | `0xaf6f58` |
| 14 | `GET, POST` | `/shell` | `main.handleShell` | `0xaf6fd4` |
| 15 | `GET, POST` | `/shell/background` | `main.handleShellBackground` | `0xaf7064` |
| 16 | `ANY` | `/stop` | `main.handleStop` | `0xaf70f8` |
| 17 | `GET, POST, DELETE` | `/services/{name}` | `main.handleServiceByName` | `0xaf7128` |
| 18 | `POST` | `/uiautomator` | `main.handleStartUiAutomator` | `0xaf71c8` |
| 19 | `DELETE` | `/uiautomator` | `main.handleStopUiAutomator` | `0xaf7244` |
| 20 | `GET` | `/uiautomator` | `main.handleCheckUiAutomator` | `0xaf72c0` |
| 21 | `ANY` | `/raw/{filepath:.*}` | `main.handleRawFilePath` | `0xaf7340` |
| 22 | `ANY` | `/finfo/{lpath:.*}` | `main.handleFilePathInfo` | `0xaf7374` |
| 23 | `ANY` | `/info` | `main.handleDeviceInfo` | `0xaf73a8` |
| 24 | `ANY` | `/upload/{target:.*}` | `main.handleFileUpload` | `0xaf73dc` |
| 25 | `ANY` | `/installLocalApk/{apkfilename}` | `main.handleInstallLocalApk` | `0xaf7410` |
| 26 | `ANY` | `/installAgentApk` | `main.handleInstallUtestAgentApk` | `0xaf7444` |
| 27 | `ANY` | `/installApk` | `main.handleInstallApk` | `0xaf7478` |
| 28 | `POST` | `/download` | `main.handleDownload` | `0xaf74a8` |
| 29 | `ANY` | `/download/{key}` | `main.handleQueryDownload` | `0xaf7528` |
| 30 | `POST` | `/packages` | `main.handleQueryPkgByPost` | `0xaf7558` |
| 31 | `GET` | `/packages` | `main.handleQueryPkgByGet` | `0xaf75d4` |
| 32 | `ANY` | `/packages/{id}` | `main.handleQueryPkgById` | `0xaf7654` |
| 33 | `ANY` | `/packages/{pkgname}/info` | `main.handlePkgInfo` | `0xaf7688` |
| 34 | `ANY` | `/packages/{pkgname}/icon` | `main.handlePckIcon` | `0xaf76bc` |
| 35 | `POST` | `/install` | `main.handleInstall` | `0xaf76ec` |
| 36 | `GET` | `/install/{id}` | `main.handleQueryInstallById` | `0xaf7768` |
| 37 | `DELETE` | `/install/{id}` | `main.handleRemoveInstallById` | `0xaf77e4` |
| 38 | `PUT` | `/minitouch` | `main.handleFixMimiTouch` | `0xaf7860` |
| 39 | `DELETE` | `/minitouch` | `main.handleStopMinitouch` | `0xaf78dc` |
| 40 | `GET` | `/minitouch` | `unknown` | `0xaf7964` |
| 41 | `ANY` | `/touchreader` | `main.handleTouchReader` | `0xaf79dc` |
| 42 | `ANY` | `/appevent/info` | `main.handleAppEventInfo` | `0xaf7a10` |
| 43 | `ANY` | `/appeventmonitor` | `main.handleMonitorAppEvent` | `0xaf7a44` |
| 44 | `PUT` | `/monitor` | `main.handleMonitor` | `0xaf7a78` |
| 45 | `GET` | `/minicap/broadcast` | `unknown` | `0xaf7d24` |
| 46 | `GET` | `/minicap` | `unknown` | `0xaf7d9c` |
| 47 | `ANY` | `/scrcpy/{type}/{definition}` | `unknown` | `0xaf7e20` |
| 48 | `POST` | `/screenrecord` | `main.handleStartScreenRecord` | `0xaf7e48` |
| 49 | `PUT` | `/screenrecord` | `main.handleScreenRecord` | `0xaf7ec4` |
| 50 | `ANY` | `/term` | `main.handleTerm` | `0xaf7f44` |
| 51 | `ANY` | `/screenshot` | `main.handleQueryScreenshot` | `0xaf7f78` |
| 52 | `ANY` | `/u2packages` | `0xb731a8` | `0xaf7fb4` |
| 53 | `ANY` | `/imeStatus` | `0xb731a8` | `0xaf7ff4` |
| 54 | `ANY` | `/setIme` | `0xb731a8` | `0xaf8034` |
| 55 | `ANY` | `/network/info` | `0xb731a8` | `0xaf8074` |
| 56 | `ANY` | `/disk/info` | `0xb731a8` | `0xaf80b4` |
| 57 | `POST, DELETE` | `/popupBoxAssistant` | `0xb731a8` | `0xaf80f4` |
| 58 | `POST` | `/pushConfig` | `0xb731a8` | `0xaf8194` |
| 59 | `GET` | `/pullConfig` | `0xb731a8` | `0xaf8220` |
| 60 | `ANY` | `/assets/{(.*)}` | `unknown` | `0xaf8338` |
| 61 | `ANY` | `/screenshot/0` | `main.handleScreenshot` | `0xaf8368` |
| 62 | `ANY` | `/wlan/ip` | `main.handleIP` | `0xaf839c` |
| 63 | `ANY` | `/device/memory` | `main.handleDeviceMemoryInfo` | `0xaf83d0` |
| 64 | `GET` | `/foregroundPkg` | `main.handleForegroundPkgInfo` | `0xaf8400` |
| 65 | `GET` | `/wakeupScreen` | `main.handleWakeupScreen` | `0xaf847c` |
| 66 | `ANY` | `/jsonrpc/0` | `0xb731a8` | `0xaf8504` |
| 67 | `ANY` | `/ping` | `0xb731a8` | `0xaf8544` |
| 68 | `ANY` | `/static/js/{(.*)}` | `0xb731a8` | `0xaf8734` |
| 69 | `ANY` | `/static/css/{(.*)}` | `0xb731a8` | `0xaf87d0` |
| 70 | `ANY` | `/static/media/{(.*)}` | `0xb731a8` | `0xaf886c` |
| 71 | `ANY` | `/{(.*)}` | `0xb731a8` | `0xaf8908` |
| 72 | `ANY` | `/` | `0xb731a8` | `0xaf89a4` |
