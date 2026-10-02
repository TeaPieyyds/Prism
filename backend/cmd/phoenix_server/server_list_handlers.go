package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	g79 "github.com/Yeah114/g79client"
	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
)

// G79 client cache — keyed by account ID, 1 minute TTL
type cachedClient struct {
	client  *g79.Client
	expires time.Time
}

// 跳过心跳的账号来源（只有手机号和邮箱登录的不需要心跳）
var skipHeartbeatSources = map[string]bool{
	"phone": true,
	"email": true,
}

var (
	g79Cache   = make(map[int64]*cachedClient)
	g79CacheMu sync.RWMutex

	// heartbeatStops 跟踪已启动心跳的账号 (accountID → stop func)
	heartbeatStops  = map[int64]func(){}
	heartbeatStopMu sync.Mutex

	// heartbeatClients 跟踪心跳账号的客户端，供批量操作复用
	heartbeatClients  = map[int64]*g79.Client{}
	heartbeatClientMu sync.RWMutex

	accountRotateMu   sync.Mutex
	accountRotateUsed = map[int64][]int64{}

	lobbySearchLimiter = auth.NewRateLimiter(1, 5*time.Second)
	lobbyListLimiter   = auth.NewRateLimiter(1, 5*time.Second)
)

// startAccountHeartbeat 检查账号来源，对 guest/cookie 或共享账号启动心跳
func startAccountHeartbeat(accID int64, c *g79.Client) {
	acc, err := db.GetAccountByID(accID)
	if err != nil || acc.Disabled || acc.IsServerOwner {
		return
	}
	if skipHeartbeatSources[acc.Source] && !acc.AutoRefreshEnabled {
		return
	}

	heartbeatStopMu.Lock()
	if oldStop, running := heartbeatStops[accID]; running {
		delete(heartbeatStops, accID)
		heartbeatStopMu.Unlock()
		oldStop()
		log.Printf("[HB] replaced heartbeat for account #%d (%s, source=%s)", accID, acc.DisplayName, acc.Source)
	} else {
		heartbeatStopMu.Unlock()
	}

	stop := c.StartOnlineHeartbeat(g79.HeartbeatConfig{
		DrpfOpts: g79.DrpfPingOptions{
			Type:        "user_log",
			OperateType: "",
			SubType:     "",
			MainType:    "",
		},
		OnError: func(err error) {
			log.Printf("[HB] account #%d (%s): %v", accID, acc.DisplayName, err)
			// code 32 = 该代理出口IP被网易封，移除代理并自动切换
			if err != nil {
				log.Printf("[HB-ERR] err=%q", err.Error())
				if strings.Contains(err.Error(), "code: 32") {
					log.Printf("[HB-ERR] 检测到code 32，触发代理切换")
					if nc := ReportAccountCode32(accID); nc != nil {
						// 用新代理的 HTTP client 重建 g79 会话并重新认证，重启心跳，
						// 避免心跳继续挂在旧代理上空转反复触发 code32。
						if c2, cErr := g79.NewClientWithHTTPClient(nc); cErr == nil {
							if aErr := c2.G79AuthenticateWithCookie(acc.CookieData); aErr == nil {
								log.Printf("[HB-ERR] 更换代理后重新认证成功，重启心跳 #%d", accID)
								startAccountHeartbeat(accID, c2)
							}
						}
					}
				}
				// 超时：代理被网易慢速风控(healthcheck 通但实际 API 慢)。
				// 转移该账号并按超时标记处理(10分钟重测)，避免在慢代理上反复超时。
				lower := strings.ToLower(err.Error())
				if strings.Contains(lower, "timeout") || strings.Contains(lower, "context deadline exceeded") || strings.Contains(lower, "i/o timeout") {
					ReportAccountProxyTimeout(accID)
				}
			}
		},
	})

	// 定期刷新账号数据到数据库（每5分钟更新成长等级等）
	refreshStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				acc, aErr := db.GetAccountByID(accID)
				if aErr != nil || acc.Disabled {
					continue
				}
				info := db.AccountInfo{
					DisplayName:    acc.DisplayName,
					UID:            acc.UID,
					Status:         acc.Status,
					GrowthLevel:    acc.GrowthLevel,
					Score:          acc.Score,
					SkinNumber:     acc.SkinNumber,
					CapeNumber:     acc.CapeNumber,
					AvatarImageURL: acc.AvatarImageURL,
					IsVip:          acc.IsVip,
					Source:         acc.Source,
				}
				fillAccountInfoFromClient(c, &info)
				if info.GrowthLevel == "" || info.GrowthLevel == "0" {
					continue
				}
				if info.GrowthLevel != acc.GrowthLevel || info.GrowthExp != acc.GrowthExp {
					log.Printf("[HB] account #%d (%s): level %s -> %s", accID, acc.DisplayName, acc.GrowthLevel, info.GrowthLevel)
				}
				db.UpdateAccountFull(accID, info)
			case <-refreshStop:
				return
			}
		}
	}()

	// token 自动刷新（30分钟）
	tokenRefreshStop := c.StartAutoRefresh(30 * time.Minute)

	// 合并 stop 函数
	origStop := stop
	stop = func() {
		tokenRefreshStop()
		close(refreshStop)
		origStop()
	}

	heartbeatStopMu.Lock()
	heartbeatStops[accID] = stop
	heartbeatStopMu.Unlock()
	heartbeatClientMu.Lock()
	heartbeatClients[accID] = c
	heartbeatClientMu.Unlock()
	log.Printf("[HB] started heartbeat for account #%d (%s, source=%s)", accID, acc.DisplayName, acc.Source)
}

