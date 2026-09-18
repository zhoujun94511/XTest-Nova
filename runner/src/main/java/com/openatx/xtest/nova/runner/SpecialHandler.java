package com.openatx.xtest.nova.runner;

import java.util.*;
import java.util.regex.Matcher;

/** Handles ads, billing, permissions and external-system interruptions before exploration input. */
final class SpecialHandler {
    private static final int MAX_EXTERNAL_RECOVERIES = 3;
    private static final String[] PERMISSION_PACKAGES = {"com.android.permissioncontroller", "com.google.android.permissioncontroller", "com.android.packageinstaller", "com.google.android.packageinstaller", "com.miui.packageinstaller", "com.miui.securitycenter", "com.samsung.android.permissioncontroller"};
    private static final String[] REVIEW_PACKAGES = {"com.android.vending"};
    private static final String[] SYSTEM_PICKER_PACKAGES = {"com.android.documentsui", "com.google.android.documentsui", "com.google.android.photopicker", "com.android.providers.media.module"};
    private static final String[] SYSTEM_SETTINGS_PACKAGES = {"com.android.settings", "com.google.android.settings", "com.samsung.android.settings"};
    private static final String[] CONSENT_MARKERS = {"terms of service", "terms and conditions", "privacy policy", "user agreement", "用户协议", "服务条款", "使用条款", "隐私政策", "隐私条款"};
    private static final String[] CONSENT_INTENT_MARKERS = {"by tapping", "by clicking", "agree to", "i agree", "accept the terms", "我已阅读并同意", "同意并继续", "点击继续即表示"};
    static final String[] CONSENT_TARGETS = {"agree", "accept", "continue", "i agree", "同意", "接受", "继续"};
    private static final String[] PERMISSION_TARGETS = {"allow", "while using the app", "only this time", "允许", "使用应用时", "仅此一次"};
    private static final String[] PAYWALL_MARKERS = {"subscription", "subscribe", "free trial", "start trial", "restore purchase", "premium", "top up", "recharge", "coin", "points", "payment", "one-time purchase", "订阅", "免费试用", "开始试用", "恢复购买", "高级版", "会员", "充值", "金币", "积分", "支付"};
    private static final String[] REVIEW_MARKERS = {"rate this app", "reviews are only visible", "first star", "评价此应用", "评分", "撰写评价"};
    private static final String[] PURCHASE_MARKERS = {"subscribe", "confirm purchase", "payment method", "订阅", "确认购买", "付款方式"};
    static final String[] TARGET_PURCHASE_CONFIRMATION_MARKERS = {"pay now", "order info", "one-time purchase", "confirm payment", "place order", "立即支付", "订单信息", "确认支付", "提交订单"};
    private static final String[] BILLING_ERROR_MARKERS = {"not configured for google play billing", "未配置为通过 google play 结算"};
    private static final String[] BILLING_ERROR_TARGETS = {"ok", "got it", "知道了", "确定"};
    private static final String[] DISMISS_TARGETS = {"close", "dismiss", "not now", "no thanks", "later", "skip", "cancel", "x", "关闭", "稍后", "不了", "跳过", "取消", "暂不"};
    private static final String[] RESOLVER_MARKERS = {"open with", "complete action using", "just once", "打开方式", "仅此一次", "始终使用选择的应用程序"};
    final Main.Config config;
    final Main.ShellInput input;
    final Map<String,Integer> attempts = new HashMap<>();
    final Map<String,Long> attemptAt = new HashMap<>();
    boolean adActive;
    int adEncounter;
    final Map<String,Long> adWaitStarted = new HashMap<>();
    boolean suppressRandom;
    boolean paywallContext;

    SpecialHandler(Main.Config config, Main.ShellInput input) { this.config = config; this.input = input; }

