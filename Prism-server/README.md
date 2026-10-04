> 📌 **本目录说明**
>
> 本目录为 **Prism (phoenix-server) 开源版本的重新上传 / 镜像**，仅用于留存与分享。
>
> - 内容来自公开开源的 `prism-oss`，**未做任何功能修改**。
> - 原始版权与许可归原作者所有，遵循 **GPL-3.0**。
> - 若需最新版本 / 提交 Issue，请优先联系原项目。
> - 仅供学习研究，请勿用于任何违反相关法律法规或服务条款的用途。

---

# Prism (phoenix-server) — 网易《我的世界》基岩版租赁服认证与账号管理平台

`prism-oss` 是 **Phoenix Server** 的开源版本：面向网易《我的世界》基岩版租赁服（Realms）的认证网关与账号管理平台，附带一个 React + TypeScript 管理前端。

> ⚠️ 这是**公开开源版**：已剥离全部生产密钥、数据库、真实账号凭据与部署 IP，且移除了密码爆破等滥用能力。若你 fork 使用，请自行审计并**不要**提交任何真实凭据。

---

## 目录结构

```
Prism-server/
├── backend/                    # Go 后端（Phoenix Server，监听 :8081）
│   ├── cmd/phoenix_server/     # 主程序 + 各业务 handler
│   ├── internal/               # 数据层 / 认证 / 插件扫描等内部包
│   ├── third_party/            # 以 replace 引用的私有模块（见下）
│   ├── config.example.json     # 配置模板（复制为 config.json 使用）
│   ├── proxies.conf.example    # 认证出口代理池模板
│   └── go.mod / go.sum
├── frontend/                   # React 19 + TypeScript + Vite 前端
├── docs/                       # API / 管理 / 结构 / 开发文档
├── LICENSE                     # GPL-3.0
└── README.md
```

---

## 构建与运行

### 后端

```bash
cd backend
go build -o cmd/phoenix_server/phoenix_server ./cmd/phoenix_server
cp config.example.json cmd/phoenix_server/config.json
cp proxies.conf.example cmd/phoenix_server/proxies.conf
go run ./cmd/phoenix_server
```

### 前端

```bash
cd frontend
npm install
npm run dev      # 开发 :5173，/api 代理到 127.0.0.1:8081
npm run build    # 产物 → dist/（由后端从 ../dist 挂载）
```

---

## 第三方依赖（replace 指向本地）

| 模块 | 用途 |
|---|---|
| `third_party/g79client` | 网易 G79 认证客户端（登录、租赁服/大厅/商店/社交操作） |
| `third_party/unmcpk` | 封包解包 / 校验码 / 热补丁工具 |

---

## 文档

见 `docs/`：`API.md`、`ADMIN.md`、`STRUCTURE.md`、`DEV-GUIDE.md`、`RELEASE.md`、`plugin-review-guide.md`。

## 许可证

[GPL-3.0](./LICENSE)
