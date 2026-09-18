package com.openatx.xtest.nova.runner;

import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.*;
import java.util.concurrent.*;
import java.util.regex.*;

public final class Main {
    private static final Pattern PACKAGE = Pattern.compile("^[A-Za-z][A-Za-z0-9_]*(\\.[A-Za-z][A-Za-z0-9_]*)+$");
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
                specialHandler = new SpecialHandler(config, input);
                continue;
            }
            decision = specialHandler.preflight(guard.activity);
            if (decision != null) {
                if (decision.startsWith("stop:")) specialHandler.relaunchTarget(guard.activity, "special", "special|preflight", decision.substring(5));
                Thread.sleep(config.throttleMillis); continue;
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
                String startupActivity = guard.activity == null ? "" : guard.activity.toLowerCase(Locale.ROOT);
                if (System.currentTimeMillis() < initialSettleUntil && (startupActivity.contains("splash") || startupActivity.contains("startup") || startupActivity.contains("launch") || startupActivity.contains("loading"))) {
                    Thread.sleep(config.throttleMillis);
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
                    specialHandler = new SpecialHandler(config, input);
                    continue;
                }
                if (System.currentTimeMillis() + restartDelay >= end || stop.exists()) break;
                Thread.sleep(restartDelay);
                boolean recoveryGeneration = hardRecovery || consecutiveNoProgressCycles >= 2;
                if (recoveryGeneration) {
                    String relaunchReason = hardRecovery ? "pingpong_confirmed_stall" : "consecutive_no_progress_cycles";
                    specialHandler.relaunchTarget(guard.activity, "exploration", "cycle-no-progress", relaunchReason);
                    renderContextQuarantine.clearOnHardRecovery();
                    consecutiveNoProgressCycles = 0;
                } else {
                    input.key("KEYCODE_BACK");
                    Thread.sleep(Math.max(300, config.throttleMillis));
                    input.wakeAndDismissKeyguard();
                    input.run("monkey", "-p", config.packageName, "-c", "android.intent.category.LAUNCHER", "1");
                }
                explorationCycle++;
                cycleStartedAtCount = count;
                cycleHadHierarchyExecution = false;
                specialHandler = new SpecialHandler(config, input);
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

    static void longRunSelfTest() {
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

    static final class NodeExplorer {
        enum ActResult { EXECUTED, WAITING, EXHAUSTED, ERROR, OBSERVATION_ERROR }
        enum UnityExhaustionRecovery { RELEASE, BACK, FALLTHROUGH }
        private static final int MAX_RECENT_TRANSITIONS = 64;
        private static final int MAX_CYCLE_PERIOD = 8;
        private static final int CYCLE_REPETITIONS = 3;
        private static final int REPEATED_TRANSITION_LIMIT = 4;
        private static final int MAX_NON_SCROLL_BEFORE_SCROLL = 2;
        private static final int MAX_STABLE_SCROLL_ATTEMPTS = 3;
        private static final long MAX_ONBOARDING_TRANSITION_WAIT_MILLIS = 20_000L;
        static final Pattern NODE_TAG = Pattern.compile("</?node\\b[^>]*>");
        private static final Pattern ATTRIBUTE = Pattern.compile("([A-Za-z0-9_-]+)=\"([^\"]*)\"");
        private static final Pattern BOUNDS = Pattern.compile("\\[(\\d+),(\\d+)]\\[(\\d+),(\\d+)]");
        private static final String[] SENSITIVE_INPUTS = {"password", "passwd", "pin", "otp", "verification code", "security code", "cvv", "card number", "bank account", "payment", "密码", "口令", "验证码", "校验码", "动态码", "支付", "银行卡", "卡号", "账户", "账号"};
        private static final int DEFAULT_INPUT_CASES_PER_FIELD = 1;
        private static final int MAX_INPUT_CASES_PER_FIELD = 18;
        private static final int DEFAULT_INPUT_MAX_LENGTH = 64;
        final Config config;
        final ShellInput input;
        final Map<String, SceneState> scenes = new LinkedHashMap<>();
        final Map<String, Map<String, String>> transitions = new LinkedHashMap<>();
        final Map<String, Map<String, EdgeStats>> transitionStats = new LinkedHashMap<>();
        final Map<String, String> parents = new HashMap<>();
        final Set<String> routed = new HashSet<>();
        final Set<String> blockedEdges = new HashSet<>();
        final Set<String> blockedSemanticEdges = new HashSet<>();
        final Map<String, Set<String>> semanticTried = new HashMap<>();
        final Set<String> onboardingTried = new HashSet<>();
        final Map<String, Integer> unityExhaustionRecoveryAttempts = new HashMap<>();
        final Set<String> inputTried = new HashSet<>();
        final AdaptiveScheduler scheduler;
        final Map<String, Integer> semanticTransitionCounts = new HashMap<>();
        final ArrayDeque<TransitionSample> recentTransitions = new ArrayDeque<>();
        String previousScene;
        String previousSemanticScene;
        String previousCycleScene;
        String previousViewportScene;
        String previousActivity;
        Action previousAction;
        long previousActionAt;
        String lastExhaustedSemanticId = "";
        boolean lastExhaustedOnboarding;
        String lastOnboardingExhaustionReason = "safe_advance_actions_exhausted";
        String onboardingActivity;
        int onboardingTransitionWaits;
        long onboardingTransitionStartedAt;
        RenderSceneProbe.Signal lastRenderSignal = RenderSceneProbe.Signal.NONE;

        NodeExplorer(Config config, ShellInput input){this.config=config;this.input=input;this.scheduler=new AdaptiveScheduler(config.seed);}

        void inheritHistory(NodeExplorer previous) {
            inputTried.addAll(previous.inputTried);
            scheduler.inherit(previous.scheduler);
            blockedSemanticEdges.addAll(previous.blockedSemanticEdges);
            semanticTransitionCounts.putAll(previous.semanticTransitionCounts);
            unityExhaustionRecoveryAttempts.putAll(previous.unityExhaustionRecoveryAttempts);
            for (Map.Entry<String, Set<String>> entry : previous.semanticTried.entrySet()) {
                semanticTried.put(entry.getKey(), new HashSet<>(entry.getValue()));
            }
            onboardingTried.addAll(previous.onboardingTried);
            onboardingActivity = previous.onboardingActivity;
            onboardingTransitionWaits = previous.onboardingTransitionWaits;
            onboardingTransitionStartedAt = previous.onboardingTransitionStartedAt;
        }

        void inheritRecoveryHistory(NodeExplorer previous) {
            inputTried.addAll(previous.inputTried);
            scheduler.inheritRecovery(previous.scheduler);
            blockedSemanticEdges.addAll(previous.blockedSemanticEdges);
            semanticTransitionCounts.putAll(previous.semanticTransitionCounts);
            unityExhaustionRecoveryAttempts.putAll(previous.unityExhaustionRecoveryAttempts);
        }

        void inheritSoftRefresh(NodeExplorer previous, String semanticScene) {
            inheritHistory(previous);
            if (semanticScene == null || semanticScene.isEmpty()) return;
            blockedSemanticEdges.removeIf(key -> key.startsWith(semanticScene + "|"));
            for (Map.Entry<String, SceneState> entry : previous.scenes.entrySet()) {
                if (semanticScene.equals(entry.getValue().semanticId)) {
                    blockedEdges.removeIf(key -> key.startsWith(entry.getKey() + "|"));
                }
            }
        }

        /** Unity continuous mode gets one replay, then an active Back recovery before normal generation recovery. */
        boolean canReleaseContinuousRenderExhaustion() {
            return lastExhaustedSemanticId != null && !lastExhaustedSemanticId.isEmpty();
        }

        UnityExhaustionRecovery recoverContinuousRenderExhaustion() {
            if (!canReleaseContinuousRenderExhaustion()) return UnityExhaustionRecovery.FALLTHROUGH;
            String semantic = lastExhaustedSemanticId;
            lastExhaustedSemanticId = "";
            int attempt = unityExhaustionRecoveryAttempts.getOrDefault(semantic, 0) + 1;
            unityExhaustionRecoveryAttempts.put(semantic, attempt);
            if (attempt == 2) return UnityExhaustionRecovery.BACK;
            if (attempt > 2) return UnityExhaustionRecovery.FALLTHROUGH;
            for (SceneState state : scenes.values()) {
                if (!semantic.equals(state.semanticId)) continue;
                state.tried.clear();
                state.attempts.clear();
                state.nonScrollSelections = 0;
                semanticTried.remove(state.cycleId);
                for (String sceneId : new ArrayList<>(scenes.keySet())) {
                    SceneState candidate = scenes.get(sceneId);
                    if (candidate != null && semantic.equals(candidate.semanticId)) {
                        blockedEdges.removeIf(key -> key.startsWith(sceneId + "|"));
                    }
                }
            }
            blockedSemanticEdges.removeIf(key -> key.startsWith(semantic + "|"));
            return UnityExhaustionRecovery.RELEASE;
        }

        Observation observe(String activity) throws Exception {
            String beforeActivity = activity == null ? "" : activity;
            String hierarchy = Guard.http("http://127.0.0.1:7912/v1/hierarchy/raw");
            String afterActivity = Guard.currentActivity(input);
            if (!beforeActivity.equals(afterActivity)) {
                emit("unstable_snapshot", afterActivity, ",\"beforeActivity\":\"" + Guard.escape(beforeActivity) + "\",\"afterActivity\":\"" + Guard.escape(afterActivity) + "\"");
                return null;
            }
            return new Observation(afterActivity.isEmpty() ? activity : afterActivity, hierarchy);
        }

        ActResult act(String activity, boolean paywallContext, String hierarchy) {
            try {
                lastExhaustedOnboarding = false;
                lastOnboardingExhaustionReason = "safe_advance_actions_exhausted";
                SceneSnapshot snapshot = SceneSnapshot.parse(config, activity, hierarchy, paywallContext);
                lastRenderSignal = RenderSceneProbe.detect(activity, snapshot.renderSurface, !snapshot.actions.isEmpty());
                if (RenderSceneProbe.isRenderScene(lastRenderSignal)) {
                    emit("render_scene_detected", activity,
                            ",\"signal\":\"" + lastRenderSignal.wireName + "\",\"mode\":\"" + config.renderFallbackMode +
                            "\",\"nodes\":" + snapshot.nodeCount);
                    if ("off".equals(config.renderFallbackMode)) return ActResult.EXHAUSTED;
                    emit("hierarchy_fallback", activity,
                            ",\"reason\":\"render_scene_without_actionable_nodes\",\"signal\":\"" + lastRenderSignal.wireName +
                            "\",\"nodes\":" + snapshot.nodeCount);
                    return ActResult.ERROR;
                }
                boolean globallyNewState = scheduler.observeState((activity == null ? "" : activity) + "|" + snapshot.semanticId);
                SceneState state = scenes.get(snapshot.id);
                boolean newScene = state == null;
                if (state == null) {
                    state = new SceneState(snapshot);
                    scenes.put(snapshot.id, state);
                    emit("scene", activity, ",\"scene\":\"" + snapshot.id + "\",\"semanticScene\":\"" + snapshot.semanticId + "\",\"cycleScene\":\"" + snapshot.cycleId + "\",\"nodes\":" + snapshot.nodeCount + ",\"actions\":" + snapshot.actions.size());
                }
                if (!recordTransition(snapshot.id, snapshot.semanticId, snapshot.cycleId, snapshot.viewportId, newScene, globallyNewState, activity)) return ActResult.WAITING;

                boolean onboardingScene = hasOnboardingAdvance(state.actions);
                if (onboardingActivity != null && !Objects.equals(onboardingActivity, activity)) {
                    onboardingActivity = null;
                    onboardingTransitionWaits = 0;
                    onboardingTransitionStartedAt = 0L;
                }
                if (onboardingScene) {
                    onboardingActivity = activity;
                    onboardingTransitionWaits = 0;
                    onboardingTransitionStartedAt = 0L;
                } else if (Objects.equals(onboardingActivity, activity) && state.actions.isEmpty()) {
                    long now = System.currentTimeMillis();
                    if (onboardingTransitionStartedAt == 0L) onboardingTransitionStartedAt = now;
                    long elapsed = Math.max(0L, now - onboardingTransitionStartedAt);
                    if (elapsed < MAX_ONBOARDING_TRANSITION_WAIT_MILLIS) {
                        onboardingTransitionWaits++;
                        emit("onboarding_transition_wait", activity,
                                ",\"attempt\":" + onboardingTransitionWaits + ",\"elapsedMillis\":" + elapsed +
                                ",\"limitMillis\":" + MAX_ONBOARDING_TRANSITION_WAIT_MILLIS);
                        return ActResult.WAITING;
                    }
                    lastExhaustedSemanticId = snapshot.semanticId;
                    lastExhaustedOnboarding = true;
                    lastOnboardingExhaustionReason = "transition_timeout";
                    return ActResult.EXHAUSTED;
                } else if (Objects.equals(onboardingActivity, activity)) {
                    onboardingActivity = null;
                    onboardingTransitionWaits = 0;
                    onboardingTransitionStartedAt = 0L;
                }

                Action action = nextUntried(state, snapshot.id, snapshot.semanticId, snapshot.cycleId, activity);
                if (action == null) {
                    action = findPathAction(snapshot.id);
                    if (action != null) {
                        emit("graph_path", activity, ",\"scene\":\"" + snapshot.id + "\",\"action\":\"" + Guard.escape(action.id) + "\"");
                    }
                }
                if (action == null && parents.containsKey(snapshot.id) && !state.tried.contains("back") && !isBlocked(snapshot.id, snapshot.cycleId, "back")) {
                    action = Action.back();
                }
                if (action == null) {
                    lastExhaustedSemanticId = snapshot.semanticId;
                    lastExhaustedOnboarding = hasOnboardingAdvance(state.actions);
                    return ActResult.EXHAUSTED;
                }

                try {
                    execute(action, activity, snapshot.id);
                } catch (Exception actionError) {
                    blockEdge(snapshot.id, snapshot.cycleId, action.id, "action_failed", activity);
                    emit("action_failed", activity, ",\"scene\":\"" + snapshot.id + "\",\"action\":\"" + Guard.escape(action.id) + "\",\"error\":\"" + Guard.escape(String.valueOf(actionError.getMessage())) + "\"");
                    return ActResult.WAITING;
                }
                markExecuted(state, snapshot.cycleId, activity, action);
                previousScene = snapshot.id;
                previousSemanticScene = snapshot.semanticId;
                previousCycleScene = snapshot.cycleId;
                previousViewportScene = snapshot.viewportId;
                previousAction = action;
                previousActivity = activity;
                previousActionAt = System.currentTimeMillis();
                lastRenderSignal = RenderSceneProbe.Signal.NONE;
                return ActResult.EXECUTED;
            } catch(Exception error) {
                lastRenderSignal = RenderSceneProbe.Signal.NONE;
                emit("hierarchy_fallback", activity, ",\"error\":\"" + Guard.escape(String.valueOf(error.getMessage())) + "\"");
            }
            return ActResult.OBSERVATION_ERROR;
        }

        RenderSceneProbe.Signal renderSignal() { return lastRenderSignal; }

        boolean isOnboardingExhaustion() { return lastExhaustedOnboarding; }

        String onboardingExhaustionReason() { return lastOnboardingExhaustionReason; }

        String lastSemanticScene() { return previousSemanticScene == null ? "" : previousSemanticScene; }

        static final class Observation {
            final String activity, hierarchy;
            Observation(String activity, String hierarchy) { this.activity = activity; this.hierarchy = hierarchy; }
        }

        Action nextUntried(SceneState state, String scene, String semanticScene, String cycleScene, String activity) {
            // The cycle fingerprint is Activity + available action identities.
            // It remains stable while prices, timers and other dynamic labels
            // churn, so completed actions are not rediscovered as new coverage.
            Set<String> attempted = semanticTried.computeIfAbsent(cycleScene, ignored -> new HashSet<>());
            List<Action> onboardingCandidates = new ArrayList<>();
            for (Action action : state.actions) {
                if (action.isOnboardingAdvance() && usable(state, attempted, scene, cycleScene, activity, action)) {
                    onboardingCandidates.add(action);
                }
            }
            Action onboarding = scheduler.select(onboardingCandidates, activity);
            if (onboarding != null) return onboarding;
            if (state.nonScrollSelections >= MAX_NON_SCROLL_BEFORE_SCROLL) {
                List<Action> scrollCandidates = new ArrayList<>();
                for (Action action : state.actions) {
                    if (action.isScrollGesture() && usable(state, attempted, scene, cycleScene, activity, action)) scrollCandidates.add(action);
                }
                Action selected = scheduler.select(scrollCandidates, activity);
                if (selected != null) return selected;
            }
            List<Action> primaryCandidates = new ArrayList<>();
            for (Action action : state.actions) {
                if (!"input".equals(action.type) && usable(state, attempted, scene, cycleScene, activity, action)) primaryCandidates.add(action);
            }
            Action selected = scheduler.select(primaryCandidates, activity);
            if (selected != null) return selected;
            List<Action> inputCandidates = new ArrayList<>();
            for (Action action : state.actions) {
                if (usable(state, attempted, scene, cycleScene, activity, action)) inputCandidates.add(action);
            }
            return scheduler.select(inputCandidates, activity);
        }

        boolean usable(SceneState state, Set<String> attempted, String scene, String cycleScene, String activity, Action action) {
            if (action.isOnboardingAdvance()) {
                if (blockedEdges.contains(edgeKey(scene, action.id))) return false;
                return !state.tried.contains(action.id) &&
                        !onboardingTried.contains(onboardingKey(activity, state.semanticId, action));
            }
            if (isBlocked(scene, cycleScene, action.id)) return false;
            if ("input".equals(action.type) && inputTried.contains(action.id)) return false;
            if (action.isScrollGesture()) return state.attempts.getOrDefault(action.id, 0) < MAX_STABLE_SCROLL_ATTEMPTS;
            if (!scheduler.eligible(activity, action)) return false;
            return !state.tried.contains(action.id) && !attempted.contains(action.id);
        }

        void markExecuted(SceneState state, String cycleScene, String activity, Action action) {
            Set<String> attempted = semanticTried.computeIfAbsent(cycleScene, ignored -> new HashSet<>());
            int attempts = state.attempts.getOrDefault(action.id, 0) + 1;
            state.attempts.put(action.id, attempts);
            if (action.isScrollGesture()) {
                state.nonScrollSelections = 0;
                if (attempts >= MAX_STABLE_SCROLL_ATTEMPTS) state.tried.add(action.id);
            } else {
                state.nonScrollSelections++;
                state.tried.add(action.id);
                if (action.isOnboardingAdvance()) {
                    onboardingTried.add(onboardingKey(activity, state.semanticId, action));
                } else {
                    attempted.add(action.id);
                }
            }
            if ("input".equals(action.type)) inputTried.add(action.id);
            scheduler.recordExecuted(activity, action);
        }

        static boolean hasOnboardingAdvance(List<Action> actions) {
            for (Action action : actions) if (action.isOnboardingAdvance()) return true;
            return false;
        }

        static String onboardingKey(String activity, String semanticScene, Action action) {
            return (activity == null ? "" : activity.toLowerCase(Locale.ROOT)) + "|" +
                    (semanticScene == null ? "" : semanticScene) + "|" + action.id;
        }

        boolean recordTransition(String scene, String semanticScene, String cycleScene, String viewportScene, boolean newScene, boolean globallyNewState, String activity) {
            if (previousScene == null || previousAction == null) return true;
            if (previousScene.equals(scene) && System.currentTimeMillis() - previousActionAt < 1200) return false;
            if (newScene && !"back".equals(previousAction.type) && !previousScene.equals(scene)) {
                parents.putIfAbsent(scene, previousScene);
            }
            Map<String, String> edges = transitions.get(previousScene);
            if (edges == null) {
                edges = new LinkedHashMap<>();
                transitions.put(previousScene, edges);
            }
            Map<String, EdgeStats> sourceStats = transitionStats.computeIfAbsent(previousScene, ignored -> new LinkedHashMap<>());
            EdgeStats stats = sourceStats.computeIfAbsent(previousAction.id, ignored -> new EdgeStats());
            stats.observe(scene);
            String old = edges.put(previousAction.id, stats.preferred());
            if (!scene.equals(old)) {
                emit("scene_transition", activity, ",\"from\":\"" + previousScene + "\",\"action\":\"" + Guard.escape(previousAction.id) + "\",\"to\":\"" + scene + "\"");
            }
            if (previousAction.isScrollGesture()) {
                boolean progressed = previousViewportScene != null && !previousViewportScene.equals(viewportScene);
                emit(progressed ? "scroll_progress" : "scroll_stall", activity, ",\"from\":\"" + previousScene + "\",\"to\":\"" + scene + "\",\"viewportFrom\":\"" + previousViewportScene + "\",\"viewportTo\":\"" + viewportScene + "\"");
            }
            if ("back".equals(previousAction.type)) {
                String expected = parents.get(previousScene);
                if (expected == null || !expected.equals(scene)) {
                    blockEdge(previousScene, previousCycleScene, previousAction.id, "backtrack_mismatch", activity);
                }
            }
            if (!"input".equals(previousAction.type)) {
                TransitionSample sample = new TransitionSample(previousScene, previousCycleScene, previousActivity, previousAction.id, scene, cycleScene, activity);
                scheduler.recordTransition(previousActivity, previousAction, globallyNewState, !Objects.equals(previousActivity, activity), previousSemanticScene != null && previousSemanticScene.equals(semanticScene));
                recentTransitions.addLast(sample);
                while (recentTransitions.size() > MAX_RECENT_TRANSITIONS) recentTransitions.removeFirst();
                String semanticPair = sample.cycleKey();
                int transitionCount = semanticTransitionCounts.getOrDefault(semanticPair, 0) + 1;
                semanticTransitionCounts.put(semanticPair, transitionCount);
                if (transitionCount >= REPEATED_TRANSITION_LIMIT) {
                    blockEdge(previousScene, previousCycleScene, previousAction.id, "repeated_transition", activity);
                }
                int period = repeatedCyclePeriod();
                if (period > 0) {
                    blockRepeatedCycle(period, activity);
                    emit("cycle_detected", activity, ",\"period\":" + period + ",\"repetitions\":" + CYCLE_REPETITIONS + ",\"from\":\"" + previousScene + "\",\"action\":\"" + Guard.escape(previousAction.id) + "\",\"to\":\"" + scene + "\"");
                }
            }
            previousScene = null;
            previousSemanticScene = null;
            previousCycleScene = null;
            previousViewportScene = null;
            previousActivity = null;
            previousAction = null;
            return true;
        }

        void blockRepeatedCycle(int period, String activity) {
            List<TransitionSample> values = new ArrayList<>(recentTransitions);
            int start = Math.max(0, values.size() - period);
            for (int i = start; i < values.size(); i++) {
                TransitionSample sample = values.get(i);
                blockEdge(sample.from, sample.semanticFrom, sample.action, "repeated_cycle", activity);
            }
        }

        int repeatedCyclePeriod() {
            List<TransitionSample> values = new ArrayList<>(recentTransitions);
            for (int period = 1; period <= MAX_CYCLE_PERIOD; period++) {
                int required = period * CYCLE_REPETITIONS;
                if (values.size() < required) continue;
                int start = values.size() - required;
                boolean same = true;
                for (int i = start + period; i < values.size(); i++) {
                    if (!values.get(i).cycleKey().equals(values.get(i - period).cycleKey())) {
                        same = false;
                        break;
                    }
                }
                if (same) return period;
            }
            return 0;
        }

        void blockEdge(String scene, String semanticScene, String action, String reason, String activity) {
            boolean added = blockedEdges.add(edgeKey(scene, action));
            added |= blockedSemanticEdges.add(edgeKey(semanticScene, action));
            if (added) emit("edge_blocked", activity, ",\"scene\":\"" + scene + "\",\"action\":\"" + Guard.escape(action) + "\",\"reason\":\"" + reason + "\"");
        }

        boolean isBlocked(String scene, String semanticScene, String action) {
            return blockedEdges.contains(edgeKey(scene, action)) || blockedSemanticEdges.contains(edgeKey(semanticScene, action));
        }

        static String edgeKey(String scene, String action) { return (scene == null ? "" : scene) + "|" + action; }

        Action findPathAction(String start) {
            ArrayDeque<PathNode> queue = new ArrayDeque<>();
            Set<String> seen = new HashSet<>();
            seen.add(start);
            Map<String, String> firstEdges = transitions.get(start);
            if (firstEdges == null) return null;
            SceneState startState = scenes.get(start);
            for (Map.Entry<String, String> edge : firstEdges.entrySet()) {
                Action first = startState.action(edge.getKey());
                if (first != null && !isBlocked(start, startState.cycleId, first.id) && seen.add(edge.getValue())) queue.add(new PathNode(edge.getValue(), first));
            }
            while (!queue.isEmpty()) {
                PathNode current = queue.removeFirst();
                SceneState state = scenes.get(current.scene);
                if (state != null && hasUsableUntried(current.scene, state)) {
                    String route = start + "|" + current.first.id + "|" + current.scene;
                    if (routed.add(route)) return current.first;
                }
                Map<String, String> edges = transitions.get(current.scene);
                if (edges == null || state == null) continue;
                for (Map.Entry<String, String> edge : edges.entrySet()) {
                    Action traversed = state.action(edge.getKey());
                    if (traversed != null && !isBlocked(current.scene, state.cycleId, traversed.id) && seen.add(edge.getValue())) {
                        queue.addLast(new PathNode(edge.getValue(), current.first));
                    }
                }
            }
            return null;
        }

        boolean hasUsableUntried(String scene, SceneState state) {
            for (Action action : state.actions) {
                if (!state.tried.contains(action.id) && !("input".equals(action.type) && inputTried.contains(action.id)) && !isBlocked(scene, state.cycleId, action.id)) return true;
            }
            return false;
        }

        void execute(Action action, String activity, String scene) throws Exception {
            if ("tap".equals(action.type)) {
                input.tap(action.node.centerX(), action.node.centerY());
                emit("node_tap", activity, ",\"scene\":\"" + scene + "\",\"node\":\"" + Guard.escape(action.node.key()) + "\"");
            } else if ("long".equals(action.type)) {
                input.longPress(action.node.centerX(), action.node.centerY());
                emit("node_long_click", activity, ",\"scene\":\"" + scene + "\",\"node\":\"" + Guard.escape(action.node.key()) + "\"");
            } else if ("scroll".equals(action.type)) {
                input.swipe(action.node.centerX(), action.node.bottom()-1, action.node.centerX(), action.node.top()+1);
                emit("node_scroll", activity, ",\"scene\":\"" + scene + "\",\"node\":\"" + Guard.escape(action.node.key()) + "\"");
            } else if ("swipe_left".equals(action.type)) {
                int inset = Math.max(1, (action.node.right - action.node.left) / 5);
                input.swipe(action.node.right - inset, action.node.centerY(), action.node.left + inset, action.node.centerY());
                emit("node_scroll", activity, ",\"scene\":\"" + scene + "\",\"direction\":\"left\",\"node\":\"" + Guard.escape(action.node.key()) + "\"");
            } else if ("input".equals(action.type)) {
                input.tap(action.node.centerX(), action.node.centerY());
                Thread.sleep(100);
                input.keyCombination("113", "29");
                Thread.sleep(100);
                input.key("KEYCODE_DEL");
                if (!action.text.isEmpty()) input.text(config.controlToken, action.text);
                emit("node_input", activity, ",\"scene\":\"" + scene + "\",\"node\":\"" + Guard.escape(action.node.fieldKey()) + "\",\"inputKind\":\"" + action.inputKind + "\",\"inputLength\":" + action.text.codePointCount(0, action.text.length()) + ",\"inputSeed\":" + config.seed);
                Thread.sleep(150);
                input.key("KEYCODE_BACK");
            } else {
                input.key("KEYCODE_BACK");
                emit("dfs_backtrack", activity, ",\"scene\":\"" + scene + "\"");
            }
        }

        void emit(String state, String activity, String extra) {
            System.out.println("{\"state\":\"" + state + "\",\"package\":\"" + config.packageName + "\",\"activity\":\"" + Guard.escape(activity == null ? "" : activity) + "\"" + extra + ",\"time\":" + System.currentTimeMillis() + "}");
        }

        static List<String[]> inputProbes(long seed, String field, int casesPerField, int maxLength) throws Exception {
            String ascii = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
            String digits = "0123456789";
            String cjk = "测试边界中文输入验证内容样例";
            String symbols = "!@#$%^&*()_+-=[]{}:,.?/\\|";
            String emoji = "😀🚀✨🧪✅🌟🌈🎉";
            List<String[]> probes = new ArrayList<>();
            probes.add(new String[]{"ascii_short", generatedText(seed, field, "ascii_short", ascii, Math.min(8, maxLength))});
            probes.add(new String[]{"empty", ""});
            probes.add(new String[]{"whitespace", limitText("   ", maxLength)});
            probes.add(new String[]{"ascii_min", generatedText(seed, field, "ascii_min", ascii, Math.min(1, maxLength))});
            probes.add(new String[]{"ascii_boundary_minus_1", generatedText(seed, field, "ascii_boundary_minus_1", ascii, Math.min(31, maxLength))});
            probes.add(new String[]{"ascii_boundary", generatedText(seed, field, "ascii_boundary", ascii, Math.min(32, maxLength))});
            probes.add(new String[]{"ascii_boundary_plus_1", generatedText(seed, field, "ascii_boundary_plus_1", ascii, Math.min(33, maxLength))});
            probes.add(new String[]{"ascii_max", generatedText(seed, field, "ascii_max", ascii, maxLength)});
            probes.add(new String[]{"cjk", generatedText(seed, field, "cjk", cjk, Math.min(8, maxLength))});
            probes.add(new String[]{"symbols", generatedText(seed, field, "symbols", symbols, Math.min(12, maxLength))});
            probes.add(new String[]{"emoji", generatedText(seed, field, "emoji", emoji, Math.min(4, maxLength))});
            probes.add(new String[]{"mixed", limitText(generatedText(seed, field, "mixed_ascii", ascii, 5) + generatedText(seed, field, "mixed_cjk", cjk, 3) + generatedText(seed, field, "mixed_symbol", symbols, 3) + generatedText(seed, field, "mixed_emoji", emoji, 2), maxLength)});
            probes.add(new String[]{"numeric_zero", limitText("0", maxLength)});
            probes.add(new String[]{"numeric_negative", limitText("-" + generatedText(seed, field, "numeric_negative", digits, 6), maxLength)});
            probes.add(new String[]{"numeric_decimal", limitText(generatedText(seed, field, "numeric_decimal_a", digits, 3) + "." + generatedText(seed, field, "numeric_decimal_b", digits, 4), maxLength)});
            probes.add(new String[]{"email_valid", limitText(generatedText(seed, field, "email_valid", ascii, 8).toLowerCase(Locale.ROOT) + "@example.test", maxLength)});
            probes.add(new String[]{"email_invalid", limitText(generatedText(seed, field, "email_invalid", ascii, 6).toLowerCase(Locale.ROOT) + "..@", maxLength)});
            probes.add(new String[]{"leading_trailing_space", limitText("  " + generatedText(seed, field, "trim", ascii, 8) + "  ", maxLength)});
            return new ArrayList<>(probes.subList(0, Math.min(casesPerField, probes.size())));
        }

        static String generatedText(long seed, String field, String kind, String alphabet, int length) throws Exception {
            int[] values = alphabet.codePoints().toArray();
            StringBuilder result = new StringBuilder();
            for (int index = 0; index < length; index++) {
                byte[] sum = MessageDigest.getInstance("SHA-256").digest((seed + "|" + field + "|" + kind + "|" + index).getBytes(StandardCharsets.UTF_8));
                result.appendCodePoint(values[(sum[0] & 0xff) % values.length]);
            }
            return result.toString();
        }

        static String limitText(String value, int maxLength) {
            int count = value.codePointCount(0, value.length());
            return count <= maxLength ? value : value.substring(0, value.offsetByCodePoints(0, maxLength));
        }

        static void selfTest() throws Exception {
            Config config = new Config();
            config.packageName = "com.example.app";
            String first = "<hierarchy><node index=\"0\" package=\"com.example.app\" class=\"Root\" clickable=\"false\" enabled=\"true\" checkable=\"false\" bounds=\"[0,0][100,100]\"><node index=\"0\" text=\"Item 12\" package=\"com.example.app\" class=\"Button\" clickable=\"true\" enabled=\"true\" checkable=\"false\" bounds=\"[1,1][20,20]\"/></node></hierarchy>";
            String second = first.replace("index=\"0\"", "index=\"9\"").replace("Item 12", "Item 99").replace("[1,1][20,20]", "[2,2][21,21]");
            SceneSnapshot a = SceneSnapshot.parse(config, "com.example.app/.Main", first, false);
            SceneSnapshot b = SceneSnapshot.parse(config, "com.example.app/.Main", second, false);
            if (!a.id.equals(b.id)) throw new AssertionError("volatile text or bounds changed the stable scene id");
            if (a.actions.size() != 1 || !a.actions.get(0).id.startsWith("tap|")) throw new AssertionError("unexpected deterministic actions");
            NodeExplorer explorer = new NodeExplorer(config, new ShellInput());
            SceneState source = new SceneState(a);
            source.tried.add(a.actions.get(0).id);
            explorer.scenes.put(a.id, source);
            explorer.scenes.put("target", new SceneState(new SceneSnapshot("target", 1, a.actions)));
            Map<String, String> edges = new LinkedHashMap<>();
            edges.put(a.actions.get(0).id, "target");
            explorer.transitions.put(a.id, edges);
            Action route = explorer.findPathAction(a.id);
            if (route == null || !route.id.equals(a.actions.get(0).id)) throw new AssertionError("known graph path was not selected");
            NodeExplorer cycles = new NodeExplorer(config, new ShellInput());
            for (int i = 0; i < 3; i++) {
                cycles.recentTransitions.add(new TransitionSample("physical-a-" + i, "semantic-a", "to-b", "physical-b-" + i, "semantic-b"));
                cycles.recentTransitions.add(new TransitionSample("physical-b-" + i, "semantic-b", "to-a", "physical-a-" + i, "semantic-a"));
            }
            if (cycles.repeatedCyclePeriod() != 2) throw new AssertionError("semantic two-state cycle was not detected across physical fingerprint churn");
            NodeExplorer activityCycles = new NodeExplorer(config, new ShellInput());
            for (int i = 0; i < 3; i++) {
                activityCycles.recentTransitions.add(new TransitionSample("main-" + i, "main-semantic-" + i, "pkg/.Main", "open-search", "search", "search-semantic", "pkg/.Search"));
                activityCycles.recentTransitions.add(new TransitionSample("search", "search-semantic", "pkg/.Search", "close-search", "main-" + i, "main-semantic-" + i, "pkg/.Main"));
            }
            if (activityCycles.repeatedCyclePeriod() != 2) throw new AssertionError("cross-activity cycle was hidden by content churn");
            for (int expectedPeriod = 1; expectedPeriod <= 4; expectedPeriod++) {
                NodeExplorer longCycles = new NodeExplorer(config, new ShellInput());
                for (int repeat = 0; repeat < CYCLE_REPETITIONS; repeat++) {
                    for (int position = 0; position < expectedPeriod; position++) longCycles.recentTransitions.add(new TransitionSample("physical-" + repeat + "-" + position, "semantic-" + position, "tap|item-" + position, "physical-" + repeat + "-" + ((position + 1) % expectedPeriod), "semantic-" + ((position + 1) % expectedPeriod)));
                }
                if (longCycles.repeatedCyclePeriod() != expectedPeriod) throw new AssertionError("cycle period " + expectedPeriod + " was not detected");
            }
            EdgeStats nondeterministic = new EdgeStats();
            nondeterministic.observe("destination-b"); nondeterministic.observe("destination-a"); nondeterministic.observe("destination-b");
            if (!"destination-b".equals(nondeterministic.preferred())) throw new AssertionError("multi-destination edge did not retain observations");
            Node financial = Node.parse("<node package=\"com.example.app\" class=\"Button\" text=\"Start trial\" clickable=\"true\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[0,0][100,100]\"/>");
            if (financial == null || !financial.eligible(Collections.<String>emptyList())) throw new AssertionError("financial entry should remain explorable");
            Node consent = Node.parse("<node package=\"com.example.app\" class=\"Button\" text=\"Continue\" clickable=\"true\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[0,0][100,100]\"/>");
            if (consent == null || SpecialHandler.find(Collections.singletonList(consent), SpecialHandler.CONSENT_TARGETS) == null) throw new AssertionError("consent action was not recognized");
            String inputXML = "<hierarchy><node package=\"com.example.app\" class=\"android.widget.EditText\" resource-id=\"com.example.app:id/search\" clickable=\"true\" enabled=\"true\" visible-to-user=\"true\" password=\"false\" bounds=\"[0,0][500,100]\"/></hierarchy>";
            SceneSnapshot inputScene = SceneSnapshot.parse(config, "com.example.app/.Search", inputXML, false);
            if (inputScene.actions.size() != DEFAULT_INPUT_CASES_PER_FIELD || !"input".equals(inputScene.actions.get(0).type) || !"ascii_short".equals(inputScene.actions.get(0).inputKind)) throw new AssertionError("editable field did not create the breadth-first input probe");
            SceneSnapshot shiftedInputScene = SceneSnapshot.parse(config, "com.example.app/.Search", inputXML.replace("[0,0][500,100]", "[0,300][500,400]"), false);
            if (!inputScene.actions.get(0).id.equals(shiftedInputScene.actions.get(0).id)) throw new AssertionError("viewport movement changed anchored input identity");
            String autoCompleteXML = inputXML.replace("android.widget.EditText", "android.widget.AutoCompleteTextView");
            SceneSnapshot autoCompleteScene = SceneSnapshot.parse(config, "com.example.app/.Search", autoCompleteXML, false);
            if (autoCompleteScene.actions.size() != DEFAULT_INPUT_CASES_PER_FIELD || !"input".equals(autoCompleteScene.actions.get(0).type)) throw new AssertionError("auto-complete field was not treated as editable");
            List<String[]> generated = inputProbes(42, "search-field", MAX_INPUT_CASES_PER_FIELD, DEFAULT_INPUT_MAX_LENGTH);
            List<String[]> repeated = inputProbes(42, "search-field", MAX_INPUT_CASES_PER_FIELD, DEFAULT_INPUT_MAX_LENGTH);
            List<String[]> changed = inputProbes(43, "search-field", MAX_INPUT_CASES_PER_FIELD, DEFAULT_INPUT_MAX_LENGTH);
            boolean seedChanged = false;
            Map<String,Integer> inputLengths = new HashMap<>();
            for (int i = 0; i < generated.size(); i++) {
                if (!generated.get(i)[1].equals(repeated.get(i)[1])) throw new AssertionError("same input seed was not reproducible");
                if (!generated.get(i)[1].equals(changed.get(i)[1])) seedChanged = true;
                inputLengths.put(generated.get(i)[0], generated.get(i)[1].codePointCount(0, generated.get(i)[1].length()));
            }
            if (!seedChanged || inputLengths.get("empty") != 0 || inputLengths.get("ascii_min") != 1 || inputLengths.get("ascii_boundary_minus_1") != 31 || inputLengths.get("ascii_boundary") != 32 || inputLengths.get("ascii_boundary_plus_1") != 33 || inputLengths.get("ascii_max") != 64) throw new AssertionError("generated input equivalence classes were incomplete");
            String passwordXML = inputXML.replace("search", "password").replace("password=\"false\"", "password=\"true\"");
            if (!SceneSnapshot.parse(config, "com.example.app/.Login", passwordXML, false).actions.isEmpty()) throw new AssertionError("password field was not filtered");
            String invalidXML = "<hierarchy><node package=\"com.example.app\" class=\"Root\" enabled=\"true\" bounds=\"[0,0][500,500]\"><node package=\"com.example.app\" class=\"Button\" clickable=\"true\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[0,0][0,0]\"/></node></hierarchy>";
            if (!SceneSnapshot.parse(config, "com.example.app/.Main", invalidXML, false).actions.isEmpty()) throw new AssertionError("zero-area node became an action");
            String onboardingXML = "<hierarchy><node package=\"com.example.app\" class=\"android.view.View\" scrollable=\"true\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[0,0][500,1000]\">" +
                    "<node package=\"com.example.app\" class=\"android.widget.TextView\" text=\"Page One\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[100,100][400,200]\"/>" +
                    "<node package=\"com.example.app\" class=\"android.view.View\" clickable=\"true\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[20,800][480,950]\">" +
                    "<node package=\"com.example.app\" class=\"android.widget.TextView\" text=\"Continue\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[180,840][320,910]\"/></node></node></hierarchy>";
            SceneSnapshot onboardingContinue = SceneSnapshot.parse(config, "com.example.app/.Onboarding", onboardingXML, false);
            SceneSnapshot onboardingStarted = SceneSnapshot.parse(config, "com.example.app/.Onboarding", onboardingXML.replace("Continue", "Get Started"), false);
            Action continueAction = null, leftSwipe = null;
            for (Action action : onboardingContinue.actions) {
                if (action.isOnboardingAdvance()) continueAction = action;
                if ("swipe_left".equals(action.type)) leftSwipe = action;
            }
            if (continueAction == null || leftSwipe == null || onboardingContinue.cycleId.equals(onboardingStarted.cycleId)) throw new AssertionError("onboarding actions did not retain descendant labels or horizontal fallback");
            NodeExplorer onboardingExplorer = new NodeExplorer(config, new ShellInput());
            SceneState onboardingState = new SceneState(onboardingContinue);
            if (onboardingExplorer.nextUntried(onboardingState, onboardingContinue.id, onboardingContinue.semanticId, onboardingContinue.cycleId, "com.example.app/.Onboarding") != continueAction) throw new AssertionError("onboarding advance action was not prioritized");
            String[] onboardingTitles = {"Page One", "Page Two", "Page Three", "Page Four", "Page Five"};
            SceneSnapshot[] onboardingPages = new SceneSnapshot[onboardingTitles.length];
            NodeExplorer multiPageOnboarding = new NodeExplorer(config, new ShellInput());
            String sharedCycle = null;
            for (int index = 0; index < onboardingTitles.length; index++) {
                onboardingPages[index] = SceneSnapshot.parse(config, "com.example.app/.Onboarding",
                        onboardingXML.replace("Page One", onboardingTitles[index]), false);
                if (sharedCycle == null) sharedCycle = onboardingPages[index].cycleId;
                if (!sharedCycle.equals(onboardingPages[index].cycleId)) throw new AssertionError("same-action onboarding pages did not reproduce a shared cycle identity");
                SceneState pageState = new SceneState(onboardingPages[index]);
                Action pageAdvance = multiPageOnboarding.nextUntried(pageState, onboardingPages[index].id,
                        onboardingPages[index].semanticId, onboardingPages[index].cycleId, "com.example.app/.Onboarding");
                if (pageAdvance == null || !pageAdvance.isOnboardingAdvance()) throw new AssertionError("same-label onboarding page was suppressed at index " + index);
                multiPageOnboarding.markExecuted(pageState, onboardingPages[index].cycleId, "com.example.app/.Onboarding", pageAdvance);
            }
            SceneState repeatedOnboardingPage = new SceneState(onboardingPages[2]);
            Action repeatedPageAction = multiPageOnboarding.nextUntried(repeatedOnboardingPage, onboardingPages[2].id,
                    onboardingPages[2].semanticId, onboardingPages[2].cycleId, "com.example.app/.Onboarding");
            if (repeatedPageAction != null && repeatedPageAction.isOnboardingAdvance()) throw new AssertionError("same semantic onboarding page was clicked twice");
            NodeExplorer inheritedOnboarding = new NodeExplorer(config, new ShellInput());
            inheritedOnboarding.inheritHistory(multiPageOnboarding);
            SceneState inheritedPage = new SceneState(onboardingPages[2]);
            Action inheritedAction = inheritedOnboarding.nextUntried(inheritedPage, onboardingPages[2].id,
                    onboardingPages[2].semanticId, onboardingPages[2].cycleId, "com.example.app/.Onboarding");
            if (inheritedAction != null && inheritedAction.isOnboardingAdvance()) throw new AssertionError("normal generation forgot completed onboarding page");
            NodeExplorer recoveredOnboarding = new NodeExplorer(config, new ShellInput());
            recoveredOnboarding.inheritRecoveryHistory(multiPageOnboarding);
            SceneState recoveredPage = new SceneState(onboardingPages[0]);
            Action recoveredAdvance = recoveredOnboarding.nextUntried(recoveredPage, onboardingPages[0].id,
                    onboardingPages[0].semanticId, onboardingPages[0].cycleId, "com.example.app/.Onboarding");
            if (recoveredAdvance == null || !recoveredAdvance.isOnboardingAdvance()) throw new AssertionError("recovery generation did not release onboarding page budget");
            if (!hasOnboardingAdvance(onboardingState.actions)) throw new AssertionError("onboarding exhaustion guard lost scene classification");
            NodeExplorer exhaustedOnboarding = new NodeExplorer(config, new ShellInput());
            SceneState exhaustedPage = new SceneState(onboardingPages[0]);
            for (Action action : exhaustedPage.actions) {
                exhaustedPage.tried.add(action.id);
                if (action.isScrollGesture()) exhaustedPage.attempts.put(action.id, MAX_STABLE_SCROLL_ATTEMPTS);
            }
            exhaustedOnboarding.scenes.put(onboardingPages[0].id, exhaustedPage);
            if (exhaustedOnboarding.act("com.example.app/.Onboarding", false, onboardingXML) != ActResult.EXHAUSTED ||
                    !exhaustedOnboarding.isOnboardingExhaustion()) throw new AssertionError("onboarding exhaustion did not request a preserve-in-place stop");
            String onboardingTransitionXML = "<hierarchy><node package=\"com.example.app\" class=\"android.widget.FrameLayout\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[0,0][500,1000]\"/></hierarchy>";
            NodeExplorer onboardingTransition = new NodeExplorer(config, new ShellInput());
            onboardingTransition.onboardingActivity = "com.example.app/.Onboarding";
            if (onboardingTransition.act("com.example.app/.Onboarding", false, onboardingTransitionXML) != ActResult.WAITING)
                throw new AssertionError("onboarding transition frame did not wait");
            onboardingTransition.onboardingTransitionStartedAt = System.currentTimeMillis() - MAX_ONBOARDING_TRANSITION_WAIT_MILLIS - 1L;
            if (onboardingTransition.act("com.example.app/.Onboarding", false, onboardingTransitionXML) != ActResult.EXHAUSTED ||
                    !onboardingTransition.isOnboardingExhaustion() ||
                    !"transition_timeout".equals(onboardingTransition.onboardingExhaustionReason())) {
                throw new AssertionError("onboarding transition timeout did not stop in place");
            }
            String unsafeOnboardingXML = onboardingXML.replace("Continue", "Download now");
            for (Action action : SceneSnapshot.parse(config, "com.example.app/.Onboarding", unsafeOnboardingXML, false).actions) {
                if ("tap".equals(action.type)) throw new AssertionError("descendant action label bypassed the safety policy");
            }
            String surfaceXML = "<hierarchy><node package=\"com.example.app\" class=\"android.widget.FrameLayout\" enabled=\"true\" bounds=\"[0,0][500,500]\"><node package=\"com.example.app\" class=\"android.view.SurfaceView\" clickable=\"false\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[0,0][500,500]\"/></node></hierarchy>";
            SceneSnapshot surfaceScene = SceneSnapshot.parse(config, "com.example.app/.Unity", surfaceXML, false);
            if (!surfaceScene.renderSurface || !surfaceScene.actions.isEmpty()) throw new AssertionError("render-only SurfaceView scene was not recognized");
            if (new NodeExplorer(config, new ShellInput()).act("com.example.app/.Unity", false, surfaceXML) != ActResult.ERROR) throw new AssertionError("render-only SurfaceView scene did not request coordinate fallback");
            String opaqueUnityXML = "<hierarchy><node package=\"com.example.app\" class=\"android.widget.FrameLayout\" enabled=\"true\" bounds=\"[0,0][500,500]\"><node package=\"com.example.app\" class=\"android.view.View\" clickable=\"false\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[0,0][500,500]\"/></node></hierarchy>";
            if (new NodeExplorer(config, new ShellInput()).act("com.example.app/com.unity3d.player.UnityPlayerActivity", false, opaqueUnityXML) != ActResult.ERROR) throw new AssertionError("opaque Unity scene did not request coordinate fallback");
            if (new NodeExplorer(config, new ShellInput()).act("com.example.app/.StaticActivity", false, opaqueUnityXML) != ActResult.EXHAUSTED) throw new AssertionError("ordinary static scene incorrectly requested coordinate fallback");
            String unityMenuXML = "<hierarchy><node package=\"com.example.app\" class=\"Root\" enabled=\"true\" bounds=\"[0,0][500,500]\">" +
                    "<node package=\"com.example.app\" class=\"Button\" text=\"Play\" clickable=\"true\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[0,0][100,100]\"/></node></hierarchy>";
            SceneSnapshot unityMenu = SceneSnapshot.parse(config, "com.example.app/com.unity3d.player.UnityPlayerActivity", unityMenuXML, false);
            NodeExplorer unityCycle = new NodeExplorer(config, new ShellInput());
            SceneState unityState = new SceneState(unityMenu);
            unityCycle.scenes.put(unityMenu.id, unityState);
            unityState.tried.add(unityMenu.actions.get(0).id);
            unityCycle.blockedSemanticEdges.add(edgeKey(unityMenu.semanticId, unityMenu.actions.get(0).id));
            unityCycle.lastExhaustedSemanticId = unityMenu.semanticId;
            if (unityCycle.recoverContinuousRenderExhaustion() != UnityExhaustionRecovery.RELEASE)
                throw new AssertionError("first unity exhaustion did not receive one replay");
            if (!unityState.tried.isEmpty() || unityCycle.blockedSemanticEdges.contains(edgeKey(unityMenu.semanticId, unityMenu.actions.get(0).id)))
                throw new AssertionError("unity replay did not reopen overlay actions");
            unityState.tried.add(unityMenu.actions.get(0).id);
            unityCycle.lastExhaustedSemanticId = unityMenu.semanticId;
            if (unityCycle.recoverContinuousRenderExhaustion() != UnityExhaustionRecovery.BACK)
                throw new AssertionError("second unity exhaustion did not request active Back recovery");
            NodeExplorer inheritedUnityCycle = new NodeExplorer(config, new ShellInput());
            inheritedUnityCycle.inheritHistory(unityCycle);
            inheritedUnityCycle.lastExhaustedSemanticId = unityMenu.semanticId;
            if (inheritedUnityCycle.recoverContinuousRenderExhaustion() != UnityExhaustionRecovery.FALLTHROUGH)
                throw new AssertionError("unity recovery budget did not survive generation replacement");
            if (new NodeExplorer(config, new ShellInput()).act("com.example.app/.Broken", false, null) != ActResult.OBSERVATION_ERROR) throw new AssertionError("invalid hierarchy incorrectly authorized coordinate fallback");
            String surfaceButtonXML = surfaceXML.replace("</node></hierarchy>", "<node package=\"com.example.app\" class=\"android.widget.Button\" text=\"Pause\" clickable=\"true\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[10,10][100,80]\"/></node></hierarchy>");
            if (SceneSnapshot.parse(config, "com.example.app/.Unity", surfaceButtonXML, false).actions.size() != 1) throw new AssertionError("actionable overlay on a SurfaceView was discarded");
            String viewportA = "<hierarchy><node package=\"com.example.app\" class=\"Root\" enabled=\"true\" bounds=\"[0,0][500,500]\"><node package=\"com.example.app\" class=\"TextView\" text=\"Alpha item\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[0,100][500,200]\"/></node></hierarchy>";
            String viewportB = viewportA.replace("Alpha item", "Beta item");
            if (SceneSnapshot.parse(config, "com.example.app/.Main", viewportA, false).viewportId.equals(SceneSnapshot.parse(config, "com.example.app/.Main", viewportB, false).viewportId)) throw new AssertionError("viewport content change was not detected");
            NodeExplorer fairness = new NodeExplorer(config, new ShellInput());
            List<Action> fairActions = Arrays.asList(new Action("tap", a.actions.get(0).node), new Action("scroll", a.actions.get(0).node));
            SceneState fairState = new SceneState(new SceneSnapshot("fair", "fair-semantic", "fair-cycle", 1, fairActions));
            fairState.nonScrollSelections = MAX_NON_SCROLL_BEFORE_SCROLL;
            Action fair = fairness.nextUntried(fairState, "fair", "fair-semantic", "fair-cycle", "com.example.app/.Main");
            if (fair == null || !"scroll".equals(fair.type)) throw new AssertionError("scroll fairness quota was not enforced");
            SceneState stableScroll = new SceneState(new SceneSnapshot("stable", "stable-semantic", "stable-cycle", 1, Collections.singletonList(new Action("scroll", a.actions.get(0).node))));
            for (int i = 0; i < MAX_STABLE_SCROLL_ATTEMPTS; i++) {
                Action selected = fairness.nextUntried(stableScroll, "stable", "stable-semantic", "stable-cycle", "com.example.app/.Main");
                if (selected == null) throw new AssertionError("stable scroll stopped before its bounded retry budget");
                fairness.markExecuted(stableScroll, "stable-cycle", "com.example.app/.Main", selected);
            }
            if (fairness.nextUntried(stableScroll, "stable", "stable-semantic", "stable-cycle", "com.example.app/.Main") != null) throw new AssertionError("stable scroll exceeded its bounded retry budget");
            NodeExplorer firstCycle = new NodeExplorer(config, new ShellInput());
            SceneState dynamicFirst = new SceneState(new SceneSnapshot("physical-a", "semantic-a", "shared-cycle", 1, Collections.singletonList(a.actions.get(0))));
            firstCycle.markExecuted(dynamicFirst, "shared-cycle", "com.example.app/.Main", a.actions.get(0));
            NodeExplorer restartedCycle = new NodeExplorer(config, new ShellInput());
            restartedCycle.inheritHistory(firstCycle);
            SceneState dynamicSecond = new SceneState(new SceneSnapshot("physical-b", "semantic-b", "changed-cycle", 1, Collections.singletonList(a.actions.get(0))));
            Action dynamicRetry = restartedCycle.nextUntried(dynamicSecond, "physical-b", "semantic-b", "changed-cycle", "com.example.app/.Main");
            if (dynamicRetry == null) throw new AssertionError("adaptive scheduler removed all probability after one attempt");
            restartedCycle.markExecuted(dynamicSecond, "changed-cycle", "com.example.app/.Main", dynamicRetry);
            SceneState dynamicThird = new SceneState(new SceneSnapshot("physical-c", "semantic-c", "third-cycle", 1, Collections.singletonList(a.actions.get(0))));
            restartedCycle.markExecuted(dynamicThird, "third-cycle", "com.example.app/.Main", a.actions.get(0));
            SceneState dynamicFourth = new SceneState(new SceneSnapshot("physical-d", "semantic-d", "fourth-cycle", 1, Collections.singletonList(a.actions.get(0))));
            if (restartedCycle.nextUntried(dynamicFourth, "physical-d", "semantic-d", "fourth-cycle", "com.example.app/.Main") != null) throw new AssertionError("bounded repeat guard allowed an Activity action indefinitely");
            restartedCycle.blockedSemanticEdges.add(edgeKey("blocked-cycle", "tap|danger"));
            restartedCycle.inputTried.add("input|sensitive-once");
            NodeExplorer recoveredCycle = new NodeExplorer(config, new ShellInput());
            recoveredCycle.inheritRecoveryHistory(restartedCycle);
            SceneState recoveredState = new SceneState(new SceneSnapshot("physical-recovered", "semantic-recovered", "recovered-cycle", 1, Collections.singletonList(a.actions.get(0))));
            if (recoveredCycle.nextUntried(recoveredState, "physical-recovered", "semantic-recovered", "recovered-cycle", "com.example.app/.Main") == null) throw new AssertionError("recovery generation did not refresh the direct-action budget");
            if (!recoveredCycle.blockedSemanticEdges.contains(edgeKey("blocked-cycle", "tap|danger")) || !recoveredCycle.inputTried.contains("input|sensitive-once")) throw new AssertionError("recovery generation discarded safety history");
            String breadthXML = "<hierarchy><node package=\"com.example.app\" class=\"Root\" enabled=\"true\" bounds=\"[0,0][500,500]\">" +
                    "<node package=\"com.example.app\" class=\"Button\" resource-id=\"com.example.app:id/enter_pointswall_ic\" clickable=\"true\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[0,0][100,100]\"/>" +
                    "<node package=\"com.example.app\" class=\"Button\" resource-id=\"com.example.app:id/search\" clickable=\"true\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[100,0][200,100]\"/></node></hierarchy>";
            SceneSnapshot breadth = SceneSnapshot.parse(config, "com.example.app/.Main", breadthXML, false);
            if (breadth.actions.size() != 2) throw new AssertionError("adaptive candidates were unexpectedly filtered");
            NodeExplorer stochastic = new NodeExplorer(config, new ShellInput());
            SceneState breadthState = new SceneState(breadth);
            Action firstBreadth = stochastic.nextUntried(breadthState, breadth.id, breadth.semanticId, breadth.cycleId, "com.example.app/.Main");
            stochastic.markExecuted(breadthState, breadth.cycleId, "com.example.app/.Main", firstBreadth);
            Action secondBreadth = stochastic.nextUntried(breadthState, breadth.id, breadth.semanticId, breadth.cycleId, "com.example.app/.Main");
            if (secondBreadth == null || firstBreadth.id.equals(secondBreadth.id)) throw new AssertionError("seeded stochastic selection did not retain candidate diversity");
            SceneSnapshot paywallBreadth = SceneSnapshot.parse(config, "com.example.app/.Main", breadthXML, true);
            if (paywallBreadth.actions.size() != 2) throw new AssertionError("paywall context suppressed ordinary or monetization exploration actions");
            System.out.println("NodeExplorer self-test passed");
        }

        static final class SceneSnapshot {
            final String id;
            final String semanticId;
            final String cycleId;
            final String viewportId;
            final int nodeCount;
            final List<Action> actions;
            final boolean renderSurface;

            SceneSnapshot(String id, int nodeCount, List<Action> actions) {
                this(id, id, id, id, nodeCount, actions, false);
            }

            SceneSnapshot(String id, String semanticId, String cycleId, int nodeCount, List<Action> actions) {
                this(id, semanticId, cycleId, cycleId, nodeCount, actions, false);
            }

            SceneSnapshot(String id, String semanticId, String cycleId, String viewportId, int nodeCount, List<Action> actions) {
                this(id, semanticId, cycleId, viewportId, nodeCount, actions, false);
            }

            SceneSnapshot(String id, String semanticId, String cycleId, String viewportId, int nodeCount, List<Action> actions, boolean renderSurface) {
                this.id = id;
                this.semanticId = semanticId;
                this.cycleId = cycleId;
                this.viewportId = viewportId;
                this.nodeCount = nodeCount;
                this.actions = actions;
                this.renderSurface = renderSurface;
            }

            static SceneSnapshot parse(Config config, String activity, String xml, boolean paywallContext) throws Exception {
                List<String> structure = new ArrayList<>(), semanticStructure = new ArrayList<>(), viewportStructure = new ArrayList<>();
                List<Node> allNodes = new ArrayList<>(), clicks = new ArrayList<>(), longClicks = new ArrayList<>(), inputs = new ArrayList<>(), scrolls = new ArrayList<>();
                Matcher tags = NODE_TAG.matcher(xml);
                int depth = 0, nodeCount = 0, screenRight = 0, screenBottom = 0;
                boolean renderSurface = false;
                while (tags.find()) {
                    String tag = tags.group();
                    if (tag.startsWith("</")) {
                        depth = Math.max(0, depth - 1);
                        continue;
                    }
                    Node node = Node.parse(tag);
                    boolean closes = tag.endsWith("/>");
                    if (node != null && config.packageName.equals(node.value("package"))) {
                        allNodes.add(node);
                        nodeCount++;
                        renderSurface = renderSurface || node.isRenderSurface();
                        if (depth == 0) {
                            screenRight = Math.max(screenRight, node.right);
                            screenBottom = Math.max(screenBottom, node.bottom());
                        }
                        structure.add(depth + "|" + node.stableFields());
                        semanticStructure.add(node.semanticFields());
                        if (node.validArea() && (node.hasViewportAnchor() || "true".equals(node.value("scrollable")))) viewportStructure.add(node.viewportFields());
                        if (node.eligible(config.blockedControls)) {
                            if (node.editable() && node.safeInput()) inputs.add(node);
                            if (!node.editable() && "true".equals(node.value("clickable")) && !"android.webkit.WebView".equals(node.value("class"))) clicks.add(node);
                            if (!node.editable() && "true".equals(node.value("long-clickable"))) longClicks.add(node);
                            if ("true".equals(node.value("scrollable"))) scrolls.add(node);
                        }
                    }
                    if (!closes) depth++;
                }
                List<Node> contextualClicks = new ArrayList<>();
                for (Node node : clicks) {
                    Node contextual = node.withDescendantActionLabel(allNodes);
                    if (contextual.eligible(config.blockedControls)) contextualClicks.add(contextual);
                }
                clicks = contextualClicks;
                Comparator<Node> order = Comparator.comparingInt(Node::priority).reversed().thenComparing(Node::key);
                clicks.sort(order);
                longClicks.sort(order);
                inputs.sort(order);
                scrolls.sort(order);
                filterInvalidActions(clicks, screenRight, screenBottom);
                filterInvalidActions(longClicks, screenRight, screenBottom);
                filterInvalidActions(inputs, screenRight, screenBottom);
                filterInvalidActions(scrolls, screenRight, screenBottom);
                for (Iterator<Node> iterator = scrolls.iterator(); iterator.hasNext();) {
                    Node node = iterator.next();
                    if ((node.bottom() - node.top()) * 4 < screenBottom) iterator.remove();
                }
                List<Action> actions = new ArrayList<>();
                for (Node node : inputs) for (String[] probe : inputProbes(config.seed, node.fieldKey(), config.inputCasesPerField, config.inputMaxLength)) actions.add(Action.input(activity, node, probe[0], probe[1]));
                for (Node node : clicks) actions.add(new Action("tap", node));
                for (Node node : longClicks) actions.add(new Action("long", node));
                for (Node node : scrolls) actions.add(new Action("scroll", node));
                boolean onboarding = false;
                for (Node node : clicks) onboarding |= node.isOnboardingAdvance();
                if (onboarding) for (Node node : scrolls) actions.add(new Action("swipe_left", node));
                Map<String, Action> uniqueActions = new LinkedHashMap<>();
                for (Action action : actions) uniqueActions.putIfAbsent(action.id, action);
                actions = new ArrayList<>(uniqueActions.values());
                List<String> semanticActions = new ArrayList<>();
                for (Action action : actions) semanticActions.add(action.id);
                Collections.sort(semanticActions);
                Collections.sort(semanticStructure);
                Collections.sort(viewportStructure);
                String activityValue = activity == null ? "" : activity;
                return new SceneSnapshot(digest(activityValue + "\n" + String.join("\n", structure)), digest(activityValue + "\n" + String.join("\n", semanticStructure) + "\n" + String.join("\n", semanticActions)), digest(activityValue + "\n" + String.join("\n", semanticActions)), digest(activityValue + "\n" + String.join("\n", viewportStructure)), nodeCount, actions, renderSurface);
            }

            static void filterInvalidActions(List<Node> nodes, int screenRight, int screenBottom) {
                for (Iterator<Node> iterator = nodes.iterator(); iterator.hasNext();) if (!iterator.next().actionableBounds(screenRight, screenBottom)) iterator.remove();
            }

            static String digest(String value) throws Exception {
                byte[] sum = MessageDigest.getInstance("SHA-256").digest(value.getBytes(StandardCharsets.UTF_8));
                StringBuilder result = new StringBuilder();
                for (int i = 0; i < 12; i++) result.append(String.format(Locale.ROOT, "%02x", sum[i]));
                return result.toString();
            }
        }

        static final class SceneState {
            final List<Action> actions;
            final String semanticId;
            final String cycleId;
            final Set<String> tried = new HashSet<>();
            final Map<String,Integer> attempts = new HashMap<>();
            int nonScrollSelections;

            SceneState(SceneSnapshot snapshot) { actions = snapshot.actions; semanticId = snapshot.semanticId; cycleId = snapshot.cycleId; }
            boolean hasUntried() { for (Action action : actions) if (!tried.contains(action.id)) return true; return false; }
            Action action(String id) { if ("back".equals(id)) return Action.back(); for (Action action : actions) if (action.id.equals(id)) return action; return null; }
        }

        static final class Action {
            final String type, id, inputKind, text;
            final Node node;
            Action(String type, Node node) { this(type, node, "", "", type + "|" + node.key()); }
            Action(String type, Node node, String inputKind, String text, String id) { this.type=type; this.node=node; this.inputKind=inputKind; this.text=text; this.id=id; }
            static Action input(String activity, Node node, String kind, String text) { return new Action("input", node, kind, text, "input|" + activity + "|" + kind + "|" + node.fieldKey()); }
            boolean isScrollGesture() { return "scroll".equals(type) || "swipe_left".equals(type); }
            boolean isOnboardingAdvance() { return "tap".equals(type) && node != null && node.isOnboardingAdvance(); }
            private Action() { type = "back"; id = "back"; node = null; inputKind = ""; text = ""; }
            static Action back() { return new Action(); }
        }

        static final class PathNode {
            final String scene;
            final Action first;
            PathNode(String scene, Action first) { this.scene = scene; this.first = first; }
        }

        static final class EdgeStats {
            final Map<String, Integer> destinations = new LinkedHashMap<>();
            void observe(String destination) { destinations.put(destination, destinations.getOrDefault(destination, 0) + 1); }
            String preferred() {
                String selected = null;
                int best = -1;
                for (Map.Entry<String, Integer> entry : destinations.entrySet()) {
                    if (entry.getValue() > best || (entry.getValue() == best && (selected == null || entry.getKey().compareTo(selected) < 0))) {
                        selected = entry.getKey();
                        best = entry.getValue();
                    }
                }
                return selected;
            }
        }

        static final class TransitionSample {
            final String from, semanticFrom, activityFrom, action, to, semanticTo, activityTo;
            TransitionSample(String from, String semanticFrom, String action, String to, String semanticTo) {
                this(from, semanticFrom, "", action, to, semanticTo, "");
            }
            TransitionSample(String from, String semanticFrom, String activityFrom, String action, String to, String semanticTo, String activityTo) {
                this.from = from; this.semanticFrom = semanticFrom; this.activityFrom = activityFrom; this.action = action; this.to = to; this.semanticTo = semanticTo; this.activityTo = activityTo;
            }
            String cycleKey() {
                String fromKey = !activityFrom.isEmpty() ? "activity:" + activityFrom : "scene:" + semanticFrom;
                String toKey = !activityTo.isEmpty() ? "activity:" + activityTo : "scene:" + semanticTo;
                return fromKey + "|" + semanticAction(action) + "|" + toKey;
            }
            static String semanticAction(String value) { return value.replaceAll("\\[\\d+,\\d+\\]\\[\\d+,\\d+\\]", "<bounds>").replaceAll("\\d+", "#"); }
        }

        static final class Node {
            final Map<String,String> values; final int left,top,right,bottom;
            Node(Map<String,String> values,int left,int top,int right,int bottom){this.values=values;this.left=left;this.top=top;this.right=right;this.bottom=bottom;}
            static Node parse(String raw){Map<String,String> values=new HashMap<>();Matcher attributes=ATTRIBUTE.matcher(raw);while(attributes.find())values.put(attributes.group(1),attributes.group(2));String encodedBounds=values.get("bounds");if(encodedBounds==null)return null;Matcher bounds=BOUNDS.matcher(encodedBounds);if(!bounds.matches())return null;return new Node(values,Integer.parseInt(bounds.group(1)),Integer.parseInt(bounds.group(2)),Integer.parseInt(bounds.group(3)),Integer.parseInt(bounds.group(4)));}
            String value(String key){String value=values.get(key);return value==null?"":value;}
            String key(){String resource=value("resource-id"),type=value("class"),label="";if(type.endsWith("Button")&&value("text").length()<10)label=value("text");else if(resource.isEmpty())label=actionLabel();String position=resource.isEmpty()&&label.isEmpty()?value("bounds"):"";return resource+"|"+type+"|"+value("clickable")+"|"+value("enabled")+"|"+value("checkable")+"|"+label.replaceAll("\\d+","#")+"|"+position;}
            String stableFields(){return value("resource-id")+"|"+value("class")+"|"+value("clickable")+"|"+value("enabled")+"|"+value("checkable")+"|"+value("scrollable")+"|"+stableLabel()+"|"+(top/512);}
            String semanticFields(){return value("resource-id")+"|"+value("class")+"|"+value("clickable")+"|"+value("scrollable")+"|"+stableLabel();}
            String stableLabel(){String label=editable()?(!value("content-desc").isEmpty()?value("content-desc"):value("resource-id")):(!value("content-desc").isEmpty()?value("content-desc"):value("text"));return label.toLowerCase(Locale.ROOT).trim().replaceAll("\\d+","#");}
            String viewportFields(){return value("resource-id")+"|"+value("class")+"|"+stableLabel()+"|"+(top/256)+"|"+(bottom/256);}
            boolean hasViewportAnchor(){return !value("resource-id").isEmpty()||!stableLabel().isEmpty();}
            boolean validArea(){return right>left&&bottom>top;}
            boolean actionableBounds(int screenRight,int screenBottom){if(!validArea()||screenRight<=0||screenBottom<=0)return false;int x=(left+right)/2,y=(top+bottom)/2;return left<screenRight&&top<screenBottom&&right>0&&bottom>0&&x>=0&&x<screenRight&&y>=0&&y<screenBottom;}
            boolean eligible(List<String> blockedControls){if(!"true".equals(value("enabled"))||"false".equals(value("visible-to-user"))||"true".equals(value("password"))||AdScenePolicy.isEmbeddedNode(this)||SafetyActionPolicy.blocksGenericAction(value("text")+" "+value("xtest-action-label"),value("content-desc"),value("resource-id")))return false;String candidate=(value("text")+" "+value("content-desc")+" "+value("resource-id")+" "+value("xtest-action-label")).toLowerCase(Locale.ROOT);for(String blocked:blockedControls)if(candidate.contains(blocked.toLowerCase(Locale.ROOT)))return false;return true;}
            boolean editable(){String type=value("class").toLowerCase(Locale.ROOT);return type.contains("edittext")||type.contains("autocompletetextview");}
            boolean isRenderSurface(){String type=value("class").toLowerCase(Locale.ROOT);return type.endsWith("surfaceview")||type.endsWith("glsurfaceview")||type.endsWith("textureview");}
            boolean safeInput(){String candidate=(value("text")+" "+value("content-desc")+" "+value("resource-id")).toLowerCase(Locale.ROOT);for(String denied:SENSITIVE_INPUTS)if(candidate.contains(denied))return false;return !"true".equals(value("password"));}
            String actionLabel(){String label=!value("content-desc").isEmpty()?value("content-desc"):value("text");return !label.isEmpty()?label:value("xtest-action-label");}
            boolean isOnboardingAdvance(){String label=actionLabel().toLowerCase(Locale.ROOT).trim();return "continue".equals(label)||"next".equals(label)||"get started".equals(label)||"start exploring".equals(label);}
            Node withDescendantActionLabel(List<Node> nodes){
                if(!value("resource-id").isEmpty()||!actionLabel().isEmpty())return this;
                Node selected=null;long selectedArea=Long.MAX_VALUE;
                for(Node candidate:nodes){
                    if(candidate==this||candidate.left<left||candidate.top<top||candidate.right>right||candidate.bottom>bottom)continue;
                    String label=!candidate.value("content-desc").isEmpty()?candidate.value("content-desc"):candidate.value("text");
                    if(label.trim().isEmpty())continue;
                    long area=(long)(candidate.right-candidate.left)*(candidate.bottom-candidate.top);
                    if(area<selectedArea){selected=candidate;selectedArea=area;}
                }
                if(selected==null)return this;
                Map<String,String> contextual=new HashMap<>(values);
                contextual.put("xtest-action-label",!selected.value("content-desc").isEmpty()?selected.value("content-desc"):selected.value("text"));
                return new Node(contextual,left,top,right,bottom);
            }
            String fieldKey(){String anchor=!value("resource-id").isEmpty()?value("resource-id"):value("content-desc");return !anchor.isEmpty()?value("resource-id")+"|"+value("content-desc")+"|"+value("class")+"|"+anchor.toLowerCase(Locale.ROOT).trim():value("class")+"|"+left+","+top;}
            int priority(){int value=0;if(!value("resource-id").isEmpty())value+=4;if(!value("text").isEmpty()||!value("content-desc").isEmpty())value+=2;return value;}
            int centerX(){return Math.max(left,(left+right)/2);}int centerY(){return Math.max(top,(top+bottom)/2);}int top(){return top;}int bottom(){return bottom;}
        }
    }
    private static void log(String state, Config config, long count) {
        System.out.println("{\"state\":\""+state+"\",\"requestId\":\""+Guard.escape(config.requestId)+"\",\"package\":\""+config.packageName+"\",\"seed\":"+config.seed+",\"events\":"+count+",\"time\":"+System.currentTimeMillis()+"}");
    }
    static final class Config {
        String packageName, requestId = "", stopFile = "/data/local/tmp/.xtest-nova-monkey.stop", activityMode = "none", renderFallbackMode = "bounded", targetToken = "", controlToken = "";
        int durationSeconds = 600, throttleMillis = 500;
        int minBattery = 15;
        int inputCasesPerField = NodeExplorer.DEFAULT_INPUT_CASES_PER_FIELD, inputMaxLength = NodeExplorer.DEFAULT_INPUT_MAX_LENGTH;
        boolean lowBatteryExit;
        final List<String> activities = new ArrayList<>(), targetActivities = new ArrayList<>(), blockedControls = new ArrayList<>(), targetCases = new ArrayList<>();
        long seed = System.currentTimeMillis();
        static Config parse(String[] args) {
            Config c=new Config();
            for(int i=0;i<args.length;i++){String arg=args[i];if("-p".equals(arg))c.packageName=next(args,++i,arg);else if("--request-id".equals(arg))c.requestId=next(args,++i,arg);else if("--duration-seconds".equals(arg))c.durationSeconds=Integer.parseInt(next(args,++i,arg));else if("--throttle".equals(arg))c.throttleMillis=Integer.parseInt(next(args,++i,arg));else if("--seed".equals(arg))c.seed=Long.parseLong(next(args,++i,arg));else if("--stop-file".equals(arg))c.stopFile=next(args,++i,arg);else if("--input-cases-per-field".equals(arg))c.inputCasesPerField=Integer.parseInt(next(args,++i,arg));else if("--input-max-length".equals(arg))c.inputMaxLength=Integer.parseInt(next(args,++i,arg));else if("--low-battery-exit".equals(arg))c.lowBatteryExit=true;else if("--min-battery".equals(arg))c.minBattery=Integer.parseInt(next(args,++i,arg));else if("--activity-mode".equals(arg))c.activityMode=next(args,++i,arg);else if("--render-fallback-mode".equals(arg))c.renderFallbackMode=next(args,++i,arg);else if("--activity".equals(arg))c.activities.add(next(args,++i,arg));else if("--target-activity".equals(arg))c.targetActivities.add(next(args,++i,arg));else if("--blocked-control".equals(arg))c.blockedControls.add(next(args,++i,arg));else if("--target-token".equals(arg))c.targetToken=next(args,++i,arg);else if("--target-case".equals(arg))c.targetCases.add(next(args,++i,arg));else if("--control-token".equals(arg))c.controlToken=next(args,++i,arg);}
            if(c.packageName==null||!PACKAGE.matcher(c.packageName).matches())throw new IllegalArgumentException("Invalid package");
            if(c.durationSeconds<1||c.durationSeconds>604800)throw new IllegalArgumentException("Invalid duration");
            if(c.throttleMillis<50||c.throttleMillis>60000)throw new IllegalArgumentException("Invalid throttle");
            if(c.inputCasesPerField<1||c.inputCasesPerField>NodeExplorer.MAX_INPUT_CASES_PER_FIELD)throw new IllegalArgumentException("Invalid input cases per field");
            if(c.inputMaxLength<1||c.inputMaxLength>256)throw new IllegalArgumentException("Invalid input max length");
            if(c.lowBatteryExit&&(c.minBattery<1||c.minBattery>100))throw new IllegalArgumentException("Invalid battery threshold");
            if(!Arrays.asList("none","allowlist","blocklist").contains(c.activityMode))throw new IllegalArgumentException("Invalid activity mode");
            CoordinateFallbackController.Mode.parse(c.renderFallbackMode);
            if(!c.targetCases.isEmpty()&&c.targetToken.isEmpty())throw new IllegalArgumentException("Missing target-case token");
            return c;
        }
        static String next(String[] args,int i,String name){if(i>=args.length)throw new IllegalArgumentException("Missing "+name);return args[i];}
    }
    static final class Guard {
        private static final Pattern ACTIVITY = Pattern.compile("(?:topResumedActivity|ResumedActivity).*? ([A-Za-z0-9_.$]+/[A-Za-z0-9_.$]+)");
        private static final Pattern LEVEL = Pattern.compile("(?m)^\\s*level:\\s*(\\d+)");
        final Config config; final ShellInput input; final Set<String> reachedTargets = new HashSet<>(), completedCases = new HashSet<>(), observedActivities = new HashSet<>();
        long nextActivity, nextBattery; String activity;
        Guard(Config config, ShellInput input){this.config=config;this.input=input;}
        String check() throws Exception {
            long now=System.currentTimeMillis();
            if(config.lowBatteryExit&&now>=nextBattery){nextBattery=now+30000;Matcher m=LEVEL.matcher(input.output("dumpsys","battery"));if(m.find()&&Integer.parseInt(m.group(1))<=config.minBattery)return "stop:low_battery";}
            if(now>=nextActivity){nextActivity=now+1000;Matcher m=ACTIVITY.matcher(input.output("dumpsys","activity","activities"));activity=m.find()?m.group(1):"";if(!activity.isEmpty()&&observedActivities.add(activity))emit("activity",activity,"");for(String target:config.targetActivities)if(matches(target,activity)&&reachedTargets.add(target))emit("target_page",activity,"");for(String mapping:config.targetCases){String[] parts=mapping.split("\\|",-1);if(parts.length==3&&matches(parts[0],activity)&&completedCases.add(mapping)){if(runTargetCase(activity,parts[1],parts[2]))emit("target_case_completed",activity,",\"task\":\""+escape(parts[1])+"\",\"case\":\""+escape(parts[2])+"\"");}}}
            boolean listed=false;for(String value:config.activities)if(matches(value,activity)){listed=true;break;}
            if(("allowlist".equals(config.activityMode)&&!listed)||("blocklist".equals(config.activityMode)&&listed)){input.key("KEYCODE_BACK");emit("guarded",activity,"");return "skip";}
            return null;
        }
        static String currentActivity(ShellInput input) throws Exception {Matcher matcher=ACTIVITY.matcher(input.output("dumpsys","activity","activities"));return matcher.find()?matcher.group(1):"";}
        boolean runTargetCase(String currentActivity,String task,String caseName){try{String body="{\"token\":\""+escape(config.targetToken)+"\",\"activity\":\""+escape(currentActivity)+"\",\"task\":\""+escape(task)+"\",\"case\":\""+escape(caseName)+"\"}";post("http://127.0.0.1:7912/v1/monkey/target-case",body);return true;}catch(Exception error){emit("target_case_failed",currentActivity,",\"error\":\""+escape(String.valueOf(error.getMessage()))+"\"");return false;}}
        void emit(String state,String currentActivity,String extra){System.out.println("{\"state\":\""+state+"\",\"requestId\":\""+escape(config.requestId)+"\",\"package\":\""+config.packageName+"\",\"activity\":\""+escape(currentActivity)+"\""+extra+",\"time\":"+System.currentTimeMillis()+"}");}
        static boolean matches(String expected,String actual){return expected.equals(actual)||(!expected.contains("/")&&actual.endsWith("/"+expected));}
        static String http(String address)throws IOException{HttpURLConnection connection=(HttpURLConnection)new URL(address).openConnection();connection.setConnectTimeout(1000);connection.setReadTimeout(5000);try{if(connection.getResponseCode()/100!=2)throw new IOException("Hierarchy HTTP failed");StringBuilder value=new StringBuilder();try(Reader reader=new InputStreamReader(connection.getInputStream(),"UTF-8")){char[] buffer=new char[4096];for(int count;(count=reader.read(buffer))>=0;){if(value.length()+count>4*1024*1024)throw new IOException("Hierarchy too large");value.append(buffer,0,count);}}return value.toString();}finally{connection.disconnect();}}
        static String post(String address,String body)throws IOException{HttpURLConnection connection=(HttpURLConnection)new URL(address).openConnection();connection.setConnectTimeout(1000);connection.setReadTimeout(300000);connection.setRequestMethod("POST");connection.setDoOutput(true);connection.setRequestProperty("Content-Type","application/json");try(OutputStream output=connection.getOutputStream()){output.write(body.getBytes("UTF-8"));}try{int status=connection.getResponseCode();InputStream stream=status<400?connection.getInputStream():connection.getErrorStream();StringBuilder value=new StringBuilder();if(stream!=null)try(Reader reader=new InputStreamReader(stream,"UTF-8")){char[] buffer=new char[1024];for(int count;(count=reader.read(buffer))>=0;)value.append(buffer,0,count);}if(status/100!=2)throw new IOException("Target case HTTP "+status+": "+value);return value.toString();}finally{connection.disconnect();}}
        static String escape(String value){return value.replace("\\","\\\\").replace("\"","\\\"");}
    }
    static final class ShellInput {
		static void selfTest()throws Exception{
			String javaName=System.getProperty("os.name","").startsWith("Windows")?"java.exe":"java";
			String javaPath=new File(new File(System.getProperty("java.home"),"bin"),javaName).getAbsolutePath();
			long started=System.nanoTime();
			try{
				new ShellInput().output(250,javaPath,"-cp",System.getProperty("java.class.path"),Main.class.getName(),"--self-test-sleep");
				throw new AssertionError("ShellInput timeout was not enforced");
			}catch(IOException expected){
				if(!String.valueOf(expected.getMessage()).contains("timed out"))throw expected;
			}
			if(TimeUnit.NANOSECONDS.toSeconds(System.nanoTime()-started)>=5)throw new AssertionError("ShellInput timeout blocked too long");
			System.out.println("ShellInput timeout self-test passed");
		}
        void tap(int x,int y)throws Exception{run("input","tap",String.valueOf(x),String.valueOf(y));}
        void wakeAndDismissKeyguard()throws Exception{
            // Keep this local to the active exploration session. Changing Android's global
            // stay-awake setting would leak test state into later runs.
            run("input","keyevent","KEYCODE_WAKEUP");
            run("wm","dismiss-keyguard");
            Thread.sleep(250L);
        }
        void longPress(int x,int y)throws Exception{run("input","swipe",String.valueOf(x),String.valueOf(y),String.valueOf(x),String.valueOf(y),"800");}
        void key(String code)throws Exception{run("input","keyevent",code);}
        void keyCombination(String... codes)throws Exception{String[] command=new String[codes.length+2];command[0]="input";command[1]="keycombination";System.arraycopy(codes,0,command,2,codes.length);run(command);}
        void text(String token,String value)throws Exception{if(token==null||token.isEmpty())throw new IOException("Missing Runner control token");Guard.post("http://127.0.0.1:7912/v1/monkey/input-text","{\"token\":\""+Guard.escape(token)+"\",\"text\":\""+Guard.escape(value)+"\"}");}
        void swipe(int x1,int y1,int x2,int y2)throws Exception{run("input","swipe",String.valueOf(x1),String.valueOf(y1),String.valueOf(x2),String.valueOf(y2),"250");}
        SafeTouchRegion safeTouchRegion(){int width=1080,height=1920,density=160;try{String value=output("wm","size");Matcher m=Pattern.compile("(\\d+)x(\\d+)").matcher(value);if(m.find()){width=Integer.parseInt(m.group(1));height=Integer.parseInt(m.group(2));}}catch(Exception ignored){}try{Matcher m=Pattern.compile("(?:Override|Physical) density:\\s*(\\d+)").matcher(output("wm","density"));while(m.find())density=Integer.parseInt(m.group(1));}catch(Exception ignored){}return new SafeTouchRegion(width,height,density);}
        void run(String... command)throws Exception{output(command);}
        String output(String... command)throws Exception{return output(10000,command);}
        String output(long timeoutMillis,String... command)throws Exception{
            final Process process=new ProcessBuilder(command).redirectErrorStream(true).start();
            FutureTask<String> drain=new FutureTask<>(() -> {
                ByteArrayOutputStream out=new ByteArrayOutputStream();
                byte[] buffer=new byte[1024];
                try(InputStream input=process.getInputStream()){
                    for(int count;(count=input.read(buffer))>=0;){
                        if(out.size()+count>4*1024*1024){process.destroyForcibly();throw new IOException("Command output too large");}
                        out.write(buffer,0,count);
                    }
                }
                return out.toString("UTF-8");
            });
            Thread reader=new Thread(drain,"xtest-nova-command-output");
            reader.setDaemon(true);
            reader.start();
            if(!process.waitFor(timeoutMillis,TimeUnit.MILLISECONDS)){
                process.destroyForcibly();
                try{process.getInputStream().close();}catch(IOException ignored){}
                drain.cancel(true);
                throw new IOException("Command timed out");
            }
            String output;
            try{output=drain.get(2,TimeUnit.SECONDS);}
            catch(ExecutionException error){
                Throwable cause=error.getCause();
                if(cause instanceof Exception)throw (Exception)cause;
                throw error;
            }
            catch(TimeoutException error){
                drain.cancel(true);
                throw new IOException("Command output did not close",error);
            }
            if(process.exitValue()!=0)throw new IOException("Command failed: "+Arrays.toString(command));
            return output;
        }
    }
}
