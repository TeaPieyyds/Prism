package main

import (
	"encoding/json"
	"net/http"

	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
)

func handleMyInviteCode(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	stats, err := db.GetInviteStats(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{
		"ok":         true,
		"code":       stats.Code,
		"count":      stats.InviteCount,
		"total_nuts": stats.TotalNuts,
		"invitees":   stats.Invitees,
	})
}

func handleRedeemInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Code == "" {
		jsonResp(w, M{"ok": false, "error": "缺少邀请码"})
		return
	}
	if err := db.RedeemInviteCode(body.Code, user.ID); err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true})
}
