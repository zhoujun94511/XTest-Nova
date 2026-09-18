package com.xtest.nova.fixture;

import android.app.Activity;
import android.os.Bundle;
import android.view.View;
import android.widget.Button;
import android.widget.LinearLayout;

public final class TransientActivity extends Activity {
    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(0, 220, 0, 0);
        Button leave = new Button(this);
        leave.setText("A返回");
        leave.setOnClickListener(new View.OnClickListener() {
            @Override public void onClick(View view) {
                finish();
            }
        });
        Button remaining = new Button(this);
        remaining.setText("Z探索");
        remaining.setOnClickListener(new View.OnClickListener() {
            @Override public void onClick(View view) {
                remaining.setEnabled(false);
            }
        });
        content.addView(leave);
        content.addView(remaining);
        setContentView(content);
    }
}
