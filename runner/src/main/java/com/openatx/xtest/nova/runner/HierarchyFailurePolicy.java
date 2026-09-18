package com.openatx.xtest.nova.runner;

import java.util.Locale;

/** Conservative recovery for unavailable hierarchy data; it never authorizes coordinate input. */
final class HierarchyFailurePolicy {
    enum Action { WAIT, BACK, RELAUNCH }

    static final class Decision {
        final Action action;
        final int attempt;
        Decision(Action action, int attempt) { this.action = action; this.attempt = attempt; }
    }

    private static final long RELAUNCH_COOLDOWN_MILLIS = 15000L;

    private String pkg = "";
    private String activity = "";
    private int failures;
    private long lastRelaunchAt;

    Decision fail(String currentActivity) {
        String current = currentActivity == null ? "" : currentActivity;
        String currentPkg = packageName(current);
        if (!currentPkg.equals(pkg)) {
            pkg = currentPkg;
            failures = 0;
        }
        activity = current;
        failures++;
        // Splash/Main dumps dying are an infrastructure failure. Back from a root
        // activity exits the app and relaunch force-stops it, which is the
        // restart-close loop seen when `uiautomator dump` is killed.
        if (isFragileRoot(current)) {
            return new Decision(Action.WAIT, failures);
        }
        long now = System.currentTimeMillis();
        if (failures >= 6 && now - lastRelaunchAt >= RELAUNCH_COOLDOWN_MILLIS) {
            Decision decision = new Decision(Action.RELAUNCH, failures);
            lastRelaunchAt = now;
            reset();
            return decision;
        }
        if (failures == 3) return new Decision(Action.BACK, failures);
        return new Decision(Action.WAIT, failures);
    }

    void reset() { activity = ""; pkg = ""; failures = 0; }

    static String packageName(String activity) {
        int slash = activity.indexOf('/');
        return slash > 0 ? activity.substring(0, slash) : activity;
    }

    static boolean isFragileRoot(String activity) {
        String name = simpleClassName(activity);
        return name.equals("splashactivity") || name.equals("startupactivity")
                || name.equals("welcomeactivity") || name.equals("welcome")
                || name.equals("loadingactivity") || name.equals("loading")
                || name.equals("launcher") || name.equals("launcheractivity")
                || name.equals("mainactivity");
    }

    static String simpleClassName(String activity) {
        String value = activity == null ? "" : activity.toLowerCase(Locale.ROOT);
        int slash = value.lastIndexOf('/');
        String className = slash >= 0 ? value.substring(slash + 1) : value;
        int dot = className.lastIndexOf('.');
        return dot >= 0 ? className.substring(dot + 1) : className;
    }

    static void selfTest() {
        HierarchyFailurePolicy policy = new HierarchyFailurePolicy();
        for (int index = 0; index < 8; index++) {
            if (policy.fail("com.example/.SplashActivity").action != Action.WAIT)
                throw new AssertionError("splash hierarchy failure must wait");
        }
        if (policy.fail("com.example/.MainActivity").action != Action.WAIT)
            throw new AssertionError("root MainActivity hierarchy failure must not back or relaunch");
        if (isFragileRoot("painting.scan.identifier.estimate.app/com.bc.artora.startup.ui.VipPaywallActivity"))
            throw new AssertionError("package path startup must not mark inner pages fragile");
        if (!isFragileRoot("painting.scan.identifier.estimate.app/com.bc.artora.SplashActivity")
                || !isFragileRoot("com.mi.android.globallauncher/com.miui.home.launcher.Launcher"))
            throw new AssertionError("splash and home launcher must stay fragile");
        policy = new HierarchyFailurePolicy();
        if (policy.fail("com.example/.startup.ui.VipPaywallActivity").action != Action.WAIT
                || policy.fail("com.example/.startup.ui.VipPaywallActivity").action != Action.WAIT
                || policy.fail("com.example/.startup.ui.VipPaywallActivity").action != Action.BACK)
            throw new AssertionError("inner startup-package page must keep Back recovery");
        policy = new HierarchyFailurePolicy();
        if (policy.fail("com.example/.DetailActivity").action != Action.WAIT || policy.fail("com.example/.DetailActivity").action != Action.WAIT)
            throw new AssertionError("hierarchy failure waited for the wrong number of attempts");
        if (policy.fail("com.example/.DetailActivity").action != Action.BACK)
            throw new AssertionError("hierarchy failure did not use bounded Back recovery");
        policy.fail("com.example/.DetailActivity");
        policy.fail("com.example/.DetailActivity");
        if (policy.fail("com.example/.DetailActivity").action != Action.RELAUNCH)
            throw new AssertionError("hierarchy failure did not finish with relaunch recovery");
        if (policy.fail("com.example/.Other").attempt != 1)
            throw new AssertionError("hierarchy failure budget did not reset across activities");
        if (policy.fail("com.example/.Other").action != Action.WAIT || policy.fail("com.example/.Other").action != Action.BACK)
            throw new AssertionError("relaunch cooldown must keep waiting instead of force-stopping again");
    }
}