    /** Pure classifier for post-coordinate evidence. It never authorizes or performs an action. */
    static String classifyEvidence(String targetPackage, String activity, String xml) {
        String foreground = activityPackage(activity);
        String page = xml == null ? "" : xml.toLowerCase(Locale.ROOT);
        if (AdScenePolicy.isKnownActivity(activity) || AdScenePolicy.hasContentEvidence(page)) return "ad";
        if (!targetPackage.equals(foreground)) {
            if (contains(PERMISSION_PACKAGES, foreground)) return "permission";
            if (contains(SYSTEM_PICKER_PACKAGES, foreground)) return "system_picker";
            if (contains(SYSTEM_SETTINGS_PACKAGES, foreground)) return "system_settings";
            if (contains(REVIEW_PACKAGES, foreground)) return containsAny(page, PURCHASE_MARKERS) ? "purchase_confirmation" : "review";
            if ("android".equals(foreground)) return "system_resolver";
            return "external_package";
        }
        if (containsAny(page, TARGET_PURCHASE_CONFIRMATION_MARKERS)) return "purchase_confirmation";
        if (containsAny(page, PAYWALL_MARKERS)) return "paywall";
        if (containsAny(page, CONSENT_MARKERS) && containsAny(page, CONSENT_INTENT_MARKERS)) return "consent";
        return "";
    }

    static String adAttemptKey(int encounter, String activity, String phase) {
        return "ad|" + encounter + "|" + (activity == null ? "" : activity) + "|" + phase;
    }

    String preflight(String activity) {
        try {
            String foreground = activityPackage(activity);
            if (config.packageName.equals(foreground)) resetExternalRecoveryBudget();
            boolean adActivity = AdScenePolicy.isKnownActivity(activity);
            boolean knownController = contains(PERMISSION_PACKAGES, foreground) || contains(REVIEW_PACKAGES, foreground) ||
                    contains(SYSTEM_PICKER_PACKAGES, foreground) || contains(SYSTEM_SETTINGS_PACKAGES, foreground) || "android".equals(foreground);
            if (!config.packageName.equals(foreground) && !knownController && !adActivity) return recoverExternal(activity, "external_package");
            if (contains(SYSTEM_PICKER_PACKAGES, foreground)) return recoverExternal(activity, "system_picker");
            if (contains(SYSTEM_SETTINGS_PACKAGES, foreground)) return recoverExternal(activity, "system_settings");
        } catch (Exception error) {
            emit("special_fallback", activity, "unknown", "safe", ",\"error\":\"" + Main.Guard.escape(String.valueOf(error.getMessage())) + "\"");
        }
        return null;
    }

