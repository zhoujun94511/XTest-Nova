package com.openatx.xtest.nova.runner;

import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.*;
import java.util.concurrent.*;
import java.util.regex.*;

final class Config {
    private static final Pattern PACKAGE = Pattern.compile("^[A-Za-z][A-Za-z0-9_]*(\\.[A-Za-z][A-Za-z0-9_]*)+$");
    String packageName, requestId = "", stopFile = "/data/local/tmp/.xtest-nova-monkey.stop", activityMode = "none", renderFallbackMode = "bounded", targetToken = "", controlToken = "";
    int durationSeconds = 600, throttleMillis = 500;
    int minBattery = 15;
    int inputCasesPerField = NodeExplorer.DEFAULT_INPUT_CASES_PER_FIELD, inputMaxLength = NodeExplorer.DEFAULT_INPUT_MAX_LENGTH;
    boolean lowBatteryExit;
    final List<String> activities = new ArrayList<>(), targetActivities = new ArrayList<>(), blockedControls = new ArrayList<>(), targetCases = new ArrayList<>();
    long seed = System.currentTimeMillis();
    static Config parse(String[] args) {
        Config c=new Config();
        for(int i=0;i<args.length;i++){String arg=args[i];if("-p".equals(arg))c.packageName=next(args,++i,arg);else if("--request-id".equals(arg))c.requestId=next(args,++i,arg);else if("--duration-seconds".equals(arg))c.durationSeconds=Integer.parseInt(next(args,++i,arg));else if("--throttle".equals(arg))c.throttleMillis=Integer.parseInt(next(args,++i,arg));else if("--seed".equals(arg))c.seed=Long.parseLong(next(args,++i,arg));else if("--stop-file".equals(arg))c.stopFile=next(args,++i,arg);else if("--input-cases-per-field".equals(arg))c.inputCasesPerField=Integer.parseInt(next(args,++i,arg));else if("--input-max-length".equals(arg))c.inputMaxLength=Integer.parseInt(next(args,++i,arg));else if("--low-battery-exit".equals(arg))c.lowBatteryExit=true;else if("--min-battery".equals(arg))c.minBattery=Integer.parseInt(next(args,++i,arg));else if("--activity-mode".equals(arg))c.activityMode=next(args,++i,arg);else if("--render-fallback-mode".equals(arg))c.renderFallbackMode=next(args,++i,arg);else if("--activity".equals(arg))c.activities.add(next(args,++i,arg));else if("--target-activity".equals(arg))c.targetActivities.add(next(args,++i,arg));else if("--blocked-control".equals(arg))c.blockedControls.add(next(args,++i,arg));else if("--target-token".equals(arg))c.targetToken=next(args,++i,arg);else if("--target-case".equals(arg))c.targetCases.add(next(args,++i,arg));else if("--control-token".equals(arg))c.controlToken=next(args,++i,arg);}
        if(c.packageName==null||!PACKAGE.matcher(c.packageName).matches())throw new IllegalArgumentException("Invalid package");
        if(c.durationSeconds<1||c.durationSeconds>604800)throw new IllegalArgumentException("Invalid duration");
        if(c.throttleMillis<50||c.throttleMillis>60000)throw new IllegalArgumentException("Invalid throttle");
        if(c.inputCasesPerField<1||c.inputCasesPerField>NodeExplorer.MAX_INPUT_CASES_PER_FIELD)throw new IllegalArgumentException("Invalid input cases per field");
        if(c.inputMaxLength<1||c.inputMaxLength>256)throw new IllegalArgumentException("Invalid input max length");
        if(c.lowBatteryExit&&(c.minBattery<1||c.minBattery>100))throw new IllegalArgumentException("Invalid battery threshold");
        if(!Arrays.asList("none","allowlist","blocklist").contains(c.activityMode))throw new IllegalArgumentException("Invalid activity mode");
        CoordinateFallbackController.Mode.parse(c.renderFallbackMode);
        if(!c.targetCases.isEmpty()&&c.targetToken.isEmpty())throw new IllegalArgumentException("Missing target-case token");
        return c;
    }
    static String next(String[] args,int i,String name){if(i>=args.length)throw new IllegalArgumentException("Missing "+name);return args[i];}
}
