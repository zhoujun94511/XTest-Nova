package com.openatx.xtest.popup;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import org.json.JSONArray;
import org.json.JSONObject;

final class AgentClient {
    private static final int MAX_RESPONSE_BYTES = 1024 * 1024;
    private static final String PRIMARY = "http://127.0.0.1:7912";
    private static final String COMPANION = "http://127.0.0.1:8912";
    private static volatile String recordingSessionId = "";
    private static volatile String recordingOwnerToken = "";
    private static volatile String replaySessionId = "";
    private static volatile String replayOwnerToken = "";
    private static volatile String monkeySessionId = "";
    private static volatile String monkeyOwnerToken = "";
    private static volatile String performanceSessionId = "";
    private static volatile String performanceOwnerToken = "";
    static final class CaseEntry {
        final String id;
        final String name;
        final int actions;
        final String task;
        CaseEntry(String id, String name, int actions, String task) { this.id = id; this.name = name; this.actions = actions; this.task = task == null ? "" : task; }
    }
    static final class DraftEntry {
        final String id;
        final String name;
        final int actions;
        final String task;
        DraftEntry(String id, String name, int actions, String task) { this.id = id; this.name = name; this.actions = actions; this.task = task == null ? "" : task; }
    }

    static String monkeyStart(String pkg, int minutes, int throttle, int minBattery, String allowActivities,
                              String blockActivities, String targetActivities, String blockedControls, String targetCases,
                              String renderFallbackMode) throws IOException {
        String mode = !allowActivities.trim().isEmpty() ? "allowlist" : (!blockActivities.trim().isEmpty() ? "blocklist" : "none");
        String activities = "allowlist".equals(mode) ? allowActivities : blockActivities;
        String response = request(COMPANION, "POST", "/monkey", "{\"package\":\"" + escape(pkg)
                + "\",\"durationSeconds\":" + (minutes * 60)
                + ",\"throttleMillis\":" + throttle
                + ",\"lowBatteryExit\":true,\"minBatteryPercent\":" + minBattery
                + ",\"activityMode\":\"" + mode + "\",\"activities\":" + stringArray(activities)
                + ",\"renderFallbackMode\":\"" + escape(renderFallbackMode) + "\""
                + ",\"targetActivities\":" + stringArray(targetActivities)
                + ",\"controlBlacklist\":" + stringArray(blockedControls)
                + ",\"targetCases\":" + targetCaseArray(targetCases) + "}");
        rememberMonkeyIdentity(response);
        return response;
    }
    static String monkeyState() throws IOException {
        String response = request(COMPANION, "GET", "/monkey", null);
        rememberMonkeyIdentity(response);
        return response;
    }
    static String monkeyStop() throws IOException {
        String response = request(COMPANION, "DELETE", "/monkey", null, monkeySessionId, monkeyOwnerToken);
        monkeySessionId = "";
        monkeyOwnerToken = "";
        return response;
    }
    static String performanceStart(String pkg) throws IOException {
        String response = request(PRIMARY, "POST", "/v1/performance/sessions", "{\"package\":\"" + escape(pkg) + "\"}");
        rememberPerformanceIdentity(response);
        return response;
    }
    static String performanceState() throws IOException {
        String response = request(PRIMARY, "GET", "/v1/performance/sessions/current", null);
        rememberPerformanceIdentity(response);
        return response;
    }
    static String performanceStop() throws IOException {
        String response = request(PRIMARY, "DELETE", "/v1/performance/sessions/current", null, performanceSessionId, performanceOwnerToken);
        performanceSessionId = "";
        performanceOwnerToken = "";
        return response;
    }
    static String formatPerformance(String response) {
        try {
            JSONObject root = new JSONObject(response);
            JSONObject sample = root.optJSONObject("last");
            if (sample == null) sample = root;
            JSONObject cpu = sample.optJSONObject("cpuinfo");
            JSONObject memory = sample.optJSONObject("memoinfo");
            JSONObject network = sample.optJSONObject("network");
            JSONObject gpuValue = sample.optJSONObject("gpu");
            JSONObject battery = sample.optJSONObject("battery");
            JSONObject states = sample.optJSONObject("metricStates");
            if (cpu == null) cpu = new JSONObject();
            if (memory == null) memory = new JSONObject();
            if (network == null) network = new JSONObject();
            if (battery == null) battery = new JSONObject();
            long memoryKb = memory.has("total pss") ? memory.optLong("total pss") : memory.optLong("total", -1);
            String cpuText = metricText(states, "cpu", String.format(Locale.US, "%.2f%%", cpu.optDouble("percent", 0)), "空闲");
            String systemText = metricText(states, "cpu", String.format(Locale.US, "%.2f%%", cpu.optDouble("systemPercent", 0)), "空闲");
            String fpsValue = sample.isNull("fps") ? "--" : String.format(Locale.US, "%.1f", sample.optDouble("fps", 0));
            String fps = metricText(states, "fps", fpsValue, "静止");
            String gpu = gpuValue == null ? "--" : String.format(Locale.US, "%.1f%%", gpuValue.optDouble("percent", 0));
            gpu = metricText(states, "gpu", gpu, "空闲");
            String current = battery.isNull("currentMa") ? "--" : String.format(Locale.US, "%.1f mA", battery.optDouble("currentMa", 0));
            current = metricText(states, "battery_current", current, "空闲");
            String networkState = metricState(states, "network");
            String rx = "failed".equals(networkState) ? "--（失败）" : formatBytes(network.optLong("rx", 0));
            String tx = "failed".equals(networkState) ? "--（失败）" : formatBytes(network.optLong("tx", 0));
			String networkRateState = metricState(states, "network_rate");
			String rxRate = metricText(states, "network_rate", formatBytes((long) network.optDouble("rxBytesPerSecond", 0)) + "/秒", "空闲");
			String txRate = metricText(states, "network_rate", formatBytes((long) network.optDouble("txBytesPerSecond", 0)) + "/秒", "空闲");
			if ("warming_up".equals(networkRateState)) {
				rxRate = txRate = "--（预热中）";
			}
            return "PID: " + cpu.optLong("pid", 0)
                    + "\nCPU: " + cpuText
                    + "\n系统: " + systemText
                    + "\n内存: " + (memoryKb < 0 ? "--" : formatKilobytes(memoryKb))
                    + "\nFPS: " + fps
                    + "\nGPU: " + gpu
                    + "\n电流原值: " + current
                    + "\n电池: " + batteryStatus(battery.optString("status", "unknown")) + " " + battery.optInt("levelPercent", 0) + "%"
                    + "\n温度: " + String.format(Locale.US, "%.1f°C", battery.optDouble("temperatureC", 0))
                    + "\n下行速率: " + rxRate
					+ "\n上行速率: " + txRate
					+ "\n累计接收: " + rx
					+ "\n累计发送: " + tx;
        } catch (Exception ignored) {
            return "性能数据暂不可解析";
        }
    }

