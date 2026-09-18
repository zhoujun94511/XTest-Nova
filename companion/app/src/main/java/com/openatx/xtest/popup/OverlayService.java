package com.openatx.xtest.popup;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.Service;
import android.content.Intent;
import android.graphics.Color;
import android.graphics.PixelFormat;
import android.graphics.drawable.GradientDrawable;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.provider.Settings;
import android.text.Editable;
import android.text.TextWatcher;
import android.util.DisplayMetrics;
import android.view.Gravity;
import android.view.View;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.CheckBox;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import java.text.SimpleDateFormat;
import java.util.Date;
import java.util.List;
import java.util.LinkedHashSet;
import java.util.Locale;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class OverlayService extends Service {
    public static final String ACTION_SUPPRESS = "com.openatx.xtest.popup.action.SUPPRESS";
    public static final String ACTION_RESTORE = "com.openatx.xtest.popup.action.RESTORE";
    private enum Flow { PERFORMANCE, RECORD_REPLAY, MONKEY }
    private static final int PANEL = Color.rgb(50, 50, 50);
    private static final int ROW = Color.rgb(69, 69, 69);
    private static final int WHITE = Color.rgb(245, 245, 245);
    private static final int RED = Color.rgb(255, 35, 20);
    private static final int AMBER = Color.rgb(255, 190, 70);
    private static final int MAIN_WIDTH_DP = 112;
    private static final int MAIN_HEIGHT_DP = 220;
    private static final int COMPACT_WIDTH_DP = 176;
    private static final int PERFORMANCE_HEIGHT_DP = 216;
    private static final int RECORDING_PANEL_HEIGHT_DP = 280;
    private static final int MINIMIZED_HANDLE_WIDTH_DP = 24;
    private static final int MINIMIZED_HANDLE_HEIGHT_DP = 48;
    private static final int MINIMIZED_TOUCH_WIDTH_DP = 56;
    private static final int MINIMIZED_TOUCH_HEIGHT_DP = 64;
    private static final int MINIMIZED_TOP_INSET_DP = 72;
    private static final int LARGE_PANEL_MAX_WIDTH_DP = 360;
    private static final int LARGE_PANEL_MAX_HEIGHT_DP = 640;
    private static final int LARGE_HEADER_HEIGHT_DP = 48;
    private static final int COMPACT_HEADER_HEIGHT_DP = 36;
    private static final int FLOATING_PANEL_MARGIN_DP = 20;
    private static final int FLOATING_PANEL_BOTTOM_DP = 52;
    private static final int TABLET_MAIN_WIDTH_DP = 152;
    private static final int TABLET_MAIN_HEIGHT_DP = 260;
    private static final int TABLET_COMPACT_WIDTH_DP = 320;
    private static final int TABLET_PERFORMANCE_HEIGHT_DP = 300;
    private static final int TABLET_RECORDING_HEIGHT_DP = 300;
    private static final int TABLET_COMPACT_HEADER_HEIGHT_DP = 44;
    private static final int TABLET_MINIMIZED_HANDLE_WIDTH_DP = 32;
    private static final int TABLET_MINIMIZED_HANDLE_HEIGHT_DP = 56;
    private static final int TABLET_MINIMIZED_TOUCH_WIDTH_DP = 64;
    private static final int TABLET_MINIMIZED_TOUCH_HEIGHT_DP = 72;
    private static final int TABLET_LARGE_PANEL_MAX_WIDTH_DP = 520;
    private static final int TABLET_LARGE_PANEL_MAX_HEIGHT_DP = 720;
    private WindowManager manager;
    private View view;
    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());
    private volatile boolean destroyed;
    private boolean automationSuppressed;
    private static volatile String controlState = "absent";
    private int generation;
    private final Runnable overlayPermissionGuard = new Runnable() {
        @Override public void run() {
            if (destroyed) return;
            if (!hasOverlayPermission()) {
                stopSelf();
                return;
            }
            main.postDelayed(this, 1000);
        }
    };

    @Override public void onCreate() {
        super.onCreate();
        createChannel();
        startForeground(10249, new Notification.Builder(this, "xtest-nova")
                .setContentTitle("XTest Nova").setContentText("悬浮控制器运行中")
                .setSmallIcon(android.R.drawable.ic_media_play).build());
        manager = (WindowManager) getSystemService(WINDOW_SERVICE);
        if (!hasOverlayPermission()) stopSelf();
        else main.postDelayed(overlayPermissionGuard, 1000);
    }

    @Override public int onStartCommand(Intent intent, int flags, int startId) {
        if (!hasOverlayPermission()) {
            stopSelf();
            return START_NOT_STICKY;
        }
        String action = intent == null ? null : intent.getAction();
        if (ACTION_SUPPRESS.equals(action)) suppressForAutomation();
        else if (ACTION_RESTORE.equals(action)) restoreAfterAutomation();
        else if (view == null && !automationSuppressed) showMain();
        return START_NOT_STICKY;
    }

    private void suppressForAutomation() {
        automationSuppressed = true;
        ++generation;
        if (view != null) {
            try { manager.removeView(view); } catch (RuntimeException ignored) { }
            view = null;
        }
        controlState = "suppressed";
    }

    private void restoreAfterAutomation() {
        if (!automationSuppressed) return;
        automationSuppressed = false;
        if (view == null) showMain();
    }

    static String controlState() { return controlState; }

    private boolean hasOverlayPermission() { return Build.VERSION.SDK_INT < 23 || Settings.canDrawOverlays(this); }
    private void createChannel() {
        if (Build.VERSION.SDK_INT >= 26) ((NotificationManager) getSystemService(NOTIFICATION_SERVICE))
                .createNotificationChannel(new NotificationChannel("xtest-nova", "XTest Nova", NotificationManager.IMPORTANCE_LOW));
    }

    private void showMain() {
        automationSuppressed = false;
        ++generation;
        LinearLayout box = column(PANEL);
        addMainRow(box, "性能测试", WHITE, v -> showApps(Flow.PERFORMANCE));
        addMainRow(box, "录制回放", WHITE, v -> showApps(Flow.RECORD_REPLAY));
        addMainRow(box, "Monkey", WHITE, v -> showApps(Flow.MONKEY));
        addMainRow(box, "最小化", WHITE, v -> showMinimized());
        addMainRow(box, "退出", RED, v -> stopSelf());
        replace(box, dp(mainWidthDp()), dp(mainHeightDp()), false, 0, 0);
    }

    private void addMainRow(LinearLayout box, String text, int color, View.OnClickListener listener) {
        Button button = menuButton(text, color, listener);
        button.setTextSize(15); button.setMinHeight(0); button.setMinimumHeight(0);
        box.addView(button, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1));
    }

    private Button menuButton(String text, int color, View.OnClickListener listener) {
        Button button = new Button(this);
        button.setText(text); button.setTextColor(color); button.setTextSize(17); button.setAllCaps(false);
        button.setMinHeight(dp(44)); button.setPadding(dp(6), 0, dp(6), 0);
        GradientDrawable background = new GradientDrawable();
        background.setColor(ROW); background.setStroke(dp(1), Color.rgb(30, 30, 30)); background.setCornerRadius(0);
        button.setBackground(background); button.setOnClickListener(listener);
        return button;
    }

    private void showApps(Flow flow) {
        int token = ++generation;
        LinearLayout shell = column(PANEL);
        LinearLayout header = new LinearLayout(this); header.setGravity(Gravity.CENTER_VERTICAL);
        header.addView(label("选择测试应用", 18, WHITE), new LinearLayout.LayoutParams(0, dp(LARGE_HEADER_HEIGHT_DP), 1));
        TextView close = closeControl(this::showMain, LARGE_HEADER_HEIGHT_DP, 22);
        header.addView(close, new LinearLayout.LayoutParams(dp(LARGE_HEADER_HEIGHT_DP), dp(LARGE_HEADER_HEIGHT_DP))); shell.addView(header);
        TextView loading = label("正在读取应用…", 16, WHITE); shell.addView(loading);
        replace(shell, largePanelWidth(), largePanelHeight(), false, 0, 0);
        worker.execute(() -> {
            List<AppCatalog.Entry> apps = AppCatalog.launchable(this);
            runUi(() -> {
                if (token != generation) return;
                shell.removeView(loading);
                ScrollView scroll = new ScrollView(this); LinearLayout list = column(PANEL);
                if (flow == Flow.MONKEY) {
                    List<AppCatalog.Entry> recent = RecentMonkeyApps.recent(this, apps);
                    if (!recent.isEmpty()) {
                        list.addView(sectionHint("最近测试", "成功启动过的应用，最新优先，最多保留 5 个。"));
                        for (AppCatalog.Entry app : recent) list.addView(appRow(app, flow));
                        list.addView(sectionHint("全部应用", "按应用名称排序；最近测试项不重复显示。"));
                    }
                    for (AppCatalog.Entry app : RecentMonkeyApps.remaining(apps, recent)) list.addView(appRow(app, flow));
                } else {
                    for (AppCatalog.Entry app : apps) list.addView(appRow(app, flow));
                }
                scroll.addView(list); shell.addView(scroll, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1));
            });
        });
    }

    private View appRow(AppCatalog.Entry app, Flow flow) {
        LinearLayout row = new LinearLayout(this); row.setGravity(Gravity.CENTER_VERTICAL);
        row.setPadding(dp(6), dp(3), dp(6), dp(3)); row.setBackgroundColor(ROW);
        ImageView icon = new ImageView(this); icon.setImageDrawable(app.icon);
        row.addView(icon, new LinearLayout.LayoutParams(dp(48), dp(48)));
        TextView name = label(app.label, 18, WHITE); name.setPadding(dp(8), 0, 0, 0);
        row.addView(name, new LinearLayout.LayoutParams(0, dp(54), 1));
        row.setContentDescription("app:" + app.packageName);
        row.setOnClickListener(v -> selectApp(app, flow));
        return row;
    }

    private void selectApp(AppCatalog.Entry app, Flow flow) {
        getSharedPreferences("nova", MODE_PRIVATE).edit().putString("package", app.packageName).apply();
        if (flow == Flow.PERFORMANCE) showPerformance(app.packageName, app.label);
        else if (flow == Flow.RECORD_REPLAY) showRecordReplay(app.packageName);
        else showMonkey(new MonkeyDraft(app.packageName, app.label));
    }

    private void showPerformance(String packageName, String appLabel) {
        int token = ++generation;
        LinearLayout box = column(PANEL); box.addView(compactHeader(appLabel, this::stopPerformance));
        TextView metrics = label("正在读取性能数据…", compactMetricsTextSizeSp(), WHITE); metrics.setPadding(dp(8), dp(6), dp(8), dp(6));
        metrics.setLineSpacing(dp(1), 1.0f);
        ScrollView scroll = new ScrollView(this); scroll.setFillViewport(false); scroll.addView(metrics);
        box.addView(scroll, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1));
        replace(box, dp(compactWidthDp()), dp(performanceHeightDp()), false, 0, 0);
        worker.execute(() -> {
            try {
                String response = AgentClient.performanceStart(packageName);
                runUi(() -> {
                    if (token != generation) { worker.execute(() -> { try { AgentClient.performanceStop(); } catch (Exception ignored) { } }); return; }
                    metrics.setText(AgentClient.formatPerformanceSession(response));
                    main.postDelayed(() -> pollPerformance(metrics, token), 1000);
                });
            } catch (Exception error) {
                runUi(() -> { if (token == generation) metrics.setText(error.getMessage() != null && error.getMessage().contains("package not found") ? "应用未启动" : "采样失败\n" + error.getMessage()); });
            }
        });
    }

    private void pollPerformance(TextView metrics, int token) {
        worker.execute(() -> {
            String response = null;
            String result;
            boolean reachable = true;
            try {
                response = AgentClient.performanceState();
                result = AgentClient.formatPerformanceSession(response);
            } catch (Exception error) { reachable = false; result = "状态暂时不可达，正在重试\n" + error.getMessage(); }
            boolean running = AgentClient.isRunning(response);
            boolean retry = !reachable;
            String display = result;
            runUi(() -> {
                if (token != generation) return;
                metrics.setText(display);
                if (running || retry) main.postDelayed(() -> pollPerformance(metrics, token), retry ? 2000 : 1000);
            });
        });
    }

    private void stopPerformance() {
        ++generation;
        worker.execute(() -> {
            try { AgentClient.performanceStop(); } catch (Exception ignored) { }
            runUi(this::showMain);
        });
    }

    private LinearLayout compactHeader(String text, Runnable closeAction) {
        LinearLayout bar = new LinearLayout(this); bar.setGravity(Gravity.CENTER_VERTICAL); bar.setBackgroundColor(Color.rgb(30, 30, 30));
        int height = compactHeaderHeightDp();
        TextView title = label(text, compactTitleTextSizeSp(), WHITE); title.setSingleLine(true); title.setEllipsize(android.text.TextUtils.TruncateAt.END);
        bar.addView(title, new LinearLayout.LayoutParams(0, dp(height), 1));
        TextView close = closeControl(closeAction, height, isTablet() ? 22 : 18);
        bar.addView(close, new LinearLayout.LayoutParams(dp(height), dp(height)));
        return bar;
    }

    private void showRecordReplay(String packageName) {
        ++generation;
        LinearLayout box = column(PANEL); box.addView(titleBar("录制回放")); box.addView(label(packageName, 13, Color.LTGRAY));
        box.addView(menuButton("新建录制", WHITE, v -> showNewRecording(packageName)));
        box.addView(menuButton("已保存用例", WHITE, v -> showSavedCases(packageName)));
        box.addView(menuButton("返回", WHITE, v -> showMain()));
        replace(box, largePanelWidth(), WindowManager.LayoutParams.WRAP_CONTENT, false, 0, 0);
    }

    private void showNewRecording(String packageName) {
        ++generation;
        LinearLayout box = column(PANEL); box.addView(titleBar("新建录制")); box.addView(label(packageName, 13, Color.LTGRAY));
        EditText task = new EditText(this); task.setHint("任务名称"); task.setSingleLine(true);
        task.setText("task-" + new SimpleDateFormat("MMdd", Locale.US).format(new Date()));
        task.setTextColor(WHITE); task.setHintTextColor(Color.GRAY); box.addView(task);
        EditText name = new EditText(this); name.setHint("用例名称"); name.setSingleLine(true);
        name.setText("case-" + new SimpleDateFormat("MMdd-HHmmss", Locale.US).format(new Date()));
        name.setTextColor(WHITE); name.setHintTextColor(Color.GRAY); box.addView(name);
        TextView state = label("请选择操作", 14, WHITE); box.addView(state);
        box.addView(menuButton("开始录制", WHITE, v -> startRecording(packageName, safeCaseName(task.getText().toString()), safeCaseName(name.getText().toString()), state)));
        box.addView(menuButton("返回", WHITE, v -> showRecordReplay(packageName)));
        replace(box, largePanelWidth(), WindowManager.LayoutParams.WRAP_CONTENT, true, 0, 0);
    }

    private void startRecording(String packageName, String task, String name, TextView state) {
        state.setText("启动中…");
        worker.execute(() -> {
            try {
                DisplayMetrics display = getResources().getDisplayMetrics();
                double overlayLeft = Math.max(0, (display.widthPixels - dp(compactWidthDp() + FLOATING_PANEL_MARGIN_DP)) / (double) display.widthPixels);
                double overlayBottom = Math.min(1, dp(recordingHeightDp() + FLOATING_PANEL_BOTTOM_DP) / (double) display.heightPixels);
                AgentClient.recordingStart(packageName, task, name, overlayLeft, overlayBottom);
                runUi(() -> showRecordingRunning(packageName));
            } catch (Exception error) { runUi(() -> state.setText("启动失败：" + error.getMessage())); }
        });
    }

    private void showRecordingRunning(String packageName) {
        int token = ++generation;
        LinearLayout box = column(PANEL);
        box.addView(label("录制中", 18, RED));
        box.addView(label(packageName, 12, WHITE));
        TextView state = label("已记录 0 个动作", 12, Color.LTGRAY); box.addView(state);
        box.addView(menuButton("采集最终文本", WHITE, v -> appendRecordingAction(state, AgentClient::recordingFocusedText)));
        box.addView(menuButton("记录返回键", WHITE, v -> appendRecordingAction(state, AgentClient::recordingBack)));
        box.addView(menuButton("截图断言", WHITE, v -> appendRecordingAction(state, AgentClient::recordingScreenshot)));
        box.addView(menuButton("完成", WHITE, v -> finishRecording(packageName, state)));
        replace(box, dp(compactWidthDp()), dp(recordingHeightDp()), false,
                dp(FLOATING_PANEL_MARGIN_DP), dp(FLOATING_PANEL_BOTTOM_DP));
        pollRecording(state, token);
    }

    private void appendRecordingAction(TextView state, Request request) {
        worker.execute(() -> {
            try {
                String response = request.run();
                runUi(() -> state.setText("已记录 " + AgentClient.actions(response) + " 个动作"));
            } catch (Exception error) { runUi(() -> state.setText("记录失败：" + error.getMessage())); }
        });
    }

    private void pollRecording(TextView state, int token) {
        worker.execute(() -> {
            try {
                String response = AgentClient.recordingState();
                runUi(() -> {
                    if (token != generation) return;
                    state.setText("已记录 " + AgentClient.actions(response) + " 个动作");
                    if (AgentClient.isRunning(response)) main.postDelayed(() -> pollRecording(state, token), 500);
                });
            } catch (Exception error) {
                runUi(() -> {
                    if (token != generation) return;
                    state.setText("状态暂时不可达，正在重试：" + error.getMessage());
                    main.postDelayed(() -> pollRecording(state, token), 2000);
                });
            }
        });
    }

    private void finishRecording(String packageName, TextView state) {
        ++generation;
        state.setText("保存中…");
        worker.execute(() -> {
            try {
                AgentClient.recordingStop();
                runUi(() -> showSavedCases(packageName));
            } catch (Exception error) { runUi(() -> state.setText("保存失败：" + error.getMessage())); }
        });
    }

    private void showSavedCases(String packageName) {
        int token = ++generation;
        LinearLayout box = column(PANEL); box.addView(titleBar("录制任务")); box.addView(label(packageName, 12, Color.LTGRAY));
        TextView loading = label("正在读取用例…", 14, WHITE); box.addView(loading);
        replace(box, largePanelWidth(), largePanelHeight(), false, 0, 0);
        worker.execute(() -> {
            try {
                List<AgentClient.CaseEntry> cases = AgentClient.recordingCases(packageName);
                List<AgentClient.DraftEntry> drafts = AgentClient.recordingDrafts(packageName);
                runUi(() -> {
                    if (token != generation) return;
                    box.removeView(loading);
                    if (cases.isEmpty() && drafts.isEmpty()) {
                        box.addView(label("暂无已保存用例", 14, WHITE));
                    } else {
                        ScrollView scroll = new ScrollView(this); LinearLayout list = column(PANEL);
                        if (!drafts.isEmpty()) {
                            list.addView(label("可恢复草稿", 13, AMBER));
                            for (AgentClient.DraftEntry entry : drafts) {
                                String display = (entry.task.isEmpty() ? "" : entry.task + " / ") + entry.name;
                                Button row = menuButton("恢复并保存：" + display + "  (" + entry.actions + ")", AMBER,
                                        v -> finalizeRecordingDraft(packageName, entry));
                                row.setContentDescription("draft:" + entry.id);
                                list.addView(row);
                            }
                        }
                        if (!cases.isEmpty()) list.addView(label("已保存用例", 13, Color.LTGRAY));
                        for (AgentClient.CaseEntry entry : cases) {
                            String display = (entry.task.isEmpty() ? "" : entry.task + " / ") + entry.name;
                            Button row = menuButton(display + "  (" + entry.actions + ")", WHITE,
                                    v -> startSavedReplay(packageName, entry, loading));
                            row.setContentDescription("case:" + entry.id);
                            list.addView(row);
                        }
                        scroll.addView(list);
                        box.addView(scroll, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1));
                    }
                    box.addView(menuButton("新建录制", WHITE, v -> showNewRecording(packageName)));
                    box.addView(menuButton("返回", WHITE, v -> showRecordReplay(packageName)));
                });
            } catch (Exception error) { runUi(() -> loading.setText("读取失败：" + error.getMessage())); }
        });
    }

    private void finalizeRecordingDraft(String packageName, AgentClient.DraftEntry entry) {
        worker.execute(() -> {
            try {
                AgentClient.recordingDraftFinalize(entry.id);
                runUi(() -> showSavedCases(packageName));
            } catch (Exception error) { runUi(() -> showMessage("草稿恢复失败", error.getMessage())); }
        });
    }

    private void startSavedReplay(String packageName, AgentClient.CaseEntry entry, TextView state) {
        worker.execute(() -> {
            try {
                String caseJson = AgentClient.recordingCase(entry.id);
                AgentClient.replayStart(caseJson);
                runUi(() -> showReplayRunning(packageName, entry.name));
            } catch (Exception error) { runUi(() -> showMessage("回放失败", error.getMessage())); }
        });
    }

    private void showReplayRunning(String packageName, String caseName) {
        int token = ++generation;
        LinearLayout box = column(PANEL);
        box.addView(label("回放中", 18, RED)); box.addView(label(caseName, 13, WHITE));
        TextView state = label(packageName, 12, Color.LTGRAY); box.addView(state);
        box.addView(menuButton("停止", RED, v -> {
            asyncState(state, AgentClient::replayStop, "回放已停止");
            main.postDelayed(() -> showSavedCases(packageName), 500);
        }));
        replace(box, dp(compactWidthDp()), WindowManager.LayoutParams.WRAP_CONTENT, false,
                dp(FLOATING_PANEL_MARGIN_DP), dp(FLOATING_PANEL_BOTTOM_DP));
        pollReplay(packageName, state, token);
    }

    private void pollReplay(String packageName, TextView state, int token) {
        worker.execute(() -> {
            String response;
            boolean reachable = true;
            try { response = AgentClient.replayState(); }
            catch (Exception error) { reachable = false; response = "回放状态暂时不可达\n" + error.getMessage(); }
            String result = response;
            boolean retry = !reachable;
            runUi(() -> {
                if (token != generation) return;
                if (retry) {
                    state.setText(result);
                    main.postDelayed(() -> pollReplay(packageName, state, token), 2000);
                } else if (AgentClient.isRunning(result)) {
                    main.postDelayed(() -> pollReplay(packageName, state, token), 300);
                } else if (AgentClient.isCompleted(result)) {
                    state.setText("回放完成");
                    main.postDelayed(() -> { if (token == generation) showSavedCases(packageName); }, 700);
                } else {
                    state.setText("回放失败\n" + AgentClient.compact(result));
                }
            });
        });
    }

    private void showMonkey(MonkeyDraft draft) {
        ++generation;
        LinearLayout box = column(PANEL); box.addView(titleBar("智能遍历"));
        TextView app = label(draft.appLabel + "\n" + draft.packageName, 14, WHITE);
        app.setPadding(dp(10), dp(6), dp(10), dp(8)); box.addView(app);
        TextView intro = label("覆盖优先的动态随机探索；广告和最终结算页由安全策略自动处理。", 12, Color.LTGRAY);
        intro.setPadding(dp(10), 0, dp(10), dp(8)); box.addView(intro);
        NumberSetting minutes = settingNumberField("运行时长", "达到时长后自然结束，可随时手动停止。范围 1–10080。", "分钟", draft.durationMinutes);
        NumberSetting throttle = settingNumberField("动作间隔", "两次操作之间的等待时间。范围 50–60000。", "毫秒", draft.throttleMillis);
        NumberSetting battery = settingNumberField("低电量保护", "电量低于该值时停止任务并保存产物。范围 1–100。", "%", draft.batteryThreshold);
        ScrollView form = new ScrollView(this); LinearLayout fields = column(PANEL);
        fields.addView(minutes.view); fields.addView(throttle.view); fields.addView(battery.view);
        Button scope = menuButton("Activity 范围：" + draft.scopeSummary(), WHITE, v -> {
            captureMonkeyNumbers(draft, minutes.input, throttle.input, battery.input); showActivityMode(draft);
        });
        Button targets = menuButton("目标页面：" + draft.targetSummary(), WHITE, v -> {
            captureMonkeyNumbers(draft, minutes.input, throttle.input, battery.input); showActivityPicker(draft, true);
        });
        fields.addView(sectionHint("页面范围", "默认不限制。仅在确有范围要求时选择 Activity，无需手工输入类名。"));
        fields.addView(scope); fields.addView(targets);
        fields.addView(menuButton("纯渲染页面：" + draft.renderFallbackSummary(), WHITE, v -> {
            captureMonkeyNumbers(draft, minutes.input, throttle.input, battery.input); showRenderFallbackMode(draft);
        }));
        fields.addView(menuButton("高级配置", Color.LTGRAY, v -> {
            captureMonkeyNumbers(draft, minutes.input, throttle.input, battery.input); showMonkeyAdvanced(draft);
        }));
        fields.setPadding(0, 0, 0, dp(12));
        form.addView(fields);
        box.addView(form, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1));
        TextView state = label("配置完成后开始遍历", 13, Color.LTGRAY); box.addView(state);
        LinearLayout actions = new LinearLayout(this);
        actions.addView(menuButton("开始遍历", WHITE, v -> {
            captureMonkeyNumbers(draft, minutes.input, throttle.input, battery.input); startMonkey(draft, state);
        }), new LinearLayout.LayoutParams(0, dp(52), 1));
        actions.addView(menuButton("返回", WHITE, v -> showMain()), new LinearLayout.LayoutParams(0, dp(52), 1));
        box.addView(actions);
        replace(box, largePanelWidth(), largePanelHeight(), true, 0, 0);
    }

    private void startMonkey(MonkeyDraft draft, TextView state) {
        if (!MonkeyDraft.MODE_NONE.equals(draft.activityMode) && draft.activities.isEmpty()) {
            state.setText("请至少选择一个 Activity，或将范围改为“不限制”"); return;
        }
        state.setText("启动中…");
        worker.execute(() -> {
            try {
                AgentClient.monkeyStart(draft.packageName, draft.durationMinutes, draft.throttleMillis, draft.batteryThreshold,
                        draft.allowedActivities(), draft.blockedActivities(), draft.targets(), draft.blockedControls(), draft.targetCases,
                        draft.renderFallbackMode);
                RecentMonkeyApps.record(this, draft.packageName);
                runUi(() -> showMonkeyRunning(draft.packageName));
            } catch (Exception error) { runUi(() -> state.setText("启动失败：" + error.getMessage())); }
        });
    }

    private void showRenderFallbackMode(MonkeyDraft draft) {
        ++generation;
        LinearLayout box = column(PANEL); box.addView(titleBar("纯渲染页面策略"));
        box.addView(sectionHint("坐标兜底授权", "探测到 SurfaceView 或游戏引擎只代表页面缺少控件语义，不会自动授予无限随机输入。普通应用保持有界兜底；游戏长测需明确选择持续探索。"));
        box.addView(menuButton("有界兜底（推荐）", WHITE, v -> {
            draft.renderFallbackMode = MonkeyDraft.RENDER_BOUNDED; showMonkey(draft);
        }));
        box.addView(menuButton("关闭坐标兜底", WHITE, v -> {
            draft.renderFallbackMode = MonkeyDraft.RENDER_OFF; showMonkey(draft);
        }));
        box.addView(menuButton("持续探索（游戏）", WHITE, v -> {
            draft.renderFallbackMode = MonkeyDraft.RENDER_CONTINUOUS; showMonkey(draft);
        }));
        box.addView(menuButton("返回", WHITE, v -> showMonkey(draft)));
        replace(box, largePanelWidth(), WindowManager.LayoutParams.WRAP_CONTENT, true, 0, 0);
    }

    private void captureMonkeyNumbers(MonkeyDraft draft, EditText minutes, EditText throttle, EditText battery) {
        draft.durationMinutes = bounded(minutes, 10, 1, 10080);
        draft.throttleMillis = bounded(throttle, 500, 50, 60000);
        draft.batteryThreshold = bounded(battery, 15, 1, 100);
    }

    private void showActivityMode(MonkeyDraft draft) {
        ++generation;
        LinearLayout box = column(PANEL); box.addView(titleBar("Activity 范围"));
        box.addView(sectionHint("选择范围策略", "大多数应用保持“不限制”即可。范围策略只约束页面，不改变动态随机动作权重。"));
        box.addView(menuButton("不限制（推荐）", WHITE, v -> {
            draft.activityMode = MonkeyDraft.MODE_NONE; draft.activities.clear(); showMonkey(draft);
        }));
        box.addView(menuButton("仅探索选中的 Activity", WHITE, v -> {
            draft.activityMode = MonkeyDraft.MODE_ALLOW; showActivityPicker(draft, false);
        }));
        box.addView(menuButton("跳过选中的 Activity", WHITE, v -> {
            draft.activityMode = MonkeyDraft.MODE_BLOCK; showActivityPicker(draft, false);
        }));
        box.addView(menuButton("返回", WHITE, v -> showMonkey(draft)));
        replace(box, largePanelWidth(), WindowManager.LayoutParams.WRAP_CONTENT, true, 0, 0);
    }

    private void showActivityPicker(MonkeyDraft draft, boolean targets) {
        int token = ++generation;
        Set<String> selected = new LinkedHashSet<>(targets ? draft.targetActivities : draft.activities);
        LinearLayout box = column(PANEL); box.addView(titleBar(targets ? "选择目标页面" : "选择 Activity"));
        TextView help = sectionHint(targets ? "目标页面（可选）" : draft.scopeSummary(),
                targets ? "到达这些页面时记录目标覆盖；不限制其他页面探索。" : "支持按类名搜索并多选，无需输入完整 Activity 名称。");
        box.addView(help);
        EditText search = textField("搜索 Activity 类名"); box.addView(search);
        TextView loading = label("正在读取目标应用 Activity…", 14, WHITE); box.addView(loading);
        LinearLayout list = column(PANEL); ScrollView scroll = new ScrollView(this); scroll.addView(list);
        box.addView(scroll, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1));
        TextView count = label("已选择 " + selected.size() + " 个", 13, Color.LTGRAY); box.addView(count);
        box.addView(menuButton("应用选择", WHITE, v -> {
            if (targets) { draft.targetActivities.clear(); draft.targetActivities.addAll(selected); }
            else { draft.activities.clear(); draft.activities.addAll(selected); }
            showMonkey(draft);
        }));
        box.addView(menuButton("取消", WHITE, v -> showMonkey(draft)));
        replace(box, largePanelWidth(), largePanelHeight(), true, 0, 0);
        worker.execute(() -> {
            List<ActivityCatalog.Entry> activities = ActivityCatalog.declared(this, draft.packageName);
            runUi(() -> {
                if (token != generation) return;
                box.removeView(loading);
                renderActivityChoices(list, activities, selected, "", count);
                search.addTextChangedListener(new TextWatcher() {
                    @Override public void beforeTextChanged(CharSequence value, int start, int count, int after) { }
                    @Override public void onTextChanged(CharSequence value, int start, int before, int countValue) {
                        renderActivityChoices(list, activities, selected, value.toString(), count);
                    }
                    @Override public void afterTextChanged(Editable value) { }
                });
            });
        });
    }

    private void renderActivityChoices(LinearLayout list, List<ActivityCatalog.Entry> activities, Set<String> selected,
                                       String query, TextView count) {
        list.removeAllViews();
        String needle = query == null ? "" : query.trim().toLowerCase(Locale.ROOT);
        int shown = 0;
        for (ActivityCatalog.Entry entry : activities) {
            if (!needle.isEmpty() && !entry.fullName.toLowerCase(Locale.ROOT).contains(needle)) continue;
            CheckBox choice = new CheckBox(this);
            choice.setText(entry.title + "\n" + entry.value); choice.setTextColor(WHITE); choice.setTextSize(13);
            choice.setPadding(dp(8), dp(4), dp(8), dp(4)); choice.setChecked(selected.contains(entry.value));
            choice.setOnCheckedChangeListener((button, checked) -> {
                if (checked) selected.add(entry.value); else selected.remove(entry.value);
                count.setText("已选择 " + selected.size() + " 个");
            });
            list.addView(choice); shown++;
        }
        if (shown == 0) list.addView(label(activities.isEmpty() ? "未读取到已声明 Activity" : "没有匹配结果", 14, Color.LTGRAY));
    }

    private void showMonkeyAdvanced(MonkeyDraft draft) {
        ++generation;
        LinearLayout box = column(PANEL); box.addView(titleBar("高级配置"));
        box.addView(sectionHint("仅对本次任务生效", "这些规则随当前 Monkey 会话创建；广告触发应用重启时继续有效，任务结束后自动失效，不会保存为设备永久设置。"));
        box.addView(menuButton("控件跳过规则：" + draft.blockedControlSummary(), WHITE,
                v -> showControlRuleEditor(draft)));
        box.addView(menuButton("目标页关联用例：" + (draft.targetCases.isEmpty() ? "未设置" : "已设置"), WHITE,
                v -> showTargetCaseEditor(draft)));
        box.addView(sectionHint("作用说明", "控件规则只跳过匹配的候选控件，不退出整页；目标页用例用于到达指定页面后触发已有录制用例。"));
        box.addView(menuButton("返回", WHITE, v -> showMonkey(draft)));
        replace(box, largePanelWidth(), WindowManager.LayoutParams.WRAP_CONTENT, true, 0, 0);
    }

    private void showControlRuleEditor(MonkeyDraft draft) {
        ++generation;
        LinearLayout box = column(PANEL); box.addView(titleBar("控件跳过规则"));
        box.addView(sectionHint("本次任务临时规则", "输入控件文字、无障碍描述或资源 ID 片段。匹配后只跳过该控件，不会返回或退出当前页面。"));
        EditText input = textField("输入一条规则，例如 ad_button_close"); box.addView(input);
        TextView state = label("已添加 " + draft.blockedControls.size() + " 条", 13, Color.LTGRAY);
        LinearLayout list = column(PANEL); ScrollView scroll = new ScrollView(this); scroll.addView(list);
        renderControlRules(list, draft, state);
        box.addView(menuButton("添加规则", WHITE, v -> {
            String value = input.getText().toString().trim();
            if (value.isEmpty()) { state.setText("请输入控件文字或资源 ID"); return; }
            if (value.length() > 128) { state.setText("单条规则不能超过 128 个字符"); return; }
            draft.blockedControls.add(value); input.setText(""); renderControlRules(list, draft, state);
        }));
        box.addView(scroll, new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1));
        box.addView(state);
        box.addView(menuButton("完成", WHITE, v -> showMonkeyAdvanced(draft)));
        replace(box, largePanelWidth(), largePanelHeight(), true, 0, 0);
    }

    private void renderControlRules(LinearLayout list, MonkeyDraft draft, TextView state) {
        list.removeAllViews();
        if (draft.blockedControls.isEmpty()) {
            list.addView(label("暂无规则，默认由安全策略和动态权重处理。", 13, Color.LTGRAY));
        } else {
            for (String rule : new LinkedHashSet<>(draft.blockedControls)) {
                list.addView(menuButton("移除  " + rule, AMBER, v -> {
                    draft.blockedControls.remove(rule); renderControlRules(list, draft, state);
                }));
            }
        }
        state.setText("已添加 " + draft.blockedControls.size() + " 条");
    }

    private void showTargetCaseEditor(MonkeyDraft draft) {
        ++generation;
        LinearLayout box = column(PANEL); box.addView(titleBar("目标页关联用例"));
        box.addView(sectionHint("可选的专家功能", "仅在已有录制用例且需要到达页面后自动回放时填写。普通智能遍历保持为空即可。"));
        EditText cases = textField("每行一条：Activity=任务/用例");
        cases.setSingleLine(false); cases.setMinLines(3); cases.setText(draft.targetCases.replace('；', '\n').replace(';', '\n'));
        box.addView(cases);
        TextView state = label("Activity 建议先从“目标页面”选择器复制确认。", 13, Color.LTGRAY); box.addView(state);
        box.addView(menuButton("保存", WHITE, v -> {
            draft.targetCases = cases.getText().toString().trim().replace('\n', ';'); showMonkeyAdvanced(draft);
        }));
        box.addView(menuButton("清空", AMBER, v -> { draft.targetCases = ""; showMonkeyAdvanced(draft); }));
        box.addView(menuButton("取消", WHITE, v -> showMonkeyAdvanced(draft)));
        replace(box, largePanelWidth(), WindowManager.LayoutParams.WRAP_CONTENT, true, 0, 0);
    }

    private void showMonkeyRunning(String packageName) {
        int token = ++generation;
        LinearLayout box = column(PANEL); box.addView(label("Monkey 运行中", 16, RED));
        TextView state = label(packageName, 11, Color.LTGRAY); box.addView(state);
        box.addView(menuButton("停止", RED, v -> {
            asyncState(state, AgentClient::monkeyStop, "Monkey 已停止");
            main.postDelayed(this::showMain, 500);
        }));
        replace(box, dp(compactWidthDp()), WindowManager.LayoutParams.WRAP_CONTENT, false,
                dp(FLOATING_PANEL_MARGIN_DP), dp(FLOATING_PANEL_BOTTOM_DP));
        pollMonkey(state, token);
    }

    private void pollMonkey(TextView state, int token) {
        worker.execute(() -> {
            String response;
            boolean reachable = true;
            try { response = AgentClient.monkeyState(); }
            catch (Exception error) { reachable = false; response = "Monkey 状态暂时不可达\n" + error.getMessage(); }
            String result = response;
            boolean retry = !reachable;
            runUi(() -> {
                if (token != generation) return;
                if (retry) { state.setText(result); main.postDelayed(() -> pollMonkey(state, token), 2000); }
                else if (AgentClient.isRunning(result)) main.postDelayed(() -> pollMonkey(state, token), 500);
                else { state.setText("Monkey 已结束"); main.postDelayed(() -> { if (token == generation) showMain(); }, 700); }
            });
        });
    }

    private void showMinimized() {
        automationSuppressed = false;
        ++generation;
        FrameLayout touchTarget = new FrameLayout(this);
        TextView handle = label("‹", isTablet() ? 18 : 16, Color.argb(210, 245, 245, 245));
        handle.setGravity(Gravity.CENTER); handle.setPadding(0, 0, 0, 0);
        GradientDrawable shape = new GradientDrawable();
        shape.setColor(Color.argb(180, 72, 76, 80));
        shape.setStroke(dp(1), Color.argb(150, 150, 154, 158));
        float radius = dp(8);
        shape.setCornerRadii(new float[]{radius, radius, 0, 0, 0, 0, radius, radius});
        handle.setBackground(shape);
        int handleWidth = dp(isTablet() ? TABLET_MINIMIZED_HANDLE_WIDTH_DP : MINIMIZED_HANDLE_WIDTH_DP);
        int handleHeight = dp(isTablet() ? TABLET_MINIMIZED_HANDLE_HEIGHT_DP : MINIMIZED_HANDLE_HEIGHT_DP);
        FrameLayout.LayoutParams handleParams = new FrameLayout.LayoutParams(
                handleWidth, handleHeight, Gravity.END | Gravity.CENTER_VERTICAL);
        touchTarget.addView(handle, handleParams);

        // Keep this developer control out of target-app discovery. A random tap
        // is harmless; reopening the menu requires a deliberate long press.
        touchTarget.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO_HIDE_DESCENDANTS);
        touchTarget.setSoundEffectsEnabled(false);
        touchTarget.setOnClickListener(v -> { });
        touchTarget.setOnLongClickListener(v -> { showMain(); return true; });
        int touchWidth = dp(isTablet() ? TABLET_MINIMIZED_TOUCH_WIDTH_DP : MINIMIZED_TOUCH_WIDTH_DP);
        int touchHeight = dp(isTablet() ? TABLET_MINIMIZED_TOUCH_HEIGHT_DP : MINIMIZED_TOUCH_HEIGHT_DP);
        replace(touchTarget, touchWidth, touchHeight, false, 0, dp(MINIMIZED_TOP_INSET_DP));
    }

    private LinearLayout titleBar(String text) {
        LinearLayout bar = new LinearLayout(this); bar.setGravity(Gravity.CENTER_VERTICAL);
        bar.addView(label(text, 18, WHITE), new LinearLayout.LayoutParams(0, dp(LARGE_HEADER_HEIGHT_DP), 1));
        TextView close = closeControl(this::showMain, LARGE_HEADER_HEIGHT_DP, 22);
        bar.addView(close, new LinearLayout.LayoutParams(dp(LARGE_HEADER_HEIGHT_DP), dp(LARGE_HEADER_HEIGHT_DP)));
        return bar;
    }

    private void showMessage(String title, String detail) {
        ++generation;
        LinearLayout box = column(PANEL); box.addView(label(title, 18, RED)); box.addView(label(detail, 14, WHITE));
        box.addView(menuButton("返回", WHITE, v -> showMain()));
        replace(box, dp(compactWidthDp()), WindowManager.LayoutParams.WRAP_CONTENT, false,
                dp(FLOATING_PANEL_MARGIN_DP), dp(FLOATING_PANEL_BOTTOM_DP));
    }

    private interface Request { String run() throws Exception; }
    private void asyncState(TextView state, Request request, String success) {
        state.setText("处理中…");
        worker.execute(() -> {
            try {
                String response = request.run();
                runUi(() -> state.setText(success + (AgentClient.isRunning(response) ? "" : "\n" + AgentClient.compact(response))));
            } catch (Exception error) { runUi(() -> state.setText("操作失败：" + error.getMessage())); }
        });
    }

    private EditText numberField(String hint, String value) {
        EditText field = new EditText(this); field.setHint(hint); field.setText(value); field.setSingleLine(true);
        field.setInputType(android.text.InputType.TYPE_CLASS_NUMBER); field.setTextColor(WHITE); field.setHintTextColor(Color.GRAY);
        return field;
    }
    private static final class NumberSetting {
        final LinearLayout view;
        final EditText input;
        NumberSetting(LinearLayout view, EditText input) { this.view = view; this.input = input; }
    }
    private NumberSetting settingNumberField(String title, String description, String unit, int value) {
        LinearLayout setting = column(ROW); setting.setPadding(dp(8), dp(7), dp(8), dp(7));
        setting.addView(label(title, 15, WHITE));
        TextView detail = label(description, 11, Color.LTGRAY); detail.setPadding(dp(6), 0, dp(6), dp(3)); setting.addView(detail);
        LinearLayout inputRow = new LinearLayout(this); inputRow.setGravity(Gravity.CENTER_VERTICAL);
        EditText input = numberField("", String.valueOf(value));
        input.setContentDescription(title + "，单位" + unit);
        inputRow.addView(input, new LinearLayout.LayoutParams(0, dp(44), 1));
        TextView suffix = label(unit, 13, Color.LTGRAY); suffix.setGravity(Gravity.CENTER);
        inputRow.addView(suffix, new LinearLayout.LayoutParams(dp(58), dp(44)));
        setting.addView(inputRow);
        return new NumberSetting(setting, input);
    }
    private TextView sectionHint(String title, String description) {
        TextView value = label(title + "\n" + description, 12, Color.LTGRAY);
        value.setPadding(dp(9), dp(8), dp(9), dp(8)); value.setLineSpacing(dp(2), 1.0f);
        return value;
    }
    private EditText textField(String hint) {
        EditText field = new EditText(this); field.setHint(hint); field.setSingleLine(true);
        field.setTextColor(WHITE); field.setHintTextColor(Color.GRAY); return field;
    }
    private int bounded(EditText field, int fallback, int minimum, int maximum) {
        try { return Math.max(minimum, Math.min(maximum, Integer.parseInt(field.getText().toString()))); }
        catch (NumberFormatException ignored) { return fallback; }
    }
    private String safeCaseName(String value) {
        String result = value.trim().replaceAll("[^A-Za-z0-9._-]", "-");
        if (result.isEmpty()) result = "case-" + System.currentTimeMillis();
        return result.length() > 80 ? result.substring(0, 80) : result;
    }
    private LinearLayout column(int color) { LinearLayout value = new LinearLayout(this); value.setOrientation(LinearLayout.VERTICAL); value.setBackgroundColor(color); return value; }
    private TextView label(String text, float size, int color) {
        TextView value = new TextView(this); value.setText(text); value.setTextSize(size); value.setTextColor(color);
        value.setGravity(Gravity.CENTER_VERTICAL); value.setPadding(dp(6), 0, dp(6), 0); return value;
    }
    private TextView closeControl(Runnable action, int sizeDp, float textSizeSp) {
        TextView close = label("×", textSizeSp, RED);
        close.setGravity(Gravity.CENTER); close.setPadding(0, 0, 0, 0);
        close.setContentDescription("关闭"); close.setOnClickListener(v -> action.run());
        close.setMinWidth(dp(sizeDp)); close.setMinimumWidth(dp(sizeDp));
        close.setMinHeight(dp(sizeDp)); close.setMinimumHeight(dp(sizeDp));
        return close;
    }
    private void replace(View next, int width, int height, boolean focusable, int x, int y) {
        if (view != null) try { manager.removeView(view); } catch (IllegalArgumentException ignored) { }
        WindowManager.LayoutParams params = new WindowManager.LayoutParams(width, height,
                Build.VERSION.SDK_INT >= 26 ? WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY : WindowManager.LayoutParams.TYPE_PHONE,
                focusable ? WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL : WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE,
                PixelFormat.TRANSLUCENT);
        params.gravity = Gravity.TOP | Gravity.END; params.x = x; params.y = y;
        try { manager.addView(next, params); view = next; controlState = "visible"; }
        catch (SecurityException | WindowManager.BadTokenException error) { view = null; stopSelf(); }
    }
    private int dp(int value) { return Math.round(value * getResources().getDisplayMetrics().density); }
    private int screenWidth(double fraction) { DisplayMetrics m = getResources().getDisplayMetrics(); return (int) (m.widthPixels * fraction); }
    private int screenHeight(double fraction) { DisplayMetrics m = getResources().getDisplayMetrics(); return (int) (m.heightPixels * fraction); }
    private boolean isTablet() { return getResources().getConfiguration().smallestScreenWidthDp >= 600; }
    private int mainWidthDp() { return isTablet() ? TABLET_MAIN_WIDTH_DP : MAIN_WIDTH_DP; }
    private int mainHeightDp() { return isTablet() ? TABLET_MAIN_HEIGHT_DP : MAIN_HEIGHT_DP; }
    private int compactWidthDp() { return isTablet() ? TABLET_COMPACT_WIDTH_DP : COMPACT_WIDTH_DP; }
    private int performanceHeightDp() { return isTablet() ? TABLET_PERFORMANCE_HEIGHT_DP : PERFORMANCE_HEIGHT_DP; }
    private int recordingHeightDp() { return isTablet() ? TABLET_RECORDING_HEIGHT_DP : RECORDING_PANEL_HEIGHT_DP; }
    private int compactHeaderHeightDp() { return isTablet() ? TABLET_COMPACT_HEADER_HEIGHT_DP : COMPACT_HEADER_HEIGHT_DP; }
    private int compactTitleTextSizeSp() { return isTablet() ? 14 : 11; }
    private int compactMetricsTextSizeSp() { return isTablet() ? 14 : 12; }
    private int largePanelWidth() { return Math.min(dp(isTablet() ? TABLET_LARGE_PANEL_MAX_WIDTH_DP : LARGE_PANEL_MAX_WIDTH_DP), screenWidth(0.80)); }
    private int largePanelHeight() { return Math.min(dp(isTablet() ? TABLET_LARGE_PANEL_MAX_HEIGHT_DP : LARGE_PANEL_MAX_HEIGHT_DP), screenHeight(0.78)); }
    private void runUi(Runnable runnable) { main.post(() -> { if (!destroyed) runnable.run(); }); }

    @Override public void onDestroy() {
        destroyed = true; ++generation; worker.shutdownNow(); main.removeCallbacksAndMessages(null);
        if (view != null && manager != null) try { manager.removeView(view); } catch (IllegalArgumentException ignored) { }
        view = null; controlState = "absent"; super.onDestroy();
    }
    @Override public IBinder onBind(Intent intent) { return null; }
}
