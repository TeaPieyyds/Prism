package main

import (
	"net/http"
	"strconv"

	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
)

// ── User stats handlers (SessionAuth) ──

func handleUserStatsSummary(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 {
		hours = 168 // default 7 days
	}
	s, err := db.GetUserStatsSummary(user.ID, hours)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "summary": s})
}

func handleUserEndpointStats(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 {
		hours = 168
	}
	granularity := r.URL.Query().Get("granularity")
	if granularity == "" {
		granularity = "hour"
	}
	stats, err := db.GetUserEndpointStats(user.ID, granularity, hours)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "endpoints": stats})
}

func handleUserServerStats(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 {
		hours = 720 // default 30 days
	}
	stats, err := db.GetUserServerJoinStats(user.ID, hours)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "servers": stats})
}

func handleUserNutsTimeline(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 {
		hours = 720
	}
	points, err := db.GetUserNutsTimeline(user.ID, hours)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "points": points})
}

func handleUserActiveEndpoints(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	eps, err := db.GetUserActiveEndpoints(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "endpoints": eps})
}

// ── Admin stats handlers (AdminAuth) ──

// parseExcludeTest reads the "exclude_test" query param (default 1 = exclude).
func parseExcludeTest(r *http.Request) bool {
	return r.URL.Query().Get("exclude_test") != "0"
}

func handleAdminStatsOverview(w http.ResponseWriter, r *http.Request) {
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 {
		hours = 168
	}
	o, err := db.GetAdminOverview(hours, parseExcludeTest(r))
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "overview": o})
}

func handleAdminEndpointStats(w http.ResponseWriter, r *http.Request) {
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 {
		hours = 168
	}
	granularity := r.URL.Query().Get("granularity")
	if granularity == "" {
		granularity = "hour"
	}
	stats, err := db.GetAdminEndpointStats(granularity, hours, parseExcludeTest(r))
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "endpoints": stats})
}

func handleAdminPerUserStats(w http.ResponseWriter, r *http.Request) {
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 {
		hours = 168
	}
	stats, err := db.GetAdminPerUserStats(hours, parseExcludeTest(r))
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "users": stats})
}

func handleAdminRegistrationStats(w http.ResponseWriter, r *http.Request) {
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 {
		hours = 720
	}
	granularity := r.URL.Query().Get("granularity")
	if granularity == "" {
		granularity = "day"
	}
	stats, err := db.GetAdminRegistrationStats(granularity, hours, parseExcludeTest(r))
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "registrations": stats})
}

func handleAdminUserDetailStats(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_id", "糟糕,编号无效或已失效,请刷新后重试~")})
		return
	}
	stats, err := db.GetUserDetailStats(id)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "user": stats})
}

func handleAdminErrorStats(w http.ResponseWriter, r *http.Request) {
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 {
		hours = 168
	}
	stats, err := db.GetAdminErrorStats(hours, parseExcludeTest(r))
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "errors": stats})
}

func handleAdminBusyHours(w http.ResponseWriter, r *http.Request) {
	stats, err := db.GetAdminBusyHours(parseExcludeTest(r))
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "hours": stats})
}

func handleAdminMethodStats(w http.ResponseWriter, r *http.Request) {
	stats, err := db.GetAdminMethodStats(parseExcludeTest(r))
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "methods": stats})
}
