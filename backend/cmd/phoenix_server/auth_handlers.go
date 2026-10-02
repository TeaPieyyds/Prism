package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	g79 "github.com/Yeah114/g79client"
	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
	"golang.org/x/crypto/bcrypt"
)

// generateCode generates a random hex code for email verification
func generateCode() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func isMainstreamEmail(email string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	allowed := map[string]bool{
		"qq.com": true, "163.com": true,
	}
	return allowed[domain]
}

func jsonResp(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// ─── POST /api/auth/register ───
func handleRegister(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	if !registerLimiter.Allow(requestIP(r)) {
		jsonResp(w, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
		return
	}
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Email      string `json:"email"`
		Username   string `json:"username"`
		Password   string `json:"password"`
		InviteCode string `json:"invite_code"`
	}
	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	logReq(r, bodyBytes)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_json", "抱歉,提交的数据格式有误,请检查后重试~")})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Username = strings.TrimSpace(req.Username)

	if req.Email == "" || req.Username == "" || req.Password == "" {
		jsonResp(w, M{"ok": false, "error": "邮箱、用户名、密码不能为空"})
		return
	}
	if len(req.Username) > 20 || !validUsername.MatchString(req.Username) {
		jsonResp(w, M{"ok": false, "error": "用户名只能包含中文、英文、数字和下划线，不超过20个字符"})
		return
	}
	if !isMainstreamEmail(req.Email) {
		jsonResp(w, M{"ok": false, "error": "仅支持国内邮箱：QQ、163、126、Yeah、Foxmail"})
		return
	}
	if len(req.Password) < 6 {
		jsonResp(w, M{"ok": false, "error": "抱歉,密码至少需要 6 位,请重新设置~"})
		return
	}

	// Check existing
	exists, err := db.IsEmailRegistered(req.Email)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("system_error", "哎呀,系统开小差了,请稍后重试;仍不行请联系管理员~")})
		return
	}
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

	code := generateCode()
	if err := db.CreateVerifyCode(req.Email, req.Username, string(hash), code, req.InviteCode); err != nil {
		log.Printf("[AUTH] 创建验证码失败: %v", err)
		db.AddSystemLog("error", "创建验证码失败", err.Error())
		jsonResp(w, M{"ok": false, "error": "系统繁忙，请稍后重试"})
		return
	}

	_ = db.AddAuditLog(nil, nil, "register_email_sent", "email", req.Email, requestIP(r))

	// 同步发送邮件，获取真实结果
	sendErr := auth.SendVerifyEmail(req.Email, code)
	if sendErr != nil {
		errMsg := sendErr.Error()
		log.Printf("[AUTH] 验证邮件发送失败: %v", sendErr)
		db.AddSystemLog("warn", "验证邮件发送失败", fmt.Sprintf("收件人=%s 错误=%s", req.Email, errMsg))

		userMsg := "验证邮件发送失败，请稍后重试"
		switch {
		case strings.Contains(errMsg, "SMTP not configured"):
			userMsg = "邮件服务未配置，请联系管理员"
		case strings.Contains(errMsg, "tls dial") || strings.Contains(errMsg, "connection refused"):
			userMsg = "邮件服务器连接失败，请稍后重试"
		case strings.Contains(errMsg, "auth") || strings.Contains(errMsg, "535"):
			userMsg = "邮件服务认证失败，请联系管理员"
		}
		jsonResp(w, M{"ok": false, "error": userMsg})
		return
	}

	log.Printf("[AUTH] 验证邮件已发送至 %s", req.Email)
	db.AddSystemLog("info", "验证邮件已发送", "收件人="+req.Email)

	jsonResp(w, M{
		"ok":      true,
		"message": "验证邮件已发送，请查收邮箱 " + req.Email,
	})
}

// ─── GET /api/auth/verify?email=xxx&code=xxx ───
// 仅校验验证码并渲染「确认注册」页，不消费验证码、不建号。
// 目的：邮件安全扫描器会自动 GET 验证链接来校验安全性，若 GET 即建号，
// 假邮箱（如 1@qq.com）会被意外注册成功；故需用户在确认页手动点击按钮
// （POST /api/auth/verify/confirm）才真正完成注册。
func handleVerify(w http.ResponseWriter, r *http.Request) {
	if !verifyLimiter.Allow(requestIP(r)) {
		http.Error(w, msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~"), 429)
		return
	}
	email := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("email")))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if email == "" || code == "" {
		http.Error(w, msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~"), http.StatusBadRequest)
		return
	}

	pending, ok, err := db.PeekVerifyCode(email, code)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<h2>验证失败</h2><p>系统错误，请重试。</p>`))
		return
	}
	if !ok || pending == nil {
		// 可能是邮件扫描器提前消费了验证码，先检查是否已注册
		if exists, _ := db.IsEmailRegistered(email); exists {
			log.Printf("[AUTH] 验证码已失效但用户已注册，跳转登录页: %s", email)
			http.Redirect(w, r, "/#/login?verified=1", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<h2>验证失败或链接已过期</h2><p>请重新注册。</p>`))
		return
	}

	// 渲染确认页：需要真人点击按钮（POST）才完成注册，扫描器仅 GET 无法通过
	// 用占位符 + strings.ReplaceAll，避免把整段 HTML 当 fmt 格式串（CSS 中的 % 会误判为格式动词）
	escEmail := html.EscapeString(pending.Email)
	escCode := html.EscapeString(code)
	page := `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Prism - 邮箱验证</title>
<style>
body{margin:0;background:#f8f8f0;font-family:'Nunito','Noto Sans SC','Zen Maru Gothic','HarmonyOS Sans SC',-apple-system,'PingFang SC','Hiragino Sans GB','Microsoft YaHei',sans-serif;color:#794f27;display:flex;min-height:100vh;align-items:center;justify-content:center;letter-spacing:0.01em;line-height:1.6}
.card{width:min(420px,100%);margin:24px;background:#fffcf4;border-radius:18px;padding:28px;border:2px solid #e8e2d6;box-shadow:0 8px 24px 0 rgba(61,52,40,0.14);text-align:center}
h1{margin:0;font-size:24px;color:#794f27;letter-spacing:-0.01em;line-height:1.2;margin-bottom:8px}
p{color:#9f927d;font-size:13px;margin:8px 0 20px;line-height:1.6}
button{display:inline-flex;align-items:center;justify-content:center;gap:6px;padding:10px 20px;border:none;border-radius:12px;font-family:inherit;font-size:14px;font-weight:700;line-height:1;cursor:pointer;width:100%;background:#19c8b9;color:#fff;transition:background 0.15s ease,transform 0.15s ease;touch-action:manipulation}
button:hover{background:#3dd4c6}
button:active{background:#50B9AB;transform:scale(0.97)}
.small{font-size:13px;color:#9f927d;margin-top:12px}
</style></head><body><div class="card">
<h1>邮箱验证</h1>
<p>验证邮箱 <b>__EMAIL__</b> 成功。<br>点击下方按钮完成注册。</p>
<form method="POST" action="/api/auth/verify/confirm">
<input type="hidden" name="email" value="__EMAIL__">
<input type="hidden" name="code" value="__CODE__">
<button type="submit">确认注册</button>
</form>
<div class="small">为安全起见，需点击按钮完成注册。</div>
</div></body></html>`
	page = strings.ReplaceAll(page, "__EMAIL__", escEmail)
	page = strings.ReplaceAll(page, "__CODE__", escCode)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, page)
}

