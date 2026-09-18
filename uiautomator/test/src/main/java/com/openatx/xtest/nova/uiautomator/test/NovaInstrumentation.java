package com.openatx.xtest.nova.uiautomator.test;

import android.app.Instrumentation;
import android.app.UiAutomation;
import android.app.Activity;
import android.content.Context;
import android.graphics.Rect;
import android.hardware.display.DisplayManager;
import android.os.Build;
import android.os.Bundle;
import android.os.SystemClock;
import android.util.Log;
import android.util.Xml;
import android.view.accessibility.AccessibilityNodeInfo;
import android.view.accessibility.AccessibilityWindowInfo;
import android.view.Display;

import org.xmlpull.v1.XmlSerializer;

import java.io.BufferedReader;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.io.OutputStreamWriter;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.net.SocketTimeoutException;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Locale;

public final class NovaInstrumentation extends Instrumentation {
    private static final String TAG = "NovaUiAutomator";
    private static final int MAX_HIERARCHY_BYTES = 16 * 1024 * 1024;
    private static final int MAX_HEADER_LINES = 100;
    private volatile boolean running = true;
    private String token;
    private int port;
    private long idleTimeoutMillis;
    private long startedAtMillis;
    private long lastRequestMillis;
    private long requests;
    private long failures;
    private String lastFailure = "";
    private long lastGetWindowsMicros;
    private long lastGetRootsMicros;
    private long lastWriteNodesMicros;
    private long lastReadAttributesMicros;
    private long lastGetChildrenMicros;
    private long lastFinalizeMicros;
    private long lastHierarchyMicros;
    private int lastHierarchyNodes;

    @Override
    public void onCreate(Bundle arguments) {
        super.onCreate(arguments);
        token = arguments == null ? null : arguments.getString("token");
        if (token == null || token.length() < 32) {
            throw new IllegalArgumentException("a random token of at least 32 characters is required");
        }
        port = parseInt(arguments.getString("port", "9009"), 9009, 1024, 65535);
        idleTimeoutMillis = parseLong(arguments.getString("idleTimeoutMillis", "10800000"), 10800000L, 1000L, 86400000L);
        startedAtMillis = System.currentTimeMillis();
        lastRequestMillis = SystemClock.elapsedRealtime();
        start();
    }

    @Override
    public void onStart() {
        Bundle result = new Bundle();
        try {
            serve();
            result.putString("status", "stopped");
            finish(Activity.RESULT_OK, result);
        } catch (Throwable error) {
            result.putString("error", error.toString());
            finish(Activity.RESULT_CANCELED, result);
        }
    }

    private void serve() throws IOException {
        try (ServerSocket server = new ServerSocket()) {
            server.setReuseAddress(true);
            // Agent currently uses an explicit IPv4 loopback endpoint. Binding
            // getLoopbackAddress() may select ::1 on Android and make 127.0.0.1
            // connections fail even though adb forwarding can still reach it.
            server.bind(new InetSocketAddress(InetAddress.getByName("127.0.0.1"), port), 1);
            server.setSoTimeout(1000);
            while (running) {
                if (SystemClock.elapsedRealtime() - lastRequestMillis >= idleTimeoutMillis) {
                    return;
                }
                try (Socket socket = server.accept()) {
                    socket.setSoTimeout(15000);
                    handle(socket);
                } catch (SocketTimeoutException ignored) {
                    // Periodically check the owner-controlled idle deadline.
                } catch (Throwable requestError) {
                    failures++;
                    lastFailure = requestError.toString();
                    Log.e(TAG, "request failed", requestError);
                }
            }
        }
    }

