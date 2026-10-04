# Prism 开发指南

## 1. 构建与部署

### 构建必须写对路径

```bash
# 后端：必须从 prism 目录构建，覆盖到 systemd 路径
cd /root/fa/prism
go build -o /root/fa/prism/cmd/phoenix_server/phoenix_server ./cmd/phoenix_server

# 前端：必须从 prism-web 目录构建
cd /root/fa/prism-web
npm run build            # tsc 类型检查 + Vite 打包 → dist/
```

**常见错误**：在 `/root/fa` 直接 `go build` 会失败（找不到 go module）。`npm run build` 同理。

### 部署后必须重启服务

```bash
systemctl restart phoenix-auth.service
```

systemd 服务配置位于：
- 主文件：`/etc/systemd/system/phoenix-auth.service`
- 代理覆盖：`/etc/systemd/system/phoenix-auth.service.d/proxy.conf`
- WorkingDirectory：`/root/fa/prism/cmd/phoenix_server`
- 二进制路径：`/root/fa/prism/cmd/phoenix_server/phoenix_server`

**注意**：`go build -o` 必须覆盖到上述路径，否则重启不生效。

---

## 2. 代理与 G79 认证

### SOCKS5 代理

G79 客户端通过 SOCKS5 代理访问网易 MCBE 服务器。代理通过环境变量 `HTTPS_PROXY` 配置。

- 代理端口：`127.0.0.1:1089`（ss-local 进程）
- 如果代理挂了，所有 G79 认证（Cookie 验证、租赁服登录等）都会失败
- 排查：`ss -tlnp | grep 1089` 确认 ss-local 存活

---

## 3. SQLite WAL 模式注意事项

### 数据库文件

- `phoenix.db` — 主数据文件
- `phoenix.db-wal` — WAL 日志
- `phoenix.db-shm` — 共享内存索引

### 重要：WAL 导致外部读取滞后

Go 进程使用 `DB.SetMaxOpenConns(1)` 单连接，写入通过 WAL 立即对同一连接可见。但**外部 `sqlite3` CLI 连接读不到未 checkpoint 的 WAL 数据**。

验证数据是否真正写入：
```bash
sqlite3 phoenix.db "PRAGMA wal_checkpoint; SELECT ..."
```

调试时调用 API 测试，不要直接用 sqlite3 CLI 验证。

---

## 4. Go 代码规范

### 编辑 Go 文件

直接使用文件编辑工具，**禁止用 Python 脚本修改 Go 源文件**（来自项目开发规范）。

### Tab 缩进

所有 `.go` 文件使用制表符（tab）缩进，编辑器不要转换。Edit 工具处理 tab 时可能失败，需要用 Bash Python 脚本或 sed 辅助。

### 结构体字段的 JSON 导出控制

- `json:"field_name"` — 导出
- `json:"-"` — 不导出（如 `CookieData`，安全原因）
- `json:"field_name,omitempty"` — 零值时省略

**典型错误**：在 API 中用了 `GameAccount` 直接序列化，导致 `CookieData` 被隐藏。需要用包装结构体暴露（如 `handleAdminAccounts` 的做法）。

### SQLite bool 扫描

`modernc.org/sqlite` 驱动可能不支持直接扫描 INTEGER 到 Go bool。用内部 `int` 字段中转：
```go
type Foo struct {
    Flag     bool `json:"flag"`
    _flagRaw int
}
// scan: &f._flagRaw, ...
// after scan: f.Flag = f._flagRaw != 0
```

### 数据库迁移

迁移写在 `internal/db/db.go` 的 `migrate()` 函数里。规则：
1. `ALTER TABLE` 用 `IF NOT EXISTS` 风格（通过捕获 "duplicate column name" 错误实现）
2. 有破坏性的迁移（去重等）要在 Go 代码里判断+处理
3. 新表用 `CREATE TABLE IF NOT EXISTS`

---

## 5. 前端规范

### TypeScript 类型

在 `api.ts` 里的函数参数类型必须和 API 实际接受的参数一致。加了新参数记得更新类型声明。

### React 受控组件

`<input>` 用 `checked` 或 `value` 属性就是受控组件，必须：
- 有对应的 state 驱动
- `onChange` 里更新 state

如果 API 调用是异步的，点击瞬间用本地 state（如 `togglingId`）临时覆盖，等 API 返回后再用真实数据替换。

### 全局 Toast

`showToast(msg, type)` 是全局函数，从 `./Toast` 导入即可使用。type 可以是 `'ok'` / `'err'` / `'info'`。