// ─── POST /api/auth/verify/confirm ───
// 用户在确认页点击后提交：此处消费验证码并创建已验证账号。
// 使用独立的 verifyConfirmLimiter，避免与 GET 预览共享限流桶
// （否则点链接(GET)后马上点按钮(POST)会被误判为过快）。
func handleVerifyConfirm(w http.ResponseWriter, r *http.Request) {
	if !verifyConfirmLimiter.Allow(requestIP(r)) {
		http.Error(w, msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~"), 429)
		return
	}
	if r.Method != "POST" {
		http.Error(w, msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~"), http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~"), http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(strings.ToLower(r.PostFormValue("email")))
	code := strings.TrimSpace(r.PostFormValue("code"))
	if email == "" || code == "" {
		http.Error(w, msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~"), http.StatusBadRequest)
		return
	}

	pending, ok, err := db.CheckVerifyCode(email, code)
	if err != nil || !ok || pending == nil {
		// 可能是邮件扫描器提前消费了验证码，先检查是否已注册
		if exists, _ := db.IsEmailRegistered(email); exists {
			log.Printf("[AUTH] 验证码已失效但用户已注册，跳转登录页: %s", email)
			http.Redirect(w, r, "/#/login?verified=1", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<h2>验证失败或链接已过期</h2><p>请重新注册。</p>`))
		return
	}

	if exists, _ := db.IsEmailRegistered(pending.Email); exists {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<h2>验证失败</h2><p>该邮箱已被激活账号占用。</p>`))
		return
	}
	if taken, _ := db.IsUsernameTaken(pending.Username); taken {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<h2>验证失败</h2><p>该用户名已被激活账号占用。</p>`))
		return
	}

	u, err := db.CreateVerifiedUser(pending.Email, pending.Username, pending.PasswordHash)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<h2>验证失败</h2><p>` + err.Error() + `</p>`))
		return
	}
	_ = db.AddAuditLog(&u.ID, nil, "register_verified", "web_user", pending.Email, requestIP(r))
	db.AddNuts(u.ID, 50, "register", nil, nil)
	if pending.InviteCode != "" {
		db.RedeemInviteCode(pending.InviteCode, u.ID)
	}
	go func() {
		if err := auth.SendAdminNewUserEmail(pending.Email, pending.Username); err != nil {
			log.Printf("[AUTH] Failed to send admin new user notification: %v", err)
		}
	}()

	// Redirect to frontend login page
	http.Redirect(w, r, "/#/login?verified=1", http.StatusFound)
}

// ─── 管理员暴力破解防护 ───
// 针对管理员账号的密码登录做 IP 级封禁：同一 IP 连续输错密码达上限即封禁 10 分钟，
// 封禁期内该 IP 对管理员账号的一切密码登录（无论密码对错）一律返回「邮箱或密码错误」，
// 防止暴力破解同时避免暴露账号是否存在。
type adminBruteForce struct {
	mu         sync.Mutex
	failures   map[string]int       // ip -> 连续失败次数
	bannedUntil map[string]time.Time // ip -> 封禁截止时间
}

const (
	adminMaxFails  = 3                // 连续失败次数上限
	adminBanWindow = 10 * time.Minute // 封禁时长
)

var adminBrute = &adminBruteForce{
	failures:    make(map[string]int),
	bannedUntil: make(map[string]time.Time),
}

// banned 返回该 IP 是否处于封禁期；若已过封禁期则清理并返回 false。
func (b *adminBruteForce) banned(ip string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	until, ok := b.bannedUntil[ip]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(b.bannedUntil, ip)
		delete(b.failures, ip)
		return false
	}
	return true
}

// recordFail 记录一次失败；连续失败达到上限即封禁该 IP 并返回 true。
func (b *adminBruteForce) recordFail(ip string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	// 已封禁期内不再累加
	if until, ok := b.bannedUntil[ip]; ok && time.Now().Before(until) {
		return false
	}
	b.failures[ip]++
	if b.failures[ip] >= adminMaxFails {
		b.bannedUntil[ip] = time.Now().Add(adminBanWindow)
		delete(b.failures, ip)
		return true
	}
	return false
}

// reset 登录成功时清空该 IP 的失败计数（不解除已有封禁）。
func (b *adminBruteForce) reset(ip string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.failures, ip)
}

// purge 清理已过期的封禁与计数记录，防止内存无限增长。
func (b *adminBruteForce) purge() {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	for ip, until := range b.bannedUntil {
		if now.After(until) {
			delete(b.bannedUntil, ip)
			delete(b.failures, ip)
		}
	}
}

// adminLoginBlockedError 是管理员账号封禁期/密码错误时的通用提示，避免暴露账号是否存在。
const adminLoginBlockedError = "糟糕,邮箱或密码不对。可能是没注册或输错了,请核对后再试;忘了密码点「找回密码」~"