    private void handle(Socket socket) throws Exception {
        BufferedReader reader = new BufferedReader(new InputStreamReader(socket.getInputStream(), StandardCharsets.US_ASCII));
        String requestLine = reader.readLine();
        if (requestLine == null || requestLine.length() > 4096) {
            respond(socket.getOutputStream(), 400, "application/json", "{\"error\":\"invalid request\"}");
            return;
        }
        String authorization = "";
		boolean headersComplete = false;
        for (int i = 0; i < MAX_HEADER_LINES; i++) {
            String line = reader.readLine();
            if (line == null || line.isEmpty()) {
				headersComplete = true;
                break;
            }
            if (line.length() > 8192) {
                respond(socket.getOutputStream(), 431, "application/json", "{\"error\":\"headers too large\"}");
                return;
            }
            int colon = line.indexOf(':');
            if (colon > 0 && "authorization".equals(line.substring(0, colon).trim().toLowerCase(Locale.ROOT))) {
                authorization = line.substring(colon + 1).trim();
            }
        }
		if (!headersComplete) {
			respond(socket.getOutputStream(), 431, "application/json", "{\"error\":\"too many headers\"}");
			return;
		}
        if (!("Bearer " + token).equals(authorization)) {
            respond(socket.getOutputStream(), 401, "application/json", "{\"error\":\"unauthorized\"}");
            return;
        }
        String[] requestParts = requestLine.split(" ");
        if (requestParts.length != 3 || !"GET".equals(requestParts[0])) {
            respond(socket.getOutputStream(), 405, "application/json", "{\"error\":\"method not allowed\"}");
            return;
        }
        lastRequestMillis = SystemClock.elapsedRealtime();
        requests++;
        String path = requestParts[1].split("\\?", 2)[0];
        if ("/health".equals(path)) {
            respond(socket.getOutputStream(), 200, "application/json", "{\"status\":\"ok\",\"service\":\"xtest-nova-uiautomator\"}");
        } else if ("/v1/hierarchy".equals(path)) {
            respondHierarchy(socket.getOutputStream(), hierarchy());
        } else if ("/v1/windows".equals(path)) {
            respond(socket.getOutputStream(), 200, "application/json", windows());
        } else if ("/v1/wait-stable".equals(path)) {
            getUiAutomation().waitForIdle(500, 5000);
            respond(socket.getOutputStream(), 200, "application/json", "{\"stable\":true}");
        } else if ("/v1/diagnostics".equals(path)) {
            long uptime = System.currentTimeMillis() - startedAtMillis;
            respond(socket.getOutputStream(), 200, "application/json", "{\"requests\":" + requests
                    + ",\"failures\":" + failures + ",\"uptimeMillis\":" + uptime
                    + ",\"lastHierarchyNodes\":" + lastHierarchyNodes
                    + ",\"lastHierarchyMicros\":" + lastHierarchyMicros
                    + ",\"lastGetWindowsMicros\":" + lastGetWindowsMicros
                    + ",\"lastGetRootsMicros\":" + lastGetRootsMicros
                    + ",\"lastWriteNodesMicros\":" + lastWriteNodesMicros
                    + ",\"lastReadAttributesMicros\":" + lastReadAttributesMicros
                    + ",\"lastGetChildrenMicros\":" + lastGetChildrenMicros
                    + ",\"lastFinalizeMicros\":" + lastFinalizeMicros
                    + ",\"lastFailure\":\"" + jsonEscape(lastFailure) + "\"}");
        } else {
            respond(socket.getOutputStream(), 404, "application/json", "{\"error\":\"not found\"}");
        }
    }

