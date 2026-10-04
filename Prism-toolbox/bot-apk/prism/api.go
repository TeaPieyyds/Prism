package prism

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

const BaseURL = "https://prism.adblanlu.qzz.io"

var secureClient = &http.Client{
	Timeout: 15 * 1000000000,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Resolver: &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "udp", "8.8.8.8:53")
				},
			},
		}).DialContext,
	},
}

func post(path string, body any, token string) (map[string]any, error) {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = strings.NewReader(string(data))
	}

	req, _ := http.NewRequest("POST", BaseURL+path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := secureClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("post %s: %w", path, err)
	}
	return result, nil
}

func get(path string, params map[string]string, token string) (map[string]any, error) {
	req, _ := http.NewRequest("GET", BaseURL+path, nil)
	q := req.URL.Query()
	for k, v := range params {
		q.Add(k, v)
	}
	req.URL.RawQuery = q.Encode()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := secureClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("get %s: %w", path, err)
	}
	return result, nil
}

// PrismLogin authenticates with email/password and returns user info including token.
// POST /api/auth/login
func PrismLogin(email, password string) (map[string]any, error) {
	return post("/api/auth/login", map[string]string{
		"email":    email,
		"password": password,
	}, "")
}

// PrismGetNuts retrieves nut (板栗) transaction records.
// GET /api/admin/nuts?limit=&offset=
func PrismGetNuts(limit, offset string, token string) (map[string]any, error) {
	return get("/api/admin/nuts", map[string]string{
		"limit":  limit,
		"offset": offset,
	}, token)
}

// PrismAdjustNuts adjusts a user's nut balance.
// POST /api/admin/nuts/adjust
func PrismAdjustNuts(userID, amount int, reason, token string) (map[string]any, error) {
	return post("/api/admin/nuts/adjust", map[string]any{
		"user_id": userID,
		"amount":  amount,
		"reason":  reason,
	}, token)
}

// ── v2 APIs ──

// VersionCheckRequest is sent to /api/v2/version/check
type VersionCheckRequest struct {
	Version       string `json:"version"`
	VersionCode   int    `json:"version_code"`
	Platform      string `json:"platform"`
	ServerCode    string `json:"server_code,omitempty"`
	SignatureHash string `json:"signature_hash,omitempty"`
}

// VersionCheckResponse from /api/v2/version/check
type VersionCheckResponse struct {
	OK               bool   `json:"ok"`
	LatestVersion    string `json:"latest_version"`
	LatestVersionCode int   `json:"latest_version_code"`
	ForceUpdate      bool   `json:"force_update"`
	UpdateURL        string `json:"update_url"`
	UpdateMessage    string `json:"update_message"`
	SignatureValid   bool   `json:"signature_valid"`
	SignatureMessage string `json:"signature_message"`
}

// CheckVersion calls POST /api/v2/version/check
func CheckVersion(req VersionCheckRequest) (*VersionCheckResponse, error) {
	result, err := post("/api/v2/version/check", req, "")
	if err != nil {
		return nil, err
	}
	resp := &VersionCheckResponse{}
	if ok, _ := result["ok"].(bool); ok {
		resp.OK = true
		if v, ok := result["latest_version"].(string); ok {
			resp.LatestVersion = v
		}
		if v, ok := result["latest_version_code"].(float64); ok {
			resp.LatestVersionCode = int(v)
		}
		if v, ok := result["force_update"].(bool); ok {
			resp.ForceUpdate = v
		}
		if v, ok := result["update_url"].(string); ok {
			resp.UpdateURL = v
		}
		if v, ok := result["update_message"].(string); ok {
			resp.UpdateMessage = v
		}
		if v, ok := result["signature_valid"].(bool); ok {
			resp.SignatureValid = v
		}
		if v, ok := result["signature_message"].(string); ok {
			resp.SignatureMessage = v
		}
	}
	return resp, nil
}

// Announcement from /api/v2/announcements
type Announcement struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Content        string   `json:"content"`
	ContentType    string   `json:"content_type"`
	Severity       string   `json:"severity"`
	DisplayMode    string   `json:"display_mode"`
	CanDismiss     bool     `json:"can_dismiss"`
	TargetVersions []string `json:"target_versions"`
	TargetServers  []string `json:"target_servers"`
	CreatedAt      string   `json:"created_at"`
	ExpiresAt      string   `json:"expires_at"`
}

// AnnouncementsResponse from /api/v2/announcements
type AnnouncementsResponse struct {
	OK            bool           `json:"ok"`
	Announcements []Announcement `json:"announcements"`
}

// FetchAnnouncements calls POST /api/v2/announcements
func FetchAnnouncements(version string, versionCode int, serverCode, lastSeenID string) (*AnnouncementsResponse, error) {
	req := map[string]any{
		"version":      version,
		"version_code": versionCode,
	}
	if serverCode != "" {
		req["server_code"] = serverCode
	}
	if lastSeenID != "" {
		req["last_seen_id"] = lastSeenID
	}
	result, err := post("/api/v2/announcements", req, "")
	if err != nil {
		return nil, err
	}
	resp := &AnnouncementsResponse{}
	if ok, _ := result["ok"].(bool); ok {
		resp.OK = true
	}
	if arr, ok := result["announcements"].([]any); ok {
		for _, item := range arr {
			if m, ok2 := item.(map[string]any); ok2 {
				a := Announcement{}
				if v, ok3 := m["id"].(string); ok3 {
					a.ID = v
				}
				if v, ok3 := m["title"].(string); ok3 {
					a.Title = v
				}
				if v, ok3 := m["content"].(string); ok3 {
					a.Content = v
				}
				if v, ok3 := m["content_type"].(string); ok3 {
					a.ContentType = v
				}
				if v, ok3 := m["severity"].(string); ok3 {
					a.Severity = v
				}
				if v, ok3 := m["display_mode"].(string); ok3 {
					a.DisplayMode = v
				}
				if v, ok3 := m["can_dismiss"].(bool); ok3 {
					a.CanDismiss = v
				}
				if v, ok3 := m["created_at"].(string); ok3 {
					a.CreatedAt = v
				}
				if v, ok3 := m["expires_at"].(string); ok3 {
					a.ExpiresAt = v
				}
				resp.Announcements = append(resp.Announcements, a)
			}
		}
	}
	return resp, nil
}

// VerifySignature calls POST /api/v2/signature/verify
func VerifySignature(version string, versionCode int, signatureHash, packageName string) (map[string]any, error) {
	return post("/api/v2/signature/verify", map[string]any{
		"version":        version,
		"version_code":   versionCode,
		"signature_hash": signatureHash,
		"package_name":   packageName,
	}, "")
}