// ─── POST /api/auth/login ───
func handleLogin(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	if !loginLimiter.Allow(requestIP(r)) {
		jsonResp(w, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
		return
	}
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_json", "抱歉,提交的数据格式有误,请检查后重试~")})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	ip := requestIP(r)

	user, err := db.GetUserByEmail(req.Email)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": adminLoginBlockedError})
		return
	}

	// 管理员账号暴力破解防护：封禁期内该 IP 对管理员账号的一切密码登录
	// （无论密码对错）都返回通用错误，既不暴露账号存在与否，也阻断暴力破解。
	if user.Role == "admin" && adminBrute.banned(ip) {
		jsonResp(w, M{"ok": false, "error": adminLoginBlockedError})
		return
	}

	if user.Disabled {
		jsonResp(w, M{"ok": false, "error": "账号已被停用"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		if user.Role == "admin" && adminBrute.recordFail(ip) {
			log.Printf("[AUTH] 管理员账号 %s 连续输错 %d 次密码,IP %s 已封禁 %v", user.Email, adminMaxFails, ip, adminBanWindow)
			_ = db.AddAuditLog(&user.ID, nil, "admin_login_bruteforce_ban", "ip", ip, ip)
		}
		jsonResp(w, M{"ok": false, "error": adminLoginBlockedError})
		return
	}
	adminBrute.reset(ip)

	if !user.Verified {
		jsonResp(w, M{"ok": false, "error": "邮箱未验证，请先查收验证邮件"})
		return
	}
	_ = db.AddAuditLog(&user.ID, nil, "login", "web", "网站账号登录", ip)

	if enableActivationCheck && user.LoginCount > 3 && !user.Activated {
		jsonResp(w, M{"ok": false, "error": "试用次数已用完，请登录网页加入QQ群获取激活码", "require_activation": true})
		return
	}

	SetSessionCookie(w, user.ID)

	jsonResp(w, M{
		"ok":      true,
		"message": "登录成功",
		"user": M{
			"id":                    user.ID,
			"username":              user.Username,
			"email":                 user.Email,
			"role":                  user.Role,
			"token":                 user.Token,
			"disabled":              user.Disabled,
			"growth_override":       user.GrowthOverride,
			"growth_override_value": user.GrowthOverrideValue,
		},
	})
}

// ─── GET /api/auth/me ───
func handleMe(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	activeAccountID := user.ActiveAccountID
	if apiTok := auth.GetAPIToken(r.Context()); apiTok != nil {
		activeAccountID = apiTok.ActiveAccountID
	}

	jsonResp(w, M{
		"ok": true,
		"user": M{
			"id":                    user.ID,
			"username":              user.Username,
			"email":                 user.Email,
			"role":                  user.Role,
			"token":                 user.Token,
			"active_account_id":     activeAccountID,
			"disabled":              user.Disabled,
			"challenge_override":    user.ChallengeOverride,
			"growth_override":       user.GrowthOverride,
			"growth_override_value": user.GrowthOverrideValue,
			"nuts_balance":          user.NutsBalance,
			"subscription_start":    user.SubscriptionStart,
			"subscription_until":    user.SubscriptionUntil,
			"activated":             user.Activated,
			"require_activation":    enableActivationCheck && user.LoginCount > 3 && !user.Activated,
		},
	})
}

// ─── POST /api/auth/token ───
// Returns the user's API token (for display in frontend)
func handleTokenRefresh(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	jsonResp(w, M{"ok": true, "token": user.Token})
}

func accountVisual(acc *db.GameAccount) (avatarURL, skinNumber, growthLevel string) {
	cookieStore.mu.RLock()
	defer cookieStore.mu.RUnlock()
	for _, sc := range cookieStore.Cookies {
		if sc.UID == acc.UID || sc.Cookie == acc.CookieData {
			avatarURL = sc.AvatarImageURL
			if sc.SkinNumber > 0 {
				skinNumber = fmt.Sprintf("%d", sc.SkinNumber)
			}
			if sc.GrowthLevel > 0 {
				growthLevel = fmt.Sprintf("%d", sc.GrowthLevel)
			}
			return
		}
	}
	return
}

func fillSkinURL(client *g79.Client, info *db.AccountInfo) {
	settings, err := client.GetUserSettingList()
	if err != nil || settings == nil {
		return
	}
	itemID := settings.Entity.SkinData.ItemID
	if itemID == "" || itemID == "-1" {
		return
	}
	di, err := client.GetDownloadInfo(itemID)
	if err != nil || di == nil {
		return
	}
	info.SkinURL = di.Entity.ResURL
}

func fillAccountInfoFromClient(client *g79.Client, info *db.AccountInfo) {
	ud, udErr := client.GetUserDetail()
	if udErr == nil && ud != nil {
		client.UserDetail = &ud.Entity
		if after, err := client.GetPeUserLoginAfter(); err == nil && after != nil && after.Entity.UsedName != "" {
			client.UserDetail.UsedName = after.Entity.UsedName
		}
		name := firstNonEmpty(client.UserDetail.Name, client.UserDetail.UsedName)
		if name != "" {
			info.DisplayName = name
		}
		info.GrowthLevel = ud.Entity.Level.Raw
		info.Score = ud.Entity.Score.Raw
		info.SkinNumber = fmt.Sprintf("%d", ud.Entity.SkinNumber.Int64())
		info.CapeNumber = fmt.Sprintf("%d", ud.Entity.CapeNumber.Int64())
		info.AvatarImageURL = ud.Entity.AvatarImageURL
		info.IsVip = ud.Entity.IsVIP
		// Only update status from G79 if explicitly banned — cookie data is more reliable
		if ud.Entity.AccessGameFlag.Raw != "" && ud.Entity.AccessGameFlag.Raw != "0" {
			info.AccessGameFlag = ud.Entity.AccessGameFlag.Raw
		}
	}
	if client.UserID != "" {
		info.UID = client.UserID
	}
	gameLevel := int64(0)
	if client.UserDetail != nil {
		gameLevel = client.UserDetail.Level.Int64()
	}
	if od, err := client.GetOtherUserDetail(client.UserID, true); err == nil && od != nil {
		if peLv := od.Entity.PEGrowth.Lv.Int64(); peLv > gameLevel {
			gameLevel = peLv
		}
		info.GrowthExp = od.Entity.PEGrowth.Exp.Raw
		info.GrowthNeedExp = od.Entity.PEGrowth.NeedExp.Raw
	}
	if gameLevel > 0 {
		info.GrowthLevel = fmt.Sprintf("%d", gameLevel)
	}
	fillSkinURL(client, info)
}

// ─── GET /api/accounts ───
func handleListAccounts(w http.ResponseWriter, r *http.Request) {
	if !accountListLimiter.Allow(requestIP(r)) {
		jsonResp(w, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	// Get user's own accounts
	myAccs, err := db.GetUserAccounts(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}

	// Get shared accounts
	sharedAccs, err := db.GetSharedAccounts()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}

	// Build response
	type accountResp struct {
		ID                 int64  `json:"id"`
		DisplayName        string `json:"display_name"`
		UID                string `json:"uid"`
		Status             string `json:"status"`
		GrowthLevel        string `json:"growth_level"`
		Score              string `json:"score"`
		SkinNumber         string `json:"skin_number"`
		SkinURL            string `json:"skin_url"`
		AvatarImageURL     string `json:"avatar_image_url"`
		CapeNumber         string `json:"cape_number"`
		IsVip              bool   `json:"is_vip"`
		Source             string `json:"source"`
		Disabled           bool   `json:"disabled"`
		OwnerID            *int64 `json:"owner_id,omitempty"`
		CreatedBy          *int64 `json:"created_by,omitempty"`
		CanReclaim         bool   `json:"can_reclaim"`
		IsShared           bool   `json:"is_shared"`
		IsActive           bool   `json:"is_active"`
		IsGuest            bool   `json:"is_guest"`
		AutoRefreshEnabled bool   `json:"auto_refresh_enabled"`
		IsServerOwner      bool   `json:"is_server_owner"`
		HasSharedCopy      bool   `json:"has_shared_copy"`
	}

	// Guest mode: return only the picked shared account
	if auth.IsGuest(user) {
		ga := getGuestAccount()
		if ga == nil {
			jsonResp(w, M{"ok": true, "accounts": []accountResp{}})
			return
		}
		jsonResp(w, M{"ok": true, "accounts": []accountResp{{
			ID:             ga.ID,
			DisplayName:    ga.DisplayName,
			UID:            ga.UID,
			Status:         ga.Status,
			GrowthLevel:    ga.GrowthLevel,
			Score:          ga.Score,
			SkinNumber:     ga.SkinNumber,
			SkinURL:        ga.SkinURL,
			AvatarImageURL: ga.AvatarImageURL,
			CapeNumber:     ga.CapeNumber,
			IsVip:          ga.IsVip,
			Source:         ga.Source,
			Disabled:       ga.Disabled,
			OwnerID:        ga.OwnerID,
			CreatedBy:      ga.CreatedBy,
			IsShared:       true,
			IsActive:       true,
			IsGuest:        true,
		}}})
		return
	}

	var accounts []accountResp
	hasShared := make(map[string]bool)
	for _, a := range myAccs {
		if s, _ := db.HasSharedCopy(a.UID); s {
			hasShared[a.UID] = true
		}
	}
	activeID := user.ActiveAccountID

	for _, a := range myAccs {
		isActive := activeID != nil && *activeID == a.ID
		accounts = append(accounts, accountResp{
			ID:                 a.ID,
			DisplayName:        a.DisplayName,
			UID:                a.UID,
			Status:             a.Status,
			GrowthLevel:        a.GrowthLevel,
			Score:              a.Score,
			SkinNumber:         a.SkinNumber,
			SkinURL:            a.SkinURL,
			AvatarImageURL:     a.AvatarImageURL,
			CapeNumber:         a.CapeNumber,
			IsVip:              a.IsVip,
			Source:             a.Source,
			Disabled:           a.Disabled,
			OwnerID:            a.OwnerID,
			CreatedBy:          a.CreatedBy,
			CanReclaim:         false,
			AutoRefreshEnabled: a.AutoRefreshEnabled,
			HasSharedCopy:      hasShared[a.UID],
			IsShared:           false,
			IsServerOwner:      a.IsServerOwner,
			IsActive:           isActive,
		})
	}
	for _, a := range sharedAccs {
		isActive := activeID != nil && *activeID == a.ID
		displayName := a.DisplayName
		if isGuestAccount(a.ID) {
			displayName = a.DisplayName + " (匿名共享)"
		}
		accounts = append(accounts, accountResp{
			ID:                 a.ID,
			DisplayName:        displayName,
			UID:                a.UID,
			Status:             a.Status,
			GrowthLevel:        a.GrowthLevel,
			Score:              a.Score,
			SkinNumber:         a.SkinNumber,
			SkinURL:            a.SkinURL,
			AvatarImageURL:     a.AvatarImageURL,
			CapeNumber:         a.CapeNumber,
			IsVip:              a.IsVip,
			Source:             a.Source,
			Disabled:           a.Disabled,
			OwnerID:            a.OwnerID,
			CreatedBy:          a.CreatedBy,
			CanReclaim:         a.CreatedBy != nil && *a.CreatedBy == user.ID,
			AutoRefreshEnabled: a.AutoRefreshEnabled,
			IsShared:           true,
			IsServerOwner:      a.IsServerOwner,
			IsActive:           isActive,
		})
	}

	jsonResp(w, M{"ok": true, "accounts": accounts})
}

// ─── POST /api/accounts/add ───
func handleAddAccount(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	var req struct {
		Cookie string `json:"cookie"`
		Shared bool   `json:"shared"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_json", "抱歉,提交的数据格式有误,请检查后重试~")})
		return
	}
	if req.Cookie == "" {
		jsonResp(w, M{"ok": false, "error": "Cookie 不能为空"})
		return
	}

	// 解析 cookie 获取基本信息
	normalizedCookie := normalizeCookie(req.Cookie)

	info, err := db.ParseAccountInfo(normalizedCookie)
	if err != nil {
		jsonResp(w, M{"ok": false, "stage": "parse", "error": "Cookie 解析失败: " + err.Error()})
		return
	}

	info.Status = "normal"
	info.Source = "web"
	info.CreatedBy = &user.ID

	// 先验证cookie有效性，通过后才保存
	hc := PickOneTimeProxy()
	client, cliErr := g79.NewClientWithHTTPClient(hc)
	if cliErr != nil {
		jsonResp(w, M{"ok": false, "error": "创建客户端失败: " + cliErr.Error()})
		return
	}
	authErr := client.G79AuthenticateWithCookie(normalizedCookie)
	if authErr != nil {
		jsonResp(w, M{"ok": false, "error": "Cookie 验证失败，请检查Cookie是否有效: " + authErr.Error()})
		return
	}
	fillAccountInfoFromClient(client, info)

	var ownerID *int64
	if !req.Shared {
		ownerID = &user.ID
	}

	// 验证通过，存入数据库
	dup, _ := db.LookupAccountByUID(info.UID)
	isDup := dup != nil
	isBanned := info.Status != "normal"
	acc, err := db.AddAccount(ownerID, normalizedCookie, *info)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "保存失败: " + err.Error()})
		return
	}
	// 新添加的账号自动开启心跳（phone/email 除外）
	startAccountHeartbeat(acc.ID, client)

	// Set as active if user has no active account
	if user.ActiveAccountID == nil {
		db.SetActiveAccount(user.ID, acc.ID)
	}
	log.Printf("[ACCOUNTS] User %s added account: %s (uid=%s)", user.Username, info.DisplayName, info.UID)
	_ = db.AddAuditLog(&user.ID, &acc.ID, "add_account", "game_account", fmt.Sprintf("%s uid=%s shared=%v", info.DisplayName, info.UID, req.Shared), requestIP(r))
	if !isBanned && !isDup {
		db.AddNuts(user.ID, 5, "add_account", nil, &acc.ID)
	}

	jsonResp(w, M{
		"ok":           true,
		"message":      fmt.Sprintf("账号 %s 添加成功", info.DisplayName),
		"id":           acc.ID,
		"display_name": info.DisplayName,
		"uid":          info.UID,
		"status":       info.Status,
	})
}

// ─── POST /api/accounts/{id}/switch ───
func handleSwitchAccount(w http.ResponseWriter, r *http.Request) {
	if !switchAccountLimiter.Allow(requestIP(r)) {
		jsonResp(w, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	if auth.IsGuest(user) {
		jsonResp(w, M{"ok": false, "error": "游客模式不支持切换账号"})
		return
	}
	// 安全：切换账号必须使用主令牌鉴权（网页会话或主 token）。
	// 子令牌只能读，不能改变任何账号绑定，防止子令牌越权改绑。
	if auth.GetAPIToken(r.Context()) != nil {
		jsonResp(w, M{"ok": false, "error": "切换账号需使用主令牌鉴权"})
		return
	}

	// Optional body: { token_id } — when >0, bind the account to that token
	// instead of the user's global active account.
	var req struct {
		TokenID int64 `json:"token_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Parse ID from URL path: /api/accounts/{id}/switch
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/accounts/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		jsonResp(w, M{"ok": false, "error": msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~")})
		return
	}

	var accountID int64
	fmt.Sscanf(parts[0], "%d", &accountID)
	if accountID == 0 {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_id", "糟糕,账号编号无效或已失效,请刷新后重试~")})
		return
	}

	// Verify account exists and is accessible (owned or shared)
	acc, err := db.GetAccountByID(accountID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
		return
	}

	// Check access: owned by user, or shared
	if acc.OwnerID != nil && *acc.OwnerID != user.ID {
		jsonResp(w, M{"ok": false, "error": "无权使用此账号"})
		return
	}
	if acc.Disabled {
		jsonResp(w, M{"ok": false, "error": "此游戏账号已被停用"})
		return
	}

	bindTarget := fmt.Sprintf("账号: %s", acc.DisplayName)
	tokenID := req.TokenID
	if tokenID == 0 {
		// Authenticated via a secondary token → bind to that token by default.
		if apiTok := auth.GetAPIToken(r.Context()); apiTok != nil {
			tokenID = apiTok.ID
		}
	}
	if tokenID > 0 {
		// Bind the account to the specified secondary token.
		if tok, err := db.GetAPITokenByID(tokenID, user.ID); err == nil && tok != nil {
			if err := db.SetTokenActiveAccount(tokenID, &accountID); err != nil {
				jsonResp(w, M{"ok": false, "error": "切换失败"})
				return
			}
			bindTarget = fmt.Sprintf("令牌#%d(%s) -> 账号: %s", tokenID, tok.Name, acc.DisplayName)
		} else {
			jsonResp(w, M{"ok": false, "error": "令牌不存在或不属于您"})
			return
		}
	} else {
		// Default: user's global active account (also the primary token's binding).
		if err := db.SetActiveAccount(user.ID, accountID); err != nil {
			jsonResp(w, M{"ok": false, "error": "切换失败"})
			return
		}
	}
	_ = db.AddAuditLog(&user.ID, &accountID, "switch_account", "game_account", bindTarget, requestIP(r))

	jsonResp(w, M{"ok": true, "message": "已切换"})
}

// ─── GET /api/accounts/active ───
func handleActiveAccount(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	if auth.IsGuest(user) {
		ga := getGuestAccount()
		if ga == nil {
			jsonResp(w, M{"ok": false, "error": "没有可用共享账号"})
			return
		}
		jsonResp(w, M{"ok": true, "id": ga.ID, "display_name": ga.DisplayName, "uid": ga.UID, "is_guest": true})
		return
	}

	// Resolve the active account: a sub-token caller sees its own binding first,
	// then an explicit ?token_id=, then the user's global active account.
	activeAccountID := user.ActiveAccountID
	if apiTok := auth.GetAPIToken(r.Context()); apiTok != nil {
		activeAccountID = apiTok.ActiveAccountID
	} else if tokID, _ := strconv.ParseInt(r.URL.Query().Get("token_id"), 10, 64); tokID > 0 {
		if tok, err := db.GetAPITokenByID(tokID, user.ID); err == nil && tok != nil {
			activeAccountID = tok.ActiveAccountID
		}
	}

	if activeAccountID == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("no_active_account", "哎呀,当前没有可用的活跃账号,请先添加或启用账号~")})
		return
	}

	acc, err := db.GetAccountByID(*activeAccountID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "哎呀,当前没有可用的活跃账号,请先添加或启用账号~"})
		return
	}
	if acc.Disabled {
		jsonResp(w, M{"ok": false, "error": "活跃账号已被停用"})
		return
	}

	info, _ := db.ParseAccountInfo(acc.CookieData)
	jsonResp(w, M{
		"ok":                   true,
		"id":                   acc.ID,
		"display_name":         acc.DisplayName,
		"uid":                  acc.UID,
		"status":               acc.Status,
		"growth_level":         acc.GrowthLevel,
		"score":                acc.Score,
		"skin_number":          acc.SkinNumber,
		"skin_url":             acc.SkinURL,
		"avatar_image_url":     acc.AvatarImageURL,
		"cape_number":          acc.CapeNumber,
		"is_vip":               acc.IsVip,
		"disabled":             acc.Disabled,
		"source":               acc.Source,
		"access_game_flag":     info.AccessGameFlag,
		"realname_status":      info.RealnameStatus,
		"anti_addition_status": info.AntiAdditionStatus,
		"signature":            info.Signature,
	})
}

