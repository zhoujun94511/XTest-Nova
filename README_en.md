<div align="center">

![XTest Nova logo](pic/logo-96.png)

# XTest Nova

**An intelligent automation tool that runs on the phone**

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](https://go.dev/)
[![Java](https://img.shields.io/badge/Java-21-ED8B00.svg)](https://openjdk.org/)
[![scrcpy](https://img.shields.io/badge/scrcpy-4.1-00C853.svg)](https://github.com/Genymobile/scrcpy)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

[中文](README.md) · [English](README_en.md)

[Features](#features) · [Architecture](#architecture) · [Quick Start](#quick-start) · [Screenshots](#screenshots) · [Documentation](#documentation)

</div>

## Overview

XTest Nova puts exploration, Monkey, record/replay, performance capture, and remote control inside a single Agent on the phone. After you push it to a device, drive it from a browser or the HTTP API. Screenshots, logs, and performance data stay on the device, grouped by target app.

One device, one Agent. Access defaults to a local ADB forward. Direct LAN access requires authentication, as documented below.

## Screenshots

On-device overlay and web console.

<div align="center">

**On-device**

<img src="pic/设备-悬浮窗界面.jpg" alt="On-device overlay" width="360" /> <img src="pic/设备-智能遍历界面.jpg" alt="Exploration settings" width="360" />

**Web console**

Overview

![Overview](pic/控制台-运行概览.png)

Apps

![Apps](pic/控制台-应用管理.png)

Automation

![Automation](pic/控制台-自动化测试.png)

Remote control

![Remote control](pic/控制台-设备远控.png)

Performance

![Performance](pic/控制台-性能采集.png)

Artifacts

![Artifacts](pic/控制台-测试产物.png)

Diagnostics

![Diagnostics](pic/控制台-运行诊断.png)

</div>

## Features

**Automation**

- Intelligent exploration: find tappable, scrollable, and input nodes from the UI tree, keep a scene graph, and support backtracking plus replay of known paths.
- Monkey: start structured random traversal for a target app, with duration, throttle, low-battery exit, and artifact finalization after stop.
- Record/replay: record touches, text, and gestures, replay them with normalized coordinates, and report passed / failed / not tested.
- On-device overlay: pick the target app, change parameters, and start or stop a run from the phone without going back to the PC.

**Device, apps, and remote control**

- Install, launch, stop, and uninstall apps, plus upload, download, and browse device files.
- Built-in web console: live screen, mouse / touch, keyboard, rotate, screenshot, and clipboard. Leaving the page disconnects and cleans up.
- Official scrcpy 4.1 video and control channel, shared by the console and automation jobs.

**Performance and artifacts**

- Collect CPU, memory, frame rate, jank, GPU, network, and battery for the target app, and write a session summary.
- Cold-start and warm-start multi-run stats, including P50, P90, and P95.
- Each run is stored under `/sdcard/xtest-nova/<package>/` with events, screenshots, diagnostics, and an evidence index. Stopping a job does not drop artifacts that are still being finalized.

**API and delivery**

- The web console and versioned `/v1/*` APIs are available together, so you can plug Nova into an existing test platform.
- Ship one Agent file per device ABI. On start, it unpacks and installs Runner, Companion, and UiAutomator.
- Listeners default to loopback. LAN access needs a token. High-risk command APIs stay off unless you turn them on.

The full interface list, compatibility scope, and limits are in the [HTTP contract](docs/compliance/http-contract.md) and [compatibility matrix](docs/compliance/compatibility.md).

## Architecture

```text
Browser / API client
        |
        | HTTP / WebSocket (ADB forward by default)
        v
Nova Agent (on device :7912)
  |-- Web console and HTTP API
  |-- Apps, files, performance, record/replay, diagnostics, artifacts
  |-- Monkey / exploration / scrcpy session control
        |
        +-- Runner: exploration and input
        +-- Companion: overlay and on-device settings (:8912)
        +-- UiAutomator: UI tree capture
        +-- scrcpy server 4.1: video and control
```

A release build produces ARM64 and ARMv7 Agents. Deploy only the file that matches the device. The Agent then verifies and installs the rest of the runtime. Sample / fixture apps are not part of the runtime and are never installed automatically.

| Component   | Role                                                            |
|-------------|-----------------------------------------------------------------|
| Agent       | On-device service entry: API, sessions, security, and artifacts |
| Runner      | Exploration, taps, swipes, and keys on the device               |
| Companion   | Overlay, permissions, and on-device parameters                  |
| UiAutomator | UI tree used by exploration and recording                       |

## Quick Start

### Requirements

- Windows PowerShell
- Go 1.25.0
- JDK 21
- Android SDK Platform 36 and Build Tools 36.0.0
- An Android device with USB debugging, plus a working `adb`

Toolchain versions are pinned in [`tools/build-toolchain.psd1`](tools/build-toolchain.psd1). The Gradle Wrapper is in the repo; you do not need a separate Gradle install.

### Build

Development build (Runner and development Agents only; reuses the signed embedded runtime already in the repo):

```powershell
.\build.ps1 -Development
```

A release build needs an out-of-repo signing certificate:

```powershell
$storePassword = Read-Host 'Keystore password' -AsSecureString
$keyPassword = Read-Host 'Key password' -AsSecureString
.\build.ps1 -KeyStore D:\secure\xtest-recovery-signing.jks -KeyAlias xtest-recovery `
  -StorePassword $storePassword -KeyPassword $keyPassword
```

Go Agent only:

```powershell
go test ./agent/...
go run ./agent/cmd/xtest-nova-agent
```

Repo-level build and release checks live in [`scripts/`](scripts/README.md). Device and end-to-end tests live in [`tests/`](tests/README.md).

### Deploy

```powershell
.\deploy.ps1 -Serial <device-serial> -StartServer
```

Run the command once per device. If you omit a local port, ADB assigns a free host port and prints the console URL. Do not forward two devices to the same fixed port.

After deploy, open the printed URL in a desktop browser, for example `http://127.0.0.1:7912/`. The on-device service listens on loopback only, so an `adb forward` is required. `deploy.ps1` sets that up for you.

Common on-device commands:

```text
adb shell /data/local/tmp/xtest-nova-agent server -d
adb shell /data/local/tmp/xtest-nova-agent server -d --stop
adb shell /data/local/tmp/xtest-nova-agent monkey status
adb shell /data/local/tmp/xtest-nova-agent monkey stop
adb shell /data/local/tmp/xtest-nova-agent version
```

`server -d` installs missing components and shows the overlay. `monkey stop` ends the current job only. It does not shut down the Agent or delete artifacts.

### Access

Default addresses:

- Agent / web console: `http://127.0.0.1:7912`
- Companion: `http://127.0.0.1:8912`

By default, 7912, 8912, and 7890 bind loopback only and are reached through a trusted `adb forward`. To reach 7912 directly on a LAN, also provide a token file:

```powershell
$tokenPath = Join-Path $HOME '.xtest-nova-api-token'
.\scripts\new-lan-token.ps1 -OutputPath $tokenPath
.\deploy.ps1 -Serial <device-serial> -StartServer -AllowLAN -ApiTokenFile $tokenPath
```

LAN mode opens only 7912 and requires a Bearer token or browser login. 8912 and 7890 stay on loopback. The token must be at least 32 bytes. Do not commit it, and do not put it in a URL. Generation, rotation, and recovery are in the [usage guide](docs/reference/usage.md).

`/shell`, `/shell/background`, and `/term` run arbitrary commands as the device shell, so they are off by default. Turn on `--legacy-unsafe-api` only in a trusted environment when an old client needs them. The flag does not authenticate and does not change listen addresses. Combined with LAN mode, those endpoints still require login. See the [`--legacy-unsafe-api` guide](docs/guides/legacy-unsafe-api.md).

## Documentation

Start here for daily use:

- [Documentation index](docs/README.md)
- [Usage](docs/reference/usage.md)
- [Architecture](docs/reference/architecture.md)
- [Release and rollback](docs/reference/release.md)
- [Exploration API](docs/api/exploration-api.md)
- [Monkey API](docs/api/monkey-api.md)
- [Record/replay API](docs/api/record-replay-api.md)
- [HTTP contract](docs/compliance/http-contract.md)
- [Compatibility matrix](docs/compliance/compatibility.md)

Design notes, implementation plans, and historical validation reports live under `docs/` by topic and are not listed here one by one. Use the [documentation index](docs/README.md) when you need a specific device run or remediation record.

## Acknowledgements

Thanks to [XTest](https://github.com/y-grey/XTest) for the reference and inspiration.

## Version and license

- [Changelog](CHANGELOG.md)
- [Notices](NOTICE.md)
- [Third-party software](THIRD_PARTY_NOTICES.md)
- [MIT License](LICENSE)
