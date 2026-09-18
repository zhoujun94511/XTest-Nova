package com.openatx.xtest.popup;

import java.util.LinkedHashSet;
import java.util.Set;

final class MonkeyDraft {
    static final String MODE_NONE = "none";
    static final String MODE_ALLOW = "allowlist";
    static final String MODE_BLOCK = "blocklist";
    static final String RENDER_OFF = "off";
    static final String RENDER_BOUNDED = "bounded";
    static final String RENDER_CONTINUOUS = "continuous";

    final String packageName;
    final String appLabel;
    int durationMinutes = 10;
    int throttleMillis = 500;
    int batteryThreshold = 15;
    String activityMode = MODE_NONE;
    String renderFallbackMode = RENDER_BOUNDED;
    final Set<String> activities = new LinkedHashSet<>();
    final Set<String> targetActivities = new LinkedHashSet<>();
    final Set<String> blockedControls = new LinkedHashSet<>();
    String targetCases = "";

    MonkeyDraft(String packageName, String appLabel) {
        this.packageName = packageName;
        this.appLabel = appLabel == null || appLabel.trim().isEmpty() ? packageName : appLabel.trim();
    }

    String allowedActivities() { return MODE_ALLOW.equals(activityMode) ? join(activities) : ""; }
    String blockedActivities() { return MODE_BLOCK.equals(activityMode) ? join(activities) : ""; }
    String targets() { return join(targetActivities); }
    String blockedControls() { return join(blockedControls); }

    String scopeSummary() {
        if (MODE_ALLOW.equals(activityMode)) return "仅包含已选 Activity（" + activities.size() + "）";
        if (MODE_BLOCK.equals(activityMode)) return "排除已选 Activity（" + activities.size() + "）";
        return "不限制 Activity（推荐）";
    }

    String targetSummary() {
        return targetActivities.isEmpty() ? "未设置（可选）" : "已选择 " + targetActivities.size() + " 个目标页面";
    }

    String blockedControlSummary() {
        return blockedControls.isEmpty() ? "未设置" : "已添加 " + blockedControls.size() + " 条";
    }

    String renderFallbackSummary() {
        if (RENDER_OFF.equals(renderFallbackMode)) return "关闭";
        if (RENDER_CONTINUOUS.equals(renderFallbackMode)) return "持续探索（游戏）";
        return "有界兜底（推荐）";
    }

    private static String join(Set<String> values) {
        StringBuilder result = new StringBuilder();
        for (String value : values) {
            if (result.length() > 0) result.append(',');
            result.append(value);
        }
        return result.toString();
    }
}
