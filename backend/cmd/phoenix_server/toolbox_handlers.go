package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	g79 "github.com/Yeah114/g79client"
	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
)

// ── Public v2 API ──

func handleVersionCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Version       string `json:"version"`
		VersionCode   int    `json:"version_code"`
		Platform      string `json:"platform"`
		ServerCode    string `json:"server_code"`
		SignatureHash string `json:"signature_hash"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	cfg, _ := db.GetAppVersionConfig()
	resp := M{
		"ok":                  true,
		"latest_version":      cfg.LatestVersion,
		"latest_version_code": cfg.LatestVersionCode,
		"force_update":        cfg.ForceUpdate,
		"update_url":          cfg.UpdateURL,
		"update_message":      cfg.UpdateMessage,
		"signature_valid":     true,
		"signature_message":   "",
	}

	// 始终比对白名单（不因签名为空就放行）。
	// 空/伪造/被清空的签名 → 不匹配白名单 → 判定无效，杜绝"不上报签名就绕过"。
	// com.prismtool.box 已在白名单登记，空哈希会命中 false。
	valid, _, msg := db.VerifySignature("com.prismtool.box", req.SignatureHash)
	resp["signature_valid"] = valid
	resp["signature_message"] = msg

	jsonResp(w, resp)
}

func handleAnnouncements(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Version     string `json:"version"`
		VersionCode int    `json:"version_code"`
		ServerCode  string `json:"server_code"`
		LastSeenID  string `json:"last_seen_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	anns, err := db.GetActiveAnnouncements(req.VersionCode, req.ServerCode)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}

	// Filter by last_seen_id
	filtered := anns
	if req.LastSeenID != "" {
		filtered = nil
		for _, a := range anns {
			if a.ID > req.LastSeenID {
				filtered = append(filtered, a)
			}
		}
	}

	if filtered == nil {
		filtered = []db.Announcement{}
	}
	jsonResp(w, M{"ok": true, "announcements": filtered})
}

func handleSignatureVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Version       string `json:"version"`
		VersionCode   int    `json:"version_code"`
		SignatureHash string `json:"signature_hash"`
		PackageName   string `json:"package_name"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	valid, knownHash, msg := db.VerifySignature(req.PackageName, req.SignatureHash)
	jsonResp(w, M{
		"ok":         true,
		"valid":      valid,
		"known_hash": knownHash,
		"message":    msg,
	})
}

// ── Admin: Version Config ──

func handleAdminVersionConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		cfg, _ := db.GetAppVersionConfig()
		jsonResp(w, M{"ok": true, "config": cfg})
		return
	}
	if r.Method == "POST" {
		adminUser := auth.GetUser(r.Context())
		var req db.AppVersionConfig
		json.NewDecoder(r.Body).Decode(&req)
		if err := db.UpdateAppVersionConfig(&req); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		log.Printf("[TOOLBOX] %s 更新版本配置: v%s (code=%d force=%v)", adminUser.Username, req.LatestVersion, req.LatestVersionCode, req.ForceUpdate)
		db.AddAuditLog(&adminUser.ID, nil, "admin_update_version", "toolbox", fmt.Sprintf("版本 %s code=%d", req.LatestVersion, req.LatestVersionCode), requestIP(r))
		jsonResp(w, M{"ok": true})
		return
	}
}

// ── Admin: Announcements CRUD ──

func handleAdminAnnouncements(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		list, err := db.ListAllAnnouncements()
		if err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		jsonResp(w, M{"ok": true, "announcements": list})
		return
	}
	if r.Method == "POST" {
		adminUser := auth.GetUser(r.Context())
		var a db.Announcement
		json.NewDecoder(r.Body).Decode(&a)
		if a.Title == "" {
			jsonResp(w, M{"ok": false, "error": "标题不能为空"})
			return
		}
		if err := db.CreateAnnouncement(&a); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		log.Printf("[TOOLBOX] %s 创建公告: %s", adminUser.Username, a.Title)
		db.AddAuditLog(&adminUser.ID, nil, "admin_create_announcement", "toolbox", fmt.Sprintf("公告: %s", a.Title), requestIP(r))
		jsonResp(w, M{"ok": true, "announcement": a})
		return
	}
}

func handleAdminAnnouncementItem(w http.ResponseWriter, r *http.Request) {
	adminUser := auth.GetUser(r.Context())
	id := r.URL.Query().Get("id")
	if id == "" {
		jsonResp(w, M{"ok": false, "error": msg.Get("missing_id", "糟糕,缺少必要信息,请刷新后重试~")})
		return
	}

	if r.Method == "PUT" {
		var a db.Announcement
		json.NewDecoder(r.Body).Decode(&a)
		a.ID = id
		if err := db.UpdateAnnouncement(&a); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		log.Printf("[TOOLBOX] %s 更新公告: %s", adminUser.Username, a.Title)
		jsonResp(w, M{"ok": true})
		return
	}
	if r.Method == "DELETE" {
		if err := db.DeleteAnnouncement(id); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		log.Printf("[TOOLBOX] %s 删除公告: %s", adminUser.Username, id)
		db.AddAuditLog(&adminUser.ID, nil, "admin_delete_announcement", "toolbox", fmt.Sprintf("公告ID: %s", id), requestIP(r))
		jsonResp(w, M{"ok": true})
		return
	}
}

func handleAdminAnnouncementToggle(w http.ResponseWriter, r *http.Request) {
	adminUser := auth.GetUser(r.Context())
	id := r.URL.Query().Get("id")
	if id == "" {
		jsonResp(w, M{"ok": false, "error": msg.Get("missing_id", "糟糕,缺少必要信息,请刷新后重试~")})
		return
	}
	active, err := db.ToggleAnnouncementActive(id)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	log.Printf("[TOOLBOX] %s 切换公告状态: %s active=%v", adminUser.Username, id, active)
	jsonResp(w, M{"ok": true, "active": active})
}

// ── Admin: Signatures ──

func handleAdminSignatures(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		list, err := db.ListSignatures()
		if err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		jsonResp(w, M{"ok": true, "signatures": list})
		return
	}
	if r.Method == "POST" {
		adminUser := auth.GetUser(r.Context())
		var req struct {
			PackageName   string `json:"package_name"`
			SignatureHash string `json:"signature_hash"`
			Label         string `json:"label"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if err := db.AddSignature(req.PackageName, req.SignatureHash, req.Label); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		log.Printf("[TOOLBOX] %s 添加签名: %s", adminUser.Username, req.PackageName)
		jsonResp(w, M{"ok": true})
		return
	}
	if r.Method == "DELETE" {
		adminUser := auth.GetUser(r.Context())
		idStr := r.URL.Query().Get("id")
		id, _ := strconv.ParseInt(idStr, 10, 64)
		if err := db.RemoveSignature(id); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		log.Printf("[TOOLBOX] %s 删除签名: %d", adminUser.Username, id)
		jsonResp(w, M{"ok": true})
		return
	}
}

// ── Scan releases directory ──

