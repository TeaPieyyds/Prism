package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// # 联机房间 — 按 res_id 列出所有房间
// 对应 /online-lobby-room/query/list-room-by-res-id
// 与 SearchOnlineLobbyRoomByKeyword 的区别：不传 keyword，扫描指定商品/地图下的全部房间

// 在线大厅房间列表条目（list-room-by-res-id 返回的实体字段子集）
type OnlineLobbyRoomListEntity struct {
	EntityID             Uncertain   `json:"entity_id"`
	RoomName             string      `json:"room_name"`
	Slogan               string      `json:"slogan"`
	Password             Uncertain   `json:"password"`
	ResID                Uncertain   `json:"res_id"`
	MaxCount             Uncertain   `json:"max_count"`
	CurNum               Uncertain   `json:"cur_num"`
	AllowSave            Uncertain   `json:"allow_save"`
	Visibility           Uncertain   `json:"visibility"`
	OwnerID              Uncertain   `json:"owner_id"`
	SaveID               string      `json:"save_id"`
	Version              string      `json:"version"`
	WorldID              string      `json:"world_id"`
	Tag                  string      `json:"tag"`
	MinLevel             Uncertain   `json:"min_level"`
	OrderID              Uncertain   `json:"order_id"`
	GameStatus           Uncertain   `json:"game_status"`
	SaveSize             Uncertain   `json:"save_size"`
	FIDs                 []Uncertain `json:"fids"`
	LobbyManifestVersion string      `json:"lobby_manifest_version"`
	BehaviourUUID        string      `json:"behaviour_uuid"`
	PlayingUUID          string      `json:"playing_uuid"`
	TeamID               string      `json:"team_id"`
	ChatGroupID          string      `json:"chat_group_id"`
	ChatGroupReady       string      `json:"chat_group_ready"`
	MemberUIDs           []string    `json:"member_uids"`
}

// 在线大厅房间列表响应
type OnlineLobbyRoomListResponse struct {
	Response
	Entities []OnlineLobbyRoomListEntity `json:"entities"`
	Total    Uncertain                   `json:"total"`
}

// # 联机房间 — 按商品 ID 扫描房间列表
// resID 为商品/地图 ID，length 默认 10000，offset 默认 0
func (c *Client) ListOnlineLobbyRoomByResID(resID string, length, offset int) (*OnlineLobbyRoomListResponse, error) {
	api := "/online-lobby-room/query/list-room-by-res-id"

	if length <= 0 {
		length = 10000
	}

	requestData := map[string]interface{}{
		"lobby_manifest_version": "",
		"length":                 length,
		"version":                GameVersion,
		"with_friend":            true,
		"offset":                 offset,
		"res_id":                 resID,
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("user-id", c.UserID)

	token := CalculateDynamicToken(api, string(jsonData), c.UserToken)
	req.Header.Set("user-token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var listResp OnlineLobbyRoomListResponse
	if err := json.Unmarshal(respBody, &listResp); err != nil {
		return nil, fmt.Errorf("解析在线大厅房间列表响应失败: %v, 响应内容: %s", err, string(respBody))
	}
	return &listResp, nil
}
