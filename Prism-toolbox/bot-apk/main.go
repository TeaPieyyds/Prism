package main

import (
	"context"
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"bot-apk/config"
	"bot-apk/media"
	"bot-apk/prism"
	"bot-apk/state"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
)

//go:embed frontend/**
var frontendFS embed.FS

// ========== SSE Hub ==========

type SSEHub struct {
	mu      sync.RWMutex
	clients map[chan string]struct{}
}

func NewSSEHub() *SSEHub { return &SSEHub{clients: map[chan string]struct{}{}} }

func (h *SSEHub) Add(ch chan string) {
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
}

func (h *SSEHub) Remove(ch chan string) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

func (h *SSEHub) Emit(event, data string) {
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

// ========== HTTP Transport ==========

var customDNS = "223.5.5.5" // 默认阿里 DNS（中国大陆友好），可通过配置覆盖
var tlsSkipVerify = true    // 是否跳过 TLS 证书验证，可通过配置关闭

func makeTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: tlsSkipVerify},
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
			Resolver: &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					dns := customDNS
					if dns == "" {
						dns = "223.5.5.5"
					}
					return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "udp", dns+":53")
				},
			},
		}).DialContext,
	}
}

// ========== JSON helpers ==========

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// setSuppressGameLogs 在导入等任务运行期间设置/清除"抑制游戏内日志"标志。
// 抑制期间，游戏服务器发来的聊天/加入/退出等消息不会转发到终端，避免干扰。
func setSuppressGameLogs(v bool) {
	suppressGameLogsMu.Lock()
	suppressGameLogs = v
	suppressGameLogsMu.Unlock()
}

func isSuppressingGameLogs() bool {
	suppressGameLogsMu.RLock()
	defer suppressGameLogsMu.RUnlock()
	return suppressGameLogs
}

// isGameInternalLog 判断消息是否为游戏内发来的内容（聊天/加入/退出/重生等）。
// 这些消息在任务运行期间对用户无意义，应过滤不显示到终端。
func isGameInternalLog(msg string) bool {
	switch {
	case strings.HasPrefix(msg, "[聊天]"),
		strings.HasPrefix(msg, "[加入]"),
		strings.HasPrefix(msg, "[退出]"),
		strings.HasPrefix(msg, "[重生]"):
		return true
	}
	return false
}

// ========== Globals ==========

var (
	hub                 *SSEHub
	activeTask          *state.TaskController
	taskMu              sync.Mutex
	updateBlocked       bool
	updateBlockedMu     sync.RWMutex
	suppressGameLogs    bool
	suppressGameLogsMu  sync.RWMutex
	handleSystemRestart func(w http.ResponseWriter, r *http.Request)
	connectStopCh       chan struct{} // closed to cancel a pending connection goroutine
	connectStopMu       sync.Mutex
	mapPlayer           *MapPlayer
	mapPlayerMu         sync.Mutex
	lastPrismToken      string
	lastPrismURL        string

	// 工具箱信息缓存
	toolboxInfo     *prism.ToolboxInfo
	toolboxInfoMu   sync.RWMutex
	toolboxInfoTime time.Time
)

func recoverCheckpoints() {
	tasks, err := state.ListCheckpoints()
	if err != nil {
		return
	}
	for _, t := range tasks {
		if t.Status == "running" {
			t.Status = "paused"
			t.Message = "应用上次退出时未完成，已暂停"
			state.SaveCheckpointNow(t)
		}
	}
}

// ========== Handlers ==========

func handleStatic(prefix, dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, prefix)
		if name == "" || strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}
		data, err := frontendFS.ReadFile(dir + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		ct := "application/octet-stream"
		if strings.HasSuffix(name, ".css") {
			ct = "text/css; charset=utf-8"
		} else if strings.HasSuffix(name, ".js") {
			ct = "application/javascript; charset=utf-8"
		} else if strings.HasSuffix(name, ".wav") {
			ct = "audio/wav"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(data)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	data, _ := frontendFS.ReadFile("frontend/index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Write(data)
}

func handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 50)
	hub.Add(ch)
	defer hub.Remove(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			fmt.Fprint(w, msg)
			flusher.Flush()
		}
	}
}

func handleTaskStart(w http.ResponseWriter, r *http.Request) {
	updateBlockedMu.RLock()
	blocked := updateBlocked
	updateBlockedMu.RUnlock()
	if blocked {
		writeJSON(w, map[string]any{"ok": false, "error": "检测到新版本，请更新后再使用导入/导出功能"})
		return
	}
	taskMu.Lock()
	defer taskMu.Unlock()
	if activeTask != nil && activeTask.Running() {
		writeJSON(w, map[string]any{"ok": false, "error": "已有任务运行中"})
		return
	}
	var req struct {
		Type   string          `json:"type"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}

	// 检查机器人连接
	botMgrMu.Lock()
	connected := botMgr != nil && botMgr.IsConnected()
	botMgrMu.Unlock()
	if !connected {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器，请先连接"})
		return
	}

	tc := state.NewTaskController(hub)
	activeTask = tc

	go func() {
		defer func() {
			if r := recover(); r != nil {
				hub.Emit("task_error", mustJSON(map[string]any{"error": fmt.Sprintf("任务异常崩溃: %v", r)}))
			}
			taskMu.Lock()
			if activeTask == tc {
				activeTask = nil
			}
			taskMu.Unlock()
		}()
		dispatchTask(tc, req.Type, req.Params)
	}()
	writeJSON(w, map[string]any{"ok": true})
}

func handleRepairStart(w http.ResponseWriter, r *http.Request) {
	updateBlockedMu.RLock()
	blocked := updateBlocked
	updateBlockedMu.RUnlock()
	if blocked {
		writeJSON(w, map[string]any{"ok": false, "error": "检测到新版本，请更新后再使用修复功能"})
		return
	}
	taskMu.Lock()
	defer taskMu.Unlock()
	if activeTask != nil && activeTask.Running() {
		writeJSON(w, map[string]any{"ok": false, "error": "已有任务运行中"})
		return
	}

	var req struct {
		Path               string `json:"path"`
		X                  int    `json:"x"`
		Y                  int    `json:"y"`
		Z                  int    `json:"z"`
		Speed              int    `json:"speed,omitempty"`
		Rotation           int    `json:"rotation,omitempty"`
		RepairCircle       bool   `json:"repair_circle,omitempty"`
		RepairNBTOnly      bool   `json:"repair_nbt_only,omitempty"`
		RepairCX           int    `json:"repair_cx,omitempty"`
		RepairCZ           int    `json:"repair_cz,omitempty"`
		RepairRadius       int    `json:"repair_radius,omitempty"`
		RepairX1           int    `json:"repair_x1,omitempty"`
		RepairY1           int    `json:"repair_y1,omitempty"`
		RepairZ1           int    `json:"repair_z1,omitempty"`
		RepairX2           int    `json:"repair_x2,omitempty"`
		RepairY2           int    `json:"repair_y2,omitempty"`
		RepairZ2           int    `json:"repair_z2,omitempty"`
		Dimension          string `json:"dimension,omitempty"`
		ImportCommands     bool   `json:"import_commands,omitempty"`
		CmdDisabled        bool   `json:"cmd_disabled,omitempty"`
		ExcludeWater       bool   `json:"exclude_water,omitempty"`
		ExcludeWaterlogged bool   `json:"exclude_waterlogged,omitempty"`
		ExcludeLava        bool   `json:"exclude_lava,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}

	// 参数验证
	if req.Path == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "缺少文件路径"})
		return
	}
	if req.X == 0 && req.Y == 0 && req.Z == 0 {
		writeJSON(w, map[string]any{"ok": false, "error": "修补模式必须提供起始坐标 (x, y, z)"})
		return
	}

	// 检查机器人连接
	botMgrMu.Lock()
	connected := botMgr != nil && botMgr.IsConnected()
	botMgrMu.Unlock()
	if !connected {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器，请先连接"})
		return
	}

	tc := state.NewTaskController(hub)
	activeTask = tc

	params := map[string]any{
		"path":                req.Path,
		"x":                   req.X,
		"y":                   req.Y,
		"z":                   req.Z,
		"speed":               req.Speed,
		"rotation":            req.Rotation,
		"repair_circle":       req.RepairCircle,
		"repair_nbt_only":     req.RepairNBTOnly,
		"repair_cx":           req.RepairCX,
		"repair_cz":           req.RepairCZ,
		"repair_radius":       req.RepairRadius,
		"repair_x1":           req.RepairX1,
		"repair_y1":           req.RepairY1,
		"repair_z1":           req.RepairZ1,
		"repair_x2":           req.RepairX2,
		"repair_y2":           req.RepairY2,
		"repair_z2":           req.RepairZ2,
		"dimension":           req.Dimension,
		"import_commands":     req.ImportCommands,
		"cmd_disabled":        req.CmdDisabled,
		"exclude_water":       req.ExcludeWater,
		"exclude_waterlogged": req.ExcludeWaterlogged,
		"exclude_lava":        req.ExcludeLava,
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				hub.Emit("task_error", mustJSON(map[string]any{"error": fmt.Sprintf("修补任务异常崩溃: %v", r)}))
			}
			taskMu.Lock()
			if activeTask == tc {
				activeTask = nil
			}
			taskMu.Unlock()
		}()
		runRepairTask(tc, params)
	}()

	writeJSON(w, map[string]any{"ok": true})
}

func handleTaskStop(w http.ResponseWriter, r *http.Request) {
	taskMu.Lock()
	defer taskMu.Unlock()
	if activeTask != nil {
		activeTask.Stop()
		activeTask = nil
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleTaskList(w http.ResponseWriter, r *http.Request) {
	botMgrMu.Lock()
	serverCode := ""
	if botMgr != nil {
		serverCode = botMgr.ServerCode()
	}
	botMgrMu.Unlock()

	tasks, err := state.ListCheckpointsByServer(serverCode)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "tasks": tasks})
}

func handleTaskResume(w http.ResponseWriter, r *http.Request) {
	updateBlockedMu.RLock()
	blocked := updateBlocked
	updateBlockedMu.RUnlock()
	if blocked {
		writeJSON(w, map[string]any{"ok": false, "error": "检测到新版本，请更新后再使用导入/导出功能"})
		return
	}
	taskMu.Lock()
	defer taskMu.Unlock()
	if activeTask != nil && activeTask.Running() {
		writeJSON(w, map[string]any{"ok": false, "error": "已有任务运行中"})
		return
	}
	// 检查机器人连接
	botMgrMu.Lock()
	connected := botMgr != nil && botMgr.IsConnected()
	currentServer := ""
	if botMgr != nil {
		currentServer = botMgr.ServerCode()
	}
	botMgrMu.Unlock()
	if !connected {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器，请先连接"})
		return
	}
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	cp, err := state.LoadCheckpoint(req.TaskID)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// 验证断点绑定的服务器
	if cp.ServerCode != "" && currentServer != "" && cp.ServerCode != currentServer {
		writeJSON(w, map[string]any{"ok": false, "error": fmt.Sprintf("该断点绑定服务器 %s，当前在 %s，无法跨服恢复", cp.ServerCode, currentServer)})
		return
	}
	// 恢复时用 Params(原始任务参数) + Data(断点位置)
	tc := state.NewTaskController(hub)
	activeTask = tc
	go func() {
		defer func() {
			if r := recover(); r != nil {
				hub.Emit("task_error", mustJSON(map[string]any{"error": fmt.Sprintf("任务异常崩溃: %v", r)}))
			}
			taskMu.Lock()
			if activeTask == tc {
				activeTask = nil
			}
			taskMu.Unlock()
		}()
		dispatchTaskWithResume(tc, cp.Type, cp.Params, req.TaskID)
	}()
	writeJSON(w, map[string]any{"ok": true})
}

func handleTaskDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := state.DeleteCheckpoint(req.TaskID); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		cfg := config.ReloadConfig()
		writeJSON(w, map[string]any{"ok": true, "config": cfg})
	case "POST":
		var cfg config.AppConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
			return
		}
		if err := config.SaveConfig(&cfg); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		// 保存配置后刷新工具箱信息（异步）
		if cfg.Token != "" {
			go refreshToolboxInfo()
		} else {
			toolboxInfoMu.Lock()
			toolboxInfo = nil
			toolboxInfoMu.Unlock()
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		writeJSON(w, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func handlePrismLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	result, err := prism.PrismLogin(req.Email, req.Password)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "result": result})
}

func handleBotConnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token          string `json:"token"`
		Server         string `json:"server"`
		Auth           string `json:"auth"`
		Password       string `json:"password"`
		UseNewProtocol bool   `json:"use_new_protocol"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	if req.Auth == "" {
		req.Auth = "https://prism.adblanlu.qzz.io"
	}

	// 取消上一次连接
	connectStopMu.Lock()
	if connectStopCh != nil {
		close(connectStopCh)
	}
	connectStopCh = nil
	connectStopMu.Unlock()

	botMgrMu.Lock()
	if botMgr != nil {
		botMgr.Disconnect()
	}
	bm := NewBotManager()
	bm.onReady = func() {
		hub.Emit("conn_ready", "{}")
		if bm.conn != nil {
			bm.SendTellraw(fmt.Sprintf("§a§l%s 已准备就绪§r", bm.conn.IdentityData().DisplayName))
		}
	}
	botMgr = bm
	botMgrMu.Unlock()

	// 异步连接，POST 立即返回，前端通过 SSE + 轮询跟踪进度
	// 根据开关设置协议版本
	if req.UseNewProtocol {
		protocol.CurrentProtocol = 860
		protocol.CurrentVersion = "1.21.120"
	} else {
		protocol.CurrentProtocol = 819
		protocol.CurrentVersion = "1.21.90"
	}

	token, server, auth, password := req.Token, req.Server, req.Auth, req.Password
	lastPrismToken = token
	lastPrismURL = auth
	// 连接前刷新工具箱信息
	go refreshToolboxInfo()
	go func() {
		go func(m *BotManager) {
			// 防御：日志转发器绝不能死。若某条消息处理 panic 退出，logCh 会被填满，
			// 导致机器人里所有 bm.logCh <- 永久阻塞 → 机器人卡死、日志通道停。
			// 因此每条消息都在 recover 的保护下处理，panic 只跳过该条，转发器永不死亡。
			for msg := range m.logCh {
				func() {
					defer func() { recover() }()
					// 任务运行期间过滤游戏内消息（聊天/加入/退出等），避免大量提示冲击终端
					if isSuppressingGameLogs() && isGameInternalLog(msg) {
						return
					}
					hub.Emit("bot_log", mustJSON(map[string]any{"msg": msg}))
				}()
			}
		}(bm)
		if err := bm.Connect(token, server, auth, password); err != nil {
			hub.Emit("bot_log", mustJSON(map[string]any{"msg": fmt.Sprintf("✗ 连接失败(已重试3次): %v", err)}))
		}
	}()

	writeJSON(w, map[string]any{"ok": true})
}

func handleBotDisconnect(w http.ResponseWriter, r *http.Request) {
	go func() {
		botMgrMu.Lock()
		if botMgr != nil {
			botMgr.Disconnect()
			botMgr = nil
		}
		botMgrMu.Unlock()
	}()
	writeJSON(w, map[string]any{"ok": true})
}

func handleBotStatus(w http.ResponseWriter, r *http.Request) {
	botMgrMu.Lock()
	defer botMgrMu.Unlock()
	isOP := false
	serverCode := ""
	if botMgr != nil {
		isOP = botMgr.IsOP()
		serverCode = botMgr.ServerCode()
	}
	writeJSON(w, map[string]any{
		"ok":        true,
		"connected": botMgr != nil && botMgr.IsConnected(),
		"is_op":     isOP,
		"server":    serverCode,
	})
}

func handleFleetConnect(w http.ResponseWriter, r *http.Request) {
	cfg := config.ReloadConfig()
	if len(cfg.BotTokens) == 0 {
		writeJSON(w, map[string]any{"ok": false, "error": "未配置子令牌，请在认证信息中通过加号添加"})
		return
	}
	if botMgr == nil || !botMgr.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "主机器人未连接，请先连接主机器人"})
		return
	}
	go FleetConnect(cfg)
	writeJSON(w, map[string]any{"ok": true})
}

func handleFleetDisconnect(w http.ResponseWriter, r *http.Request) {
	go FleetDisconnect()
	writeJSON(w, map[string]any{"ok": true})
}

func handleFleetStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"ok":     true,
		"status": FleetStatus(),
		"main_connected": botMgr != nil && botMgr.IsConnected(),
		"main_op": botMgr != nil && botMgr.IsOP(),
	})
}

func handleBotConsole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	if req.Input == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "输入为空"})
		return
	}
	// 只取 botMgr 指针，不持有锁（SendConsole 可能等待命令响应）
	botMgrMu.Lock()
	bm := botMgr
	botMgrMu.Unlock()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	result, err := bm.SendConsole(req.Input)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, result)
}

// ========== 飞行控制 API ==========

func getBotManagerLocked() *BotManager {
	botMgrMu.Lock()
	defer botMgrMu.Unlock()
	return botMgr
}

func handleFlyStatus(w http.ResponseWriter, r *http.Request) {
	botMgrMu.Lock()
	bm := botMgr
	botMgrMu.Unlock()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	status := bm.GetFlyStatus()
	status["ok"] = true
	writeJSON(w, status)
}

func handleFlyStart(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	if err := bm.StartFlying(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleFlyStop(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	if err := bm.StopFlying(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleFlyJump(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	if err := bm.Jump(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleFlyJumpStart(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	if err := bm.JumpStart(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleFlyJumpStop(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	if err := bm.JumpStop(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleFlySneakStart(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	if err := bm.SneakStart(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleFlySneakStop(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	if err := bm.SneakStop(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleFlySprintStart(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	if err := bm.SprintStart(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleFlySprintStop(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	if err := bm.SprintStop(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleFlyDown(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	if err := bm.Down(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleFlyLook(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	var req struct {
		Pitch float32 `json:"pitch"`
		Yaw   float32 `json:"yaw"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	if err := bm.SetRotation(req.Pitch, req.Yaw); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleFlyMove(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	var req struct {
		Dx float32 `json:"dx"`
		Dy float32 `json:"dy"`
		Dz float32 `json:"dz"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	if err := bm.MoveRelative(req.Dx, req.Dy, req.Dz); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleWorldEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持POST"})
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	globalWorld.SetEnabled(req.Enabled)
	writeJSON(w, map[string]any{"ok": true, "enabled": req.Enabled})
}

func handleHeartbeatEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持POST"})
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	bm := getBotManagerLocked()
	if bm != nil {
		bm.SetHeartbeatEnabled(req.Enabled)
	}
	writeJSON(w, map[string]any{"ok": true, "enabled": req.Enabled})
}

func handleFlyTeleport(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	var req struct {
		X float32 `json:"x"`
		Y float32 `json:"y"`
		Z float32 `json:"z"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	if err := bm.TeleportTo(req.X, req.Y, req.Z); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleFlyPress(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	var req struct {
		Direction string `json:"direction"` // forward/backward/left/right/up/down
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	var dx, dy, dz float32
	switch req.Direction {
	case "forward":
		dx, dz = 0, -1
	case "backward":
		dx, dz = 0, 1
	case "left":
		dx, dz = -1, 0
	case "right":
		dx, dz = 1, 0
	case "up":
		dy = 1
	case "down":
		dy = -1
	default:
		writeJSON(w, map[string]any{"ok": false, "error": "未知方向"})
		return
	}
	if err := bm.StartMoveDirection(dx, dy, dz); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleFlyRelease(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	if err := bm.StopMovement(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleFlyPosition(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	result, err := bm.QueryPosition()
	if err == nil {
		result["ok"] = true
		writeJSON(w, result)
		return
	}
	pos := bm.GetPosition()
	pos["ok"] = true
	pos["note"] = "local"
	writeJSON(w, pos)
}

func handlePathfinderGoto(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	var req struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	if globalPF == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "寻路器未初始化"})
		return
	}
	if globalPF.IsActive() {
		globalPF.Stop()
		time.Sleep(200 * time.Millisecond)
	}
	goal := &GoalBlock{X: req.X, Y: req.Y, Z: req.Z}
	if err := globalPF.Goto(goal); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handlePathfinderFollow(w http.ResponseWriter, r *http.Request) {
	bm := getBotManagerLocked()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	var req struct {
		Player string `json:"player"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	if req.Player == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "缺少玩家名"})
		return
	}
	if globalPF == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "寻路器未初始化"})
		return
	}
	if globalPF.IsActive() {
		globalPF.Stop()
		time.Sleep(200 * time.Millisecond)
	}
	pi := bm.GetPlayer(req.Player)
	if pi == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "未找到玩家"})
		return
	}
	goal := &GoalNear{X: float64(pi.Position[0]), Y: float64(pi.Position[1]), Z: float64(pi.Position[2]), Range: 2}
	if err := globalPF.Goto(goal); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handlePathfinderStop(w http.ResponseWriter, r *http.Request) {
	if globalPF != nil {
		globalPF.Stop()
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handlePathfinderLog(w http.ResponseWriter, r *http.Request) {
	f, err := os.OpenFile("/storage/emulated/0/Download/pathfinder.log", os.O_RDONLY, 0644)
	if err != nil {
		f, err = os.OpenFile("/tmp/pathfinder.log", os.O_RDONLY, 0644)
	}
	if err != nil {
		w.Write([]byte("(无日志文件)"))
		return
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

func handleMapArtPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path        string `json:"path"`
		TargetW     int    `json:"targetW"`
		TargetH     int    `json:"targetH"`
		Orientation string `json:"orientation"`
		UseRelief   bool   `json:"use_relief"`
		ColorSpace  string `json:"color_space"`
		Dither      string `json:"dither"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	if req.TargetW < 128 {
		req.TargetW = 128
	}
	if req.TargetH < 128 {
		req.TargetH = 128
	}
	if req.TargetW > 1024 {
		req.TargetW = 1024
	}
	if req.TargetH > 1024 {
		req.TargetH = 1024
	}

	srcFile, err := os.Open(req.Path)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer srcFile.Close()
	srcImg, _, err := image.Decode(srcFile)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "图片解码失败"})
		return
	}

	tW, tH := req.TargetW, req.TargetH

	// 解析色彩空间和抖动算法
	cs := parseColorSpace(req.ColorSpace)
	dither := parseDitherAlgo(req.Dither)

	// 使用新引擎处理（含抖动和色彩空间匹配）
	grid := media.ProcessMapArt(srcImg, tW, tH, cs, dither, media.Rotation0)

	var points []map[string]any
	for y := 0; y < tH; y++ {
		for x := 0; x < tW; x++ {
			block := grid[y][x]
			if block.Name == "minecraft:air" {
				continue
			}
			var py int
			if req.UseRelief && req.Orientation == "horizontal" {
				py = block.HeightOff + 1
			}
			col := media.BlockHexColor(block.Name)
			id := block.Name
			if req.Orientation == "horizontal" {
				points = append(points, map[string]any{"x": x, "y": py, "z": y, "c": col, "id": id})
			} else {
				points = append(points, map[string]any{"x": 0, "y": y, "z": x, "c": col, "id": id})
			}
		}
	}

	var sx, sy2, sz int
	if req.Orientation == "horizontal" {
		sx, sy2, sz = tW, 1, tH
		if req.UseRelief {
			sy2 = 3
		}
	} else {
		sx, sy2, sz = 1, tH, tW
	}
	voxel := map[string]any{
		"points": points, "size_x": sx, "size_y": sy2, "size_z": sz,
		"step": 1, "too_large": false, "total_solid": len(points),
	}
	writeJSON(w, map[string]any{"ok": true, "voxel": voxel, "blocks": len(points)})
}

// parseColorSpace 解析前端传入的色彩空间字符串
func parseColorSpace(s string) media.ColorSpace {
	switch s {
	case "rgb":
		return media.ColorSpaceRgb
	case "hsv":
		return media.ColorSpaceHsv
	default:
		return media.ColorSpaceLab
	}
}

// parseDitherAlgo 解析前端传入的抖动算法字符串
func parseDitherAlgo(s string) media.DitherAlgo {
	switch s {
	case "floyd_steinberg":
		return media.DitherFloydSteinberg
	case "atkinson":
		return media.DitherAtkinson
	case "burkes":
		return media.DitherBurkes
	case "stucki":
		return media.DitherStucki
	case "jarvis":
		return media.DitherJarvisJudiceNinke
	case "bayer_2x2":
		return media.DitherBayer2x2
	case "bayer_4x4":
		return media.DitherBayer4x4
	case "bayer_8x8":
		return media.DitherBayer8x8
	case "ordered_3x3":
		return media.DitherOrdered3x3
	default:
		return media.DitherNone
	}
}

func handleSounds(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/sounds/")
	data, err := frontendFS.ReadFile("frontend/sounds/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

func handleUploads(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/uploads/")
	// Search multiple locations
	var data []byte
	var err error
	for _, dir := range []string{"/tmp/td_uploads/", "/data/data/com.prismtool.box/files/"} {
		data, err = os.ReadFile(dir + name)
		if err == nil {
			break
		}
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ct := "application/octet-stream"
	if strings.HasSuffix(name, ".png") {
		ct = "image/png"
	}
	if strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") {
		ct = "image/jpeg"
	}
	w.Header().Set("Content-Type", ct)
	w.Write(data)
}

func handleMediaPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	data, err := os.ReadFile(req.Path)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	seq, err := media.ParseMidiSeq(data)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	type note struct {
		Inst  string  `json:"inst"`
		Vol   float64 `json:"vol"`
		Pitch float64 `json:"pitch"`
		Delay float64 `json:"delay"`
	}
	notes := make([]note, len(seq.Notes))
	for i, n := range seq.Notes {
		notes[i] = note{n.Instrument, n.Volume, n.Pitch, n.Delay}
	}
	writeJSON(w, map[string]any{"ok": true, "notes": notes})
}

// 实体方块超过此数量时，分析阶段不自动生成 3D 预览（由前端按需触发）
const voxelDeferredThreshold = 20000

func handleBuildingAnalyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
		X1   int    `json:"x1"`
		Y1   int    `json:"y1"`
		Z1   int    `json:"z1"`
		X2   int    `json:"x2"`
		Y2   int    `json:"y2"`
		Z2   int    `json:"z2"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}

	region := [6]int{req.X1, req.Y1, req.Z1, req.X2, req.Y2, req.Z2}
	// 新分析前先清旧缓存，让旧数据立即成为垃圾，配合后台 GC 回收内存
	clearBuildingCache()
	data, err := loadStructureFileCached(req.Path, region)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	stats := data.ComputeStats()

	// 解析完成后，后台回收内存（不阻塞响应）。大文件解析会产生大量临时内存，
	// Go 的 GC 默认延迟触发且不归还系统，需要主动回收，否则内存逐次累积。
	go func() {
		runtime.GC()
		debug.FreeOSMemory()
	}()

	// 大文件（实体方块 > 2 万）不自动生成 3D 预览，由前端提示用户按需生成，
	// 避免分析阶段卡顿和内存占用。前端渲染能力不受影响。
	if stats.SolidBlocks > voxelDeferredThreshold {
		writeJSON(w, map[string]any{"ok": true, "stats": stats, "voxel": nil, "voxel_deferred": true})
		return
	}
	voxel := data.GenerateVoxelData()

	writeJSON(w, map[string]any{"ok": true, "stats": stats, "voxel": voxel})
}

func handleFileUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(100 << 20) // 100MB max
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "读取文件失败"})
		return
	}
	defer file.Close()

	// 优先使用 output_dir 参数
	outputDir := r.FormValue("output_dir")
	filename := r.FormValue("filename")
	if filename == "" {
		filename = header.Filename
	}

	var dstPath string
	if outputDir != "" {
		os.MkdirAll(outputDir, 0755)
		dstPath = outputDir + "/" + filename
	} else {
		// 尝试多个目录，Android 上 /tmp 可能不可写
		testDirs := []string{"/data/data/com.prismtool.box/files", "/tmp/td_uploads"}
		var uploadDir string
		for _, d := range testDirs {
			if os.MkdirAll(d, 0755) == nil {
				uploadDir = d
				break
			}
		}
		if uploadDir == "" {
			uploadDir = "/tmp/td_uploads"
		}
		os.MkdirAll(uploadDir, 0755)
		dstPath = uploadDir + "/" + filename
	}
	dst, err := os.Create(dstPath)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "创建文件失败"})
		return
	}
	defer dst.Close()
	io.Copy(dst, file)
	writeJSON(w, map[string]any{"ok": true, "path": dstPath})
}

func handleBuildingStats(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	data, err := loadStructureFileCached(req.Path)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	stats := data.ComputeStats()
	writeJSON(w, map[string]any{"ok": true, "stats": stats})
}

func handleBuildingVoxel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	data, err := loadStructureFileCached(req.Path)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	voxel := data.GenerateVoxelData()
	go func() {
		runtime.GC()
		debug.FreeOSMemory()
	}()
	writeJSON(w, map[string]any{"ok": true, "voxel": voxel})
}

func handlePrismNuts(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	token = strings.TrimPrefix(token, "Bearer ")
	switch r.Method {
	case "GET":
		result, err := prism.PrismGetNuts(r.URL.Query().Get("limit"), r.URL.Query().Get("offset"), token)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "result": result})
	case "POST":
		var req struct {
			UserID int    `json:"user_id"`
			Amount int    `json:"amount"`
			Reason string `json:"reason"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		result, err := prism.PrismAdjustNuts(req.UserID, req.Amount, req.Reason, token)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "result": result})
	}
}

