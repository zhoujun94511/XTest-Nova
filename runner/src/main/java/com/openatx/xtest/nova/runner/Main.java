package com.openatx.xtest.nova.runner;

import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.*;
import java.util.concurrent.*;
import java.util.regex.*;

public final class Main {
    private static final long EXHAUSTED_RESTART_MIN_DELAY_MILLIS = 1000L;
    private static final long EXHAUSTED_RESTART_MAX_DELAY_MILLIS = 1500L;
    public static void main(String[] args) throws Exception {
        if (args.length == 1 && "--self-test".equals(args[0])) {
            longRunSelfTest();
            NodeExplorer.selfTest();
            AdScenePolicy.selfTest();
            SafetyActionPolicy.selfTest();
            RenderSceneProbe.selfTest();
            SafeTouchRegion.selfTest();
            CoordinateFallbackController.selfTest();
            CoordinateActionEvidence.selfTest();
            HierarchyFailurePolicy.selfTest();
            GenerationPingPongGuard.selfTest();
            RenderContextQuarantine.selfTest();
            ShellInput.selfTest();
            return;
        }
        if (args.length == 1 && "--self-test-sleep".equals(args[0])) {
            Thread.sleep(30000);
            return;
        }
        Config config = Config.parse(args);
        File stop = new File(config.stopFile);
        if (stop.exists() && !stop.delete()) throw new IOException("Cannot clear stop marker");
        Random random = new Random(config.seed);
        ShellInput input = new ShellInput();
        input.wakeAndDismissKeyguard();
        input.run("monkey", "-p", config.packageName, "-c", "android.intent.category.LAUNCHER", "1");
        SafeTouchRegion safeTouchRegion = input.safeTouchRegion();
        Guard guard = new Guard(config, input);
        SpecialHandler specialHandler = new SpecialHandler(config, input);
        NodeExplorer explorer = new NodeExplorer(config, input);
        CoordinateFallbackController fallbackController = new CoordinateFallbackController(config.renderFallbackMode);
        CoordinateActionEvidence coordinateEvidence = new CoordinateActionEvidence(config.packageName);
        HierarchyFailurePolicy hierarchyFailurePolicy = new HierarchyFailurePolicy();
        GenerationPingPongGuard generationPingPongGuard = new GenerationPingPongGuard();
        RenderContextQuarantine renderContextQuarantine = new RenderContextQuarantine();
        long end = System.currentTimeMillis() + config.durationSeconds * 1000L;
        long initialSettleUntil = System.currentTimeMillis() + 8000L;
        long entryHoldUntil = 0L;
        String entryHoldActivity = "";
        long handoffQuietUntil = 0L;
        String handoffQuietActivity = "";
        long count = 0;
        long cycleStartedAtCount = 0;
        int explorationCycle = 1;
        int consecutiveNoProgressCycles = 0;
        boolean cycleHadHierarchyExecution = false;
        String finishState = "completed";
        log("started", config, count);
        while (withinExecutionBudget(System.currentTimeMillis(), end, stop.exists())) {
            String decision = guard.check();
            emitCoordinateOutcome(explorer, fallbackController, renderContextQuarantine,
                    coordinateEvidence.observeActivity(guard.activity, CoordinateActionEvidence.elapsedRealtimeMillis()));
            if (decision != null) {
                if (decision.startsWith("stop:")) { finishState = decision.substring(5); break; }
                Thread.sleep(config.throttleMillis); continue;
            }
            GenerationPingPongGuard.Decision activePingPong = generationPingPongGuard.onLoop(
                    guard.activity, CoordinateActionEvidence.elapsedRealtimeMillis());
            if (activePingPong != null && activePingPong.blocked) {
                emitGenerationPingPong(explorer, guard.activity, activePingPong);
                Thread.sleep(Math.min(1500L, Math.max((long)config.throttleMillis, activePingPong.remainingMillis)));
                continue;
            }
            if (activePingPong != null && activePingPong.softRefresh) {
                explorer = softGenerationRefresh(explorer, guard.activity, explorationCycle, count, generationPingPongGuard.isConfirmed());
                explorationCycle++;
                cycleStartedAtCount = count;
                cycleHadHierarchyExecution = false;
                specialHandler = SpecialHandler.succeeding(specialHandler, config, input);
                continue;
            }
            decision = specialHandler.preflight(guard.activity);
            if (decision != null) {
                if (decision.startsWith("stop:")) specialHandler.relaunchTarget(guard.activity, "special", "special|preflight", decision.substring(5));
                Thread.sleep(config.throttleMillis); continue;
            }
            String handoff = handoffRecovery(guard.activity, handoffQuietActivity, System.currentTimeMillis(), handoffQuietUntil);
            if ("wait".equals(handoff)) {
                Thread.sleep(config.throttleMillis);
                continue;
            }
            if ("back".equals(handoff)) {
                handoffQuietActivity = guard.activity;
                handoffQuietUntil = System.currentTimeMillis() + 60_000L;
                input.key("KEYCODE_BACK");
                Thread.sleep(config.throttleMillis);
                continue;
            }
            NodeExplorer.ActResult explorationResult;
            try {
                NodeExplorer.Observation observation = explorer.observe(guard.activity);
                if (observation == null) {
                    Thread.sleep(config.throttleMillis);
                    continue;
                }
                if (!withinExecutionBudget(System.currentTimeMillis(), end, stop.exists())) break;
                emitCoordinateOutcome(explorer, fallbackController, renderContextQuarantine,
                        coordinateEvidence.observeHierarchy(observation.activity, observation.hierarchy,
                                CoordinateActionEvidence.elapsedRealtimeMillis()));
                decision = specialHandler.handle(observation.activity, observation.hierarchy);
                if (decision != null) {
                    hierarchyFailurePolicy.reset();
                    if (decision.startsWith("stop:")) specialHandler.relaunchTarget(observation.activity, "special", "special|handle", decision.substring(5));
                    Thread.sleep(config.throttleMillis); continue;
                }
                explorationResult = explorer.act(observation.activity, specialHandler.paywallContext, observation.hierarchy);
                if (explorationResult != NodeExplorer.ActResult.OBSERVATION_ERROR) hierarchyFailurePolicy.reset();
            } catch (Exception error) {
                explorer.emit("hierarchy_fallback", guard.activity, ",\"error\":\"" + Guard.escape(String.valueOf(error.getMessage())) + "\"");
                explorationResult = NodeExplorer.ActResult.OBSERVATION_ERROR;
            }
            if (explorationResult == NodeExplorer.ActResult.OBSERVATION_ERROR) {
                boolean knownAd = AdScenePolicy.isKnownActivity(guard.activity);
                explorer.emit("coordinate_fallback_suppressed", guard.activity,
                        ",\"reason\":\"hierarchy_unavailable\",\"knownAdActivity\":" + knownAd +
                        ",\"renderSignal\":\"none\"");
                if (knownAd) {
                    specialHandler.handle(guard.activity, "");
                } else {
                    HierarchyFailurePolicy.Decision recovery = hierarchyFailurePolicy.fail(guard.activity);
                    explorer.emit("hierarchy_failure_recovery", guard.activity,
                            ",\"attempt\":" + recovery.attempt + ",\"action\":\"" + recovery.action.name().toLowerCase(Locale.ROOT) + "\"");
                    if (recovery.action == HierarchyFailurePolicy.Action.BACK) {
                        input.key("KEYCODE_BACK");
                        count++;
                    } else if (recovery.action == HierarchyFailurePolicy.Action.RELAUNCH) {
                        specialHandler.relaunchTarget(guard.activity, "hierarchy", "hierarchy|unavailable", "hierarchy_unavailable");
                    }
                }
                Thread.sleep(config.throttleMillis);
                continue;
            }
            if (explorationResult == NodeExplorer.ActResult.EXHAUSTED) {
                long exhaustedAt = CoordinateActionEvidence.elapsedRealtimeMillis();
                if (explorer.isOnboardingExhaustion()) {
                    explorer.emit("onboarding_exhaustion_guard", guard.activity,
                            ",\"reason\":\"" + explorer.onboardingExhaustionReason() + "\",\"recovery\":\"stop\"");
                    finishState = "onboarding_exhausted";
                    break;
                }
                boolean unityContinuousExhaustion = fallbackController.mode() == CoordinateFallbackController.Mode.CONTINUOUS
                        && RenderSceneProbe.isUnityHost(guard.activity)
                        && explorer.canReleaseContinuousRenderExhaustion();
                if (unityContinuousExhaustion) {
                    NodeExplorer.UnityExhaustionRecovery recovery = explorer.recoverContinuousRenderExhaustion();
                    if (recovery == NodeExplorer.UnityExhaustionRecovery.RELEASE) {
                        explorer.emit("unity_cycle_released", guard.activity,
                                ",\"reason\":\"hierarchy_exhausted_continuous\",\"renderFallbackMode\":\"continuous\"");
                        Thread.sleep(config.throttleMillis);
                        continue;
                    }
                    if (recovery == NodeExplorer.UnityExhaustionRecovery.BACK) {
                        input.key("KEYCODE_BACK");
                        fallbackController.reset();
                        explorer.emit("unity_cycle_recovered", guard.activity,
                                ",\"reason\":\"repeated_hierarchy_exhaustion\",\"action\":\"back\"");
                        count++;
                        Thread.sleep(config.throttleMillis);
                        continue;
                    }
                }
                boolean recentCoordinateTarget = isGenerationPingPongCandidate(
                        fallbackController.mode(),
                        coordinateEvidence.hasRecentTargetResult(
                                exhaustedAt, GenerationPingPongGuard.RECENT_RESULT_WINDOW_MILLIS),
                        cycleHadHierarchyExecution);
                GenerationPingPongGuard.Decision pingPong = generationPingPongGuard.onExhausted(
                        guard.activity, recentCoordinateTarget, exhaustedAt);
                if (pingPong.blocked) {
                    emitGenerationPingPong(explorer, guard.activity, pingPong);
                    long waitMillis = Math.min(1500L, Math.max((long)config.throttleMillis, pingPong.remainingMillis));
                    Thread.sleep(waitMillis);
                    continue;
                }
                if (isEntryActivity(guard.activity, config.packageName) && count == cycleStartedAtCount) {
                    long now = System.currentTimeMillis();
                    if (!guard.activity.equals(entryHoldActivity)) {
                        entryHoldActivity = guard.activity;
                        entryHoldUntil = now + ENTRY_HOLD_MILLIS;
                    }
                    if (now < entryHoldUntil) {
                        explorer.emit("entry_wait", guard.activity, ",\"remainingMillis\":" + (entryHoldUntil - now));
                        Thread.sleep(config.throttleMillis);
                        continue;
                    }
                    if (now >= end || stop.exists()) break;
                    entryHoldUntil = now + ENTRY_HOLD_MILLIS;
                    specialHandler.relaunchTarget(guard.activity, "exploration", "entry-idle", "entry_activity_idle");
                    explorationCycle++;
                    cycleStartedAtCount = count;
                    cycleHadHierarchyExecution = false;
                    consecutiveNoProgressCycles = 0;
                    specialHandler = SpecialHandler.succeeding(specialHandler, config, input);
                    NodeExplorer nextExplorer = new NodeExplorer(config, input);
                    nextExplorer.inheritRecoveryHistory(explorer);
                    explorer = nextExplorer;
                    initialSettleUntil = now + 8000L;
                    explorer.emit("exploration_cycle_restarted", guard.activity,
                            ",\"cycle\":" + explorationCycle + ",\"events\":" + count +
                            ",\"consecutiveNoProgressCycles\":0,\"historyMode\":\"entry\"");
                    continue;
                }
                boolean madeProgress = count > cycleStartedAtCount;
                consecutiveNoProgressCycles = madeProgress ? 0 : consecutiveNoProgressCycles + 1;
                long restartDelay = exhaustedRestartDelayMillis(consecutiveNoProgressCycles, config.throttleMillis);
                boolean pingPongConfirmed = generationPingPongGuard.isConfirmed();
                boolean hardRecovery = pingPong.hardRecovery ||
                        pingPongConfirmed && generationPingPongGuard.shouldHardRecovery(exhaustedAt);
                explorer.emit("exploration_cycle_exhausted", guard.activity,
                        ",\"cycle\":" + explorationCycle + ",\"events\":" + count +
                        ",\"madeProgress\":" + madeProgress + ",\"restartDelayMillis\":" + restartDelay +
                        ",\"pingPongConfirmed\":" + pingPongConfirmed + ",\"hardRecovery\":" + hardRecovery);
                if (pingPongConfirmed && !hardRecovery) {
                    Thread.sleep(Math.min(restartDelay, (long)config.throttleMillis));
                    explorer = softGenerationRefresh(explorer, guard.activity, explorationCycle, count, true);
                    explorationCycle++;
                    cycleStartedAtCount = count;
                    cycleHadHierarchyExecution = false;
                    specialHandler = SpecialHandler.succeeding(specialHandler, config, input);
                    continue;
                }
                if (System.currentTimeMillis() + restartDelay >= end || stop.exists()) break;
                Thread.sleep(restartDelay);
                boolean recoveryGeneration = hardRecovery || consecutiveNoProgressCycles >= 2;
                boolean exhaustionLeftByBack = false;
                if ("relaunch".equals(cycleExhaustionAction(recoveryGeneration))) {
                    String relaunchReason = hardRecovery ? "pingpong_confirmed_stall" : "consecutive_no_progress_cycles";
                    specialHandler.relaunchTarget(guard.activity, "exploration", "cycle-no-progress", relaunchReason);
                    renderContextQuarantine.clearOnHardRecovery();
                    consecutiveNoProgressCycles = 0;
                } else {
                    exhaustionLeftByBack = true;
                    input.key("KEYCODE_BACK");
                    Thread.sleep(Math.max(300, config.throttleMillis));
                    input.wakeAndDismissKeyguard();
                }
                explorationCycle++;
                cycleStartedAtCount = count;
                cycleHadHierarchyExecution = false;
                specialHandler = SpecialHandler.succeeding(specialHandler, config, input);
                if (exhaustionLeftByBack) specialHandler.noteExhaustionBack(guard.activity);
                NodeExplorer nextExplorer = new NodeExplorer(config, input);
                if (recoveryGeneration) nextExplorer.inheritRecoveryHistory(explorer);
                else nextExplorer.inheritHistory(explorer);
                explorer = nextExplorer;
                initialSettleUntil = System.currentTimeMillis() + 8000L;
                explorer.emit("exploration_cycle_restarted", guard.activity,
                        ",\"cycle\":" + explorationCycle + ",\"events\":" + count +
                        ",\"consecutiveNoProgressCycles\":" + consecutiveNoProgressCycles +
                        ",\"historyMode\":\"" + (recoveryGeneration ? "recovery" : "inherit") + "\"" +
                        (hardRecovery ? ",\"reason\":\"pingpong_confirmed_stall\"" : ""));
                continue;
            }
            if (explorationResult == NodeExplorer.ActResult.ERROR) {
                RenderSceneProbe.Signal renderSignal = explorer.renderSignal();
                CoordinateFallbackController.Attempt fallback = fallbackController.begin(renderSignal);
                explorer.emit("fallback_action", guard.activity,
                        ",\"attempt\":" + fallback.number + ",\"limit\":" + CoordinateFallbackController.MAX_BOUNDED_ATTEMPTS +
                        ",\"renderSignal\":\"" + renderSignal.wireName + "\",\"renderFallbackMode\":\"" + fallbackController.mode().wireName() +
                        "\",\"continuousRender\":" + fallback.continuous + ",\"feedbackDowngraded\":" + fallback.downgraded);
                if (!fallback.allowed) {
                    specialHandler.relaunchTarget(guard.activity, "hierarchy", "hierarchy|fallback", "fallback_attempts_exhausted");
                    NodeExplorer nextExplorer = new NodeExplorer(config, input);
                    nextExplorer.inheritHistory(explorer);
                    explorer = nextExplorer;
                    fallbackController.reset();
                    Thread.sleep(Math.max(300, config.throttleMillis));
                    continue;
                }
                if (specialHandler.suppressRandom) {
                    input.key("KEYCODE_BACK");
                    specialHandler.emit("special_back", guard.activity, "paywall", "explore", "");
                    count++;
                    Thread.sleep(config.throttleMillis);
                    continue;
                }
                String renderContextKey = guard.activity + "|" + renderSignal.wireName;
                long coordinateElapsed = CoordinateActionEvidence.elapsedRealtimeMillis();
                CoordinateFallbackController.Gesture gesture = fallbackController.choose(random, safeTouchRegion, fallback.continuous,
                        guard.activity, renderSignal);
                if (gesture.type == CoordinateFallbackController.GestureType.TAP &&
                        !renderContextQuarantine.allowsTap(renderContextKey, coordinateElapsed)) {
                    explorer.emit("render_context_quarantine", guard.activity,
                            ",\"contextKey\":\"" + Guard.escape(renderContextKey) + "\",\"gesture\":\"tap_blocked\"" +
                            ",\"level\":\"" + renderContextQuarantine.status(renderContextKey, coordinateElapsed).level.name().toLowerCase(Locale.ROOT) + "\"");
                    gesture = new CoordinateFallbackController.Gesture(CoordinateFallbackController.GestureType.BACK);
                } else if (gesture.type == CoordinateFallbackController.GestureType.SWIPE &&
                        !renderContextQuarantine.allowsSwipe(renderContextKey, coordinateElapsed)) {
                    gesture = new CoordinateFallbackController.Gesture(CoordinateFallbackController.GestureType.BACK);
                }
                long coordinateStartedAt = coordinateElapsed;
                if (gesture.type == CoordinateFallbackController.GestureType.TAP) {
                    input.tap(gesture.coordinates[0], gesture.coordinates[1]);
                    emit(explorer, coordinateEvidence.begin(gesture, safeTouchRegion, guard.activity, renderSignal,
                            fallbackController.mode(), coordinateStartedAt));
                } else if (gesture.type == CoordinateFallbackController.GestureType.SWIPE) {
                    input.swipe(gesture.coordinates[0], gesture.coordinates[1], gesture.coordinates[2], gesture.coordinates[3]);
                    emit(explorer, coordinateEvidence.begin(gesture, safeTouchRegion, guard.activity, renderSignal,
                            fallbackController.mode(), coordinateStartedAt));
                } else {
                    input.key("KEYCODE_BACK");
                }
                count++;
            } else if (explorationResult == NodeExplorer.ActResult.EXECUTED) {
                fallbackController.reset();
                cycleHadHierarchyExecution = true;
                generationPingPongGuard.noteExplorationProgress(explorer.lastSemanticScene(),
                        CoordinateActionEvidence.elapsedRealtimeMillis());
                count++;
            }
            Thread.sleep(config.throttleMillis);
        }
        emit(explorer, coordinateEvidence.finish(guard.activity, CoordinateActionEvidence.elapsedRealtimeMillis()));
        log(stop.exists() ? "stopped" : finishState, config, count);
    }

