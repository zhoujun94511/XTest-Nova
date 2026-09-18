package com.openatx.xtest.nova.runner;

import java.util.List;
import java.util.Locale;

/** Conservative full-screen advertising recognition and dismiss-control selection. */
final class AdScenePolicy {
    static final class Scene {
        final String type, phase;
        final long waitMillis;
        Scene(String type, String phase, long waitMillis) { this.type = type; this.phase = phase; this.waitMillis = waitMillis; }
    }

    private static final String[] ACTIVITY_MARKERS = {
            "com.google.android.gms.ads.adactivity", "com.bytedance.sdk.openadsdk.activity.", "com.qq.e.ads.",
            "com.applovin.adview.", "com.unity3d.services.ads.adunit.", "com.ironsource.sdk.controller.",
            "com.inmobi.ads.rendering.", "com.mbridge.msdk.", "com.vungle.ads.", "com.chartboost.sdk.",
            "com.sigmob.sdk.", "sg.bigo.ads.", "com.huawei.openalliance.ad.", "com.miui.systemadsolution.",
            "com.kwad.sdk.", "com.baidu.mobads.", "com.anythink.", "com.tradplus.", "com.fyber.",
            "com.digitalturbine.", "com.smaato.", "com.moloco.sdk.", "com.yandex.mobile.ads."
    };
    private static final String[] CONTENT_MARKERS = {
            "advertisement", "sponsored", "skip ad", "close ad", "广告", "跳过广告"
    };
    private static final String[] DISMISS_MARKERS = {
            "close", "dismiss", "not now", "no thanks", "maybe later", "later", "skip", "cancel", "关闭", "稍后", "以后再说", "不了", "跳过", "取消", "暂不"
    };
    private static final String[] REWARD_MARKERS = {"rewarded", "reward video", "watch video", "lose reward", "lose your reward", "奖励视频", "观看视频", "获得奖励", "放弃奖励"};
    private static final String[] PLAYABLE_MARKERS = {"play now", "try now", "试玩", "立即试玩", "立即游玩"};
    private static final String[] APP_OPEN_MARKERS = {"continue to app", "continue to application", "继续打开应用", "进入应用"};
    private static final String[] CONFIRM_MARKERS = {"lose reward", "lose your reward", "give up reward", "continue watching", "放弃奖励", "继续观看", "确认退出", "仍要退出"};
    private static final String[] END_CARD_MARKERS = {"install now", "download now", "get app", "open app", "立即安装", "立即下载", "打开应用"};
    private static final String[] EXIT_MARKERS = {"close", "close ad", "end ad", "give up", "give up reward", "exit", "leave", "close anyway", "关闭", "关闭广告", "放弃", "放弃奖励", "退出", "仍要退出", "确认退出"};
    private static final String[] CONTINUE_MARKERS = {"resume", "continue", "continue watching", "keep watching", "继续", "继续观看", "继续播放"};

    private AdScenePolicy() {}

    static boolean isKnownActivity(String activity) {
        String value = normalized(activity);
        for (String marker : ACTIVITY_MARKERS) if (value.contains(marker)) return true;
        return false;
    }

    static boolean hasContentEvidence(String page) {
        String value = normalized(page);
        for (String marker : CONTENT_MARKERS) if (value.contains(marker)) return true;
        return false;
    }

    static Scene classify(String activity, String page) {
        String activityValue = normalized(activity), pageValue = normalized(page);
        String type = "interstitial";
        long wait = 8_000L;
        if (containsAny(pageValue, APP_OPEN_MARKERS) || activityValue.contains("appopen") || activityValue.contains("splash")) { type = "app_open"; wait = 8_000L; }
        else if (containsAny(pageValue, REWARD_MARKERS) || activityValue.contains("reward")) { type = "rewarded_video"; wait = 15_000L; }
        else if (containsAny(pageValue, PLAYABLE_MARKERS) || activityValue.contains("playable")) { type = "playable_fullscreen"; wait = 10_000L; }
        String phase = "waiting_close";
        if (containsAny(pageValue, CONFIRM_MARKERS)) phase = "confirm_exit";
        else if (containsAny(pageValue, END_CARD_MARKERS) && !containsAny(pageValue, APP_OPEN_MARKERS)) phase = "end_card";
        else if (containsDismissMarker(pageValue) || pageValue.contains("close-button")) phase = "close_ready";
        return new Scene(type, phase, wait);
    }

