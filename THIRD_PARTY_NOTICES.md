# XTest Nova third-party notices

This file inventories the third-party software shipped with, or used to build,
XTest Nova. The release process creates `dist/THIRD_PARTY_NOTICES.txt` by
combining this inventory with the complete licenses referenced below.

## Distributed runtime components

| Component                    | Version | License    | Purpose                                                              | Complete license                                             |
|------------------------------|--------:|------------|----------------------------------------------------------------------|--------------------------------------------------------------|
| `github.com/coder/websocket` |  1.8.15 | ISC        | WebSocket transport for screen, control, touch and terminal channels | [LICENSE.txt](vendor/github.com/coder/websocket/LICENSE.txt) |
| `github.com/creack/pty`      |  1.1.24 | MIT        | Pseudo-terminal support for the explicitly enabled legacy terminal   | [LICENSE](vendor/github.com/creack/pty/LICENSE)              |
| Genymobile `scrcpy-server`   |     4.1 | Apache-2.0 | Android video and control server                                     | [LICENSE.scrcpy](agent/internal/scrcpy/LICENSE.scrcpy)       |

The embedded scrcpy artifact is unmodified and its SHA-256 is
`deacb991ed2509715160ffdc7907e47b4160eb30d1566217e9047fd5b8850cae`.

## Build-only components

| Component                     | Version | License    | Integrity record                                                                                                                                        |
|-------------------------------|--------:|------------|---------------------------------------------------------------------------------------------------------------------------------------------------------|
| Gradle Wrapper / distribution |  8.14.5 | Apache-2.0 | [Wrapper configuration](uiautomator/gradle/wrapper/gradle-wrapper.properties) and wrapper JAR hash in the [SBOM](docs/compliance/third-party-sbom.json) |
| Android Gradle Plugin         |  8.11.2 | Apache-2.0 | [Dependency verification metadata](uiautomator/gradle/verification-metadata.xml)                                                                        |

AndroidX UIAutomator is intentionally not a runtime dependency. The two Nova
UiAutomator APKs use Android platform instrumentation and accessibility APIs
only; `releaseRuntimeClasspath` is empty for both applications.

