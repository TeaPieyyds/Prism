package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
)

// POST /api/auth/redeem — user redeems an activation code, a question-gated
// code (two-step: first fetch the question, then submit the answer), or claims
// a red packet (codes prefixed "RB-").
func handleRedeemCode(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	if !redeemLimiter.Allow(requestIP(r)) {
		jsonResp(w, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
		return
	}
	// 验证码检查
	if !checkCaptcha(r) {
		jsonResp(w, M{"ok": false, "error": "哎呀,验证码错误或已过期,请重新获取后再试~"})
		return
	}
	// 订阅期内禁用兑换激活码/抢红包
	if subActive, _, _ := db.IsSubscriptionActive(user.ID); subActive {
		jsonResp(w, M{"ok": false, "error": msg.Get("subscription_blocked", "订阅期内暂不可兑换激活码~")})
		return
	}
	var req struct {
		Code   string `json:"code"`
		Answer string `json:"answer"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Code == "" {
		jsonResp(w, M{"ok": false, "error": "抱歉,请输入激活码再兑换~"})
		return
	}
	// Red packet claim — same endpoint, dispatched by the RB- prefix
	if strings.HasPrefix(strings.ToUpper(req.Code), "RB-") {
		claimRedPacket(w, r, user, req.Code)
		return
	}
	amount, question, answerHash, found := db.GetActivationCodeInfo(req.Code)
	if !found {
		jsonResp(w, M{"ok": false, "error": "糟糕,激活码无效或已被使用,请核对后重新输入~"})
		return
	}
	if question != "" {
		if req.Answer == "" {
			jsonResp(w, M{"ok": true, "need_answer": true, "question": question, "code": req.Code})
			return
		}
		if !db.VerifyAnswer(answerHash, req.Answer) {
			jsonResp(w, M{"ok": false, "error": "答案错误"})
			return
		}
	}
	amount, err := db.RedeemActivationCode(req.Code, user.ID)
	if err != nil {
		log.Printf("[REDEEM] 用户 %s 兑换激活码失败: %v", user.Username, err)
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	log.Printf("[REDEEM] 用户 %s 兑换激活码 +%d 板栗", user.Username, amount)
	_ = db.AddAuditLog(&user.ID, nil, "redeem_code", "activation_code", fmt.Sprintf("兑换 +%d 板栗", amount), requestIP(r))
	jsonResp(w, M{"ok": true, "message": fmt.Sprintf("兑换成功，获得 %d 颗板栗", amount), "amount": amount})
}

// POST /api/admin/activation-codes — admin generates activation codes
func handleAdminGenCodes(w http.ResponseWriter, r *http.Request) {
	adminUser := auth.GetUser(r.Context())
	var req struct {
		Amount int `json:"amount"`
		Count  int `json:"count"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Amount <= 0 {
		req.Amount = 10
	}
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.Count > 100 {
		jsonResp(w, M{"ok": false, "error": "单次最多生成 100 个激活码"})
		return
	}
	codes, err := db.GenerateActivationCodes(req.Amount, req.Count, adminUser.ID)
	if err != nil {
		log.Printf("[ADMIN] %s 生成激活码失败: %v", adminUser.Username, err)
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	log.Printf("[ADMIN] %s 生成 %d 个激活码 (%d 板栗)", adminUser.Username, req.Count, req.Amount)
	_ = db.AddAuditLog(&adminUser.ID, nil, "admin_gen_codes", "activation_code", fmt.Sprintf("生成 %d 个激活码 x%d板栗", req.Count, req.Amount), requestIP(r))
	jsonResp(w, M{"ok": true, "codes": codes})
}

// GET /api/admin/activation-codes — admin lists all unused codes
func handleAdminListCodes(w http.ResponseWriter, r *http.Request) {
	codes, err := db.ListActivationCodes()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "codes": codes})
}
