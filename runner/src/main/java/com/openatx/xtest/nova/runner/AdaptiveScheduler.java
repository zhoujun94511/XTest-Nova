package com.openatx.xtest.nova.runner;

import java.util.*;

/**
 * Seeded, feedback-driven action scheduler for breadth-first exploration.
 * Safety decisions stay outside this class; every eligible action keeps a
 * non-zero probability until the bounded repeat guard is reached.
 */
final class AdaptiveScheduler {
    static final int MAX_DIRECT_ATTEMPTS_PER_ACTIVITY = 3;

    private final long seed;
    private long selectionOrdinal;
    private final Map<String, Stats> stats = new HashMap<>();
    private final Set<String> seenStates = new HashSet<>();

    AdaptiveScheduler(long seed) { this.seed = seed; }

    void inherit(AdaptiveScheduler previous) {
        selectionOrdinal = previous.selectionOrdinal;
        seenStates.addAll(previous.seenStates);
        for (Map.Entry<String, Stats> entry : previous.stats.entrySet()) stats.put(entry.getKey(), new Stats(entry.getValue()));
    }

    void inheritRecovery(AdaptiveScheduler previous) {
        // Keep deterministic diversity and coverage knowledge, but release the
        // per-action attempt budget after a deliberate target relaunch.
        selectionOrdinal = previous.selectionOrdinal;
        seenStates.addAll(previous.seenStates);
    }

    boolean observeState(String state) { return seenStates.add(state == null ? "" : state); }

    boolean eligible(String activity, Main.NodeExplorer.Action action) {
        if (action == null || "back".equals(action.type) || action.isScrollGesture()) return true;
        return stat(activity, action).attempts < MAX_DIRECT_ATTEMPTS_PER_ACTIVITY;
    }

    Main.NodeExplorer.Action select(List<Main.NodeExplorer.Action> candidates, String activity) {
        if (candidates == null || candidates.isEmpty()) return null;
        List<Main.NodeExplorer.Action> ordered = new ArrayList<>(candidates);
        ordered.sort(Comparator.comparing(action -> action.id));
        long total = 0;
        long[] weights = new long[ordered.size()];
        for (int index = 0; index < ordered.size(); index++) {
            weights[index] = weight(activity, ordered.get(index));
            total += weights[index];
        }
        Random random = new Random(seed ^ (++selectionOrdinal * 0x9E3779B97F4A7C15L) ^ (activity == null ? 0 : activity.hashCode()));
        long selected = Math.floorMod(random.nextLong(), Math.max(1L, total));
        for (int index = 0; index < ordered.size(); index++) {
            if (selected < weights[index]) return ordered.get(index);
            selected -= weights[index];
        }
        return ordered.get(ordered.size() - 1);
    }

    void recordExecuted(String activity, Main.NodeExplorer.Action action) {
        if (action == null || "back".equals(action.type) || action.isScrollGesture()) return;
        stat(activity, action).attempts++;
    }

    void recordTransition(String activity, Main.NodeExplorer.Action action, boolean newState, boolean activityChanged, boolean noProgress) {
        if (action == null || "back".equals(action.type)) return;
        Stats value = stat(activity, action);
        value.transitions++;
        if (newState) value.newStates++;
        if (activityChanged) value.newActivities++;
        if (noProgress) value.noProgress++;
    }

    int attempts(String activity, Main.NodeExplorer.Action action) { return stat(activity, action).attempts; }

    private long weight(String activity, Main.NodeExplorer.Action action) {
        Stats value = stat(activity, action);
        double novelty = value.attempts == 0 ? 4.0 : (1.0 + value.newStates * 3.0 + value.newActivities * 2.0) / (value.transitions + 1.0);
        double exploration = 2.0 / Math.sqrt(value.attempts + 1.0);
        double stagnation = 1.0 / (1.0 + value.noProgress);
        double typeFactor = "input".equals(action.type) ? 0.65 : (action.isScrollGesture() ? 0.85 : 1.0);
        return Math.max(1L, Math.round(100.0 * (novelty + exploration) * stagnation * typeFactor));
    }

    private Stats stat(String activity, Main.NodeExplorer.Action action) {
        String key = (activity == null ? "" : activity.toLowerCase(Locale.ROOT)) + "|" + action.id;
        return stats.computeIfAbsent(key, ignored -> new Stats());
    }

    private static final class Stats {
        int attempts, transitions, newStates, newActivities, noProgress;
        Stats() {}
        Stats(Stats source) {
            attempts = source.attempts;
            transitions = source.transitions;
            newStates = source.newStates;
            newActivities = source.newActivities;
            noProgress = source.noProgress;
        }
    }
}