// stopAccountHeartbeat 停止指定账号的心跳
func stopAccountHeartbeat(accID int64) {
	heartbeatClientMu.Lock()
	delete(heartbeatClients, accID)
	heartbeatClientMu.Unlock()
	heartbeatStopMu.Lock()
	stop, ok := heartbeatStops[accID]
	if ok {
		delete(heartbeatStops, accID)
	}
	heartbeatStopMu.Unlock()
	if ok && stop != nil {
		stop()
		log.Printf("[HB] stopped heartbeat for account #%d", accID)
	}
}

// getHeartbeatClient 返回心跳客户端，供批量操作复用
func getHeartbeatClient(accID int64) *g79.Client {
	heartbeatClientMu.RLock()
	defer heartbeatClientMu.RUnlock()
	return heartbeatClients[accID]
}

func lobbyLimitIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(requestIP(r))
	if err != nil {
		return requestIP(r)
	}
	return ip
}

// StartG79CacheCleanup periodically removes expired entries from g79Cache
func StartG79CacheCleanup() {
	go func() {
		for range time.NewTicker(5 * time.Minute).C {
			g79CacheMu.Lock()
			now := time.Now()
			for k, v := range g79Cache {
				if now.After(v.expires) {
					delete(g79Cache, k)
				}
			}
			g79CacheMu.Unlock()
		}
	}()
}

// startHeartbeatLogsCleanup 定期清理超过 30 天的心跳日志，防止无限增长撑爆磁盘/内存。
func startHeartbeatLogsCleanup() {
	go func() {
		for range time.NewTicker(1 * time.Hour).C {
			cutoff := time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339)
			if _, err := db.DB.Exec(`DELETE FROM heartbeat_logs WHERE created_at < ?`, cutoff); err != nil {
				log.Printf("[HB] 清理心跳日志失败: %v", err)
			}
		}
	}()
}

// StartAllHeartbeats 启动所有符合条件的账号的心跳（guest/cookie/共享）
func StartAllHeartbeats() {
	accounts, _ := db.GetAccountsExcludeSource("phone", "email")
	// 也包含勾选了自动刷新的 phone/email 账号
	extra, _ := db.GetAccountsWithAutoRefresh()
	accounts = append(accounts, extra...)
	for _, acc := range accounts {
		if acc.Disabled || acc.CookieData == "" || acc.IsServerOwner {
			continue
		}
		func(accID int64, cookie string, name string) {
			c, err := newG79ClientWithProxy(accID)
			if err != nil {
				log.Printf("[HB] account #%d (%s): NewClient失败: %v", accID, name, err)
				return
			}
			if err := c.G79AuthenticateWithCookie(cookie); err != nil {
				log.Printf("[HB] account #%d (%s): 认证失败: %v", accID, name, err)
				if strings.Contains(err.Error(), "code: 32") {
					log.Printf("[HB-ERR] code 32，移除代理并切换")
					ReportAccountCode32(accID)
				}
				lower := strings.ToLower(err.Error())
				if strings.Contains(lower, "timeout") || strings.Contains(lower, "context deadline exceeded") || strings.Contains(lower, "i/o timeout") {
					ReportAccountProxyTimeout(accID)
				}
				return
			}
			startAccountHeartbeat(accID, c)
		}(acc.ID, acc.CookieData, acc.DisplayName)
	}
}

// getCachedClient returns a valid cached client for the account, or nil
func getCachedClient(accID int64) *g79.Client {
	g79CacheMu.RLock()
	defer g79CacheMu.RUnlock()
	if cached, ok := g79Cache[accID]; ok && time.Now().Before(cached.expires) {
		return cached.client
	}
	return nil
}

// setCachedClient stores an authenticated client with a 25-minute TTL
// and starts online heartbeat for guest/cookie accounts.
func setCachedClient(accID int64, client *g79.Client) {
	g79CacheMu.Lock()
	g79Cache[accID] = &cachedClient{client: client, expires: time.Now().Add(25 * time.Minute)}
	g79CacheMu.Unlock()
	// 服主账号不启动心跳
	acc, err := db.GetAccountByID(accID)
	if err == nil && !acc.IsServerOwner {
		startAccountHeartbeat(accID, client)
	}
}

// invalidateG79Cache removes a cached client for the given account
func invalidateG79Cache(accID int64) {
	g79CacheMu.Lock()
	delete(g79Cache, accID)
	g79CacheMu.Unlock()
	stopAccountHeartbeat(accID)
}

// newG79ClientWithProxy 用账号绑定的代理创建 g79 客户端，没有代理就直连
func newG79ClientWithProxy(accountID int64) (*g79.Client, error) {
	hc := AssignProxy(accountID)
	if pu := accountProxy[accountID]; pu != "" {
		log.Printf("[G79CLIENT] #%d proxy=%s", accountID, pu)
	} else {
		log.Printf("[G79CLIENT] #%d DIRECT (accountProxy empty)", accountID)
	}
	c, err := g79.NewClientWithHTTPClient(hc)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// friendlyProxyError 把代理相关的错误转成面向用户的友好提示
func friendlyProxyError(err error) error {
	if err == nil {
		return nil
	}
	s := err.Error()
	lower := strings.ToLower(s)
	// 网易封IP（code 32）
	if strings.Contains(s, "code: 32") || strings.Contains(s, "code: 2100") || strings.Contains(s, "服务器维护中") {
		return fmt.Errorf("服务被网易达斯了 联系管理员更换代理")
	}
	// 超时：代理活着但响应慢/卡住
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "context deadline exceeded") || strings.Contains(lower, "i/o timeout") {
		return fmt.Errorf("呀，糟糕，代理在偷懒X﹏X 请联系管理员")
	}
	// 连不上：代理节点不可达
	if strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "no route to host") ||
		strings.Contains(lower, "cannot connect") ||
		strings.Contains(lower, "proxy connect") ||
		strings.Contains(lower, "connection failed") ||
		strings.Contains(lower, "network is unreachable") ||
		strings.Contains(lower, "connection reset") {
		return fmt.Errorf("咦 代理似乎消失了 稍后试试看(o_ _)ﾉ 仍然失败联系管理员")
	}
	return err
}