// ─── DELETE /api/accounts/{id} ───
func handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" && r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/accounts/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		jsonResp(w, M{"ok": false, "error": msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~")})
		return
	}
	var accountID int64
	fmt.Sscanf(parts[0], "%d", &accountID)
	if accountID == 0 {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_id", "糟糕,账号编号无效或已失效,请刷新后重试~")})
		return
	}

	acc, err := db.GetAccountByID(accountID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
		return
	}

	// Only owner can delete private accounts; anyone can delete shared? Only admin/owner
	if acc.OwnerID != nil && *acc.OwnerID != user.ID && user.Role != "admin" {
		jsonResp(w, M{"ok": false, "error": "无权删除此账号"})
		return
	}
	if acc.OwnerID == nil && user.Role != "admin" {
		jsonResp(w, M{"ok": false, "error": "共享账号仅管理员可删除"})
		return
	}

	if err := db.DeleteAccountCascade(accountID); err != nil {
		jsonResp(w, M{"ok": false, "error": "删除失败"})
		return
	}
	// 删除账号扣除 5 积分（添加账号时奖励的 5 分收回）
	db.AddNuts(user.ID, -5, "delete_account", nil, &accountID)
	_ = db.AddAuditLog(&user.ID, &accountID, "delete_account", "game_account", acc.DisplayName+" ("+acc.UID+")", requestIP(r))

	// Clear active if it was active
	if user.ActiveAccountID != nil && *user.ActiveAccountID == accountID {
		db.ClearActiveAccount(user.ID)
		client = nil
	}

	jsonResp(w, M{"ok": true, "message": "已删除"})
}