    private byte[] hierarchy() throws Exception {
        long hierarchyStarted = SystemClock.elapsedRealtimeNanos();
        UiAutomation automation = getUiAutomation();
        // Do not put waitForIdle on the mandatory capture path. On continuously
        // animated system UI it may not honor its global timeout. Callers that
        // require a stability barrier can use /v1/wait-stable explicitly.
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        XmlSerializer xml = Xml.newSerializer();
        xml.setOutput(new OutputStreamWriter(output, StandardCharsets.UTF_8));
        xml.startDocument("UTF-8", true);
        xml.startTag(null, "hierarchy");
        xml.attribute(null, "rotation", Integer.toString(displayRotation()));
        long windowsStarted = SystemClock.elapsedRealtimeNanos();
        List<AccessibilityWindowInfo> windows = automation.getWindows();
        lastGetWindowsMicros = elapsedMicros(windowsStarted);
        AccessibilityNodeInfo legacyActiveRoot = Build.VERSION.SDK_INT < 33
                ? automation.getRootInActiveWindow() : null;
        long getRootsNanos = 0;
        long writeNodesNanos = 0;
        lastReadAttributesMicros = 0;
        lastGetChildrenMicros = 0;
        int nodeCount = 0;
        int index = 0;
        for (AccessibilityWindowInfo window : windows) {
            try {
                long rootStarted = SystemClock.elapsedRealtimeNanos();
                AccessibilityNodeInfo root;
                if (legacyActiveRoot != null && window.isActive()) {
                    root = legacyActiveRoot;
                    legacyActiveRoot = null;
                } else {
                    root = window.getRoot();
                }
                getRootsNanos += SystemClock.elapsedRealtimeNanos() - rootStarted;
                if (root != null) {
                    long writeStarted = SystemClock.elapsedRealtimeNanos();
                    nodeCount += writeNode(xml, root, index++);
                    writeNodesNanos += SystemClock.elapsedRealtimeNanos() - writeStarted;
                    root.recycle();
                }
            } finally {
                recycleWindowIfRequired(window);
            }
        }
        // Some Android/OEM combinations temporarily return an empty window
        // list even though an active accessibility root is available. The old
        // SDK-gated fallback left Android 13+ returning `<hierarchy/>`, which
        // made the fast provider unusable and also blocked system dump fallback.
        if (nodeCount == 0) {
            AccessibilityNodeInfo activeRoot = legacyActiveRoot != null
                    ? legacyActiveRoot : automation.getRootInActiveWindow();
            legacyActiveRoot = null;
            if (activeRoot != null) {
                long writeStarted = SystemClock.elapsedRealtimeNanos();
                nodeCount += writeNode(xml, activeRoot, index);
                writeNodesNanos += SystemClock.elapsedRealtimeNanos() - writeStarted;
                activeRoot.recycle();
            }
        }
        if (legacyActiveRoot != null) {
            legacyActiveRoot.recycle();
        }
        long finalizeStarted = SystemClock.elapsedRealtimeNanos();
        xml.endTag(null, "hierarchy");
        xml.endDocument();
        xml.flush();
        byte[] result = output.toByteArray();
        lastGetRootsMicros = getRootsNanos / 1000L;
        lastWriteNodesMicros = writeNodesNanos / 1000L;
        lastFinalizeMicros = elapsedMicros(finalizeStarted);
        lastHierarchyNodes = nodeCount;
        lastHierarchyMicros = elapsedMicros(hierarchyStarted);
        if (result.length > MAX_HIERARCHY_BYTES) {
            throw new IOException("hierarchy exceeds response limit");
        }
        return result;
    }

    private int writeNode(XmlSerializer xml, AccessibilityNodeInfo node, int index) throws Exception {
        int count = 1;
        long attributesStarted = SystemClock.elapsedRealtimeNanos();
        xml.startTag(null, "node");
        attribute(xml, "index", Integer.toString(index));
        attribute(xml, "text", node.getText());
        attribute(xml, "resource-id", node.getViewIdResourceName());
        attribute(xml, "class", node.getClassName());
        attribute(xml, "package", node.getPackageName());
        attribute(xml, "content-desc", node.getContentDescription());
        attribute(xml, "checkable", node.isCheckable());
        attribute(xml, "checked", node.isChecked());
        attribute(xml, "clickable", node.isClickable());
        attribute(xml, "enabled", node.isEnabled());
        attribute(xml, "focusable", node.isFocusable());
        attribute(xml, "focused", node.isFocused());
        attribute(xml, "scrollable", node.isScrollable());
        attribute(xml, "long-clickable", node.isLongClickable());
        attribute(xml, "password", node.isPassword());
        attribute(xml, "selected", node.isSelected());
        Rect bounds = new Rect();
        node.getBoundsInScreen(bounds);
        attribute(xml, "bounds", bounds.toShortString());
        lastReadAttributesMicros += elapsedMicros(attributesStarted);
        for (int childIndex = 0; childIndex < node.getChildCount(); childIndex++) {
            long childStarted = SystemClock.elapsedRealtimeNanos();
            AccessibilityNodeInfo child = node.getChild(childIndex);
            lastGetChildrenMicros += elapsedMicros(childStarted);
            if (child != null) {
                count += writeNode(xml, child, childIndex);
                child.recycle();
            }
        }
        xml.endTag(null, "node");
        return count;
    }

