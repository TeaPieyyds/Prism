package g79client

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// makeRequest constructs, signs, and sends a POST G79 API request.
func (c *Client) makeRequest(api, bodyStr string) (*http.Response, error) {
	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(bodyStr))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)
	req.Header.Set("user-token", CalculateDynamicToken(api, bodyStr, c.UserToken))
	return c.httpClient.Do(req)
}

func (c *Client) makeGetRequest(api string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.ReleaseJSON.ApiGatewayUrl+api, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("user-id", c.UserID)
	req.Header.Set("user-token", CalculateDynamicToken(api, "{}", c.UserToken))
	return c.httpClient.Do(req)
}

func (c *Client) readAndUnmarshal(resp *http.Response, target any) error {
	defer resp.Body.Close()
	body, err := readResponseBody(resp)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

// ── server-time (GET) ──

// ServerTimeResponse 服务器时间响应。
type ServerTimeResponse struct {
	Response
	Entity struct {
		Current int64 `json:"current"`
	} `json:"entity"`
}

// GetServerTime 获取服务器当前时间。
func (c *Client) GetServerTime() (*ServerTimeResponse, error) {
	api := "/server-time"
	resp, err := c.makeGetRequest(api)
	if err != nil {
		return nil, err
	}
	var result ServerTimeResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析server-time响应失败: %w", err)
	}
	return &result, nil
}

// ── get-currency-online (POST) ──

// CurrencyOnlineEntity 在线货币实体。
type CurrencyOnlineEntity struct {
	RestCurrencyTime int64  `json:"rest_currency_time"`
	Date             string `json:"date"`
}

// CurrencyOnlineResponse 在线货币查询响应。
type CurrencyOnlineResponse struct {
	Response
	Entity CurrencyOnlineEntity `json:"entity"`
}

// GetCurrencyOnline 查询在线时长货币剩余。
func (c *Client) GetCurrencyOnline() (*CurrencyOnlineResponse, error) {
	api := "/get-currency-online/"
	resp, err := c.makeRequest(api, "{}")
	if err != nil {
		return nil, err
	}
	var result CurrencyOnlineResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析get-currency-online响应失败: %w", err)
	}
	return &result, nil
}

// ── message-notify (POST) ──

// MessageNotifyResponse 消息轮询响应。
type MessageNotifyResponse struct {
	Response
	Entities []any   `json:"entities"`
	Extime   float64 `json:"extime"`
}

// MessageNotify 轮询消息通知。notifyType: 1 或 2。
func (c *Client) MessageNotify(notifyType int) (*MessageNotifyResponse, error) {
	api := "/message-notify"
	bodyBytes, _ := json.Marshal(map[string]any{"type": notifyType})
	resp, err := c.makeRequest(api, string(bodyBytes))
	if err != nil {
		return nil, err
	}
	var result MessageNotifyResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析message-notify响应失败: %w", err)
	}
	return &result, nil
}

// ── salog (POST) ──

// Salog 上报行为日志。logType 例如 "launcher_rent_page_view"。
func (c *Client) Salog(logType string) error {
	return c.salogBody(map[string]any{"type": logType})
}

// SalogWithDetail 上报带详细信息的日志。
func (c *Client) SalogWithDetail(detail map[string]any) error {
	return c.salogBody(detail)
}

func (c *Client) salogBody(data map[string]any) error {
	api := "/salog"
	bodyBytes, _ := json.Marshal(data)
	resp, err := c.makeRequest(api, string(bodyBytes))
	if err != nil {
		return err
	}
	var result Response
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return fmt.Errorf("解析salog响应失败: %w", err)
	}
	if result.Code != 0 {
		return fmt.Errorf("salog失败 (code=%d): %s", result.Code, result.Message)
	}
	return nil
}

// ── pe-get-daily-growth-info (POST) ──

// DailyGrowthResponse 每日成长信息响应。
type DailyGrowthResponse struct {
	Response
	Entity map[string]int `json:"entity"`
}

// GetDailyGrowthInfo 获取每日成长信息。
func (c *Client) GetDailyGrowthInfo() (*DailyGrowthResponse, error) {
	api := "/pe-get-daily-growth-info"
	resp, err := c.makeRequest(api, "{}")
	if err != nil {
		return nil, err
	}
	var result DailyGrowthResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析daily-growth-info响应失败: %w", err)
	}
	return &result, nil
}

// ── user-currency-new/query (POST) ──

// CurrencyQueryEntity 货币查询响应实体。
// 注意：last_cash / current_cash 服务器可能返回数字或字符串，使用 Uncertain。
type CurrencyQueryEntity struct {
	LastCash        Uncertain `json:"last_cash"`
	LastCashTime    string    `json:"last_cash_time"`
	CurrentCash     Uncertain `json:"current_cash"`
	CurrentCashTime string    `json:"current_cash_time"`
	PayDiamond      int       `json:"pay_diamond"`
	FreeDiamond     int       `json:"free_diamond"`
	CurrencyTime    int64     `json:"currency_time"`
	BindDiamond     string    `json:"bind_diamond"`
}