func getG79Client(r *http.Request) (*g79.Client, *db.User, error) {
	// code32 切换代理的最大尝试次数：出口封控多为临时，多试几个干净出口即可恢复。
	const maxProxySwitchAttempts = 5
	token := ""
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
		token = strings.TrimPrefix(a, "Bearer ")
	}
	if token == "" {
		token = r.URL.Query().Get("login_token")
	}
	// 占位符处理：非 adb// 开头、非 cookie JSON 的当做未设置
	if token != "" && !strings.HasPrefix(token, "adb//") && !strings.HasPrefix(token, "{\"sauth_json") && !strings.HasPrefix(token, "{\"gameid") {
		token = ""
	}
	if token == "" {
		// Guest mode: use guest pool
		ga := getGuestAccount()
		if ga == nil {
			return nil, nil, fmt.Errorf("暂无可用共享账号，请稍后再试")
		}
		// Try cache
		g79CacheMu.RLock()
		cached, ok := g79Cache[ga.ID]
		g79CacheMu.RUnlock()
		if ok && time.Now().Before(cached.expires) {
			gu := auth.GuestUser()
			gu.ActiveAccountID = &ga.ID
			return cached.client, gu, nil
		}
		// Try up to 3 different accounts in case of auth failure
		for i := 0; i < 3; i++ {
			c, err := newG79ClientWithProxy(ga.ID)
			if err != nil {
				return nil, nil, fmt.Errorf("忙不过来了 等会试试")
			}
			if err := c.G79AuthenticateWithCookie(ga.CookieData); err != nil {
				log.Printf("[GUEST] #%d auth failed (attempt %d/3): %v", ga.ID, i+1, err)
				ga = forceRotateGuestAccount()
				if ga == nil {
					return nil, nil, fmt.Errorf("暂无可用共享账号，请稍后再试")
				}
				continue
			}
			// Auth succeeded, cache and return
			g79CacheMu.Lock()
			g79Cache[ga.ID] = &cachedClient{client: c, expires: time.Now().Add(25 * time.Minute)}
			g79CacheMu.Unlock()
			startAccountHeartbeat(ga.ID, c)
			gu := auth.GuestUser()
			gu.ActiveAccountID = &ga.ID
			return c, gu, nil
		}
		return nil, nil, fmt.Errorf("共享账号认证失败")
	}
	user, err := db.GetUserByToken(token)
	if err != nil || user.ActiveAccountID == nil {
		return nil, nil, fmt.Errorf("凭据无效或未选择活跃账号")
	}

	accID := *user.ActiveAccountID

	// Check cache
	g79CacheMu.RLock()
	cached, ok := g79Cache[accID]
	g79CacheMu.RUnlock()
	if ok && time.Now().Before(cached.expires) {
		return cached.client, user, nil
	}

	// Create new client
	acc, err := db.GetAccountByID(accID)
	if err != nil {
		return nil, nil, fmt.Errorf("活跃的游玩账号已失效，请重新选择")
	}
	c, err := newG79ClientWithProxy(accID)
	if err != nil {
		return nil, nil, fmt.Errorf("忙不过来了 等会试试")
	}
	if err := c.G79AuthenticateWithCookie(acc.CookieData); err != nil {
		errStr := err.Error()
		log.Printf("[G79CLIENT] #%d auth failed: %s", accID, errStr)
		// 超时 → 按超时标记处理（与 code32 独立，标记后每 10 分钟重测恢复）
		if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "Timeout") {
			ReportAccountProxyTimeout(accID)
		}
		// code 32 = 该代理出口IP被网易封控（多为临时，12h/24h 自动解除）。
		// 切换代理本身即时；若新代理仍 code32 则继续切下一个干净出口重试，
		// 不再固定 sleep 5s×2 死等，尽快落到能用的出口。
		if strings.Contains(errStr, "code: 32") {
			for attempt := 0; attempt < maxProxySwitchAttempts; attempt++ {
				nc := ReportAccountCode32(accID)
				if nc == nil {
					break
				}
				c2, cErr := g79.NewClientWithHTTPClient(nc)
				if cErr != nil {
					break
				}
				c = c2
				if aErr := c.G79AuthenticateWithCookie(acc.CookieData); aErr == nil {
					goto authOk
				}
				log.Printf("[G79CLIENT] #%d code 32 切换后仍失败 (尝试 %d/%d)，继续切下一个", accID, attempt+1, maxProxySwitchAttempts)
			}
			return nil, nil, friendlyProxyError(fmt.Errorf("%s (IP/服务器临时限制，请稍后再试)", errStr))
		}
		// 服务器临时不可用（code 2100）重试，不切换账号
		if strings.Contains(errStr, "code: 2100") {
			time.Sleep(5 * time.Second)
			if retryErr := c.G79AuthenticateWithCookie(acc.CookieData); retryErr == nil {
				goto authOk
			}
			time.Sleep(5 * time.Second)
			if retryErr2 := c.G79AuthenticateWithCookie(acc.CookieData); retryErr2 == nil {
				goto authOk
			}
			// 临时错误重试后仍失败：不切换账号，直接报错
			return nil, nil, friendlyProxyError(fmt.Errorf("%s (服务器临时不可用，请稍后再试)", errStr))
		}
		// 仅永久封禁（code 29）才自动切换账号；其他错误不切换
		if !bannedByMessage(errStr) {
			return nil, nil, friendlyProxyError(fmt.Errorf("%s", errStr))
		}
		userAccs, _ := db.GetUserAccounts(user.ID)
		for _, candidate := range userAccs {
			if candidate.ID == accID {
				continue
			}
			if candidate.Disabled || candidate.CookieData == "" || candidate.IsServerOwner {
				continue
			}
			if candidate.OwnerID != nil && *candidate.OwnerID != user.ID {
				continue
			}
			cc, cErr := g79.NewClient()
			if cErr != nil {
				continue
			}
			if cErr := cc.G79AuthenticateWithCookie(candidate.CookieData); cErr != nil {
				continue
			}
			db.SetActiveAccount(user.ID, candidate.ID)
			g79CacheMu.Lock()
			g79Cache[candidate.ID] = &cachedClient{client: cc, expires: time.Now().Add(25 * time.Minute)}
			g79CacheMu.Unlock()
			log.Printf("[ACCT] 搜索: 账号 %s(%d) 凭证失效，已切换到 %s(%d)", acc.DisplayName, accID, candidate.DisplayName, candidate.ID)
			startAccountHeartbeat(candidate.ID, cc)
			return cc, user, nil
		}
		// 其他账号都不行，再给原账号一次重试机会（避免临时网络波动误判）
		log.Printf("[G79CLIENT] #%d 其他账号均不可用，重试原账号", accID)
		time.Sleep(3 * time.Second)
		if retryErr := c.G79AuthenticateWithCookie(acc.CookieData); retryErr == nil {
			goto authOk
		}
		return nil, nil, friendlyProxyError(fmt.Errorf("%s (凭据已过期，且无可用备用账号)", errStr))
	}

