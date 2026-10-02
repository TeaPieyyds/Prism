package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

var proxyEnabled = true

// proxyConfigPath 代理配置文件路径（相对 WorkingDirectory）。每 10s 轮询一次，支持热加载：
// 每行一个代理，支持 # 注释；可为 `socks5://host:port`、`http://host:port` 或裸 `host:port`（默认按 socks5）。
var proxyConfigPath = "proxies.conf"

// 代理池：proxyList 为全部配置代理，proxyHealth 记录每个代理的实时健康状态。
// AssignProxy 在所有健康代理间轮询分摊，避免把全部流量压到单一出口（旧的 proxies[0] 行为）。
var (
	poolMu       sync.RWMutex
	proxyList    []string
	proxyHealth  = map[string]bool{}
	proxyCursor  int

	assignMu      sync.Mutex
	accountProxy  = map[int64]string{}
	accountClient = map[int64]*http.Client{}
	// accountAvoid 记录账号最近一次 code32 的代理，重分配时避开它，
	// 避免"账号在 X 上 code32 → 转移 → 又选回 X"的死循环。
	accountAvoid = map[int64]string{}

	// 代理风控冷却：同一代理在窗口内累计 proxyCode32BanThreshold 个【不同】账号
	// 因 code32 被转移，即判定该代理出口 IP 被网易风控，进入 proxyCooldownDuration 冷却。
	proxyCode32        = map[string]map[int64]time.Time{} // url -> accountID -> 最近一次 code32 时间
	proxyCooldownUntil = map[string]time.Time{}           // url -> 冷却到期时间
	// 代理超时标记：与 code32 独立计数。同一代理在窗口内累计 proxyTimeoutBanThreshold
	// 个【不同】账号超时，判定该代理响应异常，进入较短的 proxyTimeoutCooldown 冷却并定期重测。
	proxyTimeout = map[string]map[int64]time.Time{} // url -> accountID -> 最近一次超时时间
)

const (
	proxyCode32Window       = 10 * time.Minute // 计数窗口
	proxyCode32BanThreshold = 5                // 窗口内不同账号数达到该值判定封禁
	proxyCooldownDuration   = 12 * time.Hour   // 封禁冷却时长，到期后重新探测

	proxyTimeoutWindow       = 10 * time.Minute // 超时计数窗口
	proxyTimeoutBanThreshold = 5                // 窗口内不同账号数达到该值判定超时标记
	proxyTimeoutCooldown     = 10 * time.Minute // 超时标记后每 10 分钟重测一次，通了恢复
)

func buildDefaultProxyList() []string {
	return []string{
		"socks5://127.0.0.1:1090", // 160服务器
		"socks5://127.0.0.1:1082", // 手机隧道
		"socks5://127.0.0.1:1080", // 180服务器
	}
}

// loadProxyConfigFile 从配置文件读取代理列表；文件不存在时返回 nil（调用方回退默认列表）。
func loadProxyConfigFile() []string {
	f, err := os.Open(proxyConfigPath)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "://") {
			line = "socks5://" + line
		}
		out = append(out, line)
	}
	return out
}

// reloadProxyPool 从配置文件(或默认列表)重建代理池：新增代理默认健康，删除的代理移除，
// 并把挂在被删代理上的账号释放以便重新分配。热加载时调用。
func reloadProxyPool() {
	cfg := loadProxyConfigFile()
	if len(cfg) == 0 {
		cfg = buildDefaultProxyList()
	}
	poolMu.Lock()
	newList := make([]string, 0, len(cfg))
	seen := make(map[string]bool, len(cfg))
	for _, p := range cfg {
		if seen[p] {
			continue
		}
		seen[p] = true
		newList = append(newList, p)
		if _, ok := proxyHealth[p]; !ok {
			proxyHealth[p] = true // 新代理默认可用，交给周期健康检测把关
		}
	}
	for _, p := range proxyList {
		if !seen[p] {
			delete(proxyHealth, p)
		}
	}
	proxyList = newList
	if proxyCursor >= len(proxyList) {
		proxyCursor = 0
	}
	poolMu.Unlock()
	log.Printf("[PROXY] 代理池刷新: %d 个 (%v)", len(proxyList), proxyList)

	assignMu.Lock()
	for id, p := range accountProxy {
		if !seen[p] {
			delete(accountClient, id)
			delete(accountProxy, id)
		}
	}
	assignMu.Unlock()
}

