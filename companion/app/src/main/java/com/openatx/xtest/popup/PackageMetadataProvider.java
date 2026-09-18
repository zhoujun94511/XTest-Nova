package com.openatx.xtest.popup;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.content.Context;
import android.content.Intent;
import android.content.res.Configuration;
import android.content.pm.ActivityInfo;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.ResolveInfo;
import android.database.Cursor;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.drawable.Drawable;
import android.net.Uri;
import android.os.Binder;
import android.os.Build;
import android.os.Bundle;
import android.os.ParcelFileDescriptor;
import android.os.Process;
import java.io.ByteArrayOutputStream;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.io.OutputStream;
import java.nio.charset.StandardCharsets;
import java.util.Set;
import java.util.TreeSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.regex.Pattern;
import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

/** Read-only package metadata bridge for the shell-owned Nova Agent. */
public final class PackageMetadataProvider extends ContentProvider {
    private static final Pattern PACKAGE = Pattern.compile("^[A-Za-z][A-Za-z0-9_]*(\\.[A-Za-z][A-Za-z0-9_]*)+$");
    private static final int ICON_SIZE = 192;

    @Override public boolean onCreate() { return true; }

    @Override public Bundle call(String method, String argument, Bundle extras) {
        try {
            authorize();
        } catch (FileNotFoundException error) {
            throw new SecurityException(error.getMessage(), error);
        }
        if ("overlay-status".equals(method)) {
            Bundle result = new Bundle();
            result.putString("state", OverlayService.controlState());
            return result;
        }
        final String action;
        final String state;
        if ("overlay-suppress".equals(method)) {
            action = OverlayService.ACTION_SUPPRESS;
            state = "requested-suppress";
        } else if ("overlay-restore".equals(method)) {
            action = OverlayService.ACTION_RESTORE;
            state = "requested-restore";
        } else {
            throw new IllegalArgumentException("unknown control method");
        }
        Context context = getContext();
        Intent intent = new Intent(context, OverlayService.class).setAction(action);
        if (Build.VERSION.SDK_INT >= 26) context.startForegroundService(intent); else context.startService(intent);
        Bundle result = new Bundle();
        result.putString("state", state);
        return result;
    }

    @Override public ParcelFileDescriptor openFile(Uri uri, String mode) throws FileNotFoundException {
        authorize();
        if (!"r".equals(mode)) throw new FileNotFoundException("read-only provider");
        if (uri.getPathSegments().size() != 2) throw new FileNotFoundException("invalid metadata path");
        String operation = uri.getPathSegments().get(0);
        String packageName = uri.getPathSegments().get(1);
        if (!"catalog".equals(operation) && !PACKAGE.matcher(packageName).matches()) throw new FileNotFoundException("invalid package name");
        final byte[] data;
        try {
            if ("activities".equals(operation)) data = activities(packageName);
            else if ("icon".equals(operation)) data = icon(packageName);
            else if ("labels".equals(operation)) data = labels(packageName);
            else if ("catalog".equals(operation) && "launchable".equals(packageName)) data = catalog();
            else throw new FileNotFoundException("unknown metadata operation");
        } catch (PackageManager.NameNotFoundException error) {
            throw new FileNotFoundException("package not found");
        } catch (IOException error) {
            throw new FileNotFoundException(error.getMessage());
        }
        try {
            ParcelFileDescriptor[] pipe = ParcelFileDescriptor.createPipe();
            Thread writer = new Thread(() -> writePipe(pipe[1], data), "nova-package-metadata");
            writer.setDaemon(true);
            writer.start();
            return pipe[0];
        } catch (IOException error) {
            throw new FileNotFoundException("cannot create metadata pipe");
        }
    }

    @SuppressWarnings("deprecation")
    private byte[] activities(String packageName) throws PackageManager.NameNotFoundException {
        PackageManager manager = getContext().getPackageManager();
        PackageInfo info = manager.getPackageInfo(packageName, PackageManager.GET_ACTIVITIES | PackageManager.MATCH_DISABLED_COMPONENTS);
        Set<String> names = new TreeSet<>();
        if (info.activities != null) {
            for (ActivityInfo activity : info.activities) {
                if (activity != null && activity.name != null && !activity.name.trim().isEmpty()) names.add(activity.name.trim());
            }
        }
        StringBuilder output = new StringBuilder();
        for (String name : names) output.append(name).append('\n');
        return output.toString().getBytes(StandardCharsets.UTF_8);
    }