    private String windows() {
        List<AccessibilityWindowInfo> values = getUiAutomation().getWindows();
        StringBuilder json = new StringBuilder("{\"windows\":[");
        for (int i = 0; i < values.size(); i++) {
            AccessibilityWindowInfo window = values.get(i);
            try {
                Rect bounds = new Rect();
                window.getBoundsInScreen(bounds);
                if (i > 0) json.append(',');
                json.append("{\"id\":").append(window.getId())
                        .append(",\"type\":").append(window.getType())
                        .append(",\"layer\":").append(window.getLayer())
                        .append(",\"active\":").append(window.isActive())
                        .append(",\"focused\":").append(window.isFocused())
                        .append(",\"bounds\":\"").append(jsonEscape(bounds.toShortString())).append("\"}");
            } finally {
                recycleWindowIfRequired(window);
            }
        }
        return json.append("]}").toString();
    }

    @SuppressWarnings("deprecation")
    private static void recycleWindowIfRequired(AccessibilityWindowInfo window) {
        if (AccessibilityRecyclingPolicy.shouldRecycleWindow(Build.VERSION.SDK_INT)) {
            window.recycle();
        }
    }

    private static void attribute(XmlSerializer xml, String name, Object value) throws IOException {
        xml.attribute(null, name, value == null ? "" : value.toString());
    }

    private static void attribute(XmlSerializer xml, String name, boolean value) throws IOException {
        xml.attribute(null, name, Boolean.toString(value));
    }

    private static String jsonEscape(String value) {
        return value.replace("\\", "\\\\").replace("\"", "\\\"");
    }

    private static void respond(OutputStream output, int status, String contentType, String body) throws IOException {
        respondBytes(output, status, contentType + "; charset=utf-8", body.getBytes(StandardCharsets.UTF_8));
    }

    private static void respondBytes(OutputStream output, int status, String contentType, byte[] body) throws IOException {
        String reason = status == 200 ? "OK" : "Error";
        String headers = "HTTP/1.1 " + status + " " + reason + "\r\nContent-Type: " + contentType
                + "\r\nContent-Length: " + body.length + "\r\nCache-Control: no-store\r\nConnection: close\r\n\r\n";
        output.write(headers.getBytes(StandardCharsets.US_ASCII));
        output.write(body);
        output.flush();
    }

    private void respondHierarchy(OutputStream output, byte[] body) throws IOException {
        String headers = "HTTP/1.1 200 OK\r\nContent-Type: application/xml; charset=utf-8"
                + "\r\nContent-Length: " + body.length + "\r\nCache-Control: no-store\r\nConnection: close"
                + "\r\nX-Nova-Hierarchy-Micros: " + lastHierarchyMicros
                + "\r\nX-Nova-Get-Windows-Micros: " + lastGetWindowsMicros
                + "\r\nX-Nova-Get-Roots-Micros: " + lastGetRootsMicros
                + "\r\nX-Nova-Write-Nodes-Micros: " + lastWriteNodesMicros
                + "\r\nX-Nova-Read-Attributes-Micros: " + lastReadAttributesMicros
                + "\r\nX-Nova-Get-Children-Micros: " + lastGetChildrenMicros
                + "\r\nX-Nova-Finalize-Micros: " + lastFinalizeMicros
                + "\r\nX-Nova-Hierarchy-Nodes: " + lastHierarchyNodes + "\r\n\r\n";
        output.write(headers.getBytes(StandardCharsets.US_ASCII));
        output.write(body);
        output.flush();
    }

    private static long elapsedMicros(long startedNanos) {
        return (SystemClock.elapsedRealtimeNanos() - startedNanos) / 1000L;
    }

    private int displayRotation() {
        DisplayManager manager = (DisplayManager) getContext().getSystemService(Context.DISPLAY_SERVICE);
        Display display = manager == null ? null : manager.getDisplay(Display.DEFAULT_DISPLAY);
        return display == null ? 0 : display.getRotation();
    }

    private static int parseInt(String value, int fallback, int minimum, int maximum) {
        long result = parseLong(value, fallback, minimum, maximum);
        return (int) result;
    }

    private static long parseLong(String value, long fallback, long minimum, long maximum) {
        try {
            long parsed = Long.parseLong(value);
            return parsed >= minimum && parsed <= maximum ? parsed : fallback;
        } catch (RuntimeException ignored) {
            return fallback;
        }
    }
}
