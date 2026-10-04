package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bot-apk/config"
)

// ========== Data Structures ==========

type HeartbeatRequest struct {
	Token     string        `json:"token"`
	Version   string        `json:"version"`
	DeviceID  string        `json:"device_id"`
	Session   SessionInfo   `json:"session"`
	Increment IncrementData `json:"increment"`
}

type SessionInfo struct {
	Online      bool   `json:"online"`
	ServerCode  string `json:"server_code"`
	DeviceModel string `json:"device_model"`
	StartedAt   int64  `json:"started_at"`
}

type IncrementData struct {
	BlocksImported    int `json:"blocks_imported"`
	BuildingsImported int `json:"buildings_imported"`
	BuildingsExported int `json:"buildings_exported"`
	MapArtCompleted   int `json:"mapart_completed"`
	SkinCompleted     int `json:"skin_completed"`
	ImportSessions    int `json:"import_sessions"`
	ExportSessions    int `json:"export_sessions"`
}

type HeartbeatResponse struct {
	OK                bool          `json:"ok"`
	ServerTime        int64         `json:"server_time"`
	HeartbeatInterval int           `json:"heartbeat_interval"`
	PendingMessages   []PushMessage `json:"pending_messages"`
	Stats             UserStats     `json:"stats"`
}

type PushMessage struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Action     string `json:"action"`
	ActionData string `json:"action_data"`
	CreatedAt  string `json:"created_at"`
}

type UserStats struct {
	TotalBlocksImported    int `json:"total_blocks_imported"`
	TotalBuildingsImported int `json:"total_buildings_imported"`
	TotalBuildingsExported int `json:"total_buildings_exported"`
	TotalMapArtCompleted   int `json:"total_mapart_completed"`
	TotalSkinCompleted     int `json:"total_skin_completed"`
	TotalImportSessions    int `json:"total_import_sessions"`
	TotalExportSessions    int `json:"total_export_sessions"`
	TotalOnlineMinutes     int `json:"total_online_minutes"`
}

// ========== Increment Collector ==========

var (
	hbMu               sync.Mutex
	hbIncrement        IncrementData
	hbSessionStartedAt int64
	hbDeviceID         string
	hbStats            UserStats
	hbStatsMu          sync.RWMutex
)

func init() {
	hbSessionStartedAt = time.Now().Unix()
}

func recordHeartbeatIncrement(inc IncrementData) {
	hbMu.Lock()
	defer hbMu.Unlock()
	hbIncrement.BlocksImported += inc.BlocksImported
	hbIncrement.BuildingsImported += inc.BuildingsImported
	hbIncrement.BuildingsExported += inc.BuildingsExported
	hbIncrement.MapArtCompleted += inc.MapArtCompleted
	hbIncrement.SkinCompleted += inc.SkinCompleted
	hbIncrement.ImportSessions += inc.ImportSessions
	hbIncrement.ExportSessions += inc.ExportSessions
}

func resetHeartbeatIncrement() IncrementData {
	hbMu.Lock()
	defer hbMu.Unlock()
	inc := hbIncrement
	hbIncrement = IncrementData{}
	return inc
}

// ========== Device ID Persistence ==========

var deviceIDOnce sync.Once

func getDeviceID(dataDir string) string {
	deviceIDOnce.Do(func() {
		p := filepath.Join(dataDir, "device_id")
		data, err := os.ReadFile(p)
		if err == nil && len(data) > 0 {
			hbDeviceID = string(data)
			return
		}
		hbDeviceID = fmt.Sprintf("dev_%d_%d", time.Now().UnixNano(), os.Getpid())
		os.WriteFile(p, []byte(hbDeviceID), 0644)
	})
	return hbDeviceID
}

// ---------- HTTP Client ----------

var heartbeatHTTPClient = &http.Client{
	Timeout:   15 * time.Second,
	Transport: makeTransport(),
}

// ========== Send Heartbeat ==========