// ─── POST /api/accounts/{id}/refresh ───
func handleRefreshAccount(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	log.Printf("[REFRESH] path=%s method=%s", r.URL.Path, r.Method)
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/accounts/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		jsonResp(w, M{"ok": false, "error": msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~")})
		return
	}
	var accountID int64
	fmt.Sscanf(parts[0], "%d", &accountID)
	if accountID == 0 {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_id", "糟糕,账号编号无效或已失效,请刷新后重试~")})
		return
	}

	// Rate limit
	if !refreshLimiter.Allow(fmt.Sprintf("refresh_%d", accountID)) {
		jsonResp(w, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
		return
	}

	acc, err := db.GetAccountByID(accountID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
		return
	}

	// 优先复用缓存中未过期的已认证 client：token 有效即可实时拉取最新账号数据
	//（GetUserDetail 等轻量接口），避免每次手动刷新都走完整 G79 认证，减少认证次数与风控触发。
	// 缓存缺失或会话失效时才回退完整认证。
	var client *g79.Client
	if cached := getCachedClient(accountID); cached != nil {
		if _, udErr := cached.GetUserDetail(); udErr == nil {
			client = cached
		}
	}
	if client == nil {
		var err error
		client, err = newG79ClientWithProxy(accountID)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": "创建客户端失败: " + err.Error()})
			return
		}
		if err = client.G79AuthenticateWithCookie(acc.CookieData); err != nil {
			errStr := err.Error()
			log.Printf("[REFRESH] auth fail #%d proxy=%s err=%v", accountID, accountProxy[accountID], err)
			// code 2001 不是错误，不判离线
			if strings.Contains(errStr, "code: 2001") {
				jsonResp(w, M{"ok": false, "error": "刷新失败（临时错误，请重试）"})
				return
			}
			// code 32：IP/服务器临时限制，不判离线，并切换代理
			if strings.Contains(errStr, "code: 32") {
				ReportAccountCode32(accountID)
				jsonResp(w, M{"ok": false, "error": "刷新失败（IP/服务器临时限制，请稍后重试）"})
				return
			}
			// code 29 = 永久封禁：共享账号直接删除，私有账号标记封禁
			if strings.Contains(errStr, "code: 29") || strings.Contains(errStr, "禁止登录") {
				db.HandleAccountBanned(accountID)
				jsonResp(w, M{"ok": false, "error": "账号已封禁: " + errStr})
				return
			}
			// 其他：Cookie 失效，标记离线
			db.UpdateAccountStatus(accountID, "offline")
			jsonResp(w, M{"ok": false, "error": "Cookie 已失效: " + errStr})
			return
		}
		setCachedClient(accountID, client)
	}

	info := db.AccountInfo{
		DisplayName:    acc.DisplayName,
		UID:            acc.UID,
		Status:         "normal",
		GrowthLevel:    acc.GrowthLevel,
		Score:          acc.Score,
		SkinNumber:     acc.SkinNumber,
		CapeNumber:     acc.CapeNumber,
		AvatarImageURL: acc.AvatarImageURL,
		IsVip:          acc.IsVip,
		Source:         acc.Source,
	}
	fillAccountInfoFromClient(client, &info)

	if err := db.UpdateAccountFull(accountID, info); err != nil {
		jsonResp(w, M{"ok": false, "error": "更新失败"})
		return
	}
	log.Printf("[ACCOUNTS] Refreshed account %d: name=%s uid=%s level=%s skin_url=%t proxy=%s", accountID, info.DisplayName, info.UID, info.GrowthLevel, info.SkinURL != "", accountProxy[accountID])
	_ = db.AddAuditLog(&user.ID, &accountID, "refresh_account", "game_account", fmt.Sprintf("刷新游戏账号 %s (%s)", info.DisplayName, info.UID), requestIP(r))

	jsonResp(w, M{"ok": true, "message": "刷新成功"})
}

// ─── Admin: GET /api/admin/users ───
func handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := db.ListAllUsers()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	type userResp struct {
		ID                int64  `json:"id"`
		Email             string `json:"email"`
		Username          string `json:"username"`
		Role              string `json:"role"`
		Verified          bool   `json:"verified"`
		Token             string `json:"token"`
		Disabled          bool   `json:"disabled"`
		ActiveAccountID   *int64 `json:"active_account_id,omitempty"`
		ActiveAccountName string `json:"active_account_name,omitempty"`
		ActiveAccountUID  string `json:"active_account_uid,omitempty"`
		NutsBalance       int    `json:"nuts_balance"`
		SubscriptionStart string `json:"subscription_start,omitempty"`
		SubscriptionUntil string `json:"subscription_until,omitempty"`
	}
	var list []userResp
	for _, u := range users {
		item := userResp{
			ID: u.ID, Email: u.Email, Username: u.Username,
			Role: u.Role, Verified: u.Verified, Token: u.Token, Disabled: u.Disabled,
			NutsBalance:       u.NutsBalance,
			SubscriptionStart: u.SubscriptionStart,
			SubscriptionUntil: u.SubscriptionUntil,
			ActiveAccountID:   u.ActiveAccountID,
		}
		if u.ActiveAccountID != nil {
			if acc, err := db.GetAccountByID(*u.ActiveAccountID); err == nil && acc != nil {
				item.ActiveAccountName = acc.DisplayName
				item.ActiveAccountUID = acc.UID
			}
		}
		list = append(list, item)
	}
	jsonResp(w, M{"ok": true, "users": list})
}