authOk:
	// Store in cache
	g79CacheMu.Lock()
	g79Cache[accID] = &cachedClient{client: c, expires: time.Now().Add(25 * time.Minute)}
	g79CacheMu.Unlock()
	startAccountHeartbeat(accID, c)

	return c, user, nil
}

// GET /api/phoenix/server/list?sort_type=1&order_type=0&offset=0&login_token=adb//xxx
func handleServerList(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, user, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	accID := *user.ActiveAccountID
	sortType, _ := strconv.Atoi(r.URL.Query().Get("sort_type"))
	orderType, _ := strconv.Atoi(r.URL.Query().Get("order_type"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	resp, err := c.GetAvailableRentalServers(sortType, orderType, offset)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if resp.Code != 0 {
		invalidateG79Cache(accID)
		c2, _, err2 := getG79Client(r)
		if err2 == nil {
			resp, err = c2.GetAvailableRentalServers(sortType, orderType, offset)
			if err != nil {
				jsonResp(w, M{"ok": false, "error": err.Error()})
				return
			}
		}
	}
	jsonResp(w, M{"ok": true, "total": resp.Total, "code": resp.Code, "servers": resp.Entities})
}

// GET /api/phoenix/server/search?keyword=xxxx&offset=0&login_token=adb//xxx
func handleServerSearch(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, user, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	accID := *user.ActiveAccountID
	keyword := r.URL.Query().Get("keyword")
	if keyword == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 keyword"})
		return
	}
	// Call search API directly (g79client uses wrong URL)
	api := "/rental-server/query/search-by-name"
	body := fmt.Sprintf(`{"server_name":"%s"}`, keyword)
	jsonData := []byte(body)
	searchOnce := func(cl *g79.Client) (int, []g79.RentalServerEntity) {
		req, _ := http.NewRequest("POST", cl.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("User-Agent", "WPFLauncher/0.0.0.0")
		req.Header.Set("user-id", cl.UserID)
		req.Header.Set("user-token", g79.CalculateDynamicToken(api, string(jsonData), cl.UserToken))
		resp, _ := c.HTTPClient().Do(req)
		if resp == nil {
			return -1, nil
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		var sr struct {
			Code     int                      `json:"code"`
			Entities []g79.RentalServerEntity `json:"entities"`
		}
		json.Unmarshal(data, &sr)
		return sr.Code, sr.Entities
	}
	code, entities := searchOnce(c)
	if code != 0 {
		invalidateG79Cache(accID)
		if c2, _, err2 := getG79Client(r); err2 == nil {
			code, entities = searchOnce(c2)
		}
	}
	jsonResp(w, M{"ok": true, "code": code, "servers": entities})
}

// GET /api/phoenix/server/detail?server_id=xxx&login_token=adb//xxx
func handleServerDetail(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	sid := r.URL.Query().Get("server_id")
	if sid == "" {
		jsonResp(w, M{"ok": false, "error": msg.Get("missing_id", "糟糕,缺少必要的服务器信息,请刷新后重试~")})
		return
	}
	resp, err := c.GetRentalServerDetails(sid)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "server": resp.Entity})
}

// GET /api/phoenix/server/players?server_id=xxx&offset=0&length=20&login_token=adb//xxx
func handleServerPlayers(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	sid := r.URL.Query().Get("server_id")
	if sid == "" {
		jsonResp(w, M{"ok": false, "error": msg.Get("missing_id", "糟糕,缺少必要的服务器信息,请刷新后重试~")})
		return
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	length, _ := strconv.Atoi(r.URL.Query().Get("length"))
	if length <= 0 || length > 50 {
		length = 20
	}

	api := "/rental-server-player/query/search-by-server"
	body := fmt.Sprintf(`{"status":0,"server_id":"%s","order_type":0,"is_online":true,"length":%d,"offset":%d}`, sid, length, offset)
	jsonData := []byte(body)

	req, _ := http.NewRequest("POST", c.ReleaseJSON.ApiGatewayUrl+api, strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "WPFLauncher/0.0.0.0")
	req.Header.Set("user-id", c.UserID)
	req.Header.Set("user-token", g79.CalculateDynamicToken(api, string(jsonData), c.UserToken))

	resp2, err := c.HTTPClient().Do(req)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "咦 代理似乎消失了 稍后试试看(o_ _)ﾉ 仍然失败联系管理员"})
		return
	}
	defer resp2.Body.Close()
	data, _ := io.ReadAll(resp2.Body)

	var result struct {
		Code     int             `json:"code"`
		Total    json.RawMessage `json:"total"`
		Entities []struct {
			EntityID  string          `json:"entity_id"`
			Name      string          `json:"name"`
			UserID    json.RawMessage `json:"user_id"`
			IsOnline  bool            `json:"is_online"`
			HeadImage string          `json:"headImage"`
			FrameID   string          `json:"frame_id"`
			PeGrowth  struct {
				Lv    int  `json:"lv"`
				Exp   int  `json:"exp"`
				IsVip bool `json:"is_vip"`
			} `json:"pe_growth"`
		} `json:"entities"`
	}
	json.Unmarshal(data, &result)

	type playerInfo struct {
		Name      string `json:"name"`
		UserID    string `json:"user_id"`
		Level     int    `json:"level"`
		IsOnline  bool   `json:"is_online"`
		HeadImage string `json:"head_image"`
		FrameID   string `json:"frame_id"`
	}
	var players []playerInfo
	for _, p := range result.Entities {
		players = append(players, playerInfo{
			Name: p.Name, UserID: string(p.UserID),
			Level: p.PeGrowth.Lv, IsOnline: p.IsOnline,
			HeadImage: p.HeadImage, FrameID: p.FrameID,
		})
	}
	jsonResp(w, M{"ok": true, "code": result.Code, "total": string(result.Total), "players": players})
}