// handlePrismProxy forwards API requests to the prism auth server using the stored token.
func handlePrismProxy(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/api/prism-proxy")
	cfg := config.ReloadConfig()
	target := cfg.AuthURL + p
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	body, _ := io.ReadAll(r.Body)
	req, err := http.NewRequest(r.Method, target, strings.NewReader(string(body)))
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// ========== Music handlers ==========

// cached MIDI parse results (path → parsed notes)
// 有界缓存，防止上传大量 MIDI 文件导致 OOM
const maxMidiCacheEntries = 50

var (
	midiCache      = map[string]*media.MIDIFile{}
	midiCacheOrder = make([]string, 0, maxMidiCacheEntries) // 插入顺序，用于 FIFO 淘汰
	midiCacheMu    sync.Mutex
)

// midiCacheSet 写入缓存，超限时淘汰最早插入的条目。
func midiCacheSet(key string, midi *media.MIDIFile) {
	midiCacheMu.Lock()
	defer midiCacheMu.Unlock()

	// 如果 key 已存在，先移除旧顺序记录
	if _, exists := midiCache[key]; exists {
		removeOrderEntry(key)
	} else if len(midiCacheOrder) >= maxMidiCacheEntries {
		// 淘汰最早插入的条目
		oldest := midiCacheOrder[0]
		midiCacheOrder = midiCacheOrder[1:]
		delete(midiCache, oldest)
	}

	midiCache[key] = midi
	midiCacheOrder = append(midiCacheOrder, key)
}

// midiCacheGet 从缓存读取，返回 nil 表示不存在。
func midiCacheGet(key string) *media.MIDIFile {
	midiCacheMu.Lock()
	defer midiCacheMu.Unlock()
	return midiCache[key]
}

func removeOrderEntry(key string) {
	for i, k := range midiCacheOrder {
		if k == key {
			midiCacheOrder = append(midiCacheOrder[:i], midiCacheOrder[i+1:]...)
			return
		}
	}
}

func handleMusicUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(50 << 20)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "读取文件失败"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "读取失败"})
		return
	}

	midi, err := media.ParseMIDIFile(data)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "MIDI解析失败: " + err.Error()})
		return
	}

	// Cache by filename (bounded to maxMidiCacheEntries)
	midiCacheSet(header.Filename, midi)

	preview := midi.ToPreviewNotes()
	totalMs := 0
	for _, n := range preview {
		totalMs += n.DelayMs
	}
	duration := fmt.Sprintf("%d:%02d", totalMs/60000, (totalMs%60000)/1000)

	// Count instruments
	instCount := map[string]int{}
	for _, n := range midi.Notes {
		instCount[n.Sound]++
	}
	instruments := make([]map[string]any, 0)
	for k, v := range instCount {
		instruments = append(instruments, map[string]any{"name": k, "count": v})
	}

	writeJSON(w, map[string]any{
		"ok":          true,
		"notes":       preview,
		"total_notes": len(preview),
		"duration":    duration,
		"instruments": instruments,
		"cache_key":   header.Filename,
	})
}

func handleMusicPlay(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CacheKey   string  `json:"cache_key"`
		Target     string  `json:"target"`
		Speed      float64 `json:"speed"`
		QueueIndex *int    `json:"queue_index,omitempty"` // non-nil 表示从队列播放
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}

	// 队列播放模式（前端明确传递了 queue_index 才进入）
	if req.QueueIndex != nil {
		playlistManagerMu.Lock()
		pm := playlistManager
		playlistManagerMu.Unlock()
		if pm == nil {
			writeJSON(w, map[string]any{"ok": false, "error": "队列管理器未初始化"})
			return
		}
		if req.Target != "" {
			pm.mu.Lock()
			pm.target = req.Target
			pm.mu.Unlock()
		}
		if req.Speed > 0 {
			pm.mu.Lock()
			pm.speed = req.Speed
			pm.mu.Unlock()
		}
		if err := pm.Play(*req.QueueIndex); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "queue_play": true})
		return
	}

	// 传统单曲播放
	midi := midiCacheGet(req.CacheKey)
	if midi == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "MIDI未上传或已过期"})
		return
	}

	if req.Target == "" {
		req.Target = "@a"
	}
	if req.Speed <= 0 {
		req.Speed = 1.0
	}

	botMgrMu.Lock()
	connected := botMgr != nil && botMgr.IsConnected()
	botMgrMu.Unlock()
	if !connected {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}

	// 使用通用转换函数
	notes := convertMIDIToMusicNotes(midi)
	totalMs := 0
	for _, n := range midi.Notes {
		d := int(n.Tick) * 50
		if d < 1 {
			d = 1
		}
		totalMs += d
	}
	title := strings.TrimSuffix(req.CacheKey, ".mid")
	title = strings.TrimSuffix(title, ".midi")

	go startMusicPlayback(notes, req.Target, req.Speed, title, totalMs, nil)
	writeJSON(w, map[string]any{"ok": true})
}

func handleMusicStop(w http.ResponseWriter, r *http.Request) {
	// 捕获指针后立即释放全局锁，避免 Stop() 内部 SSE 广播时持有锁
	playlistManagerMu.Lock()
	pm := playlistManager
	playlistManagerMu.Unlock()

	if pm != nil && pm.IsActive() {
		pm.Stop()
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	// 传统单曲播放：仅停止播放器
	stopMusicPlayback()
	writeJSON(w, map[string]any{"ok": true})
}

func handleMusicPause(w http.ResponseWriter, r *http.Request) {
	musicPlayerMu.Lock()
	defer musicPlayerMu.Unlock()
	if musicPlayer == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "没有正在播放的音乐"})
		return
	}
	musicPlayer.Pause()
	writeJSON(w, map[string]any{"ok": true})
}

