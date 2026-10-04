# Prism 核心验证 API 文档

Base URL: `https://example.com`

客户端进服完整流程，按顺序调用 4 个端点：

```
① /api/new → ② /api/phoenix/login → ③ /api/phoenix/transfer_start_type → ④ /api/phoenix/transfer_check_num
```

---

## 1. 创建会话

```
GET /api/new
```

客户端首次连接时调用，分配会话 ID。后续请求通过 `Authorization: Bearer <session_id>` 关联。

**请求**: 无需参数

**响应** (纯文本 UUID)
```
4c0fa62c-5313-e50b99-9758-1674280ef86e
```

**频率限制**: 10 次 / 10 秒

---

## 2. 游戏登录（进服）

```
POST /api/phoenix/login
```

认证用户身份，搜索租赁服并进入世界。

**请求体 (JSON)**
```json
{
  "server_code": "2353607",
  "login_token": "adb//xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "server_password": ""
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| server_code | string | 是 | 服务器房间号 |
| login_token | string | 否 | `adb//` Token / G79 Cookie JSON / 空（从 Authorization 头读取） |
| server_password | string | 否 | 服务器密码 |

**成功响应**
```json
{
  "success": true,
  "message": "ok",
  "server_msg": "",
  "token": "adb//xxx",
  "respond_to": "小蓝露s2",
  "ip_address": "xxx:14212",
  "uid": "3058311351",
  "username": "小蓝露s2",
  "growth_level": 0,
  "skin_info": {
    "entity_id": "4687475176737260112",
    "is_slim": true,
    "res_url": "https://x19.gph.netease.com/item_4687475176737260112_1_v1_6892qzjd.png"
  },
  "outfit_info": {},
  "chainInfo": "{\"chain\":[\"...\"]}",
  "verify": "xxx",
  "new_verify": "xxxxx
```

| 关键字段 | 说明 |
|----------|------|
| uid / username | 活跃游戏账号的 UID 和昵称 |
| skin_info | 当前皮肤信息（entity_id / res_url） |
| chainInfo | NetEase 认证链（JWT 链） |
| verify / new_verify | 新旧验证码 |
| ip_address | 分配的服务器地址 |

**频率限制**: 6 次 / 7 秒

**特殊值**:
- `GET /api/phoenix/login` → 健康检查
- `server_code = "::DRY::"` → 连通性测试

---

## 3. 启动传输（挑战任务 ①）

```
GET /api/phoenix/transfer_start_type?content=<encrypted>
```

获取加密的连接分配结果，黑洞模式在此步骤生效。

**请求头**: `Authorization: Bearer <session_id>`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| content | string | 是 | G79 协议加密的连接内容 |

**成功响应**
```json
{
  "success": true,
  "message": "ok",
  "data": "675d1d95a8b563d9a107a587a1e2660f...",
  "result": { "start_type": "675d1d95a8b563d9a107a587a1e2660f..." },
  "err": ""
}
```

| 字段 | 说明 |
|------|------|
| data | G79 加密的 UID + 解密 plaintext（客户端用于连接） |
| result.start_type | 同 data，兼容字段 |

**黑洞模式**: 网页开启后，服务端破坏 `data` 中第 6 个字节（`data[5] ^= 1`），客户端拿到损坏数据无法正常连接。

**频率限制**: 10 次 / 10 秒

---

## 4. 传输校验（挑战任务 ②）

```
POST /api/phoenix/transfer_check_num
```

验证客户端环境，生成校验码。

**请求头**: `Authorization: Bearer <session_id>`

**请求体 (JSON)**
```json
{
  "data": "<encrypted_data>",
  "engine_version": "3.8.25.293531",
  "patch_version": "3.8.33.294249",
  "is_pc": false
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| data | string | 是 | 加密的校验数据 |
| engine_version | string | 否 | 引擎版本（可从登录会话获取） |
| patch_version | string | 否 | 补丁版本 |
| is_pc | bool | 否 | 是否 PC 端 |

**成功响应**
```json
{
  "success": true,
  "message": "ok",
  "result": "<check_number>"
}
```

**错误响应**
```json
{ "success": false, "message": "checknum: bad data" }
```

**频率限制**: 10 次 / 10 秒

---

## 完整流程 + 实际测试结果

```
① curl https://example.com/api/new
   → 4c0fa62c-5313-e50b99-9758-1674280ef86e

② curl -X POST .../api/phoenix/login -d '{"server_code":"20245469","login_token":"adb//..."}'
   → {"success":true,"uid":"3058311351","username":"小蓝露s2","skin_info":{...},"chainInfo":"...","verify":"...","new_verify":"..."}

