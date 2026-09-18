package com.openatx.xtest.nova.runner;

import java.util.Locale;

/**
 * Detects rendering-only scenes without deciding how input should be injected.
 * Keeping detection separate from execution prevents an engine fingerprint from
 * silently granting an application an unlimited coordinate-input policy.
 */
final class RenderSceneProbe {
    enum Signal {
        NONE("none"),
        RENDER_SURFACE("render_surface"),
        UNITY_ACTIVITY("unity_activity");

        final String wireName;
        Signal(String wireName) { this.wireName = wireName; }
    }

    private RenderSceneProbe() {}

    static Signal detect(String activity, boolean renderSurface, boolean hasActions) {
        if (hasActions) return Signal.NONE;
        if (isUnityHost(activity)) return Signal.UNITY_ACTIVITY;
        if (renderSurface) return Signal.RENDER_SURFACE;
        return Signal.NONE;
    }

    static boolean isRenderScene(Signal signal) {
        return signal != null && signal != Signal.NONE;
    }

    static boolean isUnityHost(String activity) {
        String value = activity == null ? "" : activity.toLowerCase(Locale.ROOT);
        return value.contains("com.unity3d.player.unityplayeractivity");
    }

    static void selfTest() {
        if (detect("com.example/.Static", false, false) != Signal.NONE)
            throw new AssertionError("ordinary static activity was classified as a render scene");
        if (detect("com.example/.Video", true, false) != Signal.RENDER_SURFACE)
            throw new AssertionError("render surface was not detected");
        if (detect("com.example/com.unity3d.player.UnityPlayerActivity", false, false) != Signal.UNITY_ACTIVITY)
            throw new AssertionError("opaque Unity activity was not detected");
        if (detect("com.example/com.unity3d.player.UnityPlayerActivity", true, true) != Signal.NONE)
            throw new AssertionError("actionable overlay must take priority over render fallback");
    }
}
