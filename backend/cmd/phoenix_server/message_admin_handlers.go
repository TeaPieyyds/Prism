package main

import (
	"encoding/json"
	"net/http"
)

// handleAdminGetMessages 返回当前消息配置(管理界面查看/编辑用)。
func handleAdminGetMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	jsonW(w, r, M{"ok": true, "path": messagesPath, "messages": msg.Snapshot()})
}

// handleAdminSetMessages 写入新配置并立即重载。
func handleAdminSetMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	var body struct {
		Messages map[string]string `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("invalid_json", "抱歉,提交的数据格式有误,请检查后重试~")})
		return
	}
	if err := msg.SetMessages(body.Messages); err != nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("system_error", "哎呀,系统开小差了,请稍后重试~") + " (" + err.Error() + ")"})
		return
	}
	jsonW(w, r, M{"ok": true, "message": "恭喜,消息配置已保存并立即生效~"})
}