func handleMusicResume(w http.ResponseWriter, r *http.Request) {
	musicPlayerMu.Lock()
	defer musicPlayerMu.Unlock()
	if musicPlayer == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "没有暂停的音乐"})
		return
	}
	musicPlayer.Play()
	writeJSON(w, map[string]any{"ok": true})
}

func handleMusicStatus(w http.ResponseWriter, r *http.Request) {
	musicPlayerMu.Lock()
	defer musicPlayerMu.Unlock()

	resp := map[string]any{"ok": true}

	if musicPlayer == nil {
		resp["playing"] = false
		resp["paused"] = false
	} else {
		current, total := musicPlayer.Progress()
		resp["playing"] = musicPlayer.IsPlaying()
		resp["paused"] = musicPlayer.IsPaused()
		resp["current"] = current
		resp["total"] = total
	}

	// 队列信息
	playlistManagerMu.Lock()
	if playlistManager != nil {
		queue, currentIdx, loopMode, active := playlistManager.GetQueue()
		resp["queue"] = queue
		resp["queue_index"] = currentIdx
		resp["queue_loop_mode"] = string(loopMode)
		resp["queue_active"] = active
	} else {
		resp["queue"] = []QueueItem{}
		resp["queue_index"] = -1
		resp["queue_loop_mode"] = "none"
		resp["queue_active"] = false
	}
	playlistManagerMu.Unlock()

	writeJSON(w, resp)
}

func handleMusicNote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Instrument string  `json:"instrument"`
		Pitch      float64 `json:"pitch"`
		Volume     float64 `json:"volume"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	if req.Instrument == "" {
		req.Instrument = "note.harp"
	}
	if req.Pitch <= 0 {
		req.Pitch = 1.0
	}
	if req.Volume <= 0 {
		req.Volume = 1.0
	}

	botMgrMu.Lock()
	defer botMgrMu.Unlock()
	if botMgr == nil || !botMgr.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}

	cmd := fmt.Sprintf("/execute as @a at @s run playsound %s @s ~ ~ ~ %.2f %.3f %.2f",
		req.Instrument, req.Volume, req.Pitch, req.Volume)
	if err := botMgr.SendWOCmd(cmd); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleMusicActionbar(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Msg string `json:"msg"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	botMgrMu.Lock()
	defer botMgrMu.Unlock()
	if botMgr == nil || !botMgr.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接"})
		return
	}
	if req.Msg == "" {
		botMgr.SendWOCmd(`titleraw @a actionbar {"rawtext":[{"text":""}]}`)
	} else {
		botMgr.SendWOCmd(fmt.Sprintf(`titleraw @a actionbar {"rawtext":[{"text":"%s"}]}`, req.Msg))
	}
	writeJSON(w, map[string]any{"ok": true})
}

// ========== 播放列表 & 循环 ==========

func handleMusicQueueGet(w http.ResponseWriter, r *http.Request) {
	playlistManagerMu.Lock()
	pm := playlistManager
	playlistManagerMu.Unlock()
	if pm == nil {
		writeJSON(w, map[string]any{"ok": true, "queue": []QueueItem{}, "current": -1, "active": false, "loop_mode": "none"})
		return
	}
	queue, current, loopMode, active := pm.GetQueue()
	writeJSON(w, map[string]any{
		"ok":        true,
		"queue":     queue,
		"current":   current,
		"loop_mode": string(loopMode),
		"active":    active,
	})
}

func handleMusicQueueAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CacheKey string `json:"cache_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}

	midi := midiCacheGet(req.CacheKey)
	if midi == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "MIDI未上传或已过期"})
		return
	}

	title := strings.TrimSuffix(req.CacheKey, ".mid")
	title = strings.TrimSuffix(title, ".midi")
	totalMs := 0
	for _, n := range midi.Notes {
		totalMs += int(n.Tick) * 50
	}
	duration := fmt.Sprintf("%d:%02d", totalMs/60000, (totalMs%60000)/1000)

	item := QueueItem{
		CacheKey: req.CacheKey,
		Title:    title,
		Duration: duration,
		Notes:    len(midi.Notes),
	}

	playlistManagerMu.Lock()
	if playlistManager == nil {
		playlistManager = NewPlaylistManager("@a", 1.0)
	}
	playlistManager.AddToQueue(item)
	playlistManagerMu.Unlock()

	writeJSON(w, map[string]any{"ok": true, "item": item})
}

func handleMusicQueueRemove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Index int `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	playlistManagerMu.Lock()
	pm := playlistManager
	playlistManagerMu.Unlock()
	if pm == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "队列为空"})
		return
	}
	if err := pm.RemoveFromQueue(req.Index); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleMusicQueueClear(w http.ResponseWriter, r *http.Request) {
	playlistManagerMu.Lock()
	pm := playlistManager
	playlistManagerMu.Unlock()
	if pm != nil {
		pm.ClearQueue()
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleMusicLoop(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		playlistManagerMu.Lock()
		mode := LoopMode("none")
		if playlistManager != nil {
			mode = playlistManager.GetLoopMode()
		}
		playlistManagerMu.Unlock()
		writeJSON(w, map[string]any{"ok": true, "loop_mode": string(mode)})

	case "POST":
		var req struct {
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
			return
		}
		mode := LoopMode(req.Mode)
		if mode != LoopModeNone && mode != LoopModeSingle && mode != LoopModeList && mode != LoopModeShuffle {
			writeJSON(w, map[string]any{"ok": false, "error": "无效循环模式，可用: none, single, list, shuffle"})
			return
		}
		playlistManagerMu.Lock()
		if playlistManager == nil {
			playlistManager = NewPlaylistManager("@a", 1.0)
		}
		playlistManager.SetLoopMode(mode)
		playlistManagerMu.Unlock()
		writeJSON(w, map[string]any{"ok": true, "loop_mode": string(mode)})

	default:
		writeJSON(w, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func handleMusicNext(w http.ResponseWriter, r *http.Request) {
	playlistManagerMu.Lock()
	pm := playlistManager
	playlistManagerMu.Unlock()
	if pm == nil || !pm.IsActive() {
		writeJSON(w, map[string]any{"ok": false, "error": "队列未激活"})
		return
	}
	pm.Next()
	writeJSON(w, map[string]any{"ok": true})
}

func handleMusicPrev(w http.ResponseWriter, r *http.Request) {
	playlistManagerMu.Lock()
	pm := playlistManager
	playlistManagerMu.Unlock()
	if pm == nil || !pm.IsActive() {
		writeJSON(w, map[string]any{"ok": false, "error": "队列未激活"})
		return
	}
	pm.Prev()
	writeJSON(w, map[string]any{"ok": true})
}

// ========== Skin Builder ==========

// handleSkinPreview 返回皮肤雕像的预览信息（尺寸、方块数），不实际放置。
func handleSkinPreview(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "缺少 path"})
		return
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		writeJSON(w, map[string]any{"ok": false, "error": "文件不存在"})
		return
	}

	f, err := os.Open(path)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无法打开文件: " + err.Error()})
		return
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "图片解码失败: " + err.Error()})
		return
	}

	opts := media.SkinBuildOptions{
		Scale:          getIntFromQuery(r, "scale", 2),
		ArmType:        media.SkinArmType(getStringFromQuery(r, "arm_type", "classic")),
		BlockSet:       media.SkinBlockSet(getStringFromQuery(r, "block_set", "mixed")),
		AlphaCutoff:    uint8(getIntFromQuery(r, "alpha_cutoff", 16)),
		OuterThickness: getIntFromQuery(r, "outer_thickness", 1),
		Rotation:       getIntFromQuery(r, "rotation", 0),
		Solid:          getStringFromQuery(r, "solid", "") == "true",
		FillBlock:      getStringFromQuery(r, "fill_block", "minecraft:stone"),
	}

	detectedArm := media.DetectArmType(img)
	_, info, err := media.BuildSkinStatue(img, opts)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{
		"ok":                true,
		"width":             info.Width,
		"height":            info.Height,
		"length":            info.Length,
		"blockCount":        info.BlockCount,
		"detected_arm_type": string(detectedArm),
	})
}

// handleSkinVoxel 返回皮肤雕像的3D体素预览数据。
func handleSkinVoxel(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "缺少 path"})
		return
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		writeJSON(w, map[string]any{"ok": false, "error": "文件不存在"})
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无法打开文件: " + err.Error()})
		return
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "图片解码失败: " + err.Error()})
		return
	}

	scale := getIntFromQuery(r, "scale", 2)
	if scale > 2 {
		scale = 2
	} // 预览降低缩放保持性能
	opts := media.SkinBuildOptions{
		Scale:          scale,
		ArmType:        media.SkinArmType(getStringFromQuery(r, "arm_type", "classic")),
		BlockSet:       media.SkinBlockSet(getStringFromQuery(r, "block_set", "mixed")),
		AlphaCutoff:    uint8(getIntFromQuery(r, "alpha_cutoff", 16)),
		OuterThickness: getIntFromQuery(r, "outer_thickness", 1),
		Rotation:       getIntFromQuery(r, "rotation", 0),
	}
	blocks, info, err := media.BuildSkinStatue(img, opts)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	maxPoints := 100000
	if len(blocks) > maxPoints {
		// uniform sample to keep all body parts visible
		sampled := make([]media.SkinBlock, 0, maxPoints)
		step := float64(len(blocks)) / float64(maxPoints)
		for i := 0; i < maxPoints; i++ {
			idx := int(float64(i) * step)
			if idx < len(blocks) {
				sampled = append(sampled, blocks[idx])
			}
		}
		blocks = sampled
	}
	points := make([]map[string]any, len(blocks))
	for i, b := range blocks {
		points[i] = map[string]any{
			"x":  int(b.X),
			"y":  int(b.Y),
			"z":  int(b.Z),
			"c":  media.BlockHexColor(b.BlockName),
			"id": b.BlockName,
		}
	}

	voxel := map[string]any{
		"points": points, "size_x": info.Width, "size_y": info.Height,
		"size_z": info.Length, "step": 1, "too_large": len(blocks) >= maxPoints,
		"total_solid": len(blocks),
	}
	writeJSON(w, map[string]any{"ok": true, "voxel": voxel})
}

// handleSkinBuild 启动皮肤雕像任务。
func handleSkinBuild(w http.ResponseWriter, r *http.Request) {
	updateBlockedMu.RLock()
	blocked := updateBlocked
	updateBlockedMu.RUnlock()
	if blocked {
		writeJSON(w, map[string]any{"ok": false, "error": "检测到新版本，请更新后再使用"})
		return
	}
	taskMu.Lock()
	if activeTask != nil && activeTask.Running() {
		taskMu.Unlock()
		writeJSON(w, map[string]any{"ok": false, "error": "已有任务运行中"})
		return
	}

	path := r.FormValue("path")
	if path == "" {
		taskMu.Unlock()
		writeJSON(w, map[string]any{"ok": false, "error": "缺少 path"})
		return
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		taskMu.Unlock()
		writeJSON(w, map[string]any{"ok": false, "error": "文件不存在"})
		return
	}

	botMgrMu.Lock()
	connected := botMgr != nil && botMgr.IsConnected()
	botMgrMu.Unlock()
	if !connected {
		taskMu.Unlock()
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器，请先连接"})
		return
	}

	params := map[string]any{
		"path":            path,
		"scale":           getIntFromForm(r, "scale", 2),
		"arm_type":        getStringFromForm(r, "arm_type", "classic"),
		"block_set":       getStringFromForm(r, "block_set", "mixed"),
		"alpha_cutoff":    getIntFromForm(r, "alpha_cutoff", 16),
		"rotation":        getIntFromForm(r, "rotation", 0),
		"outer_thickness": getIntFromForm(r, "outer_thickness", 1),
		"solid":           getStringFromForm(r, "solid", "") == "true",
		"fill_block":      getStringFromForm(r, "fill_block", "minecraft:stone"),
		"x":               getIntFromForm(r, "x", 0),
		"y":               getIntFromForm(r, "y", -64),
		"z":               getIntFromForm(r, "z", 0),
		"speed":           getIntFromForm(r, "speed", 9500),
	}
	tc := state.NewTaskController(hub)
	activeTask = tc
	taskMu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				hub.Emit("task_error", mustJSON(map[string]any{"error": fmt.Sprintf("任务异常崩溃: %v", r)}))
			}
			taskMu.Lock()
			if activeTask == tc {
				activeTask = nil
			}
			taskMu.Unlock()
		}()
		dispatchTask(tc, "skin", rawParams(params))
	}()
	writeJSON(w, map[string]any{"ok": true})
}