// POST /api/admin/toolbox/versions/scan — scan data/releases/ for new APK files
func handleScanReleases(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}

	releaseDir := "data/releases"
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		jsonResp(w, M{"ok": false, "error": "创建目录失败"})
		return
	}

	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "读取目录失败"})
		return
	}

	// Get existing filenames from DB
	rows, err := db.DB.Query(`SELECT file_name FROM app_versions`)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	existing := map[string]bool{}
	for rows.Next() {
		var fn string
		rows.Scan(&fn)
		existing[fn] = true
	}
	rows.Close()

	var added []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".apk") {
			continue
		}
		if existing[e.Name()] {
			continue
		}

		// Parse version from filename: prism-{version}.apk or prism-{version}_{versionCode}.apk
		name := strings.TrimSuffix(e.Name(), ".apk")
		var version string
		versionCode := 0

		// Try prism-1.0.5_6.apk format first
		if idx := strings.LastIndex(name, "_"); idx > 0 {
			if code, err := strconv.Atoi(name[idx+1:]); err == nil {
				version = name[:idx]
				versionCode = code
			}
		}
		// Fallback: prism-1.0.5.apk → version=1.0.5, auto version_code
		if version == "" {
			version = name
		}
		if versionCode == 0 {
			// Auto-assign: use next available code
			var maxCode int
			db.DB.QueryRow(`SELECT COALESCE(MAX(version_code),0) FROM app_versions`).Scan(&maxCode)
			versionCode = maxCode + 1
		}

		info, _ := e.Info()
		fileSize := int64(0)
		if info != nil {
			fileSize = info.Size()
		}

		_, err := db.AddVersion(version, versionCode, "", e.Name(), fileSize)
		if err != nil {
			log.Printf("[TOOLBOX] 扫描注册版本失败 %s: %v", e.Name(), err)
			continue
		}
		added = append(added, e.Name())
	}

	adminUser := auth.GetUser(r.Context())
	log.Printf("[TOOLBOX] %s 扫描版本目录, 新增 %d 个", adminUser.Username, len(added))
	jsonResp(w, M{"ok": true, "added": added})
}

// GET /dl/ — serve uploaded release files
func handleDownloadRelease(w http.ResponseWriter, r *http.Request) {
	fileName := strings.TrimPrefix(r.URL.Path, "/dl/")
	if fileName == "" || strings.Contains(fileName, "..") {
		http.NotFound(w, r)
		return
	}
	filePath := filepath.Join("data/releases", filepath.Base(fileName))
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	http.ServeFile(w, r, filePath)
}

// ── Admin: Version Management ──

func handleAdminVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		list, err := db.ListAppVersions()
		if err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		jsonResp(w, M{"ok": true, "versions": list})
		return
	}
	if r.Method == "DELETE" {
		adminUser := auth.GetUser(r.Context())
		idStr := r.URL.Query().Get("id")
		id, _ := strconv.ParseInt(idStr, 10, 64)
		if err := db.DeleteVersion(id); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		log.Printf("[TOOLBOX] %s 删除版本 id=%d", adminUser.Username, id)
		jsonResp(w, M{"ok": true})
		return
	}
}

func handleAdminVersionSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	adminUser := auth.GetUser(r.Context())
	var req struct {
		ID int64 `json:"id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.ID == 0 {
		jsonResp(w, M{"ok": false, "error": msg.Get("missing_id", "糟糕,缺少必要信息,请刷新后重试~")})
		return
	}
	v, err := db.SwitchCurrentVersion(req.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	log.Printf("[TOOLBOX] %s 切换当前版本: %s (code=%d)", adminUser.Username, v.Version, v.VersionCode)
	db.AddAuditLog(&adminUser.ID, nil, "admin_switch_version", "toolbox", fmt.Sprintf("版本 %s code=%d", v.Version, v.VersionCode), requestIP(r))
	jsonResp(w, M{"ok": true, "version": v})
}

// ── 管理员：批量点赞 ──

type adminBulkLikeResp struct {
	Success   int            `json:"success"`
	Failed    int            `json:"failed"`
	Details   []adminLikeOne `json:"details"`
	TotalTime string         `json:"total_time"`
}

type adminLikeOne struct {
	AccountID int64  `json:"account_id"`
	Name      string `json:"name"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

func handleAdminBulkLike(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		MsgID    string `json:"msg_id"`
		PushID   int64  `json:"push_id,omitempty"`
		ServerID string `json:"server_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.MsgID == "" && req.ServerID == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 msg_id 或 server_id"})
		return
	}

	start := time.Now()
	accounts, err := db.GetAccountsExcludeSource("phone", "email")
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "查询账号失败: " + err.Error()})
		return
	}

	var (
		mu      sync.Mutex
		result  adminBulkLikeResp
		wg      sync.WaitGroup
		sem     = make(chan struct{}, 10)
	)
	for _, acc := range accounts {
		if acc.Disabled || acc.CookieData == "" {
			continue
		}
		wg.Add(1)
		go func(a *db.GameAccount) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var err error
			c := getHeartbeatClient(a.ID)
			if c == nil {
				c, err = g79.NewClient()
				if err != nil {
					mu.Lock()
					result.Failed++
					result.Details = append(result.Details, adminLikeOne{AccountID: a.ID, Name: a.DisplayName, Error: err.Error()})
					mu.Unlock()
					return
				}
				if err := c.G79AuthenticateWithCookie(a.CookieData); err != nil {
					mu.Lock()
					result.Failed++
					result.Details = append(result.Details, adminLikeOne{AccountID: a.ID, Name: a.DisplayName, Error: err.Error()})
					mu.Unlock()
					return
				}
			}
			if req.ServerID != "" {
				api := "/rental-server-like/update"
				body := fmt.Sprintf(`{"server_id":"%s","is_like":1}`, req.ServerID)
				httpReq, _ := http.NewRequest("POST", c.ReleaseJSON.WebServerUrl+api, strings.NewReader(body))
				httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
				httpReq.Header.Set("User-Agent", "WPFLauncher/0.0.0.0")
				httpReq.Header.Set("user-id", c.UserID)
				httpReq.Header.Set("user-token", g79.CalculateDynamicToken(api, body, c.UserToken))
				resp, hErr := c.HTTPClient().Do(httpReq)
				if hErr != nil {
					err = hErr
				} else {
					bodyBytes, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					var likeResp struct{ Code int `json:"code"`; Message string `json:"message"` }
					json.Unmarshal(bodyBytes, &likeResp)
					if likeResp.Code != 0 {
						err = fmt.Errorf("code=%d: %s", likeResp.Code, likeResp.Message)
					}
				}
			} else {
				_, err = c.LikeMoment(req.MsgID, uint64(req.PushID))
			}
			mu.Lock()
			if err != nil {
				result.Failed++
				result.Details = append(result.Details, adminLikeOne{AccountID: a.ID, Name: a.DisplayName, Error: err.Error()})
			} else {
				result.Success++
				result.Details = append(result.Details, adminLikeOne{AccountID: a.ID, Name: a.DisplayName, OK: true})
			}
			mu.Unlock()
		}(acc)
	}
	wg.Wait()
	result.TotalTime = time.Since(start).Round(time.Millisecond).String()

	adminUser := auth.GetUser(r.Context())
	log.Printf("[ADMIN] %s 批量点赞 msg_id=%s server_id=%s success=%d failed=%d 耗时%s", adminUser.Username, req.MsgID, req.ServerID, result.Success, result.Failed, result.TotalTime)
	db.AddAuditLog(&adminUser.ID, nil, "admin_bulk_like", "toolbox", fmt.Sprintf("msg_id=%s server_id=%s success=%d failed=%d", req.MsgID, req.ServerID, result.Success, result.Failed), requestIP(r))
	jsonResp(w, M{"ok": true, "result": result})
}


// ── User-facing: Toolbox Self-Service ──

// GET /api/toolbox/my-info — returns user's toolbox config with system defaults merged
func handleMyToolboxInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	cfg, err := db.GetToolboxConfig(user.ID)
	if err != nil {
		// No config yet — return defaults
		cfg = &db.ToolboxUserConfig{UserID: user.ID, Enabled: false}
	}

	defaults, _ := db.GetToolboxDefaults()

	// Determine effective values
	effectivePrompt := defaults.DefaultPrompt
	if cfg.PromptPurchased && cfg.CustomPrompt != "" {
		effectivePrompt = cfg.CustomPrompt
	}
	effectiveName := defaults.DefaultName
	if cfg.NamePurchased && cfg.CustomName != "" {
		effectiveName = cfg.CustomName
	}

	// Calculate expiry info
	var expired bool
	var daysRemaining int
	if cfg.ExpiresAt != "" {
		t, parseErr := time.Parse(time.RFC3339, cfg.ExpiresAt)
		if parseErr == nil {
			expired = time.Now().UTC().After(t)
			if !expired {
				daysRemaining = int(t.Sub(time.Now().UTC()).Hours() / 24)
			}
		}
	}

	jsonResp(w, M{
		"ok": true,
		"data": M{
			"enabled":          cfg.Enabled,
			"expires_at":       cfg.ExpiresAt,
			"expired":          expired,
			"days_remaining":   daysRemaining,
			"trial_used":       cfg.TrialUsed,
			"prompt_purchased": cfg.PromptPurchased,
			"name_purchased":   cfg.NamePurchased,
			"custom_prompt":    cfg.CustomPrompt,
			"custom_name":      cfg.CustomName,
			"default_prompt":   defaults.DefaultPrompt,
			"default_name":     defaults.DefaultName,
			"effective_prompt": effectivePrompt,
			"effective_name":   effectiveName,
		},
	})
}

// POST /api/toolbox/my-info — update custom prompt/name (only if purchased)
func handleUpdateMyToolbox(w http.ResponseWriter, r *http.Request) {
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
		CustomPrompt string `json:"custom_prompt"`
		CustomName   string `json:"custom_name"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	cfg, err := db.GetToolboxConfig(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "尚未开通工具箱"})
		return
	}

	// Only allow updating purchased features
	if req.CustomPrompt != "" && !cfg.PromptPurchased {
		jsonResp(w, M{"ok": false, "error": "未购买自定义提示词功能"})
		return
	}
	if req.CustomName != "" && !cfg.NamePurchased {
		jsonResp(w, M{"ok": false, "error": "未购买自定义命名功能"})
		return
	}

	if req.CustomPrompt != "" {
		cfg.CustomPrompt = req.CustomPrompt
	}
	if req.CustomName != "" {
		cfg.CustomName = req.CustomName
	}

	if err := db.UpsertToolboxConfig(cfg); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}

	log.Printf("[TOOLBOX] %s 更新工具箱配置", user.Username)
	jsonResp(w, M{"ok": true})
}