// pickHealthyProxy 轮询选取下一个健康代理；无健康代理时返回空串。
func pickHealthyProxy(avoid string) string {
	poolMu.RLock()
	defer poolMu.RUnlock()
	n := len(proxyList)
	if n == 0 {
		return ""
	}
	for i := 0; i < n; i++ {
		proxyCursor = (proxyCursor + 1) % n
		p := proxyList[proxyCursor]
		if proxyHealth[p] && p != avoid {
			return p
		}
	}
	return ""
}

// pickLowCode32Proxy 在健康代理中优先选取 proxyCode32Window(10分钟) 内 code32 次数最少(最好为0)的健康代理，
// 用于账号遭遇 code32 后的重分配，尽量避开近期被风控的出口。排除 avoid(刚失败的代理)与冷却中的代理。
// 无候选时返回空串。读取 proxyCode32 需持有 poolMu（读锁）。
func pickLowCode32Proxy(avoid string, now time.Time) string {
	poolMu.RLock()
	defer poolMu.RUnlock()
	var best string
	bestCount := int(^uint(0) >> 1)
	bestClean := true
	for _, p := range proxyList {
		if !proxyHealth[p] || p == avoid {
			continue
		}
		if cd, ok := proxyCooldownUntil[p]; ok && now.Before(cd) {
			continue
		}
		count := 0
		for _, t := range proxyCode32[p] {
			if now.Sub(t) <= proxyCode32Window {
				count++
			}
		}
		// clean：该代理从未被 code32 标记过。count 相同时优先从未风控过的干净出口
		//（如外部/住宅代理），避免账号 code32 后总是切到遍历顺序靠前、近10分钟虽未
		// 触发但历史被风控过的坏代理上（那种切过去还会再 32）。
		clean := len(proxyCode32[p]) == 0
		if best == "" || count < bestCount || (count == bestCount && clean && !bestClean) {
			best = p
			bestCount = count
			bestClean = clean
		}
	}
	return best
}

// reassignProxy 账号从某代理失败后重分配：timeout 走默认轮询；code32 优先选低 code32 代理。
func reassignProxy(accountID int64, old string, isTimeout bool) *http.Client {
	if isTimeout {
		return AssignProxy(accountID)
	}
	return AssignProxyCode32Aware(accountID, old)
}

// AssignProxyCode32Aware 在账号遭遇 code32 后重分配：优先选 10 分钟内 code32 次数最少的健康代理，
// 无合适候选时回退 AssignProxy 默认逻辑。
func AssignProxyCode32Aware(accountID int64, oldProxy string) *http.Client {
	p := pickLowCode32Proxy(oldProxy, time.Now())
	if p == "" {
		return AssignProxy(accountID)
	}
	assignMu.Lock()
	if c, ok := accountClient[accountID]; ok {
		assignMu.Unlock()
		return c
	}
	rt, err := transportFor(p)
	if err != nil {
		assignMu.Unlock()
		return AssignProxy(accountID)
	}
	c := &http.Client{Transport: rt, Timeout: 30 * time.Second}
	accountClient[accountID] = c
	accountProxy[accountID] = p
	assignMu.Unlock()
	return c
}

// markProxyHealth 更新某个代理的健康状态；变坏时释放挂在其上的账号，使流量分摊到其他代理。
func markProxyHealth(url string, ok bool) {
	poolMu.Lock()
	if proxyHealth[url] != ok {
		proxyHealth[url] = ok
		if !ok {
			log.Printf("[PROXY] 代理 %s 已失效，流量分摊到其他代理", url)
		} else {
			log.Printf("[PROXY] 代理 %s 已恢复", url)
		}
	}
	if ok {
		// 代理恢复：清除其风控冷却/超时标记记录，让后续失败重新累计
		delete(proxyCooldownUntil, url)
		delete(proxyCode32, url)
		delete(proxyTimeout, url)
	}
	poolMu.Unlock()
	if !ok {
		assignMu.Lock()
		for id, p := range accountProxy {
			if p == url {
				delete(accountClient, id)
				delete(accountProxy, id)
			}
		}
		assignMu.Unlock()
	}
}

