# Prism

面向网易《我的世界》基岩版的工具集合，包含两个相互独立的部分：

| 目录 | 类型 | 说明 |
|---|---|---|
| [`Prism-server/`](./Prism-server) | 🖥️ 服务器 | Prism OSS（phoenix-server）— 租赁服认证与账号管理 Web 平台 |
| [`Prism-toolbox/`](./Prism-toolbox) | 🧰 工具箱 | prism-toolbox-bot — 运行在 Android 上的机器人工具箱 |

> 两者同名为「Prism」，但属于完全不同的项目，各自独立，请勿混淆。

---

## 🖥️ Prism-server（服务器）

`prism-oss` 是 **Phoenix Server** 的开源版本：面向网易《我的世界》基岩版租赁服（Realms）的 **认证网关与账号管理平台**，附带一个 React + TypeScript 管理前端。

- **技术栈**：Go（后端，监听 `:8081`）+ React 19 + TypeScript + Vite（前端）
- **能力**：账号注册/登录/认证、游戏账号管理、文件与插件市场、支付与积分、租赁服列表/大厅、管理后台
- **协议**：GPL-3.0
- **来源**：内容来自公开开源的 `prism-oss`，**未做任何功能修改**

详见 [`Prism-server/README.md`](./Prism-server/README.md)。

---

## 🧰 Prism-toolbox（工具箱）

`prism-toolbox-bot` 是运行在 Android 设备上的《我的世界》基岩版 **机器人工具箱**。包含：

- `bot-apk/`：Go 编写的机器人服务端与 Android 壳（前端嵌入式）
- `flowers-for-machines/`：AGPL-3.0 协议依赖的本地副本（含 tanlobby 本地联机模块）

- **协议**：GNU AGPL-3.0
- **说明**：出于安全考虑，签名校验等内容不包含在内

详见 [`Prism-toolbox/README.md`](./Prism-toolbox/README.md)。

---

## 📄 许可

- `Prism-server/` — GPL-3.0，版权归原作者所有
- `Prism-toolbox/` — AGPL-3.0

两个子项目各自保留其原始许可协议，详见各目录下的 `LICENSE`。

---

## ⚠️ 声明

- 本仓库仅用于**学习研究**与**开源留存**。
- 请勿用于任何违反相关法律法规或服务条款的用途。
- 若需最新版本 / 提交 Issue，请优先联系各自的原项目。
