package com.openatx.xtest.nova.runner;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

/** Records coordinate-fallback intent, immediate outcome and external recovery without authorizing actions. */
final class CoordinateActionEvidence {
    static final class Event {
        final String state, activity, extra, outcome, zone, contextKey;
        Event(String state, String activity, String extra, String outcome, String zone) {
            this(state, activity, extra, outcome, zone, "|none");
        }
        Event(String state, String activity, String extra, String outcome, String zone, String contextKey) {
            this.state = state;
            this.activity = activity == null ? "" : activity;
            this.extra = extra;
            this.outcome = outcome == null ? "" : outcome;
            this.zone = zone == null ? "" : zone;
            this.contextKey = contextKey == null ? "" : contextKey;
        }
    }

    private static final class Sample {
        final long id, startedAt;
        final String activity, source, gesture, zone, coordinates;
        Sample(long id, long startedAt, String activity, String source, String gesture, String zone, String coordinates) {
            this.id = id;
            this.startedAt = startedAt;
            this.activity = activity;
            this.source = source;
            this.gesture = gesture;
            this.zone = zone;
            this.coordinates = coordinates;
        }
    }

    private final String targetPackage;
    private long sequence;
    private long lastTargetResultAt = -1L;
    private Sample pending, recovering;

    CoordinateActionEvidence(String targetPackage) { this.targetPackage = targetPackage; }

    List<Event> begin(CoordinateFallbackController.Gesture gesture, SafeTouchRegion region, String activity,
                      RenderSceneProbe.Signal signal, CoordinateFallbackController.Mode mode, long elapsedMillis) {
        if (gesture.type == CoordinateFallbackController.GestureType.BACK)
            throw new IllegalArgumentException("Back is not a coordinate action");
        List<Event> events = new ArrayList<>();
        long nextId = sequence + 1;
        if (pending != null) {
            Sample sample = pending;
            pending = null;
            events.add(event("coordinate_action_result", sample, activity,
                    ",\"outcome\":\"unobserved\",\"reason\":\"superseded_by_next_action\"" +
                    ",\"supersededByCoordinateActionId\":" + nextId +
                    ",\"elapsedMillis\":" + duration(sample, elapsedMillis), "unobserved"));
        }
        int[] values = gesture.coordinates;
        int x = gesture.type == CoordinateFallbackController.GestureType.TAP ? values[0] : (values[0] + values[2]) / 2;
        int y = gesture.type == CoordinateFallbackController.GestureType.TAP ? values[1] : (values[1] + values[3]) / 2;
        String coordinates = gesture.type == CoordinateFallbackController.GestureType.TAP
                ? ",\"x\":" + values[0] + ",\"y\":" + values[1]
                : ",\"x1\":" + values[0] + ",\"y1\":" + values[1] + ",\"x2\":" + values[2] + ",\"y2\":" + values[3];
        String zone = region.zone(x, y);
        pending = new Sample(++sequence, elapsedMillis, activity == null ? "" : activity, signal.wireName,
                gesture.type.name().toLowerCase(Locale.ROOT), zone, coordinates);
        events.add(event("coordinate_action", pending, activity,
                ",\"source\":\"render_fallback\",\"renderFallbackMode\":\"" + mode.wireName() +
                "\",\"xPermille\":" + region.xPermille(x) +
                ",\"yPermille\":" + region.yPermille(y) + coordinates, ""));
        return events;
    }

    Event observeActivity(String activity, long elapsedMillis) {
        String foreground = SpecialHandler.activityPackage(activity);
        if (recovering != null && targetPackage.equals(foreground)) {
            Sample sample = recovering;
            recovering = null;
            return event("coordinate_action_recovery", sample, activity,
                    ",\"recovered\":true,\"recoveryMillis\":" + duration(sample, elapsedMillis), "");
        }
        if (pending == null || foreground.isEmpty() || targetPackage.equals(foreground)) return null;
        Sample sample = pending;
        pending = null;
        recovering = sample;
        return event("coordinate_action_result", sample, activity,
                ",\"outcome\":\"external\",\"externalPackage\":\"" + Main.Guard.escape(foreground) +
                "\",\"elapsedMillis\":" + duration(sample, elapsedMillis), "external");
    }

    Event observeHierarchy(String activity, String hierarchy, long elapsedMillis) {
        if (pending == null || !targetPackage.equals(SpecialHandler.activityPackage(activity))) return null;
        Sample sample = pending;
        pending = null;
        String specialKind = SpecialHandler.classifyEvidence(targetPackage, activity, hierarchy);
        String outcome = specialKind.isEmpty() ? "target" : "special";
        if ("target".equals(outcome)) lastTargetResultAt = elapsedMillis;
        return event("coordinate_action_result", sample, activity,
                ",\"outcome\":\"" + outcome + "\",\"specialKind\":\"" + Main.Guard.escape(specialKind) +
                "\",\"elapsedMillis\":" + duration(sample, elapsedMillis), outcome);
    }

