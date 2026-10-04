package g79client

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// RefreshTokenResponse /authentication/update 的响应体
type RefreshTokenResponse struct {
	Response
	Entity struct {
		EntityID string `json:"entity_id"`
		Token    string `json:"token"`
		Sead     string `json:"sead"`
	} `json:"entity"`
}

// RefreshToken 调用 /authentication/update 刷新 token，避免重新走完整认证。
// 返回错误说明刷新失败，调用方可用最短过期时间重试。
func (c *Client) RefreshToken() error {
	if c.UserID == "" || c.UserToken == "" {
		return fmt.Errorf("refresh token: 缺少用户凭证")
	}

	api := "/authentication/update"
	encryptedData, err := G79HttpEncrypt([]byte(""))
	if err != nil {
		return fmt.Errorf("refresh token: 加密失败: %w", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.CoreServerURL+api, strings.NewReader(hex.EncodeToString(encryptedData)))
	if err != nil {
		return fmt.Errorf("refresh token: 创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("user-id", c.UserID)
	req.Header.Set("user-token", CalculateDynamicToken(api, "", c.UserToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("refresh token: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("refresh token: 读取响应失败: %w", err)
	}

	encryptedResp, err := hex.DecodeString(string(respBody))
	if err != nil {
		return fmt.Errorf("refresh token: 响应解码失败: %w", err)
	}
	decryptedResp, err := G79HttpDecrypt(encryptedResp)
	if err != nil {
		return fmt.Errorf("refresh token: 响应解密失败: %w", err)
	}

	validJSON := GetValidJSON(decryptedResp)
	var refreshResp RefreshTokenResponse
	if err := json.Unmarshal(validJSON, &refreshResp); err != nil {
		return fmt.Errorf("refresh token: 解析响应失败: %v, 响应: %s", err, string(validJSON))
	}
	if refreshResp.Code != 0 {
		return fmt.Errorf("refresh token 失败 (code: %d): %s", refreshResp.Code, refreshResp.Message)
	}
	if refreshResp.Entity.Token == "" {
		return fmt.Errorf("refresh token: 响应缺少 token")
	}

	c.UserID = refreshResp.Entity.EntityID
	c.UserToken = refreshResp.Entity.Token
	if refreshResp.Entity.Sead != "" {
		c.Seed = refreshResp.Entity.Sead
	}
	return nil
}

// Reconnect 调用 /authentication/reconnect 用 sead 恢复会话。
func (c *Client) Reconnect() error {
	if c.UserID == "" || c.Seed == "" {
		return fmt.Errorf("reconnect: 缺少用户凭证或 sead")
	}

	api := "/authentication/reconnect"
	req, err := http.NewRequest("POST", c.ReleaseJSON.CoreServerURL+api, strings.NewReader(""))
	if err != nil {
		return fmt.Errorf("reconnect: 创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("user-id", c.UserID)
	req.Header.Set("user-sead", c.Seed)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("reconnect: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("reconnect: 读取响应失败: %w", err)
	}

	encryptedResp, err := hex.DecodeString(string(respBody))
	if err != nil {
		return fmt.Errorf("reconnect: 响应解码失败: %w", err)
	}
	decryptedResp, err := G79HttpDecrypt(encryptedResp)
	if err != nil {
		return fmt.Errorf("reconnect: 响应解密失败: %w", err)
	}

	validJSON := GetValidJSON(decryptedResp)
	var refreshResp RefreshTokenResponse
	if err := json.Unmarshal(validJSON, &refreshResp); err != nil {
		return fmt.Errorf("reconnect: 解析响应失败: %v", err)
	}
	if refreshResp.Code != 0 {
		return fmt.Errorf("reconnect 失败 (code: %d): %s", refreshResp.Code, refreshResp.Message)
	}
	if refreshResp.Entity.Token == "" {
		return fmt.Errorf("reconnect: 响应缺少 token")
	}

	c.UserID = refreshResp.Entity.EntityID
	c.UserToken = refreshResp.Entity.Token
	if refreshResp.Entity.Sead != "" {
		c.Seed = refreshResp.Entity.Sead
	}
	return nil
}

// Logout 调用 /authentication/delete 登出。
func (c *Client) Logout() error {
	if c.UserID == "" || c.UserToken == "" {
		return nil
	}

	api := "/authentication/delete"
	bodyStr := `{"logout_type":1}`
	req, err := http.NewRequest("POST", c.ReleaseJSON.CoreServerURL+api, strings.NewReader(bodyStr))
	if err != nil {
		return fmt.Errorf("logout: 创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("user-id", c.UserID)
	req.Header.Set("user-token", CalculateDynamicToken(api, bodyStr, c.UserToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("logout: 请求失败: %w", err)
	}
	defer resp.Body.Close()
	_, _ = readResponseBody(resp)
	return nil
}

// StartAutoRefresh 启动自动 token 刷新循环，每 refreshInterval 刷新一次。
// 返回 stop 函数。刷新失败会重试（间隔缩短为 5 秒）。
func (c *Client) StartAutoRefresh(refreshInterval time.Duration) (stop func()) {
	if refreshInterval <= 0 {
		refreshInterval = 30 * time.Minute
	}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := c.RefreshToken(); err != nil {
					log.Printf("[G79] refresh token 失败: %v", err)
					// 失败后 5 秒重试
					select {
					case <-time.After(5 * time.Second):
						_ = c.RefreshToken()
					case <-stopCh:
						return
					}
				}
			case <-stopCh:
				return
			}
		}
	}()

	return func() {
		close(stopCh)
		<-doneCh
	}
}

// StopAutoRefresh 停止自动 token 刷新。
func (c *Client) StopAutoRefresh(stop func()) {
	if stop != nil {
		stop()
	}
}