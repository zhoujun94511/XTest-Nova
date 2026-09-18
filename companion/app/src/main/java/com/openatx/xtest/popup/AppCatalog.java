package com.openatx.xtest.popup;

import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.content.pm.ResolveInfo;
import android.graphics.drawable.Drawable;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Comparator;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

final class AppCatalog {
    static final class Entry {
        final String packageName;
        final String label;
        final Drawable icon;
        Entry(String packageName, String label, Drawable icon) { this.packageName = packageName; this.label = label; this.icon = icon; }
    }

    static List<Entry> launchable(Context context) {
        PackageManager manager = context.getPackageManager();
        Intent query = new Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER);
        List<ResolveInfo> resolved = manager.queryIntentActivities(query, 0);
        List<Entry> entries = new ArrayList<>();
        Set<String> seen = new HashSet<>();
        for (ResolveInfo value : resolved) {
            String packageName = value.activityInfo.packageName;
            if (packageName.equals(context.getPackageName()) || !seen.add(packageName)) continue;
            entries.add(new Entry(packageName, value.loadLabel(manager).toString(), value.loadIcon(manager)));
        }
        Collections.sort(entries, Comparator.comparing(entry -> entry.label.toLowerCase()));
        return entries;
    }

}
