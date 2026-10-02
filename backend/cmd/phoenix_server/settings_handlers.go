package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
	g79 "github.com/Yeah114/g79client"
	"log"
	"time"
	"golang.org/x/crypto/bcrypt"
)

func handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	if !nameChangeLimiter.Allow(requestIP(r)) {
		jsonResp(w, M{"ok": false, "error": "改名太频繁，请5分钟后再试"})
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		jsonResp(w, M{"ok": false, "error": "用户名不能为空"})
		return
	}
	if len(req.Username) > 20 {
		jsonResp(w, M{"ok": false, "error": "用户名不能超过20个字符"})
		return
	}
	if !validUsername.MatchString(req.Username) {
		jsonResp(w, M{"ok": false, "error": "用户名只能包含中文、英文、数字和下划线"})
		return
	}
	if taken, _ := db.IsUsernameTakenByOther(req.Username, user.ID); taken {
		jsonResp(w, M{"ok": false, "error": "哎呀,这个用户名已经被占用啦,换个名字试试~"})
		return
	}
	if err := db.UpdateUsername(user.ID, req.Username); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	_ = db.AddAuditLog(&user.ID, nil, "update_username", "web_user", req.Username, requestIP(r))
	jsonResp(w, M{"ok": true, "message": "用户名已更新"})
}