    private static String metricState(JSONObject states, String key) {
        if (states == null) return "";
        JSONObject value = states.optJSONObject(key);
        return value == null ? "" : value.optString("state", "");
    }

    private static String metricText(JSONObject states, String key, String value, String idleLabel) {
        String state = metricState(states, key);
        if ("warming_up".equals(state)) return "--（预热中）";
        if ("unsupported".equals(state)) return "--（不支持）";
        if ("failed".equals(state)) return "--（失败）";
        if ("idle".equals(state)) return value + "（" + idleLabel + "）";
        return value;
    }
    static String formatPerformanceSession(String response) {
        String path = string(response, "path", "");
        return formatPerformance(response) + "\n记录: " + integer(response, "rows", 0) + " 行"
                + (path.isEmpty() ? "" : "\n文件: " + path);
    }
    static String recordingStart(String pkg, String task, String name, double overlayLeft, double overlayBottom) throws IOException {
        String response = request(PRIMARY, "POST", "/v1/recordings", "{\"package\":\"" + escape(pkg) + "\",\"task\":\"" + escape(task) + "\",\"name\":\"" + escape(name)
                + "\",\"excludedBounds\":{\"left\":" + overlayLeft + ",\"top\":0,\"right\":1,\"bottom\":" + overlayBottom + "}}");
        rememberRecordingIdentity(response);
        return response;
    }
    static String recordingStop() throws IOException {
        String response = request(PRIMARY, "DELETE", "/v1/recordings/current", null, recordingSessionId, recordingOwnerToken);
        recordingSessionId = "";
        recordingOwnerToken = "";
        return response;
    }
    static String recordingState() throws IOException {
        String response = request(PRIMARY, "GET", "/v1/recordings/current", null);
        rememberRecordingIdentity(response);
        return response;
    }
    static String recordingBack() throws IOException { return request(PRIMARY, "POST", "/v1/recordings/current/key", "{\"keyCode\":4}", recordingSessionId, recordingOwnerToken); }
    static String recordingScreenshot() throws IOException { return request(PRIMARY, "POST", "/v1/recordings/current/assertions/screenshot", "{\"maxHashDistance\":16}", recordingSessionId, recordingOwnerToken); }
    static String recordingText(String text) throws IOException { return request(PRIMARY, "POST", "/v1/recordings/current/text", "{\"text\":\"" + escape(text) + "\"}", recordingSessionId, recordingOwnerToken); }
    static String recordingFocusedText() throws IOException { return request(PRIMARY, "POST", "/v1/recordings/current/focused-text", "{}", recordingSessionId, recordingOwnerToken); }
    static List<CaseEntry> recordingCases(String pkg) throws IOException {
        String response = request(PRIMARY, "GET", "/v1/recordings?package=" + path(pkg), null);
        List<CaseEntry> result = new ArrayList<>();
        try {
            JSONArray values = new JSONObject(response).getJSONArray("cases");
            for (int index = 0; index < values.length(); index++) {
                JSONObject value = values.getJSONObject(index);
                result.add(new CaseEntry(value.getString("id"), value.getString("name"), value.optInt("actions", 0), value.optString("task", "")));
            }
        } catch (Exception error) {
            throw new IOException("Agent returned invalid recording list JSON", error);
        }
        return result;
    }
    static String recordingCase(String id) throws IOException {
        return request(PRIMARY, "GET", "/v1/recordings/cases/" + path(id), null);
    }
    static List<DraftEntry> recordingDrafts(String pkg) throws IOException {
        String response = request(PRIMARY, "GET", "/v1/recordings/drafts?package=" + path(pkg), null);
        List<DraftEntry> result = new ArrayList<>();
        try {
            JSONArray values = new JSONObject(response).getJSONArray("drafts");
            for (int index = 0; index < values.length(); index++) {
                JSONObject value = values.getJSONObject(index);
                result.add(new DraftEntry(value.getString("id"), value.getString("name"), value.optInt("actions", 0), value.optString("task", "")));
            }
        } catch (Exception error) {
            throw new IOException("Agent returned invalid recording draft list JSON", error);
        }
        return result;
    }
    static String recordingDraftFinalize(String id) throws IOException {
        return request(PRIMARY, "POST", "/v1/recordings/drafts/" + path(id) + "/finalize", "{}");
    }
    static String replayStart(String caseJson) throws IOException {
        String response = request(PRIMARY, "POST", "/v1/replays", "{\"execute\":true,\"speed\":1,\"case\":" + caseJson + "}");
        rememberReplayIdentity(response);
        return response;
    }
    static String replayState() throws IOException {
        String response = request(PRIMARY, "GET", "/v1/replays/current", null);
        rememberReplayIdentity(response);
        return response;
    }
    static String replayStop() throws IOException {
        String response = request(PRIMARY, "DELETE", "/v1/replays/current", null, replaySessionId, replayOwnerToken);
        replaySessionId = "";
        replayOwnerToken = "";
        return response;
    }
    static boolean isRunning(String response) { return booleanValue(response, "running", false); }
    static boolean isCompleted(String response) { return "completed".equals(string(response, "stopReason", "")); }
    static int actions(String response) {
        return (int) integer(response, "actions", 0);
    }
    static String compact(String response) {
        if (response == null) return "";
        String value = response.replace("{", "").replace("}", "").replace("\"", "").replace(",", "\n");
        return value.length() > 700 ? value.substring(0, 700) + "…" : value;
    }

