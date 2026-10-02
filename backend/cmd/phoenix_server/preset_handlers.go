package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
)

func handleAdminTogglePreset(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" { jsonResp(w, M{"ok":false,"error":msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")}); return }
	var req struct{ ID int64 `json:"id"`; Enabled bool `json:"enabled"` }
	json.NewDecoder(r.Body).Decode(&req)
	if req.ID == 0 { jsonResp(w, M{"ok":false,"error":msg.Get("invalid_id", "糟糕,编号无效或已失效,请刷新后重试~")}); return }
	if err := db.SetRealnamePresetEnabled(req.ID, req.Enabled); err != nil { jsonResp(w, M{"ok":false,"error":sysErr(err)}); return }
	adminUser := auth.GetUser(r.Context())
	action := "禁用"
	if req.Enabled { action = "启用" }
	log.Printf("[ADMIN] %s %s实名预设 #%d", adminUser.Username, action, req.ID)
	_ = db.AddAuditLog(&adminUser.ID, nil, "admin_toggle_preset", "realname", fmt.Sprintf("%s 预设 #%d", action, req.ID), requestIP(r))
	jsonResp(w, M{"ok":true})
}

func handleAdminListPresets(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" { jsonResp(w, M{"ok":false,"error":msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")}); return }
	presets, err := db.GetAllRealnamePresets()
	if err != nil { jsonResp(w, M{"ok":false,"error":sysErr(err)}); return }
	jsonResp(w, M{"ok":true,"presets":presets})
}
