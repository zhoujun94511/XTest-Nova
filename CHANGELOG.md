# Changelog

## Unreleased

- Separated rendering-scene detection from coordinate-input authorization. Render-only pages now use explicit `off`, `bounded`, or `continuous` policies; the default is bounded and Unity detection no longer grants unlimited input by itself.
- Added density-aware safe touch bounds, removed MENU from generic fallback, exposed the render policy in the Web console and Companion, and added an explicitly authorized 20-minute game scenario template.
- Split ad, billing, permission, and external-system recovery out of the oversized Runner entry point, and added dedicated render-probe and fallback-policy components with self-tests.
- Restored `system` as the default hierarchy provider until the documented Nova Provider U8 qualification gate is complete.
- Replaced the idle minimized overlay's red-orange tap target with a neutral edge handle, hid it from accessibility-driven exploration, and required a deliberate long press to reopen the menu while preserving the larger touch area.
- Added execution identity and owner fencing across exploration, recording and replay, plus generation-safe stop operations.
- Added observation/step freshness checks, idempotent action receipts, SHA-256 evidence indexes, case fingerprints and deterministic `passed/failed/not_tested` result judging.
- Added cold/warm application startup measurements with min/max/mean/P50/P90/P95 statistics and optional P95 baseline regression verdicts.
- Accepted Android warm-task `WaitTime` when `TotalTime` is omitted, with an explicit duration source, and classified bounded process reclamation after an owned Stop as `stopped` instead of an unexpected exit.
- Resolved the current Go IDE inspection set: close/error propagation, diagnostic-soak file ownership, wrapped network error matching, result-before-error use, duplicate HTTP/WebSocket flows, builtin-shadowing names, dead code, regex simplifications and error-string conventions.
- Changed `server -d` into the complete default startup: it bootstraps every bundled runtime component and automatically shows the Companion overlay; `--no-popup` is the explicit headless opt-out.
- Added package- and UID-level overlay permission reconciliation for Android 16 vendor AppOps behavior, plus a four-second live-overlay deployment gate that rejects transient startup success.
- Added `popupOverlayStartup` diagnostics and retained `popup start/status/uninstall` as recovery and maintenance commands.
- Canonicalized the product and runtime identity as XTest Nova across Agent release files, device paths, process files, artifact roots, web console, terminal, Companion labels, notifications, and version strings.
- Removed the unmaintained `androidbinary` dependency and its vendored parser. Activity coverage and application icons now come from Android `PackageManager` through a read-only, permission- and UID-gated Nova Companion provider.
- Added offline release checks that reject any return of the retired dependency, plus Android 13/16 package-metadata and Runner-finalization regression coverage.
- Removed the AndroidX UIAutomator runtime graph in favor of Android platform APIs, upgraded the wrapper to Gradle 8.14.5, and reduced the UiAutomator APK payloads accordingly.
- Moved the old port-9008 UiAutomator and JSON-RPC path behind the explicit `--legacy-uiautomator` switch; the default runtime neither probes nor advertises it.
- Upgraded the embedded, hash-pinned scrcpy server to 4.1 and added release-bound third-party notices, an SBOM, and dependency integrity gates.
- Added an authenticated LAN mode for the primary 7912 listener, including Bearer clients, short-lived browser sessions, login/logout/status endpoints, query-token rejection, and protected HTTP/WebSocket resources.
- Kept 8912 and 7890 loopback-only, protected optional LAN legacy APIs with the same Bearer/Cookie authentication layer, and added transactional token deployment, rotation, rollback, and default-mode recovery guidance.

## Unreleased — 2026-09-09