// refreshLimiter rate-limits account refresh
var refreshLimiter = auth.NewRateLimiter(1, 10*time.Second)

var accountListLimiter = auth.NewRateLimiter(10, 3*time.Second)
var switchAccountLimiter = auth.NewRateLimiter(6, 7*time.Second)

var forgotPwdEmailLimiter = auth.NewRateLimiter(1, 60*time.Second)

// ─── POST /api/auth/forgot-password ───
func handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if !forgotPwdLimiter.Allow(requestIP(r)) {
		jsonResp(w, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
		return
	}
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_json", "抱歉,提交的数据格式有误,请检查后重试~")})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		jsonResp(w, M{"ok": false, "error": "请输入邮箱"})
		return
	}
	// Per-email rate limit — prevent bombing from multiple IPs
	if !forgotPwdEmailLimiter.Allow("email:" + req.Email) {
		jsonResp(w, M{"ok": true, "message": "如果该邮箱已注册，重置邮件已发送至您的邮箱"})
		return
	}
	user, err := db.GetUserByEmail(req.Email)
	if err != nil || user == nil || !user.Verified || user.Disabled {
		// Always return success to prevent email enumeration
		jsonResp(w, M{"ok": true, "message": "如果该邮箱已注册，重置邮件已发送至您的邮箱"})
		return
	}
	// 管理员账号不支持自助找回密码（曾发生管理员邮箱被控后账号被接管）。
	// 响应保持模糊以防枚举，但记录审计以便发现尝试。
	if user.Role == "admin" {
		_ = db.AddAuditLog(&user.ID, nil, "forgot_password_admin_blocked", "email", req.Email+" ip="+requestIP(r), requestIP(r))
		jsonResp(w, M{"ok": true, "message": "如果该邮箱已注册，重置邮件已发送至您的邮箱"})
		return
	}
	// Prevent code flooding — max 2 reset codes per 10 minutes
	if recent := db.CountRecentVerifyCodes(req.Email, "reset", 10*time.Minute); recent > 2 {
		jsonResp(w, M{"ok": true, "message": "如果该邮箱已注册，重置邮件已发送至您的邮箱"})
		return
	}
	code := generateCode()
	if err := db.CreateGenericVerifyCode(req.Email, code, "reset"); err != nil {
		log.Printf("[AUTH] Failed to create reset code: %v", err)
		jsonResp(w, M{"ok": true, "message": "如果该邮箱已注册，重置邮件已发送至您的邮箱"})
		return
	}
	_ = db.AddAuditLog(&user.ID, nil, "forgot_password_sent", "email", req.Email+" ip="+requestIP(r), requestIP(r))
	go func() {
		if err := auth.SendResetEmail(req.Email, code); err != nil {
			log.Printf("[AUTH] Failed to send reset email: %v", err)
		}
	}()
	jsonResp(w, M{"ok": true, "message": "如果该邮箱已注册，重置邮件已发送至您的邮箱"})
}

