package netproxy

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/adb-lanlu/prism-oss/internal/console"
)

type Mode int

const (
	ModeDirect Mode = iota
	ModeTunnel
	ModeShortPool
)

type Manager struct {
	mode        Mode
	direct      *http.Client
	tunnelURL   *url.URL
	tunnelLabel string
	tunnelMu    sync.Mutex
	tunnelCache *http.Client
	shortPool   *shortProxyPool
}

type shortProxyPool struct {
	fetchURL      string
	username      string
	password      string
	rotateEvery   int
	failThreshold int
	timeout       time.Duration

	mu           sync.Mutex
	currentURL   *url.URL
	currentLabel string
	currentHTTP  *http.Client
	useCount     int
	failCount    int
	rotating     bool
	lastRotateAt time.Time
	lastErr      string
}

func Setup(reader *bufio.Reader) (*Manager, error) {
	fmt.Println()
	fmt.Println(console.Title("网络模式"))
	fmt.Println("1) 直连（不使用代理）")
	fmt.Println("2) 隧道代理（固定代理）")
	fmt.Println("3) 短效代理池（提取接口）")
	mode, err := console.PromptChoice(reader, "请选择 1/2/3（输入 q 可退出）: ", 1, 3)
	if err != nil {
		return nil, err
	}

	switch mode {
	case 1:
		return &Manager{mode: ModeDirect, direct: newDirectHTTPClient()}, nil
	case 2:
		addr, err := console.PromptText(reader, "请输入隧道代理地址（示例 1.2.3.4:8080 或 http://1.2.3.4:8080）: ")
		if err != nil {
			return nil, err
		}
		username, err := console.PromptOptionalText(reader, "代理用户名（可回车留空）: ")
		if err != nil {
			return nil, err
		}
		password, err := console.PromptOptionalText(reader, "代理密码（可回车留空）: ")
		if err != nil {
			return nil, err
		}
		proxyURL, label, err := buildProxyURL(addr, username, password)
		if err != nil {
			return nil, err
		}
		return &Manager{mode: ModeTunnel, tunnelURL: proxyURL, tunnelLabel: label}, nil
	case 3:
		fetchURL, err := console.PromptText(reader, "请输入短效代理提取接口 URL: ")
		if err != nil {
			return nil, err
		}
		username, err := console.PromptOptionalText(reader, "代理认证用户名（可回车留空）: ")
		if err != nil {
			return nil, err
		}
		password, err := console.PromptOptionalText(reader, "代理认证密码（可回车留空）: ")
		if err != nil {
			return nil, err
		}
		rotateEvery, err := console.PromptIntWithDefaultMin(reader, "每多少次登录尝试自动换 IP（回车默认 20）: ", 20, 1)
		if err != nil {
			return nil, err
		}
		failThreshold, err := console.PromptIntWithDefaultMin(reader, "连续失败多少次强制换 IP（回车默认 4）: ", 4, 1)
		if err != nil {
			return nil, err
		}
		timeoutSec, err := console.PromptIntWithDefaultMin(reader, "提取接口超时秒数（回车默认 8）: ", 8, 3)
		if err != nil {
			return nil, err
		}

		pool := &shortProxyPool{
			fetchURL:      fetchURL,
			username:      username,
			password:      password,
			rotateEvery:   rotateEvery,
			failThreshold: failThreshold,
			timeout:       time.Duration(timeoutSec) * time.Second,
		}

		if _, label, err := pool.acquireHTTPClient(); err != nil {
			return nil, fmt.Errorf("初始化短效代理失败: %w", err)
		} else {
			fmt.Printf("短效代理池初始化成功，当前代理=%s\n", label)
		}
		return &Manager{mode: ModeShortPool, shortPool: pool}, nil
	default:
		return nil, fmt.Errorf("不支持的网络模式")
	}
}

func (m *Manager) Describe() string {
	if m == nil {
		return "直连"
	}
	switch m.mode {
	case ModeDirect:
		return "直连"
	case ModeTunnel:
		return "隧道代理(" + displayProxyLabel(m.tunnelLabel) + ")"
	case ModeShortPool:
		return "短效代理池"
	default:
		return "未知"
	}
}

func displayProxyLabel(label string) string {
	if strings.TrimSpace(label) == "" {
		return "unknown"
	}
	return label
}

func (m *Manager) AcquireHTTPClient() (*http.Client, string, error) {
	if m == nil {
		return newDirectHTTPClient(), "直连", nil
	}
	switch m.mode {
	case ModeDirect:
		if m.direct == nil {
			m.direct = newDirectHTTPClient()
		}
		return m.direct, "直连", nil
	case ModeTunnel:
		if m.tunnelURL == nil {
			return nil, "", fmt.Errorf("隧道代理未初始化")
		}
		m.tunnelMu.Lock()
		if m.tunnelCache == nil {
			m.tunnelCache = newProxyHTTPClient(m.tunnelURL)
		}
		client := m.tunnelCache
		m.tunnelMu.Unlock()
		return client, m.tunnelLabel, nil
	case ModeShortPool:
		if m.shortPool == nil {
			return nil, "", fmt.Errorf("短效代理池未初始化")
		}
		client, label, err := m.shortPool.acquireHTTPClient()
		if err != nil {
			return nil, "", err
		}
		return client, label, nil
	default:
		return nil, "", fmt.Errorf("未知网络模式")
	}
}

func (m *Manager) ReportResult(err error) {
	if m == nil {
		return
	}
	if m.mode == ModeShortPool && m.shortPool != nil {
		m.shortPool.report(err)
	}
}