    private static String object(String json, String key) {
        try {
            JSONObject value = new JSONObject(json).optJSONObject(key);
            return value == null ? "{}" : value.toString();
        } catch (Exception ignored) { return "{}"; }
    }

    private static long integer(String json, String key, long fallback) {
        try { return new JSONObject(json).optLong(key, fallback); }
        catch (Exception ignored) { return fallback; }
    }

    private static double decimal(String json, String key, double fallback) {
        try { return new JSONObject(json).optDouble(key, fallback); }
        catch (Exception ignored) { return fallback; }
    }

    private static String string(String json, String key, String fallback) {
        try { return new JSONObject(json).optString(key, fallback); }
        catch (Exception ignored) { return fallback; }
    }

    private static boolean booleanValue(String json, String key, boolean fallback) {
        try { return new JSONObject(json).optBoolean(key, fallback); }
        catch (Exception ignored) { return fallback; }
    }

    private static String formatKilobytes(long value) {
        if (value >= 1024 * 1024) return String.format(Locale.US, "%.2f GB", value / 1048576.0);
        if (value >= 1024) return String.format(Locale.US, "%.2f MB", value / 1024.0);
        return value + " KB";
    }

    private static String formatBytes(long value) {
        if (value >= 1024L * 1024 * 1024) return String.format(Locale.US, "%.2f GB", value / 1073741824.0);
        if (value >= 1024L * 1024) return String.format(Locale.US, "%.2f MB", value / 1048576.0);
        if (value >= 1024) return String.format(Locale.US, "%.2f KB", value / 1024.0);
        return value + " B";
    }