    private static void emit(NodeExplorer explorer, CoordinateActionEvidence.Event event) {
        if (event != null) explorer.emit(event.state, event.activity, event.extra);
    }

    private static void emit(NodeExplorer explorer, List<CoordinateActionEvidence.Event> events) {
        if (events == null) return;
        for (CoordinateActionEvidence.Event event : events) emit(explorer, event);
    }

    private static void emitCoordinateOutcome(NodeExplorer explorer, CoordinateFallbackController controller,
                                              RenderContextQuarantine quarantine, CoordinateActionEvidence.Event event) {
        emit(explorer, event);
        CoordinateFallbackController.Feedback feedback = controller.observe(event);
        if (feedback == null) return;
        if ("external".equals(event.outcome) && !feedback.contextKey.isEmpty()) {
            RenderContextQuarantine.Status status = quarantine.onExternal(feedback.contextKey,
                    CoordinateActionEvidence.elapsedRealtimeMillis());
            explorer.emit("render_context_quarantine", event.activity,
                    ",\"contextKey\":\"" + Guard.escape(feedback.contextKey) + "\",\"level\":\"" +
                            status.level.name().toLowerCase(Locale.ROOT) + "\",\"externalResults\":" + feedback.externalResults +
                            ",\"tapBlockedUntilMillis\":" + status.tapBlockedUntilMillis);
        }
        if (feedback.zoneNewlyBlocked) {
            explorer.emit("coordinate_zone_blocked", event.activity,
                    ",\"zone\":\"" + Guard.escape(feedback.zone) + "\",\"contextKey\":\"" + Guard.escape(feedback.contextKey) +
                    "\",\"reason\":\"external_transition\"" +
                    ",\"externalResults\":" + feedback.externalResults);
        }
        if (feedback.downgradedNow) {
            explorer.emit("render_fallback_downgraded", event.activity,
                    ",\"from\":\"continuous\",\"to\":\"bounded\",\"reason\":\"external_transition_limit\"" +
                    ",\"externalResults\":" + feedback.externalResults +
                    ",\"limit\":" + CoordinateFallbackController.MAX_EXTERNAL_BEFORE_DOWNGRADE);
        }
    }

