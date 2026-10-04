package com.prismtool.box;

import android.Manifest;
import android.app.Activity;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Environment;
import android.os.Handler;
import android.os.Looper;
import android.provider.Settings;
import android.database.Cursor;
import android.provider.OpenableColumns;
import android.webkit.JavascriptInterface;
import android.webkit.WebChromeClient;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.view.View;
import android.widget.Toast;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.ByteArrayOutputStream;
import java.util.Base64;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.Signature;

public class MainActivity extends Activity {

    private WebView webView;
    private Handler handler = new Handler(Looper.getMainLooper());
    private static final int FILE_PICK_CODE = 1001;
    private static final int STORAGE_PERM_CODE = 1002;
    private String lastPickedPath = "";

    static {
        System.loadLibrary("prism");
    }

    private static native void setDataDir(String dir);
    private static native void setSignatureHash(String hash);
    private static native void setPackageName(String pkg);
    private static native void setApkPath(String path);
    private static native void GoMain();

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        webView = new WebView(this);
        WebSettings s = webView.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setAllowFileAccess(true);
        s.setAllowContentAccess(true);
        s.setDatabaseEnabled(true);
        // Reduce GPU usage: no need for smooth scrolling or animated zoom on a tool app
        s.setBuiltInZoomControls(false);
        s.setDisplayZoomControls(false);
        s.setLoadWithOverviewMode(true);
        s.setUseWideViewPort(true);
        // Use software layer initially to avoid GPU compositing
        webView.setLayerType(View.LAYER_TYPE_HARDWARE, null);
        webView.setWebChromeClient(new WebChromeClient());
        webView.addJavascriptInterface(new JSBridge(), "android");

        webView.setWebViewClient(new WebViewClient() {
            @Override
            public void onReceivedError(android.webkit.WebView v, android.webkit.WebResourceRequest req, android.webkit.WebResourceError err) {
                // Will be handled by the retry page's JS
            }
            @Override
            public void onPageFinished(WebView view, String url) {
                // Check if we loaded the actual app (not the loading page)
                if (url.startsWith("http://127.0.0.1:8080")) {
                    // App UI is loaded
                }
            }
        });

        setContentView(webView);

