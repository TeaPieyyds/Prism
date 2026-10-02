package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
)

// 进服密码错误冷却：普通用户传错房间密码后，需等待 joinCooldownDur 才能再次尝试，
// 期间其进服相关请求返回 HTTP 429。管理员豁免。冷却仅存内存，重启即清零。
var (
	joinCooldownMu sync.Mutex
	joinCooldown   = map[string]time.Time{}
)

const joinCooldownDur = 10 * time.Second

// joinCooldownKey 返回该用户的冷却键；admin 或无法解析时返回 ""(表示豁免/不参与)。
func joinCooldownKey(user *db.User) string {
	if user == nil || user.Role == "admin" {
		return ""
	}
	// 游客没有唯一 user.ID，按当前游客游戏账号键控。
	if user.ID == auth.GuestUserID && user.ActiveAccountID != nil {
		return fmt.Sprintf("g%d", *user.ActiveAccountID)
	}
	return fmt.Sprintf("u%d", user.ID)
}

// inJoinCooldown 报告该用户当前是否处于密码错误冷却期。
func inJoinCooldown(user *db.User) bool {
	key := joinCooldownKey(user)
	if key == "" {
		return false
	}
	joinCooldownMu.Lock()
	defer joinCooldownMu.Unlock()
	return time.Now().Before(joinCooldown[key])
}

// markJoinCooldown 为该用户开启一段 10 秒冷却(仅当确实发生密码错误)。
func markJoinCooldown(user *db.User) {
	key := joinCooldownKey(user)
	if key == "" {
		return
	}
	joinCooldownMu.Lock()
	joinCooldown[key] = time.Now().Add(joinCooldownDur)
	joinCooldownMu.Unlock()
}

// isPasswordError 按响应文案判断是否为密码/口令错误。
func isPasswordError(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "口令") || strings.Contains(m, "密码") ||
		strings.Contains(m, "password") || strings.Contains(m, "wrong pass")
}

// writeTooMany 以 HTTP 429 返回密码错误冷却提示。
func writeTooMany(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusTooManyRequests)
	jsonResp(w, M{"ok": false, "error": "糟糕,密码错误,请10秒后再试~"})
}