    private static NodeExplorer softGenerationRefresh(NodeExplorer explorer, String activity, int cycle, long events,
                                                      boolean pingPongConfirmed) {
        NodeExplorer nextExplorer = new NodeExplorer(explorer.config, explorer.input);
        nextExplorer.inheritSoftRefresh(explorer, explorer.lastSemanticScene());
        nextExplorer.emit("exploration_cycle_refreshed", activity,
                ",\"cycle\":" + (cycle + 1) + ",\"events\":" + events +
                ",\"historyMode\":\"soft\",\"pingPongConfirmed\":" + pingPongConfirmed);
        return nextExplorer;
    }

    private static void emitGenerationPingPong(NodeExplorer explorer, String activity,
                                                GenerationPingPongGuard.Decision decision) {
        if (!decision.report) return;
        explorer.emit(decision.entered ? "generation_pingpong_blocked" : "generation_pingpong_wait",
                activity, ",\"streak\":" + decision.streak + ",\"level\":" + decision.level +
                        ",\"holdMillis\":" + decision.holdMillis +
                        ",\"remainingMillis\":" + decision.remainingMillis +
                        ",\"reason\":\"coordinate_target_immediate_exhaustion\"");
    }

    static long exhaustedRestartDelayMillis(int consecutiveNoProgressCycles, int throttleMillis) {
        long multiplier = Math.max(1, consecutiveNoProgressCycles);
        long delay = Math.max(EXHAUSTED_RESTART_MIN_DELAY_MILLIS, (long)Math.max(1, throttleMillis) * multiplier);
        return Math.min(EXHAUSTED_RESTART_MAX_DELAY_MILLIS, delay);
    }

