package com.xtest.nova.fixture;

import android.app.Activity;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.Paint;
import android.os.Bundle;
import android.view.Surface;
import android.view.SurfaceHolder;
import android.view.SurfaceView;

public final class SurfaceLoopActivity extends Activity {
    private AnimatedSurface surface;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        surface = new AnimatedSurface();
        surface.setContentDescription("120Hz Surface游戏循环夹具");
        setContentView(surface);
    }

    @Override protected void onDestroy() {
        surface.stop();
        super.onDestroy();
    }

    private final class AnimatedSurface extends SurfaceView implements SurfaceHolder.Callback, Runnable {
        private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
        private volatile boolean running;
        private Thread thread;
        private int frame;

        AnimatedSurface() {
            super(SurfaceLoopActivity.this);
            getHolder().addCallback(this);
            setKeepScreenOn(true);
        }

        @Override public void surfaceCreated(SurfaceHolder holder) {
            if (android.os.Build.VERSION.SDK_INT >= 30) {
                holder.getSurface().setFrameRate(120.0f, Surface.FRAME_RATE_COMPATIBILITY_DEFAULT);
            }
            running = true;
            thread = new Thread(this, "xtest-surface-loop");
            thread.start();
        }

        @Override public void surfaceChanged(SurfaceHolder holder, int format, int width, int height) {}

        @Override public void surfaceDestroyed(SurfaceHolder holder) { stop(); }

        void stop() {
            running = false;
            if (thread != null) {
                thread.interrupt();
                try {
                    thread.join(1000);
                } catch (InterruptedException error) {
                    Thread.currentThread().interrupt();
                }
                thread = null;
            }
        }

        @Override public void run() {
            while (running) {
                Canvas canvas = null;
                try {
                    canvas = getHolder().lockCanvas();
                    if (canvas != null) {
                        canvas.drawColor(Color.rgb(12, 18, 28));
                        float x = (frame * 17) % Math.max(1, canvas.getWidth());
                        paint.setColor(Color.rgb(30, 200, 150));
                        canvas.drawCircle(x, canvas.getHeight() / 2.0f, 48, paint);
                        paint.setColor(Color.WHITE);
                        paint.setTextSize(42);
                        canvas.drawText("Surface frame " + frame, 32, 72, paint);
                        frame++;
                    }
                } finally {
                    if (canvas != null) getHolder().unlockCanvasAndPost(canvas);
                }
                try {
                    Thread.sleep(8);
                } catch (InterruptedException error) {
                    Thread.currentThread().interrupt();
                    return;
                }
            }
        }
    }
}