    String handle(String activity, String xml) {
        suppressRandom = false;
        paywallContext = false;
        try {
            String foreground = activityPackage(activity);
            boolean permission = contains(PERMISSION_PACKAGES, foreground);
            boolean review = contains(REVIEW_PACKAGES, foreground);
            boolean adActivity = AdScenePolicy.isKnownActivity(activity);
            boolean external = !config.packageName.equals(foreground);
            boolean systemPicker = external && contains(SYSTEM_PICKER_PACKAGES, foreground);
            boolean systemSettings = external && contains(SYSTEM_SETTINGS_PACKAGES, foreground);
            boolean potentialResolver = external && "android".equals(foreground);
            if (!permission && !review && !systemPicker && !systemSettings && !potentialResolver && !adActivity && external) {
                return recoverExternal(activity, "external_package");
            }
            List<Main.NodeExplorer.Node> nodes = new ArrayList<>();
            StringBuilder page = new StringBuilder();
            Matcher tags = Main.NodeExplorer.NODE_TAG.matcher(xml);
            while (tags.find()) {
                String tag = tags.group();
                if (tag.startsWith("</")) continue;
                Main.NodeExplorer.Node node = Main.NodeExplorer.Node.parse(tag);
                if (node == null || (!adActivity && !node.value("package").isEmpty() && !foreground.equals(node.value("package")))) continue;
                nodes.add(node);
                page.append(' ').append(node.value("text")).append(' ').append(node.value("content-desc")).append(' ').append(node.value("resource-id"));
            }
            String normalizedPage = page.toString().toLowerCase(Locale.ROOT);
            if (systemPicker) {
                return recoverExternal(activity, "system_picker");
            }
            if (systemSettings) {
                return recoverExternal(activity, "system_settings");
            }
            if (potentialResolver) {
                if (containsAny(normalizedPage, RESOLVER_MARKERS)) {
                    input.key("KEYCODE_BACK");
                    emit("special_action", activity, "system_resolver", "dismiss", ",\"gesture\":\"back\"");
                    return "skip";
                }
                return recoverExternal(activity, "external_package");
            }
            if (permission) return act(nodes, PERMISSION_TARGETS, "permission", "allow", activity, normalizedPage);
            if (review) {
                if (containsAny(normalizedPage, REVIEW_MARKERS)) return act(nodes, DISMISS_TARGETS, "review", "dismiss", activity, normalizedPage);
                if (containsAny(normalizedPage, BILLING_ERROR_MARKERS)) return act(nodes, BILLING_ERROR_TARGETS, "billing_error", "dismiss", activity, normalizedPage);
                if (containsAny(normalizedPage, PURCHASE_MARKERS)) {
                    input.key("KEYCODE_BACK");
                    emit("special_action", activity, "purchase_confirmation", "dismiss", ",\"gesture\":\"back\"");
                    return "skip";
                }
                return recoverExternal(activity, "external_package");
            }
            if (!external && containsAny(normalizedPage, TARGET_PURCHASE_CONFIRMATION_MARKERS)) {
                return act(nodes, DISMISS_TARGETS, "purchase_confirmation", "dismiss", activity, normalizedPage);
            }
            if (containsAny(normalizedPage, CONSENT_MARKERS) && containsAny(normalizedPage, CONSENT_INTENT_MARKERS)) {
                Main.NodeExplorer.Node target = find(nodes, CONSENT_TARGETS);
                if (target != null) return act(target, "consent", "accept", activity, normalizedPage);
            }
            if ((normalizedPage.contains("skip") || normalizedPage.contains("跳过")) && (normalizedPage.contains("continue") || normalizedPage.contains("继续"))) {
                Main.NodeExplorer.Node target = find(nodes, new String[]{"continue", "next", "继续", "下一步"});
                String key = "onboarding|" + Main.NodeExplorer.SceneSnapshot.digest(normalizedPage);
                if (target != null && !attempts.containsKey(key)) return act(target, "onboarding", "advance", activity, normalizedPage);
                return swipe(nodes, activity, normalizedPage);
            }
            boolean detectedAd = adActivity || AdScenePolicy.hasContentEvidence(normalizedPage);
            if (detectedAd) {
                if (!adActive) { adActive = true; adEncounter++; }
                AdScenePolicy.Scene adScene = AdScenePolicy.classify(activity, normalizedPage);
                Main.NodeExplorer.Node target = find(nodes, DISMISS_TARGETS);
                Main.NodeExplorer.Node explicitTarget = AdScenePolicy.findExplicitDismiss(nodes, adActivity ? "" : foreground, adScene.phase);
                if (explicitTarget != null) target = explicitTarget;
                if (target == null && adActivity) target = AdScenePolicy.findCornerDismiss(nodes, "");
                if (target != null) {
                    adWaitStarted.remove(activity);
                    String phaseKey = adAttemptKey(adEncounter, activity, adScene.phase);
                    String result = actAt(target, AdScenePolicy.dismissX(target), target.centerY(), "ad", "dismiss", activity, normalizedPage, phaseKey);
                    emit("ad_transition", activity, "ad", "dismiss", ",\"adType\":\"" + adScene.type + "\",\"adPhase\":\"" + adScene.phase + "\"");
                    return result;
                }
                if (adActivity) {
                    long now = System.currentTimeMillis();
                    Long started = adWaitStarted.get(activity);
                    if (started == null) { started = now; adWaitStarted.put(activity, started); }
                    suppressRandom = true;
                    if (now - started < adScene.waitMillis) {
                        emit("special_wait", activity, "ad", "dismiss", ",\"adType\":\"" + adScene.type + "\",\"adPhase\":\"" + adScene.phase + "\",\"elapsedMillis\":" + (now - started) + ",\"limitMillis\":" + adScene.waitMillis);
                        return "skip";
                    }
                    adWaitStarted.remove(activity);
                    String attemptKey = "ad-back|" + Main.NodeExplorer.SceneSnapshot.digest(normalizedPage);
                    int attempt = attempts.containsKey(attemptKey) ? attempts.get(attemptKey) : 0;
                    if (attempt >= 1) {
                        attempts.remove(attemptKey);
                        adWaitStarted.remove(activity);
                        adActive = false;
                        return relaunchTarget(activity, "ad", attemptKey, "dismiss_timeout_exhausted");
                    }
                    attempts.put(attemptKey, attempt + 1);
                    input.key("KEYCODE_BACK");
                    adWaitStarted.put(activity, now - Math.max(0L, adScene.waitMillis - 1500L));
                    emit("special_action", activity, "ad", "dismiss", ",\"gesture\":\"back\",\"reason\":\"dismiss_timeout\",\"attempt\":1,\"limit\":1,\"relaunchGraceMillis\":1500");
                    return "skip";
                }
            } else {
                adActive = false;
            }
            if (containsAny(normalizedPage, PAYWALL_MARKERS)) {
                suppressRandom = true;
                paywallContext = true;
            }
        } catch (Exception error) {
            emit("special_fallback", activity, "unknown", "safe", ",\"error\":\"" + Main.Guard.escape(String.valueOf(error.getMessage())) + "\"");
        }
        return null;
    }