- Added multi-period anti-Ping-Pong protection to both exploration engines, including coarse navigation identity, progress-window cycle detection, full-cycle edge quarantine, bounded hierarchy fallback and validated parent backtracking.
- Added before/after Activity consistency checks so hierarchies captured across a foreground transition are retried instead of entering the graph as mixed states.
- Added cycle, blocked-edge, fallback and unstable-snapshot diagnostics plus Android 13 Foloy validation evidence.
- Added owned background-command shutdown so `/stop` and Agent process exit terminate and reap commands started through the explicitly enabled legacy API.
- Changed the JSON-RPC proxy to reject responses larger than 16 MiB with an explicit gateway error instead of returning truncated successful JSON.
- Hardened the release gate to require the exact six-artifact set, current Agent version, complete APK metadata, matching signing certificates, and valid sizes and hashes.
- Added a runtime route-registration test for all 77 declared historical method-path contracts.
- Reconciled Monkey graph, Activity denominator, cross-implementation A/B, compatibility-waiver, and remaining-work documentation after the September 7–9 qualification work.

## 0.22.0-m5.9-compat — 2026-09-04

- Added the device command surface for `server -d`, `server -d --stop`, `popup start/status/uninstall`, and `version`.
- Added owned PID lifecycle, detached logging, stale PID handling and process-identity validation.
- Aligned synchronous `/shell` query, form and JSON inputs plus `output`, `exitCode` and `error` responses.
- Aligned `/stop` with Agent shutdown semantics and expanded `/info` with the historical device fields required by legacy clients.
- Renamed release Agent artifacts and the device entry point during the compatibility phase; deployment added explicit unsafe-API and optional 8912/7890 forwarding switches.
- Passed unit, vet, race, 77-route document, release, Android 13 and Android 16 compatibility gates.
- Added 77 machine-validated request/response semantic contracts and user-flow gates for Monkey, record/replay APIs, streams and a real PTY terminal session. Popup UI parity remains open.
- Fixed `popup start` to use a no-display launcher that starts the overlay directly, instead of leaving the configuration activity in the foreground; `popup status` now reports the actual service state.

## 0.21.0-m5.8-contract77 — 2026-09-04

- Completed all 77 historical method-route contracts by aligning `GET/POST /shell/background` and `ANY /term` with the ATX wire protocol.
- Kept arbitrary command execution disabled by default; all three routes require the explicit `-legacy-unsafe-api` compatibility switch.
- Added PID-bearing background command responses, form/query/JSON aliases, child reaping, a 16-command concurrency limit and a 16 KiB command limit.
- Added an embedded terminal page and PTY WebSocket support for input, resize, disconnect cleanup, 64 KiB frames and four concurrent sessions.
- Verified default-deny and enabled behavior on Android 13 and Android 16, plus full unit, vet, contract and race gates.
- Consolidated all 77 method-route contracts, behavior summaries, implementation locations and qualification boundaries into `docs/compliance/http-contract.md`.

## 0.20.1-m5.8-review1 — 2026-09-03

- Fixed Android 16 foreground-activity parsing for Xiaomi's `ResumedActivity` format.
- Prevented stale recording and UiAutomator exits from corrupting restarted session state.
- Rejected oversized uploads instead of silently truncating them and hardened writes against symlink escapes.
- Bounded completed download-task history to prevent long-running memory growth.
- Declared the Android 14+ Companion foreground-service type and serialized overlay control requests.
- Added bounded HTTP responses, explicit error handling, input validation and connection cleanup to the Companion.
- Completed Android 13/16 compatibility, short resilience and concurrent request review gates; remaining environment limits are recorded in the pre-release review.
- Passed the full Go race detector suite on Windows with MSYS2 UCRT64 GCC 16.2.0.

## 0.20.0-m5.8 — 2026-09-03

- Built maintainable Go Agent, Java shell Runner and native Android Companion layers.
- Covered 74 of 77 historical method-route contracts; three unauthenticated command endpoints remain intentionally disabled.
- Added application, file, system, automation, monitoring, recording/replay, Popup Assistant, minicap and scrcpy compatibility services.
- Added deterministic exploration, state graphs, A/B reports and safety boundaries.
- Added signed release manifests, deployment health checks, rollback and Android 13/16 validation gates.
- Completed M5.3–M5.6 stateful fixture, delivery/media, Runner and streaming end-to-end validation.
- Replaced ambiguous compatibility states with evidence-backed qualification and W-001–W-012 waiver conditions.

Known qualification limits are maintained in `docs/compliance/compatibility-waivers.md`.
