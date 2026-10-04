# Prism 项目结构文档

## 项目概述

Prism（Phoenix Server）是一个网易 Minecraft 基岩版（MCBE）租赁服认证与账号管理系统。
主要功能：网站用户注册/登录、游戏账号（Cookie）管理、MCBE 完整认证流程（登录、CheckNum 挑战码计算、转服）。

- **后端**：Go + SQLite，端口 8081
- **前端**：React + TypeScript + Vite
- **系统服务**：systemd `phoenix-auth.service`

---

## 一、后端目录结构

```
prism/
├── cmd/                          # 入口程序
│   ├── phoenix_server/           # 主服务（Web API + 前端静态文件）
│   ├── huxauth_server/           # 轻量认证服务（:9191）
│   └── checknum_fixed/           # CheckNum 命令行工具
│
├── internal/                     # 核心库（不对外暴露）
│   ├── apisrv/                   # 轻量 API 服务（huxauth 用）
│   ├── app/                      # 交互式 TUI 客户端
│   ├── auth/                     # 认证中间件、邮件、限流
│   ├── authsvc/                  # G79 认证服务封装
│   ├── console/                  # TUI 界面工具
│   ├── db/                       # SQLite 数据库层
│   └── netproxy/                 # 网络代理管理
│
├── third_party/                  # 内嵌的第三方库
│   ├── g79client/                # G79 协议客户端（网易 MCBE 网络协议）
│   └── unmcpk/                   # MCPK 解包 + CheckNum 挑战码计算
│
├── go.mod / go.sum               # Go 模块定义
├── config.json                   # 服务配置（SMTP、DB路径等）
├── API.md                        # API 文档
└── phoenix_server                # 编译产物（systemd 部署用）
```

---

## 二、入口程序 `cmd/`

### `cmd/phoenix_server/` — 主服务（端口 8081）

| 文件 | 功能 |
|---|---|
| `main.go` | 服务入口。注册所有 HTTP 路由、初始化 DB、启动 Cookie 刷新定时器、启动 HTTP 服务。包含 CookieStore（多 Cookie 管理）、session 存储、Phoenix 认证链路（login/transfer_start/transfer_check_num） |
| `auth_handlers.go` | 用户认证相关 handler：注册/登录/验证邮箱/修改密码/登出。**账号管理 CRUD**：`handleListAccounts`（列出用户私有+共享池账号）、`handleAddAccount`（添加账号并验证 Cookie）、`handleSwitchAccount`（切换活跃账号）、`handleActiveAccount`（获取当前活跃账号）、`handleDeleteAccount`（删除账号，级联清理共享副本）、`handleRefreshAccount`（刷新账号状态）。修改昵称 `handleUpdateAccountNickname` |
| `settings_handlers.go` | 账号共享/取消共享 `handleUpdateAccount`（设为共享→创建共享副本，取消共享→收回私有）、管理员功能：`handleAdminAccounts`（列出全部账号）、`handleAdminAddAccount`、`handleAdminCloneAccount`（复制共享账号到私有）、`handleAdminUpdateAccount`、`handleAdminDeleteAccount`、`handleAdminUsers`、审计日志、实名预设管理 |
| `server_list_handlers.go` | 服务器列表/搜索、社交功能（好友申请/回复/动态/消息）、商店搜索、大厅/领域、皮肤更换、`handleAccountRotate`（轮换切换账号） |
| `mpay_db.go` | NetEase MPay（手机号/邮箱注册网易账号）账号保存逻辑。认证成功后调用 `saveMPayAccountForRequest` 写入 DB |
| `session_cookie.go` | Cookie 导入/存储：`loadCookieOld`（加载旧版 cookie.json）、`addStoredCookieFromLogin`（登录后保存 Cookie）、CookieStore 初始化 |
| `activation_handlers.go` | 激活码生成与兑换 |
| `preset_handlers.go` | 皮肤预设 CRUD |

### `cmd/huxauth_server/main.go` — 轻量认证服务（端口 9191）

