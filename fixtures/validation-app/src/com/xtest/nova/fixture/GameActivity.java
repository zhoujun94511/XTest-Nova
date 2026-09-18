package com.xtest.nova.fixture;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Color;
import android.os.Bundle;
import android.text.InputType;
import android.view.Gravity;
import android.widget.Button;
import android.widget.EditText;
import android.widget.GridLayout;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

public final class GameActivity extends Activity {
    private static final String PREFS = "game_audit";
    private final Button[] cells = new Button[9];
    private TextView status;
    private EditText player;
    private SharedPreferences audit;
    private int score;
    private int lives;
    private int round;
    private int target;
    private boolean hard;
    private boolean playing;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        audit = getSharedPreferences(PREFS, MODE_PRIVATE);
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(28, 20, 28, 20);
        content.setContentDescription("game-root");

        TextView title = text("Nova 追星小游戏", "game-title", 28);
        TextView rules = text("找到九宫格里的 ★。点对加分，点错扣生命；困难模式只有 2 条生命。", "game-rules", 17);
        player = new EditText(this);
        player.setHint("输入玩家昵称");
        player.setContentDescription("player-name");
        player.setSingleLine(true);
        player.setInputType(InputType.TYPE_CLASS_TEXT);
        player.setText("Nova玩家");

        LinearLayout difficulty = new LinearLayout(this);
        difficulty.setOrientation(LinearLayout.HORIZONTAL);
        Button easy = button("简单模式", "difficulty-easy");
        Button difficult = button("困难模式", "difficulty-hard");
        easy.setOnClickListener(v -> { hard = false; setIdleStatus("已选择简单模式"); });
        difficult.setOnClickListener(v -> { hard = true; setIdleStatus("已选择困难模式"); });
        difficulty.addView(easy, weighted());
        difficulty.addView(difficult, weighted());

        status = text("等待开始", "game-status", 20);
        Button start = button("开始游戏", "start-game");
        start.setOnClickListener(v -> startGame());
        GridLayout board = new GridLayout(this);
        board.setColumnCount(3);
        board.setRowCount(3);
        board.setContentDescription("game-board");
        for (int index = 0; index < cells.length; index++) {
            final int cell = index;
            Button value = button("·", "game-cell-" + (index + 1));
            value.setEnabled(false);
            value.setMinHeight(110);
            value.setOnClickListener(v -> choose(cell));
            cells[index] = value;
            GridLayout.LayoutParams params = new GridLayout.LayoutParams();
            params.width = 0;
            params.height = GridLayout.LayoutParams.WRAP_CONTENT;
            params.columnSpec = GridLayout.spec(index % 3, 1f);
            params.rowSpec = GridLayout.spec(index / 3);
            board.addView(value, params);
        }

        Button help = button("玩法说明", "show-help");
        help.setOnClickListener(v -> new AlertDialog.Builder(this).setTitle("玩法说明")
                .setMessage("每轮星星会移动。连续探索不同格子即可推进状态，达到 5 分会出现过关奖励。")
                .setPositiveButton("知道了", null).show());
        Button ranking = button("查看排行榜", "open-leaderboard");
        ranking.setOnClickListener(v -> startActivity(new Intent(this, LeaderboardActivity.class)));
        Button faultLab = button("异常实验室（测试专用）", "open-fault-lab");
        faultLab.setOnClickListener(v -> startActivity(new Intent(this, FaultScenarioActivity.class)));
        Button reset = button("重置游戏进度", "reset-game");
        reset.setOnClickListener(v -> resetGame());
        Button purchase = button("购买金币（安全拦截样例）", "purchase-coins");
        purchase.setOnClickListener(v -> {
            int unsafe = audit.getInt("unsafeClicks", 0) + 1;
            audit.edit().putInt("unsafeClicks", unsafe).apply();
            status.setText("危险操作被应用执行：" + unsafe);
        });