// TestAllProxies 并发探测所有代理，更新健康状态。可重复调用（内部限频 30s）。
var testAllProxiesMu sync.Mutex
var lastTestAllProxies time.Time

// probeProxies 并发探测指定代理列表并更新健康状态，随后打印统计。
// probe 为实际探测的代理；total 为配置的代理总数（用于统计冷却数）。
func probeProxies(probe []string, total int) {
	if len(probe) == 0 {
		return
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 200)
	for _, p := range probe {
		wg.Add(1)
		sem <- struct{}{}
		go func(proxyURL string) {
			defer wg.Done()
			defer func() { <-sem }()
			rt, err := transportFor(proxyURL)
			if err != nil {
				markProxyHealth(proxyURL, false)
				return
			}
			c := &http.Client{Transport: rt, Timeout: 5 * time.Second}
			resp, err := c.Get("https://g79authobt.minecraft.cn")
			if err != nil {
				markProxyHealth(proxyURL, false)
				return
			}
			resp.Body.Close()
			markProxyHealth(proxyURL, true)
		}(p)
	}
	wg.Wait()

	poolMu.RLock()
	good, bad := 0, 0
	for _, p := range probe {
		if proxyHealth[p] {
			good++
		} else {
			bad++
		}
	}
	cooldown := total - len(probe)
	poolMu.RUnlock()
	log.Printf("[PROXY] 代理健康检测完成: %d 可用 / %d 失效 / %d 冷却 (共 %d)", good, bad, cooldown, total)
}

func TestAllProxies() {
	if !proxyEnabled {
		return
	}
	if !testAllProxiesMu.TryLock() {
		return
	}
	defer testAllProxiesMu.Unlock()
	if time.Since(lastTestAllProxies) < 30*time.Second {
		return
	}
	poolMu.RLock()
	list := make([]string, len(proxyList))
	copy(list, proxyList)
	poolMu.RUnlock()
	if len(list) == 0 {
		return
	}
	lastTestAllProxies = time.Now()

	// 冷却中的代理跳过探测：避免健康探测（只看 HTTP 通断）把应用层风控的代理
	// 重新标回健康，导致切换后又选回原代理。冷却到期后自动重新参与探测。
	now := time.Now()
	poolMu.RLock()
	probe := make([]string, 0, len(list))
	for _, p := range list {
		if cd, ok := proxyCooldownUntil[p]; ok && now.Before(cd) {
			continue
		}
		probe = append(probe, p)
	}
	poolMu.RUnlock()

	probeProxies(probe, len(list))
}

// TestAllProxiesAll 强制探测全部代理（含冷却中的），绕过 30s 限频。
// 用于"所有代理都被封且直连也被封"的兜底：重新探测一遍，看是否有代理已解封可用。
func TestAllProxiesAll() {
	if !proxyEnabled {
		return
	}
	poolMu.RLock()
	list := make([]string, len(proxyList))
	copy(list, proxyList)
	poolMu.RUnlock()
	if len(list) == 0 {
		return
	}
	probeProxies(list, len(list))
}

// allProxiesBlocked 返回是否所有代理当前都不可用（健康状态均为 false）。
func allProxiesBlocked() bool {
	poolMu.RLock()
	defer poolMu.RUnlock()
	if len(proxyList) == 0 {
		return true
	}
	for _, p := range proxyList {
		if proxyHealth[p] {
			return false
		}
	}
	return true
}

// preferredProxyForAccount 返回应优先给该账号使用的代理地址。
// 自写协调者有在线节点时：所有账号统一走协调者本地 SOCKS5（socks5h），由协调者按
// 轮询在在线节点间选一台出口 → 网易只见出口节点 IP，不暴露服务器本机 IP。
// 无在线节点时返回空，走 proxies.conf 的 socks/direct 兜底。账号归属亲和后续版本再加。
func preferredProxyForAccount(accountID int64) string {
	if p2pCoord.onlineCount() > 0 {
		return nodeSocksURL(p2pSocksPort)
	}
	return ""
}