// GET /api/phoenix/server/owner?uid=xxx&login_token=adb//xxx
func handleServerOwner(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, user, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	uid := r.URL.Query().Get("uid")
	if uid == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 uid"})
		return
	}
	resp, err := c.GetOtherUserDetail(uid, false)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if resp.Code != 0 {
		invalidateG79Cache(*user.ActiveAccountID)
		if c2, _, err2 := getG79Client(r); err2 == nil {
			resp, err = c2.GetOtherUserDetail(uid, false)
			if err != nil {
				jsonResp(w, M{"ok": false, "error": err.Error()})
				return
			}
		}
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "user": resp.Entity})
}

// GET /api/phoenix/social/search?keyword=xxx&login_token=adb//xxx
func handleSocialSearch(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	keyword := r.URL.Query().Get("keyword")
	if keyword == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 keyword"})
		return
	}
	resp, err := c.SearchUserByNameOrMail(keyword, 1, 10)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "total": len(resp.Entities), "users": resp.Entities})
}

// GET /api/phoenix/social/requests?login_token=adb//xxx
func handleSocialRequests(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	resp, err := c.GetAddMeFriends()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "total": len(resp.Entities), "requests": resp.Entities})
}

// GET /api/phoenix/store/search?keyword=xxx&login_token=adb//xxx
func handleStoreSearch(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	keyword := r.URL.Query().Get("keyword")
	ids := []string{}
	if keyword != "" {
		ids = append(ids, keyword)
	}
	resp, err := c.SearchMcGameItems(ids)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "商城 API 仅限游戏客户端内访问: " + err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "total": resp.Total, "items": resp.Entities})
}

// GET /api/phoenix/lobby/search?keyword=xxx&login_token=adb//xxx
func handleLobbySearch(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	if !lobbySearchLimiter.Allow(lobbyLimitIP(r)) {
		jsonResp(w, M{"ok": false, "error": "搜索过于频繁，请5秒后再试"})
		return
	}
	c, user, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	keyword := r.URL.Query().Get("keyword")
	if keyword == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 keyword"})
		return
	}
	resp, err := c.SearchOnlineLobbyRoomByKeyword(keyword, 10, 0)
	if err != nil {
		invalidateG79Cache(*user.ActiveAccountID)
		if c2, _, err2 := getG79Client(r); err2 == nil {
			resp, err = c2.SearchOnlineLobbyRoomByKeyword(keyword, 10, 0)
		}
	}
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "total": resp.Total, "rooms": resp.Entities})
}

