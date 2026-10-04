package auth

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/adb-lanlu/prism-oss/internal/db"
)

type Config struct {
	AdminEmail   string  `json:"admin_email"`
	DBPath       string  `json:"db_path"`
	BaseURL      string  `json:"base_url"`
	SMTPHost     string  `json:"smtp_host"`
	SMTPPort     int     `json:"smtp_port"`
	SMTPUser     string  `json:"smtp_user"`
	SMTPPass     string  `json:"smtp_pass"`
	SMTPFrom     string  `json:"smtp_from"`
	PayPID       string  `json:"pay_pid"`
	PayKey       string  `json:"pay_key"`
	PayAPIBase   string  `json:"pay_api_base"`
	PayMinAmount float64 `json:"pay_min_amount"`
	PayNutsRate  int     `json:"pay_nuts_rate"`
	PayNotifyURL string `json:"pay_notify_url"`
}

var Cfg Config

func LoadConfig(path string) Config {
	cfg := Config{
		DBPath:       "phoenix.db",
		BaseURL:      "http://localhost:8081",
		SMTPHost:     "smtp.163.com",
		SMTPPort:     994,
		SMTPFrom:     "onboarding@resend.dev",
		PayMinAmount: 1.0,
		PayNutsRate:  10,
	}

	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &cfg)
	}

	// Env override
	if v := os.Getenv("PHX_ADMIN_EMAIL"); v != "" {
		cfg.AdminEmail = v
	}
	if v := os.Getenv("PHX_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("PHX_BASE_URL"); v != "" {
		cfg.BaseURL = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("PHX_SMTP_HOST"); v != "" {
		cfg.SMTPHost = v
	}
	if v := os.Getenv("PHX_SMTP_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.SMTPPort)
	}
	if v := os.Getenv("PHX_SMTP_USER"); v != "" {
		cfg.SMTPUser = v
	}
	if v := os.Getenv("PHX_SMTP_PASS"); v != "" {
		cfg.SMTPPass = v
	}
	if v := os.Getenv("PHX_PAY_PID"); v != "" {
		cfg.PayPID = v
	}
	if v := os.Getenv("PHX_PAY_KEY"); v != "" {
		cfg.PayKey = v
	}
	if v := os.Getenv("PHX_PAY_API_BASE"); v != "" {
		cfg.PayAPIBase = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("PHX_PAY_MIN_AMOUNT"); v != "" {
		fmt.Sscanf(v, "%f", &cfg.PayMinAmount)
	}
	if v := os.Getenv("PHX_PAY_NUTS_RATE"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.PayNutsRate)
	}
	if v := os.Getenv("PHX_PAY_NOTIFY_URL"); v != "" {
		cfg.PayNotifyURL = v
	}

	Cfg = cfg
	if Cfg.BaseURL == "" {
		Cfg.BaseURL = "http://localhost:8081"
	}
	Cfg.BaseURL = strings.TrimRight(Cfg.BaseURL, "/")
	return cfg
}

// SendVerifyEmail sends a verification email with a link
func SendVerifyEmail(to, code string) error {
	if Cfg.SMTPHost == "" || Cfg.SMTPUser == "" {
		return fmt.Errorf("SMTP not configured")
	}

	host := Cfg.SMTPHost
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}

	verifyLink := fmt.Sprintf("%s/api/auth/verify?email=%s&code=%s", Cfg.BaseURL, url.QueryEscape(to), url.QueryEscape(code))
	subject := "Prism - 请验证您的邮箱"
	plainBody := fmt.Sprintf(`Prism 邮箱验证

您好，欢迎使用 Prism。
请打开下面的链接完成邮箱验证：

%s

链接将在 30 分钟后过期。如果这不是您本人的操作，可以忽略这封邮件。`, verifyLink)
	htmlBody := fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0"></head>