// ========== Skin Online ==========

// handleSkinOnlineList 返回有皮肤缓存的在线玩家列表。
func handleSkinOnlineList(w http.ResponseWriter, r *http.Request) {
	botMgrMu.Lock()
	connected := botMgr != nil && botMgr.IsConnected()
	botMgrMu.Unlock()
	if !connected {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	players := playerInfoMgr.GetAllPlayers()
	type skinPlayer struct {
		Name   string `json:"name"`
		Width  uint32 `json:"width"`
		Height uint32 `json:"height"`
	}
	var list []skinPlayer
	for _, pi := range players {
		if len(pi.SkinData) > 0 {
			list = append(list, skinPlayer{
				Name:   pi.Name,
				Width:  pi.SkinWidth,
				Height: pi.SkinHeight,
			})
		}
	}
	writeJSON(w, map[string]any{"ok": true, "players": list})
}

// handleSkinOnlinePreview 返回在线玩家皮肤的预览信息。
func handleSkinOnlinePreview(w http.ResponseWriter, r *http.Request) {
	botMgrMu.Lock()
	connected := botMgr != nil && botMgr.IsConnected()
	botMgrMu.Unlock()
	if !connected {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}
	playerName := r.URL.Query().Get("player")
	if playerName == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "缺少 player 参数"})
		return
	}
	skinData, sw, sh, ok := playerInfoMgr.GetPlayerSkin(playerName)
	if !ok {
		writeJSON(w, map[string]any{"ok": false, "error": "玩家不在线或没有皮肤缓存"})
		return
	}
	img := image.NewNRGBA(image.Rect(0, 0, int(sw), int(sh)))
	copy(img.Pix, skinData)

	opts := media.SkinBuildOptions{
		Scale:          getIntFromQuery(r, "scale", 2),
		ArmType:        media.SkinArmType(getStringFromQuery(r, "arm_type", "classic")),
		BlockSet:       media.SkinBlockSet(getStringFromQuery(r, "block_set", "mixed")),
		AlphaCutoff:    uint8(getIntFromQuery(r, "alpha_cutoff", 16)),
		OuterThickness: getIntFromQuery(r, "outer_thickness", 1),
		Rotation:       getIntFromQuery(r, "rotation", 0),
	}
	detectedArm := media.DetectArmType(img)
	_, info, err := media.BuildSkinStatue(img, opts)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{
		"ok":                true,
		"player":            playerName,
		"width":             info.Width,
		"height":            info.Height,
		"length":            info.Length,
		"blockCount":        info.BlockCount,
		"detected_arm_type": string(detectedArm),
	})
}

// handleSkinOnlineBuild 启动在线玩家皮肤雕像任务。
func handleSkinOnlineBuild(w http.ResponseWriter, r *http.Request) {
	updateBlockedMu.RLock()
	blocked := updateBlocked
	updateBlockedMu.RUnlock()
	if blocked {
		writeJSON(w, map[string]any{"ok": false, "error": "检测到新版本，请更新后再使用"})
		return
	}
	taskMu.Lock()
	if activeTask != nil && activeTask.Running() {
		taskMu.Unlock()
		writeJSON(w, map[string]any{"ok": false, "error": "已有任务运行中"})
		return
	}

	playerName := r.FormValue("player")
	if playerName == "" {
		taskMu.Unlock()
		writeJSON(w, map[string]any{"ok": false, "error": "缺少 player 参数"})
		return
	}

	botMgrMu.Lock()
	connected := botMgr != nil && botMgr.IsConnected()
	botMgrMu.Unlock()
	if !connected {
		taskMu.Unlock()
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器，请先连接"})
		return
	}

	params := map[string]any{
		"player":          playerName,
		"scale":           getIntFromForm(r, "scale", 2),
		"arm_type":        getStringFromForm(r, "arm_type", "classic"),
		"block_set":       getStringFromForm(r, "block_set", "mixed"),
		"alpha_cutoff":    getIntFromForm(r, "alpha_cutoff", 16),
		"rotation":        getIntFromForm(r, "rotation", 0),
		"outer_thickness": getIntFromForm(r, "outer_thickness", 1),
		"solid":           getStringFromForm(r, "solid", "") == "true",
		"fill_block":      getStringFromForm(r, "fill_block", "minecraft:stone"),
		"x":               getIntFromForm(r, "x", 0),
		"y":               getIntFromForm(r, "y", -64),
		"z":               getIntFromForm(r, "z", 0),
		"speed":           getIntFromForm(r, "speed", 9500),
	}
	tc := state.NewTaskController(hub)
	activeTask = tc
	taskMu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				hub.Emit("task_error", mustJSON(map[string]any{"error": fmt.Sprintf("任务异常崩溃: %v", r)}))
			}
			taskMu.Lock()
			if activeTask == tc {
				activeTask = nil
			}
			taskMu.Unlock()
		}()
		dispatchTask(tc, "skin", rawParams(params))
	}()
	writeJSON(w, map[string]any{"ok": true})
}

// ========== Skin Head Thumbnail ==========

// extractSkinHead 从皮肤图片中提取头部正面（8x8 像素），返回 16x16 的缩略图。
// 标准 MC 皮肤头部正面位于 UV(8,8) 到 (16,16)。
func extractSkinHead(img image.Image) *image.NRGBA {
	bounds := img.Bounds()
	w := bounds.Dx()
	unit := w / 64
	if unit < 1 {
		unit = 1
	}
	// 将图片转为 NRGBA 方便像素读取
	nrgba := image.NewNRGBA(image.Rect(0, 0, w, bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < w; x++ {
			nrgba.SetNRGBA(x, y, colorNRGBAModel(img.At(x, y)))
		}
	}

	// 头部正面：UV(8,8) 起，8x8 单位
	headX := 8 * unit
	headY := 8 * unit
	headSize := 8 * unit

	// 缩略到 16x16
	thumb := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for ty := 0; ty < 16; ty++ {
		for tx := 0; tx < 16; tx++ {
			sx := headX + tx*headSize/16
			sy := headY + ty*headSize/16
			thumb.SetNRGBA(tx, ty, nrgba.NRGBAAt(sx, sy))
		}
	}
	return thumb
}

// colorNRGBAModel 是 main.go 中已有的工具函数，在 media 包中也有定义。
// 此处内联避免跨包依赖。
func colorNRGBAModel(c color.Color) color.NRGBA {
	if n, ok := c.(color.NRGBA); ok {
		return n
	}
	r, g, b, a := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// handleSkinHead 返回皮肤 PNG 文件的头部缩略图（16x16 PNG）。
func handleSkinHead(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "缺少 path"})
		return
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		writeJSON(w, map[string]any{"ok": false, "error": "文件不存在"})
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无法打开文件: " + err.Error()})
		return
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "图片解码失败"})
		return
	}

	thumb := extractSkinHead(img)
	w.Header().Set("Content-Type", "image/png")
	png.Encode(w, thumb)
}

// handleSkinOnlineHead 返回在线玩家的皮肤头部缩略图（16x16 PNG）。
func handleSkinOnlineHead(w http.ResponseWriter, r *http.Request) {
	playerName := r.URL.Query().Get("player")
	if playerName == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "缺少 player"})
		return
	}
	skinData, sw, sh, ok := playerInfoMgr.GetPlayerSkin(playerName)
	if !ok {
		writeJSON(w, map[string]any{"ok": false, "error": "玩家不在线或没有皮肤缓存"})
		return
	}
	img := image.NewNRGBA(image.Rect(0, 0, int(sw), int(sh)))
	copy(img.Pix, skinData)

	thumb := extractSkinHead(img)
	w.Header().Set("Content-Type", "image/png")
	png.Encode(w, thumb)
}

// ========== Map Player ==========

// handleMapScan 扫描指定区域中的物品展示框，返回地图网格信息。
func handleMapScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持 POST"})
		return
	}
	mapPlayerMu.Lock()
	defer mapPlayerMu.Unlock()

	bm := getBotManager()
	if bm == nil || !bm.IsConnected() {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}

	x1 := int32(getIntFromForm(r, "x1", 0))
	y1 := int32(getIntFromForm(r, "y1", 0))
	z1 := int32(getIntFromForm(r, "z1", 0))
	x2 := int32(getIntFromForm(r, "x2", 0))
	y2 := int32(getIntFromForm(r, "y2", 0))
	z2 := int32(getIntFromForm(r, "z2", 0))

	grid, err := NewMapPlayer(bm).ScanItemFrames(x1, y1, z1, x2, y2, z2)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	player := NewMapPlayer(bm)
	player.grid = grid
	player.mapCfg.IsConfigured = true
	mapPlayer = player

	writeJSON(w, map[string]any{
		"ok":        true,
		"rows":      grid.Rows,
		"cols":      grid.Cols,
		"planeType": grid.PlaneType,
	})
}

// handleMapDisplay 在物品展示框上显示一张图片。
func handleMapDisplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持 POST"})
		return
	}
	mapPlayerMu.Lock()
	defer mapPlayerMu.Unlock()

	if mapPlayer == nil || mapPlayer.grid == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "请先扫描地图区域"})
		return
	}

	path := r.FormValue("path")
	if path == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "缺少 path"})
		return
	}

	go func() {
		if err := mapPlayer.PlayImage(path); err != nil {
			hub.Emit("map_error", mustJSON(map[string]any{"error": err.Error()}))
			return
		}
		hub.Emit("map_done", mustJSON(map[string]any{"msg": "图片显示完成"}))
	}()

	writeJSON(w, map[string]any{"ok": true})
}

// handleMapPlay 在物品展示框上播放视频。
func handleMapPlay(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持 POST"})
		return
	}
	mapPlayerMu.Lock()
	defer mapPlayerMu.Unlock()

	if mapPlayer == nil || mapPlayer.grid == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "请先扫描地图区域"})
		return
	}

	path := r.FormValue("path")
	if path == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "缺少 path"})
		return
	}

	cfg := MediaConfig{
		MediaType:   "video",
		MediaPath:   path,
		VideoFPS:    getIntFromForm(r, "fps", 10),
		VideoSpeed:  getFloatFromForm(r, "speed", 1.0),
		StartFrame:  getIntFromForm(r, "start_frame", 0),
		Concurrency: getIntFromForm(r, "concurrency", 4),
		BatchSize:   getIntFromForm(r, "batch_size", 8),
	}

	go func() {
		if err := mapPlayer.PlayVideo(cfg); err != nil {
			hub.Emit("map_error", mustJSON(map[string]any{"error": err.Error()}))
			return
		}
		hub.Emit("map_done", mustJSON(map[string]any{"msg": "视频播放结束"}))
	}()

	writeJSON(w, map[string]any{"ok": true, "msg": "开始播放"})
}

// handleMapStop 停止地图播放。
func handleMapStop(w http.ResponseWriter, r *http.Request) {
	mapPlayerMu.Lock()
	defer mapPlayerMu.Unlock()
	if mapPlayer != nil {
		mapPlayer.Stop()
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleMapGrid 返回当前地图网格信息。
func handleMapGrid(w http.ResponseWriter, r *http.Request) {
	mapPlayerMu.Lock()
	defer mapPlayerMu.Unlock()
	if mapPlayer == nil || mapPlayer.grid == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "未扫描"})
		return
	}
	writeJSON(w, map[string]any{
		"ok":        true,
		"rows":      mapPlayer.grid.Rows,
		"cols":      mapPlayer.grid.Cols,
		"planeType": mapPlayer.grid.PlaneType,
	})
}