### 全局弹窗（App 级别）

需要全站覆盖的弹窗写在 `App.tsx` 的 Router 组件里，检测 `user` 状态来决定是否显示。

---

## 6. API 设计规范

### 限流

每个 API 端点应该有独立的 RateLimiter，同时被全局 `GlobalIPLimiter`（40req/s）兜底。

新加端点时至少要考虑是否需要独立的频控。

### 认证

- `SessionAuth` — 网页 session（phx_sid cookie）或 Bearer token
- `AdminAuth` — SessionAuth + role == "admin"
- 无需认证的端点（如 `/api/auth/activate`）不包 SessionAuth

### 审计日志

任何修改操作都应该写 `db.AddAuditLog`。前端 LogsPage 和 AdminPage 各有一个 `actionText` 翻译表，两端都要同步更新。

---

## 7. 账号模型要点

### 私有副本 + 共享副本

同一 UID 可以同时存在于私有池（`owner_id = user_id`）和共享池（`owner_id IS NULL`）。共享通过**复制行**实现，不是修改原行的 owner_id。

### 删除级联

`DeleteAccountCascade` — 删任何一个副本时，同时删除同 UID 的所有其他副本。用于前端和后台的"删除账号"操作。

`DeleteAccount` — 只删单行。用于"收回私有"（删除共享副本，保留私有副本）。

### can_reclaim 逻辑

- 私有账号：`can_reclaim = false`（已经私有，无需收回）
- 共享账号：`can_reclaim = (created_by == 当前用户ID)` — 只有最初共享出去的人才能收回

管理员通过后台面板（AdminPage）操作，不走 `can_reclaim`。

### 自动刷新开关

- `game_accounts.auto_refresh_enabled` 列，默认值基于 `source` 字段：
  - cookie/guest/web → 默认开启
  - phone/email → 默认关闭
- `AddAccount` 里设置默认值
- 刷新循环里检查 `acc.AutoRefreshEnabled`，关闭的跳过
- `nextRefresh` map 每 18 秒清理一次僵尸条目

---

## 8. 激活码系统

### 网页激活码（reusable）

存在 `activation_keys` 表，固定不变，可多人复用。当前码：`66ccf`。

添加/删除：
```sql
INSERT INTO activation_keys (code, note, created_at) VALUES ('XXXX', '备注', datetime('now'));
DELETE FROM activation_keys WHERE code = 'XXXX';
```

### 兑换码（one-time）

存在 `activation_codes` 表，一次性使用，兑换后获得板栗。通过后台生成。

### 激活流程

1. 每次 Phoenix 登录（进服）计数 +1
2. 超过 3 次且 `activated=false` → Phoenix 登录返回错误 + 网页登录返回 `require_activation`
3. 前端在 `App.tsx` 检测 `/api/auth/me` 返回的 `require_activation` 标志
4. 弹出全站覆盖的激活弹窗（含 QQ 群链接）
5. 用户输入激活码 → `POST /api/auth/activate` → 成功则标记 `activated=true`

---

## 9. 日志与排查

### 查看服务日志

```bash
journalctl -u phoenix-auth.service -f          # 实时
journalctl -u phoenix-auth.service --since "10 min ago"  # 最近10分钟
```

### 关键日志标记

- `[DB]` — 数据库初始化和迁移
- `[AUTH]` — 认证相关（登录、注册、激活）
- `[AUTO-REFRESH]` — 自动刷新调度
- `[LOGIN]` — Phoenix 登录流程（含 bearer/session 追踪）
- `[CHECKNUM]` — CheckNum 挑战码流程
- `[ACCOUNTS]` — 账号添加/删除
- `[ADMIN]` — 管理员操作

### 审计日志

所有修改操作写入 `audit_logs` 表。DB 中查：
```sql
SELECT * FROM audit_logs WHERE user_id=X ORDER BY created_at DESC;
```

---

## 10. 危险操作警示

1. **不要直接 `DELETE FROM game_accounts`** — 用 `DeleteAccount` 或 `DeleteAccountCascade`，会清理 `active_account_id` 引用
2. **不要手动改 `active_account_id`** — 用 `SetActiveAccount` / `ClearActiveAccount`
3. **不要在生产环境 `git push --force`** — 会丢失提交
4. **修改 shared pool 账号时注意** — 可能有其他用户正在使用它
5. **重启服务会清除所有 session** — 所有人需要重新登录
6. **proxy.conf 端口配错会导致 G79 认证全挂** — 改动后确认 `ss -tlnp | grep <端口>` 再重启