    static boolean isGenerationPingPongCandidate(CoordinateFallbackController.Mode mode,
                                                   boolean hasRecentCoordinateTarget,
                                                   boolean cycleHadHierarchyExecution) {
        return mode == CoordinateFallbackController.Mode.CONTINUOUS
                && hasRecentCoordinateTarget
                && !cycleHadHierarchyExecution;
    }

    static boolean withinExecutionBudget(long nowMillis, long deadlineMillis, boolean stopRequested) {
        return !stopRequested && nowMillis < deadlineMillis;
    }

    private static final long ENTRY_HOLD_MILLIS = 30_000L;

    static boolean isHandoffActivity(String activity) {
        String name = SafetyActionPolicy.activitySimpleName(activity);
        return name.contains("share") || name.contains("chooser") || name.contains("resolver") || name.contains("sendintent");
    }

    /** A cycle that already made progress only leaves the page. Relaunch is for a stalled cycle. */
    static String cycleExhaustionAction(boolean recoveryGeneration) {
        return recoveryGeneration ? "relaunch" : "back";
    }

    static String handoffRecovery(String activity, String quietActivity, long now, long quietUntil) {
        if (!isHandoffActivity(activity)) return "";
        if (activity.equals(quietActivity) && now < quietUntil) return "wait";
        return "back";
    }