// AssignProxy 为 accountID 分配一个代理 client，并在健康代理间轮询分摊，避免全部挤到同一出口。
// 该代理失效(code 32)后调用 ReportAccountCode32 移除并重新分配；无可用代理时返回直连 client。
func AssignProxy(accountID int64) *http.Client {
	assignMu.Lock()
	defer assignMu.Unlock()

	if c, ok := accountClient[accountID]; ok {
		return c
	}

	// 亲和：账号 owner 贡献的在线节点优先（该账号最近 code32/timeout 的节点除外）
	if pref := preferredProxyForAccount(accountID); pref != "" && pref != accountAvoid[accountID] {
		if rt, err := transportFor(pref); err == nil {
			c := &http.Client{Transport: rt, Timeout: 30 * time.Second}
			accountClient[accountID] = c
			accountProxy[accountID] = pref
			return c
		}
	}

	// 避开该账号最近一次 code32 的代理（AssignProxy 已在 assignMu 下，可直接读）
	proxyURL := pickHealthyProxy(accountAvoid[accountID])
	if proxyURL == "" {
		log.Printf("[ASSIGN] #%d 无可用代理，回退直连", accountID)
		go TestAllProxies()
		return &http.Client{Transport: &http.Transport{}, Timeout: 30 * time.Second}
	}

	rt, err := transportFor(proxyURL)
	if err != nil {
		c := &http.Client{Timeout: 30 * time.Second}
		accountClient[accountID] = c
		accountProxy[accountID] = ""
		return c
	}

	c := &http.Client{Transport: rt, Timeout: 30 * time.Second}
	accountClient[accountID] = c
	accountProxy[accountID] = proxyURL
	return c
}

// ReportAccountCode32 账号在某代理下 code 32（该代理出口 IP 被网易风控）时：
//   - 总是先把这个账号转移到其他健康代理（不误伤整条代理）；
//   - 同时在窗口内累计该代理被转移的【不同账号】数，达到 proxyCode32BanThreshold
//     即判定该代理被风控，进入 proxyCooldownDuration 冷却，并转移其剩余全部账号。
//
// 冷却期间的代理不会被 TestAllProxies 探测恢复（避免健康探测只看 HTTP 通断、把
// 应用层风控的代理又标回健康，导致"切换后又选回原代理"）。
func ReportAccountCode32(accountID int64) *http.Client {
	return reportProxyFail(accountID, false)
}

// ReportAccountProxyTimeout 账号在某代理下超时时调用：与 code32 独立处理。
// 连续多个账号在该代理超时达到阈值后，把该代理标记为超时并进入较短的
// proxyTimeoutCooldown 冷却（每 10 分钟重测），通了即恢复。
func ReportAccountProxyTimeout(accountID int64) *http.Client {
	return reportProxyFail(accountID, true)
}

