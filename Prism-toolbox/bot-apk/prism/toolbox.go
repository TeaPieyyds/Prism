package prism

// ─── Toolbox API ──────────────────────────────────────────────────

// ToolboxInfo 用户工具箱状态
type ToolboxInfo struct {
	Enabled          bool   `json:"enabled"`
	ExpiresAt        string `json:"expires_at"`
	Expired          bool   `json:"expired"`
	DaysRemaining    int    `json:"days_remaining"`
	TrialUsed        bool   `json:"trial_used"`
	PromptPurchased  bool   `json:"prompt_purchased"`
	NamePurchased    bool   `json:"name_purchased"`
	CustomPrompt     string `json:"custom_prompt"`
	CustomName       string `json:"custom_name"`
	DefaultPrompt    string `json:"default_prompt"`
	DefaultName      string `json:"default_name"`
	EffectivePrompt  string `json:"effective_prompt"`
	EffectiveName    string `json:"effective_name"`
}

// ToolboxInfoResponse 获取工具箱信息的响应
type ToolboxInfoResponse struct {
	OK   bool         `json:"ok"`
	Data *ToolboxInfo `json:"data,omitempty"`
}

// GetToolboxInfo 获取当前用户的工具箱状态
// GET /api/toolbox/my-info
func GetToolboxInfo(token string) (*ToolboxInfoResponse, error) {
	result, err := get("/api/toolbox/my-info", nil, token)
	if err != nil {
		return nil, err
	}
	resp := &ToolboxInfoResponse{}
	if ok, _ := result["ok"].(bool); ok {
		resp.OK = true
		if data, ok2 := result["data"].(map[string]any); ok2 {
			info := &ToolboxInfo{}
			if v, ok3 := data["enabled"].(bool); ok3 {
				info.Enabled = v
			}
			if v, ok3 := data["expires_at"].(string); ok3 {
				info.ExpiresAt = v
			}
			if v, ok3 := data["expired"].(bool); ok3 {
				info.Expired = v
			}
			if v, ok3 := data["days_remaining"].(float64); ok3 {
				info.DaysRemaining = int(v)
			}
			if v, ok3 := data["trial_used"].(bool); ok3 {
				info.TrialUsed = v
			}
			if v, ok3 := data["prompt_purchased"].(bool); ok3 {
				info.PromptPurchased = v
			}
			if v, ok3 := data["name_purchased"].(bool); ok3 {
				info.NamePurchased = v
			}
			if v, ok3 := data["custom_prompt"].(string); ok3 {
				info.CustomPrompt = v
			}
			if v, ok3 := data["custom_name"].(string); ok3 {
				info.CustomName = v
			}
			if v, ok3 := data["default_prompt"].(string); ok3 {
				info.DefaultPrompt = v
			}
			if v, ok3 := data["default_name"].(string); ok3 {
				info.DefaultName = v
			}
			if v, ok3 := data["effective_prompt"].(string); ok3 {
				info.EffectivePrompt = v
			}
			if v, ok3 := data["effective_name"].(string); ok3 {
				info.EffectiveName = v
			}
			resp.Data = info
		}
	}
	return resp, nil
}

// UpdateToolboxInfo 修改自定义提示词和自定义名称
// POST /api/toolbox/my-info/update
func UpdateToolboxInfo(token, customPrompt, customName string) (map[string]any, error) {
	body := map[string]any{}
	if customPrompt != "" {
		body["custom_prompt"] = customPrompt
	}
	if customName != "" {
		body["custom_name"] = customName
	}
	return post("/api/toolbox/my-info/update", body, token)
}

// StartTrial 激活 90 天免费试用
// POST /api/toolbox/start-trial
func StartTrial(token string) (map[string]any, error) {
	return post("/api/toolbox/start-trial", nil, token)
}

// PurchasePrompt 板栗购买自定义提示词功能
// POST /api/toolbox/purchase/prompt
func PurchasePrompt(token string) (map[string]any, error) {
	return post("/api/toolbox/purchase/prompt", nil, token)
}

// PurchaseName 板栗购买自定义命名功能
// POST /api/toolbox/purchase/name
func PurchaseName(token string) (map[string]any, error) {
	return post("/api/toolbox/purchase/name", nil, token)
}

// PurchaseExtension 板栗续期工具箱使用权
// POST /api/toolbox/purchase/extension?days=N
func PurchaseExtension(token string, days int) (map[string]any, error) {
	return post("/api/toolbox/purchase/extension?days="+itoa(days), nil, token)
}

// GetNutsBalance 获取当前用户的板栗余额
// GET /api/toolbox/nuts/balance
func GetNutsBalance(token string) (map[string]any, error) {
	return get("/api/auth/nuts", nil, token)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}