package com.openatx.xtest.nova.runner;

import java.util.Arrays;
import java.util.HashSet;
import java.util.Locale;
import java.util.Random;
import java.util.Set;

/** Owns coordinate-fallback authorization, retry budget and gesture selection. */
final class CoordinateFallbackController {
    static final int MAX_BOUNDED_ATTEMPTS = 5;
    static final int MAX_EXTERNAL_BEFORE_DOWNGRADE = 3;

    enum Mode {
        OFF, BOUNDED, CONTINUOUS;

        static Mode parse(String value) {
            String normalized = value == null ? "" : value.trim().toUpperCase(Locale.ROOT);
            if (normalized.isEmpty()) return BOUNDED;
            try { return valueOf(normalized); }
            catch (IllegalArgumentException error) {
                throw new IllegalArgumentException("Invalid render fallback mode: " + value);
            }
        }

        String wireName() { return name().toLowerCase(Locale.ROOT); }
    }

    enum GestureType { TAP, SWIPE, BACK }

    static final class Gesture {
        final GestureType type;
        final int[] coordinates;
        Gesture(GestureType type, int... coordinates) {
            this.type = type;
            this.coordinates = coordinates;
        }
    }

    static final class Attempt {
        final boolean allowed;
        final boolean continuous;
        final int number;
        final boolean downgraded;
        Attempt(boolean allowed, boolean continuous, int number, boolean downgraded) {
            this.allowed = allowed;
            this.continuous = continuous;
            this.number = number;
            this.downgraded = downgraded;
        }
    }

    static final class Feedback {
        final String zone, contextKey;
        final int externalResults;
        final boolean zoneNewlyBlocked, downgradedNow;
        Feedback(String zone, String contextKey, int externalResults, boolean zoneNewlyBlocked, boolean downgradedNow) {
            this.zone = zone;
            this.contextKey = contextKey;
            this.externalResults = externalResults;
            this.zoneNewlyBlocked = zoneNewlyBlocked;
            this.downgradedNow = downgradedNow;
        }
    }

    private final Mode mode;
    private int attempts;
    private int externalResults;
    private boolean downgraded;
    private final Set<String> blockedContextZones = new HashSet<>();

    CoordinateFallbackController(String mode) { this.mode = Mode.parse(mode); }

    Mode mode() { return mode; }

    boolean renderFallbackEnabled(RenderSceneProbe.Signal signal) {
        return !RenderSceneProbe.isRenderScene(signal) || mode != Mode.OFF;
    }

    Attempt begin(RenderSceneProbe.Signal signal) {
        boolean renderScene = RenderSceneProbe.isRenderScene(signal);
        boolean continuous = renderScene && mode == Mode.CONTINUOUS && !downgraded;
        if (renderScene && mode == Mode.OFF) return new Attempt(false, false, attempts, false);
        if (continuous) {
            attempts = 0;
            return new Attempt(true, true, 1, false);
        }
        attempts++;
        return new Attempt(attempts <= MAX_BOUNDED_ATTEMPTS, false, attempts, downgraded);
    }

    void reset() { attempts = 0; }

    Gesture choose(Random random, SafeTouchRegion region, boolean continuous) {
        return choose(random, region, continuous, "", RenderSceneProbe.Signal.NONE);
    }

    Gesture choose(Random random, SafeTouchRegion region, boolean continuous, String activity, RenderSceneProbe.Signal signal) {
        String context = contextKey(activity, signal);
        for (int retry = 0; retry < 32; retry++) {
            Gesture gesture = chooseCandidate(random, region, continuous);
            if (gesture.type == GestureType.BACK || !blockedContextZones.contains(context + "|" + zone(gesture, region))) return gesture;
        }
        return new Gesture(GestureType.BACK);
    }

    Feedback observe(CoordinateActionEvidence.Event event) {
        if (event == null || !"coordinate_action_result".equals(event.state) || !"external".equals(event.outcome)) return null;
        externalResults++;
        boolean newlyBlocked = !event.zone.isEmpty() && blockedContextZones.add(event.contextKey + "|" + event.zone);
        boolean downgradeNow = mode == Mode.CONTINUOUS && !downgraded && externalResults >= MAX_EXTERNAL_BEFORE_DOWNGRADE;
        if (downgradeNow) { downgraded = true; attempts = 0; }
        return new Feedback(event.zone, event.contextKey, externalResults, newlyBlocked, downgradeNow);
    }

