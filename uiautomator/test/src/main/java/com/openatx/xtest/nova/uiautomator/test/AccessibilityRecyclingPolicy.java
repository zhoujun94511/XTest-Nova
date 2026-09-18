package com.openatx.xtest.nova.uiautomator.test;

/**
 * Keeps the SDK boundary for deprecated accessibility window recycling
 * independent from Android runtime classes.
 */
public final class AccessibilityRecyclingPolicy {
    private AccessibilityRecyclingPolicy() {}

    static boolean shouldRecycleWindow(int sdkInt) {
        return sdkInt >= 28 && sdkInt < 33;
    }

    public static void main(String[] args) {
        for (int sdk = 28; sdk <= 32; sdk++) {
            if (!shouldRecycleWindow(sdk)) {
                throw new AssertionError("window recycling disabled for SDK " + sdk);
            }
        }
        if (shouldRecycleWindow(27) || shouldRecycleWindow(33) || shouldRecycleWindow(36)) {
            throw new AssertionError("deprecated window recycling crossed its SDK boundary");
        }
        System.out.println("AccessibilityRecyclingPolicy self-test passed");
    }
}