    List<Event> finish(String activity, long elapsedMillis) {
        List<Event> events = new ArrayList<>();
        if (pending != null) {
            Sample sample = pending;
            pending = null;
            events.add(event("coordinate_action_result", sample, activity,
                    ",\"outcome\":\"unobserved\",\"reason\":\"run_finished\",\"elapsedMillis\":" + duration(sample, elapsedMillis), "unobserved"));
        }
        if (recovering != null) {
            Sample sample = recovering;
            recovering = null;
            events.add(event("coordinate_action_recovery", sample, activity,
                    ",\"recovered\":false,\"recoveryMillis\":" + duration(sample, elapsedMillis), ""));
        }
        return events;
    }

    private static Event event(String state, Sample sample, String activity, String extra, String outcome) {
        return new Event(state, activity,
                ",\"coordinateActionId\":" + sample.id + ",\"gesture\":\"" + sample.gesture +
                "\",\"zone\":\"" + sample.zone + "\",\"originActivity\":\"" +
                Main.Guard.escape(sample.activity) + "\",\"renderSignal\":\"" + sample.source + "\"" + extra,
                outcome, sample.zone, sample.activity + "|" + sample.source);
    }

    private static long duration(Sample sample, long elapsedMillis) {
        return Math.max(0L, elapsedMillis - sample.startedAt);
    }

    static long elapsedRealtimeMillis() { return System.nanoTime() / 1_000_000L; }

    boolean hasRecentTargetResult(long elapsedMillis, long maximumAgeMillis) {
        return lastTargetResultAt >= 0L && elapsedMillis >= lastTargetResultAt &&
                elapsedMillis - lastTargetResultAt <= Math.max(0L, maximumAgeMillis);
    }

    static void selfTest() {
        SafeTouchRegion region = new SafeTouchRegion(1000, 2000, 160);
        CoordinateActionEvidence evidence = new CoordinateActionEvidence("com.example.app");
        Event start = evidence.begin(new CoordinateFallbackController.Gesture(CoordinateFallbackController.GestureType.TAP, 500, 1000),
                region, "com.example.app/.Game", RenderSceneProbe.Signal.UNITY_ACTIVITY, CoordinateFallbackController.Mode.BOUNDED, 100L).get(0);
        if (!start.extra.contains("\"zone\":\"r2c1\"") || !start.extra.contains("\"xPermille\":500"))
            throw new AssertionError("coordinate zone evidence is incorrect");
        Event special = evidence.observeHierarchy("com.example.app/.Game", "<node text=\"subscribe\" />", 180L);
        if (special == null || !special.extra.contains("\"outcome\":\"special\"") || !special.extra.contains("\"specialKind\":\"paywall\""))
            throw new AssertionError("special outcome evidence is missing");
        if (evidence.hasRecentTargetResult(181L, 100L)) throw new AssertionError("special result was treated as target progress");
        evidence.begin(new CoordinateFallbackController.Gesture(CoordinateFallbackController.GestureType.SWIPE, 400, 1500, 400, 500),
                region, "com.example.app/.Game", RenderSceneProbe.Signal.RENDER_SURFACE, CoordinateFallbackController.Mode.CONTINUOUS, 200L);
        Event external = evidence.observeActivity("com.android.vending/.AssetBrowserActivity", 240L);
        Event recovery = evidence.observeActivity("com.example.app/.Game", 390L);
        if (external == null || !external.extra.contains("\"outcome\":\"external\"") || recovery == null || !recovery.extra.contains("\"recoveryMillis\":190"))
            throw new AssertionError("external recovery evidence is incorrect");
        evidence.begin(new CoordinateFallbackController.Gesture(CoordinateFallbackController.GestureType.TAP, 300, 700),
                region, "com.example.app/.Game", RenderSceneProbe.Signal.RENDER_SURFACE, CoordinateFallbackController.Mode.CONTINUOUS, 400L);
        List<Event> replacement = evidence.begin(new CoordinateFallbackController.Gesture(CoordinateFallbackController.GestureType.TAP, 700, 700),
                region, "com.example.app/.Game", RenderSceneProbe.Signal.RENDER_SURFACE, CoordinateFallbackController.Mode.CONTINUOUS, 450L);
        if (replacement.size() != 2 || !replacement.get(0).extra.contains("\"reason\":\"superseded_by_next_action\"") ||
                !replacement.get(0).extra.contains("\"supersededByCoordinateActionId\":4"))
            throw new AssertionError("superseded coordinate result is missing");
        evidence.observeHierarchy("com.example.app/.Game", "<node text=\"play\" />", 500L);
        if (!evidence.hasRecentTargetResult(550L, 100L) || evidence.hasRecentTargetResult(601L, 100L))
            throw new AssertionError("recent coordinate target window is incorrect");
    }
}