③ curl ".../api/phoenix/transfer_start_type?content=test123" -H "Authorization: Bearer <uuid>"
   → {"success":true,"data":"675d1d95a8b563d9...","result":{"start_type":"675d1d95a8b563d9..."}}

④ curl -X POST .../api/phoenix/transfer_check_num -H "Authorization: Bearer <uuid>" -d '{"data":"real_data","engine_version":"3.8.25.293531"}'
   → {"success":true,"result":"..."}
```

---

## 联机房间 API

所有联机房间接口需要先通过 `/api/new` 获取 session，并在 `Authorization` 头中携带。

### 5. 搜索房间

```
GET /api/phoenix/lobby/search?keyword=<房间号或关键词>
```

按关键词搜索联机大厅房间。

**请求头**: `Authorization: Bearer <session_id>`

**成功响应**
```json
{
  "ok": true,
  "code": 0,
  "total": "1",
  "rooms": [
    {
      "entity_id": "4683186036680040534",
      "room_name": "生存服",
      "slogan": "欢迎来玩",
      "password": "0",
      "res_id": "123456",
      "max_count": "10",
      "cur_num": "3",
      "owner_id": "3058311351",
      "owner_name": "小蓝露s2",
      "version": "1.21.90",
      "game_status": "1",
      "member_uids": ["3058311351", "1234567890"]
    }
  ]
}
```

### 6. 房间详情

```
GET /api/phoenix/lobby/room?room_id=<19位房间ID>
```

获取单个房间的详细信息（人数、房主、密码状态、地图等）。

**请求头**: `Authorization: Bearer <session_id>`

**成功响应**
```json
{
  "ok": true,
  "code": 0,
  "room": {
    "entity_id": "4683186036680040534",
    "room_name": "生存服",
    "slogan": "",
    "password": "0",
    "res_id": "123456",
    "max_count": "10",
    "cur_num": "3",
    "allow_save": "1",
    "visibility": "1",
    "owner_id": "3058311351",
    "version": "1.21.90",
    "world_id": "",
    "game_status": "1",
    "save_size": "1048576",
    "fids": [],
    "member_uids": []
  }
}
```

| 关键字段 | 说明 |
|----------|------|
| cur_num / max_count | 当前人数 / 最大人数 |
| password | `"0"` 无密码，非 `"0"` 有密码 |
| owner_id | 房主 UID |
| game_status | 房间状态（1=游戏中） |
| world_id | 存档世界 ID |

### 7. 按地图扫描房间

```
GET /api/phoenix/lobby/list?res_id=<商品/地图ID>
```

列出指定商品/地图下的所有联机房间（最多 10000 条）。

**请求头**: `Authorization: Bearer <session_id>`

**成功响应**
```json
{
  "ok": true,
  "code": 0,
  "total": "42",
  "rooms": [
    {
      "entity_id": "4683186036680040534",
      "room_name": "生存服",
      "password": "0",
      "res_id": "123456",
      "max_count": "10",
      "cur_num": "3",
      "owner_id": "3058311351",
      "version": "1.21.90",
      "game_status": "1"
    }
  ]
}
```

与搜索接口的区别：不需要关键词，按 res_id 批量拉取，适合"浏览该地图所有房间"场景。

---

## 8. 组件下载

```
GET /api/phoenix/download?item_id=<商品ID>
```

获取商城组件的下载地址（皮肤、地图、资源包等）。需要已购买才能成功返回。

**请求头**: `Authorization: Bearer <session_id>`

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| item_id | string | 是 | 19 位商品 ID，或分享链接（自动从末尾提取 19 位 ID） |

**成功响应**（已拥有该组件）
```json
{
  "ok": true,
  "code": 0,
  "message": "正常返回",
  "entity_id": "4687475176737260112",
  "res_url": "https://x19.gph.netease.com/item_4687475176737260112_1_v1_6892qzjd.png"
}
```

**失败响应**（未购买）
```json
{
  "ok": true,
  "code": 40,
  "message": "尚未购买成功",
  "entity_id": "4687804632142223267",
  "res_url": ""
}
```

**失败响应**（组件不存在）
```json
{
  "ok": true,
  "code": 10002,
  "message": "组件不存在",
  "entity_id": "",
  "res_url": ""
}
```

| 关键字段 | 说明 |
|----------|------|
| code | 0=成功, 40=未购买, 10002=组件不存在 |
| entity_id | 商品 ID（与请求 item_id 相同） |
| res_url | 实际下载地址（CNNIC CDN），失败时为空字符串 |