// ─── POST /api/auth/reset-password ───
func handleResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Email    string `json:"email"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_json", "抱歉,提交的数据格式有误,请检查后重试~")})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Code = strings.TrimSpace(req.Code)
	req.Password = strings.TrimSpace(req.Password)
	if req.Email == "" || req.Code == "" || req.Password == "" {
		jsonResp(w, M{"ok": false, "error": "参数不完整"})
		return
	}
	if len(req.Password) < 6 {
		jsonResp(w, M{"ok": false, "error": "抱歉,密码至少需要 6 位,请重新设置~"})
		return
	}
	// 管理员账号禁止自助重置密码（与忘记密码入口一致的双保险）
	if u, e := db.GetUserByEmail(req.Email); e == nil && u != nil && u.Role == "admin" {
		_ = db.AddAuditLog(&u.ID, nil, "reset_password_admin_blocked", "email", req.Email+" ip="+requestIP(r), requestIP(r))
		jsonResp(w, M{"ok": false, "error": "管理员账号不支持自助重置密码，请联系其他管理员处理"})
		return
	}
	ok, err := db.ConsumeVerifyCode(req.Email, req.Code, "reset")
	if err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("system_error", "哎呀,系统开小差了,请稍后重试;仍不行请联系管理员~")})
		return
	}
	if !ok {
		jsonResp(w, M{"ok": false, "error": "哎呀,验证码错误或已过期,请重新获取后再试~"})
		return
	}
	user, err := db.GetUserByEmail(req.Email)
	if err == nil && user.Disabled {
		jsonResp(w, M{"ok": false, "error": "账号已被停用，无法重置密码"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("system_error", "哎呀,系统开小差了,请稍后重试;仍不行请联系管理员~")})
		return
	}
	if err := db.UpdatePasswordByEmail(req.Email, string(hash)); err != nil {
		jsonResp(w, M{"ok": false, "error": "重置失败"})
		return
	}
	_ = db.AddAuditLog(nil, nil, "reset_password", "email", req.Email, requestIP(r))
	jsonResp(w, M{"ok": true, "message": "密码已重置，请重新登录"})
}

// ─── POST /api/auth/change-email ───
func handleChangeEmail(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	if !changeEmailLimiter.Allow(requestIP(r)) {
		jsonResp(w, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
		return
	}
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_json", "抱歉,提交的数据格式有误,请检查后重试~")})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		jsonResp(w, M{"ok": false, "error": "请输入新邮箱"})
		return
	}
	if !isMainstreamEmail(req.Email) {
		jsonResp(w, M{"ok": false, "error": "仅支持主流邮箱"})
		return
	}
	exists, _ := db.IsEmailRegistered(req.Email)
	if exists {
		jsonResp(w, M{"ok": false, "error": "该邮箱已被使用"})
		return
	}
	code := generateCode()
	if err := db.CreateGenericVerifyCode(req.Email, code, "change_email"); err != nil {
		log.Printf("[AUTH] Failed to create change email code: %v", err)
		jsonResp(w, M{"ok": false, "error": msg.Get("system_error", "哎呀,系统开小差了,请稍后重试;仍不行请联系管理员~")})
		return
	}
	go func() {
		if err := auth.SendChangeEmailVerify(req.Email, code); err != nil {
			log.Printf("[AUTH] Failed to send change email verify: %v", err)
		}
	}()
	_ = db.AddAuditLog(&user.ID, nil, "change_email_sent", "email", req.Email, requestIP(r))
	jsonResp(w, M{"ok": true, "message": "验证码已发送到新邮箱"})
}

// ─── POST /api/auth/confirm-email ───
func handleConfirmEmail(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	if !changeEmailLimiter.Allow(requestIP(r)) {
		jsonResp(w, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
		return
	}
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_json", "抱歉,提交的数据格式有误,请检查后重试~")})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Code = strings.TrimSpace(req.Code)
	if req.Email == "" || req.Code == "" {
		jsonResp(w, M{"ok": false, "error": "请输入新邮箱和验证码"})
		return
	}
	ok, err := db.ConsumeVerifyCode(req.Email, req.Code, "change_email")
	if err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("system_error", "哎呀,系统开小差了,请稍后重试;仍不行请联系管理员~")})
		return
	}
	if !ok {
		jsonResp(w, M{"ok": false, "error": "哎呀,验证码错误或已过期,请重新获取后再试~"})
		return
	}
	if err := db.UpdateEmail(user.ID, req.Email); err != nil {
		jsonResp(w, M{"ok": false, "error": "修改失败"})
		return
	}
	_ = db.AddAuditLog(&user.ID, nil, "email_changed", "email", fmt.Sprintf("%s -> %s", user.Email, req.Email), requestIP(r))
	jsonResp(w, M{"ok": true, "message": "邮箱修改成功", "email": req.Email})
}

// ─── POST /api/accounts/{id}/nickname ───
func handleUpdateAccountNickname(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Nickname string `json:"nickname"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	req.Nickname = strings.TrimSpace(req.Nickname)
	if req.Nickname == "" {
		jsonResp(w, M{"ok": false, "error": "昵称不能为空"})
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/accounts/"), "/")
	var accountID int64
	fmt.Sscanf(parts[0], "%d", &accountID)
	if accountID == 0 {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_id", "糟糕,账号编号无效或已失效,请刷新后重试~")})
		return
	}

	acc, err := db.GetAccountByID(accountID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
		return
	}
	if user.Role != "admin" && (acc.OwnerID == nil || *acc.OwnerID != user.ID) {
		jsonResp(w, M{"ok": false, "error": "无权操作此账号"})
		return
	}
	if acc.CookieData == "" {
		jsonResp(w, M{"ok": false, "error": "该账号没有 Cookie，无法修改昵称"})
		return
	}

	gc, g79Err := newG79ClientWithProxy(accountID)
	if g79Err != nil {
		jsonResp(w, M{"ok": false, "error": "创建客户端失败"})
		return
	}
	if err := gc.G79AuthenticateWithCookie(acc.CookieData); err != nil {
		jsonResp(w, M{"ok": false, "error": "Cookie 认证失败，请刷新账号"})
		return
	}
	if err := gc.UpdateNickname(req.Nickname); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "占用") || strings.Contains(errMsg, "已存在") {
			jsonResp(w, M{"ok": false, "error": "该昵称已被占用", "code": "nickname_taken"})
		} else if strings.Contains(errMsg, "合规") || strings.Contains(errMsg, "敏感") || strings.Contains(errMsg, "非法") {
			jsonResp(w, M{"ok": false, "error": "昵称不合规，请修改", "code": "nickname_invalid"})
		} else {
			jsonResp(w, M{"ok": false, "error": "修改失败: " + errMsg})
		}
		return
	}

	if err := db.UpdateAccountName(accountID, req.Nickname); err != nil {
		log.Printf("[ACCOUNT] Nickname updated in game but failed to update DB: %v", err)
	}
	_ = db.AddAuditLog(&user.ID, &accountID, "update_account_nickname", "game_account", req.Nickname, requestIP(r))
	log.Printf("[ACCOUNT] Nickname updated: account=%d nickname=%s", accountID, req.Nickname)
	jsonResp(w, M{"ok": true, "message": "昵称修改成功", "display_name": req.Nickname})
}

