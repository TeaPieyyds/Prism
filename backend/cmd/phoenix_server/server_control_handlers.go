package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
)

// getActiveAccount 获取当前用户的活跃游戏账号。
func getActiveAccount(user *db.User) *db.GameAccount {
	if user == nil || user.ActiveAccountID == nil {
		return nil
	}
	acc, err := db.GetAccountByID(*user.ActiveAccountID)
	if err != nil || acc == nil || acc.Disabled {
		return nil
	}
	return acc
}

// ── 头像相关 ──

// handleAvatarList 获取可用头像列表。
func handleAvatarList(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	// 优先用 account_id，否则用活跃账号
	var acc *db.GameAccount
	if aidStr := r.FormValue("account_id"); aidStr != "" {
		if aid, err := strconv.ParseInt(aidStr, 10, 64); err == nil && aid > 0 {
			acc, err = db.GetAccountByID(aid)
			if err != nil {
				jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
				return
			}
			if acc.OwnerID != nil && *acc.OwnerID != user.ID && user.Role != "admin" {
				jsonResp(w, M{"ok": false, "error": "无权操作此账号"})
				return
			}
			if acc.OwnerID == nil && user.Role != "admin" {
				jsonResp(w, M{"ok": false, "error": "无权操作共享账号"})
				return
			}
		}
	}
	if acc == nil {
		acc = getActiveAccount(user)
		if acc == nil {
			jsonResp(w, M{"ok": false, "error": msg.Get("no_active_account", "哎呀,当前没有可用的活跃账号,请先添加或启用账号~")})
			return
		}
	}
	if acc.CookieData == "" {
		jsonResp(w, M{"ok": false, "error": "该账号没有 Cookie"})
		return
	}

	client, err := newG79ClientWithProxy(acc.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if err := client.G79AuthenticateWithCookie(acc.CookieData); err != nil {
		jsonResp(w, M{"ok": false, "error": "认证失败: " + err.Error()})
		return
	}

	resp, err := client.GetUserHeadURLs()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "entities": resp.Entities})
}