// reportProxyFail 是 code32/超时共用的代理失败处理：
//   - 总是先把账号转移到其他健康代理（不误伤整条代理），并避开刚失败的代理；
//   - 窗口内累计该代理被转移的【不同账号】数，达到阈值即判定该代理异常，
//     进入冷却（code32=12h，超时=10min），并转移其剩余全部账号。
func reportProxyFail(accountID int64, isTimeout bool) *http.Client {
	assignMu.Lock()
	old := accountProxy[accountID]
	delete(accountClient, accountID)
	delete(accountProxy, accountID)
	if old != "" {
		// 记录该账号要避开的代理，重分配时不再选回它
		accountAvoid[accountID] = old
	}
	assignMu.Unlock()

	kind := "code 32"
	if isTimeout {
		kind = "timeout"
	}
	if old == "" {
		log.Printf("[PROXY] 账号 #%d %s (直连)", accountID, kind)
		// 直连也不可用 + 所有代理都不可用 → 强制重新探测全部代理（含冷却中）
		if allProxiesBlocked() {
			log.Printf("[PROXY] 所有代理均不可用且直连不可用，强制重新探测全部代理")
			go TestAllProxiesAll()
		}
		return AssignProxy(accountID)
	}
	log.Printf("[PROXY] 账号 #%d %s, 释放 %s", accountID, kind, old)

	now := time.Now()
	countMap := proxyCode32
	window, threshold, cooldown := proxyCode32Window, proxyCode32BanThreshold, proxyCooldownDuration
	if isTimeout {
		countMap = proxyTimeout
		window, threshold, cooldown = proxyTimeoutWindow, proxyTimeoutBanThreshold, proxyTimeoutCooldown
	}

	poolMu.Lock()
	// 已在冷却中的代理：只转移该账号，不重复封禁
	if cd, ok := proxyCooldownUntil[old]; ok && now.Before(cd) {
		poolMu.Unlock()
		return reassignProxy(accountID, old, isTimeout)
	}
	// 记录该账号失败时间，并清理窗口外的旧记录（同一账号反复失败不算新增）
	set := countMap[old]
	if set == nil {
		set = map[int64]time.Time{}
		countMap[old] = set
	}
	for id, t := range set {
		if now.Sub(t) > window {
			delete(set, id)
		}
	}
	set[accountID] = now
	// 窗口内不同账号数达到阈值 → 判定该代理异常，进入冷却
	if len(set) >= threshold {
		proxyCooldownUntil[old] = now.Add(cooldown)
		delete(countMap, old)
		if isTimeout {
			log.Printf("[PROXY] 代理 %s 在 %d 分钟内 %d 个不同账号超时，标记超时冷却 %d 分钟，转移其全部账号",
				old, int(window.Minutes()), len(set), int(cooldown.Minutes()))
		} else {
			log.Printf("[PROXY] 代理 %s 在 %d 分钟内 %d 个不同账号 code32，判定风控封禁 %d 小时，转移其全部账号",
				old, int(window.Minutes()), len(set), int(cooldown.Hours()))
		}
		poolMu.Unlock()
		markProxyHealth(old, false)
		return reassignProxy(accountID, old, isTimeout)
	}
	poolMu.Unlock()
	return reassignProxy(accountID, old, isTimeout)
}

// ReleaseAccountProxy 释放账号占用的代理。
func ReleaseAccountProxy(accountID int64) {
	assignMu.Lock()
	delete(accountClient, accountID)
	delete(accountProxy, accountID)
	assignMu.Unlock()
}

// GetProxyDialer 返回 accountID 对应代理的 SOCKS5 dialer，无代理时返回直连 dialer。
func GetProxyDialer(accountID int64) proxy.Dialer {
	assignMu.Lock()
	p := accountProxy[accountID]
	assignMu.Unlock()
	if p == "" {
		return proxy.Direct
	}
	u, err := url.Parse(p)
	if err != nil {
		return proxy.Direct
	}
	d, err := proxy.SOCKS5("tcp", u.Host, nil, proxy.Direct)
	if err != nil {
		return proxy.Direct
	}
	return d
}

// PickOneTimeProxy 从健康代理中随机选一个，每次调用都不同（不缓存）。
// 无可用代理时返回直连 client。
func PickOneTimeProxy() *http.Client {
	poolMu.RLock()
	list := make([]string, 0, len(proxyList))
	for _, p := range proxyList {
		if proxyHealth[p] {
			list = append(list, p)
		}
	}
	poolMu.RUnlock()

	if len(list) == 0 {
		go TestAllProxies()
		return &http.Client{Transport: &http.Transport{}, Timeout: 30 * time.Second}
	}

	for i := 0; i < 3; i++ {
		idx := rand.Intn(len(list))
		if rt, err := transportFor(list[idx]); err == nil {
			return &http.Client{Transport: rt, Timeout: 30 * time.Second}
		}
	}
	return &http.Client{Transport: &http.Transport{}, Timeout: 30 * time.Second}
}

