package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/adb-lanlu/prism-oss/internal/db"
)

// sessionCookieSecure 当 TLS 在 Go 层直接终止时设为 true；CDN/反代终止 TLS 时保持 false，
// 避免 Go 层通过纯 HTTP 下发 Secure cookie 导致登录失效。由部署方按环境调整。
var sessionCookieSecure = false

// SetSessionCookie issues an HttpOnly cookie after successful login
func SetSessionCookie(w http.ResponseWriter, userID int64) {
	b := make([]byte, 32) // 提升熵：16 → 32 字节
	rand.Read(b)
	sid := hex.EncodeToString(b)
	db.SetSession(sid, userID)
	http.SetCookie(w, &http.Cookie{
		Name:     "phx_sid",
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   sessionCookieSecure, // 由部署开关控制（TLS 在边缘时关闭）
		MaxAge:   86400 * 30,
	})
}