func handleUpdatePassword(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if len(req.NewPassword) < 6 {
		jsonResp(w, M{"ok": false, "error": "新密码至少6位"})
		return
	}
	fresh, err := db.GetUserByID(user.ID)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(fresh.PasswordHash), []byte(req.OldPassword)) != nil {
		jsonResp(w, M{"ok": false, "error": "糟糕,原密码不对。忘了密码就点「找回密码」重置~"})
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err := db.UpdatePassword(user.ID, string(hash)); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	// Rotate the primary token and drop all web sessions: any device/session
	// holding the old credentials is kicked off (prevents a hijacker who logged
	// in with the old password from keeping access after the owner rotates it).
	newToken, err := db.ResetToken(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	db.RevokeUserSessions(user.ID)
	_ = db.AddAuditLog(&user.ID, nil, "update_password", "web_user", "用户修改密码", requestIP(r))
	jsonResp(w, M{"ok": true, "message": "密码已更新，其他设备已全部下线", "token": newToken})
}

func handleResetToken(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	token, err := db.ResetToken(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	_ = db.AddAuditLog(&user.ID, nil, "reset_token", "web_user", "用户重置 adb token", requestIP(r))
	jsonResp(w, M{"ok": true, "token": token})
}

func handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/accounts/"), "/")
	var accountID int64
	fmt.Sscanf(parts[0], "%d", &accountID)
	acc, err := db.GetAccountByID(accountID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
		return
	}
	if acc.OwnerID != nil && *acc.OwnerID != user.ID && user.Role != "admin" {
		jsonResp(w, M{"ok": false, "error": "无权修改此账号"})
		return
	}
	if acc.OwnerID == nil && user.Role != "admin" && (acc.CreatedBy == nil || *acc.CreatedBy != user.ID) {
		jsonResp(w, M{"ok": false, "error": "只能收回自己创建的共享账号"})
		return
	}

	var req struct {
		DisplayName        *string `json:"display_name"`
		Shared             *bool   `json:"shared"`
		AutoRefreshEnabled *bool   `json:"auto_refresh_enabled"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if name != "" {
			db.UpdateAccountName(accountID, name)
		}
	}
	if req.Shared != nil && *req.Shared {
		// Clone to shared pool, keep private copy
		alreadyShared, _ := db.HasSharedCopy(acc.UID)
		if !alreadyShared {
			var nilOwner *int64
			dupAcc, err := db.AddAccount(nilOwner, acc.CookieData, db.AccountInfo{
				DisplayName: acc.DisplayName, UID: acc.UID, Status: acc.Status,
				GrowthLevel: acc.GrowthLevel, Score: acc.Score,
				SkinNumber: acc.SkinNumber, SkinURL: acc.SkinURL,
				CapeNumber: acc.CapeNumber, AvatarImageURL: acc.AvatarImageURL,
				IsVip: acc.IsVip, Source: acc.Source, CreatedBy: &user.ID,
			})
			if err == nil && dupAcc != nil {
				db.AddNuts(user.ID, 5, "share_account", nil, &dupAcc.ID)
			}
		}
	} else if req.Shared != nil && !*req.Shared {
		// 收回共享：删除共享行，扣除奖励
		// 原始账号取消共享时也要找共享副本扣分
		targetID := accountID
		if acc.OwnerID != nil {
			if shared, err := db.GetSharedAccountByUID(acc.UID); err == nil && shared != nil {
				targetID = shared.ID
			}
		}
		if targetID > 0 {
			db.DeleteAccount(targetID)
			db.AddNuts(user.ID, -5, "unshare_account", nil, &targetID)
		}
	}
	if req.AutoRefreshEnabled != nil {
		db.SetAutoRefresh(accountID, *req.AutoRefreshEnabled)
		if *req.AutoRefreshEnabled {
			if cached := getCachedClient(accountID); cached != nil {
				startAccountHeartbeat(accountID, cached)
			} else if cc := getHeartbeatClient(accountID); cc != nil {
				startAccountHeartbeat(accountID, cc)
			}
		} else {
			stopAccountHeartbeat(accountID)
		}
	}
	_ = db.AddAuditLog(&user.ID, &accountID, "update_account", "game_account", acc.DisplayName+" ("+acc.UID+")", requestIP(r))
	jsonResp(w, M{"ok": true, "message": "账号已更新"})
}

// --- POST /api/admin/accounts/{id}/clone (admin) ---
func handleAdminCloneAccount(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil || user.Role != "admin" {
		jsonResp(w, M{"ok": false, "error": "仅管理员可操作"})
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/admin/accounts/"), "/")
	var accountID int64
	fmt.Sscanf(parts[0], "%d", &accountID)
	if accountID == 0 {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_id", "糟糕,账号编号无效或已失效,请刷新后重试~")})
		return
	}
	src, err := db.GetAccountByID(accountID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
		return
	}
	ownerID := user.ID
	newAcc, err := db.AddAccount(&ownerID, src.CookieData, db.AccountInfo{
		DisplayName: src.DisplayName, UID: src.UID, Status: src.Status,
		GrowthLevel: src.GrowthLevel, Score: src.Score,
		SkinNumber: src.SkinNumber, SkinURL: src.SkinURL,
		CapeNumber: src.CapeNumber, AvatarImageURL: src.AvatarImageURL,
		IsVip: src.IsVip, Source: src.Source,
	})
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "克隆失败: " + err.Error()})
		return
	}
	log.Printf("[ADMIN] Cloned shared account %d to private account %d for admin %d", accountID, newAcc.ID, user.ID)
	_ = db.AddAuditLog(&user.ID, &newAcc.ID, "admin_clone_account", "game_account",
		fmt.Sprintf("管理员从共享池克隆账号 %s (%s)", src.DisplayName, src.UID), requestIP(r))
	jsonResp(w, M{"ok": true, "message": "已复制到私有账号", "account_id": newAcc.ID})
}

func handleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	accs, err := db.ListAllAccountsForAdmin()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	type adminAccount struct {
		*db.GameAccount
		CookieData string `json:"cookie_data"`
	}
	var out []adminAccount
	for _, a := range accs {
		out = append(out, adminAccount{GameAccount: a, CookieData: a.CookieData})
	}
	jsonResp(w, M{"ok": true, "accounts": out})
}

func handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/admin/users/"), "/")
	var userID int64
	fmt.Sscanf(parts[0], "%d", &userID)
	authUser := auth.GetUser(r.Context())
	var req struct {
		Role        *string `json:"role"`
		Email       *string `json:"email"`
		Username    *string `json:"username"`
		Password    *string `json:"password"`
		Disabled    *bool   `json:"disabled"`
		TestAccount *bool   `json:"test_account"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Role != nil {
		if *req.Role != "admin" && *req.Role != "user" {
			jsonResp(w, M{"ok": false, "error": "角色无效"})
			return
		}
		if err := db.SetUserRole(userID, *req.Role); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
	}
	if req.Disabled != nil {
		if err := db.SetUserDisabled(userID, *req.Disabled); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
	}
	if req.TestAccount != nil {
		if err := db.SetUserTestAccount(userID, *req.TestAccount); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		_ = db.AddAuditLog(&authUser.ID, nil, "admin_update_user", "web_user", fmt.Sprintf("test_account set to %v for user %d", *req.TestAccount, userID), requestIP(r))
	}
	if req.Email != nil {
		email := strings.TrimSpace(strings.ToLower(*req.Email))
		if email != "" {
			if exists, _ := db.IsEmailRegistered(email); exists {
				jsonResp(w, M{"ok": false, "error": "该邮箱已被使用"})
				return
			}
			if err := db.UpdateEmail(userID, email); err != nil {
				jsonResp(w, M{"ok": false, "error": err.Error()})
				return
			}
			_ = db.AddAuditLog(&authUser.ID, nil, "admin_update_user", "web_user", fmt.Sprintf("email updated for user %d", userID), requestIP(r))
		}
	}
	if req.Username != nil {
		username := strings.TrimSpace(*req.Username)
		if username != "" {
			if taken, _ := db.IsUsernameTakenByOther(username, userID); taken {
				jsonResp(w, M{"ok": false, "error": "哎呀,这个用户名已经被占用啦,换个名字试试~"})
				return
			}
			if err := db.UpdateUsername(userID, username); err != nil {
				jsonResp(w, M{"ok": false, "error": err.Error()})
				return
			}
			_ = db.AddAuditLog(&authUser.ID, nil, "admin_update_user", "web_user", fmt.Sprintf("username updated for user %d", userID), requestIP(r))
		}
	}
	if req.Password != nil {
		if len(*req.Password) >= 6 {
			hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
			if err == nil {
				if err := db.UpdatePassword(userID, string(hash)); err != nil {
					jsonResp(w, M{"ok": false, "error": err.Error()})
					return
				}
				_ = db.AddAuditLog(&authUser.ID, nil, "admin_update_user", "web_user", fmt.Sprintf("password reset for user %d", userID), requestIP(r))
			}
		}
	}
	jsonResp(w, M{"ok": true})
}

func handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/admin/users/"), "/")
	var userID int64
	fmt.Sscanf(parts[0], "%d", &userID)
	if err := db.DeleteUser(userID); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	authUser := auth.GetUser(r.Context())
	if authUser != nil {
		_ = db.AddAuditLog(&authUser.ID, nil, "admin_delete_user", "web_user", fmt.Sprintf("管理员删除用户 #%d", userID), requestIP(r))
	}
	jsonResp(w, M{"ok": true})
}

func handleAdminUpdateAccount(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/admin/accounts/"), "/")
	var accountID int64
	fmt.Sscanf(parts[0], "%d", &accountID)
	var req struct {
		DisplayName *string `json:"display_name"`
		Shared      *bool   `json:"shared"`
		OwnerID     *int64  `json:"owner_id"`
		Disabled    *bool   `json:"disabled"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if name != "" {
			if err := db.UpdateAccountName(accountID, name); err != nil {
				jsonResp(w, M{"ok": false, "error": err.Error()})
				return
			}
		}
	}
	if req.Shared != nil {
		ownerID := int64(0)
		if req.OwnerID != nil {
			ownerID = *req.OwnerID
		}
		if err := db.SetAccountShared(accountID, *req.Shared, ownerID); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
	}
	if req.Disabled != nil {
		if err := db.SetAccountDisabled(accountID, *req.Disabled); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
	}
	jsonResp(w, M{"ok": true})
}

// parseLogFilter extracts the shared search/filter params from a log request.
func parseLogFilter(r *http.Request) db.LogFilter {
	q := r.URL.Query()
	f := db.LogFilter{
		Keyword:  strings.TrimSpace(q.Get("keyword")),
		Action:   strings.TrimSpace(q.Get("action")),
		DateFrom: strings.TrimSpace(q.Get("date_from")),
		DateTo:   strings.TrimSpace(q.Get("date_to")),
	}
	if idStr := strings.TrimSpace(q.Get("user_id")); idStr != "" {
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil && id > 0 {
			f.UserID = &id
		}
	}
	return f
}

func handleMyAuditLogs(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	f := parseLogFilter(r)
	logs, err := db.ListAuditLogs(&user.ID, limit, offset, f)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	total, _ := db.CountAuditLogs(&user.ID, f)
	jsonResp(w, M{"ok": true, "logs": logs, "total": total})
}

func handleAdminAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	f := parseLogFilter(r)
	logs, err := db.ListAuditLogs(nil, limit, offset, f)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	total, _ := db.CountAuditLogs(nil, f)
	jsonResp(w, M{"ok": true, "logs": logs, "total": total})
}

func handleAdminSystemLogs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	f := parseLogFilter(r)
	logs, err := db.ListSystemLogs(limit, offset, f)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	total, _ := db.CountSystemLogs(f)
	jsonResp(w, M{"ok": true, "logs": logs, "total": total})
}

func requestIP(r *http.Request) string {
	// 安全取客户端真实 IP：绝不能信任客户端可任意伪造的首个 X-Forwarded-For 项
	// （攻击者每请求换一个伪造 IP 即可绕过基于 IP 的限流，形成无限攻击）。
	// 可信来源顺序：
	//   1. X-Real-IP —— 由反向代理(NX)覆盖为 $remote_addr；
	//   2. X-Forwarded-For 末项 —— NX 用 $proxy_add_x_forwarded_for 会把真实 IP 追加在末尾；
	//   3. TCP 对端 RemoteAddr —— 兜底。
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if fwd := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); fwd != "" {
		parts := strings.Split(fwd, ",")
		if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
			return ip
		}
	}
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}
	return r.RemoteAddr
}


// ─── POST /api/admin/accounts/add ───
func handleAdminAddAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Cookie      string `json:"cookie"`
		OwnerID     *int64 `json:"owner_id"`
		Shared      bool   `json:"shared"`
		DisplayName string `json:"display_name"`
		UID         string `json:"uid"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Determine owner
	var ownerID *int64
	if req.Shared {
		ownerID = nil
	} else if req.OwnerID != nil {
		ownerID = req.OwnerID
	} else {
		ownerID = nil // default to shared pool if no owner specified
	}

	// Parse and validate cookie or use manual fields
	var info *db.AccountInfo
	var cookieData string
	var displayName, uid string

	if req.Cookie != "" {
		cookieData = req.Cookie
		var err error
		info, err = db.ParseAccountInfo(req.Cookie)
		if err != nil {
			info = &db.AccountInfo{}
		}
		// Try to validate with g79 to get full details
		client, g79Err := g79.NewClient()
		if g79Err == nil {
			if authErr := client.G79AuthenticateWithCookie(req.Cookie); authErr == nil {
				ud, udErr := client.GetUserDetail()
				if udErr == nil {
					client.UserDetail = &ud.Entity
					fillAccountInfoFromClient(client, info)
				}
			}
		}
		if info.DisplayName != "" {
			displayName = info.DisplayName
		} else if req.DisplayName != "" {
			displayName = req.DisplayName
		} else {
			displayName = "admin_added_" + fmt.Sprintf("%x", time.Now().Unix())
		}
		if info.UID != "" {
			uid = info.UID
		} else if req.UID != "" {
			uid = req.UID
		}
	} else {
		cookieData = ""
		displayName = req.DisplayName
		uid = req.UID
		info = &db.AccountInfo{
			DisplayName: req.DisplayName,
			UID:         req.UID,
			Status:      "normal",
		}
	}

	if displayName == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 display_name 或有效 cookie"})
		return
	}

	info.DisplayName = displayName
	info.UID = uid
	info.Source = "admin"
	info.CreatedBy = nil
	if info.Status == "" {
		info.Status = "normal"
	}

	acc, err := db.AddAccount(ownerID, cookieData, *info)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	// 立即分配代理
	AssignProxy(acc.ID)
	_ = db.AddAuditLog(nil, nil, "admin_add_account", "game_account", displayName+" ("+uid+")", requestIP(r))
	log.Printf("[ADMIN] Account added: %s (%s) owner=%v", displayName, uid, ownerID)
	jsonResp(w, M{"ok": true, "message": "账号已添加", "account": M{
		"id": acc.ID, "display_name": acc.DisplayName, "uid": acc.UID,
		"status": acc.Status, "owner_id": acc.OwnerID,
	}})
}


// ─── POST /api/admin/users/add ───
func handleAdminAddUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Username = strings.TrimSpace(req.Username)
	if req.Email == "" || req.Username == "" || req.Password == "" {
		jsonResp(w, M{"ok": false, "error": "邮箱、用户名、密码不能为空"})
		return
	}
	if len(req.Password) < 6 {
		jsonResp(w, M{"ok": false, "error": "抱歉,密码至少需要 6 位,请重新设置~"})
		return
	}
	exists, _ := db.IsEmailRegistered(req.Email)
	if exists {
		jsonResp(w, M{"ok": false, "error": "哎呀,这个邮箱已经注册过了,换个邮箱试试~"})
		return
	}
	taken, _ := db.IsUsernameTaken(req.Username)
	if taken {
		jsonResp(w, M{"ok": false, "error": "哎呀,这个用户名已经被占用啦,换个名字试试~"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("system_error", "哎呀,系统开小差了,请稍后重试;仍不行请联系管理员~")})
		return
	}
	user, err := db.CreateVerifiedUser(req.Email, req.Username, string(hash))
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	_ = db.AddAuditLog(nil, nil, "admin_add_user", "web_user", req.Email+" ("+req.Username+")", requestIP(r))
	log.Printf("[ADMIN] User created: %s (%s)", req.Email, req.Username)
	jsonResp(w, M{"ok": true, "message": "用户已创建", "user": M{
		"id": user.ID, "email": user.Email, "username": user.Username,
		"role": user.Role, "token": user.Token,
	}})
}


