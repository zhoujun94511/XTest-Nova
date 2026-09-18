package com.openatx.xtest.popup;

import android.app.Activity;
import android.content.Intent;
import android.content.SharedPreferences;
import android.os.Build;
import android.os.Bundle;
import android.provider.Settings;

/**
 * No-display shell entry point. The exported activity starts the private
 * overlay service and exits in the same lifecycle callback.
 */
public final class PopupLauncherActivity extends Activity {
    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        if (Build.VERSION.SDK_INT < 23 || Settings.canDrawOverlays(this)) {
            String target = getIntent().getStringExtra("target_package");
            SharedPreferences settings = getSharedPreferences("nova", MODE_PRIVATE);
            if (settings.getString("package", "").isEmpty()
                    && target != null
                    && target.matches("^[A-Za-z][A-Za-z0-9_]*(?:\\.[A-Za-z][A-Za-z0-9_]*)+$")) {
                settings.edit().putString("package", target).apply();
            }
            Intent service = new Intent(this, OverlayService.class);
            if (Build.VERSION.SDK_INT >= 26) {
                startForegroundService(service);
            } else {
                startService(service);
            }
        }
        finish();
    }
}
