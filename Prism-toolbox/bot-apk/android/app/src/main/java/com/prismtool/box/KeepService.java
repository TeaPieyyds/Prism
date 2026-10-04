package com.prismtool.box;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.net.Uri;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.util.Log;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.URL;

/**
 * 前台保活服务 — 单通知焦点通知（灵动岛风格）
 *
 * 一个通知、一个 HIGH 渠道。通知常驻显示连接状态 + 任务进度 + 进度条。
 * setOnlyAlertOnce(true) 确保后续更新不响铃不振动不弹出。
 * 状态变更时自然更新内容，不额外弹通知。
 */
public class KeepService extends Service {

    private static final String TAG = "PrismKeepService";

    private static final int NOTIF_ID = 1;
    private static final long POLL_MS = 800;
    private static final long POLL_FAIL_MS = 3000; // 请求失败时放慢轮询
    private static final String CHANNEL = "prism_focus";

    private static final int PUSH_NOTIF_ID = 2;
    private static final String PUSH_CHANNEL = "prism_push";
    private static final long PUSH_CHECK_MS = 5_000;
    private long lastPushCheck = 0;

    private Handler handler;
    private Runnable pollLoop;
    private NotificationManager nm;

    private String  lastConnState  = "";
    private boolean lastTaskRunning = false;
    private boolean lastTaskPaused  = false;

    private static KeepService instance;