    static Main.NodeExplorer.Node findExplicitDismiss(List<Main.NodeExplorer.Node> nodes, String foreground, String phase) {
        String[] targets = "confirm_exit".equals(phase) ? EXIT_MARKERS : DISMISS_MARKERS;
        Main.NodeExplorer.Node viewport = largest(nodes, foreground), best = null;
        long bestArea = Long.MAX_VALUE;
        int bestScore = -1;
        for (Main.NodeExplorer.Node node : nodes) {
            if (!sameWindow(node, foreground) || !node.validArea() || "false".equals(node.value("enabled")) || "false".equals(node.value("visible-to-user"))) continue;
            String label = normalized(node.value("text") + " " + node.value("content-desc") + " " + node.value("resource-id"));
            if ("confirm_exit".equals(phase) && containsAny(label, CONTINUE_MARKERS)) continue;
            boolean resourceMatch = node.value("resource-id").toLowerCase(Locale.ROOT).matches(".*(close|dismiss|cancel|skip|not_now|later).*");
            boolean textMatch = containsAny(label, targets) || label.contains("close and continue to app") || label.contains("关闭广告并继续打开应用");
            if (!resourceMatch && !textMatch) continue;
            long area = area(node);
            if (viewport != null && area > area(viewport) / 4) continue;
            int score = (resourceMatch ? 200 : 0) + (textMatch ? 100 : 0) + (exactMarker(label, targets) ? 100 : 0) + ("true".equals(node.value("clickable")) ? 20 : 0);
            if (score > bestScore || score == bestScore && area < bestArea) { best = node; bestArea = area; bestScore = score; }
        }
        return best;
    }

    static int dismissX(Main.NodeExplorer.Node node) {
        if (node.value("resource-id").toLowerCase(Locale.ROOT).contains("close") && node.right - node.left > 2 * (node.bottom - node.top)) {
            return node.right - (node.bottom - node.top) / 2;
        }
        return node.centerX();
    }

    static boolean isEmbeddedNode(Main.NodeExplorer.Node node) {
        String value = normalized(node.value("resource-id") + " " + node.value("content-desc") + " " + node.value("class"));
        return value.contains("advertisement-card") || value.contains("ad_iframe") || value.contains("native_ad") ||
                value.contains("nativead") || value.contains("banner_ad") || value.contains("bannerad") ||
                value.contains("ad_container") || value.contains("adcontainer") || value.contains("ad_view") ||
                value.contains("adview") || value.contains(":id/ad_") || value.contains("/id/ad_");
    }

    static Main.NodeExplorer.Node findCornerDismiss(List<Main.NodeExplorer.Node> nodes, String foreground) {
        Main.NodeExplorer.Node viewport = largest(nodes, foreground);
        if (viewport == null || viewport.right - viewport.left < 400 || viewport.bottom - viewport.top < 600) return null;
        int width = viewport.right - viewport.left, height = viewport.bottom - viewport.top;
        long maxArea = (long) width * height / 30;
        Main.NodeExplorer.Node best = null;
        int bestScore = -1;
        for (Main.NodeExplorer.Node node : nodes) {
            if (!sameWindow(node, foreground) || !node.validArea() || !"true".equals(node.value("clickable")) ||
                    !"true".equals(node.value("enabled")) || "false".equals(node.value("visible-to-user")) ||
                    "true".equals(node.value("password")) || "android.webkit.WebView".equals(node.value("class"))) continue;
            int itemWidth = node.right - node.left, itemHeight = node.bottom - node.top;
            long area = area(node);
            int x = node.centerX(), y = node.centerY();
            boolean topBand = y <= viewport.top + height / 4;
            boolean corner = x <= viewport.left + width / 4 || x >= viewport.right - width / 4;
            if (!topBand || !corner || area < 144 || area > maxArea || itemWidth > width / 4 || itemHeight > height / 5) continue;
            String label = normalized(node.value("text") + " " + node.value("content-desc") + " " + node.value("resource-id"));
            boolean explicit = containsDismissMarker(label);
            int score = 10 + (explicit ? 100 : 0) + (label.isEmpty() ? 20 : 0) + (area < maxArea / 3 ? 10 : 0);
            if (score > bestScore) { best = node; bestScore = score; }
        }
        return best;
    }

    private static boolean sameWindow(Main.NodeExplorer.Node node, String foreground) {
        String packageName = node.value("package");
        return packageName.isEmpty() || foreground == null || foreground.isEmpty() || foreground.equals(packageName);
    }

    private static Main.NodeExplorer.Node largest(List<Main.NodeExplorer.Node> nodes, String foreground) {
        Main.NodeExplorer.Node viewport = null;
        long viewportArea = 0;
        for (Main.NodeExplorer.Node node : nodes) {
            if (!sameWindow(node, foreground) || !node.validArea()) continue;
            long area = area(node);
            if (area > viewportArea) { viewport = node; viewportArea = area; }
        }
        return viewport;
    }

    private static long area(Main.NodeExplorer.Node node) {
        return (long) (node.right - node.left) * (node.bottom - node.top);
    }

