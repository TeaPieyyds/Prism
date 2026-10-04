package g79client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ── image-upload-token (POST) ──

// ImageUploadTokenEntity 上传令牌响应。
type ImageUploadTokenEntity struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

// GetImageUploadTokenResponse 获取上传令牌响应。
type GetImageUploadTokenResponse struct {
	Response
	Entity ImageUploadTokenEntity `json:"entity"`
}

// GetImageUploadToken 获取图片上传令牌，用于上传头像等。
func (c *Client) GetImageUploadToken() (*GetImageUploadTokenResponse, error) {
	resp, err := c.makeRequest("/image-upload-token", `{"expire":false,"fp_type":0}`)
	if err != nil {
		return nil, fmt.Errorf("image-upload-token: %w", err)
	}
	var result GetImageUploadTokenResponse
	if err := c.readAndUnmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("解析image-upload-token响应失败: %w", err)
	}
	return &result, nil
}

// ── fp.ps.netease.com 文件上传 ──

// FileUploadResponse 文件上传响应。
type FileUploadResponse struct {
	URL     string `json:"url"`
	MIME    string `json:"mime"`
	FSize   int    `json:"fsize"`
	MD5     string `json:"md5"`
	PicSize []int  `json:"picSize"`
}

// UploadFileToFP 上传原始文件数据到网易 FP 服务器（application/octet-stream）。
func (c *Client) UploadFileToFP(uploadURL, authorization string, fileData []byte) (*FileUploadResponse, error) {
	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(fileData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", authorization)
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("上传请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var result FileUploadResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析上传响应失败: %w", err)
	}
	return &result, nil
}
