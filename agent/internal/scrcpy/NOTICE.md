# scrcpy third-party notice

`scrcpy-server-v4.1.jar` is the unmodified server artifact from
[Genymobile/scrcpy v4.1](https://github.com/Genymobile/scrcpy/releases/tag/v4.1).

- Upstream: `https://github.com/Genymobile/scrcpy`
- Version: `4.1`
- SHA-256: `deacb991ed2509715160ffdc7907e47b4160eb30d1566217e9047fd5b8850cae`
- License: Apache License 2.0; the complete upstream license is stored in
  `LICENSE.scrcpy` next to the artifact.

The Nova integration does not modify the JAR. It verifies the embedded bytes
before staging them under a Nova-specific path on the Android device.