    private static boolean containsDismissMarker(String value) {
        for (String marker : DISMISS_MARKERS) {
            if (value.equals(marker) || (marker.length() > 2 && value.contains(marker))) return true;
        }
        return false;
    }

    private static boolean containsAny(String value, String[] markers) {
        for (String marker : markers) if (value.contains(marker)) return true;
        return false;
    }

    private static boolean exactMarker(String value, String[] markers) {
        String text = value.replaceAll("[\\s?？!！:：]+", " ").trim();
        for (String marker : markers) if (text.equals(marker)) return true;
        return false;
    }

    private static String normalized(String value) {
        return value == null ? "" : value.trim().toLowerCase(Locale.ROOT);
    }

    static void selfTest() {
        if (!isKnownActivity("com.example/com.google.android.gms.ads.AdActivity")) throw new AssertionError("AdMob activity was not recognized");
        if (!isKnownActivity("com.example/com.mbridge.msdk.reward.player.MBRewardVideoActivity")) throw new AssertionError("Mintegral activity was not recognized");
        if (isKnownActivity("com.example/.MainActivity")) throw new AssertionError("ordinary activity was classified as an ad");
        Main.NodeExplorer.Node root = Main.NodeExplorer.Node.parse("<node package=\"com.example\" class=\"android.widget.FrameLayout\" bounds=\"[0,0][1080,2400]\" enabled=\"true\" visible-to-user=\"true\"/>");
        Main.NodeExplorer.Node corner = Main.NodeExplorer.Node.parse("<node package=\"com.example\" class=\"android.widget.ImageView\" bounds=\"[972,48][1056,132]\" clickable=\"true\" enabled=\"true\" visible-to-user=\"true\"/>");
        if (findCornerDismiss(java.util.Arrays.asList(root, corner), "com.example") != corner) throw new AssertionError("top-corner icon was not selected");
        Scene appOpen = classify("com.example/com.google.android.gms.ads.AdActivity", "关闭广告并继续打开应用 Play Now");
        if (!"app_open".equals(appOpen.type) || appOpen.waitMillis != 8_000L) throw new AssertionError("app-open ad was not classified");
        Scene interstitial = classify("com.example/com.applovin.adview.AppLovinFullscreenActivity", "advertisement");
        if (!"interstitial".equals(interstitial.type) || interstitial.waitMillis != 8_000L) throw new AssertionError("interstitial recovery budget changed");
        Scene rewarded = classify("com.example/com.applovin.adview.AppLovinFullscreenActivity", "rewarded video");
        if (!"rewarded_video".equals(rewarded.type) || rewarded.waitMillis != 15_000L) throw new AssertionError("rewarded recovery budget changed");
        Scene confirmation = classify("com.example/com.google.android.gms.ads.AdActivity", "Close Ad? You will lose your reward CLOSE RESUME");
        if (!"confirm_exit".equals(confirmation.phase)) throw new AssertionError("reward-loss confirmation was not classified");
        Main.NodeExplorer.Node close = Main.NodeExplorer.Node.parse("<node package=\"com.google.android.gms\" class=\"android.widget.Button\" text=\"CLOSE\" clickable=\"true\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[400,1200][600,1320]\"/>");
        Main.NodeExplorer.Node resume = Main.NodeExplorer.Node.parse("<node package=\"com.google.android.gms\" class=\"android.widget.Button\" text=\"RESUME\" clickable=\"true\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[620,1200][900,1320]\"/>");
        Main.NodeExplorer.Node title = Main.NodeExplorer.Node.parse("<node package=\"com.google.android.gms\" class=\"android.widget.TextView\" text=\"Close Ad?\" clickable=\"false\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[180,900][800,1020]\"/>");
        if (findExplicitDismiss(java.util.Arrays.asList(root, title, resume, close), "", confirmation.phase) != close) throw new AssertionError("close was not preferred over resume on reward confirmation");
        Main.NodeExplorer.Node closeZh = Main.NodeExplorer.Node.parse("<node package=\"com.google.android.gms\" class=\"android.widget.Button\" text=\"关闭\" clickable=\"true\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[400,1200][600,1320]\"/>");
        Main.NodeExplorer.Node continueZh = Main.NodeExplorer.Node.parse("<node package=\"com.google.android.gms\" class=\"android.widget.Button\" text=\"继续\" clickable=\"true\" enabled=\"true\" visible-to-user=\"true\" bounds=\"[620,1200][900,1320]\"/>");
        if (findExplicitDismiss(java.util.Arrays.asList(root, continueZh, closeZh), "", "confirm_exit") != closeZh) throw new AssertionError("Chinese close was not preferred over continue");
    }
}