// POST /api/toolbox/start-trial — activate one-time 3-month trial
func handleStartTrial(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	cfg, err := db.StartTrial(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}

	log.Printf("[TOOLBOX] %s 激活工具箱试用, 到期: %s", user.Username, cfg.ExpiresAt)
	db.AddAuditLog(&user.ID, nil, "toolbox_trial", "toolbox", "激活工具箱试用 90 天", requestIP(r))
	jsonResp(w, M{"ok": true, "expires_at": cfg.ExpiresAt})
}

// POST /api/toolbox/purchase/{feature} — buy prompt or name with nuts
func handlePurchaseToolboxFeature(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	// 订阅有效期内不允许购买自定义提示词/命名
	if subActive, _, _ := db.IsSubscriptionActive(user.ID); subActive {
		jsonResp(w, M{"ok": false, "error": "订阅期内无需购买此功能"})
		return
	}

	// Extract feature from path: /api/toolbox/purchase/prompt or /api/toolbox/purchase/name
	feature := strings.TrimPrefix(r.URL.Path, "/api/toolbox/purchase/")
	if feature != "prompt" && feature != "name" {
		jsonResp(w, M{"ok": false, "error": "未知功能"})
		return
	}

	if err := db.PurchaseToolboxFeature(user.ID, feature); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}

	featureName := "自定义提示词"
	if feature == "name" {
		featureName = "自定义命名"
	}
	log.Printf("[TOOLBOX] %s 购买工具箱功能: %s", user.Username, featureName)
	db.AddAuditLog(&user.ID, nil, "toolbox_purchase", "toolbox", fmt.Sprintf("购买: %s", featureName), requestIP(r))
	jsonResp(w, M{"ok": true})
}