独立的精简 Phoenix 认证服务，使用 `internal/apisrv` 包。只提供 Phoenix 登录/CheckNum/转服/大厅核心端点，无前端、无用户系统。

### `cmd/checknum_fixed/main.go` — CheckNum CLI 工具

命令行计算 MCBE 传输 CheckNum 挑战码。接受 isPC、data、engineVersion、patchVersion 参数。

---

## 三、核心库 `internal/`

### `internal/db/` — SQLite 数据库层

| 文件 | 功能 |
|---|---|
| `db.go` | **数据库初始化**。打开 SQLite（WAL 模式），执行迁移建表：`users`（网站用户）、`game_accounts`（游戏账号）、`verify_codes`（验证码）、`audit_logs`（审计日志）、`system_logs`、`nuts_transactions`（板栗交易）、`activation_codes`、`login_stats`、`realname_presets`（实名预设）、`skin_presets`（皮肤预设）。索引：`idx_accounts_owner`（加速按用户查账号）、`idx_audit_user`、`idx_audit_action` |
| `accounts.go` | **游戏账号 CRUD**。`GameAccount`/`AccountInfo` 结构体。`GetUserAccounts`（查用户的私有账号）、`GetSharedAccounts`（查共享池）、`AddAccount`（upsert：同 uid+owner 更新而非重复插入）、`DeleteAccount`（删除时级联清理同 uid 的共享副本）、`LookupAccountByUID`、`ListAllAccounts`（含内存去重）、`HasSharedCopy`（检查某 uid 是否有共享副本）、`UpdateAccountStatus`/`UpdateAccountFull`/`UpdateAccountName`、`SetAccountShared`（切换共享/私有归属） |
| `users.go` | **用户 CRUD**。`User` 结构体。`CreateUser`/`CreateVerifiedUser`、`GetUserByID`/`GetUserByEmail`/`GetUserByToken`/`GetUserBySessionCookie`、`SetActiveAccount`（切换活跃账号）、`CreateVerifyCode`/`CheckVerifyCode`（邮箱验证码）、`IsAdminExists`、session 内存存储（`map[string]int64`） |
| `logs.go` | 审计日志 `AuditLog`（`AddAuditLog`/`ListAuditLogs`）、系统日志 `SystemLog`（`AddSystemLog`/`ListSystemLogs`） |
| `nuts.go` | 板栗（虚拟货币）交易。`NutsTransaction`、`AddNuts`（增减余额）、`ListNutsTransactions` |
| `activation.go` | 激活码系统。`GenerateActivationCode`、`RedeemCode` |
| `realname.go` | 实名预设 CRUD（`AddRealnamePreset`/`ListRealnamePresets`/`TogglePreset`/`DeletePreset`） |
| `skin_presets.go` | 皮肤预设 CRUD |
| `stats.go` | 登录统计 `RecordLogin`/`GetLoginStats`（按小时分组） |

### `internal/auth/` — 认证与中间件

| 文件 | 功能 |
|---|---|
| `auth.go` | **`SessionAuth`**：HTTP 中间件，校验 `phx_sid` Cookie 或 `Authorization: Bearer` token。**`AdminAuth`**：SessionAuth + 管理员角色检查。**`Config`**：JSON 配置文件加载（SMTP、管理员邮箱等）。**`SendMail`**：SMTP 邮件发送。**`RateLimiter`**：令牌桶限流器。**`GlobalIPLimiter`**：IP 级别全局限流+封禁 |

### `internal/authsvc/` — G79 认证服务封装

| 文件 | 功能 |
|---|---|
| `client.go` | 创建 G79 客户端、调用 `AuthenticateWithCookie` |
| `login.go` | **完整登录流程**：搜索租赁服 → 进入世界 → PE 认证 v2（`PerformFullLogin`） |
| `tan_lobby.go` | 局域网大厅操作（登录/创建/获取转服列表） |
| `skin.go` | 皮肤信息获取（用户详情+其他玩家详情） |
| `verify.go` | 验证码操作 |
| `types.go` | `LoginParams`/`LoginResult`/`TanLobbyLoginResult` 等类型定义 |

