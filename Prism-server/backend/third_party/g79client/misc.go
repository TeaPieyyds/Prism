package g79client

import (
	"encoding/json"
	"fmt"
)

// MiscResponse 杂项接口通用响应。
type MiscResponse struct {
	Response
	Entity   json.RawMessage   `json:"entity"`
	Entities []json.RawMessage `json:"entities"`
}

// GetFlashSaleInfo 获取限时抢购信息。
func (c *Client) GetFlashSaleInfo() (*MiscResponse, error) {
	api := "/flash-sale/info"
	resp, err := c.makeRequest(api, "{}")
	if err != nil {
		return nil, err
	}
	var result MiscResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("flash-sale-info: %w", err)
	}
	return &result, nil
}

// CreateFlashSaleOrder 创建限时抢购订单。
func (c *Client) CreateFlashSaleOrder(itemID string) (*MiscResponse, error) {
	api := "/flash-sale/order"
	body := marshalJSON(map[string]any{
		"item_id": itemID,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result MiscResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("flash-sale-order: %w", err)
	}
	return &result, nil
}

// GetUserPetDetail 获取用户宠物详情。
func (c *Client) GetUserPetDetail() (*MiscResponse, error) {
	api := "/home-get-user-pet-detail"
	resp, err := c.makeRequest(api, "{}")
	if err != nil {
		return nil, err
	}
	var result MiscResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("user-pet-detail: %w", err)
	}
	return &result, nil
}

// BatchGetGrowLevelExp 批量获取成长等级经验。
func (c *Client) BatchGetGrowLevelExp(uidList []string) (*MiscResponse, error) {
	api := "/pe-batch-get-grow-lv-exp"
	body := marshalJSON(map[string]any{
		"uid_list": uidList,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result MiscResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("batch-get-grow-lv-exp: %w", err)
	}
	return &result, nil
}

// GetCloudSaveVIP 获取云存档VIP信息。
func (c *Client) GetCloudSaveVIP() (*MiscResponse, error) {
	api := "/pe-cloud-save/get-vip"
	resp, err := c.makeRequest(api, "{}")
	if err != nil {
		return nil, err
	}
	var result MiscResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("cloud-save-vip: %w", err)
	}
	return &result, nil
}

// GetUserAchievementBasicInfo 获取用户成就基础信息。
func (c *Client) GetUserAchievementBasicInfo(entityID string) (*MiscResponse, error) {
	api := "/pe-get-user-achievement-basic-info"
	body := marshalJSON(map[string]any{
		"entity_id": entityID,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result MiscResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("user-achievement-basic-info: %w", err)
	}
	return &result, nil
}

// RecordUserAction 记录用户行为事件。
func (c *Client) RecordUserAction(event string, data map[string]any) (*MiscResponse, error) {
	api := "/interconn/web/activtiy/record-user-action"
	body := marshalJSON(map[string]any{
		"event": event,
		"data":  data,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result MiscResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("record-user-action: %w", err)
	}
	return &result, nil
}

// SendDiagnosticLog 发送诊断日志。
func (c *Client) SendDiagnosticLog(data map[string]any) (*MiscResponse, error) {
	api := "/diagnostic-log"
	body := marshalJSON(data)
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result MiscResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("diagnostic-log: %w", err)
	}
	return &result, nil
}
