package com.openatx.xtest.nova.runner;

import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.*;
import java.util.concurrent.*;
import java.util.regex.*;

final class ShellInput {
		static void selfTest()throws Exception{
			String javaName=System.getProperty("os.name","").startsWith("Windows")?"java.exe":"java";
			String javaPath=new File(new File(System.getProperty("java.home"),"bin"),javaName).getAbsolutePath();
			long started=System.nanoTime();
			try{
				new ShellInput().output(250,javaPath,"-cp",System.getProperty("java.class.path"),Main.class.getName(),"--self-test-sleep");
				throw new AssertionError("ShellInput timeout was not enforced");
			}catch(IOException expected){
				if(!String.valueOf(expected.getMessage()).contains("timed out"))throw expected;
			}
			if(TimeUnit.NANOSECONDS.toSeconds(System.nanoTime()-started)>=5)throw new AssertionError("ShellInput timeout blocked too long");
			System.out.println("ShellInput timeout self-test passed");
		}
    void tap(int x,int y)throws Exception{run("input","tap",String.valueOf(x),String.valueOf(y));}
    void wakeAndDismissKeyguard()throws Exception{
        // Keep this local to the active exploration session. Changing Android's global
        // stay-awake setting would leak test state into later runs.
        run("input","keyevent","KEYCODE_WAKEUP");
        run("wm","dismiss-keyguard");
        Thread.sleep(250L);
    }
    void longPress(int x,int y)throws Exception{run("input","swipe",String.valueOf(x),String.valueOf(y),String.valueOf(x),String.valueOf(y),"800");}
    void key(String code)throws Exception{run("input","keyevent",code);}
    void keyCombination(String... codes)throws Exception{String[] command=new String[codes.length+2];command[0]="input";command[1]="keycombination";System.arraycopy(codes,0,command,2,codes.length);run(command);}
    void text(String token,String value)throws Exception{if(token==null||token.isEmpty())throw new IOException("Missing Runner control token");Guard.post("http://127.0.0.1:7912/v1/monkey/input-text","{\"token\":\""+Guard.escape(token)+"\",\"text\":\""+Guard.escape(value)+"\"}");}
    void swipe(int x1,int y1,int x2,int y2)throws Exception{run("input","swipe",String.valueOf(x1),String.valueOf(y1),String.valueOf(x2),String.valueOf(y2),"250");}
    SafeTouchRegion safeTouchRegion(){int width=1080,height=1920,density=160;try{String value=output("wm","size");Matcher m=Pattern.compile("(\\d+)x(\\d+)").matcher(value);if(m.find()){width=Integer.parseInt(m.group(1));height=Integer.parseInt(m.group(2));}}catch(Exception ignored){}try{Matcher m=Pattern.compile("(?:Override|Physical) density:\\s*(\\d+)").matcher(output("wm","density"));while(m.find())density=Integer.parseInt(m.group(1));}catch(Exception ignored){}return new SafeTouchRegion(width,height,density);}
    void run(String... command)throws Exception{output(command);}
    String output(String... command)throws Exception{return output(10000,command);}
    String output(long timeoutMillis,String... command)throws Exception{
        final Process process=new ProcessBuilder(command).redirectErrorStream(true).start();
        FutureTask<String> drain=new FutureTask<>(() -> {
            ByteArrayOutputStream out=new ByteArrayOutputStream();
            byte[] buffer=new byte[1024];
            try(InputStream input=process.getInputStream()){
                for(int count;(count=input.read(buffer))>=0;){
                    if(out.size()+count>4*1024*1024){process.destroyForcibly();throw new IOException("Command output too large");}
                    out.write(buffer,0,count);
                }
            }
            return out.toString("UTF-8");
        });
        Thread reader=new Thread(drain,"xtest-nova-command-output");
        reader.setDaemon(true);
        reader.start();
        if(!process.waitFor(timeoutMillis,TimeUnit.MILLISECONDS)){
            process.destroyForcibly();
            try{process.getInputStream().close();}catch(IOException ignored){}
            drain.cancel(true);
            throw new IOException("Command timed out");
        }
        String output;
        try{output=drain.get(2,TimeUnit.SECONDS);}
        catch(ExecutionException error){
            Throwable cause=error.getCause();
            if(cause instanceof Exception)throw (Exception)cause;
            throw error;
        }
        catch(TimeoutException error){
            drain.cancel(true);
            throw new IOException("Command output did not close",error);
        }
        if(process.exitValue()!=0)throw new IOException("Command failed: "+Arrays.toString(command));
        return output;
    }
}