// handleAvatarChange 更改用户头像。
func handleAvatarChange(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}

	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	var req struct {
		HeadImage string `json:"head_image"`
		AccountID *int64 `json:"account_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
		log.Printf("[AVATAR] change request: head_image=%q account_id=%v", req.HeadImage, req.AccountID)
	if req.HeadImage == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 head_image"})
		return
	}

	// 确定要操作的账号：优先用 account_id，否则用活跃账号
	var acc *db.GameAccount
	if req.AccountID != nil {
		var err error
		acc, err = db.GetAccountByID(*req.AccountID)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
			return
		}
		// 校验权限：私有账号必须是自己拥有的，共享账号管理员可操作
		if acc.OwnerID != nil && *acc.OwnerID != user.ID && user.Role != "admin" {
			jsonResp(w, M{"ok": false, "error": "无权操作此账号"})
			return
		}
		if acc.OwnerID == nil && user.Role != "admin" {
			jsonResp(w, M{"ok": false, "error": "无权操作共享账号"})
			return
		}
	} else {
		acc = getActiveAccount(user)
		if acc == nil {
			jsonResp(w, M{"ok": false, "error": msg.Get("no_active_account", "哎呀,当前没有可用的活跃账号,请先添加或启用账号~")})
			return
		}
	}
	if acc.CookieData == "" {
		jsonResp(w, M{"ok": false, "error": "该账号没有 Cookie"})
		return
	}

	client, err := newG79ClientWithProxy(acc.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if err := client.G79AuthenticateWithCookie(acc.CookieData); err != nil {
		jsonResp(w, M{"ok": false, "error": "认证失败: " + err.Error()})
		return
	}

	resp, err := client.SetUserHead(req.HeadImage, 0)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	// 同步更新本地数据库头像URL
	db.DB.Exec(`UPDATE game_accounts SET avatar_image_url = ?, updated_at = ? WHERE id = ?`,
		resp.Entity.HeadImage, time.Now().UTC().Format(time.RFC3339), acc.ID)
	log.Printf("[AVATAR] 账号 #%d 更换头像: %s (冷却: %ds)", acc.ID, resp.Entity.HeadImage, resp.Entity.HeadImageCD)
	jsonResp(w, M{"ok": true, "head_image": resp.Entity.HeadImage, "head_image_cd": resp.Entity.HeadImageCD})
}

// handleAvatarUpload 上传自定义图片并设为头像。
// POST /api/avatar/upload  multipart/form-data  field: file
func handleAvatarUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20) // 头像上传放宽到 8MB（覆盖全局 1MB 默认）

	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	// 优先用 account_id，否则用活跃账号
	var acc *db.GameAccount
	if aidStr := r.FormValue("account_id"); aidStr != "" {
		if aid, err := strconv.ParseInt(aidStr, 10, 64); err == nil && aid > 0 {
			acc, err = db.GetAccountByID(aid)
			if err != nil {
				jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
				return
			}
			if acc.OwnerID != nil && *acc.OwnerID != user.ID && user.Role != "admin" {
				jsonResp(w, M{"ok": false, "error": "无权操作此账号"})
				return
			}
			if acc.OwnerID == nil && user.Role != "admin" {
				jsonResp(w, M{"ok": false, "error": "无权操作共享账号"})
				return
			}
		}
	}
	if acc == nil {
		acc = getActiveAccount(user)
		if acc == nil {
			jsonResp(w, M{"ok": false, "error": msg.Get("no_active_account", "哎呀,当前没有可用的活跃账号,请先添加或启用账号~")})
			return
		}
	}
	if acc.CookieData == "" {
		jsonResp(w, M{"ok": false, "error": "该账号没有 Cookie"})
		return
	}

	client, err := newG79ClientWithProxy(acc.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if err := client.G79AuthenticateWithCookie(acc.CookieData); err != nil {
		jsonResp(w, M{"ok": false, "error": "认证失败: " + err.Error()})
		return
	}

	// 1. Get upload token
	tokenResp, err := client.GetImageUploadToken()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "获取上传令牌失败: " + err.Error()})
		return
	}

	// 2. Read uploaded file
	r.ParseMultipartForm(20 << 20) // 20MB max
	file, _, err := r.FormFile("file")
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "读取上传文件失败: " + err.Error()})
		return
	}
	defer file.Close()

	fileData, err := io.ReadAll(file)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "读取文件数据失败: " + err.Error()})
		return
	}

	// 3. Upload to FP server
	fpResp, err := client.UploadFileToFP(tokenResp.Entity.URL, tokenResp.Entity.Token, fileData)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "上传文件失败: " + err.Error()})
		return
	}

	// 4. Set as avatar
	headResp, err := client.SetUserHead(fpResp.URL, 0)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "设置头像失败: " + err.Error()})
		return
	}
	// 同步更新本地数据库头像URL
	db.DB.Exec(`UPDATE game_accounts SET avatar_image_url = ?, updated_at = ? WHERE id = ?`,
		headResp.Entity.HeadImage, time.Now().UTC().Format(time.RFC3339), acc.ID)
	log.Printf("[AVATAR] 用户上传自定义头像: %s (冷却: %ds)", fpResp.URL, headResp.Entity.HeadImageCD)
	jsonResp(w, M{"ok": true, "head_image": headResp.Entity.HeadImage, "head_image_cd": headResp.Entity.HeadImageCD, "upload": fpResp})
}

// ── 租赁服管理 ──

// handleRentalServerList 查询我的租赁服列表。
func handleRentalServerList(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	// 优先用 account_id，否则用活跃账号
	var acc *db.GameAccount
	if aidStr := r.FormValue("account_id"); aidStr != "" {
		if aid, err := strconv.ParseInt(aidStr, 10, 64); err == nil && aid > 0 {
			acc, err = db.GetAccountByID(aid)
			if err != nil {
				jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
				return
			}
			if acc.OwnerID != nil && *acc.OwnerID != user.ID && user.Role != "admin" {
				jsonResp(w, M{"ok": false, "error": "无权操作此账号"})
				return
			}
			if acc.OwnerID == nil && user.Role != "admin" {
				jsonResp(w, M{"ok": false, "error": "无权操作共享账号"})
				return
			}
		}
	}
	if acc == nil {
		acc = getActiveAccount(user)
		if acc == nil {
			jsonResp(w, M{"ok": false, "error": msg.Get("no_active_account", "哎呀,当前没有可用的活跃账号,请先添加或启用账号~")})
			return
		}
	}
	if acc.CookieData == "" {
		jsonResp(w, M{"ok": false, "error": "该账号没有 Cookie"})
		return
	}

	client, err := newG79ClientWithProxy(acc.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if err := client.G79AuthenticateWithCookie(acc.CookieData); err != nil {
		jsonResp(w, M{"ok": false, "error": "认证失败: " + err.Error()})
		return
	}

	resp, err := client.SearchMyRentalServers(10, 0)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "entities": resp.Entities})
}

// handleRentalServerStatus 查询租赁服状态。
func handleRentalServerStatus(w http.ResponseWriter, r *http.Request) {
	serverID := r.URL.Query().Get("server_id")
	if serverID == "" {
		jsonResp(w, M{"ok": false, "error": msg.Get("missing_id", "糟糕,缺少必要的服务器信息,请刷新后重试~")})
		return
	}

	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	// 优先用 account_id，否则用活跃账号
	var acc *db.GameAccount
	if aidStr := r.FormValue("account_id"); aidStr != "" {
		if aid, err := strconv.ParseInt(aidStr, 10, 64); err == nil && aid > 0 {
			acc, err = db.GetAccountByID(aid)
			if err != nil {
				jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
				return
			}
			if acc.OwnerID != nil && *acc.OwnerID != user.ID && user.Role != "admin" {
				jsonResp(w, M{"ok": false, "error": "无权操作此账号"})
				return
			}
			if acc.OwnerID == nil && user.Role != "admin" {
				jsonResp(w, M{"ok": false, "error": "无权操作共享账号"})
				return
			}
		}
	}
	if acc == nil {
		acc = getActiveAccount(user)
		if acc == nil {
			jsonResp(w, M{"ok": false, "error": msg.Get("no_active_account", "哎呀,当前没有可用的活跃账号,请先添加或启用账号~")})
			return
		}
	}
	if acc.CookieData == "" {
		jsonResp(w, M{"ok": false, "error": "该账号没有 Cookie"})
		return
	}

	client, err := newG79ClientWithProxy(acc.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if err := client.G79AuthenticateWithCookie(acc.CookieData); err != nil {
		jsonResp(w, M{"ok": false, "error": "认证失败: " + err.Error()})
		return
	}

	resp, err := client.GetRentalServerStatus(serverID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "entity": resp.Entity})
}

// handleRentalServerControl 启停租赁服。
func handleRentalServerControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}

	var req struct {
		ServerID  string `json:"server_id"`
		Status    int    `json:"status"`
		AccountID *int64 `json:"account_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.ServerID == "" {
		jsonResp(w, M{"ok": false, "error": msg.Get("missing_id", "糟糕,缺少必要的服务器信息,请刷新后重试~")})
		return
	}

	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	// 优先用请求中的 account_id，否则用活跃账号
	var acc *db.GameAccount
	if req.AccountID != nil && *req.AccountID > 0 {
		aid := *req.AccountID
		var err error
		acc, err = db.GetAccountByID(aid)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
			return
		}
		if acc.OwnerID != nil && *acc.OwnerID != user.ID && user.Role != "admin" {
			jsonResp(w, M{"ok": false, "error": "无权操作此账号"})
			return
		}
		if acc.OwnerID == nil && user.Role != "admin" {
			jsonResp(w, M{"ok": false, "error": "无权操作共享账号"})
			return
		}
	}
	if acc == nil {
		acc = getActiveAccount(user)
		if acc == nil {
			jsonResp(w, M{"ok": false, "error": msg.Get("no_active_account", "哎呀,当前没有可用的活跃账号,请先添加或启用账号~")})
			return
		}
	}
	if acc.CookieData == "" {
		jsonResp(w, M{"ok": false, "error": "该账号没有 Cookie"})
		return
	}

	client, err := newG79ClientWithProxy(acc.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if err := client.G79AuthenticateWithCookie(acc.CookieData); err != nil {
		jsonResp(w, M{"ok": false, "error": "认证失败: " + err.Error()})
		return
	}

	action := "关闭"
	if req.Status == 1 {
		action = "启动"
	}
	if _, err := client.ControlRentalServer(req.ServerID, req.Status); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	log.Printf("[RENTAL] %s租赁服 %s", action, req.ServerID)
	jsonResp(w, M{"ok": true, "message": action + "指令已发送"})
}

