package com.xtest.nova.fixture;

import android.Manifest;
import android.app.Activity;
import android.content.pm.PackageManager;
import android.os.Bundle;
import android.os.Handler;
import android.view.View;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.TextView;

public final class PermissionScenarioActivity extends Activity {
    private static final int CAMERA_REQUEST = 91;
    private TextView status;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(24, 220, 24, 24);
        status = new TextView(this);
        status.setText("相机权限状态：" + permissionState());
        status.setContentDescription("permission-status");
        status.setTextSize(20);
        Button request = new Button(this);
        request.setText("请求相机权限");
        request.setContentDescription("permission-camera-request");
        request.setOnClickListener(new View.OnClickListener() { @Override public void onClick(View view) { requestCamera(); } });
        content.addView(status);
        content.addView(request);
        setContentView(content);
        if (getIntent().getBooleanExtra("auto", false)) new Handler(getMainLooper()).postDelayed(new Runnable() { @Override public void run() { requestCamera(); } }, 350);
    }

    private String permissionState() { return checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED ? "已授权" : "未授权"; }
    private void requestCamera() { requestPermissions(new String[]{Manifest.permission.CAMERA}, CAMERA_REQUEST); }

    @Override public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] results) {
        super.onRequestPermissionsResult(requestCode, permissions, results);
        if (requestCode == CAMERA_REQUEST) status.setText("相机权限状态：" + permissionState());
    }
}