// transportFor 根据代理 URL 创建 http.RoundTripper。
func transportFor(proxyURL string) (http.RoundTripper, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if u.User != nil {
			pass, _ := u.User.Password()
			auth = &proxy.Auth{User: u.User.Username(), Password: pass}
		}
		d, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
		if err != nil {
			return nil, err
		}
		if u.Scheme == "socks5h" {
			// socks5h：原样传递主机名，不做本地解析。域名由出口节点用安全 DNS 解析
			// （P2P/志愿节点出口专用；部分设备 DNS 异常的代理仍走 socks5 本地解析成 IP）。
			return &http.Transport{
				Dial: func(network, addr string) (net.Conn, error) {
					return d.Dial(network, addr)
				},
			}, nil
		}
		return &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return d.Dial(network, addr)
				}
				ips, err := net.DefaultResolver.LookupHost(context.Background(), host)
				if err != nil || len(ips) == 0 {
					return d.Dial(network, addr)
				}
				return d.Dial(network, net.JoinHostPort(ips[0], port))
			},
		}, nil
	case "socks4":
		return &http.Transport{Proxy: http.ProxyURL(u)}, nil
	case "http", "https":
		return &http.Transport{Proxy: http.ProxyURL(u)}, nil
	case "direct":
		// 直连：不走 SOCKS5/HTTP 代理，直接用本机网络栈出网。
		// host 部分可指定出口源 IP（如 direct://1.2.3.4），省略则用系统默认路由出口。
		d := &net.Dialer{}
		if u.Host != "" {
			if src := net.ParseIP(u.Host); src != nil {
				d.LocalAddr = &net.TCPAddr{IP: src}
			}
		}
		return &http.Transport{
			DialContext: d.DialContext,
		}, nil
	}
	return nil, fmt.Errorf("unsupported proxy scheme: %s", u.Scheme)
}

// ── 自动获取免费代理 ──

var proxyFetchMu sync.Mutex
var lastProxyFetch time.Time

// fetchProxiesFromAPI 从免费代理 API 获取代理列表并合并到 proxyList
func fetchProxiesFromAPI(apiURL string) []string {
	const maxFetch = 500
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	text := string(body)
	var out []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// 格式: IP:PORT 或 protocol://IP:PORT
		if !strings.Contains(line, ":") {
			continue
		}
		// 添加支持的协议
		if !strings.Contains(line, "://") {
			out = append(out, "socks5://"+line, "http://"+line)
			if len(out) >= maxFetch {
				break
			}
		} else {
			out = append(out, line)
		}
	}
	return out
}

// fetchProxiesFromJSONAPI 从 JSON 格式的代理 API 获取代理
func fetchProxiesFromJSONAPI(apiURL string) []string {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	// 尝试 66daili 格式
	type entry66 struct {
		IP       string `json:"ip"`
		Port     string `json:"port"`
		Protocol string `json:"protocol"`
	}
	var result66 struct {
		Data []entry66 `json:"data"`
	}
	if err := json.Unmarshal(body, &result66); err == nil && len(result66.Data) > 0 {
		var out []string
		for _, e := range result66.Data {
			if e.IP == "" || e.Port == "" { continue }
			proto := strings.ToLower(e.Protocol)
			switch proto {
			case "http", "https": out = append(out, fmt.Sprintf("http://%s:%s", e.IP, e.Port))
			case "socks5": out = append(out, fmt.Sprintf("socks5://%s:%s", e.IP, e.Port))
			case "socks4": out = append(out, fmt.Sprintf("socks4://%s:%s", e.IP, e.Port))
			}
		}
		return out
	}
	// 尝试 ProxyScrape 格式
	type entryPS struct {
		Proxy string `json:"proxy"`
		Alive bool   `json:"alive"`
	}
	var resultPS struct {
		Proxies []entryPS `json:"proxies"`
	}
	if err := json.Unmarshal(body, &resultPS); err == nil && len(resultPS.Proxies) > 0 {
		var out []string
		for _, e := range resultPS.Proxies {
			if e.Alive && e.Proxy != "" {
				out = append(out, e.Proxy)
			}
		}
		return out
	}
	// 尝试 proxy.scdn.io 格式
	type scdnResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Proxies []string `json:"proxies"`
		} `json:"data"`
	}
	var scdnResult scdnResp
	if err := json.Unmarshal(body, &scdnResult); err == nil && scdnResult.Code == 200 && len(scdnResult.Data.Proxies) > 0 {
		var out []string
		for _, p := range scdnResult.Data.Proxies {
			if strings.TrimSpace(p) != "" {
				if !strings.Contains(p, "://") {
					out = append(out, "http://"+p)
				} else {
					out = append(out, p)
				}
			}
		}
		return out
	}
	// 尝试 Proxifly 格式（数组，每项有 proxy 字段）
	type entryPF struct {
		Proxy    string `json:"proxy"`
		Protocol string `json:"protocol"`
	}
	var resultPF []entryPF
	if err := json.Unmarshal(body, &resultPF); err == nil && len(resultPF) > 0 {
		var out []string
		for _, e := range resultPF {
			if e.Proxy != "" {
				out = append(out, e.Proxy)
			}
		}
		return out
	}
	return nil
}