    String recoverExternal(String activity, String kind) throws Exception {
        String foreground = activityPackage(activity);
        String key = "external|" + foreground;
        int count = attempts.containsKey(key) ? attempts.get(key) : 0;
        boolean launcher = foreground.toLowerCase(Locale.ROOT).contains("launcher") ||
                (activity != null && activity.toLowerCase(Locale.ROOT).contains("launcher"));
        if (launcher) {
            input.wakeAndDismissKeyguard();
            input.run("monkey", "-p", config.packageName, "-c", "android.intent.category.LAUNCHER", "1");
            Thread.sleep(Math.max(300, 300));
            if (config.packageName.equals(activityPackage(Main.Guard.currentActivity(input)))) {
                emit("special_action", activity, kind, "recover", ",\"gesture\":\"launch\",\"reason\":\"launcher_recovery\"");
                return "skip";
            }
            return relaunchTarget(activity, kind, key, "launcher_recovery");
        }
        if (count >= MAX_EXTERNAL_RECOVERIES) {
            return relaunchTarget(activity, kind, key, "back_recovery_exhausted");
        }
        attempts.put(key, count + 1);
        input.key("KEYCODE_BACK");
        emit("special_action", activity, kind, "recover", ",\"gesture\":\"back\",\"attempt\":" + (count + 1) + ",\"limit\":" + MAX_EXTERNAL_RECOVERIES);
        return "skip";
    }

    String relaunchTarget(String activity, String kind, String key, String reason) throws Exception {
        input.wakeAndDismissKeyguard();
        long requestedAt = System.currentTimeMillis();
        emit("relaunch_requested", activity, kind, "recover",
                ",\"reason\":\"" + reason + "\",\"targetPackage\":\"" + config.packageName + "\"");
        input.run("am", "force-stop", config.packageName);
        Thread.sleep(300);
        input.run("monkey", "-p", config.packageName, "-c", "android.intent.category.LAUNCHER", "1");
        attempts.remove(key);
        if ("ad".equals(kind)) resetAdRecoveryState();
        emit("relaunch_completed", activity, kind, "recover",
                ",\"reason\":\"" + reason + "\",\"targetPackage\":\"" + config.packageName +
                "\",\"durationMillis\":" + (System.currentTimeMillis() - requestedAt));
        emit("special_action", activity, kind, "recover",
                ",\"gesture\":\"relaunch\",\"reason\":\"" + reason + "\"");
        return "skip";
    }