    @Override
    public void onCreate() {
        super.onCreate();
        instance = this;
        nm = getSystemService(NotificationManager.class);

        NotificationChannel ch = new NotificationChannel(CHANNEL, "焦点通知",
                NotificationManager.IMPORTANCE_HIGH);
        ch.setShowBadge(true);
        ch.setDescription("连接状态、任务进度、进度条");
        ch.setSound(null, null);
        ch.enableVibration(false);
        ch.setLockscreenVisibility(Notification.VISIBILITY_PUBLIC);
        nm.createNotificationChannel(ch);

        NotificationChannel pushCh = new NotificationChannel(PUSH_CHANNEL, "推送消息",
                NotificationManager.IMPORTANCE_DEFAULT);
        pushCh.setDescription("来自管理后台的推送通知");
        pushCh.setSound(null, null);
        pushCh.enableVibration(true);
        nm.createNotificationChannel(pushCh);

        handler = new Handler(Looper.getMainLooper());
        pollLoop = this::pollAndUpdate;
        handler.post(pollLoop);

        startForeground(NOTIF_ID, buildNotif(false, false, ""));
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        return START_STICKY;
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    @Override
    public void onDestroy() {
        if (handler != null && pollLoop != null) handler.removeCallbacks(pollLoop);
        super.onDestroy();
    }

    // ═══════════════════════════════════════════
    //  轮询
    // ═══════════════════════════════════════════

    private void pollAndUpdate() {
        new Thread(() -> {
            boolean ok = false;
            try {
                String sj = httpGet("http://127.0.0.1:8080/api/bot/status");
                String tj = httpGet("http://127.0.0.1:8080/api/task/list");
                if (sj != null) {
                    handler.post(() -> buildAndNotify(sj, tj));
                    // 检查推送消息（每 30 秒一次）
                    checkPushMessages();
                    ok = true;
                } else {
                    Log.w(TAG, "poll: status API returned null");
                }
            } catch (Exception e) {
                Log.e(TAG, "poll: HTTP error", e);
            }
            // 请求失败时放慢轮询，避免空转耗电
            handler.postDelayed(pollLoop, ok ? POLL_MS : POLL_FAIL_MS);
        }).start();
    }

    private void buildAndNotify(String sj, String tj) {
        try {
            boolean connected = false, isOp = false;
            String server = "";
            if (sj != null) {
                org.json.JSONObject s = new org.json.JSONObject(sj);
                if (s.optBoolean("ok", false)) {
                    connected = s.optBoolean("connected", false);
                    isOp = s.optBoolean("is_op", false);
                    server = s.optString("server", "");
                }
            }

            // 断连清除任务
            String taskType = null, taskName = null, taskMsg = null;
            double taskProgress = -1;
            boolean taskPaused = false, taskRunning = false;

            if (connected && tj != null) {
                org.json.JSONObject t = new org.json.JSONObject(tj);
                if (t.optBoolean("ok", false)) {
                    org.json.JSONArray list = t.optJSONArray("tasks");
                    if (list != null) {
                        // 优先找 running 任务（有进度条的任务）
                        for (int i = 0; i < list.length(); i++) {
                            org.json.JSONObject task = list.getJSONObject(i);
                            String st = task.optString("status", "");
                            if ("running".equals(st)) {
                                taskType   = task.optString("type", "");
                                taskName   = task.optString("name", "");
                                taskMsg    = task.optString("message", "");
                                taskProgress = task.optDouble("progress", -1);
                                taskRunning = true;
                                taskPaused  = false;
                                break;
                            }
                        }
                        // 没有 running 任务才找 paused 任务
                        if (!taskRunning) {
                            for (int i = 0; i < list.length(); i++) {
                                org.json.JSONObject task = list.getJSONObject(i);
                                String st = task.optString("status", "");
                                if ("paused".equals(st)) {
                                    taskType   = task.optString("type", "");
                                    taskName   = task.optString("name", "");
                                    taskMsg    = task.optString("message", "");
                                    taskProgress = task.optDouble("progress", -1);
                                    taskPaused  = true;
                                    break;
                                }
                            }
                        }
                    }
                }
            }

            // 去重：状态没变化时不更新通知
            String curState = connected ? (isOp ? "op" : "connected") : "disconnected";
            boolean stateChanged = !curState.equals(lastConnState)
                    || lastTaskRunning != taskRunning
                    || lastTaskPaused != taskPaused;
            if (!stateChanged && !taskRunning && !taskPaused) {
                // 没有任务、状态没变 → 跳过更新
                return;
            }

            nm.notify(NOTIF_ID, buildNotif(connected, isOp, server,
                    taskType, taskName, taskMsg, taskProgress, taskPaused, taskRunning));

            lastConnState  = curState;
            lastTaskRunning = taskRunning;
            lastTaskPaused  = taskPaused;

        } catch (Exception e) {
            Log.e(TAG, "buildAndNotify: JSON parse error", e);
        }
    }

    // ═══════════════════════════════════════════
    //  推送通知
    // ═══════════════════════════════════════════

    private void checkPushMessages() {
        long now = System.currentTimeMillis();
        if (now - lastPushCheck < PUSH_CHECK_MS) return;
        lastPushCheck = now;

        new Thread(() -> {
            try {
                String pj = httpGet("http://127.0.0.1:8080/api/push/pending");
                if (pj != null) {
                    org.json.JSONObject p = new org.json.JSONObject(pj);
                    if (p.optBoolean("ok", false)) {
                        org.json.JSONArray msgs = p.optJSONArray("messages");
                        if (msgs != null && msgs.length() > 0) {
                            handler.post(() -> showPushNotifications(msgs));
                        }
                    }
                }
            } catch (Exception ignored) {}
        }).start();
    }

    private void showPushNotifications(org.json.JSONArray msgs) {
        try {
            StringBuilder ackIds = new StringBuilder("[");
            for (int i = 0; i < msgs.length(); i++) {
                org.json.JSONObject msg = msgs.getJSONObject(i);
                String id = msg.optString("id", "");
                String type = msg.optString("type", "admin_push");
                String title = msg.optString("title", "Prism 工具箱");
                String body = msg.optString("body", "");
                String action = msg.optString("action", "none");
                String actionData = msg.optString("action_data", "");

                // command 类型静默处理，不弹通知
                if ("command".equals(type)) {
                    handleSilentCommand(action, actionData);
                    ackIds.append("\"").append(id).append("\",");
                    continue;
                }

                Notification.Builder b = new Notification.Builder(this, PUSH_CHANNEL)
                        .setContentTitle(title)
                        .setContentText(body)
                        .setStyle(new Notification.BigTextStyle().bigText(body))
                        .setSmallIcon(android.R.drawable.ic_menu_info_details)
                        .setAutoCancel(true)
                        .setContentIntent(openPushIntent(action, actionData))
                        .setCategory(Notification.CATEGORY_RECOMMENDATION);

                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                    b.setColorized(true);
                }

                int notifId = PUSH_NOTIF_ID + Math.abs(id.hashCode() % 1000);
                nm.notify(notifId, b.build());

                ackIds.append("\"").append(id).append("\",");
            }
            // 替换末尾逗号
            int len = ackIds.length();
            if (len > 1 && ackIds.charAt(len - 1) == ',') {
                ackIds.setCharAt(len - 1, ']');
            } else {
                ackIds.append("]");
            }

            // 批量标记已读
            final String body = "{\"ids\":" + ackIds.toString() + "}";
            new Thread(() -> {
                try { httpPost("http://127.0.0.1:8080/api/push/ack", body); } catch (Exception ignored) {}
            }).start();
        } catch (Exception e) {
            Log.e(TAG, "showPushNotifications error", e);
        }
    }

