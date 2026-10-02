> 📌 **本仓库说明**
>
> 本仓库为 **Prism (phoenix-server) 开源版本的重新上传 / 镜像仓库**，仅用于留存与分享。
>
> - 内容来自公开开源的 `prism-oss`，**未做任何功能修改**。
> - 原始版权与许可归原作者所有，遵循 **GPL-3.0**。
> - 若需最新版本 / 提交 Issue，请优先联系原项目。
> - 仅供学习研究，请勿用于任何违反相关法律法规或服务条款的用途。

---

# Prism (phoenix-server) — 网易《我的世界》基岩版租赁服认证与账号管理平台

`prism-oss` 是 **Phoenix Server** 的开源版本:面向网易《我的世界》基岩版租赁服(Realms)的认证网关与账号管理平台,附带一个 React + TypeScript 管理前端。

> ⚠️ 这是**公开开源版**:已剥离全部生产密钥、数据库、真实账号凭据与部署 IP,且移除了密码爆破等滥用能力。若你 fork 使用,请自行审计并**不要**提交任何真实凭据。

---

## 目录结构

```
prism-oss/
├── backend/                    # Go 后端(Phoenix Server,监听 :8081)
│   ├── cmd/phoenix_server/     # 主程序 + 各业务 handler
│   ├── internal/               # 数据层 / 认证 / 插件扫描等内部包
│   ├── third_party/            # 以 replace 引用的私有模块(见下)
│   ├── config.example.json     # 配置模板(复制为 config.json 使用)
│   ├── proxies.conf.example    # 认证出口代理池模板
│   ├── go.mod / go.sum
├── frontend/                   # React 19 + TypeScript + Vite 前端
│   ├── src/                    # 组件 / 页面 / 样式
│   ├── public/                 # 静态资源(音频、SVG、vendored 播放器)
│   └── package.json / vite.config.ts / tsconfig*
├── docs/                       # API / 管理 / 结构 / 开发文档
├── LICENSE                     # GPL-3.0
└── README.md
```

### 后端 handler 划分(`backend/cmd/phoenix_server/`)

| 文件 | 职责 |
|---|---|
| `main.go` | 入口:路由注册、DB 初始化、Cookie 刷新、HTTP 服务 |
| `auth_handlers.go` | 注册/登录/邮箱验证/改密/登出 + 游戏账号 CRUD |
| `settings_handlers.go` | 账号共享/收回 + 管理员功能 |
| `server_list_handlers.go` | 服务器列表/搜索、社交、商店、大厅/领域、账号轮换 |
| `market_handlers.go` | 市场:上传/列表/详情/评论/分类/标签/管理 |
| `plugin_market_handlers.go` | 插件市场:上传/版本/经济/审核 |
| `music_handlers.go` | 音乐热榜 + 下载 |
| `payment_handlers.go` | 支付下单/查询/回调,积分充值 |
| `token_handlers.go` | API Token 管理 |
| `redpacket_/checkin_/survey_/preset_handlers.go` | 红包码/签到/问卷/皮肤预设 |
| `node_handlers.go` / `p2p_coord.go` | 令牌节点贡献与 P2P 出口协调 |
| `ai_proxy.go` | OpenAI 兼容 AI 代理端点 |
| `security_middleware.go` | 认证/限流/审计中间件 |
| `captcha_image.go` / `captcha_pool.go` | 图形验证码 |
| `messages.go` | 报错文案热重载 |

### 内部包(`backend/internal/`)

| 包 | 职责 |
|---|---|
| `db/` | 数据层:用户、账号、市场、插件市场、支付、审计、签到等 |
| `auth/` | 认证中间件、Session/Token、配置加载 |
| `authsvc/` | G79 认证服务客户端(登录、验证、大厅) |
| `apisrv/` | API 服务 |
| `netproxy/` | 网络代理 |
| `pluginscan/` | 插件市场源码安全扫描 |
| `console/` | 控制台界面 |

---

## 能力

### 账号认证
- 注册 / 登录 / 登出、邮箱验证、忘记密码、登录态刷新
- Session Cookie + Bearer Token 双认证;可选访客登录
- 图形验证码 + 每接口独立限流 + 全局限流兜底

### 游戏账号管理
- 网易 MC 账号 CRUD(列表/添加/切换/删除/刷新/改昵称)
- 私有池 / 共享池(复制行实现共享)、账号轮换
- Cookie 导入与在线心跳维护

### 市场与内容
- 文件市场:上传/下载/列表/详情、评论、分类、标签、所有者管理
- 插件市场:上传、版本管理、经济结算、源码安全审核(`pluginscan`)
- 音乐热榜 + 下载(带单曲热度冷却)
- 社交:好友、动态、消息

### 支付与积分
- 支付下单 / 查询 / 回调、积分(Nuts)充值
- 红包码、签到、问卷、皮肤预设等福利

### 服务器与大厅
- 租赁服列表 / 搜索、领域、大厅联机

### 管理后台
- 用户/账号管理、审计日志、实名预设
- 工具箱:版本发布、配置、公告
- 统计面板

### 安全
- Session / Token / Admin 三层认证中间件
- 接口限流、审计日志(与前端翻译表同步)
- AI 代理端点(OpenAI 兼容)

---

## 构建与运行

### 后端

```bash
cd backend
go build -o cmd/phoenix_server/phoenix_server ./cmd/phoenix_server
# 首次使用:把示例配置复制为真实配置,填入你自己的 SMTP/支付/URL
cp config.example.json cmd/phoenix_server/config.json
cp proxies.conf.example cmd/phoenix_server/proxies.conf
go run ./cmd/phoenix_server
```

运行时读取(均需自行准备,**不要提交真实值**):
- `cmd/phoenix_server/config.json` — SMTP、支付、基础 URL 等(参考 `config.example.json`)
- `cmd/phoenix_server/proxies.conf` — 认证出口 SOCKS5 代理池(参考 `proxies.conf.example`)
- `cmd/phoenix_server/messages.json` — 报错文案(5s 热重载)

### 前端

```bash
cd frontend
npm install
npm run dev      # 开发 :5173,/api 代理到 127.0.0.1:8081
npm run build    # 产物 → dist/(由后端从 ../dist 挂载)
```

---

## 第三方依赖(replace 指向本地)

`backend/third_party/` 下以 `replace` 方式引用的模块:

| 模块 | 用途 |
|---|---|
| `third_party/g79client` | 网易 G79 认证客户端(登录、租赁服/大厅/商店/社交操作) |
| `third_party/unmcpk` | 封包解包 / 校验码 / 热补丁工具 |

---

## 文档

见 `docs/`:`API.md`、`ADMIN.md`(管理员接口)、`STRUCTURE.md`(代码结构)、`DEV-GUIDE.md`(开发规范)、`RELEASE.md`(发布)、`plugin-review-guide.md`(插件审核)。

## 许可证

[GPL-3.0](./LICENSE)
