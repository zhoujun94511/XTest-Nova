package com.xtest.nova.fixture;

import android.app.Activity;
import android.content.Intent;
import android.net.Uri;
import android.os.Bundle;
import android.provider.Settings;
import android.view.View;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.TextView;

public final class LifecycleScenarioActivity extends Activity {
    private int instanceCount;
    private TextView status;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        instanceCount = state == null ? 1 : state.getInt("instanceCount", 1) + 1;
        int launches = getPreferences(MODE_PRIVATE).getInt("launches", 0) + 1;
        getPreferences(MODE_PRIVATE).edit().putInt("launches", launches).apply();
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(24, 200, 24, 24);
        status = new TextView(this);
        status.setText("生命周期：launches=" + launches + ", instance=" + instanceCount);
        status.setContentDescription("lifecycle-status");
        status.setTextSize(20);
        Button recreate = button("重建 Activity", "lifecycle-recreate");
        recreate.setOnClickListener(new View.OnClickListener() { @Override public void onClick(View view) { recreate(); } });
        Button settings = button("打开应用设置", "lifecycle-settings");
        settings.setOnClickListener(new View.OnClickListener() { @Override public void onClick(View view) { startActivity(new Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.parse("package:" + getPackageName()))); } });
        Button finish = button("结束当前页", "lifecycle-finish");
        finish.setOnClickListener(new View.OnClickListener() { @Override public void onClick(View view) { finish(); } });
        content.addView(status);
        content.addView(recreate);
        content.addView(settings);
        content.addView(finish);
        setContentView(content);
    }

    @Override protected void onSaveInstanceState(Bundle state) {
        state.putInt("instanceCount", instanceCount);
        super.onSaveInstanceState(state);
    }

    private Button button(String text, String description) {
        Button value = new Button(this);
        value.setText(text);
        value.setContentDescription(description);
        return value;
    }
}
