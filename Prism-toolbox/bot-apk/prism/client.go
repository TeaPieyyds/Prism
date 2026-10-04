package prism

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP:    &http.Client{Timeout: 30 * time.Second, Transport: secureClient.Transport},
	}
}

func (c *Client) do(method, path string, body any) ([]byte, error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, r)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *Client) get(path string) ([]byte, error) { return c.do("GET", path, nil) }
func (c *Client) post(path string, body any) ([]byte, error) {
	return c.do("POST", path, body)
}

// ─── Auth ──────────────────────────────────────────────────────────

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Captcha  string `json:"captcha,omitempty"`
}

type LoginResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Error   string `json:"error"`
	User    *User  `json:"user"`
}

type User struct {
	ID       int    `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Token    string `json:"token"`
	Disabled bool   `json:"disabled"`
}

func (c *Client) Login(email, password, captcha string) (*LoginResponse, error) {
	data, err := c.post("/api/auth/login", &LoginRequest{
		Email: email, Password: password, Captcha: captcha,
	})
	if err != nil {
		return nil, err
	}
	var resp LoginResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w (raw: %s)", err, string(data))
	}
	return &resp, nil
}

// ─── Accounts ──────────────────────────────────────────────────────

type Account struct {
	ID                  int    `json:"id"`
	DisplayName         string `json:"display_name"`
	UID                 string `json:"uid"`
	Status              string `json:"status"`
	GrowthLevel         string `json:"growth_level"`
	Score               string `json:"score"`
	SkinNumber          string `json:"skin_number"`
	SkinURL             string `json:"skin_url"`
	AvatarImageURL      string `json:"avatar_image_url"`
	CapeNumber          string `json:"cape_number"`
	IsVip               bool   `json:"is_vip"`
	Source              string `json:"source"`
	Disabled            bool   `json:"disabled"`
	OwnerID             int    `json:"owner_id"`
	CreatedBy           int    `json:"created_by"`
	CanReclaim          bool   `json:"can_reclaim"`
	IsShared            bool   `json:"is_shared"`
	IsActive            bool   `json:"is_active"`
	AutoRefreshEnabled  bool   `json:"auto_refresh_enabled"`
	HasSharedCopy       bool   `json:"has_shared_copy"`
}

type AccountsResponse struct {
	OK       bool      `json:"ok"`
	Accounts []Account `json:"accounts"`
}

func (c *Client) GetAccounts() ([]Account, error) {
	data, err := c.get("/api/accounts")
	if err != nil {
		return nil, err
	}
	var resp AccountsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse accounts: %w", err)
	}
	return resp.Accounts, nil
}

type ActiveAccountResponse struct {
	Account
	OK                  bool   `json:"ok"`
	AccessGameFlag      string `json:"access_game_flag"`
	AntiAddictionStatus string `json:"anti_addiction_status"`
	RealnameStatus      string `json:"realname_status"`
	Signature           string `json:"signature"`
}

func (c *Client) SetActiveAccount(accountID int) error {
	data, err := c.do("PATCH", "/api/accounts/active", map[string]int{"account_id": accountID})
	if err != nil {
		return err
	}
	var resp ActiveAccountResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("parse active account: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("set active account failed")
	}
	return nil
}

func (c *Client) GetActiveAccount() (*ActiveAccountResponse, error) {
	data, err := c.get("/api/accounts/active")
	if err != nil {
		return nil, err
	}
	var resp ActiveAccountResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse active account: %w", err)
	}
	return &resp, nil
}

// ─── Server ────────────────────────────────────────────────────────

type ServerDetail struct {
	OK          bool   `json:"ok"`
	Error       string `json:"error"`
	ServerName  string `json:"server_name"`
	ServerCode  string `json:"server_code"`
	IPAddress   string `json:"ip_address"`
	Port        int    `json:"port"`
	PlayerCount int    `json:"player_count"`
	Capacity    int    `json:"capacity"`
	Version     string `json:"version"`
}

type ServerFindResponse struct {
	OK      bool          `json:"ok"`
	Code    int           `json:"code"`
	Servers []ServerEntry `json:"servers"`
}

type ServerEntry struct {
	EntityID    string `json:"entity_id"`
	Name        string `json:"name"`
	PlayerCount int    `json:"player_count"`
	Capacity    int    `json:"capacity"`
	Status      int    `json:"status"`
	MCVersion   string `json:"mc_version"`
}

func (c *Client) GetServerInfo(keyword string) (*ServerEntry, error) {
	data, err := c.get("/api/server/find?keyword=" + keyword)
	if err != nil {
		return nil, err
	}
	var resp ServerFindResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse server find: %w", err)
	}
	if !resp.OK || len(resp.Servers) == 0 {
		return nil, fmt.Errorf("server not found")
	}
	return &resp.Servers[0], nil
}

func (c *Client) GetServerDetail(serverID string) (*ServerDetail, error) {
	data, err := c.get("/api/server/detail?server_id=" + serverID)
	if err != nil {
		return nil, err
	}
	var resp ServerDetail
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse server detail: %w", err)
	}
	return &resp, nil
}

type PlayerListResponse struct {
	OK      bool     `json:"ok"`
	Error   string   `json:"error"`
	Players []Player `json:"players"`
	Count   int      `json:"count"`
}

type Player struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}

func (c *Client) GetServerPlayers(serverID string) (*PlayerListResponse, error) {
	data, err := c.get("/api/server/players?server_id=" + serverID)
	if err != nil {
		return nil, err
	}
	var resp PlayerListResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse server players: %w", err)
	}
	return &resp, nil
}

// ─── Phoenix Login ─────────────────────────────────────────────────

type PhoenixLoginRequest struct {
	LoginToken      string `json:"login_token,omitempty"`
	UserName        string `json:"username,omitempty"`
	Password        string `json:"password,omitempty"`
	ServerCode      string `json:"server_code"`
	ServerPassword  string `json:"server_passcode"`
	ClientPublicKey string `json:"client_public_key"`
}

type PhoenixLoginResponse struct {
	Success      bool              `json:"success"`
	Message      string            `json:"message"`
	ServerMsg    string            `json:"server_msg"`
	Token        string            `json:"token"`
	RespondTo    string            `json:"respond_to"`
	IPAddress    string            `json:"ip_address"`
	UID          string            `json:"uid"`
	UserName     string            `json:"username"`
	GrowthLevel  int               `json:"growth_level"`
	ChainInfo    string            `json:"chainInfo"`
	SkinInfo     map[string]any    `json:"skin_info"`
	OutfitInfo   map[string]any    `json:"outfit_info"`
}

func (c *Client) PhoenixLogin(serverCode, serverPassword, publicKey string) (*PhoenixLoginResponse, error) {
	data, err := c.post("/api/phoenix/login", &PhoenixLoginRequest{
		LoginToken:      c.Token,
		ServerCode:      serverCode,
		ServerPassword:  serverPassword,
		ClientPublicKey: publicKey,
	})
	if err != nil {
		return nil, err
	}
	var resp PhoenixLoginResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse phoenix login: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("phoenix login failed: %s", resp.Message)
	}
	return &resp, nil
}

// ─── MCP Check Challenge ───────────────────────────────────────────

func (c *Client) TransferStartType(content string) (string, error) {
	data, err := c.get("/api/phoenix/transfer_start_type?content=" + content)
	if err != nil {
		return "", err
	}
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    string `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("transfer start type: %w", err)
	}
	if !resp.Success {
		return "", fmt.Errorf("transfer start type failed: %s", resp.Message)
	}
	return resp.Data, nil
}

func (c *Client) TransferCheckNum(jsonData string) (string, error) {
	data, err := c.post("/api/phoenix/transfer_check_num", map[string]string{"data": jsonData})
	if err != nil {
		return "", err
	}
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    string `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("transfer check num: %w", err)
	}
	if !resp.Success {
		return "", fmt.Errorf("transfer check num failed: %s", resp.Message)
	}
	return resp.Data, nil
}