// POST /api/phoenix/skin/change — 更换皮肤，body: {"item_id":"xxx"}
func handleSkinChange(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	var req struct {
		ItemID    string `json:"item_id"`
		AccountID *int64 `json:"account_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.ItemID == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 item_id"})
		return
	}

	// Determine which account to use
	var acc *db.GameAccount
	if req.AccountID != nil && *req.AccountID > 0 {
		var err error
		acc, err = db.GetAccountByID(*req.AccountID)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
			return
		}
		// Ownership check
		if user.Role != "admin" && (acc.OwnerID == nil || *acc.OwnerID != user.ID) {
			jsonResp(w, M{"ok": false, "error": "无权操作此账号"})
			return
		}
		if acc.CookieData == "" {
			jsonResp(w, M{"ok": false, "error": "该账号无 Cookie"})
			return
		}
	} else {
		if user.ActiveAccountID == nil {
			jsonResp(w, M{"ok": false, "error": "未选择活跃账号"})
			return
		}
		var err error
		acc, err = db.GetAccountByID(*user.ActiveAccountID)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": "活跃的游玩账号已失效，请重新选择"})
			return
		}
	}

	var c *g79.Client
	var g79err error
	if cached := getCachedClient(acc.ID); cached != nil {
		c = cached
	} else {
		c, g79err = newG79ClientWithProxy(acc.ID)
		if g79err != nil {
			jsonResp(w, M{"ok": false, "error": "呀，糟糕，代理在偷懒X﹏X 请联系管理员"})
			return
		}
		if g79err = c.G79AuthenticateWithCookie(acc.CookieData); g79err != nil {
			log.Printf("[SKIN] auth failed for account #%d: %v", acc.ID, g79err)
			jsonResp(w, M{"ok": false, "error": fmt.Sprintf("Cookie 认证失败: %v", g79err)})
			return
		}
		setCachedClient(acc.ID, c)
	}

	// Try up to 2 times (first attempt may silently fail for unpurchased skins)
	tryChange := func() bool {
		setResp, setErr := c.SetUserSettingList(req.ItemID)
		if setErr == nil && setResp.Code == 0 {
			check, _ := c.GetUserSettingList()
			if check != nil && check.Entity.SkinData.ItemID == req.ItemID {
				return true
			}
		}
		if err := c.ChangeSkin(req.ItemID); err != nil {
			return false
		}
		check, _ := c.GetUserSettingList()
		return check != nil && check.Entity.SkinData.ItemID == req.ItemID
	}

	changed := tryChange()
	if !changed {
		// Retry once
		changed = tryChange()
	}
	if !changed {
		jsonResp(w, M{"ok": false, "error": "更换失败：皮肤未实际切换，请重试"})
		return
	}

	// Only save preview if the actual equipped skin matches the requested one
	skinURL := ""
	if settings, err := c.GetUserSettingList(); err == nil && settings != nil {
		id := settings.Entity.SkinData.ItemID
		if id != "" && id != "-1" && id == req.ItemID {
			if di, err := c.GetDownloadInfo(id); err == nil && di != nil {
				skinURL = di.Entity.ResURL
			}
		}
	}

	// Update DB — only skin fields, don't touch growth_level
	if ud, err := c.GetUserDetail(); err == nil && ud != nil {
		info := db.AccountInfo{
			DisplayName: firstNonEmpty(ud.Entity.Name, acc.DisplayName),
			UID:         c.UserID, Status: "normal",
			SkinNumber:     fmt.Sprintf("%d", ud.Entity.SkinNumber.Int64()),
			SkinURL:        skinURL,
			AvatarImageURL: ud.Entity.AvatarImageURL, IsVip: ud.Entity.IsVIP,
		}
		db.UpdateAccountFull(acc.ID, info)
		if skinURL != "" {
			// Only cache preview if not already claimed by another preset
			allPresets, _ := db.ListSkinPresets()
			safe := true
			for _, p := range allPresets {
				if p.ItemID != req.ItemID && p.PreviewURL == skinURL {
					safe = false
					break
				}
			}
			if safe {
				db.UpdateSkinPresetPreview(req.ItemID, skinURL)
			}
		}
		jsonResp(w, M{"ok": true, "message": "皮肤已更换", "skin_url": skinURL})
	} else {
		jsonResp(w, M{"ok": true, "message": "皮肤已更换"})
	}
}

// GET|POST|DELETE /api/phoenix/skin/presets
func handleSkinPresets(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	if r.Method == "DELETE" {
		idStr := r.URL.Query().Get("id")
		id, _ := strconv.ParseInt(idStr, 10, 64)
		if id == 0 {
			jsonResp(w, M{"ok": false, "error": msg.Get("missing_id", "糟糕,缺少必要信息,请刷新后重试~")})
			return
		}
		if err := db.DeleteSkinPreset(id); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		user := auth.GetUser(r.Context())
		if user != nil {
			log.Printf("[SKIN] %s 删除皮肤预设 #%d", user.Username, id)
			_ = db.AddAuditLog(&user.ID, nil, "skin_preset_delete", "skin", fmt.Sprintf("删除预设 #%d", id), requestIP(r))
		}
		jsonResp(w, M{"ok": true, "message": "已删除"})
		return
	}
	if r.Method == "POST" {
		name := r.URL.Query().Get("name")
		itemID := r.URL.Query().Get("item_id")
		if name == "" || itemID == "" {
			jsonResp(w, M{"ok": false, "error": "缺少参数"})
			return
		}
		if err := db.AddSkinPreset(name, itemID, ""); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		user := auth.GetUser(r.Context())
		if user != nil {
			log.Printf("[SKIN] %s 添加皮肤预设: %s (%s)", user.Username, name, itemID)
			_ = db.AddAuditLog(&user.ID, nil, "skin_preset_add", "skin", fmt.Sprintf("添加预设 %s (%s)", name, itemID), requestIP(r))
		}
		jsonResp(w, M{"ok": true, "message": "已添加"})
		return
	}
	if r.Method == "PUT" {
		var body struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			ItemID string `json:"item_id"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ID == 0 || body.Name == "" || body.ItemID == "" {
			jsonResp(w, M{"ok": false, "error": "缺少参数"})
			return
		}
		if err := db.UpdateSkinPreset(body.ID, body.Name, body.ItemID); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		user := auth.GetUser(r.Context())
		if user != nil {
			log.Printf("[SKIN] %s 更新皮肤预设 #%d: %s (%s)", user.Username, body.ID, body.Name, body.ItemID)
			_ = db.AddAuditLog(&user.ID, nil, "skin_preset_update", "skin", fmt.Sprintf("更新预设 #%d %s (%s)", body.ID, body.Name, body.ItemID), requestIP(r))
		}
		jsonResp(w, M{"ok": true, "message": "已更新"})
		return
	}
	presets, err := db.ListSkinPresets()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "presets": presets})
}

// POST /api/phoenix/social/apply
func handleSocialApply(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	var req struct {
		UID uint64 `json:"uid"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.UID == 0 {
		jsonResp(w, M{"ok": false, "error": "缺少 uid"})
		return
	}
	resp, err := c.ApplyFriend(req.UID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "message": resp.Message})
}

// POST /api/phoenix/social/reply
func handleSocialReply(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	var req struct {
		UID    uint64 `json:"uid"`
		Accept bool   `json:"accept"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.UID == 0 {
		jsonResp(w, M{"ok": false, "error": "缺少 uid"})
		return
	}
	resp, err := c.ReplyFriend(req.UID, req.Accept)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "message": resp.Message})
}

