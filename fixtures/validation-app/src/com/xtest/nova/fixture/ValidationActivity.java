package com.xtest.nova.fixture;

import android.app.Activity;
import android.os.Bundle;
import android.content.Intent;
import android.widget.Button;
import android.view.View;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.TextView;
import android.widget.ScrollView;

public final class ValidationActivity extends Activity {
    private int taps;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        ScrollView scroll = new ScrollView(this);
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        TextView message = new TextView(this);
        message.setText("目标应用样例\n点击区域");
        message.setTextSize(20);
        message.setPadding(32, 32, 32, 32);
        message.setOnClickListener(new View.OnClickListener() {
            @Override public void onClick(View view) {
                taps++;
                message.setText("目标应用样例\n点击次数：" + taps);
            }
        });
        message.setOnLongClickListener(new View.OnLongClickListener() {
            @Override public boolean onLongClick(View view) {
                return true;
            }
        });
        EditText input = new EditText(this);
        input.setHint("录制最终文本样例");
        input.setText("最终文本-杭州");
        input.setSingleLine(true);
        Button details = new Button(this);
        details.setText("打开长列表");
        details.setContentDescription("进入详情页");
        details.setOnClickListener(new View.OnClickListener() {
            @Override public void onClick(View view) {
                startActivity(new Intent(ValidationActivity.this, DetailActivity.class));
            }
        });
        Button graph = new Button(this);
        graph.setText("A非栈");
        graph.setContentDescription("打开非栈图验证");
        graph.setOnClickListener(new View.OnClickListener() {
            @Override public void onClick(View view) {
                startActivity(new Intent(ValidationActivity.this, NonStackActivity.class));
            }
        });
        Button surface = new Button(this);
        surface.setText("B高刷");
        surface.setContentDescription("打开Surface高刷循环");
        surface.setOnClickListener(new View.OnClickListener() {
            @Override public void onClick(View view) {
                startActivity(new Intent(ValidationActivity.this, SurfaceLoopActivity.class));
            }
        });
        Button gpu = new Button(this);
        gpu.setText("CGPU");
        gpu.setContentDescription("打开OpenGL GPU循环");
        gpu.setOnClickListener(new View.OnClickListener() {
            @Override public void onClick(View view) {
                startActivity(new Intent(ValidationActivity.this, GLESLoopActivity.class));
            }
        });
        content.addView(navigation("复杂表单", "打开复杂表单场景", FormScenarioActivity.class));
        content.addView(navigation("弹窗与异步 UI", "打开弹窗场景", DialogScenarioActivity.class));
        content.addView(navigation("运行时权限", "打开权限场景", PermissionScenarioActivity.class));
        content.addView(navigation("本地 WebView", "打开 WebView 场景", WebViewScenarioActivity.class));
        content.addView(navigation("生命周期与外跳", "打开生命周期场景", LifecycleScenarioActivity.class));
        content.addView(navigation("游戏探索场景", "打开游戏探索场景", GameActivity.class));
        content.addView(navigation("异常实验室", "打开异常实验室", FaultScenarioActivity.class));
        content.addView(message, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT));
        content.addView(graph, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT));
        content.addView(surface, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT));
        content.addView(gpu, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT));
        content.addView(details, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT));
        content.addView(input, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT));
        scroll.addView(content);
        setContentView(scroll);
        input.requestFocus();
    }

    private Button navigation(String text, String description, final Class<?> activity) {
        Button button = new Button(this);
        button.setText(text);
        button.setContentDescription(description);
        button.setOnClickListener(new View.OnClickListener() {
            @Override public void onClick(View view) {
                startActivity(new Intent(ValidationActivity.this, activity));
            }
        });
        return button;
    }

    @Override public void onBackPressed() {
        // Keep random-runner validation inside this isolated fixture.
    }
}