func getIntFromQuery(r *http.Request, key string, def int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	var v int
	fmt.Sscanf(s, "%d", &v)
	return v
}

func getStringFromQuery(r *http.Request, key, def string) string {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	return s
}

func getIntFromForm(r *http.Request, key string, def int) int {
	s := r.FormValue(key)
	if s == "" {
		return def
	}
	var v int
	fmt.Sscanf(s, "%d", &v)
	return v
}

func getStringFromForm(r *http.Request, key, def string) string {
	s := r.FormValue(key)
	if s == "" {
		return def
	}
	return s
}

func getFloatFromForm(r *http.Request, key string, def float64) float64 {
	s := r.FormValue(key)
	if s == "" {
		return def
	}
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return v
}

// ========== Startup Protection ==========

const appVersion = "1.0.4-beta.49+arm64.20260811"
const appVersionCode = 10890

// shortVersion returns the major.minor.patch portion (e.g. "1.0.3").
func shortVersion() string {
	v := appVersion
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		v = v[:idx]
	}
	if idx := strings.IndexByte(v, '+'); idx >= 0 {
		v = v[:idx]
	}
	return v
}

func runStartupProtection() {
	// 1. Run local integrity checks
	results := RunIntegrityChecks()
	allPassed := true
	for name, r := range results {
		if !r.Passed {
			allPassed = false
			hub.Emit("integrity_alert", mustJSON(map[string]any{
				"check": name, "passed": false, "message": r.Message,
			}))
		}
	}
	if !allPassed {
		hub.Emit("integrity_alert", mustJSON(map[string]any{
			"check": "summary", "passed": false,
			"message": "资源完整性校验失败，可能被篡改，请从官方渠道重新下载",
		}))
	}

	// 2. Version check against server
	go func() {
		cfg := config.ReloadConfig()
		serverCode := ""
		if cfg != nil {
			serverCode = cfg.ServerCode
		}
		sigHash := GetApkSignatureHash()
		resp, err := prism.CheckVersion(prism.VersionCheckRequest{
			Version:       appVersion,
			VersionCode:   appVersionCode,
			Platform:      "android",
			ServerCode:    serverCode,
			SignatureHash: sigHash,
		})
		if err != nil {
			hub.Emit("version_check", mustJSON(map[string]any{
				"ok": false, "error": "版本检测失败: " + err.Error(),
			}))
			return
		}
		if resp.ForceUpdate && resp.LatestVersionCode > appVersionCode {
			updateBlockedMu.Lock()
			updateBlocked = true
			updateBlockedMu.Unlock()
		}
		// 服务器权威判定：签名无效 → 直接进入拦截模式。
		// 即使本地校验被绕过，只要应用联网上报，也会被服务器判定为盗版并锁定。
		if resp.SignatureValid == false {
			MarkTampered("服务器判定签名无效")
		}
		hub.Emit("version_check", mustJSON(map[string]any{
			"ok":                   resp.OK,
			"latest_version":       resp.LatestVersion,
			"latest_version_code":  resp.LatestVersionCode,
			"force_update":         resp.ForceUpdate,
			"update_url":           resp.UpdateURL,
			"update_message":       resp.UpdateMessage,
			"current_version":      appVersion,
			"current_version_code": appVersionCode,
			"signature_valid":      resp.SignatureValid,
			"signature_message":    resp.SignatureMessage,
		}))
	}()

	// 3. Fetch announcements
	go func() {
		cfg := config.ReloadConfig()
		serverCode := ""
		if cfg != nil {
			serverCode = cfg.ServerCode
		}
		resp, err := prism.FetchAnnouncements(appVersion, appVersionCode, serverCode, "")
		if err != nil {
			return // silently fail, not critical
		}
		if resp.OK && len(resp.Announcements) > 0 {
			// Convert to JSON-friendly format
			anns := make([]map[string]any, len(resp.Announcements))
			for i, a := range resp.Announcements {
				anns[i] = map[string]any{
					"id":           a.ID,
					"title":        a.Title,
					"content":      a.Content,
					"content_type": a.ContentType,
					"severity":     a.Severity,
					"display_mode": a.DisplayMode,
					"can_dismiss":  a.CanDismiss,
					"created_at":   a.CreatedAt,
					"expires_at":   a.ExpiresAt,
				}
			}
			hub.Emit("announcements", mustJSON(map[string]any{
				"announcements": anns,
			}))
		}
	}()
}

func handleVersionCheck(w http.ResponseWriter, r *http.Request) {
	cfg := config.ReloadConfig()
	serverCode := ""
	if cfg != nil {
		serverCode = cfg.ServerCode
	}
	sigHash := GetApkSignatureHash()
	resp, err := prism.CheckVersion(prism.VersionCheckRequest{
		Version:       appVersion,
		VersionCode:   appVersionCode,
		Platform:      "android",
		ServerCode:    serverCode,
		SignatureHash: sigHash,
	})
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if resp.SignatureValid == false {
		MarkTampered("服务器判定签名无效")
	}
	writeJSON(w, map[string]any{
		"ok":                   true,
		"latest_version":       resp.LatestVersion,
		"latest_version_code":  resp.LatestVersionCode,
		"force_update":         resp.ForceUpdate,
		"update_url":           resp.UpdateURL,
		"update_message":       resp.UpdateMessage,
		"current_version":      appVersion,
		"current_version_code": appVersionCode,
		"signature_valid":      resp.SignatureValid,
		"signature_message":    resp.SignatureMessage,
	})
}

func handleAnnouncements(w http.ResponseWriter, r *http.Request) {
	cfg := config.ReloadConfig()
	serverCode := ""
	if cfg != nil {
		serverCode = cfg.ServerCode
	}
	lastSeenID := ""
	if r.Method == "POST" {
		var req struct {
			LastSeenID string `json:"last_seen_id"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		lastSeenID = req.LastSeenID
	}
	resp, err := prism.FetchAnnouncements(appVersion, appVersionCode, serverCode, lastSeenID)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	anns := make([]map[string]any, len(resp.Announcements))
	for i, a := range resp.Announcements {
		anns[i] = map[string]any{
			"id": a.ID, "title": a.Title, "content": a.Content,
			"content_type": a.ContentType, "severity": a.Severity,
			"display_mode": a.DisplayMode, "can_dismiss": a.CanDismiss,
			"created_at": a.CreatedAt, "expires_at": a.ExpiresAt,
		}
	}
	writeJSON(w, map[string]any{"ok": true, "announcements": anns})
}

func handleIntegrityCheck(w http.ResponseWriter, r *http.Request) {
	results := RunIntegrityChecks()
	allPassed := true
	for _, r := range results {
		if !r.Passed {
			allPassed = false
		}
	}
	writeJSON(w, map[string]any{"ok": true, "all_passed": allPassed, "results": results})
}

// ========== Toolbox API ==========

// refreshToolboxInfo 从认证服务器刷新工具箱信息缓存
func refreshToolboxInfo() {
	cfg := config.ReloadConfig()
	if cfg == nil || cfg.Token == "" {
		toolboxInfoMu.Lock()
		toolboxInfo = nil
		toolboxInfoMu.Unlock()
		return
	}
	resp, err := prism.GetToolboxInfo(cfg.Token)
	if err != nil {
		return
	}
	if resp != nil && resp.OK {
		toolboxInfoMu.Lock()
		toolboxInfo = resp.Data
		toolboxInfoTime = time.Now()
		toolboxInfoMu.Unlock()
	}
}

// GetEffectivePrompt 获取实际生效的提示词
func GetEffectivePrompt() string {
	toolboxInfoMu.RLock()
	defer toolboxInfoMu.RUnlock()
	// 未购买提示词 → 一律用内置默认，不受理服务器传来的任何值
	if toolboxInfo != nil && toolboxInfo.PromptPurchased {
		if toolboxInfo.EffectivePrompt != "" {
			return toolboxInfo.EffectivePrompt
		}
		return toolboxInfo.DefaultPrompt
	}
	return ""
}

// GetEffectiveName 获取实际生效的名称
func GetEffectiveName() string {
	toolboxInfoMu.RLock()
	defer toolboxInfoMu.RUnlock()
	// 未购买命名 → 一律用内置默认（Prism），不受理服务器传来的任何值
	if toolboxInfo != nil && toolboxInfo.NamePurchased {
		if toolboxInfo.EffectiveName != "" {
			return toolboxInfo.EffectiveName
		}
		return toolboxInfo.DefaultName
	}
	return ""
}

// getCreatorName 获取召唤者显示名称，用于无权限时发送提示消息
// 匿名用户 → "匿名用户"
// 已登录+已购买命名 → EffectiveName
// 已登录+未购买命名 → "prism用户"
// isValidSkinSize 判断是否为合法的 MC 皮肤尺寸（64x64 / 64x32 / 128x128）。
func isValidSkinSize(w, h int) bool {
	return (w == 64 && h == 64) || (w == 64 && h == 32) || (w == 128 && h == 128)
}

func getCreatorName() string {
	cfg := config.ReloadConfig()
	if cfg == nil || cfg.Token == "" {
		return "匿名用户"
	}
	toolboxInfoMu.RLock()
	info := toolboxInfo
	toolboxInfoMu.RUnlock()
	if info != nil && info.NamePurchased && info.EffectiveName != "" {
		return info.EffectiveName
	}
	return "prism用户"
}

// handleToolboxMyInfo 获取当前用户的工具箱状态
func handleToolboxMyInfo(w http.ResponseWriter, r *http.Request) {
	cfg := config.ReloadConfig()
	if cfg == nil || cfg.Token == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "未配置 Token"})
		return
	}

	// 尝试从缓存读取，如果超过 30 秒则刷新
	toolboxInfoMu.RLock()
	stale := time.Since(toolboxInfoTime) > 30*time.Second || toolboxInfo == nil
	toolboxInfoMu.RUnlock()
	if stale {
		go refreshToolboxInfo()
	}

	toolboxInfoMu.RLock()
	info := toolboxInfo
	toolboxInfoMu.RUnlock()

	if info == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "获取工具箱信息失败"})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "data": info})
}

// handleToolboxUpdate 修改自定义提示词和名称
func handleToolboxUpdate(w http.ResponseWriter, r *http.Request) {
	cfg := config.ReloadConfig()
	if cfg == nil || cfg.Token == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "未配置 Token"})
		return
	}
	var req struct {
		CustomPrompt string `json:"custom_prompt"`
		CustomName   string `json:"custom_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效 JSON"})
		return
	}
	result, err := prism.UpdateToolboxInfo(cfg.Token, req.CustomPrompt, req.CustomName)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	go refreshToolboxInfo()
	writeJSON(w, result)
}

// handleToolboxStartTrial 激活免费试用
func handleToolboxStartTrial(w http.ResponseWriter, r *http.Request) {
	cfg := config.ReloadConfig()
	if cfg == nil || cfg.Token == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "未配置 Token"})
		return
	}
	result, err := prism.StartTrial(cfg.Token)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	go refreshToolboxInfo()
	writeJSON(w, result)
}

// handleToolboxPurchasePrompt 购买自定义提示词
func handleToolboxPurchasePrompt(w http.ResponseWriter, r *http.Request) {
	cfg := config.ReloadConfig()
	if cfg == nil || cfg.Token == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "未配置 Token"})
		return
	}
	result, err := prism.PurchasePrompt(cfg.Token)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	go refreshToolboxInfo()
	writeJSON(w, result)
}

