package com.openatx.xtest.nova.runner;

import java.util.Random;

/** A conservative coordinate area that excludes system bars and edge gestures. */
final class SafeTouchRegion {
    final int width, height, left, top, right, bottom;

    SafeTouchRegion(int width, int height, int densityDpi) {
        if (width <= 0 || height <= 0) throw new IllegalArgumentException("Invalid display size");
        this.width = width;
        this.height = height;
        int dpi = densityDpi <= 0 ? 160 : densityDpi;
        int horizontalInset = dp(24, dpi);
        int verticalInset = dp(32, dpi);
        left = Math.min(horizontalInset, Math.max(0, width / 4));
        top = Math.min(verticalInset, Math.max(0, height / 4));
        right = Math.max(left + 1, width - left);
        bottom = Math.max(top + 1, height - top);
    }

    int[] randomPoint(Random random) {
        return new int[]{between(random, left, right), between(random, top, bottom)};
    }

    int[] randomSwipe(Random random) {
        int x = between(random, left, right);
        int quarter = Math.max(1, (bottom - top) / 4);
        int y1 = bottom - quarter;
        int y2 = top + quarter;
        if (random.nextBoolean()) { int value = y1; y1 = y2; y2 = value; }
        return new int[]{x, y1, x, y2};
    }

    boolean contains(int x, int y) {
        return x >= left && x < right && y >= top && y < bottom;
    }

    String zone(int x, int y) {
        int column = Math.min(2, Math.max(0, x * 3 / width));
        int row = Math.min(4, Math.max(0, y * 5 / height));
        return "r" + row + "c" + column;
    }

    int xPermille(int x) { return Math.min(1000, Math.max(0, x * 1000 / width)); }
    int yPermille(int y) { return Math.min(1000, Math.max(0, y * 1000 / height)); }

    private static int between(Random random, int minimum, int exclusiveMaximum) {
        return minimum + random.nextInt(Math.max(1, exclusiveMaximum - minimum));
    }

    private static int dp(int value, int densityDpi) {
        return Math.max(1, Math.round(value * densityDpi / 160f));
    }

    static void selfTest() {
        SafeTouchRegion region = new SafeTouchRegion(1080, 2400, 420);
        Random random = new Random(9);
        for (int index = 0; index < 1000; index++) {
            int[] point = region.randomPoint(random);
            if (!region.contains(point[0], point[1])) throw new AssertionError("tap escaped safe region");
            int[] swipe = region.randomSwipe(random);
            if (!region.contains(swipe[0], swipe[1]) || !region.contains(swipe[2], swipe[3]))
                throw new AssertionError("swipe escaped safe region");
        }
        if (!"r2c1".equals(region.zone(540, 1200)) || region.xPermille(540) != 500 || region.yPermille(1200) != 500)
            throw new AssertionError("normalized touch evidence is incorrect");
    }
}