### `internal/apisrv/` — 轻量 API 服务

| 文件 | 功能 |
|---|---|
| `server.go` | huxauth_server 使用的精简服务。含内存 session 管理、Phoenix 登录/CheckNum/大厅端点 |
| `types.go` | 请求/响应结构体 |

### `internal/app/` — 交互式 TUI

| 文件 | 功能 |
|---|---|
| `app.go` | 交互式控制台应用（start-auth / direct-enter 模式） |
| `models.go` | 游戏模式目标（rental/domain/tan/online/network/main_city） |
| `token.go` | 令牌生成工具 |
| `crypto.go` | 客户端公钥生成 |

### `internal/console/` — TUI 界面

| 文件 | 功能 |
|---|---|
| `prompt.go` | 文本提示和选项选择 |
| `ui.go` | 样式化输出 |

### `internal/netproxy/` — 网络代理

| 文件 | 功能 |
|---|---|
| `proxy.go` | 代理管理器 |

---

## 四、第三方库 `third_party/`

### `g79client/` — G79 协议客户端

网易 Minecraft 基岩版网络协议客户端。核心文件：

| 文件 | 功能 |
|---|---|
| `client_core.go` | **客户端核心**。连接管理、认证、请求签名。包含 `NewClient`、`G79AuthenticateWithCookie`、`GetUserDetail`、`GetPeUserLoginAfter` |
| `client_http.go` | HTTP 请求封装（加密/解密/签名） |
| `auth.go` | 认证相关 |
| `crypto.go` | 加密工具 |
| `const.go` / `types.go` / `globals.go` / `engine_consts.go` | 常量、类型、全局配置 |
| `rental_*.go` | 租赁服相关（搜索/详情/进入） |
| `domain_*.go` | 领域服相关（加入/详情/进入/离开） |
| `online_lobby_*.go` | 在线大厅相关 |
| `user_*.go` | 用户相关（详情/好友/动态/设置/昵称） |
| `store_*.go` | 商店相关 |
| `skin_change.go` | 皮肤更换 |
| `social_profile.go` | 社交资料 |
| `nickname_tracker.go` | 昵称追踪 |
| `service/chat_connection/` | 聊天连接服务 |
| `service/link_connection/` | 链路连接服务 |
| `account/mpay/` | **网易 MPay 账号系统**：游客登录/邮箱登录/手机号验证/实名认证 |
| `utils/` | 加密工具（AES/ChaCha8） |
| `example/` | **示例代码**（登录/转服/大厅/商店/领域/社交等各功能独立示例） |

### `unmcpk/` — MCPK 解包 + CheckNum

| 文件 | 功能 |
|---|---|
| `main.go` / `checknum.go` | 入口和 CheckNum 生成 |
| `mcpk/` | MCPK 文件格式解包（目录/文件系统/哈希/转子加密） |
| `pymarshal/` | Python marshal 序列化格式的 Go 实现（网易加密协议的序列化层） |
| `hotpatch/` | 热补丁引擎（清单/补丁下载） |
| `cmd/` | CLI 工具入口 |

---

## 五、前端目录结构

