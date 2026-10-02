package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
)

// GET  /api/auth/tokens — list my secondary tokens + stats
// POST /api/auth/tokens — create a secondary token (returns raw value once)
func handleAPITokens(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	if r.Method == http.MethodPost {
		handleAPITokenCreate(w, r, user.ID)
		return
	}
	tokens, total, err := db.GetTokenStats(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "tokens": tokens, "total_calls": total, "primary_token": user.Token})
}

func handleAPITokenCreate(w http.ResponseWriter, r *http.Request, userID int64) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	var req struct {
		Name     string `json:"name"`
		MaxCalls int    `json:"max_calls"` // -1 = unlimited
		MaxNuts  int    `json:"max_nuts"`  // -1 = unlimited
	}
	json.NewDecoder(r.Body).Decode(&req)
	token, err := db.CreateAPIToken(userID, req.Name, req.MaxCalls, req.MaxNuts)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	_ = db.AddAuditLog(&user.ID, nil, "create_token", "api_token", "创建令牌", requestIP(r))
	jsonResp(w, M{"ok": true, "token": token})
}

// POST /api/auth/tokens/{id}/reset — regenerate a secondary token
func handleAPITokenReset(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	id, err := pathTokenID(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "无效的令牌ID"})
		return
	}
	token, err := db.ResetAPIToken(id, user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	_ = db.AddAuditLog(&user.ID, nil, "reset_token", "api_token", "重置子令牌 #"+strconv.FormatInt(id, 10), requestIP(r))
	jsonResp(w, M{"ok": true, "token": token})
}

// POST /api/auth/tokens/{id}/update — update name/limits/disabled
func handleAPITokenUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	id, err := pathTokenID(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "无效的令牌ID"})
		return
	}
	var req struct {
		Name     string `json:"name"`
		MaxCalls *int   `json:"max_calls"`
		MaxNuts  *int   `json:"max_nuts"`
		Disabled *bool  `json:"disabled"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	tok, err := db.GetAPITokenByID(id, user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	maxCalls, maxNuts := tok.MaxCalls, tok.MaxNuts
	if req.MaxCalls != nil {
		maxCalls = *req.MaxCalls
	}
	if req.MaxNuts != nil {
		maxNuts = *req.MaxNuts
	}
	disabled := tok.Disabled
	if req.Disabled != nil {
		disabled = *req.Disabled
	}
	if err := db.UpdateAPIToken(id, user.ID, req.Name, maxCalls, maxNuts, disabled); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	_ = db.AddAuditLog(&user.ID, nil, "update_token", "api_token", "更新令牌 #"+strconv.FormatInt(id, 10), requestIP(r))
	jsonResp(w, M{"ok": true})
}

// POST /api/auth/tokens/{id}/switch — validate ownership and echo the token's
// current account binding (the web "current token" is kept client-side).
func handleAPITokenSwitch(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	id, err := pathTokenID(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "无效的令牌ID"})
		return
	}
	tok, err := db.GetAPITokenByID(id, user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	accName := ""
	if tok.ActiveAccountID != nil {
		if acc, e := db.GetAccountByID(*tok.ActiveAccountID); e == nil && acc != nil {
			accName = acc.DisplayName
		}
	}
	jsonResp(w, M{"ok": true, "id": tok.ID, "name": tok.Name, "active_account_id": tok.ActiveAccountID, "active_account_name": accName})
}

// DELETE /api/auth/tokens/{id} — delete a secondary token
func handleAPITokenDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	id, err := pathTokenID(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "无效的令牌ID"})
		return
	}
	if err := db.DeleteAPIToken(id, user.ID); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	_ = db.AddAuditLog(&user.ID, nil, "delete_token", "api_token", "删除令牌 #"+strconv.FormatInt(id, 10), requestIP(r))
	jsonResp(w, M{"ok": true})
}

// POST /api/auth/nuts/to-code — convert own nuts balance into an activation code
func handleNutsToCode(w http.ResponseWriter, r *http.Request) {
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
	// 订阅期内禁用生成兑换码
	if subActive, _, _ := db.IsSubscriptionActive(user.ID); subActive {
		jsonResp(w, M{"ok": false, "error": msg.Get("subscription_blocked", "订阅期内暂不可生成兑换码~")})
		return
	}
	var req struct {
		Amount int `json:"amount"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	code, codeValue, err := db.ConvertNutsToCode(user.ID, req.Amount)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	_ = db.AddAuditLog(&user.ID, nil, "nuts_to_code", "activation_code", "积分转兑换码 -"+strconv.Itoa(req.Amount)+" 得 +"+strconv.Itoa(codeValue)+" 码: "+code, requestIP(r))
	jsonResp(w, M{"ok": true, "code": code, "code_value": codeValue, "message": "兑换码已生成"})
}

// POST /api/auth/nuts/to-question-code — convert own nuts balance into a
// single-use question-gated activation code (redeemers must answer correctly).
func handleNutsToQuestionCode(w http.ResponseWriter, r *http.Request) {
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
	// 订阅期内禁用生成答题兑换码
	if subActive, _, _ := db.IsSubscriptionActive(user.ID); subActive {
		jsonResp(w, M{"ok": false, "error": msg.Get("subscription_blocked", "订阅期内暂不可生成答题兑换码~")})
		return
	}
	var req struct {
		Amount   int    `json:"amount"`
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	code, codeValue, err := db.ConvertNutsToQuestionCode(user.ID, req.Amount, req.Question, req.Answer)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	_ = db.AddAuditLog(&user.ID, nil, "nuts_to_question_code", "activation_code",
		"积分转答题码 -"+strconv.Itoa(req.Amount)+" 得 +"+strconv.Itoa(codeValue)+" 码: "+code, requestIP(r))
	jsonResp(w, M{"ok": true, "code": code, "code_value": codeValue, "message": "答题兑换码已生成"})
}

// GET /api/auth/codes — list redemption codes created by the current user
func handleMyCodes(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	codes, err := db.GetUserCreatedCodes(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "codes": codes})
}

func pathTokenID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