    void resetAdRecoveryState() {
        String prefix = "ad|" + adEncounter + "|";
        attempts.keySet().removeIf(value -> value.startsWith(prefix) || value.startsWith("ad-back|"));
        attemptAt.keySet().removeIf(value -> value.startsWith(prefix));
        adWaitStarted.clear();
        adActive = false;
    }

    void resetExternalRecoveryBudget() {
        Iterator<String> keys = attempts.keySet().iterator();
        while (keys.hasNext()) if (keys.next().startsWith("external|")) keys.remove();
    }

    String act(List<Main.NodeExplorer.Node> nodes, String[] targets, String kind, String policy, String activity, String page) throws Exception {
        Main.NodeExplorer.Node target = find(nodes, targets);
        if (target == null) {
            if ("permission".equals(kind)) {
                String key = "permission-wait|" + activity;
                int count = attempts.containsKey(key) ? attempts.get(key) : 0;
                if (count < 10) {
                    attempts.put(key, count + 1);
                    emit("special_wait", activity, kind, policy, ",\"reason\":\"controls_loading\",\"attempt\":" + (count + 1) + ",\"limit\":10");
                    return "skip";
                }
            }
            if ("review".equals(kind)) {
                input.key("KEYCODE_BACK");
                emit("special_action", activity, kind, policy, ",\"gesture\":\"back\",\"reason\":\"no_explicit_dismiss_control\"");
                return "skip";
            }
            emit("special_unhandled", activity, kind, policy, ",\"recovery\":\"relaunch\"");
            return relaunchTarget(activity, kind, kind + "|unhandled", "controls_unavailable");
        }
        return act(target, kind, policy, activity, page);
    }

    String act(Main.NodeExplorer.Node target, String kind, String policy, String activity, String page) throws Exception {
        return actAt(target, target.centerX(), target.centerY(), kind, policy, activity, page);
    }

    String actAt(Main.NodeExplorer.Node target, int x, int y, String kind, String policy, String activity, String page) throws Exception {
        return actAt(target, x, y, kind, policy, activity, page, null);
    }

    String actAt(Main.NodeExplorer.Node target, int x, int y, String kind, String policy, String activity, String page, String scopedKey) throws Exception {
        String key = scopedKey != null ? scopedKey : ("ad".equals(kind) ? "ad|" + adEncounter + "|" + activity : kind + "|" + Main.NodeExplorer.SceneSnapshot.digest(page));
        int count = attempts.containsKey(key) ? attempts.get(key) : 0;
        long now = System.currentTimeMillis();
        Long previous = attemptAt.get(key);
        if (count > 0 && previous != null && now - previous < 2000L) {
            emit("special_wait", activity, kind, policy, ",\"reason\":\"action_cooldown\",\"remainingMillis\":" + (2000L - (now - previous)));
            return "skip";
        }
        if (count >= 3) {
            if ("ad".equals(kind)) {
                emit("special_attempt_limit", activity, kind, policy, ",\"recovery\":\"relaunch\",\"adPhaseKey\":\"" + Main.Guard.escape(key) + "\"");
                return relaunchTarget(activity, kind, key, "dismiss_attempts_exhausted");
            }
            emit("special_attempt_limit", activity, kind, policy, ",\"recovery\":\"relaunch\"");
            return relaunchTarget(activity, kind, key, "action_attempts_exhausted");
        }
        attempts.put(key, count + 1);
        attemptAt.put(key, now);
        input.tap(x, y);
        emit("special_action", activity, kind, policy, ",\"node\":\"" + Main.Guard.escape(target.key()) + "\"");
        return "skip";
    }