    private PendingIntent openPushIntent(String action, String actionData) {
        Intent i;
        switch (action) {
            case "open_url":
                if (actionData != null && !actionData.isEmpty()) {
                    i = new Intent(Intent.ACTION_VIEW, Uri.parse(actionData));
                    i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
                    return PendingIntent.getActivity(this, 0, i,
                            PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
                }
                break;
        }
        // 默认：打开工具箱
        i = new Intent(this, MainActivity.class);
        i.setFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        return PendingIntent.getActivity(this, 0, i,
                PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
    }

    private void handleSilentCommand(String action, String actionData) {
        switch (action) {
            case "refresh_config":
                new Thread(() -> {
                    try { httpGet("http://127.0.0.1:8080/api/config"); } catch (Exception ignored) {}
                }).start();
                break;
        }
    }

    // ═══════════════════════════════════════════
    //  主通知
    // ═══════════════════════════════════════════

    private Notification buildNotif(boolean connected, boolean isOp, String server) {
        return buildNotif(connected, isOp, server, null, null, null, -1, false, false);
    }

    private Notification buildNotif(boolean connected, boolean isOp, String server,
                                    String taskType, String taskName, String taskMsg,
                                    double taskProgress, boolean taskPaused,
                                    boolean taskRunning) {
        String title;
        int color;
        if (!connected) {
            title = "○ 未连接";
            color = 0xFF94A3B8;
        } else if (isOp) {
            title = "● 已连接 (OP)";
            color = 0xFF4ADE80;
        } else {
            title = "● 已连接";
            color = 0xFFFBBF24;
        }

        // 内容行（单行）
        String content;
        String expanded;
        if (taskRunning || taskPaused) {
            String label = typeLabel(taskType);
            int pct = taskProgress >= 0 ? (int) Math.round(taskProgress * 100) : -1;
            String pctStr = pct >= 0 ? pct + "%" : "…";
            content = (taskPaused ? "⏸ " : "") + label + "  " + pctStr;

            StringBuilder bd = new StringBuilder();
            bd.append(title);
            if (server != null && !server.isEmpty()) bd.append("  ·  ").append(server);
            bd.append("\n").append(label);
            if (taskName != null && !taskName.isEmpty()) bd.append("  ").append(taskName);
            bd.append("\n");
            if (taskMsg != null && !taskMsg.isEmpty()) bd.append(taskMsg).append("\n");
            bd.append("进度 ").append(pctStr);
            if (taskPaused) bd.append("  已暂停");
            expanded = bd.toString();
        } else {
            content = server != null && !server.isEmpty() ? server : "等待连接…";
            expanded = title + "\n当前无运行中的任务";
        }

        Notification.Builder b = new Notification.Builder(this, CHANNEL)
                .setContentTitle(title)
                .setContentText(content)
                .setSmallIcon(android.R.drawable.ic_menu_manage)
                .setOngoing(true)
                .setOnlyAlertOnce(true)
                .setContentIntent(openIntent())
                .setColor(color)
                .setCategory(Notification.CATEGORY_SERVICE)
                .setStyle(new Notification.BigTextStyle().bigText(expanded));

        // 进度条
        if (taskRunning) {
            int pct = taskProgress >= 0 ? (int) Math.round(taskProgress * 100) : 0;
            b.setProgress(100, pct, taskProgress < 0);
            b.addAction(android.R.drawable.ic_menu_close_clear_cancel,
                    "停止任务", stopTaskPendingIntent());
        }

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            b.setColorized(true);
        }

        return b.build();
    }

    // ═══════════════════════════════════════════
    //  静态 — 供 JSBridge 调用
    // ═══════════════════════════════════════════

    public static void postFocusNotification(String title, String message) {
        KeepService svc = instance;
        if (svc == null) return;
        int color = 0xFF60A5FA;
        if (message != null && (message.contains("失败") || message.contains("崩溃") || message.contains("error"))) {
            color = 0xFFF87171;
        } else if (title != null && (title.contains("完成") || title.contains("成功"))) {
            color = 0xFF4ADE80;
        } else if (title != null && (title.contains("更新") || title.contains("版本"))) {
            color = 0xFFFBBF24;
        }

        Notification.Builder b = new Notification.Builder(svc, CHANNEL)
                .setContentTitle(title != null ? title : "prism工具箱")
                .setContentText(message != null ? message : "")
                .setStyle(new Notification.BigTextStyle().bigText(message != null ? message : ""))
                .setSmallIcon(android.R.drawable.ic_menu_manage)
                .setOngoing(true)
                .setOnlyAlertOnce(true)
                .setContentIntent(svc.openIntent())
                .setColor(color)
                .setCategory(Notification.CATEGORY_SERVICE);

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            b.setColorized(true);
        }
        b.addAction(android.R.drawable.ic_menu_view, "查看", svc.openIntent());

        svc.nm.notify(NOTIF_ID, b.build());
    }

