package com.xtest.nova.fixture;

import android.app.Activity;
import android.os.Bundle;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

public final class LeaderboardActivity extends Activity {
    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        ScrollView scroll = new ScrollView(this);
        scroll.setContentDescription("leaderboard-scroll");
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(28, 28, 28, 40);
        TextView title = new TextView(this);
        title.setText("Nova 排行榜");
        title.setContentDescription("leaderboard-title");
        title.setTextSize(28);
        title.setPadding(12, 18, 12, 24);
        content.addView(title);
        for (int rank = 1; rank <= 40; rank++) {
            TextView row = new TextView(this);
            row.setText("第 " + rank + " 名｜玩家-" + rank + "｜" + (4100 - rank * 73) + " 分");
            row.setContentDescription("leaderboard-row-" + rank);
            row.setTextSize(17);
            row.setPadding(12, 22, 12, 22);
            content.addView(row);
        }
        Button back = new Button(this);
        back.setText("返回游戏");
        back.setContentDescription("return-to-game");
        back.setOnClickListener(v -> finish());
        content.addView(back);
        scroll.addView(content);
        setContentView(scroll);
    }
}
