package com.openatx.xtest.popup;

import android.content.Context;
import android.content.SharedPreferences;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;

/** Keeps only successful Monkey starts, newest first. */
final class RecentMonkeyApps {
    private static final String PREFERENCES = "nova";
    private static final String KEY = "recent_monkey_packages";
    private static final int LIMIT = 5;

    static void record(Context context, String packageName) {
        if (packageName == null || packageName.trim().isEmpty()) return;
        LinkedHashSet<String> ordered = new LinkedHashSet<>();
        ordered.add(packageName.trim());
        ordered.addAll(packages(context));
        List<String> bounded = new ArrayList<>();
        for (String value : ordered) {
            bounded.add(value);
            if (bounded.size() == LIMIT) break;
        }
        context.getSharedPreferences(PREFERENCES, Context.MODE_PRIVATE).edit()
                .putString(KEY, join(bounded)).apply();
    }

    static List<AppCatalog.Entry> recent(Context context, List<AppCatalog.Entry> all) {
        List<AppCatalog.Entry> result = new ArrayList<>();
        for (String packageName : packages(context)) {
            for (AppCatalog.Entry entry : all) {
                if (packageName.equals(entry.packageName)) { result.add(entry); break; }
            }
        }
        return result;
    }

    static List<AppCatalog.Entry> remaining(List<AppCatalog.Entry> all, List<AppCatalog.Entry> recent) {
        Set<String> pinned = new HashSet<>();
        for (AppCatalog.Entry entry : recent) pinned.add(entry.packageName);
        List<AppCatalog.Entry> result = new ArrayList<>();
        for (AppCatalog.Entry entry : all) if (!pinned.contains(entry.packageName)) result.add(entry);
        return result;
    }

    private static List<String> packages(Context context) {
        SharedPreferences preferences = context.getSharedPreferences(PREFERENCES, Context.MODE_PRIVATE);
        List<String> result = new ArrayList<>();
        Set<String> seen = new HashSet<>();
        for (String raw : preferences.getString(KEY, "").split(",")) {
            String value = raw.trim();
            if (!value.isEmpty() && seen.add(value) && result.size() < LIMIT) result.add(value);
        }
        return result;
    }

    private static String join(List<String> values) {
        StringBuilder result = new StringBuilder();
        for (String value : values) {
            if (result.length() > 0) result.append(',');
            result.append(value);
        }
        return result.toString();
    }
}