    private byte[] icon(String packageName) throws PackageManager.NameNotFoundException, IOException {
        Drawable icon = getContext().getPackageManager().getApplicationIcon(packageName);
        Bitmap bitmap = Bitmap.createBitmap(ICON_SIZE, ICON_SIZE, Bitmap.Config.ARGB_8888);
        Canvas canvas = new Canvas(bitmap);
        canvas.drawColor(Color.WHITE);
        int inset = ICON_SIZE / 16;
        icon.setBounds(inset, inset, ICON_SIZE - inset, ICON_SIZE - inset);
        icon.draw(canvas);
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        if (!bitmap.compress(Bitmap.CompressFormat.JPEG, 85, output)) throw new IOException("package icon encoding failed");
        bitmap.recycle();
        return output.toByteArray();
    }

    private byte[] labels(String packageName) throws PackageManager.NameNotFoundException, IOException {
        try {
            JSONObject value = new JSONObject();
            value.put("zh", labelForLocale(packageName, Locale.SIMPLIFIED_CHINESE));
            value.put("en", labelForLocale(packageName, Locale.ENGLISH));
            return value.toString().getBytes(StandardCharsets.UTF_8);
        } catch (JSONException error) {
            throw new IOException("package label encoding failed", error);
        }
    }

    private byte[] catalog() throws IOException {
        PackageManager manager = getContext().getPackageManager();
        Intent launcher = new Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER);
        List<ResolveInfo> resolved = manager.queryIntentActivities(launcher, PackageManager.MATCH_DISABLED_COMPONENTS);
        Map<String, Boolean> packages = new LinkedHashMap<>();
        for (ResolveInfo item : resolved) {
            if (item != null && item.activityInfo != null && item.activityInfo.packageName != null) {
                packages.put(item.activityInfo.packageName, Boolean.TRUE);
            }
        }
        JSONArray output = new JSONArray();
        try {
            for (String packageName : packages.keySet()) {
                JSONObject item = new JSONObject();
                item.put("packageName", packageName);
                try {
                    item.put("zh", labelForLocale(packageName, Locale.SIMPLIFIED_CHINESE));
                    item.put("en", labelForLocale(packageName, Locale.ENGLISH));
                } catch (PackageManager.NameNotFoundException ignored) {
                    continue;
                }
                output.put(item);
            }
        } catch (JSONException error) {
            throw new IOException("package catalog encoding failed", error);
        }
        return output.toString().getBytes(StandardCharsets.UTF_8);
    }

    @SuppressWarnings("deprecation")
    private String labelForLocale(String packageName, Locale locale) throws PackageManager.NameNotFoundException {
        Configuration configuration = new Configuration(getContext().getResources().getConfiguration());
        configuration.setLocale(locale);
        Context localized = getContext().createConfigurationContext(configuration);
        PackageManager manager = localized.getPackageManager();
        ApplicationInfo info = manager.getApplicationInfo(packageName, PackageManager.MATCH_DISABLED_COMPONENTS);
        CharSequence label = manager.getApplicationLabel(info);
        return label == null ? "" : label.toString().trim();
    }

    private static void writePipe(ParcelFileDescriptor descriptor, byte[] data) {
        try (OutputStream output = new ParcelFileDescriptor.AutoCloseOutputStream(descriptor)) {
            output.write(data);
        } catch (IOException ignored) {
            // The Agent may cancel or time out and close the read side.
        }
    }

    private static void authorize() throws FileNotFoundException {
        int uid = Binder.getCallingUid();
        if (uid != Process.SHELL_UID && uid != Process.ROOT_UID && uid != Process.myUid()) {
            throw new FileNotFoundException("caller is not the Nova Agent owner");
        }
    }

    @Override public String getType(Uri uri) {
        if (uri.getPathSegments().size() == 0) return "application/octet-stream";
        String operation = uri.getPathSegments().get(0);
        if ("icon".equals(operation)) return "image/jpeg";
        if ("labels".equals(operation) || "catalog".equals(operation)) return "application/json";
        return "text/plain";
    }
    @Override public Cursor query(Uri uri, String[] projection, String selection, String[] selectionArgs, String sortOrder) { return null; }
    @Override public Uri insert(Uri uri, ContentValues values) { throw new UnsupportedOperationException("read-only provider"); }
    @Override public int delete(Uri uri, String selection, String[] selectionArgs) { throw new UnsupportedOperationException("read-only provider"); }
    @Override public int update(Uri uri, ContentValues values, String selection, String[] selectionArgs) { throw new UnsupportedOperationException("read-only provider"); }
}