// GET /api/phoenix/social/messages
func handleSocialMessages(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	uid := r.URL.Query().Get("uid")
	if uid == "" {
		uid = c.UserID
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	num, _ := strconv.Atoi(r.URL.Query().Get("num"))
	if num <= 0 || num > 50 {
		num = 10
	}
	resp, err := c.GetUserMessages(uid, page, num)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "messages": resp})
}

// GET /api/phoenix/domain/list
func handleDomainList(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	resp, err := c.GetOtherDomainServers()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "servers": resp.Entities})
}

// GET /api/phoenix/domain/detail
func handleDomainDetail(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	sid := r.URL.Query().Get("sid")
	if sid == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 sid"})
		return
	}
	resp, err := c.GetDomainServerDetail(sid)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "server": resp.Entity})
}

// POST /api/phoenix/domain/enter
func handleDomainEnter(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, user, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if inJoinCooldown(user) {
		writeTooMany(w, r)
		return
	}
	var req struct {
		SID string `json:"sid"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.SID == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 sid"})
		return
	}
	resp, err := c.RequestEnterDomainServer(req.SID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "server": resp.Entity})
}

// POST /api/phoenix/domain/leave
func handleDomainLeave(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	var req struct {
		SID string `json:"sid"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.SID == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 sid"})
		return
	}
	resp, err := c.RequestLeaveDomainServer(req.SID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "message": resp.Message})
}

// POST /api/phoenix/domain/join — 通过邀请码加入山头服 (mc.163.com/open/mcrealms/?realms=XXX)
func handleDomainJoin(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, user, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if inJoinCooldown(user) {
		writeTooMany(w, r)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.Code) == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 code"})
		return
	}
	resp, err := c.JoinDomainServerWithInviteCode(req.Code)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "entities": resp.Entities})
}

// GET /api/phoenix/lobby/room
func handleLobbyRoom(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, user, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	rid := r.URL.Query().Get("room_id")
	if rid == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 room_id"})
		return
	}
	resp, err := c.GetOnlineLobbyRoom(rid)
	if err != nil {
		invalidateG79Cache(*user.ActiveAccountID)
		if c2, _, err2 := getG79Client(r); err2 == nil {
			resp, err = c2.GetOnlineLobbyRoom(rid)
		}
	}
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "room": resp.Entity})
}

// POST /api/phoenix/lobby/game-enter — 进入联机大厅游戏
func handleLobbyGameEnter(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, user, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if inJoinCooldown(user) {
		writeTooMany(w, r)
		return
	}
	resp, err := c.OnlineLobbyGameEnter()
	if err != nil {
		invalidateG79Cache(*user.ActiveAccountID)
		if c2, _, err2 := getG79Client(r); err2 == nil {
			resp, err = c2.OnlineLobbyGameEnter()
		}
	}
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "entity": resp.Entity})
}

// POST /api/phoenix/lobby/enter
func handleLobbyEnter(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, user, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if inJoinCooldown(user) {
		writeTooMany(w, r)
		return
	}
	var req struct {
		RoomID   string `json:"room_id"`
		EntityID string `json:"entity_id"`
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	rid := req.RoomID
	if rid == "" {
		rid = req.EntityID
	}
	if rid == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 room_id 或 entity_id"})
		return
	}

	// 先查房间详情，确认房间存在并获取商品ID
	info, iErr := c.GetOnlineLobbyRoom(rid)
	if iErr != nil {
		invalidateG79Cache(*user.ActiveAccountID)
		if c2, _, err2 := getG79Client(r); err2 == nil {
			info, iErr = c2.GetOnlineLobbyRoom(rid)
		}
	}
	if iErr != nil {
		jsonResp(w, M{"ok": false, "error": "无法获取房间详情: " + iErr.Error()})
		return
	}
	if info.Code != 0 {
		jsonResp(w, M{"ok": false, "error": "房间不存在或已关闭"})
		return
	}

	// 尝试进入房间（501 = 需购买 → 购买后重试，最多3次）
	resID := info.Entity.ResID.String()
	for attempt := 1; attempt <= 3; attempt++ {
		resp, eErr := c.EnterOnlineLobbyRoom(rid, req.Password)
		if eErr != nil {
			invalidateG79Cache(*user.ActiveAccountID)
			if c2, _, err2 := getG79Client(r); err2 == nil {
				resp, eErr = c2.EnterOnlineLobbyRoom(rid, req.Password)
				if eErr == nil && resp.Code == 0 {
					jsonResp(w, M{"ok": true, "code": resp.Code, "room": resp.Entity})
					return
				}
			}
			if req.Password != "" && isPasswordError(eErr.Error()) {
				markJoinCooldown(user)
			}
			jsonResp(w, M{"ok": false, "error": eErr.Error()})
			return
		}
		if resp.Code == 0 {
			jsonResp(w, M{"ok": true, "code": resp.Code, "room": resp.Entity})
			return
		}
		if resp.Code != 501 {
			if req.Password != "" && isPasswordError(resp.Message) {
				markJoinCooldown(user)
			}
			jsonResp(w, M{"ok": false, "error": fmt.Sprintf("进房失败: %s(%d)", resp.Message, resp.Code)})
			return
		}
		if attempt < 3 {
			c.PurchaseItem(resID)
			time.Sleep(1 * time.Second)
		}
	}
	jsonResp(w, M{"ok": false, "error": "进入房间失败: 需要购买商品但多次尝试失败"})
}

// # 联机房间 — 按商品 ID 列出所有房间
// GET /api/phoenix/lobby/list?res_id=xxx
func handleLobbyListByRes(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	if !lobbyListLimiter.Allow(lobbyLimitIP(r)) {
		jsonResp(w, M{"ok": false, "error": "搜索过于频繁，请5秒后再试"})
		return
	}
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	resID := r.URL.Query().Get("res_id")
	if resID == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 res_id"})
		return
	}
	resp, err := c.ListOnlineLobbyRoomByResID(resID, 10000, 0)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "total": resp.Total, "rooms": resp.Entities})
}