// handleToolboxPurchaseName 购买自定义命名
func handleToolboxPurchaseName(w http.ResponseWriter, r *http.Request) {
	cfg := config.ReloadConfig()
	if cfg == nil || cfg.Token == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "未配置 Token"})
		return
	}
	result, err := prism.PurchaseName(cfg.Token)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	go refreshToolboxInfo()
	writeJSON(w, result)
}

// handleToolboxPurchaseExtension 续期工具箱使用权
func handleToolboxPurchaseExtension(w http.ResponseWriter, r *http.Request) {
	cfg := config.ReloadConfig()
	if cfg == nil || cfg.Token == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "未配置 Token"})
		return
	}
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if v, err := fmt.Sscanf(d, "%d", &days); err != nil || v != 1 {
			days = 30
		}
	}
	result, err := prism.PurchaseExtension(cfg.Token, days)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	go refreshToolboxInfo()
	writeJSON(w, result)
}

// handleToolboxNutsBalance 查询板栗余额
func handleToolboxNutsBalance(w http.ResponseWriter, r *http.Request) {
	cfg := config.ReloadConfig()
	if cfg == nil || cfg.Token == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "未配置 Token"})
		return
	}
	result, err := prism.GetNutsBalance(cfg.Token)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, result)
}

// ========== Server Start ==========

func handleFileScan(w http.ResponseWriter, r *http.Request) {
	rootPath := r.URL.Query().Get("path")
	filter := r.URL.Query().Get("filter") // "building" or "music"
	if rootPath == "" {
		rootPath = "/storage/emulated/0"
	}
	// Prevent directory traversal: clean and ensure under allowed roots
	clean := filepath.Clean(rootPath)
	allowed := []string{"/storage/emulated", "/sdcard", "/data/data/com.prismtool.box/files"}
	valid := false
	for _, a := range allowed {
		if strings.HasPrefix(clean, a) {
			valid = true
			break
		}
	}
	if !valid {
		writeJSON(w, map[string]any{"ok": false, "error": "路径不允许"})
		return
	}

	entries, err := os.ReadDir(clean)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "读取目录失败: " + err.Error()})
		return
	}

	// Extension filter map
	buildingExts := map[string]bool{
		".mcstructure": true, ".schematic": true, ".schem": true,
		".mcworld": true, ".bdx": true,
	}
	musicExts := map[string]bool{
		".mid": true, ".midi": true,
	}
	var filterExts map[string]bool
	skinExts := map[string]bool{
		".png": true,
	}
	switch filter {
	case "building":
		filterExts = buildingExts
	case "music":
		filterExts = musicExts
	case "skin":
		filterExts = skinExts
	}

	type entry struct {
		Name        string `json:"name"`
		Path        string `json:"path"`
		Type        string `json:"type"`
		Size        int64  `json:"size,omitempty"`
		ModTime     int64  `json:"mtime"`
		Ext         string `json:"ext,omitempty"`
		HasBuilding bool   `json:"has_building"`
	}
	var dirs, files []entry

	for _, e := range entries {
		name := e.Name()
		// Skip hidden files and Android directories
		if strings.HasPrefix(name, ".") {
			continue
		}
		if name == "Android" && e.IsDir() {
			continue
		}

		fullPath := clean + "/" + name
		info, err := e.Info()
		if err != nil {
			continue
		}

		en := entry{
			Name:    name,
			Path:    fullPath,
			ModTime: info.ModTime().Unix(),
		}

		if e.IsDir() {
			en.Type = "dir"
			dirs = append(dirs, en)
		} else {
			ext := strings.ToLower(filepath.Ext(name))
			if filterExts != nil && !filterExts[ext] {
				continue
			}
			// 皮肤模式：只保留合法 MC 皮肤尺寸的 PNG（64x64, 64x32, 128x128）
			if filter == "skin" && ext == ".png" {
				f, err := os.Open(fullPath)
				if err == nil {
					cfg, _, decodeErr := image.DecodeConfig(f)
					f.Close()
					if decodeErr != nil || !isValidSkinSize(cfg.Width, cfg.Height) {
						continue
					}
				} else {
					continue
				}
			}
			en.Type = "file"
			en.Size = info.Size()
			en.Ext = ext
			files = append(files, en)
		}
	}

	// Sort: dirs by name asc, files by mtime desc (newest first)
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime > files[j].ModTime
	})

	parent := filepath.Dir(clean)
	// Don't go above allowed roots
	parentValid := false
	for _, a := range allowed {
		if strings.HasPrefix(parent, a) {
			parentValid = true
			break
		}
	}
	if !parentValid {
		parent = ""
	}

	// Quick recursive scan: mark dirs that contain building files
	if filter == "building" {
		deadline := time.Now().Add(500 * time.Millisecond)
		var wg sync.WaitGroup
		for i := range dirs {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				dirs[idx].HasBuilding = hasBuildingFile(dirs[idx].Path, 0, 6, deadline)
			}(i)
		}
		wg.Wait()
	}

	all := append(dirs, files...)
	writeJSON(w, map[string]any{
		"ok":      true,
		"path":    clean,
		"entries": all,
		"parent":  parent,
	})
}

func hasBuildingFile(path string, depth, maxDepth int, deadline time.Time) bool {
	if depth >= maxDepth || time.Now().After(deadline) {
		return false
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	// Check files directly first (fast path)
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".mcstructure", ".schematic", ".schem", ".mcworld",
			".bdx":
			return true
		}
	}
	// Collect subdirectories
	var subDirs []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !e.IsDir() {
			continue
		}
		switch name {
		case "cache", "LOST.DIR", "Music", "Alarms", "Ringtones", "Notifications", "Podcasts":
			continue
		}
		subDirs = append(subDirs, path+"/"+name)
	}
	if len(subDirs) == 0 {
		return false
	}
	// Scan subdirectories in parallel (max 32 concurrent)
	type scanResult struct{ found bool }
	ch := make(chan scanResult, len(subDirs))
	sem := make(chan struct{}, 32)
	for _, sd := range subDirs {
		sem <- struct{}{}
		go func(dir string) {
			defer func() { <-sem }()
			ch <- scanResult{hasBuildingFile(dir, depth+1, maxDepth, deadline)}
		}(sd)
	}
	for range subDirs {
		if (<-ch).found {
			return true
		}
	}
	return false
}

func handleCacheClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}

	// 需要清理的目录
	scanDirs := []string{"/data/data/com.prismtool.box/files", "/tmp/td_uploads"}
	// 保护的文件（不删除）
	protected := map[string]bool{
		"config.json": true,
		"td_state.db": true,
	}

	// 可删除的建筑/音乐文件扩展名
	deleteExts := map[string]bool{
		".mcstructure": true, ".schematic": true, ".schem": true,
		".mcworld": true, ".bdx": true,
		".mid": true, ".midi": true,
		".png": true, ".jpg": true, ".jpeg": true,
		".zip": true, ".rar": true,
	}

	var deleted int64
	var freed int64
	var errors []string

	for _, dir := range scanDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // 目录不存在就跳过
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if protected[name] {
				continue
			}
			ext := strings.ToLower(filepath.Ext(name))
			if !deleteExts[ext] {
				continue
			}
			fullPath := filepath.Join(dir, name)
			info, err := e.Info()
			if err != nil {
				continue
			}
			if err := os.Remove(fullPath); err != nil {
				errors = append(errors, name+": "+err.Error())
				continue
			}
			deleted++
			freed += info.Size()
		}
	}

	resp := map[string]any{
		"ok":      true,
		"deleted": deleted,
		"freed":   freed,
	}
	if len(errors) > 0 {
		resp["errors"] = errors
	}
	writeJSON(w, resp)
}

func handleStoragePermission(w http.ResponseWriter, r *http.Request) {
	// Check if /storage/emulated/0 is readable
	testPath := "/storage/emulated/0"
	_, err := os.ReadDir(testPath)
	granted := err == nil
	writeJSON(w, map[string]any{"ok": true, "granted": granted})
}
func handleFileRead(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "缺少路径"})
		return
	}
	clean := filepath.Clean(filePath)
	allowed := []string{"/storage/emulated", "/sdcard", "/data/data/com.prismtool.box/files"}
	valid := false
	for _, a := range allowed {
		if strings.HasPrefix(clean, a) {
			valid = true
			break
		}
	}
	if !valid {
		writeJSON(w, map[string]any{"ok": false, "error": "路径不允许"})
		return
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "读取文件失败: " + err.Error()})
		return
	}
	name := filepath.Base(clean)
	ct := "application/octet-stream"
	if strings.HasSuffix(name, ".mid") || strings.HasSuffix(name, ".midi") {
		ct = "audio/midi"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	w.Write(data)
}
func handleFileWrite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	if req.Path == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "缺少路径"})
		return
	}
	clean := filepath.Clean(req.Path)
	allowed := []string{"/storage/emulated", "/sdcard", "/data/data/com.prismtool.box/files"}
	valid := false
	for _, a := range allowed {
		if strings.HasPrefix(clean, a) {
			valid = true
			break
		}
	}
	if !valid {
		writeJSON(w, map[string]any{"ok": false, "error": "路径不允许"})
		return
	}
	if err := os.WriteFile(clean, []byte(req.Content), 0644); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "写入失败: " + err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// tamperedPageHTML 是检测到应用被篡改/重打包时显示的拦截页（自包含，无外部依赖）。
const tamperedPageHTML = `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Prism 工具箱</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'Noto Sans SC','PingFang SC',system-ui,sans-serif;background:#0f1720;color:#e2e8f0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px}
.card{max-width:420px;text-align:center;background:#1e293b;border:1px solid #334155;border-radius:16px;padding:40px 32px}
.shield{font-size:52px;margin-bottom:16px}
h1{font-size:20px;margin-bottom:12px;color:#f8fafc}
p{font-size:14px;line-height:1.7;color:#94a3b8;margin-bottom:8px}
.dim{font-size:12px;color:#64748b}
</style></head><body><div class="card">
<div class="shield">🛡️</div>
<h1>此应用已被篡改</h1>
<p>检测到当前安装的 APK 不是官方版本（签名或包名校验失败）。</p>
<p>为保护您的账号与设备安全，本应用已停止运行。</p>
<p class="dim">请从官方渠道 prism.adblanlu.qzz.io 重新下载正版应用。</p>
</div></body></html>`