    // ═══════════════════════════════════════════
    //  辅助
    // ═══════════════════════════════════════════

    private PendingIntent openIntent() {
        Intent i = new Intent(this, MainActivity.class);
        i.setFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        return PendingIntent.getActivity(this, 0, i,
                PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
    }

    private PendingIntent stopTaskPendingIntent() {
        Intent i = new Intent(this, NotificationActionReceiver.class);
        i.setAction("stop_task");
        return PendingIntent.getBroadcast(this, 1, i,
                PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
    }

    private String typeLabel(String t) {
        if (t == null) return "";
        switch (t) {
            case "import": return "导入";
            case "export": return "导出";
            case "mapart": return "地图画";
            case "skin":   return "皮肤雕像";
            default: return t;
        }
    }

    private String httpGet(String urlStr) throws Exception {
        HttpURLConnection c = (HttpURLConnection) new URL(urlStr).openConnection();
        c.setConnectTimeout(1500);
        c.setReadTimeout(1500);
        if (c.getResponseCode() != 200) return null;
        BufferedReader r = new BufferedReader(new InputStreamReader(c.getInputStream()));
        StringBuilder sb = new StringBuilder();
        String line;
        while ((line = r.readLine()) != null) sb.append(line);
        r.close();
        return sb.toString();
    }

    private void httpPost(String urlStr, String body) throws Exception {
        HttpURLConnection c = (HttpURLConnection) new URL(urlStr).openConnection();
        c.setRequestMethod("POST");
        c.setRequestProperty("Content-Type", "application/json");
        c.setConnectTimeout(1500);
        c.setReadTimeout(1500);
        c.setDoOutput(true);
        c.getOutputStream().write(body.getBytes("UTF-8"));
        c.getOutputStream().flush();
        c.getOutputStream().close();
        // 不关心响应，读到 200 就够
        int code = c.getResponseCode();
        c.disconnect();
    }
}