        // 全屏绘制，透出 WebView 背景色
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            getWindow().setDecorFitsSystemWindows(false);
        } else {
            getWindow().setFlags(
                    android.view.WindowManager.LayoutParams.FLAG_LAYOUT_NO_LIMITS,
                    android.view.WindowManager.LayoutParams.FLAG_LAYOUT_NO_LIMITS
            );
        }
        getWindow().setStatusBarColor(android.graphics.Color.TRANSPARENT);

        // Background service to keep alive
        startService(new Intent(this, KeepService.class));

        // Auto-request permissions on first run
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            if (checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS)
                    != PackageManager.PERMISSION_GRANTED) {
                requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, 1003);
            }
        }
        // Verify APK signature before starting
        String sigHash = getApkSignatureHash();
        setSignatureHash(sigHash != null ? sigHash : "");
        String pkgName = getPackageName();
        setPackageName(pkgName != null ? pkgName : "");
        // 传 APK 路径给 Go，Go 直接读签名证书校验（不信任 Java 上报值）
        try {
            setApkPath(getPackageCodePath());
        } catch (Exception ignored) {}

        // Write device info to file for Go backend
        try {
            String info = android.os.Build.BRAND + " " + android.os.Build.MODEL + " Android " + android.os.Build.VERSION.RELEASE;
            java.io.FileWriter fw = new java.io.FileWriter(new java.io.File(getFilesDir(), "device_model"));
            fw.write(info);
            fw.close();
        } catch (Exception ignored) {}

        // Start Go backend with data directory
        String dataDir = getFilesDir().getAbsolutePath();
        new Thread(() -> {
            setDataDir(dataDir);
            GoMain();
        }).start();

        // Show loading page that retries until Go server is ready
        // 简化版：Logo 淡入 → 标题打字机 → 副标题淡入，服务就绪后立即跳转
        String retryPage = "<!DOCTYPE html><html lang='zh-CN'><head><meta charset='UTF-8'><meta name='viewport' content='width=device-width,initial-scale=1,maximum-scale=1,user-scalable=no'>" +
            "<style>*{margin:0;padding:0;box-sizing:border-box}" +
            "body{font-family:'Nunito','Noto Sans SC','PingFang SC','system-ui',sans-serif;background:#0f1720;color:#e2e8f0;display:flex;align-items:center;justify-content:center;height:100vh;flex-direction:column;overflow:hidden}" +
            ".wrap{text-align:center}" +
            ".logo{margin-bottom:16px;opacity:0;animation:fadeIn .6s ease-out forwards}" +
            ".logo img{width:128px;height:128px;display:block;margin:0 auto}" +
            "#title{font-size:22px;font-weight:900;letter-spacing:-.3px;color:#e2e8f0;height:30px;line-height:30px;margin-bottom:4px}" +
            "#title::after{content:'|';animation:blink .7s step-end infinite;margin-left:2px;color:#d48a0e}" +
            "@keyframes blink{0%,100%{opacity:1}50%{opacity:0}}" +
            ".sub{font-size:10px;color:#475569;opacity:0}" +
            ".sub.show{opacity:1;transition:opacity .5s ease}" +
            "@keyframes fadeIn{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:translateY(0)}}" +
            "</style></head><body><div class='wrap'>" +
            "<div class='logo'><img src='logo.png' alt='logo' style='width:128px;height:128px'></div>" +
            "<div id='title'></div>" +
            "<p class='sub' id='sub'>developed by adb_lanlu</p>" +
            "</div>" +
            "<script>" +
            "var txt='Prism 工具箱';var i=0;var el=document.getElementById('title');" +
            "function type(){" +
            "  if(i<txt.length){el.textContent+=txt[i];i++;setTimeout(type,80)}" +
            "  else{document.getElementById('sub').classList.add('show')}" +
            "}" +
            "setTimeout(type,80);" +
            "var n=0;var startTime=Date.now();" +
            "function tryLoad(){" +
            "  n++;var x=new XMLHttpRequest();x.timeout=2000;" +
            "  x.onload=function(){if(x.status===200||x.status===404){" +
            "    var elapsed=Date.now()-startTime;" +
            "    var minShow=2500;" +
            "    if(elapsed<minShow){setTimeout(function(){location.href='http://127.0.0.1:8080/'},minShow-elapsed)}" +
            "    else{location.href='http://127.0.0.1:8080/'}" +
            "  }};" +
            "  x.onerror=function(){setTimeout(tryLoad,Math.min(n*300,2000))};" +
            "  x.ontimeout=function(){setTimeout(tryLoad,300)};" +
            "  x.open('GET','http://127.0.0.1:8080/',true);x.send()}" +
            "setTimeout(tryLoad,80);" +
            "</script></body></html>";
        webView.loadDataWithBaseURL("file:///android_asset/", retryPage, "text/html", "UTF-8", null);
    }

    public class JSBridge {
        @JavascriptInterface
        public void pickFile(String type) {
            Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
            intent.addCategory(Intent.CATEGORY_OPENABLE);
            if ("building".equals(type)) {
                // .mcstructure .schematic .bdx .mcworld
                intent.setType("*/*");
                intent.putExtra(Intent.EXTRA_MIME_TYPES, new String[]{
                    "application/octet-stream", "application/x-nbt", "*/*"
                });
            } else if ("image".equals(type)) {
                intent.setType("image/*");
            } else if ("media".equals(type)) {
                intent.setType("*/*");
                intent.putExtra(Intent.EXTRA_MIME_TYPES, new String[]{
                    "application/octet-stream", "audio/*", "*/*"
                });
            } else {
                intent.setType("*/*");
            }
            startActivityForResult(intent, FILE_PICK_CODE);
        }

        @JavascriptInterface
        public void toast(String msg) {
            handler.post(() -> Toast.makeText(MainActivity.this, msg, Toast.LENGTH_SHORT).show());
        }

        @JavascriptInterface
        public void notifyFocus(String title, String msg) {
            KeepService.postFocusNotification(title, msg != null ? msg : "");
        }

        @JavascriptInterface
        public int getStatusBarHeight() {
            int resId = getResources().getIdentifier("status_bar_height", "dimen", "android");
            if (resId > 0) return getResources().getDimensionPixelSize(resId);
            // 兜底：按屏幕密度估算 25dp
            return (int) (25 * getResources().getDisplayMetrics().density);
        }

        @JavascriptInterface
        public void setLightStatusBar(boolean light) {
            handler.post(() -> {
                int flags = getWindow().getDecorView().getSystemUiVisibility();
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
                    if (light) {
                        flags &= ~View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR;
                    } else {
                        flags |= View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR;
                    }
                    getWindow().getDecorView().setSystemUiVisibility(flags);
                }
            });
        }

        @JavascriptInterface
        public void setStatusBarColor(String color) {
            handler.post(() -> {
                try {
                    int c = android.graphics.Color.parseColor(color);
                    getWindow().setStatusBarColor(c);
                } catch (Exception ignored) {}
            });
        }

        @JavascriptInterface
        public void openExternal(String url) {
            Intent intent = new Intent(Intent.ACTION_VIEW, Uri.parse(url));
            intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            startActivity(intent);
        }

        @JavascriptInterface
        public String getLastPickedPath() {
            return lastPickedPath;
        }

        @JavascriptInterface
        public void setEnv(String key, String val) {
            // Pass data dir to JS so it knows where Go stores files
        }

        @JavascriptInterface
        public String getDataDir() {
            return getFilesDir().getAbsolutePath();
        }

        @JavascriptInterface
        public String readFileAsBase64(String path) {
            try {
                InputStream in;
                if (path.startsWith("content://")) {
                    in = getContentResolver().openInputStream(Uri.parse(path));
                } else {
                    in = new FileInputStream(path);
                }
                ByteArrayOutputStream buf = new ByteArrayOutputStream();
                byte[] tmp = new byte[8192];
                int n;
                while ((n = in.read(tmp)) > 0) {
                    buf.write(tmp, 0, n);
                }
                in.close();
                byte[] data = buf.toByteArray();
                // 检测图片格式
                String mime = "image/jpeg";
                if (data.length > 2) {
                    if (data[0] == (byte)0xFF && data[1] == (byte)0xD8) mime = "image/jpeg";
                    else if (data[0] == (byte)0x89 && data[1] == (byte)0x50) mime = "image/png";
                    else if (data[0] == (byte)0x47 && data[1] == (byte)0x49) mime = "image/gif";
                    else if (data[0] == (byte)0x52 && data[1] == (byte)0x49) mime = "image/webp";
                    else if (data[0] == (byte)0x42 && data[1] == (byte)0x4D) mime = "image/bmp";
                }
                String b64 = Base64.getEncoder().encodeToString(data);
                return "data:" + mime + ";base64," + b64;
            } catch (Exception e) {
                return "";
            }
        }

        @JavascriptInterface
        public boolean checkStoragePermission() {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                return Environment.isExternalStorageManager();
            }
            // Android 10 及以下需要检查 READ_EXTERNAL_STORAGE
            return checkSelfPermission(Manifest.permission.READ_EXTERNAL_STORAGE)
                    == PackageManager.PERMISSION_GRANTED;
        }

        @JavascriptInterface
        public void requestStoragePermission() {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                Intent intent = new Intent(Settings.ACTION_MANAGE_APP_ALL_FILES_ACCESS_PERMISSION);
                intent.setData(Uri.parse("package:" + getPackageName()));
                startActivityForResult(intent, STORAGE_PERM_CODE);
            } else {
                // Android 10 及以下用普通权限请求
                requestPermissions(
                    new String[]{Manifest.permission.READ_EXTERNAL_STORAGE},
                    STORAGE_PERM_CODE
                );
            }
        }

    }

    private String copyFileFromUri(Uri uri) {
        try {
            // 获取真实文件名
            String name = null;
            try (Cursor cursor = getContentResolver().query(uri, null, null, null, null)) {
                if (cursor != null && cursor.moveToFirst()) {
                    int idx = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME);
                    if (idx >= 0) name = cursor.getString(idx);
                }
            } catch (Exception e) { /* fallback */ }
            if (name == null || name.isEmpty()) {
                name = "imported_" + System.currentTimeMillis();
                String ext = guessExtension(uri);
                name += ext;
            }
            File outFile = new File(getFilesDir(), name);

            try (InputStream in = getContentResolver().openInputStream(uri);
                 FileOutputStream out = new FileOutputStream(outFile)) {
                byte[] buf = new byte[8192];
                int n;
                while ((n = in.read(buf)) > 0) {
                    out.write(buf, 0, n);
                }
            }
            outFile.setReadable(true);
            return outFile.getAbsolutePath();
        } catch (Exception e) {
            return "";
        }
    }

    private String getApkSignatureHash() {
        try {
            PackageInfo info = getPackageManager().getPackageInfo(
                getPackageName(), PackageManager.GET_SIGNING_CERTIFICATES
            );
            if (info.signingInfo != null && info.signingInfo.getApkContentsSigners() != null
                && info.signingInfo.getApkContentsSigners().length > 0) {
                Signature sig = info.signingInfo.getApkContentsSigners()[0];
                MessageDigest md = MessageDigest.getInstance("SHA-256");
                byte[] hash = md.digest(sig.toByteArray());
                StringBuilder hex = new StringBuilder();
                for (byte b : hash) {
                    hex.append(String.format("%02x", b));
                }
                return hex.toString();
            }
        } catch (Exception e) {
            // silently fail - signature check will report as unavailable
        }
        return null;
    }

    private String guessExtension(Uri uri) {
        String name = uri.getLastPathSegment();
        if (name == null) return "";
        int dot = name.lastIndexOf('.');
        if (dot >= 0) return name.substring(dot);
        return "";
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == FILE_PICK_CODE && resultCode == RESULT_OK && data != null && data.getData() != null) {
            Uri uri = data.getData();
            String realPath = copyFileFromUri(uri);
            // 如果 copyFileFromUri 失败，用 content:// URI 兜底
            if (realPath.isEmpty()) {
                realPath = uri.toString();
            }
            lastPickedPath = realPath;
            String escaped = realPath.replace("\\", "\\\\").replace("'", "\\'");
            handler.post(() -> webView.evaluateJavascript(
                "window.__onFilePicked__('" + escaped + "')", null));
        } else if (requestCode == STORAGE_PERM_CODE) {
            // 从系统设置页返回后通知 JS
            boolean granted = new JSBridge().checkStoragePermission();
            handler.post(() -> webView.evaluateJavascript(
                "window.__onStoragePermissionResult__(" + granted + ")", null));
        }
    }

    @Override
    public void onBackPressed() {
        webView.evaluateJavascript("window.__onBackPressed__()", val -> {
            if ("false".equals(val) || "null".equals(val) || "\"\"".equals(val)) {
                super.onBackPressed();
            }
        });
    }

    @Override
    public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode == STORAGE_PERM_CODE && grantResults.length > 0) {
            boolean granted = grantResults[0] == PackageManager.PERMISSION_GRANTED;
            handler.post(() -> webView.evaluateJavascript(
                "window.__onStoragePermissionResult__(" + granted + ")", null));
        }
    }

    @Override
    protected void onPause() {
        super.onPause();
        if (webView != null) webView.onPause();
    }

    @Override
    protected void onResume() {
        super.onResume();
        if (webView != null) webView.onResume();
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        if (webView != null) webView.destroy();
    }
}
