package g79client

import (
	"encoding/json"
	"fmt"
)

// GroupResponse 群组相关接口的统一响应体。
type GroupResponse struct {
	Response
	Entity   json.RawMessage   `json:"entity"`
	Entities []json.RawMessage `json:"entities"`
}

// CreateGroup 创建群组。
func (c *Client) CreateGroup(groupName string) (*GroupResponse, error) {
	api := "/create-group"
	body := marshalJSON(map[string]any{
		"group_name": groupName,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result GroupResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("create-group: %w", err)
	}
	return &result, nil
}

// ChangeGroupName 修改群组名称。
func (c *Client) ChangeGroupName(groupID, groupName string) (*GroupResponse, error) {
	api := "/change-group-name"
	body := marshalJSON(map[string]any{
		"group_id":   groupID,
		"group_name": groupName,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result GroupResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("change-group-name: %w", err)
	}
	return &result, nil
}

// ChangeGroupInvitePerm 修改群组邀请权限。
func (c *Client) ChangeGroupInvitePerm(groupID string, perm int) (*GroupResponse, error) {
	api := "/change-group-invite-perm"
	body := marshalJSON(map[string]any{
		"group_id": groupID,
		"perm":     perm,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result GroupResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("change-group-invite-perm: %w", err)
	}
	return &result, nil
}

// UpdateGroupTop 更新群组置顶消息。
func (c *Client) UpdateGroupTop(groupID, topMsg string) (*GroupResponse, error) {
	api := "/update-group-top"
	body := marshalJSON(map[string]any{
		"group_id": groupID,
		"top_msg":  topMsg,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result GroupResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("update-group-top: %w", err)
	}
	return &result, nil
}

// GetAllGroups 获取所有群组列表。
func (c *Client) GetAllGroups() (*GroupResponse, error) {
	api := "/get-all-groups/"
	resp, err := c.makeRequest(api, "{}")
	if err != nil {
		return nil, err
	}
	var result GroupResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("get-all-groups: %w", err)
	}
	return &result, nil
}

// GetGroupMembers 获取群组成员列表。
func (c *Client) GetGroupMembers(groupID string) (*GroupResponse, error) {
	api := "/get-group-member"
	body := marshalJSON(map[string]any{
		"group_id": groupID,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result GroupResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("get-group-members: %w", err)
	}
	return &result, nil
}

// InviteToGroup 邀请用户加入群组。
func (c *Client) InviteToGroup(groupID, uid string) (*GroupResponse, error) {
	api := "/group-invite"
	body := marshalJSON(map[string]any{
		"group_id": groupID,
		"uid":      uid,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result GroupResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("invite-to-group: %w", err)
	}
	return &result, nil
}