<body style="margin:0;padding:0;background:#f8f3e8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','Noto Sans SC',Arial,sans-serif;color:#5d4b36;">
  <div style="max-width:560px;margin:0 auto;padding:32px 16px;">
    <div style="background:#fffaf0;border:2px solid #eadfc9;border-radius:24px;box-shadow:0 12px 32px rgba(93,75,54,.12);overflow:hidden;">
      <div style="background:linear-gradient(135deg,#a7d7c5,#f7d794);padding:28px 28px 22px;text-align:center;">
        <div style="font-size:42px;line-height:1;margin-bottom:8px;">🦉</div>
        <h1 style="margin:0;font-size:24px;color:#4b3a29;letter-spacing:.2px;">Prism</h1>
        <p style="margin:8px 0 0;color:#6d5a41;font-size:14px;">验证你的邮箱，开启账号管理后台</p>
      </div>
      <div style="padding:30px 28px;">
        <p style="font-size:16px;line-height:1.8;margin:0 0 18px;">你好，</p>
        <p style="font-size:15px;line-height:1.8;margin:0 0 24px;">请点击下方按钮完成邮箱验证。验证成功后，你就可以使用自己的 <b style="color:#19a79b;">adb// Token</b> 管理独立账号与共享账号池。</p>
        <div style="text-align:center;margin:28px 0;">
          <a href="%s" style="display:inline-block;background:#19c8b9;color:#fff;text-decoration:none;font-size:16px;font-weight:800;padding:14px 28px;border-radius:999px;box-shadow:0 8px 18px rgba(25,200,185,.28);">完成邮箱验证</a>
        </div>
        <p style="font-size:13px;line-height:1.7;color:#8b7a61;margin:0 0 10px;">如果按钮无法打开，请复制下面的链接到浏览器：</p>
        <div style="background:#f4ead9;border:1px dashed #d8c9aa;border-radius:14px;padding:12px;font-size:12px;line-height:1.6;word-break:break-all;color:#6d5a41;">%s</div>
        <p style="font-size:12px;color:#9f927d;margin:20px 0 0;">链接将在 30 分钟后过期。如果这不是你本人的操作，请忽略这封邮件。</p>
      </div>
    </div>
    <div style="text-align:center;color:#b5a58a;font-size:12px;margin-top:16px;">Prism</div>
  </div>