// ─── POST /api/auth/activate ───
func handleActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_json", "抱歉,提交的数据格式有误,请检查后重试~")})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Code == "" {
		jsonResp(w, M{"ok": false, "error": "邮箱和激活码不能为空"})
		return
	}
	user, err := db.GetUserByEmail(req.Email)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "邮箱不存在"})
		return
	}
	if user.Disabled {
		jsonResp(w, M{"ok": false, "error": "账号已被停用"})
		return
	}
	if user.Activated {
		jsonResp(w, M{"ok": true, "message": "账号已激活"})
		return
	}
	if !db.ValidateActivationKey(req.Code) {
		jsonResp(w, M{"ok": false, "error": "激活码无效"})
		return
	}
	db.ActivateUser(user.ID)
	_ = db.AddAuditLog(&user.ID, nil, "activate", "web_user", "激活账号", requestIP(r))
	log.Printf("[AUTH] User %s (%s) activated", user.Username, user.Email)
	jsonResp(w, M{"ok": true, "message": "激活成功，请重新登录"})
}

// normalizeCookie 统一转为 sauth_json 格式：扁平格式自动提取字段打包
func normalizeCookie(raw string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw // 不是 JSON，原样返回
	}
	if _, ok := m["sauth_json"]; ok {
		return raw // 已有 sauth_json，原样返回
	}
	// 扁平格式：提取需要字段，打包成 sauth_json
	fields := []string{"sdkuid", "sessionid", "udid", "deviceid", "gameid", "platform", "sdk_version", "app_channel", "login_channel", "nickname", "aim_info", "client_login_sn", "source_platform", "ip", "is_unisdk_guest", "get_access_token", "access_token", "source_app_channel", "step", "step2"}
	inner := make(map[string]any)
	for _, k := range fields {
		if v, exists := m[k]; exists {
			inner[k] = v
		}
	}
	innerJSON, _ := json.Marshal(inner)
	packed, _ := json.Marshal(M{"sauth_json": string(innerJSON)})
	return string(packed)
}

// ─── POST /api/accounts/{id}/server-owner ───
func handleSetServerOwner(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	// 从路径中提取 ID
	parts := strings.Split(strings.TrimRight(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		jsonResp(w, M{"ok": false, "error": "缺少账号ID"})
		return
	}
	accID, err := strconv.ParseInt(parts[len(parts)-2], 10, 64)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_id", "糟糕,账号编号无效或已失效,请刷新后重试~")})
		return
	}
	log.Printf("[SET-OWNER] path=%s parts=%v accID=%d", r.URL.Path, parts, accID)

	var req struct {
		Enabled bool `json:"enabled"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	acc, err := db.GetAccountByID(accID)
	if err != nil {
		log.Printf("[SET-OWNER] GetAccountByID(%d) error: %v", accID, err)
		jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
		return
	}

	// 权限检查：必须是自己的私有账号
	if acc.OwnerID == nil || *acc.OwnerID != user.ID {
		jsonResp(w, M{"ok": false, "error": "只能操作自己的私有账号"})
		return
	}

	if req.Enabled {
		// 启用：检查同 UID 是否被其他用户占用
		other, err := db.HasOtherUserWithSameUID(acc.UID, user.ID)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": msg.Get("system_error", "哎呀,系统开小差了,请稍后重试;仍不行请联系管理员~")})
			return
		}
		if other {
			jsonResp(w, M{"ok": false, "error": "该账号已被其他用户占用，无法设为服主账号"})
			return
		}
		// 停止心跳
		stopAccountHeartbeat(accID)
		// 如果该账号是当前活跃账号，清除活跃账号标记
		if user.ActiveAccountID != nil && *user.ActiveAccountID == accID {
			db.ClearActiveAccount(user.ID)
			user.ActiveAccountID = nil
			log.Printf("[SET-OWNER] cleared active account #%d because it's now a server owner", accID)
		}
	}

	if err := db.SetServerOwner(accID, req.Enabled); err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}

	action := "已取消服主标记"
	if req.Enabled {
		action = "已设为服主账号"
	}
	jsonResp(w, M{"ok": true, "message": action})
}

// ─── GET /api/accounts/server-owners ───
func handleListServerOwners(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	accs, err := db.GetServerOwnerAccounts(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	type ownerResp struct {
		ID          int64  `json:"id"`
		DisplayName string `json:"display_name"`
		UID         string `json:"uid"`
	}
	var result []ownerResp
	for _, a := range accs {
		result = append(result, ownerResp{ID: a.ID, DisplayName: a.DisplayName, UID: a.UID})
	}
	jsonResp(w, M{"ok": true, "accounts": result})
}
