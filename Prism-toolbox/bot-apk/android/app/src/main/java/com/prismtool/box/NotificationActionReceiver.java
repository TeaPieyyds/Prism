package com.prismtool.box;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.widget.Toast;

import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;

/**
 * 处理通知栏按钮点击（停止任务等）。
 * 通过 HTTP 调用 Go 后端的 API 执行操作。
 */
public class NotificationActionReceiver extends BroadcastReceiver {

    @Override
    public void onReceive(Context context, Intent intent) {
        String action = intent.getAction();
        if ("stop_task".equals(action)) {
            Toast.makeText(context, "正在停止任务…", Toast.LENGTH_SHORT).show();
            new Thread(() -> {
                try {
                    HttpURLConnection c = (HttpURLConnection)
                            new URL("http://127.0.0.1:8080/api/task/stop").openConnection();
                    c.setRequestMethod("POST");
                    c.setDoOutput(true);
                    c.setConnectTimeout(2000);
                    c.setReadTimeout(2000);
                    try (OutputStream os = c.getOutputStream()) {
                        os.write("{}".getBytes());
                    }
                    c.getResponseCode();
                } catch (Exception ignored) {}
            }).start();
        }
    }
}