// handleRentalServerUpdate 更新租赁服信息（公告、名称等）。
func handleRentalServerUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}

	var req map[string]any
	json.NewDecoder(r.Body).Decode(&req)
	if req == nil {
		jsonResp(w, M{"ok": false, "error": "无效请求体"})
		return
	}
	serverID, ok := req["server_id"].(string)
	if !ok || serverID == "" {
		jsonResp(w, M{"ok": false, "error": msg.Get("missing_id", "糟糕,缺少必要的服务器信息,请刷新后重试~")})
		return
	}

	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	// 优先用 account_id，否则用活跃账号
	var acc *db.GameAccount
	if aidStr := r.FormValue("account_id"); aidStr != "" {
		if aid, err := strconv.ParseInt(aidStr, 10, 64); err == nil && aid > 0 {
			acc, err = db.GetAccountByID(aid)
			if err != nil {
				jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
				return
			}
			if acc.OwnerID != nil && *acc.OwnerID != user.ID && user.Role != "admin" {
				jsonResp(w, M{"ok": false, "error": "无权操作此账号"})
				return
			}
			if acc.OwnerID == nil && user.Role != "admin" {
				jsonResp(w, M{"ok": false, "error": "无权操作共享账号"})
				return
			}
		}
	}
	if acc == nil {
		acc = getActiveAccount(user)
		if acc == nil {
			jsonResp(w, M{"ok": false, "error": msg.Get("no_active_account", "哎呀,当前没有可用的活跃账号,请先添加或启用账号~")})
			return
		}
	}
	if acc.CookieData == "" {
		jsonResp(w, M{"ok": false, "error": "该账号没有 Cookie"})
		return
	}

	client, err := newG79ClientWithProxy(acc.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if err := client.G79AuthenticateWithCookie(acc.CookieData); err != nil {
		jsonResp(w, M{"ok": false, "error": "认证失败: " + err.Error()})
		return
	}

	resp, err := client.UpdateRentalServer(serverID, req)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	log.Printf("[RENTAL] 更新租赁服 %s 信息", serverID)
	jsonResp(w, M{"ok": true, "entity": resp.Entity})
}