func sendHeartbeat(dataDir string) {
	cfg := config.ReloadConfig()

	// 匿名用户也用 device_id 上报
	token := ""
	if cfg != nil {
		token = cfg.Token
	}

	bm := getBotManager()
	online := bm != nil && bm.IsConnected()
	serverCode := ""
	if bm != nil {
		serverCode = bm.ServerCode()
	}

	inc := resetHeartbeatIncrement()

	req := HeartbeatRequest{
		Token:    token,
		Version:  appVersion,
		DeviceID: getDeviceID(dataDir),
		Session: SessionInfo{
			Online:      online,
			ServerCode:  serverCode,
			DeviceModel: getDeviceModel(),
			StartedAt:   hbSessionStartedAt,
		},
		Increment: inc,
	}

	baseURL := "https://prism.adblanlu.qzz.io"
	respData, err := postJSON(baseURL+"/api/v2/heartbeat", req)
	if err != nil {
		return
	}

	var resp HeartbeatResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		return
	}

	if !resp.OK {
		return
	}

	hbStatsMu.Lock()
	hbStats = resp.Stats
	hbStatsMu.Unlock()

	if len(resp.PendingMessages) > 0 {
		savePushMessages(dataDir, resp.PendingMessages)
	}
}

// ========== Helpers ==========

func postJSON(url string, v any) ([]byte, error) {
	data, _ := json.Marshal(v)
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := heartbeatHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// ========== Push Message Local Storage ==========

var pushMu sync.Mutex

func pushMessagesPath(dataDir string) string {
	return filepath.Join(dataDir, "push_messages.json")
}

func savePushMessages(dataDir string, msgs []PushMessage) {
	pushMu.Lock()
	defer pushMu.Unlock()

	existing := loadPushMessagesLocked(dataDir)
	existingMap := make(map[string]bool, len(existing))
	for _, m := range existing {
		existingMap[m.ID] = true
	}

	for _, m := range msgs {
		if !existingMap[m.ID] {
			existing = append(existing, m)
			existingMap[m.ID] = true
		}
	}

	data, _ := json.Marshal(existing)
	os.WriteFile(pushMessagesPath(dataDir), data, 0644)
}

func loadPushMessagesLocked(dataDir string) []PushMessage {
	data, err := os.ReadFile(pushMessagesPath(dataDir))
	if err != nil {
		return nil
	}
	var msgs []PushMessage
	json.Unmarshal(data, &msgs)
	return msgs
}

func loadPushMessages(dataDir string) []PushMessage {
	pushMu.Lock()
	defer pushMu.Unlock()
	return loadPushMessagesLocked(dataDir)
}

func ackPushMessages(dataDir string, ids []string) {
	pushMu.Lock()
	defer pushMu.Unlock()

	existing := loadPushMessagesLocked(dataDir)
	keep := make([]PushMessage, 0, len(existing))
	delMap := make(map[string]bool, len(ids))
	for _, id := range ids {
		delMap[id] = true
	}
	for _, m := range existing {
		if !delMap[m.ID] {
			keep = append(keep, m)
		}
	}
	data, _ := json.Marshal(keep)
	os.WriteFile(pushMessagesPath(dataDir), data, 0644)
}

// ========== Heartbeat Loop ==========

var heartbeatDataDir string

func startHeartbeatLoop(dataDir string) {
	heartbeatDataDir = dataDir
	sendHeartbeat(dataDir)
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for range ticker.C {
			sendHeartbeat(dataDir)
		}
	}()

	}

// ========== HTTP Endpoints ==========

func handlePushPending(w http.ResponseWriter, r *http.Request) {
	msgs := loadPushMessages(heartbeatDataDir)
	writeJSON(w, map[string]any{
		"ok":       true,
		"messages": msgs,
	})
}

func handlePushAck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	ackPushMessages(heartbeatDataDir, req.IDs)

	// 同时告知服务端已读，避免下次心跳重复推送
	cfg := config.ReloadConfig()
	if cfg != nil && cfg.Token != "" && len(req.IDs) > 0 {
		go func() {
			body, _ := json.Marshal(map[string]any{
				"token": cfg.Token,
				"ids":   req.IDs,
			})
			http.Post("https://prism.adblanlu.qzz.io/api/v2/heartbeat/ack",
				"application/json", bytes.NewReader(body))
		}()
	}

	writeJSON(w, map[string]any{"ok": true})
}