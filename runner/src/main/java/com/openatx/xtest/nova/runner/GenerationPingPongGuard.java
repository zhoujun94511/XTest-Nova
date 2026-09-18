package com.openatx.xtest.nova.runner;

import java.util.LinkedHashSet;
import java.util.Set;

/** Detects coordinate-result/exhaustion loops that span reconstructed Explorer generations. */
final class GenerationPingPongGuard {
    static final int DETECTION_THRESHOLD = 3;
    static final int DISTINCT_SCENES_TO_CLEAR = 2;
    static final long RECENT_RESULT_WINDOW_MILLIS = 1_500L;
    static final long BASE_HOLD_MILLIS = 30_000L;
    static final long MAX_HOLD_MILLIS = 120_000L;
    static final long WAIT_REPORT_INTERVAL_MILLIS = 10_000L;
    static final long NO_IMMEDIATE_EXHAUSTION_CLEAR_MILLIS = 60_000L;
    static final long HARD_RECOVERY_MILLIS = 120_000L;

    static final class Decision {
        final boolean blocked, entered, report, softRefresh, hardRecovery;
        final boolean confirmed;
        final int streak, level;
        final long holdMillis, remainingMillis;

        Decision(boolean blocked, boolean entered, boolean report, boolean softRefresh, boolean hardRecovery,
                 boolean confirmed, int streak, int level, long holdMillis, long remainingMillis) {
            this.blocked = blocked;
            this.entered = entered;
            this.report = report;
            this.softRefresh = softRefresh;
            this.hardRecovery = hardRecovery;
            this.confirmed = confirmed;
            this.streak = streak;
            this.level = level;
            this.holdMillis = holdMillis;
            this.remainingMillis = remainingMillis;
        }
    }

    private String activity = "";
    private int streak, level;
    private long holdUntil, lastWaitReportAt;
    private boolean confirmed, pendingSoftRefresh;
    private long confirmedSince, lastImmediateExhaustionAt, lastValidProgressAt;
    private final Set<String> distinctSemanticScenes = new LinkedHashSet<>();

    Decision onLoop(String currentActivity, long elapsedMillis) {
        String current = currentActivity == null ? "" : currentActivity;
        if (!current.equals(activity)) {
            reset(current);
            return allow();
        }
        Decision active = activeHold(elapsedMillis);
        if (active != null) return active;
        if (pendingSoftRefresh) {
            pendingSoftRefresh = false;
            return new Decision(false, false, true, true, false, true, streak, level, 0L, 0L);
        }
        maybeClearConfirmedWithoutImmediateExhaustion(elapsedMillis);
        return allow();
    }

    Decision onExhausted(String currentActivity, boolean recentCoordinateTarget, long elapsedMillis) {
        String current = currentActivity == null ? "" : currentActivity;
        if (!current.equals(activity)) reset(current);
        Decision active = activeHold(elapsedMillis);
        if (active != null) return active;
        maybeClearConfirmedWithoutImmediateExhaustion(elapsedMillis);
        if (shouldHardRecovery(elapsedMillis)) {
            return new Decision(false, false, true, false, true, true,
                    streak, level, 0L, 0L);
        }
        if (!recentCoordinateTarget) {
            if (!confirmed) streak = 0;
            return allow();
        }
        lastImmediateExhaustionAt = elapsedMillis;
        if (confirmed) return enterHold(elapsedMillis);
        streak++;
        if (streak < DETECTION_THRESHOLD) return allow();
        confirmed = true;
        confirmedSince = elapsedMillis;
        lastValidProgressAt = elapsedMillis;
        return enterHold(elapsedMillis);
    }

    void noteExplorationProgress(String semanticScene, long elapsedMillis) {
        if (semanticScene == null || semanticScene.isEmpty()) return;
        lastValidProgressAt = elapsedMillis;
        if (!confirmed) return;
        if (distinctSemanticScenes.add(semanticScene) && distinctSemanticScenes.size() >= DISTINCT_SCENES_TO_CLEAR) {
            reset(activity);
        }
    }

    boolean isConfirmed() { return confirmed; }

    boolean shouldHardRecovery(long elapsedMillis) {
        if (!confirmed) return false;
        long anchor = lastValidProgressAt > 0L ? lastValidProgressAt : confirmedSince;
        return anchor > 0L && elapsedMillis - anchor >= HARD_RECOVERY_MILLIS;
    }

    private Decision enterHold(long elapsedMillis) {
        level++;
        long hold = Math.min(MAX_HOLD_MILLIS, BASE_HOLD_MILLIS << Math.min(2, level - 1));
        holdUntil = elapsedMillis + hold;
        lastWaitReportAt = elapsedMillis;
        return new Decision(true, true, true, false, false, confirmed, streak, level, hold, hold);
    }

    private void maybeClearConfirmedWithoutImmediateExhaustion(long elapsedMillis) {
        if (!confirmed || lastImmediateExhaustionAt <= 0L) return;
        if (elapsedMillis - lastImmediateExhaustionAt >= NO_IMMEDIATE_EXHAUSTION_CLEAR_MILLIS) {
            reset(activity);
        }
    }

    private Decision allow() {
        return new Decision(false, false, false, false, false, confirmed, streak, level, 0L, 0L);
    }