</body>
</html>`, verifyLink, verifyLink)

	boundary := "prism-boundary"
	encodedSubject := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(subject)) + "?="
	msg := fmt.Sprintf("From: Prism <%s>\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n--%s\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n--%s\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n--%s--\r\n",
		Cfg.SMTPFrom, to, encodedSubject, boundary, boundary, plainBody, boundary, htmlBody, boundary)

	addr := fmt.Sprintf("%s:%d", Cfg.SMTPHost, Cfg.SMTPPort)
	auth := smtp.PlainAuth("", Cfg.SMTPUser, Cfg.SMTPPass, host)

	// SSL/TLS connection
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err = client.Mail(Cfg.SMTPFrom); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	_, err = w.Write([]byte(msg))
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}

	log.Printf("[SMTP] Verification email sent to %s", to)
	return nil
}

func SendAdminNewUserEmail(email, username string) error {
	if Cfg.AdminEmail == "" {
		return nil
	}
	if Cfg.SMTPHost == "" || Cfg.SMTPUser == "" {
		return fmt.Errorf("SMTP not configured")
	}
	host := Cfg.SMTPHost
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}
	subject := "Prism - 新用户注册通知"
	plainBody := fmt.Sprintf("新用户已完成邮箱验证\n\n用户名：%s\n邮箱：%s\n时间：%s", username, email, time.Now().Format(time.RFC3339))
	htmlBody := fmt.Sprintf(`<!doctype html><html lang="zh-CN"><body style="margin:0;padding:28px;background:#f8f3e8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','Noto Sans SC',sans-serif;color:#4b3a29;"><div style="max-width:560px;margin:auto;background:#fffaf0;border:2px solid #eadfc9;border-radius:22px;padding:26px;box-shadow:0 12px 32px rgba(93,75,54,.12);"><h2 style="margin:0 0 16px;">Prism 新用户注册</h2><p>有新用户完成邮箱验证并注册成功。</p><div style="background:#f4ead9;border-radius:14px;padding:14px;margin-top:16px;"><p><b>用户名：</b>%s</p><p><b>邮箱：</b>%s</p><p><b>时间：</b>%s</p></div></div></body></html>`, username, email, time.Now().Format(time.RFC3339))

	boundary := "prism-boundary"
	encodedSubject := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(subject)) + "?="
	msg := fmt.Sprintf("From: Prism <%s>\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n--%s\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n--%s\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n--%s--\r\n",
		Cfg.SMTPFrom, Cfg.AdminEmail, encodedSubject, boundary, boundary, plainBody, boundary, htmlBody, boundary)
	addr := fmt.Sprintf("%s:%d", Cfg.SMTPHost, Cfg.SMTPPort)
	smtpAuth := smtp.PlainAuth("", Cfg.SMTPUser, Cfg.SMTPPass, host)
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, InsecureSkipVerify: true})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()
	if err = client.Auth(smtpAuth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err = client.Mail(Cfg.SMTPFrom); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err = client.Rcpt(Cfg.AdminEmail); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err = w.Write([]byte(msg)); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	log.Printf("[SMTP] New user notification sent to %s", Cfg.AdminEmail)
	return nil
}

// GuestUserID is the sentinel user ID for anonymous/guest users
const GuestUserID int64 = 0

// GuestUser returns a virtual user representing an anonymous guest.
func GuestUser() *db.User {
	return &db.User{
		ID:       GuestUserID,
		Username: "anonymous",
		Role:     "guest",
		Verified: true,
	}
}

// IsGuest reports whether the user is the virtual guest user.
func IsGuest(user *db.User) bool {
	return user != nil && user.ID == GuestUserID
}

// SessionAuth is HTTP middleware that checks Bearer token (user's API token)
// and the HttpOnly session cookie. Unauthenticated requests get 401.
func SessionAuth(next http.HandlerFunc) http.HandlerFunc {
	return sessionAuth("哎呀,你还没登录,请先登录后再操作~", next)
}

// SessionAuthCustom is like SessionAuth but returns a custom message when the
// request carries no valid credentials. Used for endpoints that need a
// security-specific hint.
func SessionAuthCustom(msg string, next http.HandlerFunc) http.HandlerFunc {
	return sessionAuth(msg, next)
}

func sessionAuth(unauthorizedMsg string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var user *db.User

		// 1st: HttpOnly session cookie (web)
		if c, cookieErr := r.Cookie("phx_sid"); cookieErr == nil && c.Value != "" {
			user, _ = db.GetUserBySessionCookie(c.Value)
		}

		// 2nd: Bearer token (API clients)
		var apiToken *db.APIToken
		if user == nil {
			if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
				token := strings.TrimPrefix(a, "Bearer ")
				if u, tok, err := db.ResolveToken(token); err == nil && u != nil {
					user = u
					apiToken = tok
				}
			}
		}

		if user == nil {
			http.Error(w, `{"ok":false,"error":"`+unauthorizedMsg+`"}`, http.StatusUnauthorized)
			return
		}
		if user.Disabled {
			http.Error(w, `{"ok":false,"error":"抱歉,你的账号已被停用,如有疑问请联系管理员~"}`, http.StatusForbidden)
			return
		}
		// 子令牌不继承管理员身份：以子令牌鉴权时把角色降级为普通用户，
		// 防止管理员的子令牌获得 admin 权限。
		if apiToken != nil && user.Role == "admin" {
			cp := *user
			cp.Role = "user"
			user = &cp
		}
		ctx := context.WithValue(r.Context(), ctxKeyUser, user)
		if apiToken != nil {
			ctx = context.WithValue(ctx, ctxKeyAPIToken, apiToken)
		}
		next(w, r.WithContext(ctx))
	}
}

// OptionalSessionAuth 软鉴权:尝试识别用户(会话 cookie 或 Bearer token)并放入 context,
// 但无有效凭据时也放行(handler 内部自行判断)。用于需要可选登录的公开接口(如市场浏览/下载),
// 付费/敏感操作在 handler 内依据 user == nil 拒绝。
func OptionalSessionAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var user *db.User
		if c, cookieErr := r.Cookie("phx_sid"); cookieErr == nil && c.Value != "" {
			user, _ = db.GetUserBySessionCookie(c.Value)
		}
		var apiToken *db.APIToken
		if user == nil {
			if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
				token := strings.TrimPrefix(a, "Bearer ")
				if u, tok, err := db.ResolveToken(token); err == nil && u != nil {
					user = u
					apiToken = tok
				}
			}
		}
		ctx := r.Context()
		if user != nil {
			if apiToken != nil && user.Role == "admin" {
				cp := *user
				cp.Role = "user"
				user = &cp
			}
			ctx = context.WithValue(ctx, ctxKeyUser, user)
			if apiToken != nil {
				ctx = context.WithValue(ctx, ctxKeyAPIToken, apiToken)
			}
		}
		next(w, r.WithContext(ctx))
	}
}

// PrimaryToken 拒绝用子令牌鉴权的请求，强制使用主令牌/网页会话。用于敏感操作。
func PrimaryToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if GetAPIToken(r.Context()) != nil {
			http.Error(w, `{"ok":false,"error":"抱歉,此操作需使用主令牌鉴权,请用主令牌重试~"}`, http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// GetAPIToken returns the secondary API token used to authenticate the request,
// or nil when authenticated via the primary token / web session cookie.
func GetAPIToken(ctx context.Context) *db.APIToken {
	v := ctx.Value(ctxKeyAPIToken)
	if v == nil {
		return nil
	}
	return v.(*db.APIToken)
}

// AdminAuth wraps SessionAuth and additionally checks admin role
func AdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return SessionAuth(func(w http.ResponseWriter, r *http.Request) {
		user := GetUser(r.Context())
		if user == nil || user.Role != "admin" {
			http.Error(w, `{"ok":false,"error":"抱歉,此操作需要管理员权限,当前账号无权执行~"}`, http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

type ctxKeyType string

const ctxKeyUser ctxKeyType = "user"
const ctxKeyAPIToken ctxKeyType = "api_token"

func GetUser(ctx context.Context) *db.User {
	v := ctx.Value(ctxKeyUser)
	if v == nil {
		return nil
	}
	return v.(*db.User)
}

// Rate limit helper
type rateEntry struct { count int; windowAt time.Time }

var (
	rateLimitersMu sync.Mutex
	rateLimiters   = []*RateLimiter{}
	globalLimiter  *GlobalIPLimiter
)

type RateLimiter struct {
	mu	sync.Mutex
	visitors map[string]*rateEntry
	burst    int
	window   time.Duration
}

func NewRateLimiter(burst int, window time.Duration) *RateLimiter {
	l := &RateLimiter{
		visitors: make(map[string]*rateEntry),
		burst:    burst,
		window:   window,
	}
	rateLimitersMu.Lock()
	rateLimiters = append(rateLimiters, l)
	rateLimitersMu.Unlock()
	return l
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	e, ok := rl.visitors[key]
	if !ok || now.Sub(e.windowAt) > rl.window {
		rl.visitors[key] = &rateEntry{count: 1, windowAt: now}
		return true
	}
	if e.count < rl.burst {
		e.count++
		return true
	}
	return false
}

// Wait blocks until window resets, then returns
func (rl *RateLimiter) Wait(key string) {
	for !rl.Allow(key) {
		time.Sleep(200 * time.Millisecond)
	}
}

func (rl *RateLimiter) purgeStale(now time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for k, e := range rl.visitors {
		if now.Sub(e.windowAt) > rl.window {
			delete(rl.visitors, k)
		}
	}
}

// GlobalIPLimiter bans IPs that exceed burst requests per window.
type GlobalIPLimiter struct {
	mu       sync.Mutex
	visitors map[string]*banEntry
	burst    int
	window   time.Duration
	banDur   time.Duration
}

type banEntry struct {
	count    int
	windowAt time.Time
	bannedUntil time.Time
}

func NewGlobalIPLimiter(burst int, window, banDur time.Duration) *GlobalIPLimiter {
	return &GlobalIPLimiter{
		visitors: make(map[string]*banEntry),
		burst:    burst,
		window:   window,
		banDur:   banDur,
	}
}

func (gl *GlobalIPLimiter) Allow(ip string) (bool, string) {
	if ip == "127.0.0.1" || ip == "::1" || ip == "localhost" {
		return true, ""
	}
	gl.mu.Lock()
	defer gl.mu.Unlock()
	now := time.Now()
	e, ok := gl.visitors[ip]
	if !ok {
		gl.visitors[ip] = &banEntry{count: 1, windowAt: now}
		return true, ""
	}
	if now.Before(e.bannedUntil) {
		remaining := e.bannedUntil.Sub(now).Round(time.Second)
		return false, fmt.Sprintf("请求过于频繁，IP 已被临时限制，剩余 %v", remaining)
	}
	if now.Sub(e.windowAt) > gl.window {
		e.count = 1
		e.windowAt = now
		return true, ""
	}
	e.count++
	if e.count > gl.burst {
		e.bannedUntil = now.Add(gl.banDur)
		return false, fmt.Sprintf("请求过于频繁，IP 已被临时限制 %v", gl.banDur)
	}
	return true, ""
}

func (gl *GlobalIPLimiter) purgeStale(now time.Time) {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	for k, e := range gl.visitors {
		if now.After(e.bannedUntil) && now.Sub(e.windowAt) > gl.window {
			delete(gl.visitors, k)
		}
	}
}

func JSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// Get preferred server IP for verification links
func GetServerAddr(r *http.Request) string {
	host := r.Host
	if host == "" {
		host = "localhost:8081"
	}
	return host
}

// CheckPort checks if a port is available
func CheckPort(port string) bool {
	conn, err := net.DialTimeout("tcp", ":"+port, time.Second)
	if err != nil {
		return true // port is free
	}
	conn.Close()
	return false
}


func SendResetEmail(to, code string) error {
	if Cfg.SMTPHost == "" || Cfg.SMTPUser == "" {
		return fmt.Errorf("SMTP not configured")
	}
	host := Cfg.SMTPHost
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}
	link := fmt.Sprintf("%s/#/forgot-password?email=%s&code=%s", Cfg.BaseURL, url.QueryEscape(to), url.QueryEscape(code))
	subject := "Prism - 密码重置"
	plainBody := fmt.Sprintf(`Prism 密码重置