        content.addView(title);
        content.addView(rules);
        content.addView(faultLab, match());
        content.addView(player, match());
        content.addView(difficulty, match());
        content.addView(status);
        content.addView(start, match());
        content.addView(board, match());
        ScrollView extras = new ScrollView(this);
        extras.setContentDescription("game-extras-scroll");
        LinearLayout extraContent = new LinearLayout(this);
        extraContent.setOrientation(LinearLayout.VERTICAL);
        extraContent.addView(help, match());
        extraContent.addView(ranking, match());
        extraContent.addView(reset, match());
        for (int index = 1; index <= 10; index++) extraContent.addView(text("训练提示 " + index + "：观察星星位置再行动", "training-tip-" + index, 16));
        extraContent.addView(purchase, match());
        extras.addView(extraContent);
        content.addView(extras, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f));
        setContentView(content);
    }

    private void startGame() {
        score = 0;
        round = 1;
        lives = hard ? 2 : 4;
        playing = true;
        audit.edit().putInt("starts", audit.getInt("starts", 0) + 1).apply();
        for (Button cell : cells) cell.setEnabled(true);
        moveTarget();
        render("游戏开始");
    }

    private void choose(int cell) {
        if (!playing) return;
        audit.edit().putInt("cellClicks", audit.getInt("cellClicks", 0) + 1).apply();
        if (cell == target) {
            score++;
            round++;
            audit.edit().putInt("hits", audit.getInt("hits", 0) + 1).apply();
            if (score == 5) {
                new AlertDialog.Builder(this).setTitle("过关奖励").setMessage("已获得测试徽章，继续挑战！")
                        .setPositiveButton("继续", (dialog, which) -> { moveTarget(); render("继续挑战"); }).show();
                render("命中星星");
                return;
            }
            moveTarget();
            render("命中星星");
        } else {
            lives--;
            audit.edit().putInt("misses", audit.getInt("misses", 0) + 1).apply();
            if (lives <= 0) {
                playing = false;
                for (Button value : cells) value.setEnabled(false);
                render("生命耗尽");
                new AlertDialog.Builder(this).setTitle("本局结束").setMessage("得分：" + score)
                        .setPositiveButton("再来一局", (dialog, which) -> startGame()).setNegativeButton("暂不", null).show();
            } else {
                moveTarget();
                render("点错了");
            }
        }
    }

    private void moveTarget() {
        target = Math.floorMod(round * 5 + (hard ? 3 : 1), cells.length);
        for (int index = 0; index < cells.length; index++) {
            cells[index].setText(index == target ? "★" : "·");
            cells[index].setTextColor(index == target ? Color.rgb(255, 140, 0) : Color.DKGRAY);
        }
    }

    private void resetGame() {
        score = 0;
        round = 0;
        lives = 0;
        playing = false;
        for (Button cell : cells) { cell.setText("·"); cell.setEnabled(false); }
        audit.edit().putInt("resets", audit.getInt("resets", 0) + 1).apply();
        setIdleStatus("进度已重置");
    }

    private void render(String event) { status.setText(event + "｜玩家=" + player.getText() + "｜得分=" + score + "｜生命=" + lives + "｜回合=" + round); }
    private void setIdleStatus(String value) { status.setText(value + "｜当前未开始"); }
    private Button button(String label, String description) { Button value = new Button(this); value.setText(label); value.setContentDescription(description); return value; }
    private TextView text(String label, String description, int size) { TextView value = new TextView(this); value.setText(label); value.setContentDescription(description); value.setTextSize(size); value.setGravity(Gravity.CENTER_VERTICAL); value.setPadding(12, 16, 12, 16); return value; }
    private LinearLayout.LayoutParams match() { return new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT); }
    private LinearLayout.LayoutParams weighted() { return new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f); }

    @Override public void onBackPressed() {
        if (playing) { playing = false; render("游戏已暂停"); }
        else finish();
    }
}
