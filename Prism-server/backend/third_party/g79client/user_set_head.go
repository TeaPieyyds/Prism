package g79client

import "fmt"

// ── set-user-head (POST) ──

// SetUserHeadResponse 设置头像响应。
type SetUserHeadResponse struct {
	Response
	Entity struct {
		EntityID    string `json:"entity_id"`
		HeadImage   string `json:"head_image"`
		HeadImageCD int64  `json:"head_image_cd"`
	} `json:"entity"`
}

// SetUserHead 设置用户头像。rollbackLast=0 设置新头像，=1 回退到上一个。
func (c *Client) SetUserHead(headImageURL string, rollbackLast int) (*SetUserHeadResponse, error) {
	body := marshalJSON(map[string]any{
		"rollback_last": rollbackLast,
		"head_image":    headImageURL,
	})
	resp, err := c.makeRequest("/set-user-head/", body)
	if err != nil {
		return nil, fmt.Errorf("set-user-head: %w", err)
	}
	var result SetUserHeadResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析set-user-head响应失败: %w", err)
	}
	return &result, nil
}

// ── user-head-url-v2 (POST) ──

// HeadURLItem 头像URL项。
type HeadURLItem struct {
	ID         int    `json:"id"`
	Type       int    `json:"type"`
	StaticURL  string `json:"static_url"`
	Tips       string `json:"tips"`
	JumpType   string `json:"jump_type"`
	JumpTarget string `json:"jump_target"`
	Name       string `json:"name"`
	Status     int    `json:"status"`
	FirstType  int    `json:"first_type"`
	InnerID    int    `json:"inner_id"`
	IsGif      int    `json:"is_gif"`
	ItemID     int64  `json:"item_id"`
	ItemName   string `json:"item_name"`
	URL        string `json:"url"`
	URLGIF     string `json:"url_gif,omitempty"`
}

// GetUserHeadURLsResponse 获取可用头像列表响应。
type GetUserHeadURLsResponse struct {
	Response
	Entities []HeadURLItem `json:"entities"`
}

// GetUserHeadURLs 获取当前可用的头像列表。
func (c *Client) GetUserHeadURLs() (*GetUserHeadURLsResponse, error) {
	resp, err := c.makeRequest("/user-head-url-v2", "{}")
	if err != nil {
		return nil, fmt.Errorf("user-head-url-v2: %w", err)
	}
	var result GetUserHeadURLsResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析user-head-url-v2响应失败: %w", err)
	}
	return &result, nil
}
