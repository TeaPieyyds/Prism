package g79client

import (
	"encoding/json"
	"fmt"
)

// LobbyManageResponse 联机大厅通用响应。
type LobbyManageResponse struct {
	Response
	Entity   json.RawMessage   `json:"entity"`
	Entities []json.RawMessage `json:"entities"`
}

// ── online-lobby-room/get-multi-room-info (POST) ──

// GetMultiRoomInfo 批量获取联机大厅房间信息。
func (c *Client) GetMultiRoomInfo(roomIDList []string) (*LobbyManageResponse, error) {
	body := marshalJSON(map[string]any{"room_id_list": roomIDList})
	resp, err := c.makeRequest("/online-lobby-room/get-multi-room-info", body)
	if err != nil {
		return nil, fmt.Errorf("lobby-multi-room: %w", err)
	}
	var result LobbyManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("lobby-multi-room: %w", err)
	}
	return &result, nil
}

// ── online-lobby-room/query/friend-playing-room (POST) ──

// GetFriendPlayingRooms 获取好友正在游玩的房间列表。
func (c *Client) GetFriendPlayingRooms() (*LobbyManageResponse, error) {
	resp, err := c.makeRequest("/online-lobby-room/query/friend-playing-room", "{}")
	if err != nil {
		return nil, fmt.Errorf("lobby-friend-playing-rooms: %w", err)
	}
	var result LobbyManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("lobby-friend-playing-rooms: %w", err)
	}
	return &result, nil
}

// ── online-lobby-room/query/get-slogan-config (POST) ──

// GetLobbySloganConfig 获取联机大厅标语配置。
func (c *Client) GetLobbySloganConfig() (*LobbyManageResponse, error) {
	resp, err := c.makeRequest("/online-lobby-room/query/get-slogan-config", "{}")
	if err != nil {
		return nil, fmt.Errorf("lobby-slogan-config: %w", err)
	}
	var result LobbyManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("lobby-slogan-config: %w", err)
	}
	return &result, nil
}

// ── online-lobby-room/query/search-by-name-v2 (POST) ──

// SearchLobbyRoomByNameV2 按名称搜索联机大厅房间（v2）。
func (c *Client) SearchLobbyRoomByNameV2(keyword string, length, offset int) (*LobbyManageResponse, error) {
	body := marshalJSON(map[string]any{
		"keyword": keyword,
		"length":  length,
		"offset":  offset,
	})
	resp, err := c.makeRequest("/online-lobby-room/query/search-by-name-v2", body)
	if err != nil {
		return nil, fmt.Errorf("lobby-search-by-name-v2: %w", err)
	}
	var result LobbyManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("lobby-search-by-name-v2: %w", err)
	}
	return &result, nil
}

// ── query_lobby_room_list (POST) ──

// QueryLobbyRoomList 查询联机大厅房间列表。
func (c *Client) QueryLobbyRoomList(roomIDList []string) (*LobbyManageResponse, error) {
	body := marshalJSON(map[string]any{"room_id_list": roomIDList})
	resp, err := c.makeRequest("/query_lobby_room_list", body)
	if err != nil {
		return nil, fmt.Errorf("query-lobby-room-list: %w", err)
	}
	var result LobbyManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("query-lobby-room-list: %w", err)
	}
	return &result, nil
}

// ── single-room-info (POST) ──

// GetSingleRoomInfo 获取单个房间信息。
func (c *Client) GetSingleRoomInfo(roomID string) (*LobbyManageResponse, error) {
	body := marshalJSON(map[string]any{"room_id": roomID})
	resp, err := c.makeRequest("/single-room-info", body)
	if err != nil {
		return nil, fmt.Errorf("single-room-info: %w", err)
	}
	var result LobbyManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("single-room-info: %w", err)
	}
	return &result, nil
}

// ── room-related (POST) ──

// GetRelatedRooms 获取关联房间。
func (c *Client) GetRelatedRooms(roomID string) (*LobbyManageResponse, error) {
	body := marshalJSON(map[string]any{"room_id": roomID})
	resp, err := c.makeRequest("/room-related", body)
	if err != nil {
		return nil, fmt.Errorf("room-related: %w", err)
	}
	var result LobbyManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("room-related: %w", err)
	}
	return &result, nil
}

// ── room-with-friend (POST) ──

// GetRoomsWithFriends 获取好友所在的房间列表。
func (c *Client) GetRoomsWithFriends() (*LobbyManageResponse, error) {
	resp, err := c.makeRequest("/room-with-friend", "{}")
	if err != nil {
		return nil, fmt.Errorf("room-with-friend: %w", err)
	}
	var result LobbyManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("room-with-friend: %w", err)
	}
	return &result, nil
}

// ── send-lobby-invite-notice (POST) ──

// SendLobbyInviteNotice 发送联机大厅邀请通知。
func (c *Client) SendLobbyInviteNotice(roomID, uid string) (*LobbyManageResponse, error) {
	body := marshalJSON(map[string]any{
		"room_id": roomID,
		"uid":     uid,
	})
	resp, err := c.makeRequest("/send-lobby-invite-notice", body)
	if err != nil {
		return nil, fmt.Errorf("send-lobby-invite-notice: %w", err)
	}
	var result LobbyManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("send-lobby-invite-notice: %w", err)
	}
	return &result, nil
}

// ── transfer-room/get-room-chat-group-id (POST) ──

// GetRoomChatGroupID 获取房间聊天群组ID。
func (c *Client) GetRoomChatGroupID(roomID string) (*LobbyManageResponse, error) {
	body := marshalJSON(map[string]any{"room_id": roomID})
	resp, err := c.makeRequest("/transfer-room/get-room-chat-group-id", body)
	if err != nil {
		return nil, fmt.Errorf("room-chat-group-id: %w", err)
	}
	var result LobbyManageResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("room-chat-group-id: %w", err)
	}
	return &result, nil
}
