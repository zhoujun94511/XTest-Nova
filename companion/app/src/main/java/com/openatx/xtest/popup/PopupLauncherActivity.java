package com.openatx.xtest.popup;

import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Color;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.provider.Settings;
import android.view.Gravity;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.TextView;
import android.widget.Toast;

/**
 * Starts the overlay when SYSTEM_ALERT_WINDOW is already granted.
 * Otherwise shows how to grant it and opens the AOSP overlay page.
 * Android 11+ ignores the package URI and shows an app list, so the
 * prompt names XTest Nova explicitly.
 */
public final class PopupLauncherActivity extends Activity {
    private static final String APP_LABEL = "XTest Nova";
    private boolean requestedSettings;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        persistTarget();
        if (hasOverlay()) {
            startOverlayAndFinish();
            return;
        }
        copyAppLabel();
        showPrompt();
        openOverlaySettings();
    }

    @Override protected void onResume() {
        super.onResume();
        if (hasOverlay()) {
            startOverlayAndFinish();
        }
    }

    private void persistTarget() {
        String target = getIntent().getStringExtra("target_package");
        SharedPreferences settings = getSharedPreferences("nova", MODE_PRIVATE);
        if (settings.getString("package", "").isEmpty()
                && target != null
                && target.matches("^[A-Za-z][A-Za-z0-9_]*(?:\\.[A-Za-z][A-Za-z0-9_]*)+$")) {
            settings.edit().putString("package", target).apply();
        }
    }

    private boolean hasOverlay() {
        return Build.VERSION.SDK_INT < 23 || Settings.canDrawOverlays(this);
    }

    private void startOverlayAndFinish() {
        Intent service = new Intent(this, OverlayService.class);
        if (Build.VERSION.SDK_INT >= 26) {
            startForegroundService(service);
        } else {
            startService(service);
        }
        finish();
    }

    private void copyAppLabel() {
        ClipboardManager clipboard = (ClipboardManager) getSystemService(CLIPBOARD_SERVICE);
        if (clipboard != null) {
            clipboard.setPrimaryClip(ClipData.newPlainText("app", APP_LABEL));
        }
    }

    private void showPrompt() {
        LinearLayout box = new LinearLayout(this);
        box.setOrientation(LinearLayout.VERTICAL);
        box.setGravity(Gravity.CENTER);
        box.setPadding(48, 48, 48, 48);
        box.setBackgroundColor(Color.WHITE);
        TextView text = new TextView(this);
        text.setText("请在系统列表中找到「" + APP_LABEL + "」，打开「显示在其他应用上层」。"
                + "应用名已复制，可在设置页搜索。返回后会自动拉起悬浮窗。");
        text.setTextColor(Color.BLACK);
        text.setTextSize(16);
        text.setGravity(Gravity.CENTER);
        Button button = new Button(this);
        button.setText("去授权");
        button.setOnClickListener(v -> {
            requestedSettings = false;
            copyAppLabel();
            openOverlaySettings();
        });
        box.addView(text);
        box.addView(button);
        setContentView(box);
    }

    private void openOverlaySettings() {
        if (requestedSettings) {
            return;
        }
        requestedSettings = true;
        Toast.makeText(this, "请选择 " + APP_LABEL, Toast.LENGTH_LONG).show();
        Intent aosp = new Intent(Settings.ACTION_MANAGE_OVERLAY_PERMISSION,
                Uri.parse("package:" + getPackageName()));
        try {
            startActivity(aosp);
            return;
        } catch (ActivityNotFoundException ignored) {
        }
        Intent details = new Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                Uri.parse("package:" + getPackageName()));
        if (details.resolveActivity(getPackageManager()) != null) {
            startActivity(details);
        }
    }
}
