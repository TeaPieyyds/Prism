package apisrv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	g79 "github.com/Yeah114/g79client"
	"github.com/adb-lanlu/prism-oss/internal/authsvc"
	"github.com/adb-lanlu/prism-oss/internal/netproxy"
)

type TokenMode int

const (
	TokenModeCookieAsToken TokenMode = iota + 1
	TokenModeFixedToken
)

type StartConfig struct {
	TokenMode     TokenMode
	FixedToken    string
	FixedCookie   string
	CookieFile    string
	HotReloadFile bool
	Proxy         *netproxy.Manager
}

type sessionData struct {
	UserID        string
	EngineVersion string
	PatchVersion  string
	IsPC          bool
}

type sessionStore struct {
	mu   sync.RWMutex
	data map[string]sessionData
}

func newSessionStore() *sessionStore { return &sessionStore{data: map[string]sessionData{}} }
func (s *sessionStore) Put(k string, v sessionData) {
	if strings.TrimSpace(k) == "" {
		return
	}
	s.mu.Lock()
	s.data[k] = v
	s.mu.Unlock()
}
func (s *sessionStore) Get(k string) (sessionData, bool) {
	s.mu.RLock()
	v, ok := s.data[k]
	s.mu.RUnlock()
	return v, ok
}

type Server struct {
	cfg      StartConfig
	sessions *sessionStore
}

func NewServer(cfg StartConfig) *Server {
	return &Server{cfg: cfg, sessions: newSessionStore()}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/api/new", s.handleNew)

	mux.HandleFunc("/api/phoenix/login", s.handleLogin)
	mux.HandleFunc("/api/phoenix/transfer_check_num", s.handleTransferCheckNum)
	mux.HandleFunc("/api/phoenix/transfer_start_type", s.handleTransferStartType)
	mux.HandleFunc("/api/phoenix/tan_lobby_login", s.handleTanLobbyLogin)
	mux.HandleFunc("/api/phoenix/tan_lobby_create", s.handleTanLobbyCreate)
	mux.HandleFunc("/api/phoenix/tan_lobby_transfer_server", s.handleTanLobbyTransferServer)

	mux.HandleFunc("/api/huxauth/login", s.handleLogin)
	mux.HandleFunc("/api/huxauth/transfer_check_num", s.handleTransferCheckNum)
	mux.HandleFunc("/api/huxauth/transfer_start_type", s.handleTransferStartType)
	mux.HandleFunc("/api/huxauth/tan_lobby_login", s.handleTanLobbyLogin)
	mux.HandleFunc("/api/huxauth/tan_lobby_create", s.handleTanLobbyCreate)
	mux.HandleFunc("/api/huxauth/tan_lobby_transfer_server", s.handleTanLobbyTransferServer)

	return s.loggingMiddleware(mux)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": "Prism", "service": "verification", "status": "ok"})
}
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
func (s *Server) handleNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	writeText(w, http.StatusOK, mustUUID())
}

