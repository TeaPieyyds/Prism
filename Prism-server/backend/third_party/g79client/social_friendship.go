package g79client

import (
	"encoding/json"
	"fmt"
)

// SocialResponse 社交/好友相关接口的统一响应体。
type SocialResponse struct {
	Response
	Entity   json.RawMessage   `json:"entity"`
	Entities []json.RawMessage `json:"entities"`
}

// GetRecentPlayedPlayers 获取最近一起玩过的玩家列表。
func (c *Client) GetRecentPlayedPlayers(length, offset int) (*SocialResponse, error) {
	api := "/friend-ship/get-together-recent-player"
	body := marshalJSON(map[string]any{
		"length": length,
		"offset": offset,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result SocialResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("get-recent-players: %w", err)
	}
	return &result, nil
}

// ApplyIntimacy 申请亲密关系。
func (c *Client) ApplyIntimacy(fuid string, intimacyType int) (*SocialResponse, error) {
	api := "/friend-ship/intimacy-apply"
	body := marshalJSON(map[string]any{
		"fuid": fuid,
		"type": intimacyType,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result SocialResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("intimacy-apply: %w", err)
	}
	return &result, nil
}

// GetIntimacyApplyList 获取亲密关系申请列表。
func (c *Client) GetIntimacyApplyList(length, offset int) (*SocialResponse, error) {
	api := "/friend-ship/intimacy-apply-list"
	body := marshalJSON(map[string]any{
		"length": length,
		"offset": offset,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result SocialResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("intimacy-apply-list: %w", err)
	}
	return &result, nil
}

// GetIntimacyBuildable 获取可建立亲密关系的列表。
func (c *Client) GetIntimacyBuildable(length, offset int) (*SocialResponse, error) {
	api := "/friend-ship/intimacy-buildable"
	body := marshalJSON(map[string]any{
		"length": length,
		"offset": offset,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result SocialResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("intimacy-buildable: %w", err)
	}
	return &result, nil
}

// GetIntimacyList 获取亲密关系列表。
func (c *Client) GetIntimacyList(length, offset int) (*SocialResponse, error) {
	api := "/friend-ship/intimacy-list"
	body := marshalJSON(map[string]any{
		"length": length,
		"offset": offset,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result SocialResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("intimacy-list: %w", err)
	}
	return &result, nil
}

// ReplyIntimacy 回复亲密关系申请。
func (c *Client) ReplyIntimacy(fuid string, agree bool) (*SocialResponse, error) {
	api := "/friend-ship/intimacy-reply"
	body := marshalJSON(map[string]any{
		"fuid":  fuid,
		"agree": agree,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result SocialResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("intimacy-reply: %w", err)
	}
	return &result, nil
}

// GetAllFriends 获取所有好友列表。
func (c *Client) GetAllFriends() (*SocialResponse, error) {
	api := "/user-allfriends"
	resp, err := c.makeRequest(api, "{}")
	if err != nil {
		return nil, err
	}
	var result SocialResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("all-friends: %w", err)
	}
	return &result, nil
}

// GetAllFriendsWithDetail 获取所有好友列表（带详情）。
func (c *Client) GetAllFriendsWithDetail(length, offset int) (*SocialResponse, error) {
	api := "/user-allfriends-with-detail"
	body := marshalJSON(map[string]any{
		"length": length,
		"offset": offset,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result SocialResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("all-friends-with-detail: %w", err)
	}
	return &result, nil
}

// GetMultiFriendsDetail 批量获取多个好友的详细信息。
func (c *Client) GetMultiFriendsDetail(fuidList []string) (*SocialResponse, error) {
	api := "/user-multi-friends-with-detail"
	body := marshalJSON(map[string]any{
		"fuid_list": fuidList,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result SocialResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("multi-friends-detail: %w", err)
	}
	return &result, nil
}

// CommentMoment 评论指定动态。
func (c *Client) CommentMoment(msgID string, pushUID uint64, content string) (*SocialResponse, error) {
	api := "/user-comment-moment"
	body := marshalJSON(map[string]any{
		"msg_id":   msgID,
		"push_uid": pushUID,
		"content":  content,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result SocialResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("comment-moment: %w", err)
	}
	return &result, nil
}

// ReportMoment 举报指定动态。
func (c *Client) ReportMoment(msgID, reason string) (*SocialResponse, error) {
	api := "/user-moment-report"
	body := marshalJSON(map[string]any{
		"msg_id": msgID,
		"reason": reason,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result SocialResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("report-moment: %w", err)
	}
	return &result, nil
}

// QueryOtherUserDetail 查询其他用户的详细信息。
func (c *Client) QueryOtherUserDetail(entityID string) (*SocialResponse, error) {
	api := "/user-detail/query/other"
	body := marshalJSON(map[string]any{
		"entity_id": entityID,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result SocialResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("query-other-user-detail: %w", err)
	}
	return &result, nil
}

// GetGMToken 获取 GM 管理令牌。
func (c *Client) GetGMToken() (*SocialResponse, error) {
	api := "/get-gm-token/"
	resp, err := c.makeRequest(api, "{}")
	if err != nil {
		return nil, err
	}
	var result SocialResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("get-gm-token: %w", err)
	}
	return &result, nil
}
