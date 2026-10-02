package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/adb-lanlu/prism-oss/internal/auth"
)

// maxRequestBodyBytes 请求体大小上限（默认 1MB；上传端点单独放宽）。
var maxRequestBodyBytes int64 = 1 << 20

// ---- panic 恢复 ----
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[PANIC] %v path=%s", rec, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(M{"ok": false, "error": "服务器内部错误"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ---- 真实 IP 入 context ----
type realIPKeyType struct{}

var realIPKey = realIPKeyType{}

func realIPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := resolveRealIP(r)
		ctx := context.WithValue(r.Context(), realIPKey, ip)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func clientIP(ctx context.Context) string {
	if v, ok := ctx.Value(realIPKey).(string); ok && v != "" {
		return v
	}
	return ""
}

// resolveRealIP 集中可信头链解析，默认顺序与 requestIP 一致：
// X-Real-IP → X-Forwarded-For 末项 → RemoteAddr。
func resolveRealIP(r *http.Request) string {
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

// ---- 请求体大小限制 ----
func limitBody(maxBytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// frameOptionsPolicy 控制 X-Frame-Options。
// 默认空字符串 = 不发送该头，允许本站被内联界面(iframe)嵌入——这是业务需求，
// 大量内联界面需要嵌本站。若某环境无需嵌入且要防点击劫持，可设为 "SAMEORIGIN" 或 "DENY"。
var frameOptionsPolicy = ""

// ---- 安全响应头 ----
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		if frameOptionsPolicy != "" {
			h.Set("X-Frame-Options", frameOptionsPolicy)
		}
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// ---- 全局 IP 限流 ----
func globalRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r.Context())
		if ip == "" {
			ip = requestIP(r)
		}
		if ok, msg := globalIPLimiter.Allow(ip); !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(M{"ok": false, "error": msg})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// limited 为未鉴权端点提供端点级独立限流（本地前置拦截，仅拒绝超额）。
func limited(rl *auth.RateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r.Context())
		if ip == "" {
			ip = requestIP(r)
		}
		if !rl.Allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(M{"ok": false, "error": "请求过于频繁，请稍后再试"})
			return
		}
		next(w, r)
	}
}