    static boolean isEntryActivity(String activity, String targetPackage) {
        if (activity == null || targetPackage == null || targetPackage.isEmpty() || !activity.startsWith(targetPackage + "/")) return false;
        String name = SafetyActionPolicy.activitySimpleName(activity);
        if (name.contains("launcher")) return false;
        return name.contains("splash") || name.contains("startup") || name.contains("launch")
                || name.contains("loading") || name.contains("routing");
    }

    static void longRunSelfTest() {
        if (!isEntryActivity("com.example/.RoutingActivity", "com.example")
                || !isEntryActivity("com.example/.SplashActivity", "com.example")
                || !isEntryActivity("com.example/.LaunchActivity", "com.example"))
            throw new AssertionError("route and splash pages must wait before relaunch");
        if (isEntryActivity("com.android.launcher/.Launcher", "com.example")
                || isEntryActivity("com.example/.MainActivity", "com.example")
                || isEntryActivity("com.example/.LauncherActivity", "com.example")
                || isEntryActivity("com.example.launch/.MainActivity", "com.example.launch")
                || isEntryActivity("com.example/.startup.ui.VipPaywallActivity", "com.example")
                || isEntryActivity("com.example/com.google.android.gms.ads.AdActivity", "com.example"))
            throw new AssertionError("home, ordinary, and ad playback pages must not use the entry hold");
        if (!SpecialHandler.keepsExhaustionBack("com.example/.MainActivity", "com.example/.MainActivity", 1_000L, 9_000L)
                || !SpecialHandler.keepsExhaustionBack("com.example/.MainActivity", "", 1_000L, 9_000L)
                || SpecialHandler.keepsExhaustionBack("com.example/.MainActivity", "com.example/.ListActivity", 1_000L, 9_000L)
                || SpecialHandler.keepsExhaustionBack("com.example/.MainActivity", "com.example/.MainActivity", 9_000L, 9_000L))
            throw new AssertionError("an exhaustion return kept the cold start after the page stayed or changed");
        String[] systemLaunchers = {
                "com.android.launcher/com.android.launcher.Launcher",
                "com.android.launcher3/com.android.launcher3.uioverrides.QuickstepLauncher",
                "com.google.android.apps.nexuslauncher/com.google.android.apps.nexuslauncher.NexusLauncherActivity",
                "com.sec.android.app.launcher/com.sec.android.app.launcher.activities.LauncherActivity",
                "com.miui.home/com.miui.home.launcher.Launcher",
                "com.mi.android.globallauncher/com.miui.home.launcher.Launcher",
                "com.huawei.android.launcher/com.huawei.android.launcher.unihome.UniHomeLauncher",
                "com.hihonor.android.launcher/com.hihonor.android.launcher.unihome.UniHomeLauncher",
                "com.oppo.launcher/com.oppo.launcher.Launcher",
                "net.oneplus.launcher/net.oneplus.launcher.Launcher",
                "com.bbk.launcher2/com.bbk.launcher2.Launcher",
                "com.vivo.launcher/com.vivo.launcher.Launcher",
                "com.transsion.hilauncher/com.android.launcher3.Launcher",
                "com.transsion.XOSLauncher/com.android.launcher3.Launcher",
                "com.transsion.itel.launcher/com.android.launcher3.Launcher",
                "com.motorola.launcher3/com.motorola.launcher3.Launcher",
                "com.nothing.launcher/com.nothing.launcher.Launcher",
                "com.lge.launcher3/com.lge.launcher3.Launcher",
                "com.asus.launcher/com.asus.launcher.Launcher",
                "com.sonymobile.home/com.sonymobile.home.HomeActivity",
                "com.sonyericsson.home/com.sonyericsson.home.HomeActivity",
                "com.htc.launcher/com.htc.launcher.Launcher",
                "com.meizu.flyme.launcher/com.meizu.flyme.launcher.Launcher",
                "cn.nubia.launcher/cn.nubia.launcher.Launcher",
                "com.zte.mifavor.launcher/com.zte.mifavor.launcher.Launcher",
                "com.zui.launcher/com.zui.launcher.Launcher",
                "com.lenovo.launcher/com.lenovo.launcher.Launcher",
                "com.smartisanos.launcher/com.smartisanos.launcher.Launcher",
                "com.blackshark.launcher/com.blackshark.launcher.Launcher",
                "com.example/.LauncherActivity",
                "com.example/.QuickstepLauncher",
                "com.example/.HomeActivity"
        };
        for (String surface : systemLaunchers) {
            int slash = surface.indexOf('/');
            String pkg = slash > 0 ? surface.substring(0, slash) : surface;
            if (!SpecialHandler.isLauncherSurface(pkg, surface))
                throw new AssertionError("system launcher was not recognized: " + surface);
        }
        for (String pkg : SpecialHandler.SYSTEM_LAUNCHER_PACKAGES) {
            if (!SpecialHandler.isLauncherSurface(pkg, pkg + "/.Launcher"))
                throw new AssertionError("system launcher package was not recognized: " + pkg);
        }
        String[] ordinarySurfaces = {
                "com.example/.MainActivity",
                "com.example.share/.ListActivity",
                "com.example.launcher.settings/.MainActivity",
                "com.example.launcherhelper/.MainActivity",
                "com.example/.MyLauncherActivity",
                "com.example/.ChooserActivityLauncher"
        };
        for (String surface : ordinarySurfaces) {
            int slash = surface.indexOf('/');
            String pkg = slash > 0 ? surface.substring(0, slash) : surface;
            if (SpecialHandler.isLauncherSurface(pkg, surface))
                throw new AssertionError("launcher recognition leaked onto an ordinary page: " + surface);
        }
        if (!SpecialHandler.coldStartAfterExhaustionBack(true, true)
                || SpecialHandler.coldStartAfterExhaustionBack(false, true)
                || SpecialHandler.coldStartAfterExhaustionBack(true, false))
            throw new AssertionError("leaving an exhausted page for the launcher resumed the same task");
        if (!isHandoffActivity("com.example/.ShareActivity")
                || !isHandoffActivity("android/com.android.internal.app.ChooserActivity")
                || isHandoffActivity("com.example/.MainActivity")
                || isHandoffActivity("com.example.share/.MainActivity")
                || isHandoffActivity("com.example/com.google.android.gms.ads.AdActivity"))
            throw new AssertionError("handoff recognition leaked onto ordinary or ad playback pages");
        if (!"back".equals(cycleExhaustionAction(false)) || !"relaunch".equals(cycleExhaustionAction(true)))
            throw new AssertionError("a cycle that made progress relaunched the same task");
        if (!"back".equals(handoffRecovery("com.example/.ChooserActivityLauncher", "", 1_000L, 0L))
                || !"back".equals(handoffRecovery("com.example/.ShareActivity", "", 1_000L, 0L))
                || !"wait".equals(handoffRecovery("com.example/.ShareActivity", "com.example/.ShareActivity", 1_010L, 61_000L))
                || !"back".equals(handoffRecovery("com.example/.ShareActivity", "com.example/.ShareActivity", 62_000L, 61_000L)))
            throw new AssertionError("handoff cooldown bounced or relaunched");
        AdaptiveScheduler scheduler = new AdaptiveScheduler(1);
        NodeExplorer.Node node = NodeExplorer.Node.parse("<node text=\"Refresh\" class=\"android.widget.ImageView\" resource-id=\"com.example:id/refresh\" enabled=\"true\" visible-to-user=\"true\" password=\"false\" clickable=\"true\" bounds=\"[0,0][10,10]\" />");
        NodeExplorer.Action refresh = new NodeExplorer.Action("tap", node);
        NodeExplorer.Action other = new NodeExplorer.Action("tap", node, "", "", "tap|other");
        for (int attempt = 0; attempt < AdaptiveScheduler.MAX_DIRECT_ATTEMPTS_PER_ACTIVITY; attempt++) {
            if (!scheduler.eligible("com.example/.ListActivity", refresh)) throw new AssertionError("stable action was exhausted before its attempt budget");
            scheduler.recordExecuted("com.example/.ListActivity", refresh);
        }
        if (scheduler.eligible("com.example/.ListActivity", refresh)) throw new AssertionError("the same action became eligible again on a new scene");
        if (!scheduler.eligible("com.example/.ListActivity", other)) throw new AssertionError("a different action was blocked with the cooled action");
        if (!withinExecutionBudget(999L, 1000L, false)
                || withinExecutionBudget(1000L, 1000L, false)
                || withinExecutionBudget(999L, 1000L, true))
            throw new AssertionError("execution crossed its time or stop budget");
        if (exhaustedRestartDelayMillis(0, 300) != 1000L) throw new AssertionError("minimum exhausted restart delay");
        if (exhaustedRestartDelayMillis(4, 300) != 1200L) throw new AssertionError("progressive exhausted restart delay");
        if (exhaustedRestartDelayMillis(100, 300) != 1500L) throw new AssertionError("maximum exhausted restart delay");
        if (!isGenerationPingPongCandidate(CoordinateFallbackController.Mode.CONTINUOUS, true, false))
            throw new AssertionError("continuous coordinate exhaustion was not eligible for generation guard");
        if (isGenerationPingPongCandidate(CoordinateFallbackController.Mode.BOUNDED, true, false)
                || isGenerationPingPongCandidate(CoordinateFallbackController.Mode.OFF, true, false)
                || isGenerationPingPongCandidate(CoordinateFallbackController.Mode.CONTINUOUS, false, false)
                || isGenerationPingPongCandidate(CoordinateFallbackController.Mode.CONTINUOUS, true, true))
            throw new AssertionError("generation guard leaked outside continuous coordinate-only exhaustion");
        Config config = new Config();
        config.packageName = "com.example.app";
        SpecialHandler handler = new SpecialHandler(config, new ShellInput());
        handler.attempts.put("external|com.android.vending", 3);
        handler.preflight("com.example.app/.MainActivity");
        if (handler.attempts.containsKey("external|com.android.vending")) throw new AssertionError("external recovery budget was not reset");
        String unavailableAd = handler.handle("com.example.app/com.google.android.gms.ads.AdActivity", "");
        if (!"skip".equals(unavailableAd) || !handler.suppressRandom)
            throw new AssertionError("unavailable ad hierarchy did not suppress random input");
        if (!SpecialHandler.containsAny(" order info pay now card ", SpecialHandler.TARGET_PURCHASE_CONFIRMATION_MARKERS)) throw new AssertionError("target-owned final purchase confirmation was not recognized");
        config.activityMode = "blocklist";
        config.activities.add(".TemporaryActivity");
        config.blockedControls.add("skip-me");
        Config nextRun = new Config();
        if (!"none".equals(nextRun.activityMode) || !nextRun.activities.isEmpty() || !nextRun.blockedControls.isEmpty())
            throw new AssertionError("advanced rules leaked into a later Runner session");
        NodeExplorer.Node candidate = NodeExplorer.Node.parse("<node text=\"skip-me\" enabled=\"true\" visible-to-user=\"true\" password=\"false\" bounds=\"[0,0][100,100]\" />");
        if (candidate == null || candidate.eligible(config.blockedControls) || !candidate.eligible(Collections.<String>emptyList()))
            throw new AssertionError("control skip rule was not scoped to the matching candidate");
        if (SpecialHandler.adAttemptKey(7, "com.example/.Ad", "close_ready").equals(SpecialHandler.adAttemptKey(7, "com.example/.Ad", "confirm_exit")))
            throw new AssertionError("ad phases shared a retry budget");
        handler.adEncounter = 7;
        handler.adActive = true;
        handler.attempts.put(SpecialHandler.adAttemptKey(7, "com.example/.Ad", "confirm_exit"), 3);
        handler.attemptAt.put(SpecialHandler.adAttemptKey(7, "com.example/.Ad", "confirm_exit"), 1L);
        handler.adWaitStarted.put("com.example/.Ad", 1L);
        handler.resetAdRecoveryState();
        if (handler.adActive || !handler.attempts.isEmpty() || !handler.attemptAt.isEmpty() || !handler.adWaitStarted.isEmpty())
            throw new AssertionError("ad recovery state was not reset after relaunch");
    }

    private static void log(String state, Config config, long count) {
        System.out.println("{\"state\":\""+state+"\",\"requestId\":\""+Guard.escape(config.requestId)+"\",\"package\":\""+config.packageName+"\",\"seed\":"+config.seed+",\"events\":"+count+",\"time\":"+System.currentTimeMillis()+"}");
    }
}
