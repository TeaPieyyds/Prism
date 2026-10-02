package g79client

import "fmt"

// ── rental-server-control/update (POST) ──

// ControlRentalServerResponse 租赁服控制响应。
type ControlRentalServerResponse struct {
	Response
}

// ControlRentalServer 控制租赁服启停。status=0 关闭，=1 启动。
func (c *Client) ControlRentalServer(serverID string, status int) (*ControlRentalServerResponse, error) {
	body := marshalJSON(map[string]any{
		"status":    status,
		"server_id": serverID,
		"version":   nil,
	})
	resp, err := c.makeRequest("/rental-server-control/update", body)
	if err != nil {
		return nil, fmt.Errorf("rental-server-control: %w", err)
	}
	var result ControlRentalServerResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析rental-server-control响应失败: %w", err)
	}
	return &result, nil
}

// ── rental-server-control/get-status (POST) ──

// RentalServerStatusResponse 租赁服状态响应。
type RentalServerStatusResponse struct {
	Response
	Entity struct {
		EntityID string `json:"entity_id"`
		Status   int    `json:"status"`
		Version  string `json:"version"`
	} `json:"entity"`
}

// GetRentalServerStatus 查询租赁服状态。
func (c *Client) GetRentalServerStatus(serverID string) (*RentalServerStatusResponse, error) {
	body := marshalJSON(map[string]any{"server_id": serverID})
	resp, err := c.makeRequest("/rental-server-control/get-status", body)
	if err != nil {
		return nil, fmt.Errorf("rental-server-get-status: %w", err)
	}
	var result RentalServerStatusResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析rental-server-get-status响应失败: %w", err)
	}
	return &result, nil
}

// ── my-rental-server/update (POST) ──

// UpdateRentalServerResponse 更新租赁服信息响应。
type UpdateRentalServerResponse struct {
	Response
	Entity map[string]any `json:"entity"`
}

// UpdateRentalServer 更新租赁服信息（公告、名称、可见性等）。
// params 可包含 server_name, brief_summary, min_level, visibility, pwd, image_url 等,
// 不传的字段设为 nil 表示不修改。
func (c *Client) UpdateRentalServer(serverID string, params map[string]any) (*UpdateRentalServerResponse, error) {
	params["server_id"] = serverID
	body := marshalJSON(params)
	resp, err := c.makeRequest("/my-rental-server/update", body)
	if err != nil {
		return nil, fmt.Errorf("my-rental-server-update: %w", err)
	}
	var result UpdateRentalServerResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析my-rental-server-update响应失败: %w", err)
	}
	return &result, nil
}

// ── my-rental-server/query/search-by-user (POST) ──

// MyRentalServerEntity 我的租赁服实体（简化）。
type MyRentalServerEntity struct {
	EntityID     string `json:"entity_id"`
	Name         string `json:"name"`
	BriefSummary string `json:"brief_summary"`
	Status       int    `json:"status"`
	MCVersion    string `json:"mc_version"`
	Capacity     int    `json:"capacity"`
	PlayerCount  int    `json:"player_count"`
}

// SearchMyRentalServersResponse 搜索我的租赁服响应。
type SearchMyRentalServersResponse struct {
	Response
	Entities []MyRentalServerEntity `json:"entities"`
}

// SearchMyRentalServers 查询我租用的租赁服列表。
func (c *Client) SearchMyRentalServers(length, offset int) (*SearchMyRentalServersResponse, error) {
	body := marshalJSON(map[string]any{
		"length": length,
		"offset": offset,
	})
	resp, err := c.makeRequest("/my-rental-server/query/search-by-user", body)
	if err != nil {
		return nil, fmt.Errorf("my-rental-server-query: %w", err)
	}
	var result SearchMyRentalServersResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析my-rental-server-query响应失败: %w", err)
	}
	return &result, nil
}