```
prism-web/
├── src/
│   ├── main.tsx                  # React 入口
│   ├── App.tsx                   # 根组件：路由配置（/login /register /dashboard /admin /logs /settings）
│   ├── api.ts                    # API 调用层：所有后端接口的封装函数
│   ├── AuthContext.tsx            # 认证上下文：登录状态、用户信息、token 管理
│   ├── types.ts                  # TypeScript 类型定义
│   ├── sounds.ts                 # 音效管理（点击音效）
│   ├── styles/
│   │   ├── app.css               # 全局样式
│   │   └── themes.css            # 主题变量
│   ├── assets/                   # 静态资源（Steve 头像图片）
│   └── components/
│       ├── LoginPage.tsx         # 登录页
│       ├── RegisterPage.tsx      # 注册页
│       ├── ForgotPasswordPage.tsx # 忘记密码页
│       ├── Dashboard.tsx         # **主面板**：账号列表（私有/共享池）、添加账号（游客/手机/邮箱/Cookie）、切换/刷新/删除/共享/收回、皮肤更换、昵称修改
│       ├── AdminPage.tsx         # **管理后台**：用户管理、账号管理（停用/共享切换/删除/复制Cookie）、审计日志、系统日志、实名预设、皮肤预设、登录统计
│       ├── UserSettingsPage.tsx   # 用户设置：修改用户名/密码/邮箱、重置Token、板栗交易记录、挑战码开关、成长值开关
│       ├── LogsPage.tsx           # 日志查看页
│       ├── CookieLogin.tsx        # Cookie 登录面板
│       ├── EmailLogin.tsx         # 邮箱登录面板
│       ├── GuestLogin.tsx         # 游客登录面板
│       ├── PhoneLogin.tsx         # 手机号登录面板
│       ├── LoginChart.tsx         # 登录统计图表
│       ├── NutsPolicy.tsx         # 板栗（虚拟货币）消费规则说明
│       ├── ApiDoc.tsx             # API 文档页面
│       ├── SkinHead.tsx           # 皮肤头像 3D 渲染组件
│       ├── Modal.tsx              # 通用弹窗组件
│       ├── Toast.tsx              # Toast 提示组件
│       ├── ResultBox.tsx          # 结果显示框
│       └── SoundFX.tsx            # 音效包装组件
├── dist/                          # Vite 构建产物（gitignore，部署时由 Go 服务 serve）
├── vite.config.ts                 # Vite 配置（含 /api 代理到 8081）
├── tsconfig.json                  # TypeScript 配置
└── package.json                   # 依赖和脚本（npm run build = tsc + vite build）
```

---

## 六、前端组件功能速览

### 页面组件

| 组件 | 路由 | 功能 |
|---|---|---|
| `LoginPage` | `/login` | 邮箱+密码登录 |
| `RegisterPage` | `/register` | 邮箱注册（含验证码） |
| `ForgotPasswordPage` | `/forgot-password` | 忘记密码重置 |
| `Dashboard` | `/dashboard` | **核心页面**。展示私有账号+共享池账号卡片，切换/删除/共享/刷新/昵称/皮肤操作，添加账号入口 |
| `AdminPage` | `/admin` | 管理后台（仅 admin 角色可见） |
| `UserSettingsPage` | `/settings` | 个人设置 |
| `LogsPage` | `/logs` | 操作日志 |
| `ApiDoc` | `/api-doc` | API 文档 |

### 功能组件

| 组件 | 功能 |
|---|---|
| `GuestLogin` | 网易游客登录 → 创建游戏账号 |
| `PhoneLogin` | 手机号+验证码登录 → 创建游戏账号 |
| `EmailLogin` | 邮箱密码登录 → 创建游戏账号 |
| `CookieLogin` | 直接粘贴 Cookie JSON 添加账号 |
| `SkinHead` | Minecraft 皮肤头像渲染 |
| `Modal` | 通用确认弹窗 |
| `Toast` | 操作提示条 |
| `LoginChart` | 登录量统计柱状图 |
| `NutsPolicy` | 板栗积分消费规则表 |
| `SoundFX` | 按钮点击音效 |

---

## 七、数据流概览

```
浏览器 → React 前端 (Vite)
  ├─ /api/* → Vite proxy → Go 后端 (:8081)
  │   ├─ SessionAuth 中间件 → 校验 phx_sid / Bearer token
  │   ├─ Handler → db.Query → SQLite (phoenix.db)
  │   └─ Handler → g79client → 网易 MCBE API
  └─ 其他 → Go FileServer → prism-web/dist/ 静态文件
```

### 关键数据表