// POST /api/toolbox/purchase/extension?days=N — extend toolbox access with nuts
func handleExtendToolbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	// 订阅有效期内不允许续费工具箱
	if subActive, _, _ := db.IsSubscriptionActive(user.ID); subActive {
		jsonResp(w, M{"ok": false, "error": "订阅期内无需续费工具箱"})
		return
	}

	daysStr := r.URL.Query().Get("days")
	days := 30
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
			days = d
		}
	}
	// Cap a single extension at 1 year; the DB layer additionally hard-caps the
	// resulting expiry at 2099-12-31 to prevent time overflow.
	if days > 365 {
		days = 365
	}

	if err := db.ExtendToolboxAccess(user.ID, days); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}

	log.Printf("[TOOLBOX] %s 续期工具箱 %d 天", user.Username, days)
	db.AddAuditLog(&user.ID, nil, "toolbox_extend", "toolbox", fmt.Sprintf("续期 %d 天", days), requestIP(r))
	jsonResp(w, M{"ok": true})
}

// GET /api/toolbox/download — redirect to the latest version's download URL
func handleToolboxDownload(w http.ResponseWriter, r *http.Request) {
	cfg, err := db.GetAppVersionConfig()
	if err != nil || cfg == nil || cfg.UpdateURL == "" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, cfg.UpdateURL, http.StatusFound)
}

// ── Admin: Toolbox User Management ──

// GET /api/admin/toolbox/users — list all users with toolbox config
func handleAdminToolboxUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	list, err := db.ListAllToolboxConfigs()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "users": list})
}

// GET/POST /api/admin/toolbox/user-config — get or update a user's toolbox config
func handleAdminToolboxUserConfig(w http.ResponseWriter, r *http.Request) {
	adminUser := auth.GetUser(r.Context())

	if r.Method == "GET" {
		userIDStr := r.URL.Query().Get("user_id")
		userID, _ := strconv.ParseInt(userIDStr, 10, 64)
		if userID == 0 {
			jsonResp(w, M{"ok": false, "error": "缺少 user_id"})
			return
		}
		cfg, err := db.GetToolboxConfig(userID)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": "未找到该用户的工具箱配置"})
			return
		}
		jsonResp(w, M{"ok": true, "config": cfg})
		return
	}

	if r.Method == "POST" {
		var req struct {
			UserID         int64  `json:"user_id"`
			Enabled        *bool  `json:"enabled,omitempty"`
			ExpiresAt      string `json:"expires_at"`
			PromptPurchased *bool `json:"prompt_purchased,omitempty"`
			NamePurchased  *bool  `json:"name_purchased,omitempty"`
			CustomPrompt   string `json:"custom_prompt"`
			CustomName     string `json:"custom_name"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.UserID == 0 {
			jsonResp(w, M{"ok": false, "error": "缺少 user_id"})
			return
		}

		// Get existing config or create new
		cfg, err := db.GetToolboxConfig(req.UserID)
		if err != nil {
			cfg = &db.ToolboxUserConfig{UserID: req.UserID}
		}

		if req.Enabled != nil {
			cfg.Enabled = *req.Enabled
		}
		if req.ExpiresAt != "" {
			cfg.ExpiresAt = req.ExpiresAt
		}
		if req.PromptPurchased != nil {
			cfg.PromptPurchased = *req.PromptPurchased
		}
		if req.NamePurchased != nil {
			cfg.NamePurchased = *req.NamePurchased
		}
		if req.CustomPrompt != "" {
			cfg.CustomPrompt = req.CustomPrompt
		}
		if req.CustomName != "" {
			cfg.CustomName = req.CustomName
		}

		if err := db.UpsertToolboxConfig(cfg); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}

		log.Printf("[TOOLBOX] %s 更新用户 %d 工具箱配置", adminUser.Username, req.UserID)
		db.AddAuditLog(&adminUser.ID, &req.UserID, "admin_toolbox_config", "toolbox", fmt.Sprintf("用户 %d 工具箱配置已更新", req.UserID), requestIP(r))
		jsonResp(w, M{"ok": true})
		return
	}
}

// GET/POST /api/admin/toolbox/defaults — get or set system defaults
func handleAdminToolboxDefaults(w http.ResponseWriter, r *http.Request) {
	adminUser := auth.GetUser(r.Context())

	if r.Method == "GET" {
		d, _ := db.GetToolboxDefaults()
		jsonResp(w, M{"ok": true, "defaults": d})
		return
	}

	if r.Method == "POST" {
		var req struct {
			DefaultPrompt string `json:"default_prompt"`
			DefaultName   string `json:"default_name"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		d := &db.ToolboxDefaults{
			DefaultPrompt: req.DefaultPrompt,
			DefaultName:   req.DefaultName,
		}
		if err := db.UpsertToolboxDefaults(d); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}

		log.Printf("[TOOLBOX] %s 更新工具箱默认配置", adminUser.Username)
		db.AddAuditLog(&adminUser.ID, nil, "admin_toolbox_defaults", "toolbox", "工具箱默认配置已更新", requestIP(r))
		jsonResp(w, M{"ok": true})
		return
	}
}


