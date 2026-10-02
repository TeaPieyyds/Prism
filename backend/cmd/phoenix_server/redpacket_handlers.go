package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
)

// claimRedPacket is called from handleRedeemCode after auth, rate-limit and
// captcha checks have already passed for the "RB-" code.
func claimRedPacket(w http.ResponseWriter, r *http.Request, user *db.User, code string) {
	// 订阅期内禁用抢红包
	if subActive, _, _ := db.IsSubscriptionActive(user.ID); subActive {
		jsonResp(w, M{"ok": false, "error": msg.Get("subscription_blocked", "订阅期内暂不可抢红包~")})
		return
	}
	amount, err := db.ClaimRedPacket(code, user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	_ = db.AddAuditLog(&user.ID, nil, "redeem_redpacket", "red_packet", fmt.Sprintf("抢红包 +%d 板栗", amount), requestIP(r))
	jsonResp(w, M{"ok": true, "message": fmt.Sprintf("抢到 %d 颗板栗", amount), "amount": amount})
}

// POST /api/auth/nuts/redpacket — user creates a red packet from own balance
func handleCreateRedPacket(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	if !redeemLimiter.Allow(requestIP(r)) {
		jsonResp(w, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
		return
	}
	if !checkCaptcha(r) {
		jsonResp(w, M{"ok": false, "error": "哎呀,验证码错误或已过期,请重新获取后再试~"})
		return
	}
	// 订阅期内禁用发红包
	if subActive, _, _ := db.IsSubscriptionActive(user.ID); subActive {
		jsonResp(w, M{"ok": false, "error": msg.Get("subscription_blocked", "订阅期内暂不可发红包~")})
		return
	}
	var req struct {
		Amount int `json:"amount"`
		Count  int `json:"count"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	code, pool, err := db.CreateRedPacketFromNuts(user.ID, req.Amount, req.Count)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	_ = db.AddAuditLog(&user.ID, nil, "create_redpacket", "red_packet",
		fmt.Sprintf("创建红包 -%d 积分 池=%d %d份 码:%s", req.Amount, pool, req.Count, code), requestIP(r))
	jsonResp(w, M{"ok": true, "code": code, "pool": pool, "count": req.Count, "message": fmt.Sprintf("红包已创建，池 %d 板栗，共 %d 份", pool, req.Count)})
}

// POST /api/admin/redpackets — admin creates a red packet (no cost)
func handleAdminCreateRedPacket(w http.ResponseWriter, r *http.Request) {
	adminUser := auth.GetUser(r.Context())
	if adminUser == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	var req struct {
		Total int `json:"total"`
		Count int `json:"count"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	code, err := db.CreateRedPacket(req.Total, req.Count, adminUser.ID, "admin")
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	_ = db.AddAuditLog(&adminUser.ID, nil, "admin_create_redpacket", "red_packet",
		fmt.Sprintf("管理员创建红包 总=%d %d份 码:%s", req.Total, req.Count, code), requestIP(r))
	jsonResp(w, M{"ok": true, "code": code, "message": fmt.Sprintf("红包已创建，池 %d 板栗，共 %d 份", req.Total, req.Count)})
}

// GET /api/auth/redpackets — list red packets created by the current user
func handleListRedPackets(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	packets, err := db.ListRedPackets(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "packets": packets})
}