// CurrencyQueryResponse 货币查询响应。
type CurrencyQueryResponse struct {
	Response
	Entity CurrencyQueryEntity `json:"entity"`
}

// CurrencyItem 货币条目。
type CurrencyItem struct {
	Type string `json:"type"`
	ID   int    `json:"id"`
}

// QueryCurrency 查询货币余额。
func (c *Client) QueryCurrency(items []CurrencyItem) (*CurrencyQueryResponse, error) {
	api := "/user-currency-new/query"
	bodyBytes, _ := json.Marshal(map[string]any{"currency_list": items})
	resp, err := c.makeRequest(api, string(bodyBytes))
	if err != nil {
		return nil, err
	}
	var result CurrencyQueryResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析currency查询响应失败: %w", err)
	}
	return &result, nil
}

// ── user-stat/get-user-state (POST) ──

// UserStateEntity 用户统计状态实体。
type UserStateEntity struct {
	EntityID               string `json:"entity_id"`
	PersonalPageViewCount  int    `json:"personal_page_view_count"`
	PersonalPageLikeCount  int    `json:"personal_page_like_count"`
	VideoViewCount         int    `json:"video_view_count"`
	GamePurchaseCount      int    `json:"game_purchase_count"`
	ComponentPurchaseCount int    `json:"component_purchase_count"`
	FriendCnt              int    `json:"friend_cnt"`
	PublicUserFansCnt      int    `json:"public_user_fans_cnt"`
	MyPublicFollowCnt      int    `json:"my_public_follow_cnt"`
	HasLike                bool   `json:"has_like"`
}

// UserStateResponse 用户统计状态响应。
type UserStateResponse struct {
	Response
	Entity UserStateEntity `json:"entity"`
}

// GetUserState 查询用户统计状态。
func (c *Client) GetUserState(searchID string) (*UserStateResponse, error) {
	api := "/user-stat/get-user-state"
	bodyBytes, _ := json.Marshal(map[string]any{"search_id": searchID})
	resp, err := c.makeRequest(api, string(bodyBytes))
	if err != nil {
		return nil, err
	}
	var result UserStateResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析user-state响应失败: %w", err)
	}
	return &result, nil
}

// ═══════════════════════════════════════════════
// 新接口：模拟真实客户端 3.8.25 行为
// ═══════════════════════════════════════════════

// ── pe-get-grow-lv-exp (POST) ──

// GrowLevelExpEntity 成长等级经验实体。
type GrowLevelExpEntity struct {
	Lv       int    `json:"lv"`
	Exp      int    `json:"exp"`
	NeedExp  int    `json:"need_exp"`
	IsVip    int    `json:"is_vip"`
	Decorate []any  `json:"decorate"`
}

// GrowLevelExpResponse 成长等级经验响应。
type GrowLevelExpResponse struct {
	Response
	SummaryMD5 string            `json:"summary_md5"`
	Entity     GrowLevelExpEntity `json:"entity"`
}

// GetGrowLevelExp 获取成长等级经验。
func (c *Client) GetGrowLevelExp() (*GrowLevelExpResponse, error) {
	resp, err := c.makeRequest("/pe-get-grow-lv-exp", "{}")
	if err != nil {
		return nil, err
	}
	var result GrowLevelExpResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析pe-get-grow-lv-exp响应失败: %w", err)
	}
	return &result, nil
}

// ── user-notify-cnt (POST) ──

// UserNotifyCountResponse 通知计数响应。
type UserNotifyCountResponse struct {
	Response
	Time   int64   `json:"time"`
	Extime float64 `json:"extime"`
	Entity struct {
		Like        int `json:"like"`
		Comment     int `json:"comment"`
		At          int `json:"at"`
		CommentLike int `json:"comment_like"`
	} `json:"entity"`
}

// GetUserNotifyCount 获取用户通知数。
func (c *Client) GetUserNotifyCount() (*UserNotifyCountResponse, error) {
	resp, err := c.makeRequest("/user-notify-cnt/", "{}")
	if err != nil {
		return nil, err
	}
	var result UserNotifyCountResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析user-notify-cnt响应失败: %w", err)
	}
	return &result, nil
}

// ── activity_task/get_task_info (POST) ──

// ActivityTaskInfoResponse 活动任务信息响应。
type ActivityTaskInfoResponse struct {
	Response
	Entity json.RawMessage `json:"entity"`
}

