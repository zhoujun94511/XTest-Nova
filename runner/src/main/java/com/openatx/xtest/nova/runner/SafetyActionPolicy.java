package com.openatx.xtest.nova.runner;

import java.util.Locale;

/** Conservative filter for irreversible or externally fulfilled generic actions. */
final class SafetyActionPolicy {
    private static final String[] INSTALL_OR_DOWNLOAD_MARKERS = {
            "install", "download", "get app", "play store", "app store",
            "安装", "下载", "获取应用", "应用商店"
    };

    private SafetyActionPolicy() {}

    static boolean blocksGenericAction(String text, String description, String resourceId) {
        String candidate = ((text == null ? "" : text) + " " +
                (description == null ? "" : description) + " " +
                (resourceId == null ? "" : resourceId)).toLowerCase(Locale.ROOT);
        for (String marker : INSTALL_OR_DOWNLOAD_MARKERS) {
            if (candidate.contains(marker)) return true;
        }
        return false;
    }

    static void selfTest() {
        if (!blocksGenericAction("安装", "", "")) throw new AssertionError("Chinese install CTA was not blocked");
        if (!blocksGenericAction("", "Download now", "")) throw new AssertionError("download CTA was not blocked");
        if (!blocksGenericAction("", "", "com.example:id/install_button")) throw new AssertionError("install resource was not blocked");
        if (blocksGenericAction("开始游戏", "", "com.example:id/play")) throw new AssertionError("ordinary game action was blocked");
        System.out.println("SafetyActionPolicy self-test passed");
    }
}