// ── Heartbeat + Push Notification Handlers ──

// POST /api/v2/heartbeat — receive heartbeat, return push messages + stats
	func handleHeartbeat(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var req struct {
			Token    string             `json:"token"`
			Version  string             `json:"version"`
			DeviceID string             `json:"device_id"`
			Session  struct {
				Online      bool   `json:"online"`
				ServerCode  string `json:"server_code"`
				DeviceModel string `json:"device_model"`
				StartedAt   int64  `json:"started_at"`
			} `json:"session"`
			Increment db.IncrementData `json:"increment"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		var userID int64
		if req.Token != "" {
			user, err := db.GetUserByToken(req.Token)
			if err != nil {
				jsonResp(w, M{"ok": false, "error": "无效的 token"})
				return
			}
			userID = user.ID
		} else if req.DeviceID != "" {
			// 匿名设备：用 device_id 哈希作为负数 user_id 标识
			h := int64(0)
			for _, c := range req.DeviceID {
				h = h*31 + int64(c)
			}
			if h < 0 {
				h = -h
			}
			userID = -(h % 100000000) // 负数标识匿名设备
		} else {
			jsonResp(w, M{"ok": false, "error": "缺少 token 或 device_id"})
			return
		}

		// 服务器端强制心跳频率：按设备/用户 key 限流，防止攻击者无视 heartbeat_interval 高频反复上报
		hbKey := req.DeviceID
		if hbKey == "" {
			hbKey = req.Token
		}
		if !heartbeatLimiter.Allow("hb:" + hbKey) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(M{"ok": false, "error": "心跳过于频繁，请稍后再试"})
			return
		}

		// 匿名设备数量上限：防止攻击者伪造无限 device_id 撑爆 user_stats/heartbeat_logs
		if userID < 0 {
			var cnt int64
			if err := db.DB.QueryRow(`SELECT COUNT(*) FROM user_stats WHERE user_id < 0`).Scan(&cnt); err == nil && cnt >= maxAnonymousDevices {
				jsonResp(w, M{"ok": false, "error": "匿名设备数量已达上限"})
				return
			}
		}

		resp, err := db.HandleHeartbeat(userID, req.Version, req.DeviceID, req.Session.DeviceModel, req.Session.ServerCode, req.Session.Online, req.Increment)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}

		// 匿名用户也返回全体推送消息
		pendingMsgs := resp.PendingMessages

		jsonResp(w, M{"ok": true, "server_time": resp.ServerTime, "heartbeat_interval": resp.HeartbeatInterval, "pending_messages": pendingMsgs, "stats": resp.Stats})
	}

// GET /api/v2/heartbeat/pending — get pending push messages for a user (by token query param)
func handleHeartbeatPending(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 token"})
		return
	}
	user, err := db.GetUserByToken(token)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "无效的 token"})
		return
	}
	msgs, _ := db.GetPendingPushMessages(user.ID)
	jsonResp(w, M{"ok": true, "messages": msgs})
}

// POST /api/v2/heartbeat/ack — mark messages as read
func handleHeartbeatAck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Token string   `json:"token"`
		IDs   []string `json:"ids"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Token == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 token"})
		return
	}
	user, err := db.GetUserByToken(req.Token)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "无效的 token"})
		return
	}
	if err := db.AckPushMessages(user.ID, req.IDs); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true})
}