// GetActivityTaskInfo 获取活动任务信息。
func (c *Client) GetActivityTaskInfo(taskID int) (*ActivityTaskInfoResponse, error) {
	bodyBytes, _ := json.Marshal(map[string]any{"id": taskID})
	resp, err := c.makeRequest("/activity_task/get_task_info", string(bodyBytes))
	if err != nil {
		return nil, err
	}
	var result ActivityTaskInfoResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析activity_task响应失败: %w", err)
	}
	return &result, nil
}

// ── interconn/client-event/fire (POST) ──

// FireClientEvent 上报客户端事件（如火币活动页面浏览）。
func (c *Client) FireClientEvent(eventName string, eventValue any) error {
	bodyBytes, _ := json.Marshal(map[string]any{
		"event_name":  eventName,
		"event_value": eventValue,
	})
	resp, err := c.makeRequest("/interconn/client-event/fire", string(bodyBytes))
	if err != nil {
		return err
	}
	var result Response
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return fmt.Errorf("解析client-event响应失败: %w", err)
	}
	if result.Code != 0 {
		return fmt.Errorf("FireClientEvent失败 (code=%d): %s", result.Code, result.Message)
	}
	return nil
}

// ── get-general-mail-list (POST) ──

// GeneralMailListResponse 邮件列表响应。
type GeneralMailListResponse struct {
	Response
	SummaryMD5 string            `json:"summary_md5"`
	Entities   []json.RawMessage `json:"entities"`
}

// GetGeneralMailList 获取邮件列表。mailID=0 表示从最新开始。
func (c *Client) GetGeneralMailList(mailID int64) (*GeneralMailListResponse, error) {
	bodyBytes, _ := json.Marshal(map[string]any{"mailid": mailID})
	resp, err := c.makeRequest("/get-general-mail-list/", string(bodyBytes))
	if err != nil {
		return nil, err
	}
	var result GeneralMailListResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析general-mail-list响应失败: %w", err)
	}
	return &result, nil
}

// ── home-shop-all (POST) ──

// HomeShopAllResponse 商店全量数据响应。
type HomeShopAllResponse struct {
	Response
	Entity json.RawMessage `json:"entity"`
}

// GetHomeShopAll 获取商店全量数据。
func (c *Client) GetHomeShopAll() (*HomeShopAllResponse, error) {
	resp, err := c.makeRequest("/home-shop-all", "{}")
	if err != nil {
		return nil, err
	}
	var result HomeShopAllResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析home-shop-all响应失败: %w", err)
	}
	return &result, nil
}

// ── user_bag/query_user_bag (POST) ──

// UserBagResponse 背包查询响应。
type UserBagResponse struct {
	Response
	Entity json.RawMessage `json:"entity"`
}

// GetUserBag 查询用户背包物品。
func (c *Client) GetUserBag(firstType, length, offset int) (*UserBagResponse, error) {
	bodyBytes, _ := json.Marshal(map[string]any{
		"first_type": firstType,
		"length":     length,
		"offset":     offset,
	})
	resp, err := c.makeRequest("/user_bag/query_user_bag", string(bodyBytes))
	if err != nil {
		return nil, err
	}
	var result UserBagResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析user_bag响应失败: %w", err)
	}
	return &result, nil
}

// ── pe-get-user-setting-list (POST) ──

// PeUserSettingListResponse 用户设置列表响应。
type PeUserSettingListResponse struct {
	Response
	Entity json.RawMessage `json:"entity"`
}

// PeGetUserSettingList 获取用户设置列表。
func (c *Client) PeGetUserSettingList(settings []string) (*PeUserSettingListResponse, error) {
	bodyBytes, _ := json.Marshal(map[string]any{"settings": settings})
	resp, err := c.makeRequest("/pe-get-user-setting-list", string(bodyBytes))
	if err != nil {
		return nil, err
	}
	var result PeUserSettingListResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析pe-get-user-setting-list响应失败: %w", err)
	}
	return &result, nil
}

// ── has-shop-red-dots-v2 (POST) ──

// HasShopRedDotsV2Response 商店红点响应。
type HasShopRedDotsV2Response struct {
	Response
	Entity json.RawMessage `json:"entity"`
}

// GetHasShopRedDotsV2 获取商店红点状态。
func (c *Client) GetHasShopRedDotsV2() (*HasShopRedDotsV2Response, error) {
	resp, err := c.makeRequest("/has-shop-red-dots-v2", "{}")
	if err != nil {
		return nil, err
	}
	var result HasShopRedDotsV2Response
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析has-shop-red-dots-v2响应失败: %w", err)
	}
	return &result, nil
}

// ── user-page-friends-with-detail (POST) ──

// UserPageFriendsResponse 好友列表响应。
type UserPageFriendsResponse struct {
	Response
	Entity json.RawMessage `json:"entity"`
}

