# Prism 工具箱 — 运维手册

## 前置条件

1. 有一个 Prism 网站的管理员账号（角色为 admin）
2. 登录后底部导航能看到「后台管理」

---

## 一、发布新版本

### 1. 把 APK 放到服务器

用 SCP/FTP/SFTP 把编译好的 APK 上传到服务器的这个目录：

```
data/releases/
```

文件名格式（二选一）：

```
prism-1.0.5.apk           → 版本号 1.0.5，版本代号自动分配
prism-1.0.5_5.apk         → 版本号 1.0.5，版本代号 5
```

### 2. 扫描注册

1. 打开网站 → 底部「后台管理」→ 顶部标签「工具箱」
2. 在「发布版本」卡片里点 **扫描新版本**
3. 系统自动读取 `data/releases/` 里的 APK 文件，没注册过的自动登记

### 3. 切换当前版本

在版本列表里，找到你要上线的版本，点 **切换**。

点完之后：
- `POST /api/v2/version/check` 会返回这个版本的信息
- 客户端的更新检测、下载链接全部自动更新
- 无需重启服务

### 4. 删除旧版本

只有非当前版本才能删除。当前版本需要先切换到别的版本再删。

---

## 二、管理公告

### 进入方式

后台管理 → 工具箱 → 公告管理

### 新建公告

点 **新建公告**，填写弹窗里的字段：

| 字段 | 说明 |
|------|------|
| 标题 | 公告标题 |
| 内容 | Markdown 格式正文，支持链接、图片、列表等 |
| 严重程度 | `info`（蓝色信息）/ `warning`（橙色警告）/ `critical`（红色严重） |
| 展示方式 | `modal`（弹窗）/ `modal_once`（一次弹窗）/ `banner`（顶部横幅）/ `inline`（内嵌卡片） |
| 目标版本 | 逗号分隔的版本代号，只对匹配的客户端展示，留空=全部版本 |
| 目标服务器 | 逗号分隔的服务器号，只对匹配的客户端展示，留空=全部服务器 |
| 可关闭 | 用户能否关掉这个公告 |
| 立即启用 | 勾选后创建即生效 |

点保存后公告立即生效（如果勾了「立即启用」）。

### 编辑/停用/删除

每条公告右侧有对应按钮。

---

## 三、签名白名单

防止 APK 被二次打包篡改。客户端每次启动上报签名哈希，服务端比对。

### 添加

后台管理 → 工具箱 → 签名管理：

- **包名**：`com.prismtool.box`
- **哈希**：APK 签名的 SHA-256（用 keytool/apksigner 查）
- **标签**：随便写，比如 `正式版 v1.0.5`

### 查签名哈希

```bash
# 方法 1: apksigner
apksigner verify --print-certs xxx.apk

# 方法 2: keytool（查 keystore）
keytool -list -v -keystore xxx.jks
```

拿到证书后计算 SHA-256 填进去。

---

## 四、版本检测接口说明

客户端调的是这两个：

### 版本检测

```
POST /api/v2/version/check
{"version": "1.0.4", "version_code": 4}
```

返回最新版本号、下载地址、是否强制更新、签名是否合法。

### 公告拉取

```
POST /api/v2/announcements
{"version_code": 4, "server_code": "12345678"}
```

返回当前生效的公告列表。服务端会根据版本/服务器/过期时间自动过滤。

---

## 五、目录结构

```
/opt/phoenix/
├── phoenix_server          # 编译好的二进制
├── config.json             # SMTP、DB路径、支付等配置
├── phoenix.db*             # SQLite 数据库（WAL 模式）
├── cookie.json             # 当前活跃 Cookie
├── cookies.json            # CookieStore 持久化
├── accounts_v2.json        # 账号列表持久化（备用）
├── capture.log             # 请求抓包日志
├── data/
│   └── releases/           ← APK 文件放这里
│       ├── prism-1.0.4.apk
│       └── prism-1.0.5_5.apk
├── frontend/               # Vite 构建产物（静态文件）
│   └── index.html
└── _data/
    └── wordlists/          # 密码测试用词库
        ├── 6digits_full.txt
        ├── seclist_pwdb_top100k.txt
        └── ...
```

下载路径格式：`https://example.com/dl/prism-1.0.5.apk`