您好，
请打开下面的链接重置密码：

%s

链接将在 30 分钟后过期。如果这不是您本人的操作，请忽略这封邮件。`, link)
	htmlBody := fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0"></head>
<body style="margin:0;padding:0;background:#f8f3e8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','Noto Sans SC',Arial,sans-serif;color:#5d4b36;">
  <div style="max-width:560px;margin:0 auto;padding:32px 16px;">
    <div style="background:#fffaf0;border:2px solid #eadfc9;border-radius:24px;box-shadow:0 12px 32px rgba(93,75,54,.12);overflow:hidden;">
      <div style="background:linear-gradient(135deg,#a7d7c5,#f7d794);padding:28px 28px 22px;text-align:center;">
        <div style="font-size:42px;line-height:1;margin-bottom:8px;">🦉</div>
        <h1 style="margin:0;font-size:24px;color:#4b3a29;letter-spacing:.2px;">Prism</h1>
        <p style="margin:8px 0 0;color:#6d5a41;font-size:14px;">重置你的密码</p>
      </div>
      <div style="padding:30px 28px;">
        <p style="font-size:16px;line-height:1.8;margin:0 0 18px;">你好，</p>
        <p style="font-size:15px;line-height:1.8;margin:0 0 24px;">请点击下方按钮重置密码。如果这不是你本人的操作，请忽略这封邮件。</p>
        <div style="text-align:center;margin:28px 0;">
          <a href="%s" style="display:inline-block;background:#19c8b9;color:#fff;text-decoration:none;font-size:16px;font-weight:800;padding:14px 28px;border-radius:999px;box-shadow:0 8px 18px rgba(25,200,185,.28);">重置密码</a>
        </div>
        <p style="font-size:13px;line-height:1.7;color:#8b7a61;margin:0 0 10px;">如果按钮无法打开，请复制下面的链接到浏览器：</p>
        <div style="background:#f4ead9;border:1px dashed #d8c9aa;border-radius:14px;padding:12px;font-size:12px;line-height:1.6;word-break:break-all;color:#6d5a41;">%s</div>
        <p style="font-size:12px;color:#9f927d;margin:20px 0 0;">链接将在 30 分钟后过期。</p>
      </div>
    </div>
    <div style="text-align:center;color:#b5a58a;font-size:12px;margin-top:16px;">Prism</div>
  </div>
</body>
</html>`, link, link)

	boundary := "prism-reset-boundary"
	encodedSubject := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(subject)) + "?="
	msg := fmt.Sprintf("From: Prism <%s>\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n--%s\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n--%s\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n--%s--\r\n",
		Cfg.SMTPFrom, to, encodedSubject, boundary, boundary, plainBody, boundary, htmlBody, boundary)
	addr := fmt.Sprintf("%s:%d", Cfg.SMTPHost, Cfg.SMTPPort)
	smtpAuth := smtp.PlainAuth("", Cfg.SMTPUser, Cfg.SMTPPass, host)
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, InsecureSkipVerify: true})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()
	cli, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer cli.Close()
	if err = cli.Auth(smtpAuth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err = cli.Mail(Cfg.SMTPFrom); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err = cli.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := cli.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err = w.Write([]byte(msg)); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	log.Printf("[SMTP] Reset email sent to %s", to)
	return nil
}


