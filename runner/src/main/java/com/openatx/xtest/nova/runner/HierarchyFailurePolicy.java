package com.openatx.xtest.nova.runner;

/** Conservative recovery for unavailable hierarchy data; it never authorizes coordinate input. */
final class HierarchyFailurePolicy {
    enum Action { WAIT, BACK, RELAUNCH }

    static final class Decision {
        final Action action;
        final int attempt;
        Decision(Action action, int attempt) { this.action = action; this.attempt = attempt; }
    }

    private String activity = "";
    private int failures;

    Decision fail(String currentActivity) {
        String current = currentActivity == null ? "" : currentActivity;
        if (!current.equals(activity)) { activity = current; failures = 0; }
        failures++;
        if (failures >= 6) {
            Decision decision = new Decision(Action.RELAUNCH, failures);
            reset();
            return decision;
        }
        if (failures == 3) return new Decision(Action.BACK, failures);
        return new Decision(Action.WAIT, failures);
    }

    void reset() { activity = ""; failures = 0; }

    static void selfTest() {
        HierarchyFailurePolicy policy = new HierarchyFailurePolicy();
        if (policy.fail("com.example/.Main").action != Action.WAIT || policy.fail("com.example/.Main").action != Action.WAIT)
            throw new AssertionError("hierarchy failure waited for the wrong number of attempts");
        if (policy.fail("com.example/.Main").action != Action.BACK)
            throw new AssertionError("hierarchy failure did not use bounded Back recovery");
        policy.fail("com.example/.Main");
        policy.fail("com.example/.Main");
        if (policy.fail("com.example/.Main").action != Action.RELAUNCH)
            throw new AssertionError("hierarchy failure did not finish with relaunch recovery");
        if (policy.fail("com.example/.Other").attempt != 1)
            throw new AssertionError("hierarchy failure budget did not reset across activities");
    }
}
