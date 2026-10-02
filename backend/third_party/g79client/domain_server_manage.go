package g79client

import (
	"encoding/json"
	"fmt"
)

// ── 响应类型定义 ──

// DomainServerActionResponse 通用操作响应（开关服、备份等）。
type DomainServerActionResponse struct {
	Response
	Entity json.RawMessage `json:"entity"`
}

// DomainServerMembersResponse 成员列表响应。
type DomainServerMembersResponse struct {
	Response
	Entities []json.RawMessage `json:"entities"`
}

// DomainServerStorageListResponse 存档列表响应。
type DomainServerStorageListResponse struct {
	Response
	Entities []json.RawMessage `json:"entities"`
}

// DomainServerBackupListResponse 备份列表响应。
type DomainServerBackupListResponse struct {
	Response
	Entities []json.RawMessage `json:"entities"`
}

// DomainServerInviteCodeResponse 邀请码生成响应。
type DomainServerInviteCodeResponse struct {
	Response
	Entity json.RawMessage `json:"entity"`
}

// DomainServerOnlinePlayersResponse 在线玩家列表响应。
type DomainServerOnlinePlayersResponse struct {
	Response
	Entities []json.RawMessage `json:"entities"`
}

// DomainServerInviteListResponse 邀请码列表响应。
type DomainServerInviteListResponse struct {
	Response
	Entities []json.RawMessage `json:"entities"`
}

// DomainServerOwnerInvitationsResponse 所有者邀请记录响应。
type DomainServerOwnerInvitationsResponse struct {
	Response
	Entities []json.RawMessage `json:"entities"`
}

// DomainServerComponentPoolResponse 组件池响应。
type DomainServerComponentPoolResponse struct {
	Response
	Entities []json.RawMessage `json:"entities"`
}

// ── API 方法 ──

// StartDomainServer 开启山头服。
func (c *Client) StartDomainServer(sid string) (*DomainServerActionResponse, error) {
	api := "/domain-server/start-server"
	body := marshalJSON(map[string]any{"sid": sid})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerActionResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析开启山头服响应失败: %w", err)
	}
	return &result, nil
}

// StopDomainServer 关闭山头服。
func (c *Client) StopDomainServer(sid string) (*DomainServerActionResponse, error) {
	api := "/domain-server/stop-server"
	body := marshalJSON(map[string]any{"sid": sid})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerActionResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析关闭山头服响应失败: %w", err)
	}
	return &result, nil
}

// GetDomainServerMembers 获取山头服成员列表。
func (c *Client) GetDomainServerMembers(sid string, length, offset int) (*DomainServerMembersResponse, error) {
	api := "/domain-server/get-server-members"
	body := marshalJSON(map[string]any{
		"sid":    sid,
		"length": length,
		"offset": offset,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerMembersResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析山头服成员列表响应失败: %w", err)
	}
	return &result, nil
}

// GetDomainServerStorageList 获取存档列表。
func (c *Client) GetDomainServerStorageList(sid string) (*DomainServerStorageListResponse, error) {
	api := "/domain-server/get-server-storage-list"
	body := marshalJSON(map[string]any{"sid": sid})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerStorageListResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析山头存档列表响应失败: %w", err)
	}
	return &result, nil
}

// GetDomainServerBackupList 获取备份列表。
func (c *Client) GetDomainServerBackupList(sid string) (*DomainServerBackupListResponse, error) {
	api := "/domain-server/get-server-backup-list"
	body := marshalJSON(map[string]any{"sid": sid})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerBackupListResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析山头服备份列表响应失败: %w", err)
	}
	return &result, nil
}

// CreateDomainServerBackup 创建备份。
func (c *Client) CreateDomainServerBackup(sid string) (*DomainServerActionResponse, error) {
	api := "/domain-server/create-server-backup"
	body := marshalJSON(map[string]any{"sid": sid})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerActionResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析创建山头服备份响应失败: %w", err)
	}
	return &result, nil
}

