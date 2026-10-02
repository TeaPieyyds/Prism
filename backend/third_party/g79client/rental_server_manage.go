package g79client

import (
	"encoding/json"
	"fmt"
)

// RentalServerManageResponse 租赁服管理通用响应。
type RentalServerManageResponse struct {
	Response
	Entity   json.RawMessage   `json:"entity"`
	Entities []json.RawMessage `json:"entities"`
}

// RentalServerBackupEntity 租赁服备份实体。
type RentalServerBackupEntity struct {
	WorldID    string `json:"world_id"`
	BackupID   int    `json:"backup_id"`
	BackupName string `json:"backup_name"`
	BackupTS   int64  `json:"backup_ts"`
	ServerID   string `json:"server_id"`
	Status     int    `json:"status"`
	FileSize   int64  `json:"file_size"`
}

// ── my-rental-server-info/query/info (POST) ──

// GetMyRentalServerInfo 查询我的租赁服详细信息。
func (c *Client) GetMyRentalServerInfo(serverID string) (*RentalServerManageResponse, error) {
	body := marshalJSON(map[string]any{"server_id": serverID})
	resp, err := c.makeRequest("/my-rental-server-info/query/info", body)
	if err != nil {
		return nil, fmt.Errorf("my-rental-server-info: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("my-rental-server-info: %w", err)
	}
	return &result, nil
}

// ── my-rental-server/initialize (POST) ──

// InitializeRentalServer 初始化租赁服。
func (c *Client) InitializeRentalServer(serverID, worldID string) (*RentalServerManageResponse, error) {
	body := marshalJSON(map[string]any{
		"server_id": serverID,
		"world_id":  worldID,
	})
	resp, err := c.makeRequest("/my-rental-server/initialize", body)
	if err != nil {
		return nil, fmt.Errorf("initialize-rental-server: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("initialize-rental-server: %w", err)
	}
	return &result, nil
}

// ── rental-server-backup/query/search-by-server (POST) ──

// SearchRentalServerBackups 搜索租赁服备份列表。
func (c *Client) SearchRentalServerBackups(worldID, sid string) (*RentalServerManageResponse, error) {
	body := marshalJSON(map[string]any{
		"world_id": worldID,
		"sid":      sid,
	})
	resp, err := c.makeRequest("/rental-server-backup/query/search-by-server", body)
	if err != nil {
		return nil, fmt.Errorf("rental-server-backup-search: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("rental-server-backup-search: %w", err)
	}
	return &result, nil
}

// ── rental-server-backup-restore (POST) ──

// RestoreRentalServerBackup 恢复租赁服备份。
func (c *Client) RestoreRentalServerBackup(worldID string, backupID int) (*RentalServerManageResponse, error) {
	body := marshalJSON(map[string]any{
		"world_id":  worldID,
		"backup_id": backupID,
	})
	resp, err := c.makeRequest("/rental-server-backup-restore", body)
	if err != nil {
		return nil, fmt.Errorf("rental-server-backup-restore: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("rental-server-backup-restore: %w", err)
	}
	return &result, nil
}

// ── rental-server-backup-upload/create-async (POST) ──

// CreateRentalServerBackup 创建租赁服备份（异步）。
// worldID 世界ID，sid 服务器ID，backupID 备份序号（从备份列表获取），backupName 备份名称。
func (c *Client) CreateRentalServerBackup(worldID, sid string, backupID int, backupName string) (*RentalServerManageResponse, error) {
	body := marshalJSON(map[string]any{"world_id": worldID, "sid": sid, "backup_id": backupID, "backup_name": backupName})
	resp, err := c.makeRequest("/rental-server-backup-upload/create-async", body)
	if err != nil {
		return nil, fmt.Errorf("rental-server-backup-create: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("rental-server-backup-create: %w", err)
	}
	return &result, nil
}

// ── rental-server-favorite/get-all (POST) ──

// GetAllRentalServerFavorites 获取所有租赁服收藏列表。
func (c *Client) GetAllRentalServerFavorites() (*RentalServerManageResponse, error) {
	resp, err := c.makeRequest("/rental-server-favorite/get-all", "{}")
	if err != nil {
		return nil, fmt.Errorf("rental-server-favorite-get-all: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("rental-server-favorite-get-all: %w", err)
	}
	return &result, nil
}

// ── rental-server-favorite/query-status (POST) ──

// QueryRentalServerFavoriteStatus 查询租赁服收藏状态。
func (c *Client) QueryRentalServerFavoriteStatus(serverID string) (*RentalServerManageResponse, error) {
	body := marshalJSON(map[string]any{"server_id": serverID})
	resp, err := c.makeRequest("/rental-server-favorite/query-status", body)
	if err != nil {
		return nil, fmt.Errorf("rental-server-favorite-query-status: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("rental-server-favorite-query-status: %w", err)
	}
	return &result, nil
}

// ── rental-server-favorite/update (POST) ──

// UpdateRentalServerFavorite 更新租赁服收藏状态。isFavorite: 0 取消收藏，1 收藏。
func (c *Client) UpdateRentalServerFavorite(serverID string, isFavorite int) (*RentalServerManageResponse, error) {
	body := marshalJSON(map[string]any{
		"server_id":   serverID,
		"is_favorite": isFavorite,
	})
	resp, err := c.makeRequest("/rental-server-favorite/update", body)
	if err != nil {
		return nil, fmt.Errorf("rental-server-favorite-update: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("rental-server-favorite-update: %w", err)
	}
	return &result, nil
}

// ── rental-server-like/get (POST) ──

// GetRentalServerLike 获取租赁服点赞状态。
func (c *Client) GetRentalServerLike(serverID string) (*RentalServerManageResponse, error) {
	body := marshalJSON(map[string]any{"server_id": serverID})
	resp, err := c.makeRequest("/rental-server-like/get", body)
	if err != nil {
		return nil, fmt.Errorf("rental-server-like-get: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("rental-server-like-get: %w", err)
	}
	return &result, nil
}

// ── rental-server-like/update (POST) ──

// UpdateRentalServerLike 更新租赁服点赞状态。isLike: 0 取消点赞，1 点赞。
func (c *Client) UpdateRentalServerLike(serverID string, isLike int) (*RentalServerManageResponse, error) {
	body := marshalJSON(map[string]any{
		"server_id": serverID,
		"is_like":   isLike,
	})
	resp, err := c.makeRequest("/rental-server-like/update", body)
	if err != nil {
		return nil, fmt.Errorf("rental-server-like-update: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("rental-server-like-update: %w", err)
	}
	return &result, nil
}

// ── rental-server-player/query/search-by-server (POST) ──

// SearchRentalServerPlayers 搜索租赁服玩家列表。
func (c *Client) SearchRentalServerPlayers(serverID string, length, offset int) (*RentalServerManageResponse, error) {
	body := marshalJSON(map[string]any{
		"server_id": serverID,
		"length":    length,
		"offset":    offset,
	})
	resp, err := c.makeRequest("/rental-server-player/query/search-by-server", body)
	if err != nil {
		return nil, fmt.Errorf("rental-server-player-search: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("rental-server-player-search: %w", err)
	}
	return &result, nil
}

// ── rental-server-player/update (POST) ──

// UpdateRentalServerPlayer 更新租赁服玩家（踢出/封禁/解封）。
// action 可选值: "kick", "ban", "unban"。
// unban 时使用 entity_id 而非 uid，且传入 status: 0。
func (c *Client) UpdateRentalServerPlayer(serverID, uid, action string) (*RentalServerManageResponse, error) {
	bodyMap := map[string]any{
		"server_id": serverID,
		"action":    action,
	}
	if action == "unban" {
		bodyMap["entity_id"] = uid
		bodyMap["status"] = 0
	} else {
		bodyMap["uid"] = uid
	}
	body := marshalJSON(bodyMap)
	resp, err := c.makeRequest("/rental-server-player/update", body)
	if err != nil {
		return nil, fmt.Errorf("rental-server-player-update: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("rental-server-player-update: %w", err)
	}
	return &result, nil
}

// ── rental-server/query/available-friend-server (POST) ──

// GetFriendRentalServers 获取好友的租赁服列表。
func (c *Client) GetFriendRentalServers() (*RentalServerManageResponse, error) {
	resp, err := c.makeRequest("/rental-server/query/available-friend-server", "{}")
	if err != nil {
		return nil, fmt.Errorf("rental-server-friend: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("rental-server-friend: %w", err)
	}
	return &result, nil
}

// ── rental-server/query/recent-visit-server (POST) ──

// GetRecentVisitRentalServers 获取最近访问的租赁服列表。
func (c *Client) GetRecentVisitRentalServers() (*RentalServerManageResponse, error) {
	resp, err := c.makeRequest("/rental-server/query/recent-visit-server", "{}")
	if err != nil {
		return nil, fmt.Errorf("rental-server-recent-visit: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("rental-server-recent-visit: %w", err)
	}
	return &result, nil
}

// ── rental-server-world-settings/get (POST) ──

// GetRentalServerWorldSettings 获取租赁服世界设置。
func (c *Client) GetRentalServerWorldSettings(serverID string) (*RentalServerManageResponse, error) {
	body := marshalJSON(map[string]any{"server_id": serverID})
	resp, err := c.makeRequest("/rental-server-world-settings/get", body)
	if err != nil {
		return nil, fmt.Errorf("rental-server-world-settings: %w", err)
	}
	var result RentalServerManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("rental-server-world-settings: %w", err)
	}
	return &result, nil
}