    private Gesture chooseCandidate(Random random, SafeTouchRegion region, boolean continuous) {
        int event = random.nextInt(100);
        if (continuous) {
            if (event < 65) {
                int[] point = region.randomPoint(random);
                return new Gesture(GestureType.TAP, point[0], point[1]);
            }
            return new Gesture(GestureType.SWIPE, region.randomSwipe(random));
        }
        if (event < 45) {
            int[] point = region.randomPoint(random);
            return new Gesture(GestureType.TAP, point[0], point[1]);
        }
        if (event < 75) return new Gesture(GestureType.SWIPE, region.randomSwipe(random));
        return new Gesture(GestureType.BACK);
    }

    private static String zone(Gesture gesture, SafeTouchRegion region) {
        int[] values = gesture.coordinates;
        int x = gesture.type == GestureType.TAP ? values[0] : (values[0] + values[2]) / 2;
        int y = gesture.type == GestureType.TAP ? values[1] : (values[1] + values[3]) / 2;
        return region.zone(x, y);
    }

    private static String contextKey(String activity, RenderSceneProbe.Signal signal) {
        return (activity == null ? "" : activity) + "|" + (signal == null ? "none" : signal.wireName);
    }

    static void selfTest() {
        CoordinateFallbackController bounded = new CoordinateFallbackController("bounded");
        for (int index = 0; index < MAX_BOUNDED_ATTEMPTS; index++)
            if (!bounded.begin(RenderSceneProbe.Signal.RENDER_SURFACE).allowed)
                throw new AssertionError("bounded render fallback ended too early");
        if (bounded.begin(RenderSceneProbe.Signal.RENDER_SURFACE).allowed)
            throw new AssertionError("bounded render fallback exceeded its limit");

        CoordinateFallbackController continuous = new CoordinateFallbackController("continuous");
        for (int index = 0; index < MAX_BOUNDED_ATTEMPTS + 3; index++)
            if (!continuous.begin(RenderSceneProbe.Signal.UNITY_ACTIVITY).continuous)
                throw new AssertionError("explicit continuous fallback was capped");

        if (new CoordinateFallbackController("off").renderFallbackEnabled(RenderSceneProbe.Signal.UNITY_ACTIVITY))
            throw new AssertionError("disabled render fallback remained enabled");
        if (!new CoordinateFallbackController("off").begin(RenderSceneProbe.Signal.NONE).allowed)
            throw new AssertionError("render policy disabled generic hierarchy recovery");

        SafeTouchRegion region = new SafeTouchRegion(1080, 2400, 420);
        Gesture gesture = continuous.choose(new Random(7), region, true);
        if (gesture.type == GestureType.BACK || Arrays.stream(gesture.coordinates).anyMatch(value -> value < 0))
            throw new AssertionError("continuous render gesture was unsafe");

        CoordinateFallbackController feedback = new CoordinateFallbackController("continuous");
        for (int index = 0; index < MAX_EXTERNAL_BEFORE_DOWNGRADE; index++) {
            CoordinateActionEvidence.Event event = new CoordinateActionEvidence.Event("coordinate_action_result", "com.store/.Main", "", "external", "r2c1");
            Feedback result = feedback.observe(event);
            if (result == null || !result.zoneNewlyBlocked && index == 0) throw new AssertionError("external zone was not blocked");
        }
        Attempt downgradedAttempt = feedback.begin(RenderSceneProbe.Signal.UNITY_ACTIVITY);
        if (downgradedAttempt.continuous || !downgradedAttempt.downgraded || !downgradedAttempt.allowed)
            throw new AssertionError("continuous fallback did not downgrade to bounded");
        for (int index = 0; index < 100; index++) {
            Gesture safe = feedback.choose(new Random(index), region, false);
            if (safe.type != GestureType.BACK && "r2c1".equals(zone(safe, region)))
                throw new AssertionError("blocked external zone was selected again");
        }
    }
}
