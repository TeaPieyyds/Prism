package mpay

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	mathrand "math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Fixed-device guest registration constants, matching the Python script.
const (
	FixedDeviceID       = "amawufyaaxtu3ufq-d"
	FixedHarParams      = "f3308f7fe03fb267158eddeedc4b6eac"
	FixedAppVersionCode = "840293532"
	FixedAppVersionName = "3.8.25.293532"
	FixedSV             = "32"
	FixedDeviceModel    = "23117RK66C"
	FixedPkgChannel     = "netease"
	FixedResolution     = "1080*1920"

	defaultSDKVersion = "5.16.0"

	uniSauthURL = "https://mgbsdk.matrix.netease.com/x19/sdk/uni_sauth"
	mkeyBaseURL = "https://service.mkey.163.com"
)

// GuestFixedResult 保存 GuestWithFixedParams 的返回结果。
type GuestFixedResult struct {
	Token     string // 直接登录时的 session token
	SDKUID    string // 直接登录时的用户 ID
	VerifyURL string // 需要短信验证时的验证 URL
	Ticket    string // 从 verify_url 提取的 ticket
}

// GuestWithFixedParams 使用固定设备凭证调用 by_guest。
func GuestWithFixedParams(ctx context.Context, httpClient *http.Client, deviceID, harParams string) (*GuestFixedResult, error) {
	ts := time.Now().UnixMilli()
	udid, _ := randomHex(16)
	transid := fmt.Sprintf("%s_%d_%d", udid, ts, mathrand.Int63n(1000000000))
	mcountTid := fmt.Sprintf("%s_%d_%d", udid, ts+int64(mathrand.Intn(10000)+1), mathrand.Int63n(1000000000))

	form := buildFixedGuestForm(udid, harParams, transid, mcountTid)

	path := fmt.Sprintf("/mpay/games/%s/devices/%s/users/by_guest", defaultGameID, deviceID)
	body, _, err := fixedPostForm(ctx, httpClient, mkeyBaseURL+path, form)
	if err != nil {
		return nil, fmt.Errorf("mpay: by_guest 请求失败: %w", err)
	}

	var resp struct {
		Code      *int            `json:"code"`
		Reason    string          `json:"reason"`
		VerifyURL string          `json:"verify_url"`
		User      map[string]any  `json:"user"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("mpay: 解析 by_guest 响应失败: %w body=%s", err, string(body))
	}

	// code 1351 = need SMS
	if resp.Code != nil && *resp.Code == 1351 {
		return &GuestFixedResult{
			VerifyURL: resp.VerifyURL,
			Ticket:    extractTicket(resp.VerifyURL),
		}, nil
	}

	// code 0/201 with user = direct login
	if resp.User != nil {
		token := toStringField(resp.User["token"])
		sdkuid := toStringField(resp.User["id"])
		if token != "" && sdkuid != "" {
			return &GuestFixedResult{Token: token, SDKUID: sdkuid}, nil
		}
	}

	// error response
	if resp.Code != nil {
		return nil, &NeedVerifyError{Code: *resp.Code, Reason: resp.Reason, VerifyURL: resp.VerifyURL}
	}
	return nil, fmt.Errorf("mpay: by_guest 未知响应 body=%s", string(body))
}

// FinishSMS 通过 upload_sms/result 完成短信验证。
func FinishSMS(ctx context.Context, httpClient *http.Client, ticket, gv string) (token, userID string, err error) {
	form := url.Values{}
	form.Set("ticket", ticket)
	form.Set("lang", "")
	form.Set("cv", defaultCV)
	form.Set("gv", gv)
	form.Set("app_mode", defaultAppMode)
	form.Set("app_channel", defaultAppChannel)
	form.Set("chg_pwd", "0")

	body, _, err := fixedPostForm(ctx, httpClient, mkeyBaseURL+"/mpay/api/reverify/upload_sms/result", form)
	if err != nil {
		return "", "", fmt.Errorf("mpay: upload_sms 失败: %w", err)
	}

	var resp struct {
		User map[string]any `json:"user"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", fmt.Errorf("mpay: upload_sms 解析失败: %w body=%s", err, string(body))
	}

	if resp.User != nil {
		token = toStringField(resp.User["token"])
		userID = toStringField(resp.User["id"])
	}
	if token == "" || userID == "" {
		return "", "", fmt.Errorf("mpay: upload_sms 响应缺少 token/id body=%s", string(body))
	}
	return token, userID, nil
}

