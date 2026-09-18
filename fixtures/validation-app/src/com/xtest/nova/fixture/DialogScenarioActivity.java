package com.xtest.nova.fixture;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.DialogInterface;
import android.os.Bundle;
import android.os.Handler;
import android.view.View;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.TextView;
import android.widget.Toast;

public final class DialogScenarioActivity extends Activity {
    private TextView status;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(24, 180, 24, 24);
        status = new TextView(this);
        status.setText("弹窗状态：空闲");
        status.setContentDescription("dialog-status");
        status.setTextSize(20);
        Button confirm = button("显示确认弹窗", "dialog-confirm");
        confirm.setOnClickListener(new View.OnClickListener() { @Override public void onClick(View view) { showConfirmation(); } });
        Button delayed = button("显示延迟弹窗", "dialog-delayed");
        delayed.setOnClickListener(new View.OnClickListener() {
            @Override public void onClick(View view) {
                status.setText("弹窗状态：等待延迟弹窗");
                new Handler(getMainLooper()).postDelayed(new Runnable() { @Override public void run() { showConfirmation(); } }, 750);
            }
        });
        Button toast = button("显示短消息", "dialog-toast");
        toast.setOnClickListener(new View.OnClickListener() { @Override public void onClick(View view) { Toast.makeText(DialogScenarioActivity.this, "fixture-toast", Toast.LENGTH_SHORT).show(); } });
        content.addView(status);
        content.addView(confirm);
        content.addView(delayed);
        content.addView(toast);
        setContentView(content);
        if (getIntent().getBooleanExtra("auto", false)) new Handler(getMainLooper()).postDelayed(new Runnable() { @Override public void run() { showConfirmation(); } }, 350);
    }

    private Button button(String text, String description) {
        Button value = new Button(this);
        value.setText(text);
        value.setContentDescription(description);
        return value;
    }

    private void showConfirmation() {
        status.setText("弹窗状态：确认中");
        new AlertDialog.Builder(this).setTitle("确认弹窗场景").setMessage("唯一弹窗内容 fixture-dialog-confirm")
            .setPositiveButton("确认执行", new DialogInterface.OnClickListener() { @Override public void onClick(DialogInterface dialog, int which) { status.setText("弹窗状态：已确认"); } })
            .setNegativeButton("取消执行", new DialogInterface.OnClickListener() { @Override public void onClick(DialogInterface dialog, int which) { status.setText("弹窗状态：已取消"); } })
            .setCancelable(false).show();
    }
}