    /** Returns null when no hold has ever been entered, otherwise the current hold/release decision. */
    private Decision activeHold(long elapsedMillis) {
        if (holdUntil > elapsedMillis) {
            boolean report = lastWaitReportAt <= 0L || elapsedMillis - lastWaitReportAt >= WAIT_REPORT_INTERVAL_MILLIS;
            if (report) lastWaitReportAt = elapsedMillis;
            return new Decision(true, false, report, false, false, confirmed, streak, level, 0L, holdUntil - elapsedMillis);
        }
        if (holdUntil > 0L) {
            holdUntil = 0L;
            lastWaitReportAt = 0L;
            if (confirmed) {
                // Waiting cannot demonstrate stability because exploration is gated.
                // Start the quiet window after exploration is allowed to resume.
                lastImmediateExhaustionAt = elapsedMillis;
                pendingSoftRefresh = true;
                return null;
            }
            streak = 0;
            return allow();
        }
        return null;
    }

    private void reset(String currentActivity) {
        activity = currentActivity;
        streak = 0;
        level = 0;
        holdUntil = 0L;
        lastWaitReportAt = 0L;
        confirmed = false;
        pendingSoftRefresh = false;
        confirmedSince = 0L;
        lastImmediateExhaustionAt = 0L;
        lastValidProgressAt = 0L;
        distinctSemanticScenes.clear();
    }

    static void selfTest() {
        GenerationPingPongGuard guard = new GenerationPingPongGuard();
        if (guard.onExhausted("com.example/.Game", true, 100L).blocked ||
                guard.onExhausted("com.example/.Game", true, 200L).blocked)
            throw new AssertionError("generation ping-pong blocked before threshold");
        Decision first = guard.onExhausted("com.example/.Game", true, 300L);
        if (!first.blocked || !first.entered || first.level != 1 || first.holdMillis != BASE_HOLD_MILLIS || !first.confirmed)
            throw new AssertionError("generation ping-pong did not enter first hold");
        if (guard.onExhausted("com.example/.Game", false, 1_000L).report)
            throw new AssertionError("generation ping-pong wait reported too frequently");
        if (!guard.onExhausted("com.example/.Game", false, 10_300L).report)
            throw new AssertionError("generation ping-pong wait heartbeat is missing");
        if (!guard.onLoop("com.example/.Game", 20_300L).blocked)
            throw new AssertionError("generation ping-pong loop gate did not preserve hold");
        Decision released = guard.onLoop("com.example/.Game", 30_300L);
        if (released.blocked || !released.softRefresh || !released.confirmed)
            throw new AssertionError("generation ping-pong hold did not schedule soft refresh");
        Decision immediate = guard.onExhausted("com.example/.Game", true, 31_000L);
        if (!immediate.blocked || immediate.level != 2)
            throw new AssertionError("confirmed ping-pong did not re-block on first immediate exhaustion");
        if (!guard.onLoop("com.example/.Game", 90_999L).blocked)
            throw new AssertionError("second hold was cleared before its deadline");
        Decision secondRelease = guard.onLoop("com.example/.Game", 91_000L);
        if (!secondRelease.softRefresh || !secondRelease.confirmed)
            throw new AssertionError("second hold did not release through a soft refresh");
        if (!guard.onLoop("com.example/.Game", 150_999L).confirmed)
            throw new AssertionError("quiet window included time spent inside the hold");
        if (guard.onLoop("com.example/.Game", 151_000L).confirmed)
            throw new AssertionError("confirmed state did not clear after a full quiet window");

        guard.onExhausted("com.example/.Game", true, 152_000L);
        guard.onExhausted("com.example/.Game", true, 152_100L);
        guard.onExhausted("com.example/.Game", true, 152_200L);
        guard.onLoop("com.example/.Game", 182_200L);
        guard.onExhausted("com.example/.Game", true, 183_000L);
        guard.onLoop("com.example/.Game", 243_000L);
        Decision third = guard.onExhausted("com.example/.Game", true, 244_000L);
        if (!third.blocked || third.level != 3 || third.holdMillis != MAX_HOLD_MILLIS)
            throw new AssertionError("generation ping-pong did not enter capped third hold");
        if (!guard.onLoop("com.example/.Game", 363_999L).blocked)
            throw new AssertionError("third hold was cleared before its deadline");
        Decision thirdRelease = guard.onLoop("com.example/.Game", 364_000L);
        if (!thirdRelease.softRefresh || !thirdRelease.confirmed)
            throw new AssertionError("third hold did not release through a soft refresh");
        Decision hardRecovery = guard.onExhausted("com.example/.Game", true, 364_100L);
        if (hardRecovery.blocked || !hardRecovery.hardRecovery)
            throw new AssertionError("confirmed stall did not request hard recovery");

        guard.noteExplorationProgress("scene-a", 365_000L);
        guard.noteExplorationProgress("scene-b", 365_100L);
        if (guard.isConfirmed())
            throw new AssertionError("distinct semantic scenes did not clear confirmed state");
        guard.onExhausted("com.example/.Game", true, 365_200L);
        guard.onExhausted("com.example/.Game", true, 365_300L);
        Decision afterProgress = guard.onExhausted("com.example/.Game", true, 365_400L);
        if (!afterProgress.blocked || afterProgress.level != 1)
            throw new AssertionError("generation ping-pong did not re-enter after progress reset");
        if (guard.onExhausted("com.example/.Other", false, 365_500L).blocked)
            throw new AssertionError("generation ping-pong leaked across activities");
    }
}