// UniSauthResult 保存 DoUniSauth 的返回结果。
type UniSauthResult struct {
	OAuthToken string
	GameUID    string
}

// DoUniSauth 调用 uni_sauth 端点获取 OAuth2 access_token。
func DoUniSauth(ctx context.Context, httpClient *http.Client, deviceID, sdkuid, sessionID, udid, clientLoginSN, transid, mcountTid, ip, countryCode string) (*UniSauthResult, error) {
	aimInfo := fmt.Sprintf(`{"aim":"%s","country":"%s","tz":"+0800","tzid":"Asia/Shanghai"}`, ip, countryCode)
	sdklog := fmt.Sprintf(
		`{"device_model":"%s","os_name":"android","os_ver":"%s","udid":"%s","app_ver":"%s","imei":"","area_code":"%s","is_emulator":1,"is_root":1,"oaid":""}`,
		FixedDeviceModel, FixedSV, udid, FixedAppVersionCode, countryCode,
	)

	step := fmt.Sprintf("%d", time.Now().UnixMilli()%1000000000)

	payload := map[string]any{
		"gameid":             "x19",
		"login_channel":      "netease",
		"app_channel":        defaultAppChannel,
		"platform":           "ad",
		"sdkuid":             sdkuid,
		"udid":               udid,
		"sessionid":          sessionID,
		"sdk_version":        defaultSDKVersion,
		"is_unisdk_guest":    0,
		"ip":                 ip,
		"aim_info":           aimInfo,
		"source_app_channel": defaultAppChannel,
		"source_platform":    "ad",
		"get_access_token":   "1",
		"deviceid":           deviceID,
		"client_login_sn":    clientLoginSN,
		"step":               step,
		"step2":              "0",
		"hostid":             0,
		"sdklog":             sdklog,
	}

	jsonBody, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", uniSauthURL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("mpay: uni_sauth 请求创建失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TASK-ID", fmt.Sprintf("transid=%s,uni_transaction_id=%s", transid, mcountTid))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mpay: uni_sauth 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mpay: uni_sauth 读取响应失败: %w", err)
	}

	var result struct {
		Code            int    `json:"code"`
		UnisdkLoginJSON string `json:"unisdk_login_json"`
		AID             string `json:"aid"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("mpay: uni_sauth 解析失败: %w body=%s", err, string(body))
	}

	if result.Code != 200 {
		return nil, fmt.Errorf("mpay: uni_sauth 返回错误 code=%d body=%s", result.Code, string(body))
	}

	loginBytes, err := base64.StdEncoding.DecodeString(result.UnisdkLoginJSON)
	if err != nil {
		return nil, fmt.Errorf("mpay: uni_sauth base64 解码失败: %w", err)
	}

	var loginData struct {
		OAuth2 struct {
			AccessToken string `json:"access_token"`
		} `json:"oauth2"`
	}
	if err := json.Unmarshal(loginBytes, &loginData); err != nil {
		return nil, fmt.Errorf("mpay: uni_sauth login_json 解析失败: %w", err)
	}

	return &UniSauthResult{
		OAuthToken: loginData.OAuth2.AccessToken,
		GameUID:    result.AID,
	}, nil
}

// SetNickname 通过游戏 API 设置昵称。返回设置的昵称（格式 XR_XXXXx）。
func SetNickname(ctx context.Context, httpClient *http.Client, gameToken string) (nickname string, err error) {
	digits := fmt.Sprintf("%04d", time.Now().UnixNano()%10000)
	letter := string(rune('a' + time.Now().UnixNano()%26))
	nickname = "pr_" + digits + letter

	hosts := []string{
		"g79apigatewayobt.minecraft.cn",
		"g79mclobt.minecraft.cn",
	}

	for _, host := range hosts {
		baseURL := fmt.Sprintf("https://%s", host)
		gameUID := ""

		for _, ep := range []string{"/user-account-id", "/user-sdkinfo"} {
			body, eErr := fixedPostJSON(ctx, httpClient, baseURL+ep, map[string]any{}, "", gameToken)
			if eErr != nil {
				continue
			}
			var r struct {
				Code   int            `json:"code"`
				Entity map[string]any `json:"entity"`
			}
			if json.Unmarshal(body, &r) == nil && r.Code == 0 {
				if uid, ok := r.Entity["USERINFO_UID"]; ok {
					gameUID = fmt.Sprintf("%v", uid)
				} else if eid, ok := r.Entity["entity_id"]; ok {
					gameUID = fmt.Sprintf("%v", eid)
				}
				if gameUID != "" {
					break
				}
			}
		}

		body, pErr := fixedPostJSON(ctx, httpClient, baseURL+"/nickname-setting",
			map[string]string{"name": nickname}, gameUID, gameToken)
		if pErr != nil {
			continue
		}
		var r struct {
			Code int `json:"code"`
		}
		if json.Unmarshal(body, &r) == nil && r.Code == 0 {
			return nickname, nil
		}
	}

	return nickname, nil // 即使失败也返回昵称，调用方可以继续
}

// SetNicknameName 用指定昵称对账号改名(必须改名后才能正常使用)。
// 返回错误并附带服务端 message，便于调用方识别 请求过快/名字已被占用/名字不合规 等。
func SetNicknameName(ctx context.Context, httpClient *http.Client, uid, token, name string) error {
	hosts := []string{
		"g79apigatewayobt.minecraft.cn",
		"g79mclobt.minecraft.cn",
	}
	var lastErr error
	for _, host := range hosts {
		baseURL := fmt.Sprintf("https://%s", host)
		gameUID := uid
		// 获取游戏 uid
		for _, ep := range []string{"/user-account-id", "/user-sdkinfo"} {
			body, eErr := fixedPostJSON(ctx, httpClient, baseURL+ep, map[string]any{}, gameUID, token)
			if eErr != nil {
				continue
			}
			var r struct {
				Code   int            `json:"code"`
				Entity map[string]any `json:"entity"`
			}
			if json.Unmarshal(body, &r) == nil && r.Code == 0 {
				if u, ok := r.Entity["USERINFO_UID"]; ok {
					gameUID = fmt.Sprintf("%v", u)
				} else if eid, ok := r.Entity["entity_id"]; ok {
					gameUID = fmt.Sprintf("%v", eid)
				}
				break
			}
		}
		// 设置昵称
		body, pErr := fixedPostJSON(ctx, httpClient, baseURL+"/nickname-setting", map[string]string{"name": name}, gameUID, token)
		if pErr != nil {
			lastErr = pErr
			continue
		}
		var r struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &r); err == nil && r.Code == 0 {
			return nil
		}
		msg := r.Message
		if msg == "" {
			msg = "请换一个名字重试"
		}
		lastErr = fmt.Errorf("%s", msg)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("改名失败：所有主机均失败")
	}
	return lastErr
}

// GenerateFixedSauthJSON 生成 Python 脚本格式的 sauth_json。
func GenerateFixedSauthJSON(deviceID, sdkuid, sessionID, udid, clientLoginSN, ip, countryCode, nickname string) string {
	aimInfo := fmt.Sprintf(`{"aim":"%s","country":"%s","tz":"+0800","tzid":""}`, ip, countryCode)

	inner := map[string]string{
		"gameid":          "x19",
		"login_channel":   "netease",
		"app_channel":     defaultAppChannel,
		"platform":        "pc",
		"sdkuid":          sdkuid,
		"sessionid":       sessionID,
		"sdk_version":     defaultSDKVersion,
		"udid":            deviceID,
		"deviceid":        deviceID,
		"aim_info":        aimInfo,
		"client_login_sn": clientLoginSN,
		"gas_token":       "",
		"source_platform": "netease",
		"ip":              ip,
		"nickname":        nickname,
	}

	innerJSON, _ := json.Marshal(inner)
	outer := map[string]string{"sauth_json": string(innerJSON)}
	outerJSON, _ := json.Marshal(outer)
	return string(outerJSON)
}

// AuthRealNameFixed 调用 update_by_token + verify 完成实名认证。
func AuthRealNameFixed(ctx context.Context, httpClient *http.Client, deviceID, userID, token, realName, idNum string) error {
	form := url.Values{}
	form.Set("device_id", deviceID)
	form.Set("user_id", userID)
	form.Set("token", token)
	form.Set("realname", realName)
	form.Set("id_region", "86")
	form.Set("id_num", idNum)
	form.Set("game_id", defaultGameID)
	form.Set("gv", FixedAppVersionCode)
	form.Set("gvn", FixedAppVersionName)
	form.Set("cv", defaultCV)
	form.Set("sv", FixedSV)
	form.Set("app_type", defaultAppType)
	form.Set("app_mode", defaultAppMode)
	form.Set("app_channel", defaultAppChannel)
	form.Set("mcount_app_key", defaultMCountAppKey)

	// 1. update_by_token —— 这一步真正把实名绑定到账号，必须检查响应 code
	updateBody, _, err := fixedPostForm(ctx, httpClient, mkeyBaseURL+"/mpay/api/users/realname/update_by_token", form)
	if err != nil {
		return fmt.Errorf("mpay: realname update_by_token 失败: %w", err)
	}
	if code, reason := mpayResponseCode(updateBody); code != 0 {
		return &NeedVerifyError{Code: code, Reason: reason}
	}

	// 2. verify
	body, _, err := fixedPostForm(ctx, httpClient, mkeyBaseURL+"/mpay/api/users/realname/verify", form)
	if err != nil {
		return fmt.Errorf("mpay: realname verify 失败: %w", err)
	}

	var resp struct {
		Code         *int   `json:"code"`
		Reason       string `json:"reason"`
		RealnameType string `json:"realname_type"`
		NeedAas      *bool  `json:"need_aas"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("mpay: realname 解析失败: %w body=%s", err, string(body))
	}
	if resp.Code != nil && *resp.Code != 0 {
		return &NeedVerifyError{Code: *resp.Code, Reason: resp.Reason}
	}

	if strings.Contains(resp.RealnameType, "成年") || (resp.NeedAas != nil && !*resp.NeedAas) {
		return nil
	}
	return fmt.Errorf("mpay: 实名认证未通过 realname_type=%q body=%s", resp.RealnameType, string(body))
}