func SendChangeEmailVerify(to, code string) error {
	if Cfg.SMTPHost == "" || Cfg.SMTPUser == "" {
		return fmt.Errorf("SMTP not configured")
	}
	host := Cfg.SMTPHost
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}
	subject := "Prism - 邮箱修改验证"
	plainBody := fmt.Sprintf(`Prism 邮箱修改验证

您好，
请在修改邮箱页面输入以下验证码完成邮箱修改：

验证码：%s

验证码将在 30 分钟后过期。如果这不是您本人的操作，请忽略这封邮件。`, code)
	htmlBody := fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0"></head>
<body style="margin:0;padding:0;background:#f8f3e8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','Noto Sans SC',Arial,sans-serif;color:#5d4b36;">
  <div style="max-width:560px;margin:0 auto;padding:32px 16px;">
    <div style="background:#fffaf0;border:2px solid #eadfc9;border-radius:24px;box-shadow:0 12px 32px rgba(93,75,54,.12);overflow:hidden;">
      <div style="background:linear-gradient(135deg,#a7d7c5,#f7d794);padding:28px 28px 22px;text-align:center;">
        <div style="font-size:42px;line-height:1;margin-bottom:8px;">🦉</div>
        <h1 style="margin:0;font-size:24px;color:#4b3a29;letter-spacing:.2px;">Prism</h1>
        <p style="margin:8px 0 0;color:#6d5a41;font-size:14px;">邮箱修改验证</p>
      </div>
      <div style="padding:30px 28px;">
        <p style="font-size:16px;line-height:1.8;margin:0 0 18px;">你好，</p>
        <p style="font-size:15px;line-height:1.8;margin:0 0 24px;">请在修改邮箱页面输入以下验证码：</p>
        <div style="text-align:center;margin:28px 0;padding:16px;background:#f4ead9;border-radius:14px;font-size:32px;font-weight:800;letter-spacing:6px;color:#19a79b;">%s</div>
        <p style="font-size:12px;color:#9f927d;margin:20px 0 0;">验证码将在 30 分钟后过期。如果这不是你本人的操作，请忽略这封邮件。</p>
      </div>
    </div>
    <div style="text-align:center;color:#b5a58a;font-size:12px;margin-top:16px;">Prism</div>
  </div>
