package com.xtest.nova.fixture;

import android.app.Activity;
import android.content.Intent;
import android.os.Bundle;
import android.view.View;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.TextView;

public final class NonStackActivity extends Activity {
    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(0, 220, 0, 0);
        TextView description = new TextView(this);
        description.setText("非栈式图路径起点");
        description.setTextSize(20);
        Button open = new Button(this);
        open.setText("打开中转");
        open.setOnClickListener(new View.OnClickListener() {
            @Override public void onClick(View view) {
                startActivity(new Intent(NonStackActivity.this, TransientActivity.class));
            }
        });
        content.addView(description);
        content.addView(open);
        setContentView(content);
    }
}
