package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
)

// ── 用户端 ──

func handleActiveSurvey(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	surveys, err := db.GetActiveSurveys(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if surveys == nil { surveys = []db.Survey{} }
	surveysWithQuestions := make([]M, 0, len(surveys))
	for _, s := range surveys {
		qs, _ := db.GetSurveyQuestions(s.ID)
		if qs == nil { qs = []db.SurveyQuestion{} }
		surveysWithQuestions = append(surveysWithQuestions, M{"survey": s, "questions": qs})
	}
	jsonResp(w, M{"ok": true, "surveys": surveysWithQuestions})
}

func handleSubmitSurvey(w http.ResponseWriter, r *http.Request) {
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
		SurveyID int64             `json:"survey_id"`
		Answers  []db.SurveyAnswer `json:"answers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResp(w, M{"ok": false, "error": "请求格式错误"})
		return
	}
	if body.SurveyID == 0 || len(body.Answers) == 0 {
		jsonResp(w, M{"ok": false, "error": "缺少问卷ID或答案"})
		return
	}
	answered, _ := db.HasUserAnsweredSurvey(body.SurveyID, user.ID)
	if answered {
		jsonResp(w, M{"ok": false, "error": "你已经回答过这个问卷"})
		return
	}
	s, err := db.GetSurvey(body.SurveyID)
	if err != nil || s == nil || !s.IsActive {
		jsonResp(w, M{"ok": false, "error": "问卷不存在或已关闭"})
		return
	}
	reward, err := db.SubmitSurveyAnswers(body.SurveyID, user.ID, body.Answers)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "message": "提交成功", "reward_nuts": reward})
}

// ── 管理员端 ──

func handleAdminListSurveys(w http.ResponseWriter, r *http.Request) {
	surveys, err := db.ListSurveys()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if surveys == nil {
		surveys = []db.Survey{}
	}
	jsonResp(w, M{"ok": true, "surveys": surveys})
}

func handleAdminCreateSurvey(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var body struct {
		Title      string              `json:"title"`
		RewardNuts int                 `json:"reward_nuts"`
		Questions  []db.SurveyQuestion `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResp(w, M{"ok": false, "error": "请求格式错误"})
		return
	}
	if body.Title == "" {
		jsonResp(w, M{"ok": false, "error": "标题不能为空"})
		return
	}
	if len(body.Questions) == 0 {
		jsonResp(w, M{"ok": false, "error": "至少需要一个问题"})
		return
	}
	survey, err := db.CreateSurvey(body.Title, body.RewardNuts, body.Questions)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	adminLog(r, "admin_create_survey", "创建问卷: "+body.Title)
	jsonResp(w, M{"ok": true, "survey": survey})
}

func handleAdminUpdateSurvey(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var body struct {
		ID         int64  `json:"id"`
		Title      string `json:"title"`
		RewardNuts int    `json:"reward_nuts"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.ID == 0 || body.Title == "" {
		jsonResp(w, M{"ok": false, "error": msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~")})
		return
	}
	if err := db.UpdateSurvey(body.ID, body.Title, body.RewardNuts); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	adminLog(r, "admin_update_survey", "更新问卷 #"+strconv.FormatInt(body.ID, 10))
	jsonResp(w, M{"ok": true})
}

func handleAdminUpdateQuestions(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var body struct {
		ID        int64               `json:"id"`
		Questions []db.SurveyQuestion `json:"questions"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.ID == 0 || len(body.Questions) == 0 {
		jsonResp(w, M{"ok": false, "error": msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~")})
		return
	}
	if err := db.UpdateSurveyQuestions(body.ID, body.Questions); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	adminLog(r, "admin_update_survey_questions", "更新问卷问题 #"+strconv.FormatInt(body.ID, 10))
	jsonResp(w, M{"ok": true})
}

func handleAdminToggleSurvey(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var body struct{ ID int64 `json:"id"` }
	json.NewDecoder(r.Body).Decode(&body)
	if body.ID == 0 {
		jsonResp(w, M{"ok": false, "error": msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~")})
		return
	}
	if err := db.ToggleSurveyActive(body.ID); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	adminLog(r, "admin_toggle_survey", "切换问卷状态 #"+strconv.FormatInt(body.ID, 10))
	jsonResp(w, M{"ok": true})
}

func handleAdminDeleteSurvey(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var body struct{ ID int64 `json:"id"` }
	json.NewDecoder(r.Body).Decode(&body)
	if body.ID == 0 {
		jsonResp(w, M{"ok": false, "error": msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~")})
		return
	}
	if err := db.DeleteSurvey(body.ID); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	adminLog(r, "admin_delete_survey", "删除问卷 #"+strconv.FormatInt(body.ID, 10))
	jsonResp(w, M{"ok": true})
}

func handleAdminSurveyStats(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id == 0 {
		jsonResp(w, M{"ok": false, "error": "缺少问卷ID"})
		return
	}
	stats, err := db.GetSurveyStats(id)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "stats": stats})
}

func handleAdminSurveyUserAnswers(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	uidStr := r.URL.Query().Get("user_id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	uid, _ := strconv.ParseInt(uidStr, 10, 64)
	if id == 0 || uid == 0 {
		jsonResp(w, M{"ok": false, "error": msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~")})
		return
	}
	answers, err := db.GetUserSurveyAnswers(id, uid)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	questions, _ := db.GetSurveyQuestions(id)
	jsonResp(w, M{"ok": true, "answers": answers, "questions": questions})
}

func handleAdminGetSurveyQuestions(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id == 0 {
		jsonResp(w, M{"ok": false, "error": "缺少问卷ID"})
		return
	}
	questions, err := db.GetSurveyQuestions(id)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if questions == nil { questions = []db.SurveyQuestion{} }
	jsonResp(w, M{"ok": true, "questions": questions})
}

func handleAdminClearSurveyAnswers(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var body struct {
		ID     int64  `json:"id"`
		UserID *int64 `json:"user_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.ID == 0 {
		jsonResp(w, M{"ok": false, "error": msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~")})
		return
	}
	n, err := db.ClearSurveyAnswers(body.ID, body.UserID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if body.UserID != nil {
		adminLog(r, "admin_clear_survey_user", fmt.Sprintf("清除问卷 #%d 用户 #%d 的答案", body.ID, *body.UserID))
	} else {
		adminLog(r, "admin_clear_survey_all", fmt.Sprintf("清除问卷 #%d 全部答案 (%d 条)", body.ID, n))
	}
	jsonResp(w, M{"ok": true, "cleared": n})
}

func adminLog(r *http.Request, action, detail string) {
	u := auth.GetUser(r.Context())
	if u != nil {
		_ = db.AddAuditLog(&u.ID, nil, action, "web", detail, requestIP(r))
	}
}
