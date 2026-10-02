package g79client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const drpfEndpoint = "https://drpf-g79.proxima.nie.netease.com"

// DrpfPingOptions 控制 drpf 埋点的可变字段。
type DrpfPingOptions struct {
	CommonInt2  int
	CommonInt3  int
	Type        string
	OperateType string
	SubType     string
	MainType    string
}

// DrpfPing 向 drpf 服务器发送设备指纹埋点，模拟真实游戏客户端行为。
// 每次调用会携带设备信息、会话标识和指定的场景字段。
func (c *Client) DrpfPing(opts DrpfPingOptions) error {
	if c.UserID == "" {
		return fmt.Errorf("drpf ping: missing user id")
	}

	roleID := c.UserID
	if c.UserDetail != nil && c.UserDetail.Name != "" {
		roleID = c.UserDetail.Name
	}

	payload := map[string]any{
		"is_emulator":       "None",
		"jf_gameid":         "None",
		"ip":                "None",
		"common_int2":       opts.CommonInt2,
		"game_session_id":   fmt.Sprintf("%s%d", c.UserID, time.Now().UnixNano()),
		"common_int3":       opts.CommonInt3,
		"app_ver":           c.EngineVersion,
		"country_code":      86,
		"uid":               c.UserID,
		"device_level":      2,
		"transid":           "None",
		"operate_type":      opts.OperateType,
		"isp_name":          "bgp",
		"is_oversea_ip":     false,
		"sead":              c.Seed,
		"os_ver":            "11",
		"network":           "CHANNEL_UNKNOW",
		"login_channel":     "netease",
		"oaid":              "None",
		"app_channel":       "netease",
		"role_id":           c.UserID,
		"source":            "netease_p2",
		"patch_ver":         c.G79LatestVersion,
		"msg":               "",
		"engine_ver":        c.EngineVersion,
		"extra_info":        "{}",
		"type":              opts.Type,
		"location":          "None",
		"common_int1":       0,
		"testFlag":          false,
		"os_name":           OSName,
		"account_id":        "",
		"is_root":           "None",
		"role_name":         roleID,
		"imei":              "None",
		"WebServerUrl":      c.ReleaseJSON.WebServerUrl,
		"common_str2":       "",
		"sub_type":          opts.SubType,
		"common_str1":       "",
		"main_type":         opts.MainType,
		"device_model":      "UNKNOWN",
		"patchVersion":      "None",
		"server":            c.ReleaseJSON.CoreServerURL,
		"project":           "g79",
		"udid":              c.UserID,
		"caid":              "None",
		"mac_addr":          "02:00:00:00:00:00",
		"account_user_name": "None",
		"error_code":        "",
		"common_str3":       "",
		"launch_type":       "cocos",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("drpf ping: %w", err)
	}

	req, err := http.NewRequest("POST", drpfEndpoint, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("drpf ping: %w", err)
	}

	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("drpf ping: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("drpf ping: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("drpf ping: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if strings.TrimSpace(string(respBody)) != "ok" {
		return fmt.Errorf("drpf ping: unexpected response %q", strings.TrimSpace(string(respBody)))
	}

	return nil
}