// GetUserPageFriendsWithDetail 获取好友列表（带详情）。
func (c *Client) GetUserPageFriendsWithDetail(limit, offset int) (*UserPageFriendsResponse, error) {
	bodyBytes, _ := json.Marshal(map[string]any{"limit": limit, "offset": offset})
	resp, err := c.makeRequest("/user-page-friends-with-detail", string(bodyBytes))
	if err != nil {
		return nil, err
	}
	var result UserPageFriendsResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析user-page-friends-with-detail响应失败: %w", err)
	}
	return &result, nil
}

// ── pe-item/common-abtest (POST) ──

// PeItemCommonABTestResponse AB测试配置响应。
type PeItemCommonABTestResponse struct {
	Response
	Entity json.RawMessage `json:"entity"`
}

// GetPeItemCommonABTest 获取AB测试配置。
func (c *Client) GetPeItemCommonABTest() (*PeItemCommonABTestResponse, error) {
	resp, err := c.makeRequest("/pe-item/common-abtest", "{}")
	if err != nil {
		return nil, err
	}
	var result PeItemCommonABTestResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析pe-item/common-abtest响应失败: %w", err)
	}
	return &result, nil
}

// ── user-chat-bubble-get (POST) ──

// UserChatBubbleGetResponse 聊天泡泡响应。
type UserChatBubbleGetResponse struct {
	Response
	Entity json.RawMessage `json:"entity"`
}

// GetUserChatBubble 获取聊天泡泡信息。
func (c *Client) GetUserChatBubble() (*UserChatBubbleGetResponse, error) {
	resp, err := c.makeRequest("/user-chat-bubble-get", "{}")
	if err != nil {
		return nil, err
	}
	var result UserChatBubbleGetResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析user-chat-bubble-get响应失败: %w", err)
	}
	return &result, nil
}

// HeartbeatConfig 在线心跳配置参数。
type HeartbeatConfig struct {
	// DrpfInterval 控制 DRPF 心跳频率（默认 3s）
	DrpfInterval time.Duration
	// CurrencyInterval 控制 get-currency-online 查询频率（默认 60s）
	CurrencyInterval time.Duration
	// ServerTimeInterval 控制 server-time 查询频率（默认 30s）
	ServerTimeInterval time.Duration
	// DrpfOpts 传递给 DrpfPing 的场景参数
	DrpfOpts DrpfPingOptions
	// OnError 可选错误回调；nil 则只打日志
	OnError func(err error)
}

func (cfg *HeartbeatConfig) fillDefaults() {
	if cfg.DrpfInterval <= 0 {
		cfg.DrpfInterval = 3 * time.Second
	}
	if cfg.CurrencyInterval <= 0 {
		cfg.CurrencyInterval = 60 * time.Second
	}
	if cfg.ServerTimeInterval <= 0 {
		cfg.ServerTimeInterval = 30 * time.Second
	}
	if cfg.OnError == nil {
		cfg.OnError = func(err error) {
			log.Printf("[HB] heartbeat error: %v", err)
		}
	}
}

// StartOnlineHeartbeat 启动在线心跳循环，模拟真实客户端保持在线状态。
// 返回 stop 函数，调用后停止心跳。每个 Client 可独立控制启停。
func (c *Client) StartOnlineHeartbeat(cfg HeartbeatConfig) (stop func()) {
	cfg.fillDefaults()

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)

		drpfTick := time.NewTicker(cfg.DrpfInterval)
		currencyTick := time.NewTicker(cfg.CurrencyInterval)
		serverTimeTick := time.NewTicker(cfg.ServerTimeInterval)

		// 首次立即执行
		c.doCurrencyOnline(cfg)
		c.doServerTime(cfg)

		for {
			select {
			case <-drpfTick.C:
				if err := c.DrpfPing(cfg.DrpfOpts); err != nil {
					cfg.OnError(fmt.Errorf("drpf: %w", err))
				}

			case <-currencyTick.C:
				c.doCurrencyOnline(cfg)

			case <-serverTimeTick.C:
				c.doServerTime(cfg)

			case <-stopCh:
				drpfTick.Stop()
				currencyTick.Stop()
				serverTimeTick.Stop()
				return
			}
		}
	}()

	return func() {
		close(stopCh)
		<-doneCh
	}
}

// StopOnlineHeartbeat 是 StartOnlineHeartbeat 的便捷包装，直接停止当前 client 的心跳。
func (c *Client) StopOnlineHeartbeat(stop func()) {
	if stop != nil {
		stop()
	}
}

func (c *Client) doCurrencyOnline(cfg HeartbeatConfig) {
	if _, err := c.GetCurrencyOnline(); err != nil {
		cfg.OnError(fmt.Errorf("currency-online: %w", err))
	}
}

func (c *Client) doServerTime(cfg HeartbeatConfig) {
	if _, err := c.GetServerTime(); err != nil {
		cfg.OnError(fmt.Errorf("server-time: %w", err))
	}
}
