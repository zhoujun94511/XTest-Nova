package com.xtest.nova.fixture;

import android.app.Activity;
import android.opengl.GLES20;
import android.opengl.GLSurfaceView;
import android.os.Bundle;
import android.view.Surface;
import android.view.WindowManager;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.FloatBuffer;
import javax.microedition.khronos.egl.EGLConfig;
import javax.microedition.khronos.opengles.GL10;

public final class GLESLoopActivity extends Activity {
    private GLSurfaceView surface;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        WindowManager.LayoutParams attributes = getWindow().getAttributes();
        attributes.preferredRefreshRate = 120.0f;
        getWindow().setAttributes(attributes);
        surface = new GLSurfaceView(this);
        surface.setEGLContextClientVersion(2);
        surface.setRenderer(new LoadRenderer());
        surface.setRenderMode(GLSurfaceView.RENDERMODE_CONTINUOUSLY);
        surface.setContentDescription("OpenGL ES GPU游戏循环夹具");
        setContentView(surface);
    }

    @Override public void onWindowFocusChanged(boolean focused) {
        super.onWindowFocusChanged(focused);
        if(focused && android.os.Build.VERSION.SDK_INT >= 30) {
            Surface target = surface.getHolder().getSurface();
            if(target.isValid()) target.setFrameRate(120.0f, Surface.FRAME_RATE_COMPATIBILITY_DEFAULT);
        }
    }

    @Override protected void onPause() { surface.onPause(); super.onPause(); }
    @Override protected void onResume() { super.onResume(); surface.onResume(); }

    private static final class LoadRenderer implements GLSurfaceView.Renderer {
        private final FloatBuffer vertices;
        private int program;
        private int position;
        private int color;

        LoadRenderer() {
            float[] data = {-0.95f,-0.9f,0.95f,-0.9f,0.0f,0.95f};
            vertices = ByteBuffer.allocateDirect(data.length * 4).order(ByteOrder.nativeOrder()).asFloatBuffer();
            vertices.put(data).position(0);
        }

        @Override public void onSurfaceCreated(GL10 unused, EGLConfig config) {
            String vertexSource = "attribute vec2 aPosition; void main(){ gl_Position=vec4(aPosition,0.0,1.0); }";
            String fragmentSource = "precision mediump float; uniform vec4 uColor; void main(){ gl_FragColor=uColor; }";
            program = GLES20.glCreateProgram();
            GLES20.glAttachShader(program, compile(GLES20.GL_VERTEX_SHADER, vertexSource));
            GLES20.glAttachShader(program, compile(GLES20.GL_FRAGMENT_SHADER, fragmentSource));
            GLES20.glLinkProgram(program);
            position = GLES20.glGetAttribLocation(program, "aPosition");
            color = GLES20.glGetUniformLocation(program, "uColor");
            GLES20.glClearColor(0.02f, 0.03f, 0.05f, 1.0f);
        }

        @Override public void onSurfaceChanged(GL10 unused, int width, int height) {
            GLES20.glViewport(0, 0, width, height);
        }

        @Override public void onDrawFrame(GL10 unused) {
            GLES20.glClear(GLES20.GL_COLOR_BUFFER_BIT);
            GLES20.glUseProgram(program);
            GLES20.glEnableVertexAttribArray(position);
            GLES20.glVertexAttribPointer(position, 2, GLES20.GL_FLOAT, false, 0, vertices);
            for(int index=0; index<240; index++) {
                float phase=(index % 32) / 31.0f;
                GLES20.glUniform4f(color, 0.1f + phase * 0.7f, 0.8f - phase * 0.5f, 0.45f, 0.014f);
                GLES20.glDrawArrays(GLES20.GL_TRIANGLES, 0, 3);
            }
            GLES20.glDisableVertexAttribArray(position);
        }

        private static int compile(int type, String source) {
            int shader=GLES20.glCreateShader(type);
            GLES20.glShaderSource(shader, source);
            GLES20.glCompileShader(shader);
            return shader;
        }
    }
}