</body>
</html>`, code)

	boundary := "prism-change-email-boundary"
	encodedSubject := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(subject)) + "?="
	msg := fmt.Sprintf("From: Prism <%s>\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n--%s\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n--%s\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n--%s--\r\n",
		Cfg.SMTPFrom, to, encodedSubject, boundary, boundary, plainBody, boundary, htmlBody, boundary)
	addr := fmt.Sprintf("%s:%d", Cfg.SMTPHost, Cfg.SMTPPort)
	smtpAuth := smtp.PlainAuth("", Cfg.SMTPUser, Cfg.SMTPPass, host)
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, InsecureSkipVerify: true})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer conn.Close()
	cli, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer cli.Close()
	if err = cli.Auth(smtpAuth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err = cli.Mail(Cfg.SMTPFrom); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err = cli.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := cli.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err = w.Write([]byte(msg)); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	log.Printf("[SMTP] Change email verify sent to %s", to)
	return nil
}

// RegisterGlobalLimiter 供全局 IP 限流器注册，统一参与清扫。
func RegisterGlobalLimiter(gl *GlobalIPLimiter) {
	rateLimitersMu.Lock()
	globalLimiter = gl
	rateLimitersMu.Unlock()
}

// StartRateLimiterCleanup 启动后台清扫，清理过期限流条目，防止内存无限增长。
func StartRateLimiterCleanup(stop <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			rateLimitersMu.Lock()
			for _, l := range rateLimiters {
				l.purgeStale(now)
			}
			if globalLimiter != nil {
				globalLimiter.purgeStale(now)
			}
			rateLimitersMu.Unlock()
		}
	}
}