// RefreshProxyPool 从外部源刷新代理池，每 2 小时可调用一次
func RefreshProxyPool() {
	proxyFetchMu.Lock()
	defer proxyFetchMu.Unlock()
	if time.Since(lastProxyFetch) < 2*time.Hour {
		return
	}
	lastProxyFetch = time.Now()

	apis := []string{}
	seen := make(map[string]bool)
	var allNew []string
	for _, p := range proxyList {
		seen[p] = true
	}
	for _, api := range apis {
		log.Printf("[PROXY-FETCH] 获取中: %s", api)
		var list []string
		if strings.Contains(api, "66daili") || strings.Contains(api, "format=json") {
			list = fetchProxiesFromJSONAPI(api)
		} else {
			list = fetchProxiesFromAPI(api)
		}
		log.Printf("[PROXY-FETCH] 获取到 %d 个代理", len(list))
		for _, p := range list {
			if !seen[p] {
				seen[p] = true
				allNew = append(allNew, p)
			}
		}
	}
	if len(allNew) > 0 {
		proxyList = append(proxyList, allNew...)
		log.Printf("[PROXY-FETCH] 新增 %d 个代理，总数 %d", len(allNew), len(proxyList))
		// 立即检测新代理
		go TestAllProxies()
	} else {
		log.Printf("[PROXY-FETCH] 没有新代理")
	}
}

func init() {
	if !proxyEnabled {
		return
	}

	// 优先读配置文件；缺失/为空则用默认列表，并生成一份默认配置文件供后续热加载编辑。
	cfg := loadProxyConfigFile()
	if len(cfg) == 0 {
		cfg = buildDefaultProxyList()
		if err := os.WriteFile(proxyConfigPath,
			[]byte("# 每行一个代理：支持 socks5:// / http:// / https:// 或裸 host:port（默认 socks5）\n"+
				strings.Join(cfg, "\n")+"\n"), 0644); err != nil {
			log.Printf("[PROXY] 写默认配置文件失败: %v", err)
		}
	}
	poolMu.Lock()
	proxyList = cfg
	for _, p := range cfg {
		proxyHealth[p] = true
	}
	poolMu.Unlock()
	log.Printf("[PROXY] 初始化代理池: %d 个 (%v)", len(proxyList), proxyList)

	// 热加载：每 10s 轮询配置文件，改动实时生效，无需重启/重新打包。
	go func() {
		var lastMod time.Time
		for range time.NewTicker(10 * time.Second).C {
			if st, err := os.Stat(proxyConfigPath); err == nil && !st.ModTime().Equal(lastMod) {
				lastMod = st.ModTime()
				reloadProxyPool()
			}
		}
	}()

	// 周期健康检测：失效代理自动分摊，恢复后自动重新启用。
	go func() {
		for range time.NewTicker(30 * time.Second).C {
			go TestAllProxies()
		}
	}()

	// 每 24 小时重置代理健康状态：IP 封禁约 24 小时自动解封，恢复全部代理。
	// 同时清除所有风控冷却记录（12h 冷却本应先行到期，这里作为兜底）。
	go func() {
		for range time.NewTicker(24 * time.Hour).C {
			poolMu.Lock()
			for p := range proxyHealth {
				proxyHealth[p] = true
			}
			proxyCooldownUntil = map[string]time.Time{}
			proxyCode32 = map[string]map[int64]time.Time{}
			proxyTimeout = map[string]map[int64]time.Time{}
			poolMu.Unlock()
			log.Printf("[PROXY] 24小时重置代理池，恢复默认代理")
		}
	}()

	// 每 30 分钟清理过期客户端缓存，防止连接堆积。
	go func() {
		for range time.NewTicker(30 * time.Minute).C {
			assignMu.Lock()
			for id := range accountClient {
				delete(accountClient, id)
				delete(accountProxy, id)
				delete(accountAvoid, id)
			}
			assignMu.Unlock()
			log.Printf("[PROXY] 清理客户端缓存，释放全部连接")
		}
	}()
}
