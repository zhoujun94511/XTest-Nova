package com.xtest.nova.fixture;

import android.app.Activity;
import android.graphics.Color;
import android.os.Bundle;
import android.os.Handler;
import android.os.Process;
import android.os.SystemClock;
import android.system.ErrnoException;
import android.system.Os;
import android.system.OsConstants;
import android.view.Gravity;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.TextView;

public final class FaultScenarioActivity extends Activity {
    private Handler handler;
    private Button crashButton;
    private Button anrButton;
    private Button nativeButton;
    private Button signalButton;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        handler = new Handler(getMainLooper());
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(32, 48, 32, 32);
        content.setContentDescription("fault-lab-root");

        TextView title = text("异常实验室", "fault-lab-title", 28);
        TextView warning = text("以下按钮会故意让样例应用崩溃、无响应或退出，仅用于验证测试工具的异常采集。", "fault-lab-warning", 17);
        warning.setTextColor(Color.rgb(170, 40, 40));
        crashButton = button("触发 Java Crash", "fault-java-crash");
        anrButton = button("触发主线程 ANR", "fault-main-thread-anr");
        nativeButton = button("触发 Native Crash (SIGSEGV)", "fault-native-crash");
        signalButton = button("触发进程信号退出", "fault-process-signal");
        crashButton.setOnClickListener(v -> trigger("crash"));
        anrButton.setOnClickListener(v -> trigger("anr"));
        nativeButton.setOnClickListener(v -> trigger("native"));
        signalButton.setOnClickListener(v -> trigger("signal"));
        Button probe = button("输入分发探针（无副作用）", "fault-input-probe");
        Button back = button("返回测试入口", "fault-back-to-fixture");
        back.setOnClickListener(v -> finish());

        content.addView(title);
        content.addView(warning);
        content.addView(crashButton, match());
        content.addView(anrButton, match());
        content.addView(nativeButton, match());
        content.addView(signalButton, match());
        content.addView(probe, match());
        content.addView(back, match());
        setContentView(content);

        final String mode = getIntent().getStringExtra("mode");
        // Keep compatibility with the original fixture: mode alone still arms the
        // directed Java crash/ANR cases. New native/signal cases require auto=true.
        boolean legacyDirected = "crash".equals(mode) || "anr".equals(mode);
        if (legacyDirected || getIntent().getBooleanExtra("auto", false)) {
            setFaultControlsEnabled(false);
            handler.postDelayed(() -> trigger(mode), legacyDirected ? 500 : 800);
        }
    }

    private void setFaultControlsEnabled(boolean enabled) {
        crashButton.setEnabled(enabled);
        anrButton.setEnabled(enabled);
        nativeButton.setEnabled(enabled);
        signalButton.setEnabled(enabled);
    }

    private void trigger(String mode) {
        if ("crash".equals(mode)) throw new IllegalStateException("fixture-controlled-java-crash");
        if ("anr".equals(mode)) {
            SystemClock.sleep(15000);
            return;
        }
        if ("native".equals(mode)) {
            try {
                Os.kill(Process.myPid(), OsConstants.SIGSEGV);
            } catch (ErrnoException error) {
                throw new IllegalStateException("fixture-native-crash-trigger-failed", error);
            }
            return;
        }
        if ("signal".equals(mode)) Process.killProcess(Process.myPid());
    }

    private Button button(String label, String description) {
        Button value = new Button(this);
        value.setText(label);
        value.setContentDescription(description);
        value.setMinHeight(112);
        return value;
    }

    private TextView text(String label, String description, int size) {
        TextView value = new TextView(this);
        value.setText(label);
        value.setContentDescription(description);
        value.setTextSize(size);
        value.setGravity(Gravity.CENTER_VERTICAL);
        value.setPadding(12, 16, 12, 16);
        return value;
    }

    private LinearLayout.LayoutParams match() {
        return new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
    }
}