// ── Admin: Push Management ──

// GET /api/admin/push/messages — list push messages
func handleAdminPushMessages(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 { limit = 50 }
	msgs, err := db.ListPushMessages(limit, offset)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "messages": msgs})
}

// POST /api/admin/push/send — create a push message
func handleAdminPushSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	adminUser := auth.GetUser(r.Context())
	var m db.PushMessage
	json.NewDecoder(r.Body).Decode(&m)
	if m.Type == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 type"})
		return
	}
	if m.Title == "" && m.Body == "" {
		jsonResp(w, M{"ok": false, "error": "标题和正文不能同时为空"})
		return
	}
	m.ID = fmt.Sprintf("push_%s_%d", time.Now().Format("20060102_150405"), time.Now().UnixNano()%100000)
	m.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	if m.ExpiresAt == "" {
		m.ExpiresAt = time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	}
	if err := db.CreatePushMessage(&m); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	log.Printf("[TOOLBOX] %s 发送推送: %s", adminUser.Username, m.Title)
	db.AddAuditLog(&adminUser.ID, nil, "admin_push_send", "toolbox", fmt.Sprintf("推送: %s type=%s", m.Title, m.Type), requestIP(r))
	jsonResp(w, M{"ok": true, "message": m})
}

// DELETE /api/admin/push/message?id=xxx — delete/recall a push message
func handleAdminPushDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	adminUser := auth.GetUser(r.Context())
	id := r.URL.Query().Get("id")
	if id == "" {
		jsonResp(w, M{"ok": false, "error": msg.Get("missing_id", "糟糕,缺少必要信息,请刷新后重试~")})
		return
	}
	if err := db.DeletePushMessage(id); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	log.Printf("[TOOLBOX] %s 撤回推送: %s", adminUser.Username, id)
	db.AddAuditLog(&adminUser.ID, nil, "admin_push_delete", "toolbox", fmt.Sprintf("撤回推送: %s", id), requestIP(r))
	jsonResp(w, M{"ok": true})
}

// GET /api/admin/push/stats — user stats overview
func handleAdminPushStats(w http.ResponseWriter, r *http.Request) {
	stats, err := db.GetUserStatsOverview()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "stats": stats})
}

// GET /api/admin/push/online-users — list users with recent heartbeat
func handleAdminOnlineUsers(w http.ResponseWriter, r *http.Request) {
	users, err := db.GetOnlineUsers()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "users": users})
}

// GET /api/admin/push/user-detail?user_id=X — get user stats + info
func handleAdminUserDetail(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	if userID == 0 {
		jsonResp(w, M{"ok": false, "error": "缺少 user_id"})
		return
	}
	detail, err := db.GetUserStatsDetail(userID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "用户不存在"})
		return
	}
	// 获取工具箱配置（提示词、命名等）
	var toolboxCfg map[string]interface{}
	if userID > 0 {
		if cfg, err := db.GetToolboxConfig(userID); err == nil {
			defaults, _ := db.GetToolboxDefaults()
			toolboxCfg = map[string]interface{}{
				"enabled": cfg.Enabled,
				"expires_at": cfg.ExpiresAt,
				"prompt_purchased": cfg.PromptPurchased,
				"name_purchased": cfg.NamePurchased,
				"custom_prompt": cfg.CustomPrompt,
				"custom_name": cfg.CustomName,
				"default_prompt": defaults.DefaultPrompt,
				"default_name": defaults.DefaultName,
			}
		}
	}
	jsonResp(w, M{"ok": true, "user": detail, "toolbox_config": toolboxCfg})
}
