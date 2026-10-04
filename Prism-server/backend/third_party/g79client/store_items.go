package g79client

import (
	"encoding/json"
	"fmt"
)

// StoreResponse 商店相关接口通用响应。
type StoreResponse struct {
	Response
	Entity   json.RawMessage   `json:"entity"`
	Entities []json.RawMessage `json:"entities"`
}

// SearchItemChannelListByID 按频道ID搜索频道列表。
func (c *Client) SearchItemChannelListByID(channelID, length, offset int) (*StoreResponse, error) {
	api := "/item-channel/query/search-item-channel-list-by-id/"
	body := marshalJSON(map[string]any{
		"channel_id": channelID,
		"length":     length,
		"offset":     offset,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result StoreResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("search-item-channel-list: %w", err)
	}
	return &result, nil
}

// SearchItemsByIDs 按ID列表搜索物品。
func (c *Client) SearchItemsByIDs(itemIDList []string) (*StoreResponse, error) {
	api := "/item/query/search-by-ids"
	body := marshalJSON(map[string]any{
		"item_id_list": itemIDList,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result StoreResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("search-items-by-ids: %w", err)
	}
	return &result, nil
}

// GetItemDetailV2 获取物品详情V2。
func (c *Client) GetItemDetailV2(itemID string) (*StoreResponse, error) {
	api := "/pe-item-detail-v2"
	body := marshalJSON(map[string]any{
		"item_id": itemID,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result StoreResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("item-detail-v2: %w", err)
	}
	return &result, nil
}

// SearchItemsByIDList 按频道ID和物品ID列表搜索物品。
func (c *Client) SearchItemsByIDList(channelID int, itemIDList []string, isRefund bool) (*StoreResponse, error) {
	api := "/pe-item/query/search-by-id-list"
	body := marshalJSON(map[string]any{
		"channel_id":   channelID,
		"item_id_list": itemIDList,
		"is_refund":    isRefund,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result StoreResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("search-items-by-id-list: %w", err)
	}
	return &result, nil
}

// SearchItemsByKeyword 按关键词搜索物品。
func (c *Client) SearchItemsByKeyword(keyword string, length, offset int) (*StoreResponse, error) {
	api := "/pe-item/query/search-by-keyword/"
	body := marshalJSON(map[string]any{
		"keyword": keyword,
		"length":  length,
		"offset":  offset,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result StoreResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("search-items-by-keyword: %w", err)
	}
	return &result, nil
}

// SearchItemsByType 按类型搜索物品。
func (c *Client) SearchItemsByType(itemType string, length, offset int) (*StoreResponse, error) {
	api := "/pe-item/query/search-by-type/"
	body := marshalJSON(map[string]any{
		"type":   itemType,
		"length": length,
		"offset": offset,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result StoreResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("search-items-by-type: %w", err)
	}
	return &result, nil
}

// SearchLobbyResByKeyword 按关键词搜索大厅资源。
func (c *Client) SearchLobbyResByKeyword(keyword string, length, offset int) (*StoreResponse, error) {
	api := "/pe-item/query/search-lobby-res-by-keyword"
	body := marshalJSON(map[string]any{
		"keyword": keyword,
		"length":  length,
		"offset":  offset,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result StoreResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("search-lobby-res: %w", err)
	}
	return &result, nil
}

// SearchRecommendAssociation 搜索推荐联想。
func (c *Client) SearchRecommendAssociation(keyword string) (*StoreResponse, error) {
	api := "/pe-item/search-recommend-association-v2"
	body := marshalJSON(map[string]any{
		"keyword": keyword,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result StoreResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("search-recommend-association: %w", err)
	}
	return &result, nil
}

// PurchaseUserItem 购买用户物品。
func (c *Client) PurchaseUserItem(itemID string, count int) (*StoreResponse, error) {
	api := "/pe-user-item-purchase/"
	body := marshalJSON(map[string]any{
		"item_id": itemID,
		"count":   count,
	})
	resp, err := c.makeRequest(api, body)
	if err != nil {
		return nil, err
	}
	var result StoreResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("purchase-user-item: %w", err)
	}
	return &result, nil
}

// GetShoppingCartListV2 获取购物车列表V2。
func (c *Client) GetShoppingCartListV2() (*StoreResponse, error) {
	api := "/user-shopping-cart/get-list-v2"
	resp, err := c.makeRequest(api, "{}")
	if err != nil {
		return nil, err
	}
	var result StoreResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("shopping-cart-list: %w", err)
	}
	return &result, nil
}
