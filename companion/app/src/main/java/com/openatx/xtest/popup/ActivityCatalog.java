package com.openatx.xtest.popup;

import android.content.Context;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Comparator;
import java.util.List;

final class ActivityCatalog {
    static final class Entry {
        final String value;
        final String title;
        final String fullName;

        Entry(String value, String title, String fullName) {
            this.value = value;
            this.title = title;
            this.fullName = fullName;
        }
    }

    static List<Entry> declared(Context context, String packageName) {
        List<Entry> result = new ArrayList<>();
        try {
            PackageInfo info = context.getPackageManager().getPackageInfo(
                    packageName, PackageManager.GET_ACTIVITIES | PackageManager.MATCH_DISABLED_COMPONENTS);
            if (info.activities != null) {
                for (ActivityInfo activity : info.activities) {
                    if (activity == null || activity.name == null || activity.name.trim().isEmpty()) continue;
                    String fullName = activity.name.trim();
                    String value = fullName.startsWith(packageName + ".") ? fullName.substring(packageName.length()) : fullName;
                    int dot = fullName.lastIndexOf('.');
                    String title = dot >= 0 ? fullName.substring(dot + 1) : fullName;
                    result.add(new Entry(value, title, fullName));
                }
            }
        } catch (PackageManager.NameNotFoundException ignored) { }
        Collections.sort(result, Comparator.comparing(entry -> entry.fullName.toLowerCase()));
        return result;
    }
}
