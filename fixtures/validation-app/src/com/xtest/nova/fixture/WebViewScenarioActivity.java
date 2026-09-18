package com.xtest.nova.fixture;

import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebSettings;
import android.webkit.WebView;

public final class WebViewScenarioActivity extends Activity {
    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        WebView.setWebContentsDebuggingEnabled(true);
        WebView webView = new WebView(this);
        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        String html = "<!doctype html><html><body><h1>本地 WebView 场景</h1>" +
            "<label for='fixtureInput'>Web 输入</label><input id='fixtureInput' value='web-fixture'>" +
            "<button id='fixtureButton' onclick=\"document.getElementById('result').textContent='web-clicked'\">执行脚本</button>" +
            "<p id='result'>web-idle</p></body></html>";
        webView.loadDataWithBaseURL("https://fixture.invalid/", html, "text/html", "UTF-8", null);
        setContentView(webView);
    }
}
