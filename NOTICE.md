# XTest Nova notices

Copyright 2026 zhoujun94511. XTest Nova source code is distributed under the
[MIT License](LICENSE).

The repository embeds the unmodified `scrcpy-server-v4.1.jar` from
Genymobile/scrcpy 4.1. That artifact is distributed under Apache License 2.0.
Its source, version, fixed SHA-256, license and notice are documented in the
[scrcpy notice](agent/internal/scrcpy/NOTICE.md) and
[scrcpy license](agent/internal/scrcpy/LICENSE.scrcpy).

The Agent embeds `github.com/coder/websocket` v1.8.15 under the ISC License and
`github.com/creack/pty` v1.1.24 under the MIT License. Their complete license
texts are preserved in the vendored module directories and summarized in
[the third-party notices](THIRD_PARTY_NOTICES.md).

The independently built `uiautomator` module uses only Android platform APIs at runtime. Gradle and the Android Gradle Plugin remain build-time tools; their versions and verified artifact hashes are locked under `uiautomator/gradle`.

XTest Nexus is used only as a behavior and protocol reference. Nova does not include recovered Nexus DEX, Smali, signing keys, credentials or proprietary binary resources.
