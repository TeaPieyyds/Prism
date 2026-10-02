# Prism 版本发布指南

## 概述

Prism 的版本管理通过后台"工具箱"完成，包含三个部分：
- **版本发布** — APK 上传、扫描、版本切换
- **版本配置** — 最新版本号、更新日志、强制更新、下载地址
- **公告管理** — 面向 Android 客户端的弹窗/横幅公告

---

## 一、发布新版本

### 1. 放置 APK 文件

将编译好的 APK 放到服务器的 `data/releases/` 目录：

```bash
# 放在 /root/fa/prism/cmd/phoenix_server/data/releases/
```

文件名格式：

```
prism-{版本号}_{版本代号}.apk
```

示例：
```
prism-1.0.4_8.apk          # 版本号 1.0.4，版本代号 8
prism-1.0.5-beta.1_9.apk   # 版本号 1.0.5-beta.1，版本代号 9
```

如果不写版本代号（如 `prism-1.0.5.apk`），系统会自动分配下一个可用代号。

### 2. 扫描新版本

1. 打开网页后台 → **后台管理** → **工具箱** 标签
2. 点击 **"扫描新版本"** 按钮
3. 系统会扫描 `data/releases/` 目录，自动识别新的 APK 文件
4. 新版本会出现在列表中

扫描逻辑：
- 从文件名解析版本号（`prism-` 和 `.apk` 之间的部分）
- 从文件名或自动分配版本代号（`version_code`，整数）
- 记录文件大小
- 避免重复添加（已存在的文件名会跳过）

### 3. 切换到新版本

1. 在版本列表中找到新版本
2. 点击 **"切换"** 按钮
3. 该版本被设为"当前版本"

Android 客户端下次检查更新时就会收到这个版本。

### 4. 配置版本信息

在 **"版本配置"** 区域填写：

| 字段 | 说明 | 示例 |
|---|---|---|
| 最新版本号 | 如 `1.0.4` 或 `1.0.5-beta.1` | `1.0.4` |
| 版本号(整数) | 数字越大越新，用于比较 | `10400` |
| 下载地址 | APK 的公网下载地址 | `https://example.com/dl/prism-1.0.4_8.apk` |
| 更新说明 | Markdown 格式，客户端会渲染 | 见下方示例 |
| 强制更新 | 勾选后旧版本必须更新才能使用 | ✓ |

点击 **"保存版本配置"**。

---

## 二、更新日志（Markdown 格式）

更新说明支持 Markdown，示例：

```markdown
# v1.0.4 更新内容

## 新增
[+] 建筑导出 — MCStructure / Schematic / MCWorld 存档
[+] 深色蓝灰 / 墨绿 / 暗紫多主题
[+] 断点任务按服务器隔离
[+] OP 后自动切换创造模式

## 变更
[≈] 导入进度条改为实体方块进度
[≈] 像素画自动旋转 180°

## 修复
[✓] 导入任务无法停止
[✓] 音乐暂停/换曲异常
[✓] 模组服被踢下线
```

---

## 三、下载地址

客户端通过 `/dl/{文件名}` 下载 APK：

```
https://你的域名/dl/prism-1.0.4_8.apk
```

这个地址需要填写到版本配置的 **"下载地址"** 字段里。

---

## 四、发布公告

如果需要配合版本更新发送公告：

1. 工具箱 → **公告管理** → **新建公告**
2. 填写：

| 字段 | 说明 |
|---|---|
| 标题 | 公告标题 |
| 内容 | Markdown 格式 |
| content_type | markdown / text / html |
| severity | info（信息）/ warning（警告）/ critical（严重） |
| display_mode | modal（弹窗）/ modal_once（一次弹窗）/ banner（横幅）/ inline（内嵌） |
| 可关闭 | 用户能否关闭公告 |
| 立即启用 | 创建后立刻生效 |
| 目标版本 | 逗号分隔的版本号，只对指定版本显示，留空=所有版本 |
| 目标服务器 | 逗号分隔的服务器号，留空=所有服务器 |
| 过期时间 | 公告自动失效的时间，留空=不失效 |

---

## 五、签名管理

如果换了签名密钥，需要更新签名白名单：

1. 工具箱 → **签名管理** → **添加**
2. 填写包名（通常 `com.prismtool.box`）和 SHA256 哈希

客户端检查更新时会发送签名哈希，服务端验证是否在白名单中。不在白名单的签名会被标记为无效。

---

## 六、完整发布流程

```
1. 构建 APK
       ↓
2. 上传到 data/releases/
       ↓
3. 后台"工具箱" → 扫描新版本
       ↓
4. 切换到新版本（设为当前版本）
       ↓
5. 更新版本配置（版本号、下载地址、更新日志）
       ↓
6. （可选）新建公告通知用户更新
       ↓
7. 客户端下次启动时自动检测更新
```

---

## 七、API 端点速查

| 端点 | 用途 |
|---|---|
| `GET /dl/{filename}` | 下载 APK 文件 |
| `POST /api/v2/version` | 客户端检查更新 |
| `POST /api/v2/announcements` | 客户端拉取公告 |
| `GET /api/admin/toolbox/version` | 后台获取版本配置 |
| `POST /api/admin/toolbox/version` | 后台更新版本配置 |
| `GET /api/admin/toolbox/versions` | 后台获取版本列表 |
| `POST /api/admin/toolbox/versions/scan` | 后台扫描新版本 |
| `POST /api/admin/toolbox/versions/switch` | 后台切换当前版本 |
| `GET /api/admin/toolbox/announcements` | 后台获取公告列表 |
| `POST /api/admin/toolbox/announcements` | 后台创建公告 |
| `PUT /api/admin/toolbox/announcement` | 后台更新公告 |
| `DELETE /api/admin/toolbox/announcement` | 后台删除公告 |

---

## 八、两种操作方式

### 方式一：后台管理页面（推荐）

通过网页后台 → **后台管理** → **工具箱** 标签页操作。背后调用 API，自动写审计日志，字段格式和约束由代码保证，不会出错。

### 方式二：直接改数据库

也可以直接操作 SQLite 表：

```sql
-- 添加版本
INSERT INTO app_versions (version, version_code, file_name, file_size)
VALUES ('1.0.5', 10500, 'prism-1.0.5.apk', 5242880);

-- 添加公告
INSERT INTO app_announcements (id, title, content, is_active, target_versions, target_servers)
VALUES ('ann_v1.0.5', '更新公告', '内容...', 1, '[]', '[]');

-- 更新版本配置
UPDATE app_version_config SET latest_version='1.0.5', latest_version_code=10500;
```

> **注意**：直接改库不会有审计日志，`target_versions`/`target_servers` 字段必须是合法 JSON 数组格式（如 `[]` 或 `["1.0.4","1.0.3"]`），写错会导致 API 返回异常。

---

## 九、常见问题

**Q: 扫描不到新 APK？**
- 检查文件是否在 `data/releases/` 目录
- 文件名必须以 `.apk` 结尾
- 已在数据库中的文件名不会被重复添加

**Q: 客户端收不到更新？**
- 检查版本配置的 `version_code` 是否大于旧版本
- 检查下载地址是否可访问
- 检查强制更新开关

**Q: 公告不显示？**
- 检查 `is_active` 是否为启用
- 检查 `expires_at` 是否已过期
- 检查 `target_versions` 是否包含了客户端版本
