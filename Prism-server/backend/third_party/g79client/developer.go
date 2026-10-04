package g79client

import (
	"encoding/json"
	"fmt"
)

// DeveloperResponse 开发者相关接口通用响应。
type DeveloperResponse struct {
	Response
	Entity   json.RawMessage   `json:"entity"`
	Entities []json.RawMessage `json:"entities"`
}

// FellowDeveloper 关注开发者。
func (c *Client) FellowDeveloper(developerID string) (*DeveloperResponse, error) {
	api := "/pe-developer-homepage/fellow_developer"
	body := marshalJSON(map[string]any{
		"developer_id": developerID,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DeveloperResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("fellow-developer: %w", err)
	}
	return &result, nil
}

// CancelFellowDeveloper 取消关注开发者。
func (c *Client) CancelFellowDeveloper(developerID string) (*DeveloperResponse, error) {
	api := "/pe-developer-homepage/cancel_fellow"
	body := marshalJSON(map[string]any{
		"developer_id": developerID,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DeveloperResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("cancel-fellow-developer: %w", err)
	}
	return &result, nil
}

// CommitDeveloperComment 提交开发者评论。
func (c *Client) CommitDeveloperComment(developerID, content string) (*DeveloperResponse, error) {
	api := "/pe-developer-homepage-comment/commit_homepage_comment"
	body := marshalJSON(map[string]any{
		"developer_id": developerID,
		"content":      content,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DeveloperResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("commit-developer-comment: %w", err)
	}
	return &result, nil
}

// GetDeveloperUploads 获取开发者上传资源。
func (c *Client) GetDeveloperUploads(developerID string, length, offset int) (*DeveloperResponse, error) {
	api := "/pe-developer-homepage/get-developer-upload"
	body := marshalJSON(map[string]any{
		"developer_id": developerID,
		"length":       length,
		"offset":       offset,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DeveloperResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("get-developer-uploads: %w", err)
	}
	return &result, nil
}

// LoadDeveloperHomepage 加载开发者主页。
func (c *Client) LoadDeveloperHomepage(id string) (*DeveloperResponse, error) {
	api := "/pe-developer-homepage/load_developer_homepage/get"
	body := marshalJSON(map[string]any{
		"id": id,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DeveloperResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("load-developer-homepage: %w", err)
	}
	return &result, nil
}

// LoadDeveloperItems 加载开发者物品列表。
func (c *Client) LoadDeveloperItems(developerID string, length, offset int) (*DeveloperResponse, error) {
	api := "/pe-developer-homepage/load_items_by_developer_info_id"
	body := marshalJSON(map[string]any{
		"developer_id": developerID,
		"length":       length,
		"offset":       offset,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DeveloperResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("load-developer-items: %w", err)
	}
	return &result, nil
}

// SearchDeveloperByKeyword 按关键词搜索开发者。
func (c *Client) SearchDeveloperByKeyword(keyword string, length, offset int) (*DeveloperResponse, error) {
	api := "/pe-developer-homepage/search_developer_by_keyword"
	body := marshalJSON(map[string]any{
		"keyword": keyword,
		"length":  length,
		"offset":  offset,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DeveloperResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("search-developer: %w", err)
	}
	return &result, nil
}