    private static String batteryStatus(String value) {
        if ("charging".equals(value)) return "充电中";
        if ("discharging".equals(value)) return "放电中";
        if ("not_charging".equals(value)) return "未充电";
        if ("full".equals(value)) return "已充满";
        return "未知";
    }

    private static String request(String base, String method, String path, String body) throws IOException {
        return request(base, method, path, body, "", "");
    }

    private static String request(String base, String method, String path, String body, String sessionId, String ownerToken) throws IOException {
        HttpURLConnection connection = (HttpURLConnection) new URL(base + path).openConnection();
        try {
            connection.setRequestMethod(method);
            connection.setConnectTimeout(3000);
            connection.setReadTimeout("DELETE".equals(method) ? 15000 : 5000);
            connection.setRequestProperty("X-XTest-Control", "true");
            if (sessionId != null && !sessionId.isEmpty()) connection.setRequestProperty("X-XTest-Session-Id", sessionId);
            if (ownerToken != null && !ownerToken.isEmpty()) connection.setRequestProperty("X-XTest-Owner-Token", ownerToken);
            if (body != null) {
                connection.setDoOutput(true);
                connection.setRequestProperty("Content-Type", "application/json");
                try (OutputStream output = connection.getOutputStream()) { output.write(body.getBytes(StandardCharsets.UTF_8)); }
            }
            int status = connection.getResponseCode();
            InputStream stream = status < 400 ? connection.getInputStream() : connection.getErrorStream();
            if (stream == null) throw new IOException("Agent returned HTTP " + status + " without a response body");
            try (InputStream input = stream; ByteArrayOutputStream output = new ByteArrayOutputStream()) {
                byte[] buffer = new byte[1024];
                for (int count; (count = input.read(buffer)) >= 0; ) {
                    if (output.size() + count > MAX_RESPONSE_BYTES) throw new IOException("Agent response is too large");
                    output.write(buffer, 0, count);
                }
                String response = output.toString("UTF-8");
                if (status >= 400) throw new IOException("Agent returned HTTP " + status + ": " + response);
                return response;
            }
        } finally { connection.disconnect(); }
    }

    private static void rememberRecordingIdentity(String response) {
        String identity = object(response, "identity");
        recordingSessionId = string(identity, "sessionId", recordingSessionId);
        recordingOwnerToken = string(identity, "ownerToken", recordingOwnerToken);
    }

    private static void rememberReplayIdentity(String response) {
        String identity = object(response, "identity");
        replaySessionId = string(identity, "sessionId", replaySessionId);
        replayOwnerToken = string(identity, "ownerToken", replayOwnerToken);
    }

    private static void rememberMonkeyIdentity(String response) {
        String identity = object(response, "identity");
        monkeySessionId = string(identity, "sessionId", monkeySessionId);
        monkeyOwnerToken = string(identity, "ownerToken", monkeyOwnerToken);
    }

    private static void rememberPerformanceIdentity(String response) {
        String identity = object(response, "identity");
        performanceSessionId = string(identity, "sessionId", performanceSessionId);
        performanceOwnerToken = string(identity, "ownerToken", performanceOwnerToken);
    }

    private static String path(String value) throws IOException { return URLEncoder.encode(value, "UTF-8").replace("+", "%20"); }
    private static String stringArray(String csv) {
        StringBuilder result = new StringBuilder("[");
        for (String raw : csv.split(",")) {
            String value = raw.trim();
            if (value.isEmpty()) continue;
            if (result.length() > 1) result.append(',');
            result.append('\"').append(escape(value)).append('\"');
        }
        return result.append(']').toString();
    }
    private static String targetCaseArray(String definitions) throws IOException {
        StringBuilder result = new StringBuilder("[");
        for (String raw : definitions.split("[;；]")) {
            String value = raw.trim();
            if (value.isEmpty()) continue;
            int equals = value.indexOf('=');
            int slash = value.lastIndexOf('/');
            if (equals < 1 || slash <= equals + 1 || slash == value.length() - 1) {
                throw new IOException("目标页用例格式应为 Activity=任务/用例；...");
            }
            String activity = value.substring(0, equals).trim();
            String task = value.substring(equals + 1, slash).trim();
            String caseName = value.substring(slash + 1).trim();
            if (result.length() > 1) result.append(',');
            result.append("{\"activity\":\"").append(escape(activity)).append("\",\"task\":\"")
                    .append(escape(task)).append("\",\"case\":\"").append(escape(caseName)).append("\"}");
        }
        return result.append(']').toString();
    }
    static String escape(String value) { return value.replace("\\", "\\\\").replace("\"", "\\\""); }
}