func StartServer(port string, dataDir string) error {
	if dataDir == "" {
		dataDir = "/data/data/com.prismtool.box/files"
	}
	os.MkdirAll(dataDir, 0755)

	hub = NewSSEHub()
	state.InitCheckpointDB(dataDir)
	config.InitConfig(dataDir)

	// 启动前先做一次性签名/包名校验。Android 已在 GoMain 之前通过 JNI 上报。
	// 判定为被篡改后，下方 HTTP 中间件会对所有请求返回拦截页。
	VerifyApkIntegrity()
	if IsApkTampered() {
		logln("[integrity] 检测到应用被篡改，已进入拦截模式: " + TamperReason())
	}

	// 从配置读取自定义 DNS 和 TLS 设置
	cfg := config.ReloadConfig()
	if cfg != nil && cfg.CustomDNS != "" {
		customDNS = cfg.CustomDNS
	}
	if cfg != nil && cfg.TLSSkipVerify != nil {
		tlsSkipVerify = *cfg.TLSSkipVerify
	}

	// 初始化播放列表管理器
	playlistManagerMu.Lock()
	playlistManager = NewPlaylistManager("@a", 1.0)
	playlistManagerMu.Unlock()

	recoverCheckpoints()

	// 启动心跳上报循环
	go startHeartbeatLoop(dataDir)

	// 启动时刷新工具箱信息（异步）
	go refreshToolboxInfo()

	go runStartupProtection()

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/css/", handleStatic("/css/", "frontend/css/"))
	mux.HandleFunc("/js/", handleStatic("/js/", "frontend/js/"))
	mux.HandleFunc("/wizard.css", func(w http.ResponseWriter, r *http.Request) {
		data, _ := frontendFS.ReadFile("frontend/wizard.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(data)
	})
	mux.HandleFunc("/wizard.js", func(w http.ResponseWriter, r *http.Request) {
		data, _ := frontendFS.ReadFile("frontend/wizard.js")
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(data)
	})
	mux.HandleFunc("/stream", handleSSE)
	mux.HandleFunc("/api/task/start", handleTaskStart)
	mux.HandleFunc("/api/repair/start", handleRepairStart)
	mux.HandleFunc("/api/task/stop", handleTaskStop)
	mux.HandleFunc("/api/task/list", handleTaskList)
	mux.HandleFunc("/api/task/resume", handleTaskResume)
	mux.HandleFunc("/api/world/enable", handleWorldEnable)
	mux.HandleFunc("/api/heartbeat/enable", handleHeartbeatEnable)
	mux.HandleFunc("/api/task/delete", handleTaskDelete)
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/bot/connect", handleBotConnect)
	mux.HandleFunc("/api/bot/disconnect", handleBotDisconnect)
	mux.HandleFunc("/api/bot/status", handleBotStatus)
	mux.HandleFunc("/api/bot/console", handleBotConsole)
	mux.HandleFunc("/api/fleet/connect", handleFleetConnect)
	mux.HandleFunc("/api/fleet/disconnect", handleFleetDisconnect)
	mux.HandleFunc("/api/fleet/status", handleFleetStatus)
	mux.HandleFunc("/api/fly/status", handleFlyStatus)
	mux.HandleFunc("/api/fly/start", handleFlyStart)
	mux.HandleFunc("/api/fly/stop", handleFlyStop)
	mux.HandleFunc("/api/fly/jump", handleFlyJump)
	mux.HandleFunc("/api/fly/jump-start", handleFlyJumpStart)
	mux.HandleFunc("/api/fly/jump-stop", handleFlyJumpStop)
	mux.HandleFunc("/api/fly/sneak-start", handleFlySneakStart)
	mux.HandleFunc("/api/fly/sneak-stop", handleFlySneakStop)
	mux.HandleFunc("/api/fly/sprint-start", handleFlySprintStart)
	mux.HandleFunc("/api/fly/sprint-stop", handleFlySprintStop)
	mux.HandleFunc("/api/fly/down", handleFlyDown)
	mux.HandleFunc("/api/fly/look", handleFlyLook)
	mux.HandleFunc("/api/fly/move", handleFlyMove)
	mux.HandleFunc("/api/fly/teleport", handleFlyTeleport)
	mux.HandleFunc("/api/fly/press", handleFlyPress)
	mux.HandleFunc("/api/fly/release", handleFlyRelease)
	mux.HandleFunc("/api/fly/position", handleFlyPosition)
	mux.HandleFunc("/api/pathfinder/goto", handlePathfinderGoto)
	mux.HandleFunc("/api/pathfinder/follow", handlePathfinderFollow)
	mux.HandleFunc("/api/pathfinder/stop", handlePathfinderStop)
	mux.HandleFunc("/api/pathfinder/log", handlePathfinderLog)
	mux.HandleFunc("/sounds/", handleSounds)
	mux.HandleFunc("/uploads/", handleUploads)
	mux.HandleFunc("/api/mapart/preview", handleMapArtPreview)
	mux.HandleFunc("/api/media/preview", handleMediaPreview)
	mux.HandleFunc("/api/file/upload", handleFileUpload)
	mux.HandleFunc("/api/building/analyze", handleBuildingAnalyze)
	mux.HandleFunc("/api/building/stats", handleBuildingStats)
	mux.HandleFunc("/api/building/voxel", handleBuildingVoxel)
	mux.HandleFunc("/api/itemmaker/generate", handleItemMakerGenerate)
	mux.HandleFunc("/api/preview/scan", handlePreviewScan)
	mux.HandleFunc("/api/preview/generate", handlePreviewGenerate)
	mux.HandleFunc("/api/preview/status", handlePreviewStatus)
	mux.HandleFunc("/api/preview/cancel", handlePreviewCancel)
	mux.HandleFunc("/api/preview/zip", handlePreviewZip)
	mux.HandleFunc("/api/preview/zip-building", handlePreviewZipBuilding)
	mux.HandleFunc("/api/preview/render-isometric", handlePreviewRenderIsometric)
	mux.HandleFunc("/api/preview/save-image", handlePreviewSaveImage)
	mux.HandleFunc("/api/preview/generate-gif", handlePreviewGenerateGif)
	mux.HandleFunc("/api/prism/login", handlePrismLogin)
	mux.HandleFunc("/api/prism/nuts", handlePrismNuts)
	mux.HandleFunc("/api/prism-proxy/", handlePrismProxy)
	mux.HandleFunc("/api/music/upload", handleMusicUpload)
	mux.HandleFunc("/api/music/play", handleMusicPlay)
	mux.HandleFunc("/api/music/stop", handleMusicStop)
	mux.HandleFunc("/api/music/pause", handleMusicPause)
	mux.HandleFunc("/api/music/resume", handleMusicResume)
	mux.HandleFunc("/api/music/status", handleMusicStatus)
	mux.HandleFunc("/api/music/note", handleMusicNote)
	mux.HandleFunc("/api/music/actionbar", handleMusicActionbar)
	mux.HandleFunc("/api/music/queue", handleMusicQueueGet)
	mux.HandleFunc("/api/music/queue/add", handleMusicQueueAdd)
	mux.HandleFunc("/api/music/queue/remove", handleMusicQueueRemove)
	mux.HandleFunc("/api/music/queue/clear", handleMusicQueueClear)
	mux.HandleFunc("/api/music/loop", handleMusicLoop)
	mux.HandleFunc("/api/music/next", handleMusicNext)
	mux.HandleFunc("/api/music/prev", handleMusicPrev)
	mux.HandleFunc("/api/version/check", handleVersionCheck)
	mux.HandleFunc("/api/announcements", handleAnnouncements)
	mux.HandleFunc("/api/integrity/check", handleIntegrityCheck)
	mux.HandleFunc("/api/files/scan", handleFileScan)
	mux.HandleFunc("/api/files/read", handleFileRead)
	mux.HandleFunc("/api/files/write", handleFileWrite)
	mux.HandleFunc("/api/map/scan", handleMapScan)
	mux.HandleFunc("/api/map/display", handleMapDisplay)
	mux.HandleFunc("/api/map/play", handleMapPlay)
	mux.HandleFunc("/api/map/stop", handleMapStop)
	mux.HandleFunc("/api/map/grid", handleMapGrid)
	mux.HandleFunc("/api/skin/build", handleSkinBuild)
	mux.HandleFunc("/api/skin/preview", handleSkinPreview)
	mux.HandleFunc("/api/skin/voxel", handleSkinVoxel)
	mux.HandleFunc("/api/skin/online-list", handleSkinOnlineList)
	mux.HandleFunc("/api/skin/online-preview", handleSkinOnlinePreview)
	mux.HandleFunc("/api/skin/online-build", handleSkinOnlineBuild)
	mux.HandleFunc("/api/skin/head", handleSkinHead)
	mux.HandleFunc("/api/skin/online-head", handleSkinOnlineHead)
	mux.HandleFunc("/api/permission/storage", handleStoragePermission)
	mux.HandleFunc("/api/toolbox/my-info", handleToolboxMyInfo)
	mux.HandleFunc("/api/toolbox/my-info/update", handleToolboxUpdate)
	mux.HandleFunc("/api/toolbox/start-trial", handleToolboxStartTrial)
	mux.HandleFunc("/api/toolbox/purchase/prompt", handleToolboxPurchasePrompt)
	mux.HandleFunc("/api/toolbox/purchase/name", handleToolboxPurchaseName)
	mux.HandleFunc("/api/toolbox/purchase/extension", handleToolboxPurchaseExtension)
	mux.HandleFunc("/api/toolbox/nuts/balance", handleToolboxNutsBalance)
	mux.HandleFunc("/api/cache/clear", handleCacheClear)
	mux.HandleFunc("/api/push/pending", handlePushPending)
	mux.HandleFunc("/api/push/ack", handlePushAck)
	mux.HandleFunc("/api/marquee/start", handleMarqueeStart)
	mux.HandleFunc("/api/marquee/stop", handleMarqueeStop)
	mux.HandleFunc("/api/marquee/status", handleMarqueeStatus)
	mux.HandleFunc("/api/marquee/preview", handleMarqueePreview)
	mux.HandleFunc("/api/players/list", handlePlayersList)
	mux.HandleFunc("/api/mcfunction/parse", handleMcfunctionParse)
	mux.HandleFunc("/api/mcfunction/execute", handleMcfunctionExecute)
	mux.HandleFunc("/api/mcfunction/stop", handleMcfunctionStop)
	mux.HandleFunc("/api/mcfunction/status", handleMcfunctionStatus)
	mux.HandleFunc("/api/mcfunction/syntax", handleMcfunctionSyntax)
	mux.HandleFunc("/mcf-syntax.mtsx", func(w http.ResponseWriter, r *http.Request) {
		data, err := frontendFS.ReadFile("frontend/mcf-syntax.mtsx")
		if err != nil {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(data)
	})

	// 命令方块工程
	mux.HandleFunc("/api/cb/export/scan", handleCBExportScan)
	mux.HandleFunc("/api/cb/export/save", handleCBExportSave)
	mux.HandleFunc("/api/cb/import/preview", handleCBImportPreview)
	mux.HandleFunc("/api/cb/import/start", handleCBImportStart)
	mux.HandleFunc("/api/cb/stats", handleCBStats)
	mux.HandleFunc("/api/cb/stats/preview", handleCBStatsPreview)

	// Camera 工具
	mux.HandleFunc("/api/camera/start", handleCameraStart)
	mux.HandleFunc("/api/camera/stop", handleCameraStop)
	mux.HandleFunc("/api/camera/status", handleCameraStatus)
	mux.HandleFunc("/api/camera/pivot", handleCameraPivot)
	mux.HandleFunc("/api/camera/shots", handleCameraShots)
	mux.HandleFunc("/api/camera/shot/update", handleCameraShotUpdate)
	mux.HandleFunc("/api/camera/shot/delete", handleCameraShotDelete)
	mux.HandleFunc("/api/camera/generate", handleCameraGenerate)
	mux.HandleFunc("/api/camera/test", handleCameraTest)
	mux.HandleFunc("/api/camera/test/all", handleCameraTestAll)
	mux.HandleFunc("/api/camera/clear", handleCameraClear)
	if handleSystemRestart != nil {
		mux.HandleFunc("/api/system/restart", handleSystemRestart)
	}

	corsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		mux.ServeHTTP(w, r)
	})

	// 篡改拦截：一旦判定为被篡改，所有请求（包括 / 首页）都返回自包含的拦截页。
	// WebView 直接显示拦截页，应用无法进入功能界面。
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IsApkTampered() {
			// 返回 200：MainActivity 的加载页只在 200/404 时跳转到 /。
			// 若返回 403，加载页既不跳转也不重试，会卡在加载动画。
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(200)
			w.Write([]byte(tamperedPageHTML))
			return
		}
		corsHandler.ServeHTTP(w, r)
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ln, err := net.Listen("tcp", "0.0.0.0:"+port)
	if err != nil {
		return err
	}

	go func() {
		<-sigCh
		logln("[shutdown] 收到退出信号，正在关闭...")

		botMgrMu.Lock()
		if botMgr != nil {
			botMgr.Disconnect()
		}
		botMgrMu.Unlock()

		taskMu.Lock()
		if activeTask != nil {
			activeTask.Stop()
		}
		taskMu.Unlock()

		state.CloseDB()
		logln("[shutdown] 清理完成，退出")
		os.Exit(0)
	}()

	return http.Serve(ln, handler)
}

func logln(msg string) {
	f, err := os.OpenFile("/tmp/prism_shutdown.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, msg)
}
