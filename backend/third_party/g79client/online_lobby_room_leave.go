package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// # 联机房间 — 离开房间
// 对应 /online-lobby-room-enter/leave-room
// 调用 EnterOnlineLobbyRoom 后需要离开时调用

// 在线大厅离开房间响应
type OnlineLobbyRoomLeaveResponse struct {
	Response
}

// # 联机房间 — 离开在线大厅房间
func (c *Client) LeaveOnlineLobbyRoom(roomID string) (*OnlineLobbyRoomLeaveResponse, error) {
	api := "/online-lobby-room-enter/leave-room"

	requestData := map[string]interface{}{
		"team_quit": false,
		"room_id":   roomID,
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

	var leaveResp OnlineLobbyRoomLeaveResponse
	if err := json.Unmarshal(respBody, &leaveResp); err != nil {
		return nil, fmt.Errorf("解析离开房间响应失败: %v, 响应内容: %s", err, string(respBody))
	}
	return &leaveResp, nil
}