```
users ──< game_accounts
  │            ├── owner_id = X   → 用户 X 的私有账号
  │            ├── owner_id = NULL → 共享池账号
  │            ├── created_by      → 谁添加的（影响 can_reclaim）
  │            └── auto_refresh_enabled → 自动刷新开关（cookie/guest 默认开，phone/email 默认关）
  │
  ├── active_account_id → 当前激活的游戏账号
  ├── token             → API 认证令牌（adb//...）
  ├── nuts_balance      → 板栗余额
  ├── activated         → 是否已激活（登录超3次后需激活）
  └── login_count       → 登录次数（Phoenix 进服计数）
```

---

## 八、运维指南

### 基本信息

| 项目 | 值 |
|---|---|
| 代码仓库 | `https://github.com/adb-lanlu/prism.git`（或本地 `/root/fa`） |
| 后端模块 | `github.com/adb-lanlu/prism-oss`（Go module） |
| 源码路径 | `/root/fa/prism`（后端）、`/root/fa/prism-web`（前端） |
| 运行端口 | `:8081`（主服务）、`:9191`（huxauth 轻量服务） |
| 运行用户 | `phoenix`（非 root 系统账号） |
| 部署路径 | `/opt/phoenix` |
| 数据库文件 | `/opt/phoenix/phoenix.db`（SQLite） |
| 配置文件 | `/opt/phoenix/config.json`（SMTP、DB路径、管理员邮箱） |
| 前端文件 | `/opt/phoenix/frontend/`（Vite 构建产物） |
| 密码表 | `/opt/phoenix/_data/wordlists/` |
| 系统服务 | `phoenix-auth.service`（systemd 管理） |

### 构建

```bash
# 后端
cd /root/fa/prism
go build -o /opt/phoenix/phoenix_server ./cmd/phoenix_server

# 前端
cd /root/fa/prism-web
npm run build          # tsc 类型检查 + Vite 打包 → dist/
cp -r dist /opt/phoenix/frontend
chown -R phoenix:phoenix /opt/phoenix/frontend
```

### 部署

服务以 `phoenix` 用户（非 root）运行，部署目录为 `/opt/phoenix`：

```bash
# 完整部署流程
go build -o /opt/phoenix/phoenix_server ./cmd/phoenix_server
chown phoenix:phoenix /opt/phoenix/phoenix_server
systemctl restart phoenix-auth.service

# 仅推送前端更新
cp -r dist /opt/phoenix/frontend
chown -R phoenix:phoenix /opt/phoenix/frontend
# 前端是热加载的，无需重启服务
```

### 重启服务

```bash
systemctl restart phoenix-auth.service   # 重启
systemctl status phoenix-auth.service    # 查看状态
journalctl -u phoenix-auth.service -f    # 实时日志
```

> **注意**：构建后二进制必须覆盖 `/opt/phoenix/phoenix_server`，否则重启不会生效。

### 服务配置

```ini
# /etc/systemd/system/phoenix-auth.service
[Service]
User=phoenix
WorkingDirectory=/opt/phoenix
ExecStart=/opt/phoenix/phoenix_server
Restart=always
RestartSec=3

# /etc/systemd/system/phoenix-auth.service.d/proxy.conf
[Service]
Environment="HTTPS_PROXY=socks5://127.0.0.1:1089"
Environment="NO_PROXY=localhost,127.0.0.1"
```

### 开发调试

```bash
# 前端开发模式（热更新，代理 API 到 8081）
cd /root/fa/prism-web
npm run dev             # Vite dev server，默认端口 5173

# 直接查看数据库
sqlite3 /opt/phoenix/phoenix.db

# 测试 API
TOKEN="adb//xxx"
curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8081/api/accounts
```

### 常用工具

| 工具 | 用途 |
|---|---|
| `sqlite3 phoenix.db` | 直接查询/修改数据库 |
| `go build ./cmd/phoenix_server` | 编译后端 |
| `npm run build` | 构建前端 |
| `systemctl restart phoenix-auth` | 重启线上服务 |
| `journalctl -u phoenix-auth -f` | 查看实时日志 |
| `grep` / `git log` | 搜索代码 / 查看提交历史 |
| `checknum_dynamic` | 独立 CheckNum 计算二进制（调试认证流程用） |