func (p *shortProxyPool) acquireHTTPClient() (*http.Client, string, error) {
	for i := 0; i < 8; i++ {
		p.mu.Lock()
		client := p.currentHTTP
		label := p.currentLabel
		needRotate := p.needRotateLocked()

		if client != nil {
			p.useCount++
			if needRotate && !p.rotating {
				p.rotating = true
				go p.rotateAsync()
			}
			p.mu.Unlock()
			return client, label, nil
		}

		if !p.rotating {
			p.rotating = true
			p.mu.Unlock()
			if err := p.rotateSync(); err != nil {
				return nil, "", err
			}
			continue
		}
		p.mu.Unlock()
		time.Sleep(80 * time.Millisecond)
	}

	p.mu.Lock()
	lastErr := p.lastErr
	p.mu.Unlock()
	if strings.TrimSpace(lastErr) == "" {
		lastErr = "代理池初始化超时"
	}
	return nil, "", fmt.Errorf("%s", lastErr)
}

func (p *shortProxyPool) report(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err == nil {
		p.failCount = 0
		return
	}
	if isRateLimitedError(err) {
		p.failCount = p.failThreshold
		if !p.rotating && p.currentHTTP != nil {
			p.rotating = true
			go p.rotateAsync()
		}
		return
	}

	p.failCount++
	if p.failThreshold > 0 && p.failCount >= p.failThreshold && !p.rotating && p.currentHTTP != nil {
		p.rotating = true
		go p.rotateAsync()
	}
}

func (p *shortProxyPool) needRotateLocked() bool {
	needRotate := p.currentURL == nil
	if !needRotate && p.rotateEvery > 0 && p.useCount >= p.rotateEvery {
		needRotate = true
	}
	if !needRotate && p.failThreshold > 0 && p.failCount >= p.failThreshold {
		needRotate = true
	}
	return needRotate
}

func (p *shortProxyPool) rotateSync() error {
	proxyURL, label, err := p.fetchAndBuildProxy()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rotating = false
	if err != nil {
		p.lastErr = err.Error()
		return err
	}
	p.applyRotatedProxyLocked(proxyURL, label)
	return nil
}

func (p *shortProxyPool) rotateAsync() {
	proxyURL, label, err := p.fetchAndBuildProxy()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rotating = false
	if err != nil {
		p.lastErr = err.Error()
		return
	}
	p.applyRotatedProxyLocked(proxyURL, label)
}

func (p *shortProxyPool) fetchAndBuildProxy() (*url.URL, string, error) {
	raw, err := fetchProxyRaw(p.fetchURL, p.timeout)
	if err != nil {
		return nil, "", err
	}
	addr, err := parseProxyAddressFromBody(raw)
	if err != nil {
		return nil, "", err
	}
	proxyURL, label, err := buildProxyURL(addr, p.username, p.password)
	if err != nil {
		return nil, "", err
	}
	return proxyURL, label, nil
}

func (p *shortProxyPool) applyRotatedProxyLocked(proxyURL *url.URL, label string) {
	p.currentURL = proxyURL
	p.currentLabel = label
	p.currentHTTP = newProxyHTTPClient(proxyURL)
	p.useCount = 0
	p.failCount = 0
	p.lastErr = ""
	p.lastRotateAt = time.Now()
}

func fetchProxyRaw(fetchURL string, timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, fetchURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	body := strings.TrimSpace(string(bodyBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("提取代理失败 status=%d body=%s", resp.StatusCode, truncateText(body, 180))
	}
	if body == "" {
		return "", fmt.Errorf("提取代理接口返回为空")
	}
	if strings.Contains(body, "超过额度") || strings.Contains(strings.ToLower(body), "quota") {
		return "", fmt.Errorf("提取代理失败: %s", truncateText(body, 180))
	}
	return body, nil
}

func parseProxyAddressFromBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", fmt.Errorf("代理返回为空")
	}

	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if addr := extractProxyAddress(line); addr != "" {
			return addr, nil
		}
	}

	if addr := extractProxyAddress(body); addr != "" {
		return addr, nil
	}
	return "", fmt.Errorf("无法从返回内容解析代理地址: %s", truncateText(body, 180))
}

var proxyAddrRegexp = regexp.MustCompile(`(?i)(https?://[^\s"']+|[a-z0-9\.\-]+:\d{2,5})`)

func extractProxyAddress(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	matches := proxyAddrRegexp.FindAllString(text, -1)
	for _, item := range matches {
		item = strings.Trim(item, "\"' ,")
		if strings.Contains(item, "://") {
			if u, err := url.Parse(item); err == nil && u.Host != "" {
				return u.String()
			}
			continue
		}
		if strings.Count(item, ":") == 1 {
			return item
		}
	}
	return ""
}

func buildProxyURL(addr, username, password string) (*url.URL, string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, "", fmt.Errorf("代理地址不能为空")
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(u.Host) == "" {
		return nil, "", fmt.Errorf("代理地址格式无效: %s", addr)
	}
	if strings.TrimSpace(username) != "" {
		u.User = url.UserPassword(username, password)
	}
	return u, u.Host, nil
}

func newDirectHTTPClient() *http.Client {
	transport := &http.Transport{
		MaxIdleConns:          300,
		MaxIdleConnsPerHost:   120,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 25 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: 45 * time.Second}
}

func newProxyHTTPClient(proxyURL *url.URL) *http.Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyURL(proxyURL),
		MaxIdleConns:          300,
		MaxIdleConnsPerHost:   120,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 25 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: 45 * time.Second}
}

func truncateText(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}

func isRateLimitedError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "code: 2100") ||
		strings.Contains(s, "操作过于频繁") ||
		strings.Contains(s, "too many") ||
		strings.Contains(s, "rate limit")
}