// ------- 内部辅助函数 -------

func fixedPostForm(ctx context.Context, httpClient *http.Client, urlStr string, form url.Values) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", urlStr, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", fmt.Sprintf("com.netease.x19/%s NeteaseMobileGame/%s (%s;%s)",
		FixedAppVersionCode, defaultCV, FixedDeviceModel, FixedSV))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

func fixedPostJSON(ctx context.Context, httpClient *http.Client, urlStr string, data any, userID, userToken string) ([]byte, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", urlStr, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("accept-encoding", "gzip")
	req.Header.Set("user-agent", "libhttpclient/1.0.0.0")
	if userID != "" {
		req.Header.Set("user-id", userID)
	}
	if userToken != "" {
		req.Header.Set("user-token", userToken)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 手动设置了 Accept-Encoding: gzip 时 Go 不会自动解压，需自行处理
	reader := resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, gzErr := gzip.NewReader(resp.Body)
		if gzErr != nil {
			return nil, gzErr
		}
		defer gz.Close()
		reader = gz
	}
	return io.ReadAll(reader)
}

func buildFixedGuestForm(udid, harParams, transid, mcountTid string) url.Values {
	form := url.Values{}
	form.Set("game_id", defaultGameID)
	form.Set("gv", FixedAppVersionCode)
	form.Set("gvn", FixedAppVersionName)
	form.Set("cv", defaultCV)
	form.Set("sv", FixedSV)
	form.Set("app_type", defaultAppType)
	form.Set("app_mode", defaultAppMode)
	form.Set("jf_game_id", "x19")
	form.Set("pkg_channel", FixedPkgChannel)
	form.Set("app_channel", defaultAppChannel)
	form.Set("transid", transid)
	form.Set("mcount_app_key", defaultMCountAppKey)
	form.Set("mcount_transaction_id", mcountTid)
	form.Set("_cloud_extra_base64", defaultCloudExtraBase64)
	form.Set("sc", "1")
	form.Set("opt_fields", "nickname,avatar,realname_status,mobile_bind_status,exit_popup_info,mask_related_mobile,related_login_status,detect_is_new_user")
	form.Set("params", harParams)
	return form
}

// mpayResponseCode 解析响应的 code/reason。code 缺失时按 0(成功) 处理。
func mpayResponseCode(body []byte) (int, string) {
	var resp struct {
		Code   *int   `json:"code"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Code == nil {
		return 0, ""
	}
	return *resp.Code, resp.Reason
}

func extractTicket(verifyURL string) string {
	if verifyURL == "" {
		return ""
	}
	u, err := url.Parse(verifyURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("ticket")
}

func toStringField(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%.0f", val)
	default:
		return fmt.Sprintf("%v", v)
	}
}