func (s *Server) authenticateByToken(ctx context.Context, reqToken string) (*g79.Client, string, error) {
	cookie := strings.TrimSpace(reqToken)
	switch s.cfg.TokenMode {
	case TokenModeCookieAsToken:
		if cookie == "" {
			return nil, "", fmt.Errorf("login_token 不能为空（cookie当token）")
		}
	case TokenModeFixedToken:
		if strings.TrimSpace(s.cfg.FixedToken) == "" {
			return nil, "", fmt.Errorf("服务端固定token未配置")
		}
		if cookie != strings.TrimSpace(s.cfg.FixedToken) {
			return nil, "", fmt.Errorf("login_token 无效")
		}
		fixedCookie := strings.TrimSpace(s.cfg.FixedCookie)
		if s.cfg.HotReloadFile && strings.TrimSpace(s.cfg.CookieFile) != "" {
			if fileCookie, err := readCookieFromFile(s.cfg.CookieFile); err == nil {
				fixedCookie = fileCookie
			}
		}
		if fixedCookie == "" {
			return nil, "", fmt.Errorf("服务端固定cookie未配置")
		}
		cookie = fixedCookie
	default:
		return nil, "", fmt.Errorf("未知 token 模式")
	}
	cli, err := authsvc.AuthenticateWithCookie(ctx, cookie, s.cfg.Proxy)
	if err != nil {
		return nil, "", err
	}
	return cli, cookie, nil
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	authKey := authorizationSessionKey(r)
	if authKey == "" {
		writeJSON(w, http.StatusOK, LoginResponse{SuccessStates: false, Message: Message{Information: "Authorization header missing Bearer token"}})
		return
	}

	var req LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusOK, LoginResponse{SuccessStates: false, Message: Message{Information: fmt.Sprintf("Login: 绑定请求体时出现问题, 原因是 %v", err)}})
		return
	}

	cli, cookie, err := s.authenticateByToken(r.Context(), req.FBToken)
	if err != nil {
		writeJSON(w, http.StatusOK, LoginResponse{SuccessStates: false, Message: Message{Information: fmt.Sprintf("Login: 使用 Cookie 认证时出现问题, 原因是 %v", err)}})
		return
	}

	provider := func(ctx context.Context) (*g79.Client, error) {
		c, err := authsvc.NewClient(ctx, s.cfg.Proxy)
		if err != nil {
			return nil, err
		}
		if err := c.G79AuthenticateWithCookie(cookie); err != nil {
			return nil, err
		}
		return c, nil
	}

	loginRes, err := authsvc.Login(r.Context(), cli, authsvc.LoginParams{
		ServerCode:      req.ServerCode,
		ServerPassword:  req.ServerPassword,
		ClientPublicKey: req.ClientPublicKey,
	}, provider)
	if err != nil {
		writeJSON(w, http.StatusOK, LoginResponse{SuccessStates: false, Message: Message{Information: fmt.Sprintf("Login: 登录到租赁服时出现问题, 原因是 %v", err)}})
		return
	}

	skinInfo, err := authsvc.GetSkinInfo(cli)
	if err != nil {
		writeJSON(w, http.StatusOK, LoginResponse{SuccessStates: false, Message: Message{Information: fmt.Sprintf("Login: 获取皮肤信息时出现问题, 原因是 %v", err)}})
		return
	}

	s.sessions.Put(authKey, sessionData{UserID: loginRes.UID, EngineVersion: loginRes.EngineVersion, PatchVersion: loginRes.PatchVersion, IsPC: loginRes.IsPC})
	resp := LoginResponse{
		SuccessStates:  true,
		Message:        Message{Information: "ok"},
		BotLevel:       loginRes.BotLevel,
		BotSkin:        SkinInfo{ItemID: skinInfo.ItemID, SkinDownloadURL: skinInfo.SkinDownloadURL, SkinIsSlim: skinInfo.SkinIsSlim},
		BotComponent:   loginRes.BotComponent,
		FBToken:        req.FBToken,
		MasterName:     loginRes.MasterName,
		RentalServerIP: loginRes.IP,
		ChainInfo:      loginRes.ChainInfo,
	}
	if loginRes.TanLobby != nil {
		// 本地联机（TAN）房间：返回 raknet/signaling 地址与密钥，
		// 客户端无需再调用 /api/phoenix/tan_lobby_login。
		t := loginRes.TanLobby
		resp.RaknetServerAddress = t.RaknetServerAddress
		resp.SignalingServerAddress = t.SignalingServerAddress
		resp.RaknetRand = fmt.Sprintf("%x", t.RaknetRand)
		resp.RaknetAESRand = fmt.Sprintf("%x", t.RaknetAESRand)
		resp.EncryptKeyBytes = fmt.Sprintf("%x", t.EncryptKeyBytes)
		resp.DecryptKeyBytes = fmt.Sprintf("%x", t.DecryptKeyBytes)
		resp.SignalingSeed = fmt.Sprintf("%x", t.SignalingSeed)
		resp.SignalingTicket = fmt.Sprintf("%x", t.SignalingTicket)
		resp.RoomModDisplayName = t.RoomModDisplayName
		resp.RoomModDownloadURL = t.RoomModDownloadURL
		resp.RoomOwnerID = t.RoomOwnerID
		resp.UserUniqueID = t.UserUniqueID
		resp.BotLevel = t.BotLevel
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleTransferCheckNum(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req TransferCheckNumRequest
	if err := decodeJSON(r, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	engineVersion := req.EngineVersion
	patchVersion := req.PatchVersion
	isPC := false
	if req.IsPC != nil {
		isPC = *req.IsPC
	}
	if authKey := authorizationSessionKey(r); authKey != "" {
		if sess, ok := s.sessions.Get(authKey); ok {
			if engineVersion == "" {
				engineVersion = sess.EngineVersion
			}
			if patchVersion == "" {
				patchVersion = sess.PatchVersion
			}
			if req.IsPC == nil {
				isPC = sess.IsPC
			}
		}
	}
	value, err := authsvc.TransferCheckNum(r.Context(), isPC, req.Data, engineVersion, patchVersion)
	if err != nil {
		writeJSON(w, http.StatusOK, TransferCheckNumResponse{Success: false, Message: "TransferCheckNum: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, TransferCheckNumResponse{Success: true, Message: "ok", Value: value})
}

func (s *Server) handleTransferStartType(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	content := strings.TrimSpace(r.URL.Query().Get("content"))
	if content == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	authKey := authorizationSessionKey(r)
	if authKey == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sess, ok := s.sessions.Get(authKey)
	if !ok || strings.TrimSpace(sess.UserID) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	enc, err := authsvc.TransferStartType(sess.UserID, content)
	if err != nil {
		writeJSON(w, http.StatusOK, TransferStartTypeResponse{Success: false, Message: "TransferStartType: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, TransferStartTypeResponse{Success: true, Message: "ok", Data: enc})
}

func (s *Server) handleTanLobbyLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req TanLobbyLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusOK, TanLobbyLoginResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyLogin: 绑定请求体时出现问题, 原因是 %v", err)})
		return
	}
	cli, _, err := s.authenticateByToken(r.Context(), req.FBToken)
	if err != nil {
		writeJSON(w, http.StatusOK, TanLobbyLoginResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyLogin: 使用 Cookie 认证时出现问题, 原因是 %v", err)})
		return
	}
	loginRes, err := authsvc.TanLobbyLogin(r.Context(), cli, authsvc.TanLobbyLoginParams{RoomID: req.RoomID})
	if err != nil {
		writeJSON(w, http.StatusOK, TanLobbyLoginResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyLogin: %v", err)})
		return
	}
	skinInfo, err := authsvc.GetSkinInfo(cli)
	if err != nil {
		writeJSON(w, http.StatusOK, TanLobbyLoginResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyLogin: 获取皮肤信息时出现问题, 原因是 %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, TanLobbyLoginResponse{
		Success:                true,
		ErrorInfo:              "",
		UserUniqueID:           loginRes.UserUniqueID,
		UserPlayerName:         loginRes.UserPlayerName,
		BotLevel:               loginRes.BotLevel,
		BotSkin:                SkinInfo{ItemID: skinInfo.ItemID, SkinDownloadURL: skinInfo.SkinDownloadURL, SkinIsSlim: skinInfo.SkinIsSlim},
		BotComponent:           loginRes.BotComponent,
		RoomOwnerID:            loginRes.RoomOwnerID,
		RoomModDisplayName:     loginRes.RoomModDisplayName,
		RoomModDownloadURL:     loginRes.RoomModDownloadURL,
		RoomModEncryptKey:      loginRes.RoomModEncryptKey,
		RaknetServerAddress:    loginRes.RaknetServerAddress,
		RaknetRand:             loginRes.RaknetRand,
		RaknetAESRand:          loginRes.RaknetAESRand,
		EncryptKeyBytes:        loginRes.EncryptKeyBytes,
		DecryptKeyBytes:        loginRes.DecryptKeyBytes,
		SignalingServerAddress: loginRes.SignalingServerAddress,
		SignalingSeed:          loginRes.SignalingSeed,
		SignalingTicket:        loginRes.SignalingTicket,
	})
}

func (s *Server) handleTanLobbyCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req TanLobbyCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusOK, TanLobbyCreateResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyCreate: 绑定请求体时出现问题, 原因是 %v", err)})
		return
	}
	cli, _, err := s.authenticateByToken(r.Context(), req.FBToken)
	if err != nil {
		writeJSON(w, http.StatusOK, TanLobbyCreateResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyCreate: 使用 Cookie 认证时出现问题, 原因是 %v", err)})
		return
	}
	createRes, err := authsvc.TanLobbyCreate(r.Context(), cli)
	if err != nil {
		writeJSON(w, http.StatusOK, TanLobbyCreateResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyCreate: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, TanLobbyCreateResponse{
		Success:                true,
		ErrorInfo:              "",
		UserUniqueID:           createRes.UserUniqueID,
		UserPlayerName:         createRes.UserPlayerName,
		RaknetServerAddress:    createRes.RaknetServerAddress,
		RaknetRand:             createRes.RaknetRand,
		RaknetAESRand:          createRes.RaknetAESRand,
		EncryptKeyBytes:        createRes.EncryptKeyBytes,
		DecryptKeyBytes:        createRes.DecryptKeyBytes,
		SignalingServerAddress: createRes.SignalingServerAddress,
		SignalingSeed:          createRes.SignalingSeed,
		SignalingTicket:        createRes.SignalingTicket,
	})
}

func (s *Server) handleTanLobbyTransferServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	raknetServers, websocketServers, err := authsvc.TransferServerList()
	if err != nil {
		writeJSON(w, http.StatusOK, TanLobbyTransferServersResponse{Success: false, ErrorInfo: fmt.Sprintf("TransferServerList: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, TanLobbyTransferServersResponse{Success: true, ErrorInfo: "", RaknetServers: raknetServers, WebsocketServers: websocketServers})
}

func readCookieFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(data))
	if v == "" {
		return "", fmt.Errorf("cookie file empty")
	}
	return v, nil
}

func Start9191(cfg StartConfig) error {
	srv := NewServer(cfg)
	httpServer := &http.Server{Addr: ":9191", Handler: srv.Routes(), ReadHeaderTimeout: 15 * time.Second}
	log.Printf("[Prism] 验证服务已启动: http://0.0.0.0:9191")
	return httpServer.ListenAndServe()
}

func authorizationSessionKey(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return ""
	}
	if len(auth) >= 7 && strings.EqualFold(auth[:7], "Bearer ") {
		auth = strings.TrimSpace(auth[7:])
	}
	return auth
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	return decoder.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeText(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(text))
}
func writeMethodNotAllowed(w http.ResponseWriter) {
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func mustUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).String())
	})
}
