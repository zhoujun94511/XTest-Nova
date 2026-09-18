package com.openatx.xtest.nova.runner;

/** Session-scoped coordinate tap isolation after external transitions in a render context. */
final class RenderContextQuarantine {
    static final long COOLDOWN_MILLIS = 12_000L;
    static final long SECOND_EXTERNAL_WINDOW_MILLIS = 60_000L;
    static final long PAUSE_TAP_MILLIS = 120_000L;

    enum Level { NONE, COOLDOWN, PAUSED_TAP, SESSION_NO_TAP }

    static final class Status {
        final Level level;
        final long tapBlockedUntilMillis;
        Status(Level level, long tapBlockedUntilMillis) {
            this.level = level;
            this.tapBlockedUntilMillis = tapBlockedUntilMillis;
        }
    }

    private String contextKey = "";
    private Level level = Level.NONE;
    private long firstExternalAt, tapBlockedUntil;
    private int externalCount;

    Status onExternal(String currentContextKey, long elapsedMillis) {
        String key = currentContextKey == null ? "" : currentContextKey;
        if (!key.equals(contextKey)) {
            contextKey = key;
            level = Level.NONE;
            firstExternalAt = 0L;
            tapBlockedUntil = 0L;
            externalCount = 0;
        }
        externalCount++;
        if (externalCount == 1) {
            level = Level.COOLDOWN;
            tapBlockedUntil = elapsedMillis + COOLDOWN_MILLIS;
            firstExternalAt = elapsedMillis;
        } else if (externalCount == 2 && elapsedMillis - firstExternalAt <= SECOND_EXTERNAL_WINDOW_MILLIS) {
            level = Level.PAUSED_TAP;
            tapBlockedUntil = elapsedMillis + PAUSE_TAP_MILLIS;
        } else if (externalCount >= 3) {
            level = Level.SESSION_NO_TAP;
            tapBlockedUntil = Long.MAX_VALUE;
        }
        return status(elapsedMillis);
    }

    Status status(String currentContextKey, long elapsedMillis) {
        if (currentContextKey == null || !currentContextKey.equals(contextKey)) {
            return new Status(Level.NONE, 0L);
        }
        return status(elapsedMillis);
    }

    boolean allowsTap(String currentContextKey, long elapsedMillis) {
        Status status = status(currentContextKey, elapsedMillis);
        return status.level == Level.NONE || elapsedMillis >= status.tapBlockedUntilMillis;
    }

    boolean allowsSwipe(String currentContextKey, long elapsedMillis) {
        Status status = status(currentContextKey, elapsedMillis);
        if (status.level == Level.SESSION_NO_TAP) return true;
        return allowsTap(currentContextKey, elapsedMillis);
    }

    void clearOnHardRecovery() {
        contextKey = "";
        level = Level.NONE;
        firstExternalAt = 0L;
        tapBlockedUntil = 0L;
        externalCount = 0;
    }

    private Status status(long elapsedMillis) {
        if (level == Level.NONE) return new Status(Level.NONE, 0L);
        if (level == Level.SESSION_NO_TAP) return new Status(level, Long.MAX_VALUE);
        if (elapsedMillis >= tapBlockedUntil && level != Level.SESSION_NO_TAP) {
            return new Status(Level.NONE, 0L);
        }
        return new Status(level, tapBlockedUntil);
    }

    static void selfTest() {
        RenderContextQuarantine quarantine = new RenderContextQuarantine();
        String key = "com.game/.Main|unity_activity";
        Status first = quarantine.onExternal(key, 1_000L);
        if (first.level != Level.COOLDOWN || quarantine.allowsTap(key, 2_000L))
            throw new AssertionError("first external did not enter cooldown");
        if (quarantine.allowsTap(key, 12_999L))
            throw new AssertionError("cooldown ended too early");
        if (!quarantine.allowsTap(key, 14_000L))
            throw new AssertionError("cooldown did not release taps");
        quarantine.onExternal(key, 20_000L);
        if (quarantine.status(key, 20_000L).level != Level.PAUSED_TAP)
            throw new AssertionError("second external did not pause taps");
        quarantine.onExternal(key, 25_000L);
        if (quarantine.allowsTap(key, 30_000L))
            throw new AssertionError("session no-tap was not enforced");
        if (!quarantine.allowsSwipe(key, 30_000L))
            throw new AssertionError("session no-tap blocked swipe");
    }
}