// DeleteDomainServerStorage 删除存档。
func (c *Client) DeleteDomainServerStorage(sid string, backupID int) (*DomainServerActionResponse, error) {
	api := "/domain-server/delete-server-storage-list"
	body := marshalJSON(map[string]any{
		"sid":       sid,
		"backup_id": backupID,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerActionResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析删除山头服存档响应失败: %w", err)
	}
	return &result, nil
}

// RebuildDomainServerWorld 备份覆盖世界。
func (c *Client) RebuildDomainServerWorld(sid string, backupID int) (*DomainServerActionResponse, error) {
	api := "/domain-server/rebuild-world-by-storage"
	body := marshalJSON(map[string]any{
		"sid":       sid,
		"backup_id": backupID,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerActionResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析备份覆盖世界响应失败: %w", err)
	}
	return &result, nil
}

// InitDomainServer 初始化山头世界。
func (c *Client) InitDomainServer(sid string) (*DomainServerActionResponse, error) {
	api := "/domain-server/init-server"
	body := marshalJSON(map[string]any{"sid": sid})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerActionResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析初始化山头世界响应失败: %w", err)
	}
	return &result, nil
}

// GenerateDomainInviteCode 生成邀请码。
func (c *Client) GenerateDomainInviteCode(sid string, codeCount, validDay int) (*DomainServerInviteCodeResponse, error) {
	api := "/domain-server/generate-invite-code"
	body := marshalJSON(map[string]any{
		"sid":        sid,
		"code_count": codeCount,
		"valid_day":  validDay,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerInviteCodeResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析生成邀请码响应失败: %w", err)
	}
	return &result, nil
}

// RemoveDomainInviteCode 删除邀请码。
func (c *Client) RemoveDomainInviteCode(sid, code string) (*DomainServerActionResponse, error) {
	api := "/domain-server/remove-invite-code"
	body := marshalJSON(map[string]any{
		"sid":  sid,
		"code": code,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerActionResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析删除邀请码响应失败: %w", err)
	}
	return &result, nil
}

// UpdateDomainInviteCode 更新邀请码参数。
func (c *Client) UpdateDomainInviteCode(sid, code string, validDay int) (*DomainServerActionResponse, error) {
	api := "/domain-server/update-invite-code"
	body := marshalJSON(map[string]any{
		"sid":       sid,
		"code":      code,
		"valid_day": validDay,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerActionResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析更新邀请码响应失败: %w", err)
	}
	return &result, nil
}

// UpdateDomainServerBasic 更新山头基本设置（名称/描述/最大人数）。
// capacity=0 表示不修改。
func (c *Client) UpdateDomainServerBasic(sid, name, desc string, capacity int) (*DomainServerActionResponse, error) {
	api := "/domain-server/update-server-basic-settings"
	bodyMap := map[string]any{
		"sid":  sid,
		"name": name,
		"desc": desc,
	}
	if capacity > 0 {
		bodyMap["current_capacity"] = capacity
	}
	body := marshalJSON(bodyMap)
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerActionResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析更新山头基本设置响应失败: %w", err)
	}
	return &result, nil
}

// UpdateDomainServerGameSettings 更新游戏设置。
func (c *Client) UpdateDomainServerGameSettings(sid string, settings map[string]any) (*DomainServerActionResponse, error) {
	api := "/domain-server/update-server-game-settings"
	bodyMap := make(map[string]any, len(settings)+1)
	bodyMap["sid"] = sid
	for k, v := range settings {
		bodyMap[k] = v
	}
	body := marshalJSON(bodyMap)
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerActionResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析更新游戏设置响应失败: %w", err)
	}
	return &result, nil
}

// RemoveDomainServerMember 移除山头成员。fid="" 且 removeAll=true 时清除全部成员。
func (c *Client) RemoveDomainServerMember(sid, fid string, removeAll bool) (*DomainServerActionResponse, error) {
	api := "/domain-server/remove-server-member"
	body := marshalJSON(map[string]any{
		"sid":        sid,
		"fid":        fid,
		"remove_all": removeAll,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerActionResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析移除山头成员响应失败: %w", err)
	}
	return &result, nil
}

// InviteFriendToDomainServer 邀请好友进入山头服。
func (c *Client) InviteFriendToDomainServer(sid, uid string) (*DomainServerActionResponse, error) {
	api := "/domain-server/invite-friend"
	body := marshalJSON(map[string]any{
		"sid": sid,
		"uid": uid,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerActionResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析邀请好友响应失败: %w", err)
	}
	return &result, nil
}

// GetDomainServerOnlinePlayers 获取在线玩家。
func (c *Client) GetDomainServerOnlinePlayers(sid string) (*DomainServerOnlinePlayersResponse, error) {
	api := "/domain-server/get-online-players"
	body := marshalJSON(map[string]any{"sid": sid})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerOnlinePlayersResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析在线玩家列表响应失败: %w", err)
	}
	return &result, nil
}

// GetDomainServerInviteList 获取邀请码列表。
func (c *Client) GetDomainServerInviteList(sid string) (*DomainServerInviteListResponse, error) {
	api := "/domain-server/get-invite-list"
	body := marshalJSON(map[string]any{"sid": sid})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerInviteListResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析邀请码列表响应失败: %w", err)
	}
	return &result, nil
}

// GetDomainServerOwnerInvitations 获取所有者邀请记录。
func (c *Client) GetDomainServerOwnerInvitations(sid string) (*DomainServerOwnerInvitationsResponse, error) {
	api := "/domain-server/get-owner-invitations"
	body := marshalJSON(map[string]any{"sid": sid})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerOwnerInvitationsResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析所有者邀请记录响应失败: %w", err)
	}
	return &result, nil
}

// GetDomainServerComponentPool 获取组件池。
func (c *Client) GetDomainServerComponentPool(sid string, length, offset int) (*DomainServerComponentPoolResponse, error) {
	api := "/domain-server/get-server-component-pool"
	body := marshalJSON(map[string]any{
		"sid":    sid,
		"length": length,
		"offset": offset,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result DomainServerComponentPoolResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析组件池响应失败: %w", err)
	}
	return &result, nil
}