// # 联机房间 — 离开房间
// POST /api/phoenix/lobby/leave  body: {"room_id":"xxx"}
func handleLobbyLeave(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	var req struct {
		RoomID string `json:"room_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.RoomID == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 room_id"})
		return
	}
	resp, err := c.LeaveOnlineLobbyRoom(req.RoomID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "message": resp.Message})
}

// GET /api/phoenix/hot
func handleHotCategories(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	resp, err := c.GetHotCategories()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "categories": resp})
}

// GET /api/phoenix/transfer_room
func handleTransferRoom(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 name"})
		return
	}
	resp, err := c.GetTransferRoomWithName(name)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "rooms": resp.List})
}

// POST /api/phoenix/social/like — 点赞动态 body: {"msg_id":"...","push_id":123}
func handleSocialLike(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	var req struct {
		MsgID  string `json:"msg_id"`
		PushID uint64 `json:"push_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.MsgID == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 msg_id"})
		return
	}
	resp, err := c.LikeMoment(req.MsgID, req.PushID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "message": resp.Message})
}

// POST /api/phoenix/social/moment — 发动态 body: {"content":"..."}
func handleSocialMoment(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Content == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 content"})
		return
	}
	resp, err := c.SendMoment(req.Content, nil)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true, "code": resp.Code, "message": resp.Message})
}

// POST /api/phoenix/server/like — 点赞/取消点赞租赁服 body: {"server_id":"...","is_like":true}
func handleServerLike(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	var req struct {
		ServerID string `json:"server_id"`
		IsLike   bool   `json:"is_like"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.ServerID == "" {
		jsonResp(w, M{"ok": false, "error": msg.Get("missing_id", "糟糕,缺少必要的服务器信息,请刷新后重试~")})
		return
	}
	api := "/rental-server-like/update"
	likeVal := "0"
	if req.IsLike {
		likeVal = "1"
	}
	body := fmt.Sprintf(`{"server_id":"%s","is_like":%s}`, req.ServerID, likeVal)
	httpReq, _ := http.NewRequest("POST", c.ReleaseJSON.WebServerUrl+api, strings.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpReq.Header.Set("User-Agent", "WPFLauncher/0.0.0.0")
	httpReq.Header.Set("user-id", c.UserID)
	httpReq.Header.Set("user-token", g79.CalculateDynamicToken(api, body, c.UserToken))
	resp, err := c.HTTPClient().Do(httpReq)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	json.Unmarshal(data, &result)
	jsonResp(w, M{"ok": true, "code": result.Code, "message": result.Message})
}

// POST /api/accounts/rotate — 切换下一个私有账号
func handleAccountRotate(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	accs, err := db.GetUserAccounts(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}

	// Filter only private, normal, non-disabled accounts with cookies
	var available []*db.GameAccount
	for _, a := range accs {
		if a.OwnerID != nil && a.Status == "normal" && !a.Disabled && a.CookieData != "" {
			available = append(available, a)
		}
	}
	if len(available) == 0 {
		jsonResp(w, M{"ok": false, "error": "没有可用的私有账号"})
		return
	}

	accountRotateMu.Lock()
	used := accountRotateUsed[user.ID]

	// Find first unused account
	var next *db.GameAccount
	for _, a := range available {
		found := false
		for _, u := range used {
			if u == a.ID {
				found = true
				break
			}
		}
		if !found {
			next = a
			break
		}
	}

	// All used — reset cycle
	if next == nil {
		accountRotateUsed[user.ID] = nil
		next = available[0]
	}

	accountRotateUsed[user.ID] = append(accountRotateUsed[user.ID], next.ID)
	accountRotateMu.Unlock()

	db.SetActiveAccount(user.ID, next.ID)
	jsonResp(w, M{"ok": true, "message": "已切换到 " + next.DisplayName, "account_id": next.ID, "display_name": next.DisplayName, "uid": next.UID})
}

// GET /api/phoenix/download?item_id=xxx — 获取组件/皮肤下载地址
// item_id 支持 19 位商品 ID，也支持分享链接（自动从末尾提取 19 位 ID）
func handleDownload(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	itemID := extractItemID(r.URL.Query().Get("item_id"))
	if itemID == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 item_id"})
		return
	}
	resp, err := c.GetDownloadInfo(itemID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{
		"ok":        true,
		"code":      resp.Code,
		"message":   resp.Message,
		"entity_id": resp.Entity.EntityID.String(),
		"res_url":   resp.Entity.ResURL,
	})
}

// 从输入中提取 19 位商品 ID，兼容纯数字和分享链接两种格式
func extractItemID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// 纯数字直接返回
	if matched, _ := regexp.MatchString(`^\d{15,20}$`, raw); matched {
		return raw
	}
	// 从链接中提取末尾的 15-20 位数字 ID
	re := regexp.MustCompile(`(\d{15,20})`)
	matches := re.FindAllString(raw, -1)
	if len(matches) > 0 {
		return matches[len(matches)-1]
	}
	return ""
}

// POST /api/phoenix/purchase — 购买组件 body: {"item_id":"xxx"}
// item_id 支持 19 位数字或分享链接
func handlePurchase(w http.ResponseWriter, r *http.Request) {
	defer recordCall(r, true)
	c, _, err := getG79Client(r)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	var req struct {
		ItemID string `json:"item_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	itemID := extractItemID(req.ItemID)
	if itemID == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 item_id"})
		return
	}
	resp, err := c.PurchaseItem(itemID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{
		"ok":        true,
		"code":      resp.Code,
		"message":   resp.Message,
		"entity_id": resp.Entity.EntityID.String(),
		"buy_type":  resp.Entity.BuyType.String(),
	})
}