    String swipe(List<Main.NodeExplorer.Node> nodes, String activity, String page) throws Exception {
        Main.NodeExplorer.Node largest = null;
        long largestArea = 0;
        for (Main.NodeExplorer.Node node : nodes) {
            long area = (long)(node.right - node.left) * (node.bottom - node.top);
            if (area > largestArea) { largest = node; largestArea = area; }
        }
        if (largest == null || largest.right - largest.left < 200) return relaunchTarget(activity, "onboarding", "onboarding|invalid", "no_swipe_surface");
        String key = "onboarding|" + Main.NodeExplorer.SceneSnapshot.digest(page);
        int count = attempts.containsKey(key) ? attempts.get(key) : 0;
        if (count >= 3) return relaunchTarget(activity, "onboarding", key, "advance_attempts_exhausted");
        attempts.put(key, count + 1);
        int y = largest.top + (largest.bottom - largest.top) / 2;
        int startX = largest.left + (largest.right - largest.left) * 4 / 5;
        int endX = largest.left + (largest.right - largest.left) / 5;
        input.swipe(startX, y, endX, y);
        emit("special_action", activity, "onboarding", "advance", ",\"gesture\":\"swipe_left\"");
        return "skip";
    }

    static Main.NodeExplorer.Node find(List<Main.NodeExplorer.Node> nodes, String[] targets) {
        for (Main.NodeExplorer.Node node : nodes) {
            if ("android.webkit.WebView".equals(node.value("class"))) continue;
            if (!node.validArea() || !"true".equals(node.value("clickable")) || !node.eligible(Collections.<String>emptyList())) continue;
            String label = (node.value("text") + " " + node.value("content-desc") + " " + node.value("resource-id")).trim().toLowerCase(Locale.ROOT);
            if (matchesLabel(label, targets)) return node;
        }
        for (Main.NodeExplorer.Node labelNode : nodes) {
            String label = (labelNode.value("text") + " " + labelNode.value("content-desc") + " " + labelNode.value("resource-id")).trim().toLowerCase(Locale.ROOT);
            if (!matchesLabel(label, targets)) continue;
            Main.NodeExplorer.Node best = null;
            long bestArea = Long.MAX_VALUE;
            int x = labelNode.centerX(), y = labelNode.centerY();
            for (Main.NodeExplorer.Node candidate : nodes) {
                if ("android.webkit.WebView".equals(candidate.value("class"))) continue;
                if (!candidate.validArea() || !"true".equals(candidate.value("clickable")) || !candidate.eligible(Collections.<String>emptyList())) continue;
                if (x < candidate.left || x > candidate.right || y < candidate.top || y > candidate.bottom) continue;
                long area = (long)(candidate.right - candidate.left) * (candidate.bottom - candidate.top);
                if (area < bestArea) { best = candidate; bestArea = area; }
            }
            if (best != null) return best;
        }
        return null;
    }

    static boolean matchesLabel(String label, String[] targets) {
        for (String target : targets) if (label.equals(target) || (target.length() > 2 && label.contains(target))) return true;
        return false;
    }

    static boolean containsAny(String value, String[] candidates) { for (String candidate : candidates) if (value.contains(candidate)) return true; return false; }
    static boolean contains(String[] values, String candidate) { for (String value : values) if (value.equals(candidate)) return true; return false; }
    static String activityPackage(String activity) { int slash = activity == null ? -1 : activity.indexOf('/'); return slash < 0 ? "" : activity.substring(0, slash); }
    void emit(String state, String activity, String kind, String policy, String extra) { System.out.println("{\"state\":\""+state+"\",\"requestId\":\""+Main.Guard.escape(config.requestId)+"\",\"package\":\""+config.packageName+"\",\"activity\":\""+Main.Guard.escape(activity == null ? "" : activity)+"\",\"specialKind\":\""+kind+"\",\"policy\":\""+policy+"\""+extra+",\"time\":"+System.currentTimeMillis()+"}"); }
}
