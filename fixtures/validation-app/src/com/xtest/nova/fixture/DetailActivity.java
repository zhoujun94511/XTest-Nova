package com.xtest.nova.fixture;

import android.app.Activity;
import android.os.Bundle;
import android.view.View;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

public final class DetailActivity extends Activity {
    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        ScrollView scroll = new ScrollView(this);
        scroll.setContentDescription("可滚动详情列表");
        LinearLayout list = new LinearLayout(this);
        list.setOrientation(LinearLayout.VERTICAL);
        TextView hold = new TextView(this);
        hold.setText("长按操作");
        hold.setContentDescription("详情页长按操作");
        hold.setTextSize(18);
        hold.setPadding(32, 34, 32, 34);
        hold.setOnLongClickListener(new View.OnLongClickListener() {
            @Override public boolean onLongClick(View view) {
                return true;
            }
        });
        list.addView(hold, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT));
        for (int index = 1; index <= 30; index++) {
            TextView row = new TextView(this);
            row.setText("详情条目 " + index);
            row.setContentDescription("详情操作 " + index);
            row.setTextSize(18);
            row.setPadding(32, 34, 32, 34);
            list.addView(row, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT));
        }
        scroll.addView(list);
        setContentView(scroll);
    }
}