// ─── GET /api/realname-presets ───
func handleRealnamePresets(w http.ResponseWriter, r *http.Request) {
	presets, err := db.GetRealnamePresets()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "presets": presets})
}

// ─── POST /api/admin/realname-presets/add ───
func handleAdminAddPreset(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Name     string `json:"name"`
		IDNumber string `json:"id_number"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	req.Name = strings.TrimSpace(req.Name)
	req.IDNumber = strings.TrimSpace(req.IDNumber)
	if req.Name == "" || req.IDNumber == "" {
		jsonResp(w, M{"ok": false, "error": "姓名和身份证号不能为空"})
		return
	}
	preset, err := db.AddRealnamePreset(req.Name, req.IDNumber, true)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	adminUser := auth.GetUser(r.Context())
	log.Printf("[ADMIN] %s 添加实名预设: %s", adminUser.Username, req.Name)
	_ = db.AddAuditLog(&adminUser.ID, nil, "admin_add_preset", "realname", req.Name, requestIP(r))
	jsonResp(w, M{"ok": true, "preset": preset})
}

// ─── DELETE /api/admin/realname-presets/{id} ───
func handleAdminDeletePreset(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/admin/realname-presets/"), "/")
	var id int64
	fmt.Sscanf(parts[0], "%d", &id)
	if id == 0 {
		jsonResp(w, M{"ok": false, "error": "无效的ID"})
		return
	}
	if err := db.DeleteRealnamePreset(id); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	adminUser := auth.GetUser(r.Context())
	log.Printf("[ADMIN] %s 删除实名预设 #%d", adminUser.Username, id)
	_ = db.AddAuditLog(&adminUser.ID, nil, "admin_delete_preset", "realname", fmt.Sprintf("删除预设 #%d", id), requestIP(r))
	jsonResp(w, M{"ok": true})
}
