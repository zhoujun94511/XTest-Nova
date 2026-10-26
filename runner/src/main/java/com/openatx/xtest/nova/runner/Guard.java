package com.openatx.xtest.nova.runner;

import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.*;
import java.util.concurrent.*;
import java.util.regex.*;

final class Guard {
    private static final Pattern ACTIVITY = Pattern.compile("(?:topResumedActivity|ResumedActivity).*? ([A-Za-z0-9_.$]+/[A-Za-z0-9_.$]+)");
    private static final Pattern LEVEL = Pattern.compile("(?m)^\\s*level:\\s*(\\d+)");
    final Config config; final ShellInput input; final Set<String> reachedTargets = new HashSet<>(), completedCases = new HashSet<>(), observedActivities = new HashSet<>();
    long nextActivity, nextBattery; String activity;
    Guard(Config config, ShellInput input){this.config=config;this.input=input;}
    String check() throws Exception {
        long now=System.currentTimeMillis();
        if(config.lowBatteryExit&&now>=nextBattery){nextBattery=now+30000;Matcher m=LEVEL.matcher(input.output("dumpsys","battery"));if(m.find()&&Integer.parseInt(m.group(1))<=config.minBattery)return "stop:low_battery";}
        if(now>=nextActivity){nextActivity=now+1000;Matcher m=ACTIVITY.matcher(input.output("dumpsys","activity","activities"));activity=m.find()?m.group(1):"";if(!activity.isEmpty()&&observedActivities.add(activity))emit("activity",activity,"");for(String target:config.targetActivities)if(matches(target,activity)&&reachedTargets.add(target))emit("target_page",activity,"");for(String mapping:config.targetCases){String[] parts=mapping.split("\\|",-1);if(parts.length==3&&matches(parts[0],activity)&&completedCases.add(mapping)){if(runTargetCase(activity,parts[1],parts[2]))emit("target_case_completed",activity,",\"task\":\""+escape(parts[1])+"\",\"case\":\""+escape(parts[2])+"\"");}}}
        boolean listed=false;for(String value:config.activities)if(matches(value,activity)){listed=true;break;}
        if(("allowlist".equals(config.activityMode)&&!listed)||("blocklist".equals(config.activityMode)&&listed)){input.key("KEYCODE_BACK");emit("guarded",activity,"");return "skip";}
        return null;
    }
    static String currentActivity(ShellInput input) throws Exception {Matcher matcher=ACTIVITY.matcher(input.output("dumpsys","activity","activities"));return matcher.find()?matcher.group(1):"";}
    boolean runTargetCase(String currentActivity,String task,String caseName){try{String body="{\"token\":\""+escape(config.targetToken)+"\",\"activity\":\""+escape(currentActivity)+"\",\"task\":\""+escape(task)+"\",\"case\":\""+escape(caseName)+"\"}";post("http://127.0.0.1:7912/v1/monkey/target-case",body);return true;}catch(Exception error){emit("target_case_failed",currentActivity,",\"error\":\""+escape(String.valueOf(error.getMessage()))+"\"");return false;}}
    void emit(String state,String currentActivity,String extra){System.out.println("{\"state\":\""+state+"\",\"requestId\":\""+escape(config.requestId)+"\",\"package\":\""+config.packageName+"\",\"activity\":\""+escape(currentActivity)+"\""+extra+",\"time\":"+System.currentTimeMillis()+"}");}
    static boolean matches(String expected,String actual){return expected.equals(actual)||(!expected.contains("/")&&actual.endsWith("/"+expected));}
    static String http(String address)throws IOException{HttpURLConnection connection=(HttpURLConnection)new URL(address).openConnection();connection.setConnectTimeout(2000);connection.setReadTimeout(15000);try{if(connection.getResponseCode()/100!=2)throw new IOException("Hierarchy HTTP failed");StringBuilder value=new StringBuilder();try(Reader reader=new InputStreamReader(connection.getInputStream(),"UTF-8")){char[] buffer=new char[4096];for(int count;(count=reader.read(buffer))>=0;){if(value.length()+count>4*1024*1024)throw new IOException("Hierarchy too large");value.append(buffer,0,count);}}return value.toString();}finally{connection.disconnect();}}
    static String post(String address,String body)throws IOException{HttpURLConnection connection=(HttpURLConnection)new URL(address).openConnection();connection.setConnectTimeout(1000);connection.setReadTimeout(300000);connection.setRequestMethod("POST");connection.setDoOutput(true);connection.setRequestProperty("Content-Type","application/json");try(OutputStream output=connection.getOutputStream()){output.write(body.getBytes("UTF-8"));}try{int status=connection.getResponseCode();InputStream stream=status<400?connection.getInputStream():connection.getErrorStream();StringBuilder value=new StringBuilder();if(stream!=null)try(Reader reader=new InputStreamReader(stream,"UTF-8")){char[] buffer=new char[1024];for(int count;(count=reader.read(buffer))>=0;)value.append(buffer,0,count);}if(status/100!=2)throw new IOException("Target case HTTP "+status+": "+value);return value.toString();}finally{connection.disconnect();}}
    static String escape(String value){return value.replace("\\","\\\\").replace("\"","\\\"");}
}
