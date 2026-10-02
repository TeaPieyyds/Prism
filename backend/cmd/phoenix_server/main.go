package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	g79 "github.com/Yeah114/g79client"
	"github.com/Yeah114/g79client/account/mpay"
	"github.com/Yeah114/g79client/service/link_connection"
	g79utils "github.com/Yeah114/g79client/utils"
	"github.com/Yeah114/unmcpk"
	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/authsvc"
	"github.com/adb-lanlu/prism-oss/internal/db"
	"golang.org/x/crypto/bcrypt"
)

func init() {
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 900 * time.Millisecond}
			return d.DialContext(ctx, "udp", "8.8.8.8:53")
		},
	}
	auth.RegisterGlobalLimiter(globalIPLimiter) // 全局 IP 限流器参与后台清扫
}

type LoginReq struct {
	FBToken         string `json:"login_token,omitempty"`
	Authorization   string `json:"authorization,omitempty"`
	UserName        string `json:"username,omitempty"`
	Password        string `json:"password,omitempty"`
	ServerCode      string `json:"server_code"`
	ServerPassword  string `json:"server_passcode"`
	ClientPublicKey string `json:"client_public_key"`
}

type LoginResp struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	ServerMsg   string `json:"server_msg"`
	Token       string `json:"token"`
	RespondTo   string `json:"respond_to"`
	IPAddress   string `json:"ip_address"`
	UID         string `json:"uid"`
	Username    string `json:"username"`
	GrowthLevel int    `json:"growth_level"`
	SkinInfo    any    `json:"skin_info"`
	OutfitInfo  any    `json:"outfit_info,omitempty"`
	ChainInfo   string `json:"chainInfo"`
	Verify      string `json:"verify"`
	NewVerify   string `json:"new_verify"`
}

var (
	nv1Sid   string
	nv1SidMu sync.Mutex

	// Toggle NV1 proxy for each API endpoint (true = proxy to NV1)
	// proxyLogin removed
	proxyTransferStartType = false
	proxyTransferCheckNum  = false

	nv1ProxyUID   string
	nv1ProxyUIDMu sync.Mutex
	cookieStr     string
	sessionMu     sync.Mutex
	client        *g79.Client
	lastAuth      time.Time
	curGameLevel  int
	accountMu     sync.Mutex
	accounts      = map[string]*Account{}
	accFile       = "accounts_v2.json"
	sessMu        sync.RWMutex
	sessions      = map[string]*sessionData{}

	// GuestPool — anonymous guest access
	guestPoolMu      sync.Mutex
	guestAccount     *db.GameAccount
	guestPickedAt    time.Time
	guestAccountID   int64
	guestRotateEvery = 10 * time.Minute
)

type sessionData struct {
	UserID        string
	WebUserID     int64
	EngineVersion string
	PatchVersion  string
	GameEngineVer string
	GamePatchVer  string
	IsPC          bool
	LoginToken    string
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

var (
	g79OverrideMu  sync.RWMutex
	g79OverrideMap = map[int64]bool{} // web user ID -> challenge override active
)

// ── Rate limiters (one per endpoint, isolated state) ──
var (
	registerLimiter       = auth.NewRateLimiter(1, 18*time.Second) // POST /api/auth/register
	captchaAnswers        = map[string]*captchaEntry{}
	captchaAnswersMu      sync.RWMutex
	captchaNewLimiter     = auth.NewRateLimiter(5, 2*time.Second) // POST /api/auth/captcha/new（放宽：验证码由预生成池返回，开销小）
	captchaMaxAttempts    = 3
	verifyLimiter         = auth.NewRateLimiter(5, 60*time.Second)                     // GET /api/auth/verify（仅渲染确认页，放宽避免误杀）
	verifyConfirmLimiter  = auth.NewRateLimiter(5, 60*time.Second)                     // POST /api/auth/verify/confirm（独立限流，避免与 GET 共享桶）
	loginLimiter          = auth.NewRateLimiter(1, 900*time.Millisecond)               // POST /api/auth/login
	newSessionLimiter     = auth.NewRateLimiter(1, 900*time.Millisecond)               // GET /api/new
	guestCreateLimiter    = auth.NewRateLimiter(1, 36*time.Second)                     // POST /api/mpay/guest
	guestSmsLimiter       = auth.NewRateLimiter(1, 18*time.Second)                     // POST /api/mpay/guest/send-verify-sms
	guestContinueLimiter  = auth.NewRateLimiter(1, 9*time.Second)                      // POST /api/mpay/guest/continue
	guestRealnameLimiter  = auth.NewRateLimiter(1, 9*time.Second)                      // POST /api/mpay/guest/realname
	guestNicknameLimiter  = auth.NewRateLimiter(1, 9*time.Second)                      // POST /api/mpay/guest/nickname
	phoneSmsLimiter       = auth.NewRateLimiter(1, 18*time.Second)                     // POST /api/mpay/phone/sms
	phoneVerifyLimiter    = auth.NewRateLimiter(1, 9*time.Second)                      // POST /api/mpay/phone/verify
	mpayLoginLimiter      = auth.NewRateLimiter(1, 3*time.Second)                      // POST /api/mpay/login
	challengeOverrideLim  = auth.NewRateLimiter(1, 3*time.Second)                      // POST /api/auth/challenge-override
	profileLimiter        = auth.NewRateLimiter(1, 3*time.Second)                      // POST /api/auth/profile
	passwordLimiter       = auth.NewRateLimiter(1, 9*time.Second)                      // POST /api/auth/password
	resetTokenLimiter     = auth.NewRateLimiter(1, 9*time.Second)                      // POST /api/auth/reset-token
	forgotPwdLimiter      = auth.NewRateLimiter(1, 18*time.Second)                     // POST /api/auth/forgot-password
	changeEmailLimiter    = auth.NewRateLimiter(1, 9*time.Second)                      // POST /api/auth/change-email
	redeemLimiter         = auth.NewRateLimiter(1, 3*time.Second)                      // POST /api/auth/redeem (1次/3秒)
	payOrderLimiter       = auth.NewRateLimiter(3, 60*time.Second)                     // POST /api/payment/order
	nameChangeLimiter     = auth.NewRateLimiter(1, 5*time.Minute)                      // POST /api/auth/profile — 改名限频
	phoenixLoginLimiter   = auth.NewRateLimiter(10, 10*time.Second)                    // POST /api/phoenix/login — 10req/10s
	tanLobbyCreateLimiter = auth.NewRateLimiter(1, 3*time.Second)                      // POST /api/phoenix/tan_lobby_create
	tanLobbyTargetLimiter = auth.NewRateLimiter(1, 3*time.Second)                      // POST /api/phoenix/tan_lobby_rental_target
	globalIPLimiter       = auth.NewGlobalIPLimiter(40, 1*time.Second, 30*time.Minute) // 40req/s → ban 30min
	validUsername         = regexp.MustCompile(`^[\p{Han}a-zA-Z0-9_]+$`)
	domainChoiceRe        = regexp.MustCompile(`[_\-](\d+)$`)       // 用户名：中文/英文/数字/下划线
	enableJoinCost        = true                                    // 进服扣板栗开关
	enableActivationCheck = true                                    // 激活码检查开关（true=超过3次需激活才能继续使用）
	enableLobby           = true                                    // 联机大厅开关（false=拒绝所有以 # 开头的联机房间请求）
	phoenixStartLimiter   = auth.NewRateLimiter(10, 10*time.Second) // GET /api/phoenix/transfer_start_type — waits
	phoenixCheckLimiter   = auth.NewRateLimiter(10, 10*time.Second) // POST /api/phoenix/transfer_check_num — waits
	heartbeatLimiter      = auth.NewRateLimiter(4, 15*time.Second) // POST /api/v2/heartbeat — 客户端每15s心跳；限流留4倍余量防误杀，仅拦截远超15s节奏的单设备滥用
	// 未鉴权读取端点限流（本地前置拦截，仅拒绝超额；阈值保守、可配置）
	serverListLimiter   = auth.NewRateLimiter(6, 1*time.Second) // /api/phoenix/server/list 等读取
	serverSearchLimiter = auth.NewRateLimiter(6, 1*time.Second) // /api/phoenix/server/search
	socialLimiter       = auth.NewRateLimiter(4, 1*time.Second) // /api/phoenix/social/*
	domainLimiter       = auth.NewRateLimiter(4, 1*time.Second) // /api/phoenix/domain/*
	lobbyLimiter        = auth.NewRateLimiter(4, 1*time.Second) // /api/phoenix/lobby/*
	transferRoomLimiter = auth.NewRateLimiter(2, 1*time.Second) // /api/phoenix/transfer_room
	hotLimiter          = auth.NewRateLimiter(6, 1*time.Second) // /api/phoenix/hot
)

// maxAnonymousDevices 匿名心跳设备数上限，防止攻击者伪造无限 device_id 撑爆 user_stats/heartbeat_logs。
const maxAnonymousDevices = 100000

// 山头服邀请码→sid 缓存（避免多个服时选错）
var (
	domainSidCache   = map[string]string{}
	domainSidCacheMu sync.RWMutex
	pendingBan       = map[int64]time.Time{}
	pendingMu        sync.Mutex
)

// leaveAllDomainServers 清除账号已加入的所有山头服，并通过轮询确认清空。
// 必须用 del-other-server（DeleteOtherDomainServer）从"已加入列表"移除；
// req-leave（RequestLeaveDomainServer）只退出当前会话、不移除列表，无法清空。
// 删除是异步的，故反复删除 + 轮询列表直到为空或达上限。终止条件由列表是否为空
// 决定（服务端事实源），不依赖删除响应码。
func leaveAllDomainServers(c *g79.Client) {
	for attempt := 0; attempt < 6; attempt++ {
		list, err := c.GetOtherDomainServers()
		if err == nil && (list == nil || len(list.Entities) == 0) {
			log.Printf("domain _0: all servers left (attempt %d)", attempt)
			return
		}
		if list != nil {
			for _, e := range list.Entities {
				if _, lErr := c.DeleteOtherDomainServer(e.Sid); lErr == nil {
					log.Printf("domain _0: removed server sid=%s", e.Sid)
				} else {
					log.Printf("domain _0: remove sid=%s failed (err=%v), will retry", e.Sid, lErr)
				}
			}
		}
		time.Sleep(500 * time.Millisecond) // 给异步删除传播时间
	}
	log.Printf("domain _0: leave-all reached attempt cap, some servers may remain")
}

func getBearer(r *http.Request) string {
	if a := r.Header.Get("Authorization"); len(a) > 7 && strings.EqualFold(a[:7], "Bearer ") {
		return a[7:]
	}
	return ""
}

// ── Guest pool (anonymous access) ──

// getGuestAccount returns the current guest pool account.
// Rotates every guestRotateEvery (10 min) by picking a random named shared account.
// Returns nil if the pool is empty.
func getGuestAccount() *db.GameAccount {
	guestPoolMu.Lock()
	defer guestPoolMu.Unlock()

	// Check if rotation is needed
	if guestAccount == nil || time.Since(guestPickedAt) > guestRotateEvery {
		log.Printf("[GUEST-DEBUG] getGuestAccount: picking new account")
		accs, err := db.GetNamedSharedAccounts()
		if err != nil || len(accs) == 0 {
			log.Printf("[GUEST-DEBUG] GetNamedSharedAccounts: err=%v len=%d", err, len(accs))
			guestAccount = nil
			guestAccountID = 0
			return nil
		}
		// Pick a random one (the query already uses RANDOM())
		guestAccount = accs[0]
		guestPickedAt = time.Now()
		guestAccountID = guestAccount.ID
		log.Printf("[GUEST] pool rotated → account #%d (%s)", guestAccount.ID, guestAccount.DisplayName)
	}
	return guestAccount
}

// forceRotateGuestAccount forces the guest pool to pick a new account immediately.
func forceRotateGuestAccount() *db.GameAccount {
	guestPoolMu.Lock()
	oldID := guestAccountID
	guestAccount = nil
	guestPickedAt = time.Time{}
	guestAccountID = 0
	guestPoolMu.Unlock()
	// Also invalidate g79 cache for the old account
	if oldID != 0 {
		g79CacheMu.Lock()
		delete(g79Cache, oldID)
		g79CacheMu.Unlock()
	}
	// Re-acquire with rotation (getGuestAccount takes its own lock)
	return getGuestAccount()
}

// isGuestAccount reports whether the given account ID is the current guest account.
func isGuestAccount(accID int64) bool {
	guestPoolMu.Lock()
	defer guestPoolMu.Unlock()
	return guestAccount != nil && guestAccount.ID == accID
}

type Account struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UID         string `json:"uid"`
	Cookie      string `json:"cookie"` // g79 cookie JSON
	Token       string `json:"token"`
	IsGuest     bool   `json:"is_guest"`
	DeviceID    string `json:"device_id"`
	CreatedAt   int64  `json:"created_at"`
}

var cookieStore = NewCookieStore("cookies.json")

type StoredCookie struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	UID                string   `json:"uid"`
	Cookie             string   `json:"cookie"`
	Status             string   `json:"status"`
	GrowthLevel        int      `json:"growth_level"`
	DisplayName        string   `json:"display_name"`
	Account            string   `json:"account,omitempty"`
	Source             string   `json:"source,omitempty"`
	AvatarImageURL     string   `json:"avatar_image_url,omitempty"`
	Signature          string   `json:"signature,omitempty"`
	Score              int64    `json:"score,omitempty"`
	SkinNumber         int64    `json:"skin_number,omitempty"`
	CapeNumber         int64    `json:"cape_number,omitempty"`
	AccessGameFlag     int64    `json:"access_game_flag,omitempty"`
	AntiAdditionStatus int64    `json:"anti_addition_status,omitempty"`
	RealnameStatus     int64    `json:"realname_status,omitempty"`
	RechargeVIPLevel   int64    `json:"recharge_vip_level,omitempty"`
	IsVIP              bool     `json:"is_vip,omitempty"`
	IsSubscribe        bool     `json:"is_subscribe,omitempty"`
	BanInfo            *BanInfo `json:"ban_info,omitempty"`
	CreatedAt          int64    `json:"created_at"`
	LastUsed           int64    `json:"last_used"`
}

type BanInfo struct {
	Reason    string `json:"reason"`
	BanType   string `json:"ban_type"`
	ExpiredAt int64  `json:"expired_at"`
}

type CookieStore struct {
	mu             sync.RWMutex
	Cookies        map[string]*StoredCookie `json:"cookies"`
	ActiveID       string                   `json:"active_id"`
	filePath       string
	lastRefresh    map[string]time.Time
	cycleRemaining []string
}

func NewCookieStore(filePath string) *CookieStore {
	return &CookieStore{
		Cookies:     make(map[string]*StoredCookie),
		filePath:    filePath,
		lastRefresh: make(map[string]time.Time),
	}
}

func (cs *CookieStore) Load() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	data, err := os.ReadFile(cs.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, cs)
}

func (cs *CookieStore) Save() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	data, err := json.MarshalIndent(cs, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(cs.filePath, data, 0644)
}

func (cs *CookieStore) Add(sc *StoredCookie) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if sc.ID == "" {
		b := make([]byte, 8)
		rand.Read(b)
		sc.ID = fmt.Sprintf("ck_%x", b)
	}
	for id, existing := range cs.Cookies {
		if sc.UID != "" && existing.UID == sc.UID {
			sc.ID = id
			sc.CreatedAt = existing.CreatedAt
			if sc.Cookie == "" {
				sc.Cookie = existing.Cookie
			}
			cs.Cookies[id] = sc
			return
		}
		if sc.Cookie != "" && existing.Cookie == sc.Cookie {
			sc.ID = id
			sc.CreatedAt = existing.CreatedAt
			cs.Cookies[id] = sc
			return
		}
	}
	cs.Cookies[sc.ID] = sc
}

func (cs *CookieStore) AddIfMissing(sc *StoredCookie) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if sc.ID == "" {
		b := make([]byte, 8)
		rand.Read(b)
		sc.ID = fmt.Sprintf("ck_%x", b)
	}
	for id, existing := range cs.Cookies {
		if sc.UID != "" && existing.UID == sc.UID {
			if existing.Cookie == "" && sc.Cookie != "" {
				existing.Cookie = sc.Cookie
			}
			if existing.Status == "unknown" && sc.Status != "" {
				existing.Status = sc.Status
			}
			if existing.DisplayName == "" || strings.HasPrefix(existing.DisplayName, "未验证账号_") {
				existing.DisplayName = sc.DisplayName
				existing.Name = sc.Name
			}
			cs.Cookies[id] = existing
			return false
		}
		if sc.Cookie != "" && existing.Cookie == sc.Cookie {
			return false
		}
	}
	cs.Cookies[sc.ID] = sc
	return true
}

func (cs *CookieStore) Get(id string) (*StoredCookie, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	sc, ok := cs.Cookies[id]
	return sc, ok
}

func (cs *CookieStore) Delete(id string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.Cookies, id)
	if cs.ActiveID == id {
		cs.ActiveID = ""
	}
}

func (cs *CookieStore) SetActive(id string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if _, ok := cs.Cookies[id]; !ok {
		return false
	}
	cs.ActiveID = id
	cs.Cookies[id].LastUsed = time.Now().Unix()
	return true
}

func (cs *CookieStore) CanRefresh(id string) (bool, int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.lastRefresh == nil {
		cs.lastRefresh = make(map[string]time.Time)
	}
	last, ok := cs.lastRefresh[id]
	if !ok {
		cs.lastRefresh[id] = time.Now()
		return true, 0
	}
	elapsed := int(time.Since(last).Seconds())
	if elapsed >= 4 {
		cs.lastRefresh[id] = time.Now()
		return true, 0
	}
	return false, elapsed
}

func (cs *CookieStore) Rotate() (*StoredCookie, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// collect normal (usable) cookie ids
	allIds := []string{}
	for id, sc := range cs.Cookies {
		if sc.Status == "normal" && strings.TrimSpace(sc.Cookie) != "" {
			allIds = append(allIds, id)
		}
	}
	if len(allIds) == 0 {
		return nil, errors.New("没有可用的普通账号")
	}
	// refill cycle if empty or invalid
	if len(cs.cycleRemaining) == 0 {
		cs.cycleRemaining = make([]string, len(allIds))
		copy(cs.cycleRemaining, allIds)
		// shuffle
		for i := len(cs.cycleRemaining) - 1; i > 0; i-- {
			j := rand.Intn(i + 1)
			cs.cycleRemaining[i], cs.cycleRemaining[j] = cs.cycleRemaining[j], cs.cycleRemaining[i]
		}
	}
	// clean stale ids from cycle
	filtered := []string{}
	for _, cid := range cs.cycleRemaining {
		if sc, ok := cs.Cookies[cid]; ok && sc.Status == "normal" && sc.Cookie != "" {
			filtered = append(filtered, cid)
		}
	}
	cs.cycleRemaining = filtered
	if len(cs.cycleRemaining) == 0 {
		cs.cycleRemaining = make([]string, len(allIds))
		copy(cs.cycleRemaining, allIds)
		for i := len(cs.cycleRemaining) - 1; i > 0; i-- {
			j := rand.Intn(i + 1)
			cs.cycleRemaining[i], cs.cycleRemaining[j] = cs.cycleRemaining[j], cs.cycleRemaining[i]
		}
	}
	// pick the last one (pop)
	idx := len(cs.cycleRemaining) - 1
	pick := cs.cycleRemaining[idx]
	cs.cycleRemaining = cs.cycleRemaining[:idx]
	cs.ActiveID = pick
	cs.Cookies[pick].LastUsed = time.Now().Unix()
	return cs.Cookies[pick], nil
}

func (cs *CookieStore) GetActiveCookie() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.ActiveID == "" {
		return ""
	}
	if sc, ok := cs.Cookies[cs.ActiveID]; ok {
		return sc.Cookie
	}
	return ""
}

func (cs *CookieStore) DedupeByUID() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	seen := map[string]string{}
	removed := 0
	for id, sc := range cs.Cookies {
		if sc.UID == "" {
			continue
		}
		if keepID, ok := seen[sc.UID]; ok {
			keep := cs.Cookies[keepID]
			if keep.Cookie == "" && sc.Cookie != "" {
				keep.Cookie = sc.Cookie
			}
			if keep.Status != "normal" && sc.Status == "normal" {
				sc.ID = keepID
				sc.CreatedAt = keep.CreatedAt
				cs.Cookies[keepID] = sc
			}
			delete(cs.Cookies, id)
			if cs.ActiveID == id {
				cs.ActiveID = keepID
			}
			removed++
			continue
		}
		seen[sc.UID] = id
	}
	return removed
}

func loadAccounts() {
	data, err := os.ReadFile(accFile)
	if err != nil {
		return
	}
	json.Unmarshal(data, &accounts)
}

func saveAccounts() {
	accountMu.Lock()
	defer accountMu.Unlock()
	data, _ := json.MarshalIndent(accounts, "", "  ")
	os.WriteFile(accFile, data, 0644)
}

// saveGuestCookieToFile 将需要验证的游客Cookie存入恢复文件
func saveGuestCookieToFile(cookie, uid, source string) {
	f, err := os.OpenFile("mpay_guest_cookies.jsonl", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("[MPAY] 保存恢复文件失败: %v", err)
		return
	}
	defer f.Close()
	record := M{"cookie": cookie, "uid": uid, "source": source, "saved_at": time.Now().Format(time.RFC3339)}
	b, _ := json.Marshal(record)
	f.Write(append(b, '\n'))
	log.Printf("[MPAY] Cookie已保存到恢复文件: uid=%s", uid)
}
type pendingGuestAccount struct {
	Cookie        string
	UID           string
	Token         string
	Nickname      string
	UDID          string // uni_sauth 所需
	ClientLoginSN string
	TransID       string
	MCountTid     string
}

type guestVerifyFlow struct {
	Device          *mpay.Device
	Ticket          string
	VerifyURL       string
	CreatedAt       time.Time
	Verified        bool                  // 短信验证(finish-sms)已通过
	PendingRealname []pendingGuestAccount // 待用户实名(预设失败或未预设)
	PendingRename   []pendingGuestAccount // 待用户改名的账号(前2个实名成功，改名成功才入库)
	GivenCount      int                   // 已发放给用户(改名成功)的账号数
}

// createGuestAccount 生成并保存一个游客账号(不实名，实名统一由用户手动输入后处理)。
// 返回完整账号信息(含 uni_sauth 所需 udid/client_login_sn/transid 等，供改名时取 oauth token)。
func createGuestAccount(r *http.Request, device *mpay.Device, token, sdkuid, source string) pendingGuestAccount {
	udid, _ := mpay.RandomHex(16)
	clsn, _ := mpay.RandomHex(32)
	now := time.Now().UnixMilli()
	transid := fmt.Sprintf("%s_%d_%d", udid, now, rand.Int63n(1e9))
	mcountTid := fmt.Sprintf("%s_%d_%d", udid, now+int64(rand.Intn(10000)+1), rand.Int63n(1e9))
	nickname := fmt.Sprintf("pr_%04d%c", time.Now().UnixNano()%10000, 'a'+time.Now().UnixNano()%26)
	cookie := mpay.GenerateFixedSauthJSON(device.ID, sdkuid, token, udid, clsn, "127.0.0.1", "CN", nickname)
	// 所有账号先存恢复文件(兜底)
	saveGuestCookieToFile(cookie, sdkuid, source)
	return pendingGuestAccount{
		Cookie: cookie, UID: sdkuid, Token: token, Nickname: nickname,
		UDID: udid, ClientLoginSN: clsn, TransID: transid, MCountTid: mcountTid,
	}
}

// tryStockRealname 随机抽一个库存身份证，对传入账号尝试实名。
// 成功→返回其成功实名到的账号数；该身份证对全部账号失败→删除它。返回剩余未实名账号。
func tryStockRealname(flow *guestVerifyFlow, accounts []pendingGuestAccount) []pendingGuestAccount {
	presets, _ := db.GetRealnamePresets() // 只取 enabled
	if len(presets) == 0 {
		return accounts
	}
	p := presets[rand.Intn(len(presets))]
	log.Printf("[MPAY-GUEST] 随机抽取库存身份证尝试: %s", p.Name)
	var remaining []pendingGuestAccount
	var unsent []string
	anyOK := false
	for _, acc := range accounts {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := mpay.AuthRealNameFixed(ctx, PickOneTimeProxy(), flow.Device.ID, acc.UID, acc.Token, p.Name, p.IDNumber)
		cancel()
		if err == nil {
			log.Printf("[MPAY-GUEST] 库存实名成功 uid=%s name=%s", acc.UID, p.Name)
			anyOK = true
			unsent = append(unsent, routeRealnamed(flow, []pendingGuestAccount{acc})...)
			continue
		}
		log.Printf("[MPAY-GUEST] 库存实名失败 uid=%s %s: %v", acc.UID, p.Name, err)
		remaining = append(remaining, acc)
	}
	if len(unsent) > 0 {
		writeGuestFile(guestUnsentDir, unsent)
	}
	if !anyOK {
		if delErr := db.DeleteRealnamePreset(p.ID); delErr != nil {
			log.Printf("[MPAY-GUEST] 删除失效身份证失败 id=%d: %v", p.ID, delErr)
		} else {
			log.Printf("[MPAY-GUEST] 库存身份证失效已删除: %s", p.Name)
		}
	}
	return remaining
}

// routeRealnamed 把新实名的账号路由：填满 PendingRename 前2个供改名发放，超出返回未发送列表。
func routeRealnamed(flow *guestVerifyFlow, accounts []pendingGuestAccount) []string {
	var unsent []string
	for _, acc := range accounts {
		if len(flow.PendingRename) < 2 {
			flow.PendingRename = append(flow.PendingRename, acc)
		} else {
			unsent = append(unsent, acc.Cookie)
		}
	}
	return unsent
}

// 游客账号输出路径(与 test/extracted PY 版一致)
const (
	guestUnsentDir     = "/root/fa/test/extracted/未发送"
	guestUnverifiedDir = "/root/fa/test/extracted/未实名"
)

// writeGuestFile 将 sauth_json 逐行写入 <dir>/<时间戳>.txt，格式同 PY 版。
func writeGuestFile(dir string, lines []string) {
	if len(lines) == 0 {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[MPAY-GUEST] 创建目录失败 %s: %v", dir, err)
		return
	}
	name := time.Now().Format("0102_150405") + ".txt"
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		log.Printf("[MPAY-GUEST] 写入失败 %s: %v", filepath.Join(dir, name), err)
		return
	}
	log.Printf("[MPAY-GUEST] 已写入 %d 个到 %s", len(lines), filepath.Join(dir, name))
}

func loadCookie() string { return "" }
func loadCookieOld() string {
	if c := cookieStore.GetActiveCookie(); c != "" {
		return c
	}
	if cookieStr != "" {
		return cookieStr
	}
	data, _ := os.ReadFile("cookie.json")
	cookieStr = strings.TrimSpace(string(data))
	return cookieStr
}

func currentNv1Session() string {
	nv1SidMu.Lock()
	defer nv1SidMu.Unlock()
	return nv1Sid
}

func initNv1Session() {
	go func() {
		resp, err := http.Get("https://example.com/api/new")
		if err != nil {
			log.Printf("nv1 init: %v", err)
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		nv1SidMu.Lock()
		sid := strings.TrimSpace(string(body))
		if len(sid) > 20 {
			sid = sid[:20]
		}
		nv1Sid = sid
		nv1SidMu.Unlock()
		log.Printf("nv1 session: %s", sid)
	}()
}

func getClient() (*g79.Client, error) { return nil, nil }
func getClientOld() (*g79.Client, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if client != nil && time.Since(lastAuth) < 30*time.Minute {
		return client, nil
	}
	time.Sleep(time.Duration(500+rand.Intn(1500)) * time.Millisecond)
	c, err := g79.NewClient()
	if err != nil {
		return nil, err
	}
	if err := c.G79AuthenticateWithCookie(loadCookie()); err != nil {
		return nil, err
	}
	ud, udErr := c.GetUserDetail()
	if udErr != nil {
		log.Printf("GetUserDetail error: %v", udErr)
	} else {
		log.Printf("DETAIL: Level=%q Score=%q Skin=%q Cape=%q VIP=%q Realname=%q",
			ud.Entity.Level.Raw, ud.Entity.Score.Raw, ud.Entity.SkinNumber.Raw,
			ud.Entity.CapeNumber.Raw, ud.Entity.RechargeVIPLevel.Raw, ud.Entity.RealnameStatus.Raw)
	}
	c.UserDetail = &ud.Entity
	client = c
	lastAuth = time.Now()
	gameLevel := int(c.UserDetail.Level.Int64())
	if od, oerr := c.GetOtherUserDetail(c.UserID, true); oerr == nil && od != nil {
		peLv := int(od.Entity.PEGrowth.Lv.Int64())
		if peLv > gameLevel {
			gameLevel = peLv
		}
		log.Printf("PEGrowth lv=%d for %s", peLv, c.UserDetail.Name)
	} else if oerr != nil {
		log.Printf("OtherDetail err: %v", oerr)
	}
	curGameLevel = gameLevel
	log.Printf("PE OK: id=%s name=%s lv=%d", c.UserID, c.UserDetail.Name, gameLevel)
	return client, nil
}

func fillStoredCookieFromClient(sc *StoredCookie, c *g79.Client) {
	if c == nil || c.UserDetail == nil {
		return
	}
	ud := c.UserDetail
	sc.Name = ud.Name
	sc.UID = c.UserID
	sc.DisplayName = ud.Name
	sc.Account = ud.Account
	sc.AvatarImageURL = ud.AvatarImageURL
	sc.Signature = ud.Signature
	sc.GrowthLevel = int(ud.Level.Int64())
	sc.Score = ud.Score.Int64()
	sc.SkinNumber = ud.SkinNumber.Int64()
	sc.CapeNumber = ud.CapeNumber.Int64()
	sc.AccessGameFlag = ud.AccessGameFlag.Int64()
	sc.AntiAdditionStatus = ud.AntiAdditionStatus.Int64()
	sc.RealnameStatus = ud.RealnameStatus.Int64()
	sc.RechargeVIPLevel = ud.RechargeVIPLevel.Int64()
	sc.IsVIP = ud.IsVIP
	sc.IsSubscribe = ud.IsSubscribe
	sc.Status = "normal"
	sc.BanInfo = nil
}

func newStoredCookie(cookieValue, source, fallbackName string) *StoredCookie {
	b := make([]byte, 8)
	rand.Read(b)
	cid := fmt.Sprintf("ck_%x", b)
	now := time.Now().Unix()
	if fallbackName == "" {
		fallbackName = "未验证账号_" + cid[3:9]
	}
	return &StoredCookie{ID: cid, Cookie: cookieValue, Name: fallbackName, DisplayName: fallbackName, Source: source, Status: "unknown", CreatedAt: now, LastUsed: now}
}

func addStoredCookieFromLogin(cookieValue, source, fallbackName string, setActive bool) *StoredCookie {
	sc := newStoredCookie(cookieValue, source, fallbackName)
	if cli, _, err := validateCookie(cookieValue); err == nil {
		fillStoredCookieFromClient(sc, cli)
	} else {
		sc.Status = "unknown"
		log.Printf("store login cookie detail unavailable: %v", err)
	}
	cookieStore.Add(sc)
	if setActive {
		cookieStore.SetActive(sc.ID)
		os.WriteFile("cookie.json", []byte(sc.Cookie), 0644)
		sessionMu.Lock()
		client = nil
		cookieStr = sc.Cookie
		sessionMu.Unlock()
	}
	cookieStore.Save()
	return sc
}

func importStoredCookie(cookieValue, source, fallbackName, uid string, createdAt int64) bool {
	if strings.TrimSpace(cookieValue) == "" {
		return false
	}
	time.Sleep(time.Duration(600+rand.Intn(800)) * time.Millisecond)
	sc := newStoredCookie(cookieValue, source, fallbackName)
	if uid != "" {
		sc.UID = uid
	}
	if createdAt > 0 {
		sc.CreatedAt = createdAt
	}
	// 启动时不验证 cookie，autoRefreshLoop 会在后台用代理重新验证
	// cookie validation skipped at startup, auto-refresh handles it later
	return cookieStore.AddIfMissing(sc)
}

func migrateAllCookies() {
	added := 0
	repaired := 0
	cookieStore.mu.RLock()
	existing := make([]*StoredCookie, 0, len(cookieStore.Cookies))
	for _, sc := range cookieStore.Cookies {
		existing = append(existing, sc)
	}
	cookieStore.mu.RUnlock()
	for _, sc := range existing {
		if repairStoredCookie(sc) {
			repaired++
		}
	}
	accountMu.Lock()
	for _, acc := range accounts {
		source := "account"
		if acc.IsGuest {
			source = "guest"
		}
		if importStoredCookie(acc.Cookie, source, firstNonEmpty(acc.DisplayName, acc.Username), acc.UID, acc.CreatedAt) {
			added++
		}
	}
	accountMu.Unlock()
	// cookie.json import skipped - already imported via accounts
	if cookieStore.ActiveID == "" {
		cookieStore.mu.RLock()
		for id := range cookieStore.Cookies {
			cookieStore.mu.RUnlock()
			cookieStore.SetActive(id)
			break
		}
		if cookieStore.ActiveID == "" {
			cookieStore.mu.RUnlock()
		}
	}
	if added > 0 || repaired > 0 {
		cookieStore.Save()
		log.Printf("migrated %d cookies into CookieStore, repaired %d empty cookies", added, repaired)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func repairStoredCookie(sc *StoredCookie) bool {
	if sc == nil || strings.TrimSpace(sc.Cookie) != "" {
		return false
	}
	accountMu.Lock()
	defer accountMu.Unlock()
	for _, acc := range accounts {
		if strings.TrimSpace(acc.Cookie) == "" {
			continue
		}
		if (sc.UID != "" && acc.UID == sc.UID) || (sc.DisplayName != "" && acc.DisplayName == sc.DisplayName) || (sc.Name != "" && acc.DisplayName == sc.Name) {
			sc.Cookie = acc.Cookie
			if sc.Source == "" {
				if acc.IsGuest {
					sc.Source = "guest"
				} else {
					sc.Source = "account"
				}
			}
			return true
		}
	}
	return false
}

func cookieResponse(sc *StoredCookie) M {
	entry := M{"id": sc.ID, "name": sc.Name, "uid": sc.UID, "status": sc.Status, "growth_level": sc.GrowthLevel, "display_name": sc.DisplayName, "account": sc.Account, "source": sc.Source, "avatar_image_url": sc.AvatarImageURL, "signature": sc.Signature, "score": sc.Score, "skin_number": sc.SkinNumber, "cape_number": sc.CapeNumber, "access_game_flag": sc.AccessGameFlag, "anti_addition_status": sc.AntiAdditionStatus, "realname_status": sc.RealnameStatus, "recharge_vip_level": sc.RechargeVIPLevel, "is_vip": sc.IsVIP, "is_subscribe": sc.IsSubscribe, "created_at": sc.CreatedAt, "last_used": sc.LastUsed}
	if sc.BanInfo != nil {
		entry["ban_info"] = sc.BanInfo
	}
	return entry
}

func extractCookieValue(body []byte) (string, string, error) {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return "", "", errors.New("请输入 Cookie 内容")
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		start := strings.Index(raw, "{")
		end := strings.LastIndex(raw, "}")
		if start >= 0 && end > start {
			candidate := raw[start : end+1]
			if json.Unmarshal([]byte(candidate), &v) == nil {
				raw = candidate
			}
		}
	}
	if v == nil {
		if strings.Contains(raw, "sauth_json") || strings.Contains(raw, "sessionid") || strings.Contains(raw, "sdkuid") {
			return raw, "raw", nil
		}
		return "", "", errors.New("无法识别 Cookie 格式，请粘贴包含 sauth_json/sessionid/sdkuid 的内容")
	}
	cookie, source := findCookieFields(v)
	if cookie == "" {
		return "", "", errors.New("没有找到可用字段，需要 sauth_json、cookie、sessionid、sdkuid 等字段")
	}
	return cookie, source, nil
}

func findCookieFields(v any) (string, string) {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range []string{"cookie", "Cookie", "g79_cookie"} {
			if raw, ok := x[k].(string); ok && strings.TrimSpace(raw) != "" {
				s := strings.TrimSpace(raw)
				var child any
				if json.Unmarshal([]byte(s), &child) == nil {
					if nested, src := findCookieFields(child); nested != "" {
						return nested, k + "." + src
					}
					if m, ok := child.(map[string]any); ok && hasAnyKey(m, "sessionid", "sdkuid", "gameid", "udid") {
						b, _ := json.Marshal(m)
						return mustCookieJSON(M{"sauth_json": string(b)}), k + ".sauth_fields"
					}
				}
				return s, k
			}
		}
		if sj, ok := x["sauth_json"]; ok {
			switch sv := sj.(type) {
			case string:
				if strings.TrimSpace(sv) != "" {
					return mustCookieJSON(M{"sauth_json": strings.TrimSpace(sv)}), "sauth_json"
				}
			case map[string]any:
				b, _ := json.Marshal(sv)
				return mustCookieJSON(M{"sauth_json": string(b)}), "sauth_json_object"
			}
		}
		if hasAnyKey(x, "sessionid", "sdkuid") {
			b, _ := json.Marshal(x)
			return mustCookieJSON(M{"sauth_json": string(b)}), "sauth_fields"
		}
		for k, child := range x {
			if s, src := findCookieFields(child); s != "" {
				return s, k + "." + src
			}
		}
	case []any:
		for _, child := range x {
			if s, src := findCookieFields(child); s != "" {
				return s, src
			}
		}
	case string:
		s := strings.TrimSpace(x)
		if strings.HasPrefix(s, "{") {
			var child any
			if json.Unmarshal([]byte(s), &child) == nil {
				return findCookieFields(child)
			}
		}
	}
	return "", ""
}

func hasAnyKey(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func mustCookieJSON(v M) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// bannedByMessage reports whether an auth error indicates a permanent ban
// (code 29).  Codes 32 / 2100 / 27003 etc. are temporary and must NOT trigger
// account switching.  Extracts the numeric code from messages like
// "登录失败 (code: 29): 禁止登录" so that a temporary error whose message
// also contains "禁止登录" does not cause a false-positive switch.
func bannedByMessage(msg string) bool {
	// Prefer the numeric code: only 29 is a permanent ban.
	idx := strings.Index(msg, "code:")
	if idx < 0 {
		idx = strings.Index(msg, "code：")
	}
	if idx >= 0 {
		rest := msg[idx+5:]
		for len(rest) > 0 && rest[0] == ' ' {
			rest = rest[1:]
		}
		num := 0
		for _, ch := range rest {
			if ch < '0' || ch > '9' {
				break
			}
			num = num*10 + int(ch-'0')
		}
		if num > 0 {
			return num == 29
		}
	}
	// No numeric code found: fall back to the Chinese ban text.
	return strings.Contains(msg, "禁止登录")
}

func validateCookieRecord(cookieValue, source, fallbackName string) (*StoredCookie, error) {
	c, err := g79.NewClientWithHTTPClient(PickOneTimeProxy())
	if err != nil {
		return nil, fmt.Errorf("创建客户端失败: %w", err)
	}
	if err := c.G79AuthenticateWithCookie(cookieValue); err != nil {
		msg := err.Error()
		// 仅永久封禁（code 29）标记为封禁状态；临时错误不标记
		if bannedByMessage(msg) || strings.Contains(msg, "未实名") {
			sc := newStoredCookie(cookieValue, source, firstNonEmpty(fallbackName, "封禁账号"))
			sc.Status = "banned"
			if uid := extractSDKUID(cookieValue); uid != "" {
				sc.UID = uid
			}
			sc.BanInfo = &BanInfo{Reason: msg, BanType: "login_rejected"}
			return sc, nil
		}
		return nil, fmt.Errorf("认证失败: %w", err)
	}
	sc := newStoredCookie(cookieValue, source, fallbackName)
	sc.UID = c.UserID
	ud, err := c.GetUserDetail()
	if err != nil {
		sc.Status = "banned"
		if sc.Name == "" || strings.HasPrefix(sc.Name, "未验证账号_") {
			sc.Name = firstNonEmpty(c.UserID, fallbackName, sc.Name)
			sc.DisplayName = sc.Name
		}
		sc.BanInfo = &BanInfo{Reason: err.Error(), BanType: "detail_unavailable"}
		return sc, nil
	}
	c.UserDetail = &ud.Entity
	fillStoredCookieFromClient(sc, c)
	// Get real game level from social profile
	if od, oerr := c.GetOtherUserDetail(c.UserID, true); oerr == nil && od != nil {
		sc.GrowthLevel = int(od.Entity.PEGrowth.Lv.Int64())
	}
	return sc, nil
}

func scanWorkspaceCookies() {
	added := 0
	seen := map[string]bool{}
	_ = filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() {
				name := d.Name()
				if name == ".git" || name == "node_modules" || name == "third_party" || name == "phoenix_server" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".bak") || strings.Contains(name, "cookies.json.bak") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".json" && ext != ".log" && ext != ".txt" && ext != ".conf" {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil || info.Size() > 4*1024*1024 {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		candidates := extractCookieCandidates(data)
		for _, cookieValue := range candidates {
			if cookieValue == "" || seen[cookieValue] {
				continue
			}
			seen[cookieValue] = true
			sc, valErr := validateCookieRecord(cookieValue, "scan:"+path, filepath.Base(path))
			if valErr != nil {
				continue
			}
			if cookieStore.AddIfMissing(sc) {
				added++
			}
		}
		return nil
	})
	if added > 0 {
		cookieStore.Save()
		log.Printf("scanned and added %d valid cookies", added)
	}
}

func extractCookieCandidates(data []byte) []string {
	out := []string{}
	if cookieValue, _, err := extractCookieValue(data); err == nil {
		out = append(out, cookieValue)
	}
	var v any
	if json.Unmarshal(data, &v) == nil {
		collectCookieCandidates(v, &out)
	}
	text := string(data)
	for _, key := range []string{"sauth_json", "sessionid", "sdkuid"} {
		idx := 0
		for {
			pos := strings.Index(text[idx:], key)
			if pos < 0 {
				break
			}
			pos += idx
			start := strings.LastIndex(text[:pos], "{")
			endRel := strings.Index(text[pos:], "}")
			if start >= 0 && endRel >= 0 {
				frag := text[start : pos+endRel+1]
				if cookieValue, _, err := extractCookieValue([]byte(frag)); err == nil {
					out = append(out, cookieValue)
				}
			}
			idx = pos + len(key)
		}
	}
	return out
}

func collectCookieCandidates(v any, out *[]string) {
	if cookieValue, _ := findCookieFields(v); cookieValue != "" {
		*out = append(*out, cookieValue)
	}
	switch x := v.(type) {
	case map[string]any:
		for _, child := range x {
			collectCookieCandidates(child, out)
		}
	case []any:
		for _, child := range x {
			collectCookieCandidates(child, out)
		}
	case string:
		if strings.Contains(x, "sauth_json") || strings.Contains(x, "sessionid") || strings.Contains(x, "sdkuid") {
			if cookieValue, _, err := extractCookieValue([]byte(x)); err == nil {
				*out = append(*out, cookieValue)
			}
		}
	}
}

func extractSDKUID(cookieValue string) string {
	var wrapper map[string]any
	if json.Unmarshal([]byte(cookieValue), &wrapper) != nil {
		return ""
	}
	sj, ok := wrapper["sauth_json"]
	if !ok {
		return ""
	}
	var sauth map[string]any
	switch v := sj.(type) {
	case string:
		if json.Unmarshal([]byte(v), &sauth) != nil {
			return ""
		}
	case map[string]any:
		sauth = v
	default:
		return ""
	}
	if uid, ok := sauth["sdkuid"].(string); ok {
		return uid
	}
	return ""
}

func validateCookie(cookieValue string) (*g79.Client, *StoredCookie, error) {
	c, err := g79.NewClient()
	if err != nil {
		return nil, nil, fmt.Errorf("创建客户端失败: %w", err)
	}
	if err := c.G79AuthenticateWithCookie(cookieValue); err != nil {
		return nil, nil, fmt.Errorf("认证失败: %w", err)
	}
	ud, err := c.GetUserDetail()
	if err != nil {
		return nil, nil, fmt.Errorf("获取账号信息失败: %w", err)
	}
	c.UserDetail = &ud.Entity
	sc := newStoredCookie(cookieValue, "validated", "")
	fillStoredCookieFromClient(sc, c)
	return c, sc, nil
}

func logReq(r *http.Request, body []byte) {
	f, _ := os.OpenFile("capture.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if f != nil {
		fmt.Fprintf(f, "\n=== %s %s %s ===\n", time.Now().Format("15:04:05"), r.Method, r.URL.String())
		if len(body) > 0 {
			fmt.Fprintf(f, "BODY: %s\n", string(body))
		}
		f.Close()
	}
	// Also write to system_logs for web detail view (truncate body to 500 chars)
	detail := ""
	if len(body) > 0 {
		detail = string(body)
		if len(detail) > 500 {
			detail = detail[:500]
		}
	}
	db.AddSystemLog("info", "请求: "+r.Method+" "+r.URL.RequestURI(), detail)
}

func logRsp(r *http.Request, data any) {
	f, _ := os.OpenFile("capture.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if f != nil {
		bs, _ := json.Marshal(data)
		fmt.Fprintf(f, "RSP: %s\n", string(bs))
		f.Close()
	}
}

func deprecated(msg string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusGone) // 410 Gone
		fmt.Fprintf(w, `{"ok":false,"error":"%s"}`, msg)
	}
}

func jsonW(w http.ResponseWriter, r *http.Request, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(data)
	logRsp(r, data)
}

// apiTokenCtxKey carries the secondary-token id/name resolved for a request, so
// recordCall and audit logging can attribute a call to a specific token.
type apiTokenCtxKey struct{}

type apiTokenCtxVal struct {
	ID   int64
	Name string
}

// currentAPIToken reads the secondary-token info stashed on the request context.
// Returns 0,"" when the request used the primary token.
func currentAPIToken(r *http.Request) (int64, string) {
	if v, ok := r.Context().Value(apiTokenCtxKey{}).(apiTokenCtxVal); ok {
		return v.ID, v.Name
	}
	return 0, ""
}

// apiCallRecord 一次 API 调用审计项，由后台 worker 批量落库。
type apiCallRecord struct {
	userID    *int64
	endpoint  string
	method    string
	success   bool
	ip        string
	tokenID   *int64
	tokenName string
}

var (
	apiCallCh      = make(chan apiCallRecord, 4096) // 有界队列，防攻击时写放大
	apiCallDropped uint64
)

// startAPICallRecorder 消费有界队列批量落库；每请求起 goroutine 改为单 worker，
// 避免攻击时 goroutine 爆炸 + DB 写放大。队列满时丢弃并计数（不阻塞 handler）。
func startAPICallRecorder() {
	go func() {
		for rec := range apiCallCh {
			db.RecordAPICall(rec.userID, rec.endpoint, rec.method, rec.success, 200, 0, rec.ip, "", rec.tokenID, rec.tokenName)
		}
	}()
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if d := atomic.LoadUint64(&apiCallDropped); d > 0 {
				log.Printf("[API-CALL] dropped %d audit records under load", d)
				atomic.StoreUint64(&apiCallDropped, 0)
			}
		}
	}()
}

func recordCall(r *http.Request, success bool) {
	endpoint := r.URL.Path
	var userID *int64
	if u := auth.GetUser(r.Context()); u != nil {
		userID = &u.ID
	} else if bearer := getBearer(r); bearer != "" {
		sessMu.RLock()
		if s, ok := sessions[bearer]; ok && s.WebUserID != 0 {
			uid := s.WebUserID
			userID = &uid
		}
		sessMu.RUnlock()
	}
	var tokenID *int64
	tokenName := ""
	if tid, tname := currentAPIToken(r); tid != 0 {
		tokenID = &tid
		tokenName = tname
	}
	rec := apiCallRecord{
		userID: userID, endpoint: endpoint, method: r.Method,
		success: success, ip: requestIP(r), tokenID: tokenID, tokenName: tokenName,
	}
	select {
	case apiCallCh <- rec:
	default:
		atomic.AddUint64(&apiCallDropped, 1)
	}
}

func checknumBinaryPath() string {
	candidates := []string{"./checknum_dynamic", "/root/fa/xiaoruoawa-main/checknum_dynamic"}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "checknum_dynamic"))
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return p
		}
	}
	return ""
}

type captchaEntry struct {
	answer   int
	attempts int
	created  time.Time
	image    string // 已绘制的 base64 图片，丢弃时放入可复用池
}

func checkCaptcha(r *http.Request) bool {
	var body struct {
		C string `json:"c"`
		A string `json:"a"`
	}
	if b, _ := io.ReadAll(r.Body); len(b) > 0 {
		r.Body = io.NopCloser(bytes.NewReader(b))
		json.Unmarshal(b, &body)
	}
	cid := strings.TrimSpace(r.URL.Query().Get("c"))
	if cid == "" {
		cid = strings.TrimSpace(body.C)
	}
	userAns := strings.TrimSpace(r.URL.Query().Get("a"))
	if userAns == "" {
		userAns = strings.TrimSpace(body.A)
	}
	if cid == "" || userAns == "" {
		return false
	}
	captchaAnswersMu.Lock()
	defer captchaAnswersMu.Unlock()
	entry, ok := captchaAnswers[cid]
	if !ok {
		return false
	}
	// 过期：未消费，放入可复用池供高负载时复用
	if time.Since(entry.created) > 5*time.Minute {
		delete(captchaAnswers, cid)
		discardCaptcha(entry)
		return false
	}
	entry.attempts++
	if entry.attempts > captchaMaxAttempts {
		delete(captchaAnswers, cid)
		discardCaptcha(entry)
		return false
	}
	got, _ := strconv.Atoi(userAns)
	if got == entry.answer {
		delete(captchaAnswers, cid)
		return true
	}
	return false
}

func main() {
	go func() {
		// 直连模式，不走代理
		http.DefaultTransport = http.DefaultTransport
		http.DefaultClient = &http.Client{Timeout: 30 * time.Second}
		log.Printf("[GLOBAL] 直连模式，不使用代理")
		for range time.NewTicker(10 * time.Minute).C {

			captchaAnswersMu.Lock()
			for k, v := range captchaAnswers {
				if time.Since(v.created) > 5*time.Minute {
					delete(captchaAnswers, k)
					discardCaptcha(v)
				}
			}
			captchaAnswersMu.Unlock()
		}
	}()
	cookieStore.Load()
	// scanWorkspaceCookies skipped on startup
	if removed := cookieStore.DedupeByUID(); removed > 0 {
		cookieStore.Save()
		log.Printf("deduped %d duplicate cookie entries by UID", removed)
	}

	// ── Config & DB init ──
	cfg := auth.LoadConfig("config.json")
	db.Init(cfg.DBPath)
	log.SetOutput(os.Stderr)
	go db.BackfillFromAuditLogs()
	// Periodic system log cleanup (keep 2 days)
	go func() {
		pruneLogs := func() {
			if n, err := db.PruneSystemLogs(3); err != nil {
				log.Printf("[SYSLOG] prune error: %v", err)
			} else if n > 0 {
				log.Printf("[SYSLOG] pruned %d old entries", n)
			}
			// API 调用日志驱动统计,保留 30 天以匹配 30 天统计回看窗口
			if n, err := db.PruneAPICallLogs(30); err != nil {
				log.Printf("[APILOG] prune error: %v", err)
			} else if n > 0 {
				log.Printf("[APILOG] pruned %d old entries", n)
			}
		}
		pruneLogs() // 启动即清理一次,避免整点才生效
		for range time.NewTicker(1 * time.Hour).C {
			pruneLogs()
		}
	}()
	if cfg.AdminEmail != "" {
		exists, _ := db.IsAdminExists()
		if !exists {
			hash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
			db.CreateAdmin(cfg.AdminEmail, string(hash))
			log.Printf("[AUTH] Admin user created: %s (password: admin123)", cfg.AdminEmail)
		}
	}

	loadChallengeOverrides()
	go TestAllProxies()
	// cache cleanup only (no auto-refresh)
	go StartG79CacheCleanup()
	go StartAllHeartbeats()
	go startHeartbeatLogsCleanup()
	// 自写 P2P 协调者：内嵌反向隧道 + 本地 SOCKS5，替代已停用的 gost 桥方案。
	// 监听 :8443（节点经 160:19205 前台跳板转发至此）+ 127.0.0.1:19003。
	if err := startCoordinator(); err != nil {
		log.Printf("[COORD] 协调者启动失败: %v", err)
	} else {
		log.Printf("[COORD] 协调者已启动（节点隧道 :%d / SOCKS5 127.0.0.1:%d）", p2pTunPort, p2pSocksPort)
	}
	go startCaptchaPool()
	// 内存安全：清扫过期限流条目与过期会话，防止 map 无限增长
	go auth.StartRateLimiterCleanup(nil)
	// 内存安全：清扫管理员暴力破解防护的过期封禁记录
	go func() {
		for range time.NewTicker(5 * time.Minute).C {
			adminBrute.purge()
		}
	}()
	// 审计写入：有界队列 + 单 worker 批量落库
	go startAPICallRecorder()
	go func() {
		for range time.NewTicker(1 * time.Hour).C {
			now := time.Now()
			sessMu.Lock()
			for k, s := range sessions {
				if s.ExpiresAt.IsZero() {
					s.ExpiresAt = s.CreatedAt.Add(7 * 24 * time.Hour)
				}
				if now.After(s.ExpiresAt) {
					delete(sessions, k)
				}
			}
			sessMu.Unlock()
		}
	}()

	// ── AI 代理（/api/prism/ai，登录鉴权，多上游 429 降级）──
	if aicfg, aierr := buildAIConfig("ai_proxy.json"); aierr != nil {
		log.Printf("[AI代理] 未加载配置 ai_proxy.json: %v（/api/prism/ai 不可用）", aierr)
	} else {
		aiproxy := newAIProxy(aicfg)
		// 临时诊断（测完恢复鉴权）：去掉 SessionAuth，免令牌访问
		http.HandleFunc("/api/prism/ai/v1/chat/completions", aiproxy.handleChat)
		http.HandleFunc("/api/prism/ai/v1/models", aiproxy.handleModels)
		log.Printf("[AI代理] 已启用，降级链 %d 个目标", len(aiproxy.chain))
	}

	// ── Toolbox v2 public API ──
	http.HandleFunc("/api/v2/version/check", handleVersionCheck)
	http.HandleFunc("/api/v2/announcements", handleAnnouncements)
	http.HandleFunc("/api/v2/signature/verify", handleSignatureVerify)

	// ── Heartbeat / Push ──
	http.HandleFunc("/api/v2/heartbeat", handleHeartbeat)
	http.HandleFunc("/api/v2/heartbeat/pending", handleHeartbeatPending)
	http.HandleFunc("/api/v2/heartbeat/ack", handleHeartbeatAck)

	// ── MC 建筑文件市场 ──
	http.HandleFunc("/api/market/upload/precheck", auth.SessionAuth(handleMarketPrecheck))
	http.HandleFunc("/api/market/upload", handleMarketUpload) // 用 TTS 鉴权,不包 SessionAuth
	http.HandleFunc("/api/music/hot", handleMusicHot)
	http.HandleFunc("/api/music/download", handleMusicDownload)
	http.HandleFunc("/api/market/files", handleMarketFiles)
	http.HandleFunc("/api/market/files/{id}", auth.OptionalSessionAuth(handleMarketFileDetail))
	http.HandleFunc("/api/market/files/{id}/download", auth.OptionalSessionAuth(handleMarketFileDownload))
	http.HandleFunc("/api/market/files/{id}/gif", auth.OptionalSessionAuth(handleMarketFileGif))
	http.HandleFunc("/api/market/files/{id}/report", auth.SessionAuth(handleMarketReport))
	http.HandleFunc("/api/market/files/{id}/edit", auth.SessionAuth(handleMarketFileOwnerEdit))
	http.HandleFunc("/api/market/files/{id}/delete", auth.SessionAuth(handleMarketFileOwnerDelete))
	http.HandleFunc("/api/market/files/{id}/comments", auth.OptionalSessionAuth(handleMarketFileComments)) // GET 公开;POST 需登录
	http.HandleFunc("/api/market/comments/{id}", auth.SessionAuth(handleMarketCommentDelete))
	http.HandleFunc("/api/market/my", auth.SessionAuth(handleMarketMyFiles))
	http.HandleFunc("/api/market/parts/{id}/preview", handleMarketPartPreview)
	http.HandleFunc("/api/market/categories", handleMarketCategories)
	http.HandleFunc("/api/market/tags", handleMarketTags)
	// 管理
	http.HandleFunc("/api/admin/market/files", auth.AdminAuth(handleAdminMarketFiles))
	http.HandleFunc("/api/admin/market/flagged", auth.AdminAuth(handleAdminMarketFlagged))
	http.HandleFunc("/api/admin/market/files/{id}/status", auth.AdminAuth(handleAdminMarketFileStatus))
	http.HandleFunc("/api/admin/market/files/{id}/unflag", auth.AdminAuth(handleAdminMarketFileUnflag))
	http.HandleFunc("/api/admin/market/files/{id}/delete", auth.AdminAuth(handleAdminMarketFileDelete))
	http.HandleFunc("/api/admin/market/files/{id}/update", auth.AdminAuth(handleAdminMarketFileUpdate))
	http.HandleFunc("/api/admin/market/categories", auth.AdminAuth(handleAdminMarketCategories))
	http.HandleFunc("/api/admin/market/categories/{id}", auth.AdminAuth(handleAdminMarketCategories))
	http.HandleFunc("/api/admin/market/config", auth.AdminAuth(handleAdminMarketConfig))

	// ── 插件市场 ──
	http.HandleFunc("/api/plugin-market/upload/precheck", auth.SessionAuth(handlePluginMarketPrecheck))
	http.HandleFunc("/api/plugin-market/upload", handlePluginMarketUpload) // 用 TTS 鉴权
	http.HandleFunc("/api/plugin-market/categories", handlePluginMarketCategories)
	http.HandleFunc("/api/plugin-market/tags", handlePluginMarketTags)
	http.HandleFunc("/api/plugin-market/files", handlePluginMarketFiles)
	http.HandleFunc("/api/plugin-market/files/{id}", auth.OptionalSessionAuth(handlePluginMarketFileDetail))
	http.HandleFunc("/api/plugin-market/files/{id}/download", auth.OptionalSessionAuth(handlePluginMarketFileDownload))
	http.HandleFunc("/api/plugin-market/files/{id}/report", auth.SessionAuth(handlePluginMarketFileReport))
	http.HandleFunc("/api/plugin-market/files/{id}/edit", auth.SessionAuth(handlePluginMarketFileOwnerEdit))
	http.HandleFunc("/api/plugin-market/files/{id}/delete", auth.SessionAuth(handlePluginMarketFileOwnerDelete))
	http.HandleFunc("/api/plugin-market/files/{id}/comments", auth.OptionalSessionAuth(handlePluginMarketFileComments))
	http.HandleFunc("/api/plugin-market/comments/{id}", auth.SessionAuth(handlePluginMarketCommentDelete))
	http.HandleFunc("/api/plugin-market/my", auth.SessionAuth(handlePluginMarketMyFiles))
	// 管理员
	http.HandleFunc("/api/admin/plugin-market/files", auth.AdminAuth(handleAdminPluginMarketFiles))
	http.HandleFunc("/api/admin/plugin-market/flagged", auth.AdminAuth(handleAdminPluginMarketFlagged))
	http.HandleFunc("/api/admin/plugin-market/files/{id}/review", auth.AdminAuth(handleAdminPluginMarketReview))
	http.HandleFunc("/api/admin/plugin-market/files/{id}/unflag", auth.AdminAuth(handleAdminPluginMarketFileUnflag))
	http.HandleFunc("/api/admin/plugin-market/files/{id}/delete", auth.AdminAuth(handleAdminPluginMarketFileDelete))
	http.HandleFunc("/api/admin/plugin-market/files/{id}/update", auth.AdminAuth(handleAdminPluginMarketFileUpdate))
	http.HandleFunc("/api/admin/plugin-market/categories", auth.AdminAuth(handleAdminPluginMarketCategories))
	http.HandleFunc("/api/admin/plugin-market/config", auth.AdminAuth(handleAdminPluginMarketConfig))

	// ── Auth routes ──
	http.HandleFunc("/api/auth/captcha/new", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		if !captchaNewLimiter.Allow(requestIP(r)) {
			jsonResp(w, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
			return
		}
		cid, img := serveCaptcha()
		jsonResp(w, M{"ok": true, "id": cid, "image": img})
	})
	http.HandleFunc("/api/auth/register", func(w http.ResponseWriter, r *http.Request) {
		if !checkCaptcha(r) {
			jsonResp(w, M{"ok": false, "error": "哎呀,验证码不对,请重新输入~"})
			return
		}
		handleRegister(w, r)
	})
	http.HandleFunc("/api/auth/verify", handleVerify)
	http.HandleFunc("/api/auth/verify/confirm", handleVerifyConfirm)
	http.HandleFunc("/api/auth/login", handleLogin)
	http.HandleFunc("/api/auth/me", auth.SessionAuth(handleMe))
	http.HandleFunc("/api/auth/token", auth.SessionAuth(handleTokenRefresh))
	http.HandleFunc("/api/auth/profile", auth.SessionAuth(handleUpdateProfile))
	http.HandleFunc("/api/auth/password", auth.SessionAuth(handleUpdatePassword))
	http.HandleFunc("/api/auth/reset-token", auth.SessionAuth(handleResetToken))
	http.HandleFunc("/api/auth/logs", auth.SessionAuth(handleMyAuditLogs))
	// Stats endpoints
	http.HandleFunc("/api/auth/stats/summary", auth.SessionAuth(handleUserStatsSummary))
	http.HandleFunc("/api/auth/stats/endpoints", auth.SessionAuth(handleUserEndpointStats))
	http.HandleFunc("/api/auth/stats/servers", auth.SessionAuth(handleUserServerStats))
	http.HandleFunc("/api/auth/stats/nuts", auth.SessionAuth(handleUserNutsTimeline))
	http.HandleFunc("/api/auth/stats/active-endpoints", auth.SessionAuth(handleUserActiveEndpoints))
	http.HandleFunc("/api/auth/forgot-password", func(w http.ResponseWriter, r *http.Request) {
		if !checkCaptcha(r) {
			jsonResp(w, M{"ok": false, "error": "哎呀,验证码不对,请重新输入~"})
			return
		}
		handleForgotPassword(w, r)
	})
	http.HandleFunc("/api/auth/reset-password", handleResetPassword)

	// ── Invite link redirect ──
	http.HandleFunc("/invite/", func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/invite/"))
		if code == "" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/#/register?invite="+url.QueryEscape(code), http.StatusFound)
	})

	http.HandleFunc("/api/auth/change-email", auth.SessionAuth(func(w http.ResponseWriter, r *http.Request) {
		if !checkCaptcha(r) {
			jsonResp(w, M{"ok": false, "error": "哎呀,验证码不对,请重新输入~"})
			return
		}
		handleChangeEmail(w, r)
	}))
	http.HandleFunc("/api/auth/confirm-email", auth.SessionAuth(handleConfirmEmail))
	http.HandleFunc("/api/auth/challenge-override", auth.SessionAuth(func(w http.ResponseWriter, r *http.Request) {
		if !challengeOverrideLim.Allow(requestIP(r)) {
			jsonW(w, r, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
			return
		}
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		user := auth.GetUser(r.Context())
		if user == nil {
			jsonW(w, r, M{"ok": false, "error": "未登录"})
			return
		}
		var body struct {
			Enabled bool `json:"enabled"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if err := db.SetChallengeOverride(user.ID, body.Enabled); err != nil {
			jsonW(w, r, M{"ok": false, "error": sysErr(err)})
			return
		}
		// Update in-memory override map
		g79OverrideMu.Lock()
		if body.Enabled {
			if accs, err := db.GetUserAccounts(user.ID); err == nil {
				for _, a := range accs {
					_ = a
					if a.UID != "" {
						g79OverrideMap[user.ID] = true
					}
				}
			}
		} else {
			if accs, err := db.GetUserAccounts(user.ID); err == nil {
				for _, a := range accs {
					_ = a
					delete(g79OverrideMap, user.ID)
				}
			}
		}
		g79OverrideMu.Unlock()
		status := "关闭"
		if body.Enabled {
			status = "开启"
		}
		_ = db.AddAuditLog(&user.ID, nil, "challenge_override", "web", fmt.Sprintf("挑战版本覆盖已%s", status), requestIP(r))
		log.Printf("[AUTH] 用户 %d 挑战版本覆盖已%s", user.ID, status)
		jsonW(w, r, M{"ok": true, "enabled": body.Enabled, "message": fmt.Sprintf("挑战版本覆盖已%s", status)})
	}))
	http.HandleFunc("/api/auth/growth-override", auth.SessionAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		user := auth.GetUser(r.Context())
		if user == nil {
			jsonW(w, r, M{"ok": false, "error": "未登录"})
			return
		}
		var body struct {
			Enabled bool `json:"enabled"`
			Value   int  `json:"value"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Enabled {
			if body.Value < 0 || body.Value > 999 {
				jsonW(w, r, M{"ok": false, "error": "等级值必须在 0-999 之间"})
				return
			}
		}
		if err := db.SetGrowthOverride(user.ID, body.Enabled, body.Value); err != nil {
			jsonW(w, r, M{"ok": false, "error": sysErr(err)})
			return
		}
		status := "关闭"
		if body.Enabled {
			status = fmt.Sprintf("开启 (等级=%d)", body.Value)
		}
		_ = db.AddAuditLog(&user.ID, nil, "growth_override", "web", fmt.Sprintf("等级覆盖已%s", status), requestIP(r))
		jsonW(w, r, M{"ok": true, "enabled": body.Enabled, "value": body.Value, "message": fmt.Sprintf("等级覆盖已%s", status)})
	}))
	http.HandleFunc("/api/auth/logout", auth.SessionAuth(func(w http.ResponseWriter, r *http.Request) {
		user := auth.GetUser(r.Context())
		if user != nil {
			_ = db.AddAuditLog(&user.ID, nil, "logout", "web", "用户退出登录", requestIP(r))
		}
		jsonResp(w, M{"ok": true})
	}))
	http.HandleFunc("/api/auth/redeem", auth.SessionAuth(handleRedeemCode))
	http.HandleFunc("/api/auth/nuts/to-code", auth.SessionAuth(handleNutsToCode))
	http.HandleFunc("/api/auth/nuts/to-question-code", auth.SessionAuth(handleNutsToQuestionCode))
	http.HandleFunc("/api/auth/nuts/redpacket", auth.SessionAuth(handleCreateRedPacket))
	http.HandleFunc("/api/auth/redpackets", auth.SessionAuth(handleListRedPackets))
	http.HandleFunc("/api/auth/codes", auth.SessionAuth(handleMyCodes))
	http.HandleFunc("/api/auth/tokens", auth.SessionAuth(handleAPITokens))
	http.HandleFunc("/api/auth/tokens/{id}/reset", auth.SessionAuth(handleAPITokenReset))
	http.HandleFunc("/api/auth/tokens/{id}/switch", auth.SessionAuth(handleAPITokenSwitch))
	http.HandleFunc("/api/auth/tokens/{id}/update", auth.SessionAuth(handleAPITokenUpdate))
	http.HandleFunc("/api/auth/tokens/{id}/delete", auth.SessionAuth(handleAPITokenDelete))
	http.HandleFunc("/api/auth/activate", handleActivate)

	// ── Payment routes ──
	http.HandleFunc("/api/payment/products", handlePaymentProducts)
	http.HandleFunc("/api/payment/order", auth.SessionAuth(handleCreatePaymentOrder))
	http.HandleFunc("/api/payment/orders", auth.SessionAuth(handleUserPaymentOrders))
	http.HandleFunc("/api/payment/order/status", auth.SessionAuth(handlePaymentOrderStatus))
	http.HandleFunc("/api/payment/notify", handlePaymentNotify)
	http.HandleFunc("/api/admin/payment/orders", auth.AdminAuth(handleAdminPaymentOrders))
	http.HandleFunc("/api/admin/payment/retry", auth.AdminAuth(handleAdminPaymentRetry))

	// ── Survey routes (user-facing) ──
	http.HandleFunc("/api/survey/active", auth.SessionAuth(handleActiveSurvey))
	http.HandleFunc("/api/survey/submit", auth.SessionAuth(handleSubmitSurvey))

	// ── Survey admin routes ──
	http.HandleFunc("/api/admin/surveys", auth.AdminAuth(handleAdminListSurveys))
	http.HandleFunc("/api/admin/surveys/create", auth.AdminAuth(handleAdminCreateSurvey))
	http.HandleFunc("/api/admin/surveys/update", auth.AdminAuth(handleAdminUpdateSurvey))
	http.HandleFunc("/api/admin/surveys/questions", auth.AdminAuth(handleAdminUpdateQuestions))
	http.HandleFunc("/api/admin/surveys/toggle", auth.AdminAuth(handleAdminToggleSurvey))
	http.HandleFunc("/api/admin/surveys/delete", auth.AdminAuth(handleAdminDeleteSurvey))
	http.HandleFunc("/api/admin/surveys/clear-answers", auth.AdminAuth(handleAdminClearSurveyAnswers))
	http.HandleFunc("/api/admin/surveys/questions/list", auth.AdminAuth(handleAdminGetSurveyQuestions))
	http.HandleFunc("/api/admin/surveys/stats", auth.AdminAuth(handleAdminSurveyStats))
	http.HandleFunc("/api/admin/surveys/user-answers", auth.AdminAuth(handleAdminSurveyUserAnswers))
	// ── Public download ──
	http.HandleFunc("/dl/", handleDownloadRelease)

	// ── Toolbox admin routes ──
	http.HandleFunc("/api/admin/toolbox/version", auth.AdminAuth(handleAdminVersionConfig))
	http.HandleFunc("/api/admin/toolbox/announcements", auth.AdminAuth(handleAdminAnnouncements))
	http.HandleFunc("/api/admin/toolbox/announcement", auth.AdminAuth(handleAdminAnnouncementItem))
	http.HandleFunc("/api/admin/toolbox/announcement/toggle", auth.AdminAuth(handleAdminAnnouncementToggle))
	http.HandleFunc("/api/admin/toolbox/signatures", auth.AdminAuth(handleAdminSignatures))
	http.HandleFunc("/api/admin/toolbox/versions", auth.AdminAuth(handleAdminVersions))
	http.HandleFunc("/api/admin/toolbox/versions/switch", auth.AdminAuth(handleAdminVersionSwitch))
	http.HandleFunc("/api/admin/toolbox/versions/scan", auth.AdminAuth(handleScanReleases))
	http.HandleFunc("/api/admin/bulk-like", auth.AdminAuth(handleAdminBulkLike))
	http.HandleFunc("/api/admin/messages", auth.AdminAuth(handleAdminGetMessages))
	http.HandleFunc("/api/admin/messages/set", auth.AdminAuth(handleAdminSetMessages))
	http.HandleFunc("/api/admin/toolbox/users", auth.AdminAuth(handleAdminToolboxUsers))
	http.HandleFunc("/api/admin/toolbox/user-config", auth.AdminAuth(handleAdminToolboxUserConfig))
	http.HandleFunc("/api/admin/toolbox/defaults", auth.AdminAuth(handleAdminToolboxDefaults))
	http.HandleFunc("/api/admin/push/messages", auth.AdminAuth(handleAdminPushMessages))
	http.HandleFunc("/api/admin/push/send", auth.AdminAuth(handleAdminPushSend))
	http.HandleFunc("/api/admin/push/message", auth.AdminAuth(handleAdminPushDelete))
	http.HandleFunc("/api/admin/push/stats", auth.AdminAuth(handleAdminPushStats))
	http.HandleFunc("/api/admin/push/online-users", auth.AdminAuth(handleAdminOnlineUsers))
	http.HandleFunc("/api/admin/push/user-detail", auth.AdminAuth(handleAdminUserDetail))

	// ── Toolbox user-facing routes ──
	http.HandleFunc("/api/toolbox/my-info", auth.SessionAuth(handleMyToolboxInfo))
	http.HandleFunc("/api/toolbox/my-info/update", auth.SessionAuth(handleUpdateMyToolbox))
	http.HandleFunc("/api/toolbox/start-trial", auth.SessionAuth(handleStartTrial))
	http.HandleFunc("/api/toolbox/purchase/prompt", auth.SessionAuth(handlePurchaseToolboxFeature))
	http.HandleFunc("/api/toolbox/purchase/name", auth.SessionAuth(handlePurchaseToolboxFeature))
	http.HandleFunc("/api/toolbox/purchase/extension", auth.SessionAuth(handleExtendToolbox))
	http.HandleFunc("/api/toolbox/download", handleToolboxDownload)

	http.HandleFunc("/api/auth/nuts", auth.SessionAuth(func(w http.ResponseWriter, r *http.Request) {
		user := auth.GetUser(r.Context())
		if user == nil {
			jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
			return
		}
		txns, _ := db.GetNutsTransactions(user.ID, 30)
		jsonResp(w, M{"ok": true, "balance": user.NutsBalance, "transactions": txns})
	}))
	http.HandleFunc("/api/admin/nuts", auth.AdminAuth(func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		txns, err := db.GetAllNutsTransactions(limit, offset)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": sysErr(err)})
			return
		}
		jsonResp(w, M{"ok": true, "transactions": txns})
	}))
	http.HandleFunc("/api/v2/nuts/consume", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var req struct {
			Token  string `json:"token"`
			Amount int    `json:"amount"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResp(w, M{"ok": false, "error": "参数解析失败"})
			return
		}
		if req.Token == "" || req.Amount <= 0 {
			jsonResp(w, M{"ok": false, "error": msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~")})
			return
		}
		user, err := db.GetUserByToken(req.Token)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": "无效的 token"})
			return
		}
		reason := req.Reason
		if reason == "" {
			reason = "toolbox:consumption"
		}
		remaining, err := db.AddNuts(user.ID, -req.Amount, reason, nil, nil)
		if err != nil {
			// 余额不足时返回实际余额，其他错误也返回真实余额
			actualBalance, _ := db.GetNutsBalance(user.ID)
			jsonResp(w, M{"ok": false, "error": sysErr(err), "remaining": actualBalance})
			return
		}
		jsonResp(w, M{"ok": true, "remaining": remaining})
	})
	http.HandleFunc("/api/admin/nuts/adjust", auth.AdminAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var body struct {
			UserID int64  `json:"user_id"`
			Amount int    `json:"amount"`
			Reason string `json:"reason"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.UserID == 0 || body.Amount == 0 {
			jsonResp(w, M{"ok": false, "error": msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~")})
			return
		}
		if body.Reason == "" {
			body.Reason = "admin_adjust"
		}
		bal, err := db.AddNuts(body.UserID, body.Amount, body.Reason, nil, nil)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": sysErr(err)})
			return
		}
		adminUser := auth.GetUser(r.Context())
		_ = db.AddAuditLog(&adminUser.ID, nil, "admin_nuts", "web", fmt.Sprintf("调整用户#%d 板栗 %+d，余额=%d", body.UserID, body.Amount, bal), requestIP(r))
		jsonResp(w, M{"ok": true, "balance": bal, "message": fmt.Sprintf("已调整，当前余额 %d", bal)})
	}))
	http.HandleFunc("/api/admin/subscription/revoke", auth.AdminAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var body struct {
			UserID int64 `json:"user_id"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.UserID == 0 {
			jsonResp(w, M{"ok": false, "error": msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~")})
			return
		}
		if err := db.RevokeSubscription(body.UserID); err != nil {
			jsonResp(w, M{"ok": false, "error": sysErr(err)})
			return
		}
		adminUser := auth.GetUser(r.Context())
		_ = db.AddAuditLog(&adminUser.ID, nil, "admin_subscription_revoke", "web",
			fmt.Sprintf("撤销用户#%d 订阅", body.UserID), requestIP(r))
		jsonResp(w, M{"ok": true})
	}))
	http.HandleFunc("/api/admin/subscription", auth.AdminAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var body struct {
			UserID int64 `json:"user_id"`
			Days   int   `json:"days"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.UserID == 0 || body.Days <= 0 {
			jsonResp(w, M{"ok": false, "error": msg.Get("param_error", "抱歉,提交的信息有误,请检查后重试~")})
			return
		}
		until, err := db.GrantSubscriptionDays(body.UserID, body.Days)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": sysErr(err)})
			return
		}
		adminUser := auth.GetUser(r.Context())
		_ = db.AddAuditLog(&adminUser.ID, nil, "admin_subscription", "web",
			fmt.Sprintf("授予用户#%d 订阅 %d 天，到期=%s", body.UserID, body.Days, until), requestIP(r))
		jsonResp(w, M{"ok": true, "until": until, "message": fmt.Sprintf("已授予 %d 天订阅，到期 %s", body.Days, until)})
	}))
	http.HandleFunc("/api/admin/activation-codes", auth.AdminAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			handleAdminListCodes(w, r)
			return
		}
		handleAdminGenCodes(w, r)
	}))
	http.HandleFunc("/api/admin/redpackets", auth.AdminAuth(handleAdminCreateRedPacket))
	http.HandleFunc("/api/newexe", func(w http.ResponseWriter, r *http.Request) {
		jsonW(w, r, M{"success": false, "message": "请先登录"})
	})

	// ── Account routes (require auth) ──
	http.HandleFunc("/api/accounts", auth.SessionAuthCustom("抱歉,安全起见,请你登录获取令牌后再继续使用", handleListAccounts))
	http.HandleFunc("/api/accounts/add", auth.SessionAuth(handleAddAccount))
	http.HandleFunc("/api/realname-presets", auth.SessionAuth(handleRealnamePresets))
	http.HandleFunc("/api/accounts/active", auth.SessionAuthCustom("抱歉,安全起见,请你登录获取令牌后再继续使用", handleActiveAccount))
	http.HandleFunc("/api/accounts/rotate", auth.SessionAuth(handleAccountRotate))
	http.HandleFunc("/api/accounts/server-owners", auth.SessionAuth(handleListServerOwners))
	http.HandleFunc("/api/accounts/", auth.SessionAuth(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/switch") {
			handleSwitchAccount(w, r)
		} else if strings.HasSuffix(path, "/refresh") {
			handleRefreshAccount(w, r)
		} else if strings.HasSuffix(path, "/update") {
			handleUpdateAccount(w, r)
		} else if strings.HasSuffix(path, "/nickname") {
			handleUpdateAccountNickname(w, r)
		} else if strings.HasSuffix(path, "/server-owner") {
			handleSetServerOwner(w, r)
		} else {
			handleDeleteAccount(w, r)
		}
	}))

	// ── P2P 节点归属（令牌 affinity 系统）──
	http.HandleFunc("/api/node/register", auth.SessionAuth(handleNodeRegister))
	http.HandleFunc("/api/node/heartbeat", auth.SessionAuth(handleNodeHeartbeat))
	http.HandleFunc("/api/node/bye", auth.SessionAuth(handleNodeBye))

	// ── Checkin & invite routes ──
	http.HandleFunc("/api/checkin", auth.SessionAuth(handleCheckin))
	http.HandleFunc("/api/checkin/status", auth.SessionAuth(handleCheckinStatus))
	http.HandleFunc("/api/invite/my-code", auth.SessionAuth(handleMyInviteCode))
	http.HandleFunc("/api/invite/redeem", auth.SessionAuth(handleRedeemInvite))

	// ── Admin routes ──
	http.HandleFunc("/api/admin/realname-preset-toggle", auth.AdminAuth(handleAdminTogglePreset))
	http.HandleFunc("/api/admin/realname-presets-all", auth.AdminAuth(handleAdminListPresets))
	http.HandleFunc("/api/admin/realname-presets/add", auth.AdminAuth(handleAdminAddPreset))
	http.HandleFunc("/api/admin/realname-presets/", auth.AdminAuth(handleAdminDeletePreset))
	http.HandleFunc("/api/admin/skin-presets", auth.AdminAuth(handleSkinPresets))
	http.HandleFunc("/api/admin/users", auth.AdminAuth(handleAdminUsers))
	http.HandleFunc("/api/admin/accounts", auth.AdminAuth(handleAdminAccounts))
	http.HandleFunc("/api/admin/logs/audit", auth.AdminAuth(handleAdminAuditLogs))
	http.HandleFunc("/api/admin/stats/overview", auth.AdminAuth(handleAdminStatsOverview))
	http.HandleFunc("/api/admin/stats/endpoints", auth.AdminAuth(handleAdminEndpointStats))
	http.HandleFunc("/api/admin/stats/users", auth.AdminAuth(handleAdminPerUserStats))
	http.HandleFunc("/api/admin/stats/registrations", auth.AdminAuth(handleAdminRegistrationStats))
	http.HandleFunc("/api/admin/stats/user-detail", auth.AdminAuth(handleAdminUserDetailStats))
	http.HandleFunc("/api/admin/stats/errors", auth.AdminAuth(handleAdminErrorStats))
	http.HandleFunc("/api/admin/stats/busy-hours", auth.AdminAuth(handleAdminBusyHours))
	http.HandleFunc("/api/admin/stats/methods", auth.AdminAuth(handleAdminMethodStats))
	http.HandleFunc("/api/admin/logs/system", auth.AdminAuth(handleAdminSystemLogs))
	http.HandleFunc("/api/admin/users/add", auth.AdminAuth(handleAdminAddUser))
	http.HandleFunc("/api/admin/users/", auth.AdminAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			handleAdminDeleteUser(w, r)
			return
		}
		handleAdminUpdateUser(w, r)
	}))
	http.HandleFunc("/api/admin/accounts/add", auth.AdminAuth(handleAdminAddAccount))
	http.HandleFunc("/api/admin/accounts/del", auth.AdminAuth(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID int64 `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ID == 0 {
			jsonResp(w, M{"ok": false, "error": msg.Get("invalid_id", "糟糕,编号无效或已失效,请刷新后重试~")})
			return
		}
		if err := db.DeleteAccountCascade(body.ID); err != nil {
			jsonResp(w, M{"ok": false, "error": sysErr(err)})
			return
		}
		jsonResp(w, M{"ok": true, "message": "deleted"})
	}))
	http.HandleFunc("/api/admin/accounts/", auth.AdminAuth(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/clone") {
			handleAdminCloneAccount(w, r)
		} else {
			handleAdminUpdateAccount(w, r)
		}
	}))
	// ── Phoenix auth endpoints ──

	http.HandleFunc("/api/new", func(w http.ResponseWriter, r *http.Request) {
		defer recordCall(r, true)
		if !newSessionLimiter.Allow(requestIP(r)) {
			http.Error(w, "429 Too Many Requests", 429)
			return
		}
		uid := fmt.Sprintf("%08x-%04x-%05x-%04x-%012x",
			rand.Uint32()&0xFFFFFFFF, rand.Uint32()&0xFFFF, (rand.Uint32()&0x0FFFFFFF)|0x50000,
			(rand.Uint32()&0x3FFF)|0x8000, rand.Uint64()&0xFFFFFFFFFFFF)
		// Create session
		sessMu.Lock()
		sessions[uid] = &sessionData{CreatedAt: time.Now(), ExpiresAt: time.Now().Add(7 * 24 * time.Hour)}
		sessMu.Unlock()
		w.Write([]byte(uid))
	})

	http.HandleFunc("/api/phoenix/server/list", limited(serverListLimiter, handleServerList))
	http.HandleFunc("/api/phoenix/server/search", limited(serverSearchLimiter, handleServerSearch))
	http.HandleFunc("/api/phoenix/server/detail", handleServerDetail)
	http.HandleFunc("/api/phoenix/server/players", handleServerPlayers)
	http.HandleFunc("/api/phoenix/server/owner", handleServerOwner)
	http.HandleFunc("/api/owner/servers/refresh", auth.SessionAuth(handleOwnerServersRefresh))
	http.HandleFunc("/api/owner/servers", auth.SessionAuth(handleOwnerServers))
	http.HandleFunc("/api/phoenix/social/search", limited(socialLimiter, handleSocialSearch))
	http.HandleFunc("/api/phoenix/social/requests", handleSocialRequests)
	http.HandleFunc("/api/phoenix/store/search", handleStoreSearch)
	http.HandleFunc("/api/phoenix/lobby/search", limited(lobbyLimiter, handleLobbySearch))
	http.HandleFunc("/api/phoenix/skin/change", auth.SessionAuth(handleSkinChange))
	http.HandleFunc("/api/phoenix/skin/presets", auth.SessionAuth(handleSkinPresets))
	http.HandleFunc("/api/phoenix/social/apply", handleSocialApply)
	http.HandleFunc("/api/phoenix/social/reply", handleSocialReply)
	http.HandleFunc("/api/phoenix/social/like", handleSocialLike)
	http.HandleFunc("/api/phoenix/social/moment", handleSocialMoment)
	http.HandleFunc("/api/phoenix/social/messages", handleSocialMessages)
	http.HandleFunc("/api/phoenix/domain/list", limited(domainLimiter, handleDomainList))
	http.HandleFunc("/api/phoenix/domain/detail", handleDomainDetail)
	http.HandleFunc("/api/phoenix/domain/join", handleDomainJoin)
	http.HandleFunc("/api/phoenix/domain/enter", handleDomainEnter)
	http.HandleFunc("/api/phoenix/domain/leave", handleDomainLeave)
	http.HandleFunc("/api/phoenix/lobby/room", handleLobbyRoom)
	http.HandleFunc("/api/phoenix/lobby/game-enter", handleLobbyGameEnter)
	http.HandleFunc("/api/phoenix/lobby/enter", handleLobbyEnter)
	http.HandleFunc("/api/phoenix/hot", limited(hotLimiter, handleHotCategories))
	http.HandleFunc("/api/phoenix/transfer_room", limited(transferRoomLimiter, handleTransferRoom))
	http.HandleFunc("/api/phoenix/server/like", handleServerLike)

	// Shorter aliases (no /phoenix prefix) — same handlers
	http.HandleFunc("/api/server/list", deprecated("请移步 /api/server/ls"))
	http.HandleFunc("/api/server/search", deprecated("请移步 /api/server/find"))
	http.HandleFunc("/api/server/ls", handleServerList)
	http.HandleFunc("/api/server/find", handleServerSearch)
	http.HandleFunc("/api/server/detail", handleServerDetail)
	http.HandleFunc("/api/server/players", handleServerPlayers)
	http.HandleFunc("/api/server/owner", handleServerOwner)
	http.HandleFunc("/api/server/like", handleServerLike)
	http.HandleFunc("/api/social/search", handleSocialSearch)
	http.HandleFunc("/api/social/requests", handleSocialRequests)
	http.HandleFunc("/api/social/apply", handleSocialApply)
	http.HandleFunc("/api/social/reply", handleSocialReply)
	http.HandleFunc("/api/social/like", handleSocialLike)
	http.HandleFunc("/api/social/moment", handleSocialMoment)
	http.HandleFunc("/api/social/messages", handleSocialMessages)
	http.HandleFunc("/api/lobby/search", handleLobbySearch)
	http.HandleFunc("/api/lobby/room", handleLobbyRoom)
	http.HandleFunc("/api/lobby/enter", handleLobbyEnter)
	http.HandleFunc("/api/domain/join", handleDomainJoin)
	http.HandleFunc("/api/hot", handleHotCategories)
	http.HandleFunc("/api/skin/change", auth.SessionAuth(handleSkinChange))
	http.HandleFunc("/api/skin/presets", auth.SessionAuth(handleSkinPresets))
	http.HandleFunc("/api/avatar/list", auth.SessionAuth(handleAvatarList))
	http.HandleFunc("/api/avatar/change", auth.SessionAuth(handleAvatarChange))
	http.HandleFunc("/api/avatar/upload", auth.SessionAuth(handleAvatarUpload))
	// 租赁服务仅主令牌可用：子令牌无权调用
	http.HandleFunc("/api/rental/list", auth.SessionAuth(auth.PrimaryToken(handleRentalServerList)))
	http.HandleFunc("/api/rental/status", auth.SessionAuth(auth.PrimaryToken(handleRentalServerStatus)))
	http.HandleFunc("/api/rental/control", auth.SessionAuth(auth.PrimaryToken(handleRentalServerControl)))
	http.HandleFunc("/api/rental/update", auth.SessionAuth(auth.PrimaryToken(handleRentalServerUpdate)))

	http.HandleFunc("/api/phoenix/transfer_start_type", func(w http.ResponseWriter, r *http.Request) {
		defer recordCall(r, true)
		if !phoenixStartLimiter.Allow(requestIP(r)) {
			jsonW(w, r, M{"success": false, "message": "请求过于频繁，请稍后再试"})
			return
		}

		content := r.URL.Query().Get("content")
		logReq(r, nil)
		if proxyTransferStartType {
			if ns := currentNv1Session(); ns != "" {
				nv1URL := "https://example.com/api/phoenix/transfer_start_type?content=" + url.QueryEscape(content)
				nReq, _ := http.NewRequest("GET", nv1URL, nil)
				nReq.Header.Set("Authorization", "Bearer "+ns)
				if nResp, nErr := http.DefaultClient.Do(nReq); nErr == nil {
					nBody, _ := io.ReadAll(nResp.Body)
					nResp.Body.Close()
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					_, _ = w.Write(nBody)
					return
				}
			}
		}
		if content == "" {
			jsonW(w, r, M{"success": false, "message": "缺少必要参数"})
			return
		}
		plain, _ := g79utils.G79HttpDecrypt(content)
		nv1ProxyUIDMu.Lock()
		uid := ""
		if bearer := getBearer(r); bearer != "" {
			sessMu.RLock()
			if s, ok := sessions[bearer]; ok {
				uid = s.UserID
			}
			sessMu.RUnlock()
		}
		if uid == "" {
			uid = nv1ProxyUID
		}
		nv1ProxyUIDMu.Unlock()
		data, _ := g79utils.G79HttpEncrypt(uid + plain)
		// Challenge override: corrupt response by flipping one hex char
		if bearer := getBearer(r); bearer != "" {
			sessMu.RLock()
			if s, ok := sessions[bearer]; ok && s.WebUserID != 0 {
				g79OverrideMu.RLock()
				if g79OverrideMap[s.WebUserID] {
					g79OverrideMu.RUnlock()
					sessMu.RUnlock()
					log.Printf("[START_TYPE] override active for userID=%d, corrupting data", s.WebUserID)
					if len(data) > 10 {
						b := []byte(data)
						b[5] ^= 1
						data = string(b)
					}
				} else {
					g79OverrideMu.RUnlock()
					sessMu.RUnlock()
				}
			} else {
				sessMu.RUnlock()
			}
		}
		jsonW(w, r, M{"success": true, "message": "ok", "data": data, "result": M{"start_type": data}, "err": ""})
	})

	http.HandleFunc("/api/phoenix/transfer_check_num", func(w http.ResponseWriter, r *http.Request) {
		defer recordCall(r, true)
		body, _ := io.ReadAll(r.Body)
		if !phoenixCheckLimiter.Allow(requestIP(r)) {
			jsonW(w, r, M{"success": false, "message": "请求过于频繁，请稍后再试"})
			return
		}

		logReq(r, body)
		var req struct {
			Data, EngineVersion, PatchVersion string
			IsPC                              *bool `json:"is_pc"`
		}
		json.Unmarshal(body, &req)
		isPC := false
		if req.IsPC != nil {
			isPC = *req.IsPC
		}
		// 版本优先级：请求体 > /api/phoenix/login 记录的 Bearer session > 上游最新版本 > 兜底值
		ev := strings.TrimSpace(req.EngineVersion)
		pv := strings.TrimSpace(req.PatchVersion)
		if bearer := getBearer(r); bearer != "" {
			log.Printf("[CHECKNUM] bearer found: len=%d", len(bearer))
			sessMu.RLock()
			if s, ok := sessions[bearer]; ok {
				log.Printf("[CHECKNUM] session found, has LoginToken=%v", s.LoginToken != "")
				if req.EngineVersion == "" && s.EngineVersion != "" {
					ev = s.EngineVersion
				}
				if req.PatchVersion == "" && s.PatchVersion != "" {
					pv = s.PatchVersion
				}
			} else {
				log.Printf("[CHECKNUM] session NOT found for bearer")
			}
			sessMu.RUnlock()
		} else {
			log.Printf("[CHECKNUM] no bearer token in request")
		}
		log.Printf("checknum: data_len=%d ev=%s pv=%s isPC=%v", len(req.Data), ev, pv, isPC)
		value, checkErr := unmcpk.GenerateTransferCheckNum(isPC, req.Data, ev, pv, os.Getenv("HUXAUTH_PYTHON3"))
		if checkErr != nil {
			log.Printf("checknum error: %v", checkErr)
			jsonW(w, r, M{"success": false, "message": fmt.Sprintf("checknum: %v", checkErr)})
			return
		}
		log.Printf("checknum result: %s", value)
		jsonW(w, r, M{
			"success": true, "message": "ok",
			"value":  value,
			"result": M{"check_num": value, "server_code": "0"},
			"err":    "",
		})
	})

	http.HandleFunc("/api/phoenix/tan_lobby", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		logReq(r, body)
		jsonW(w, r, M{"success": true})
	})

	// g79ClientFromToken 用 login_token（adb// 或裸 cookie）解析用户活跃账号并构造 g79 client。
	// 返回 (client, errorInfo)；errorInfo 非空即失败，可直接作为 error_info 返回给前端。
	g79ClientFromToken := func(token string) (*g79.Client, string) {
		var c *g79.Client
		var err error
		if strings.HasPrefix(token, "adb//") {
			u, uErr := db.GetUserByToken(token)
			if uErr != nil || u.ActiveAccountID == nil {
				return nil, "登录凭据无效或未选择活跃账号"
			}
			acc, aErr := db.GetAccountByID(*u.ActiveAccountID)
			if aErr != nil || (acc.OwnerID != nil && *acc.OwnerID != u.ID) {
				return nil, "游戏账号不存在或不属于您"
			}
			if acc.CookieData == "" {
				return nil, "游戏账号未绑定 cookie"
			}
			g79CacheMu.RLock()
			cached, hasCached := g79Cache[*u.ActiveAccountID]
			g79CacheMu.RUnlock()
			useCache := hasCached && time.Now().Before(cached.expires)
			if useCache {
				// 若缓存 client 绑定的代理最近被 code32 风控，忽略缓存重新认证，
				// 避免进服一直用坏代理缓存导致反复 32（之前只能靠手动刷新恢复）。
				poolMu.RLock()
				badCached := false
				if cur := accountProxy[*u.ActiveAccountID]; cur != "" {
					if set, ok := proxyCode32[cur]; ok && len(set) > 0 {
						badCached = true
					}
				}
				poolMu.RUnlock()
				if badCached {
					useCache = false
					g79CacheMu.Lock()
					delete(g79Cache, *u.ActiveAccountID)
					g79CacheMu.Unlock()
				}
			}
			if useCache {
				c = cached.client
			} else {
				c, err = g79.NewClientWithHTTPClient(PickOneTimeProxy())
				if err != nil {
					return nil, sysErr(err)
				}
				if err := c.G79AuthenticateWithCookie(acc.CookieData); err != nil {
					errStr := err.Error()
					// code32 = 出口IP被网易封控，切换代理重试（避免卡在坏代理，需手动刷新才恢复）
					if strings.Contains(errStr, "code: 32") {
						if nc := ReportAccountCode32(*u.ActiveAccountID); nc != nil {
							if c2, cErr := g79.NewClientWithHTTPClient(nc); cErr == nil {
								if aErr2 := c2.G79AuthenticateWithCookie(acc.CookieData); aErr2 == nil {
									g79CacheMu.Lock()
									g79Cache[*u.ActiveAccountID] = &cachedClient{client: c2, expires: time.Now().Add(25 * time.Minute)}
									g79CacheMu.Unlock()
									return c2, ""
								}
							}
						}
					}
					return nil, "认证失败: " + errStr
				}
				g79CacheMu.Lock()
				g79Cache[*u.ActiveAccountID] = &cachedClient{client: c, expires: time.Now().Add(25 * time.Minute)}
				g79CacheMu.Unlock()
			}
		} else {
			c, err = g79.NewClientWithHTTPClient(PickOneTimeProxy())
			if err != nil {
				return nil, sysErr(err)
			}
			if err := c.G79AuthenticateWithCookie(token); err != nil {
				return nil, "cookie 验证失败: " + err.Error()
			}
		}
		return c, ""
	}

	http.HandleFunc("/api/phoenix/tan_lobby_create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonW(w, r, M{"success": false, "error_info": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		body, _ := io.ReadAll(r.Body)
		logReq(r, body)
		var req struct {
			LoginToken string `json:"login_token"`
		}
		json.Unmarshal(body, &req)
		if req.LoginToken == "" {
			jsonW(w, r, M{"success": false, "error_info": "请登录并填写令牌，再继续使用"})
			return
		}
		c, errInfo := g79ClientFromToken(req.LoginToken)
		if errInfo != "" {
			jsonW(w, r, M{"success": false, "error_info": errInfo})
			return
		}
		res, err := authsvc.TanLobbyCreate(r.Context(), c)
		if err != nil {
			jsonW(w, r, M{"success": false, "error_info": sysErr(err)})
			return
		}
		jsonW(w, r, M{
			"success": true, "user_unique_id": res.UserUniqueID,
			"user_player_name":         res.UserPlayerName,
			"raknet_server_address":    res.RaknetServerAddress,
			"raknet_rand":              fmt.Sprintf("%x", res.RaknetRand),
			"raknet_aes_rand":          fmt.Sprintf("%x", res.RaknetAESRand),
			"encrypt_key_bytes":        fmt.Sprintf("%x", res.EncryptKeyBytes),
			"decrypt_key_bytes":        fmt.Sprintf("%x", res.DecryptKeyBytes),
			"signaling_server_address": res.SignalingServerAddress,
			"signaling_seed":           fmt.Sprintf("%x", res.SignalingSeed),
			"signaling_ticket":         fmt.Sprintf("%x", res.SignalingTicket),
		})
	})

	http.HandleFunc("/api/phoenix/tan_lobby_login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonW(w, r, M{"success": false, "error_info": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		body, _ := io.ReadAll(r.Body)
		logReq(r, body)
		var req struct {
			RoomID         string `json:"room_id"`
			LoginToken     string `json:"login_token"`
			GrowthOverride int    `json:"growth_override"`
		}
		json.Unmarshal(body, &req)
		if req.RoomID == "" {
			jsonW(w, r, M{"success": false, "error_info": "缺少 room_id"})
			return
		}
		if req.LoginToken == "" {
			jsonW(w, r, M{"success": false, "error_info": "请登录并填写令牌，再继续使用"})
			return
		}
		c, errInfo := g79ClientFromToken(req.LoginToken)
		if errInfo != "" {
			jsonW(w, r, M{"success": false, "error_info": errInfo})
			return
		}
		res, err := authsvc.TanLobbyLogin(r.Context(), c, authsvc.TanLobbyLoginParams{RoomID: req.RoomID})
		if err != nil {
			jsonW(w, r, M{"success": false, "error_info": sysErr(err)})
			return
		}
		botLevel := res.BotLevel
		if od, oErr := c.GetOtherUserDetail(c.UserID, false); oErr == nil {
			peLv := int(od.Entity.PEGrowth.Lv.Int64())
			if peLv > botLevel {
				botLevel = peLv
			}
		}
		if req.GrowthOverride > 0 {
			botLevel = req.GrowthOverride
		}
		jsonW(w, r, M{
			"success": true, "room_owner_id": res.RoomOwnerID,
			"user_unique_id":           res.UserUniqueID,
			"user_player_name":         res.UserPlayerName,
			"growth_level":             botLevel,
			"raknet_server_address":    res.RaknetServerAddress,
			"signaling_server_address": res.SignalingServerAddress,
			"raknet_rand":              fmt.Sprintf("%x", res.RaknetRand),
			"raknet_aes_rand":          fmt.Sprintf("%x", res.RaknetAESRand),
			"encrypt_key_bytes":        fmt.Sprintf("%x", res.EncryptKeyBytes),
			"decrypt_key_bytes":        fmt.Sprintf("%x", res.DecryptKeyBytes),
			"signaling_seed":           fmt.Sprintf("%x", res.SignalingSeed),
			"signaling_ticket":         fmt.Sprintf("%x", res.SignalingTicket),
			"room_mod_display_name":    res.RoomModDisplayName,
			"room_mod_download_url":    res.RoomModDownloadURL,
		})
	})

	http.HandleFunc("/api/phoenix/tan_lobby_transfer_server", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonW(w, r, M{"success": false, "message": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		logReq(r, nil)
		raknetServers, websocketServers, err := authsvc.TransferServerList()
		if err != nil {
			jsonW(w, r, M{"success": false, "error": sysErr(err)})
			return
		}
		jsonW(w, r, M{"success": true, "raknet_servers": raknetServers, "websocket_servers": websocketServers})
	})

	// tan_lobby_rental_target 用房主活跃账号把租赁服号解析成可拨号地址，供联机大厅中继使用。
	http.HandleFunc("/api/phoenix/tan_lobby_rental_target", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonW(w, r, M{"success": false, "error_info": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		if !tanLobbyTargetLimiter.Allow(requestIP(r)) {
			jsonW(w, r, M{"success": false, "error_info": "请求过于频繁，请稍后再试"})
			return
		}
		body, _ := io.ReadAll(r.Body)
		logReq(r, body)
		var req struct {
			LoginToken string `json:"login_token"`
			ServerCode string `json:"server_code"`
			Password   string `json:"password"`
		}
		json.Unmarshal(body, &req)
		if req.LoginToken == "" {
			jsonW(w, r, M{"success": false, "error_info": "请登录并填写令牌，再继续使用"})
			return
		}
		if req.ServerCode == "" {
			jsonW(w, r, M{"success": false, "error_info": "缺少 server_code"})
			return
		}
		c, errInfo := g79ClientFromToken(req.LoginToken)
		if errInfo != "" {
			jsonW(w, r, M{"success": false, "error_info": errInfo})
			return
		}
		ipAddress, err := authsvc.ResolveRentalServerAddress(c, req.ServerCode, req.Password)
		if err != nil {
			jsonW(w, r, M{"success": false, "error_info": sysErr(err)})
			return
		}
		jsonW(w, r, M{"success": true, "ip_address": ipAddress})
	})

	http.HandleFunc("/api/phoenix/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		success := false
		defer func() { recordCall(r, success) }()
		if r.Method == "OPTIONS" {
			return
		}

		if !phoenixLoginLimiter.Allow(requestIP(r)) {
			jsonW(w, r, M{"success": false, "message": "请求过于频繁，请稍后再试"})
			return
		}
		if r.Method == "GET" {
			jsonW(w, r, M{"success": true, "message": "ok"})
			return
		}

		body, _ := io.ReadAll(r.Body)
		logReq(r, body)
		var req LoginReq
		json.Unmarshal(body, &req)
		sc := req.ServerCode
		domainChoice := -1
		_ = domainChoice
		if m := domainChoiceRe.FindStringSubmatch(sc); len(m) == 2 {
			domainChoice, _ = strconv.Atoi(m[1])
			sc = strings.TrimSuffix(sc, m[0])
		}
		// 联机大厅开关：关闭时拒绝所有以 # 开头的联机房间请求
		if !enableLobby && strings.HasPrefix(sc, "#") && len(sc) > 1 {
			jsonW(w, r, LoginResp{Success: false, Message: "联机大厅已禁用"})
			return
		}
		db.AddSystemLog("info", "进服请求: "+sc, "")

		getAuditUser := func() (int64, int64) {
			if strings.HasPrefix(req.FBToken, "adb//") {
				if u, apiTok, uErr := db.ResolveToken(req.FBToken); uErr == nil && u != nil {
					aid := int64(0)
					if accID := db.ResolveActiveAccountID(u, apiTok); accID != nil {
						aid = *accID
					}
					return u.ID, aid
				}
			}
			sessMu.RLock()
			defer sessMu.RUnlock()
			if bearer := getBearer(r); bearer != "" {
				if s, ok := sessions[bearer]; ok {
					return s.WebUserID, 0
				}
			}
			return 0, 0
		}
		// ::DRY:: — 连通性测试
		if sc == "::DRY::" {
			jsonW(w, r, LoginResp{Success: true, Message: "ok"})
			return
		}
		loginUser := ""
		if strings.HasPrefix(req.FBToken, "adb//") {
			if u, ue := db.GetUserByToken(req.FBToken); ue == nil && u != nil {
				loginUser = fmt.Sprintf(" user=%s(%d)", u.Username, u.ID)
				db.IncrementLoginCount(u.ID)
				db.ProcessInviteJoinRewards(u.ID)
				u.LoginCount++
				if enableActivationCheck && u.LoginCount > 3 && !u.Activated {
					jsonW(w, r, LoginResp{Success: false, Message: "试用次数已用完，请登录网页加入QQ群获取激活码"})
					return
				}
			}
		}
		log.Printf("login: server=%s%s", sc, loginUser)
		if ns := currentNv1Session(); ns != "" && strings.HasPrefix(req.FBToken, "nv1//") {
			lReq, _ := http.NewRequest("POST", "https://example.com/api/phoenix/login", strings.NewReader(string(body)))
			lReq.Header.Set("Content-Type", "application/json")
			lReq.Header.Set("Authorization", "Bearer "+ns)
			if lResp, lErr := http.DefaultClient.Do(lReq); lErr == nil {
				lBody, _ := io.ReadAll(lResp.Body)
				lResp.Body.Close()
				// Store the NV1 UID for local start_type to use
				var nv1LoginResp LoginResp
				if json.Unmarshal(lBody, &nv1LoginResp) == nil && nv1LoginResp.UID != "" {
					nv1ProxyUIDMu.Lock()
					nv1ProxyUID = nv1LoginResp.UID
					nv1ProxyUIDMu.Unlock()
				}
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				_, _ = w.Write(lBody)
				return
			}
		}
		// login_token 支持三种来源：adb//网站用户 token、直接 cookie JSON、旧全局活跃 cookie
		if req.FBToken == "" && req.Authorization != "" {
			req.FBToken = req.Authorization
		}
		if strings.HasPrefix(req.FBToken, "adb//adb//") {
			req.FBToken = "adb//" + req.FBToken[10:]
		}
		// 占位符处理：非 adb// 开头、非 cookie JSON 的当做未设置
		if req.FBToken != "" && !strings.HasPrefix(req.FBToken, "adb//") && !strings.HasPrefix(req.FBToken, "{\"sauth_json") && !strings.HasPrefix(req.FBToken, "{\"gameid") {
			req.FBToken = ""
		}
		var c *g79.Client
		var guestAcc *db.GameAccount // non-nil when using guest pool
		if strings.HasPrefix(req.FBToken, "adb//") {
			u, apiTok, uErr := db.ResolveToken(req.FBToken)
			// 先判空再解引用,避免无效 token(u==nil)时 ResolveActiveAccountID 空指针 panic。
			if uErr != nil || u == nil {
				jsonW(w, r, LoginResp{Success: false, Message: "登录凭据无效，请检查后在网站中选择要使用的游戏账号"})
				db.AddSystemLog("warn", "登录失败: 无效token", "server="+sc)
				return
			}
			activeAccountID := db.ResolveActiveAccountID(u, apiTok)
			if activeAccountID == nil {
				jsonW(w, r, LoginResp{Success: false, Message: "登录凭据有效，但未选择活跃账号，请在网站中选择要使用的游戏账号"})
				db.AddSystemLog("warn", "登录失败: 未选择活跃账号", "server="+sc)
				return
			}
			if u.Disabled {
				jsonW(w, r, LoginResp{Success: false, Message: "您的网站账号已被管理员停用"})
				return
			}
			// 次令牌：调用次数上限检查 + 标记本次调用归属
			if apiTok != nil {
				if err := db.IncTokenCall(apiTok.ID); err != nil {
					jsonW(w, r, LoginResp{Success: false, Message: sysErr(err)})
					db.AddSystemLog("warn", "进服失败: 令牌调用上限", fmt.Sprintf("server=%s token#%d", sc, apiTok.ID))
					return
				}
				r = r.WithContext(context.WithValue(r.Context(), apiTokenCtxKey{}, apiTokenCtxVal{ID: apiTok.ID, Name: apiTok.Name}))
			}
			acc, aErr := db.GetAccountByID(*activeAccountID)
			if aErr != nil || (acc.OwnerID != nil && *acc.OwnerID != u.ID) {
				jsonW(w, r, LoginResp{Success: false, Message: "活跃游戏账号不存在或不属于您"})
				return
			}
			if acc.Disabled {
				jsonW(w, r, LoginResp{Success: false, Message: "活跃游戏账号已被停用，请选择其他账号"})
				return
			}
			// Try cache first
			g79CacheMu.RLock()
			if cached, ok := g79Cache[*activeAccountID]; ok && time.Now().Before(cached.expires) {
				c = cached.client
			}
			g79CacheMu.RUnlock()
			if c == nil {
				c, _ = newG79ClientWithProxy(*activeAccountID)
				if c != nil {
					if err := c.G79AuthenticateWithCookie(acc.CookieData); err != nil {
						msg := err.Error()
						// 仅永久封禁（code 29）触发自动切换；code 32/2100/27003 等为临时错误，不切换
						if bannedByMessage(msg) {
							bannedName := acc.DisplayName
							log.Printf("[BAN] 账号 %s(%d) 被封禁，尝试自动切换", bannedName, *activeAccountID)
							switched := false
							userAccs, _ := db.GetUserAccounts(u.ID)
							for _, candidate := range userAccs {
								if candidate.ID == *activeAccountID {
									continue
								}
								if candidate.Disabled || candidate.CookieData == "" || candidate.IsServerOwner {
									continue
								}
								if candidate.OwnerID != nil && *candidate.OwnerID != u.ID {
									continue
								}
								cc, cErr := newG79ClientWithProxy(candidate.ID)
								if cErr != nil {
									continue
								}
								if cErr := cc.G79AuthenticateWithCookie(candidate.CookieData); cErr != nil {
									continue
								}
								db.SetActiveAccount(u.ID, candidate.ID)
								g79CacheMu.Lock()
								g79Cache[candidate.ID] = &cachedClient{client: cc, expires: time.Now().Add(25 * time.Minute)}
								g79CacheMu.Unlock()
								startAccountHeartbeat(candidate.ID, cc)
								c = cc
								acc = candidate
								log.Printf("[BAN] 已自动从 %s 切换到账号 %s(%d)", bannedName, candidate.DisplayName, candidate.ID)
								switched = true
								break
							}
							if !switched {
								jsonW(w, r, LoginResp{Success: false, Message: fmt.Sprintf("账号 %s 已被封禁，且无可用备用账号", bannedName)})
								return
							}
						} else {
							jsonW(w, r, LoginResp{Success: false, Message: fmt.Sprintf("认证失败: %v", err)})
							return
						}
					}
					g79CacheMu.Lock()
					g79Cache[*activeAccountID] = &cachedClient{client: c, expires: time.Now().Add(25 * time.Minute)}
					g79CacheMu.Unlock()
					startAccountHeartbeat(*activeAccountID, c)
				}
			}
		} else if strings.HasPrefix(req.FBToken, "{\"sauth_json") || strings.HasPrefix(req.FBToken, "{\"gameid") {
			c, _ = g79.NewClient()
			if c != nil {
				if err := c.G79AuthenticateWithCookie(req.FBToken); err != nil {
					c = nil
					db.AddSystemLog("warn", "Cookie认证失败", err.Error()[:min(len(err.Error()), 100)])
				}
			}
		} else if req.FBToken == "" && req.Authorization == "" {
			// ── Guest mode: no token provided ──
			// 判断是否联机/本地联机房间（#xxx / @xxx）— 均需要登录
			isLobbyPrefix := strings.HasPrefix(sc, "LobbyGame:") || strings.HasPrefix(sc, "PCLobbyGame:")
			isLobbyHash := strings.HasPrefix(sc, "#") && len(sc) > 1
			isTanHash := strings.HasPrefix(sc, "@") && len(sc) > 1
			if isLobbyPrefix || isLobbyHash || isTanHash {
				jsonW(w, r, LoginResp{Success: false, Message: "联机/本地联机房间需要登录，请先获取登录凭据"})
				return
			}
			// 租赁服 / 山头服 → 使用 GuestPool（自动重试最多3个账号）
			guestAcc = getGuestAccount()
			if guestAcc == nil {
				jsonW(w, r, LoginResp{Success: false, Message: "暂无可用共享账号，请稍后再试"})
				return
			}
			// Try cache
			g79CacheMu.RLock()
			if cached, ok := g79Cache[guestAcc.ID]; ok && time.Now().Before(cached.expires) {
				c = cached.client
			}
			g79CacheMu.RUnlock()
			if c == nil {
				for i := 0; i < 3; i++ {
					cc, ccErr := newG79ClientWithProxy(guestAcc.ID)
					if ccErr != nil {
						continue
					}
					if ccErr := cc.G79AuthenticateWithCookie(guestAcc.CookieData); ccErr != nil {
						log.Printf("[GUEST] #%d auth failed (attempt %d/3): %v", guestAcc.ID, i+1, ccErr)
						guestAcc = forceRotateGuestAccount()
						if guestAcc == nil {
							break
						}
						continue
					}
					c = cc
					g79CacheMu.Lock()
					g79Cache[guestAcc.ID] = &cachedClient{client: c, expires: time.Now().Add(25 * time.Minute)}
					g79CacheMu.Unlock()
					startAccountHeartbeat(guestAcc.ID, c)
					break
				}
			}
		}
		if c == nil {
			uid, aid := getAuditUser()
			db.AddAuditLog(&uid, &aid, "join_server_failed", sc, "认证失败: 无法创建客户端", requestIP(r))
			db.AddSystemLog("warn", "无法创建客户端", "所有认证方式均失败")
			jsonW(w, r, LoginResp{Success: false, Message: "登录凭据无效或已过期，请重新登录网站获取新凭据"})
			return
		}

		// ── 判断 server_code 类型 ──
		// 山头服特征：URL链接、DomainGame:前缀、或16进制邀请码
		isDomainURL := strings.Contains(sc, "realms=") || strings.HasPrefix(sc, "http")
		isDomainPrefix := strings.HasPrefix(sc, "DomainGame:") || strings.HasPrefix(sc, "PCDomainGame:")
		// 联机房间：以 # 开头或 LobbyGame:/PCLobbyGame: 前缀
		isLobbyPrefix := strings.HasPrefix(sc, "LobbyGame:") || strings.HasPrefix(sc, "PCLobbyGame:")
		isLobbyHash := strings.HasPrefix(sc, "#") && len(sc) > 1
		isLobby := isLobbyPrefix || isLobbyHash
		// 本地联机（TAN 房间）：以 @ 开头
		isTanHash := strings.HasPrefix(sc, "@") && len(sc) > 1

		isRental := true
		if isLobby {
			isRental = false
		} else if isTanHash {
			isRental = false
		} else if isDomainURL || isDomainPrefix {
			isRental = false
		} else {
			// 检查是否为山头服邀请码（必须含 a-f，纯数字走租赁服）
			isHex := len(sc) >= 8
			hasLetter := false
			for _, ch := range sc {
				if (ch >= 'A' && ch <= 'F') || (ch >= 'a' && ch <= 'f') {
					hasLetter = true
				}
				if !((ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'F') || (ch >= 'a' && ch <= 'f')) {
					isHex = false
					break
				}
			}
			if isHex && hasLetter {
				isRental = false
			} else {
				for _, ch := range sc {
					if ch < '0' || ch > '9' {
						isRental = false
						break
					}
				}
			}
		}

		var sid, ip string
		var av2 []byte

		if isRental {
			// Guest mode retry: try up to 3 different accounts for rental server
			var guestRetryErr error
			for guestRetry := 0; guestRetry < 3; guestRetry++ {
				if guestRetry > 0 && guestAcc != nil {
					// Rotate to a different guest account
					log.Printf("[GUEST] rental server failed, rotating account (attempt %d/3)", guestRetry+1)
					guestAcc = forceRotateGuestAccount()
					if guestAcc == nil {
						break
					}
					// Re-create client with new account
					cc, ccErr := newG79ClientWithProxy(guestAcc.ID)
					if ccErr != nil {
						continue
					}
					if ccErr := cc.G79AuthenticateWithCookie(guestAcc.CookieData); ccErr != nil {
						continue
					}
					c = cc
					g79CacheMu.Lock()
					g79Cache[guestAcc.ID] = &cachedClient{client: c, expires: time.Now().Add(25 * time.Minute)}
					g79CacheMu.Unlock()
					startAccountHeartbeat(guestAcc.ID, c)
				}
				sr, err := c.SearchRentalServerByName(sc)
				if err != nil || sr.Code != 0 || len(sr.Entities) == 0 {
					guestRetryErr = fmt.Errorf("找不到服务器 %s", sc)
					if guestAcc == nil {
						break
					}
					continue
				}
				sid = sr.Entities[0].EntityID.String()
				er, err := c.EnterRentalServerWorld(sid, req.ServerPassword)
				if err != nil || er.Code != 0 {
					guestRetryErr = fmt.Errorf("进入失败: %v", er)
					if guestAcc == nil {
						break
					}
					continue
				}
				ip = fmt.Sprintf("%s:%d", er.Entity.McserverHost, er.Entity.McserverPort.Int64())
				av2, _ = c.GenerateRentalGameAuthV2(sid, req.ClientPublicKey)
				guestRetryErr = nil
				break
			}
			if guestRetryErr != nil {
				uid, aid := getAuditUser()
				db.AddAuditLog(&uid, &aid, "join_server_failed", sc, guestRetryErr.Error(), requestIP(r))
				db.AddSystemLog("warn", "进入失败: "+sc, guestRetryErr.Error())
				jsonW(w, r, LoginResp{Success: false, Message: guestRetryErr.Error()})
				return
			}
		} else if isLobby {
			// 联机房间: 提取房间号（支持 #房间号、LobbyGame:ID、PCLobbyGame:ID）
			roomID := strings.TrimPrefix(sc, "LobbyGame:")
			roomID = strings.TrimPrefix(roomID, "PCLobbyGame:")
			roomID = strings.TrimPrefix(roomID, "#")
			roomID = strings.TrimSpace(roomID)
			if roomID == "" {
				uid, aid := getAuditUser()
				db.AddAuditLog(&uid, &aid, "join_server_failed", sc, "联机房间号为空", requestIP(r))
				jsonW(w, r, LoginResp{Success: false, Message: "房间号不能为空"})
				return
			}
			if len(roomID) < 10 {
				uid, aid := getAuditUser()
				db.AddAuditLog(&uid, &aid, "join_server_failed", sc, "房间号位数不足", requestIP(r))
				jsonW(w, r, LoginResp{Success: false, Message: "请使用19位ID进入"})
				return
			}
			log.Printf("lobby enter: roomID=%s (from %s)", roomID, sc)

			// 查房间详情 - 短号搜不到就自动用搜索API找 entity_id
			roomInfo, rErr := c.GetOnlineLobbyRoom(roomID)
			if rErr != nil || roomInfo.Code != 0 {
				if rErr != nil {
					log.Printf("lobby: direct get failed for %s err=%v, trying keyword search...", roomID, rErr)
				} else {
					log.Printf("lobby: direct get failed for %s code=%d msg=%q, trying keyword search...", roomID, roomInfo.Code, roomInfo.Message)
				}
				sr, sErr := c.SearchOnlineLobbyRoomByKeyword(roomID, 20, 0)
				if sErr == nil && sr != nil && sr.Code == 0 && len(sr.Entities) > 0 {
					log.Printf("lobby: search %q returned %d results; first room_id=%s entity_id=%s name=%q", roomID, len(sr.Entities), sr.Entities[0].RoomID, sr.Entities[0].EntityID, sr.Entities[0].RoomName)
					eid := sr.Entities[0].ID()
					if eid != "" && eid != roomID {
						log.Printf("lobby: found entity_id %s for room %s (first result)", eid, roomID)
						roomID = eid
						roomInfo, rErr = c.GetOnlineLobbyRoom(roomID)
						if rErr != nil {
							log.Printf("lobby: re-get after search failed for %s err=%v", roomID, rErr)
						} else {
							log.Printf("lobby: re-get after search code=%d for %s", roomInfo.Code, roomID)
						}
					}
				}
				// 搜不到用"1"兜底搜全局
				if (rErr != nil || roomInfo == nil || roomInfo.Code != 0) && len(roomID) < 10 {
					if sr2, sErr2 := c.SearchOnlineLobbyRoomByKeyword("1", 50, 0); sErr2 == nil && sr2 != nil && sr2.Code == 0 {
						for _, e := range sr2.Entities {
							if e.RoomName == roomID {
								eid := e.ID()
								if eid != "" && eid != roomID {
									roomID = eid
									roomInfo, rErr = c.GetOnlineLobbyRoom(roomID)
									if rErr == nil && roomInfo != nil && roomInfo.Code == 0 {
										break
									}
								}
							}
						}
					}
				}
				if rErr != nil || roomInfo == nil || roomInfo.Code != 0 {
					// 联机大厅找不到，尝试 TAN 本地联机
					if tanResult, tanErr := authsvc.TanLobbyLogin(r.Context(), c, authsvc.TanLobbyLoginParams{RoomID: roomID}); tanErr == nil {
						log.Printf("lobby: TAN room found for %s as fallback", roomID)
						jsonW(w, r, M{
							"success": true, "user_unique_id": tanResult.UserUniqueID,
							"user_player_name":         tanResult.UserPlayerName,
							"growth_level":             tanResult.BotLevel,
							"raknet_server_address":    tanResult.RaknetServerAddress,
							"signaling_server_address": tanResult.SignalingServerAddress,
							"raknet_rand":              fmt.Sprintf("%x", tanResult.RaknetRand),
							"raknet_aes_rand":          fmt.Sprintf("%x", tanResult.RaknetAESRand),
							"encrypt_key_bytes":        fmt.Sprintf("%x", tanResult.EncryptKeyBytes),
							"decrypt_key_bytes":        fmt.Sprintf("%x", tanResult.DecryptKeyBytes),
							"signaling_seed":           fmt.Sprintf("%x", tanResult.SignalingSeed),
							"signaling_ticket":         fmt.Sprintf("%x", tanResult.SignalingTicket),
							"room_mod_display_name":    tanResult.RoomModDisplayName,
							"room_mod_download_url":    tanResult.RoomModDownloadURL,
						})
						return
					}
					uid, aid := getAuditUser()
					db.AddAuditLog(&uid, &aid, "join_server_failed", sc, "找不到联机房间", requestIP(r))
					jsonW(w, r, LoginResp{Success: false, Message: fmt.Sprintf("找不到联机房间: %v", rErr)})
					return
				}
			}

			// 尝试进入房间（501 = 需购买 → 购买后重试）
			resID := roomInfo.Entity.ResID.String()
			enterOK := false
			for attempt := 1; attempt <= 3; attempt++ {
				er, eErr := c.EnterOnlineLobbyRoom(roomID, req.ServerPassword)
				if eErr != nil {
					uid, aid := getAuditUser()
					db.AddAuditLog(&uid, &aid, "join_server_failed", sc, "进房网络错误", requestIP(r))
					jsonW(w, r, LoginResp{Success: false, Message: eErr.Error()})
					return
				}
				if er.Code == 0 {
					enterOK = true
					break
				}
				if er.Code != 501 {
					uid, aid := getAuditUser()
					db.AddAuditLog(&uid, &aid, "join_server_failed", sc, fmt.Sprintf("进房失败 code=%d", er.Code), requestIP(r))
					jsonW(w, r, LoginResp{Success: false, Message: fmt.Sprintf("进房失败: %s(%d)", er.Message, er.Code)})
					return
				}
				if attempt < 3 {
					c.PurchaseItem(resID)
					time.Sleep(500 * time.Millisecond)
				}
			}
			if !enterOK {
				uid, aid := getAuditUser()
				db.AddAuditLog(&uid, &aid, "join_server_failed", sc, "进房多次失败", requestIP(r))
				jsonW(w, r, LoginResp{Success: false, Message: "进入房间失败: 需要购买商品但多次尝试失败"})
				return
			}

			// 获取游戏服务器地址
			ge, gErr := c.OnlineLobbyGameEnter()
			if gErr != nil || ge.Code != 0 {
				uid, aid := getAuditUser()
				db.AddAuditLog(&uid, &aid, "join_server_failed", sc, "获取游戏服务器失败", requestIP(r))
				jsonW(w, r, LoginResp{Success: false, Message: fmt.Sprintf("获取游戏服务器失败: %v", gErr)})
				return
			}
			ip = ge.Entity.BestAddr()
			log.Printf("lobby game addr: %s", ip)

			// PC 版使用 PC 认证
			isPC := strings.HasPrefix(sc, "PCLobbyGame:")
			if isPC {
				av2, _ = c.GeneratePCLobbyGameAuthV2(roomID, req.ClientPublicKey)
			} else {
				av2, _ = c.GenerateLobbyGameAuthV2(roomID, req.ClientPublicKey)
			}
		} else if isTanHash {
			// 本地联机: 剥离 @ 标识符后查询进入房间。
			// 之前把 @ 一并带入房间号导致查询接口找不到房间，这里统一剥离。
			roomID := strings.TrimSpace(strings.TrimPrefix(sc, "@"))
			if roomID == "" {
				uid, aid := getAuditUser()
				db.AddAuditLog(&uid, &aid, "join_server_failed", sc, "本地联机房间号为空", requestIP(r))
				jsonW(w, r, LoginResp{Success: false, Message: "本地联机房间号不能为空"})
				return
			}
			log.Printf("tan lobby enter: roomID=%s (from %s)", roomID, sc)
			tanResult, tanErr := authsvc.TanLobbyLogin(r.Context(), c, authsvc.TanLobbyLoginParams{RoomID: roomID})
			if tanErr != nil {
				uid, aid := getAuditUser()
				db.AddAuditLog(&uid, &aid, "join_server_failed", sc, tanErr.Error(), requestIP(r))
				db.AddSystemLog("warn", "本地联机进房失败: "+sc, tanErr.Error())
				jsonW(w, r, LoginResp{Success: false, Message: "找不到本地联机房间: " + tanErr.Error()})
				return
			}
			jsonW(w, r, M{
				"success":                  true,
				"user_unique_id":           tanResult.UserUniqueID,
				"user_player_name":         tanResult.UserPlayerName,
				"growth_level":             tanResult.BotLevel,
				"raknet_server_address":    tanResult.RaknetServerAddress,
				"signaling_server_address": tanResult.SignalingServerAddress,
				"raknet_rand":              fmt.Sprintf("%x", tanResult.RaknetRand),
				"raknet_aes_rand":          fmt.Sprintf("%x", tanResult.RaknetAESRand),
				"encrypt_key_bytes":        fmt.Sprintf("%x", tanResult.EncryptKeyBytes),
				"decrypt_key_bytes":        fmt.Sprintf("%x", tanResult.DecryptKeyBytes),
				"signaling_seed":           fmt.Sprintf("%x", tanResult.SignalingSeed),
				"signaling_ticket":         fmt.Sprintf("%x", tanResult.SignalingTicket),
				"room_mod_display_name":    tanResult.RoomModDisplayName,
				"room_mod_download_url":    tanResult.RoomModDownloadURL,
			})
			return
		} else {
			// 山头服: 提取邀请码（支持 URL 和纯码）
			code := strings.TrimPrefix(sc, "DomainGame:")
			code = strings.TrimPrefix(code, "PCDomainGame:")
			if strings.Contains(sc, "realms=") || strings.HasPrefix(sc, "http") {
				if u, uErr := url.Parse(sc); uErr == nil {
					if rv := u.Query().Get("realms"); rv != "" {
						code = rv
					}
				} else if idx := strings.Index(sc, "realms="); idx >= 0 {
					code = sc[idx+7:]
					if semi := strings.IndexAny(code, "& \t\n"); semi >= 0 {
						code = code[:semi]
					}
				}
			}
			code = strings.TrimSpace(code)
			log.Printf("domain join: code=%s (from %s)", code, sc)

			if m := domainChoiceRe.FindStringSubmatch(code); len(m) == 2 {
				domainChoice, _ = strconv.Atoi(m[1])
				code = strings.TrimSuffix(code, m[0])
				log.Printf("domain join: extracted choice=%d code=%s", domainChoice, code)
			}

			// 校验邀请码格式：至少8位十六进制，避免无效请求污染 G79 客户端
			validCode := len(code) >= 8
			for _, ch := range code {
				if !((ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'F') || (ch >= 'a' && ch <= 'f')) {
					validCode = false
					break
				}
			}
			if !validCode {
				log.Printf("domain join: invalid code format, skipping G79 call")
				uid, aid := getAuditUser()
				db.AddAuditLog(&uid, &aid, "join_server_failed", sc, "山头服邀请码格式无效", requestIP(r))
				jsonW(w, r, LoginResp{Success: false, Message: "邀请码格式无效，请检查链接是否正确"})
				return
			}

			// 先查缓存是否有该邀请码对应的 sid
			leaveAll := domainChoice == 0 // _0: 退出所有山头服后重新加入
			needJoin := false
			joinOK := false
			var joinErrMsg string // 透传 join 失败的真实原因，避免笼统误报"邀请码无效或已过期"
			domainSidCacheMu.RLock()
			cachedSid := domainSidCache[c.UserID+":"+code]
			domainSidCacheMu.RUnlock()
			if cachedSid != "" && domainChoice == 0 {
				// _0：清除缓存，退出所有山头服后重新加入
				domainSidCacheMu.Lock()
				delete(domainSidCache, c.UserID+":"+code)
				domainSidCacheMu.Unlock()
				log.Printf("domain join: _0 mode, will leave all and re-join")
				needJoin = true
			} else if cachedSid != "" {
				// 验证缓存的 sid 是否还在列表中
				cachedSidOk := false
				if sv, e2 := c.GetOtherDomainServers(); e2 == nil && sv != nil {
					for _, e := range sv.Entities {
						if e.Sid == cachedSid {
							cachedSidOk = true
							break
						}
					}
				}
				if cachedSidOk {
					sid = cachedSid
					joinOK = true
					log.Printf("domain join: cache hit sid=%s for code=%s", cachedSid, code)
				} else {
					// 缓存过期：清除后重新加入
					domainSidCacheMu.Lock()
					delete(domainSidCache, c.UserID+":"+code)
					domainSidCacheMu.Unlock()
					log.Printf("domain join: cache stale for sid=%s, will re-join", cachedSid)
					needJoin = true
				}
			} else {
				// 无缓存：需要加入
				needJoin = true
			}

			if needJoin {
				// 先记住已加入的山头服列表，方便 join 后对比找新服
				oldDomains := map[string]bool{}
				if leaveAll {
					// _0: 退出所有山头服（轮询确认清空），然后重新加入获取真实服务器
					leaveAllDomainServers(c)
					// 退出后列表已确认清空，快照清空，方便 join 后对比找新服
					oldDomains = map[string]bool{}
				} else {
					if preList, preErr := c.GetOtherDomainServers(); preErr == nil && preList != nil {
						for _, e := range preList.Entities {
							oldDomains[e.Sid] = true
						}
					}
				}
				jr, jErr := c.JoinDomainServerWithInviteCode(code)
				joinOK = jErr == nil && (jr.Code == 0 || jr.Code == 1005 || jr.Code == 22)
				if !joinOK {
					// 域名操作污染了客户端，清除缓存
					uid, _ := getAuditUser()
					g79CacheMu.Lock()
					for k := range g79Cache {
						delete(g79Cache, k)
					}
					g79CacheMu.Unlock()
					_ = uid
					// 透传服务器真实原因，不再笼统报"邀请码无效或已过期"
					if jErr != nil {
						joinErrMsg = "进入山头服失败: " + jErr.Error()
					} else if jr != nil {
						switch jr.Code {
						case 1006:
							joinErrMsg = "山头服人数已满，请稍后再试"
						case 1014:
							joinErrMsg = "邀请码无效，请检查链接是否正确"
						default:
							if jr.Message != "" {
								joinErrMsg = "进入山头服失败: " + jr.Message
							} else {
								joinErrMsg = fmt.Sprintf("进入山头服失败 code=%d", jr.Code)
							}
						}
					}
				}
				// 从 entity 或 entities[0] 提取 sid
				if jErr == nil && jr.Entity != nil {
					if s, ok := jr.Entity["sid"].(string); ok && s != "" {
						sid = s
					} else if sn, ok := jr.Entity["sid"].(float64); ok && sn != 0 {
						sid = fmt.Sprintf("%.0f", sn)
					}
					if sid != "" {
						domainSidCacheMu.Lock()
						domainSidCache[c.UserID+":"+code] = sid
						domainSidCacheMu.Unlock()
					}
				}
				if sid == "" && jErr == nil && len(jr.Entities) > 0 {
					if s, ok := jr.Entities[0]["sid"].(string); ok && s != "" {
						sid = s
					} else if sn, ok := jr.Entities[0]["sid"].(float64); ok && sn != 0 {
						sid = fmt.Sprintf("%.0f", sn)
					}
				}
				// 如果 join 成功但 sid 仍为空，对比列表找到目标山头服
				if sid == "" && joinOK {
					if newList, nErr := c.GetOtherDomainServers(); nErr == nil && newList != nil {
						// 优先找新出现的服（首次加入 code=0 的情况）
						for _, e := range newList.Entities {
							if !oldDomains[e.Sid] {
								sid = e.Sid
								log.Printf("domain join: found new server sid=%s by list diff", sid)
								domainSidCacheMu.Lock()
								domainSidCache[c.UserID+":"+code] = sid
								domainSidCacheMu.Unlock()
								break
							}
						}
					}
					// 没新服且只有一个服 → 直接用它（已加入 code=1005 的情况）
					if sid == "" {
						if newList, nErr := c.GetOtherDomainServers(); nErr == nil && newList != nil {
							if len(newList.Entities) == 1 {
								sid = newList.Entities[0].Sid
								log.Printf("domain join: only one domain server, using sid=%s", sid)
								domainSidCacheMu.Lock()
								domainSidCache[c.UserID+":"+code] = sid
								domainSidCacheMu.Unlock()
							} else if domainChoice >= 1 && domainChoice <= len(newList.Entities) {
								sid = newList.Entities[domainChoice-1].Sid
								log.Printf("domain join: using choice #%d sid=%s name=%s", domainChoice, sid, newList.Entities[domainChoice-1].Name)
								domainSidCacheMu.Lock()
								domainSidCache[c.UserID+":"+code] = sid
								domainSidCacheMu.Unlock()
							} else if domainChoice > len(newList.Entities) {
								jsonW(w, r, LoginResp{Success: false, Message: fmt.Sprintf(">>序号 %d 无效，你有 %d 个山头服，请在邀请链接末尾加上 _1~_%d 选择，_0 清理缓存<<", domainChoice, len(newList.Entities), len(newList.Entities))})
								return
							} else if len(newList.Entities) == 0 {
								// 服务器列表为空：可能服务器已过期或已被删除
								uid, aid := getAuditUser()
								db.AddAuditLog(&uid, &aid, "join_server_failed", sc, "山头服列表为空", requestIP(r))
								jsonW(w, r, LoginResp{Success: false, Message: ">>山头服列表中无可用服务器，该服务器可能已过期或已被删除，请确认邀请码是否正确<<"})
								return
							} else {
								var sb strings.Builder
								sb.WriteString(fmt.Sprintf(">>你有 %d 个山头服，请在邀请链接末尾加上 _1~_%d 选择，_0 清理缓存：", len(newList.Entities), len(newList.Entities)))
								for i, e := range newList.Entities {
									sb.WriteString(fmt.Sprintf(" [%d] %s", i+1, e.Name))
								}
								sb.WriteString("<<")
								jsonW(w, r, LoginResp{Success: false, Message: sb.String()})
								return
							}
						}
					}
				}
			}
			if sid == "" {
				msg := "邀请码无效或已过期，请检查链接是否正确，请尝试更换账号后重试"
				if joinOK {
					msg = "已加入山头服，但未找到可进入的服务器，请尝试更换账号后重试"
				} else if joinErrMsg != "" {
					msg = joinErrMsg
				}
				uid, aid := getAuditUser()
				db.AddAuditLog(&uid, &aid, "join_server_failed", sc, msg, requestIP(r))
				jsonW(w, r, LoginResp{Success: false, Message: msg})
				return
			}
			log.Printf("domain enter: sid=%s", sid)

			// clear cache before enter, so subsequent requests dont hit stale cache while this one hangs
			domainSidCacheMu.Lock()
			delete(domainSidCache, c.UserID+":"+code)
			domainSidCacheMu.Unlock()

			der, dErr := c.RequestEnterDomainServer(sid)
			if dErr != nil || der.Code != 0 {
				uid, aid := getAuditUser()
				db.AddAuditLog(&uid, &aid, "join_server_failed", sc, "进入山头服失败", requestIP(r))
				if dErr != nil {
					log.Printf("[DOMAIN] enter failed: sid=%s err=%v", sid, dErr)
					domainSidCacheMu.Lock()
					delete(domainSidCache, c.UserID+":"+code)
					domainSidCacheMu.Unlock()
					log.Printf("[DOMAIN] cleared domainSidCache for %s:%s", c.UserID, code)
					jsonW(w, r, LoginResp{Success: false, Message: fmt.Sprintf("进入山头服失败: %v", dErr)})
					return
				}
				log.Printf("[DOMAIN] enter failed: sid=%s code=%d msg=%q details=%q", sid, der.Code, der.Message, der.Details)
				domainSidCacheMu.Lock()
				delete(domainSidCache, c.UserID+":"+code)
				domainSidCacheMu.Unlock()
				log.Printf("[DOMAIN] cleared domainSidCache for %s:%s", c.UserID, code)
				errMsg := fmt.Sprintf("进入山头服失败 code=%d", der.Code)
				if der.Code == 1005028 || strings.Contains(der.Message, "静默拉起") {
					// 冷启动中的可重试临时状态，不是故障
					errMsg = "山头服正在启动中，请稍后重试"
				} else {
					if der.Message != "" {
						errMsg += " msg=" + der.Message
					}
					if der.Details != "" {
						errMsg += " details=" + der.Details
					}
				}
				jsonW(w, r, LoginResp{Success: false, Message: errMsg})
				return
			}
			ip = fmt.Sprintf("%s:%d", der.Entity.ServerHost, der.Entity.ServerPort.Int64())
			var av2Err error
			av2, av2Err = c.GenerateDomainGameAuthV2(sid, req.ClientPublicKey)
			if av2Err != nil || len(av2) == 0 {
				uid, aid := getAuditUser()
				db.AddAuditLog(&uid, &aid, "join_server_failed", sc, "生成AuthV2失败", requestIP(r))
				jsonW(w, r, LoginResp{Success: false, Message: fmt.Sprintf("认证失败: %v", av2Err)})
				return
			}
			// 域名操作后清除缓存，避免状态污染后续请求
			heartbeatStopMu.Lock()
			for k, stop := range heartbeatStops {
				stop()
				delete(heartbeatStops, k)
			}
			heartbeatStopMu.Unlock()
			g79CacheMu.Lock()
			for k := range g79Cache {
				delete(g79Cache, k)
			}
			g79CacheMu.Unlock()
		}

		// Link connection: temporarily disabled for debugging
		// Link connection: disabled for debugging
		if false {
			linkSvc, linkErr := link_connection.NewLinkConnectionService(c)
			if linkErr == nil {
				conn, dialErr := linkSvc.Dial(context.Background())
				if dialErr == nil {
					gameInfo, _ := json.Marshal(map[string]interface{}{
						"min_level": 0,
						"room_name": sc, "gameType": "RentalGame", "res_name": sc,
						"ownerName": "", "ownerId": "", "id": sid,
					})
					_ = conn.SendGameStart(map[string]interface{}{
						"game_info": string(gameInfo), "strict_mode": true,
						"game_type": 10, "is_free_play": false, "game_id": sid,
						"play_iids": []string{},
					})
					conn.Conn().Close()
				}
			}
		}
		ci, err := c.SendAuthV2Request(av2)
		if err != nil {
			// chainInfo 生成失败（如 payload too short）时重试一次；guest 模式还切换匿名账号
			log.Printf("[CHAIN] chainInfo失败: %v, 重试一次", err)
			if guestAcc != nil {
				// guest 模式：切换匿名共享账号并重新认证
				guestAcc = forceRotateGuestAccount()
				if guestAcc != nil {
					cc, ccErr := newG79ClientWithProxy(guestAcc.ID)
					if ccErr == nil {
						if ccErr := cc.G79AuthenticateWithCookie(guestAcc.CookieData); ccErr == nil {
							c = cc
							g79CacheMu.Lock()
							g79Cache[guestAcc.ID] = &cachedClient{client: c, expires: time.Now().Add(25 * time.Minute)}
							g79CacheMu.Unlock()
							startAccountHeartbeat(guestAcc.ID, c)
							// 重新生成 av2 并重试
							if av2, err2 := c.GenerateRentalGameAuthV2(sid, req.ClientPublicKey); err2 == nil {
								ci, err = c.SendAuthV2Request(av2)
							}
						}
					}
				}
			} else {
				// 非 guest 模式：直接重试一次
				ci, err = c.SendAuthV2Request(av2)
			}
			if err != nil {
				uid, aid := getAuditUser()
				db.AddAuditLog(&uid, &aid, "join_server_failed", sc, "chainInfo失败", requestIP(r))
				jsonW(w, r, LoginResp{Success: false, Message: fmt.Sprintf("chainInfo: %v", err)})
				db.AddSystemLog("error", "登录失败: chainInfo生成失败 "+sc, fmt.Sprintf("%v", err))
				return
			}
		}
		ciStr := strings.TrimSpace(string(ci))

		h := md5.Sum([]byte(ciStr))
		vfy := hex.EncodeToString(h[:])
		sh := sha512.Sum512([]byte(ciStr))
		newVfy := hex.EncodeToString(sh[:]) + hex.EncodeToString(sh[:]) + hex.EncodeToString(sh[:]) + hex.EncodeToString(sh[:])
		log.Printf("OK: %s uid=%s", c.UserDetail.Name, c.UserID)
		// Record join_server audit log — try adb// first, then session, then find by G79 UID
		var auditUserID, auditAcctID int64
		var isGuestAudit bool
		if strings.HasPrefix(req.FBToken, "adb//") {
			if u, apiTok, err := db.ResolveToken(req.FBToken); err == nil && u != nil {
				if aid := db.ResolveActiveAccountID(u, apiTok); aid != nil {
					auditUserID = u.ID
					auditAcctID = *aid
				}
			}
		}
		if auditUserID == 0 {
			if bearer := getBearer(r); bearer != "" {
				sessMu.RLock()
				if s, ok := sessions[bearer]; ok && s.WebUserID != 0 {
					auditUserID = s.WebUserID
					if acc, err := db.LookupAccountByUID(c.UserID); err == nil && acc != nil {
						auditAcctID = acc.ID
					}
				}
				sessMu.RUnlock()
			}
		}
		if auditUserID == 0 {
			if acc, err := db.LookupAccountByUID(c.UserID); err == nil && acc != nil && acc.OwnerID != nil {
				auditUserID = *acc.OwnerID
				auditAcctID = acc.ID
			}
		}
		// Guest mode: use sentinel ID 0
		if auditUserID == 0 && guestAcc != nil {
			auditUserID = auth.GuestUserID
			auditAcctID = guestAcc.ID
			isGuestAudit = true
		}
		if auditUserID != 0 || isGuestAudit {
			joinDetail := fmt.Sprintf("%s 使用 %s 进入服务器 %s", c.UserDetail.Name, c.UserID, sc)
			if tid, tname := currentAPIToken(r); tid != 0 {
				if tname != "" {
					joinDetail += fmt.Sprintf("（令牌#%d(%s)）", tid, tname)
				} else {
					joinDetail += fmt.Sprintf("（令牌#%d）", tid)
				}
			}
			// 板栗扣除：10分钟窗口机制（join_server 日志在扣费后再写，
			// 否则本次进服会被当成"最近一次进服"，导致永远判定为重复进服而免费）
			isShared := false
			var createdBy *int64
			if auditAcctID != 0 {
				if acc, err := db.GetAccountByID(auditAcctID); err == nil && acc != nil {
					isShared = acc.OwnerID == nil
					createdBy = acc.CreatedBy
				}
			}
			subActive, _, _ := db.IsSubscriptionActive(auditUserID)
			if isGuestAudit {
				// Guest users: no cost check, skip to reward
			} else if subActive {
				// 订阅期内进服免费：跳过扣费/余额/次令牌累计；下方 shared_used/游客奖励一并跳过
			} else if enableJoinCost {
				cost := 1
				if isShared {
					cost = 2
				}
				// 10分钟内重复进同一服务器：自己免费，共享扣1
				cutoff := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
				curTokID, _ := currentAPIToken(r)
				if recentSame, _ := db.HasRecentJoin(auditUserID, auditAcctID, curTokID, sc, cutoff); recentSame {
					if recentOther, _ := db.HasRecentOtherJoin(auditUserID, auditAcctID, curTokID, sc, cutoff); !recentOther {
						cost = 0
						if isShared {
							cost = 1
						}
					}
				}
				balance, err := db.GetNutsBalance(auditUserID)
				if err == nil && balance < cost {
					db.AddAuditLog(&auditUserID, &auditAcctID, "join_server_failed", sc, fmt.Sprintf("板栗不足: %d<%d", balance, cost), requestIP(r))
					jsonW(w, r, LoginResp{Success: false, Message: fmt.Sprintf("板栗不足（当前 %d 个，需要 %d 个）", balance, cost)})
					db.AddSystemLog("warn", "登录失败: 板栗不足 "+sc, fmt.Sprintf("user=%d balance=%d need=%d", auditUserID, balance, cost))
					return
				}
				// 次令牌：累计消耗积分上限检查
				if tid, _ := currentAPIToken(r); tid != 0 {
					if err := db.IncTokenNuts(tid, cost); err != nil {
						jsonW(w, r, LoginResp{Success: false, Message: sysErr(err)})
						return
					}
				}
				if _, err := db.AddNuts(auditUserID, -cost, "join_server", nil, &auditAcctID); err != nil {
					jsonW(w, r, LoginResp{Success: false, Message: fmt.Sprintf("板栗扣除失败: %v", err)})
					return
				}
			}
			// 共享账号被他人使用：奖励创建者（订阅用户进服免费，不产生奖励）
			if isShared && createdBy != nil && *createdBy != auditUserID && !subActive {
				db.AddNuts(*createdBy, 1, "shared_used", &auditUserID, &auditAcctID)
			}
			// 匿名游客使用共享账号：奖励创建者 2 板栗
			if isGuestAudit && guestAcc != nil && guestAcc.CreatedBy != nil && !subActive {
				db.AddNuts(*guestAcc.CreatedBy, 2, "guest_used", &auditUserID, &auditAcctID)
			}
			// 扣费完成后再记录 join_server 审计日志，
			// 避免本次进服被 10 分钟窗口当成"最近一次进服"而永远免费
			joinTokID, _ := currentAPIToken(r)
			_ = db.AddAuditLog(&auditUserID, &auditAcctID, "join_server", sc, joinDetail, requestIP(r), joinTokID)
		}

		// Store session data for Bearer token
		if bearer := getBearer(r); bearer != "" {
			log.Printf("[LOGIN] bearer found, len=%d", len(bearer))
			sessMu.Lock()
			if s, ok := sessions[bearer]; ok {
				log.Printf("[LOGIN] existing session found, storing LoginToken")
				s.UserID = c.UserID
				s.EngineVersion = c.EngineVersion
				s.PatchVersion = c.G79LatestVersion
				if strings.HasPrefix(req.FBToken, "adb//") {
					s.LoginToken = req.FBToken
					if u, err := db.GetUserByToken(req.FBToken); err == nil && u != nil {
						s.WebUserID = u.ID
					}
				}
			} else if strings.HasPrefix(req.FBToken, "adb//") {
				var webUID int64
				if u, err := db.GetUserByToken(req.FBToken); err == nil && u != nil {
					webUID = u.ID
				}
				log.Printf("[LOGIN] no session, auto-creating with LoginToken")
				sessions[bearer] = &sessionData{
					UserID:        c.UserID,
					WebUserID:     webUID,
					EngineVersion: c.EngineVersion,
					PatchVersion:  c.G79LatestVersion,
					LoginToken:    req.FBToken,
					CreatedAt:     time.Now(),
				}
			}
			sessMu.Unlock()
		} else {
			log.Printf("[LOGIN] no bearer token in request")
		}
		var skinInfo = map[string]any{}
		var outfitInfo = map[string]any{}
		if settings, err := c.GetUserSettingList(); err == nil && settings != nil {
			itemID := settings.Entity.SkinData.ItemID
			if itemID != "" && itemID != "-1" {
				if di, diErr := c.GetDownloadInfo(itemID); diErr == nil && di != nil {
					skinInfo = map[string]any{
						"entity_id": itemID,
						"res_url":   di.Entity.ResURL,
						"is_slim":   true,
					}
				} else {
					log.Printf("[SKIN] GetDownloadInfo 失败 uid=%s itemID=%s err=%v diNil=%v", c.UserID, itemID, diErr, di == nil)
				}
			} else {
				log.Printf("[SKIN] 账号无皮肤 itemID=%q uid=%s", itemID, c.UserID)
			}
		} else {
			log.Printf("[SKIN] GetUserSettingList 失败 uid=%s err=%v nil=%v", c.UserID, err, settings == nil)
		}
		// compute effective growth level, then apply user override
		growLv := 0
		if c.UserDetail != nil {
			growLv = int(c.UserDetail.Level.Int64())
			if od, oerr := c.GetOtherUserDetail(c.UserID, true); oerr == nil && od != nil {
				if peLv := int(od.Entity.PEGrowth.Lv.Int64()); peLv > growLv {
					growLv = peLv
				}
			}
		}
		if strings.HasPrefix(req.FBToken, "adb//") {
			if u, ue := db.GetUserByToken(req.FBToken); ue == nil && u != nil && u.GrowthOverride {
				growLv = u.GrowthOverrideValue
			}
		} else if bearer := getBearer(r); bearer != "" {
			sessMu.RLock()
			var webUID int64
			if s, ok := sessions[bearer]; ok {
				webUID = s.WebUserID
			}
			sessMu.RUnlock()
			if webUID > 0 {
				if u, ue := db.GetUserByID(webUID); ue == nil && u.GrowthOverride {
					growLv = u.GrowthOverrideValue
				}
			}
		}
		success = true
		jsonW(w, r, LoginResp{
			Success: true, Message: "ok", ServerMsg: "", Token: req.FBToken, RespondTo: c.UserDetail.Name, IPAddress: ip,
			UID: c.UserID, Username: c.UserDetail.Name,
			GrowthLevel: growLv,
			SkinInfo:    skinInfo, OutfitInfo: outfitInfo,
			ChainInfo: ciStr, Verify: vfy, NewVerify: newVfy,
		})
	})

	// ── 网易 MPay 账号端点 ──

	guestFlowMu := sync.Mutex{}
	guestFlows := map[string]*guestVerifyFlow{}

	newGuestFlow := func(device *mpay.Device, ticket, verifyURL string) string {
		flowID := fmt.Sprintf("guest_%x", rand.Uint64())
		guestFlowMu.Lock()
		guestFlows[flowID] = &guestVerifyFlow{Device: device, Ticket: ticket, VerifyURL: verifyURL, CreatedAt: time.Now()}
		guestFlowMu.Unlock()
		return flowID
	}

	getGuestFlow := func(flowID string) *guestVerifyFlow {
		guestFlowMu.Lock()
		defer guestFlowMu.Unlock()
		f := guestFlows[flowID]
		if f == nil {
			return nil
		}
		if time.Since(f.CreatedAt) > 15*time.Minute {
			delete(guestFlows, flowID)
			return nil
		}
		return f
	}

	http.HandleFunc("/api/mpay/guest", func(w http.ResponseWriter, r *http.Request) {
		if !guestCreateLimiter.Allow(requestIP(r)) {
			jsonW(w, r, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
			return
		}
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		log.Println("[MPAY] 开始创建游客账号...")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		device, err := mpay.GenerateDeviceWithHTTPClient(ctx, PickOneTimeProxy())
		if err != nil {
			jsonW(w, r, M{"ok": false, "error": fmt.Sprintf("生成设备失败: %v", err)})
			return
		}
		user, guestErr := device.Guest(ctx)
		if guestErr == nil {
			cookie, _ := user.CookieString()
			uid := user.Sauth.SDKUID
			acc := &Account{
				Username:    fmt.Sprintf("guest_%s", uid[:8]),
				DisplayName: fmt.Sprintf("游客_%s", uid[:6]),
				UID:         uid, Cookie: cookie,
				Token:   user.Sauth.SessionID,
				IsGuest: true, DeviceID: user.Sauth.DeviceID,
				CreatedAt: time.Now().Unix(),
			}
			accountMu.Lock()
			accounts[acc.Username] = acc
			accountMu.Unlock()
			saveAccounts()
			sc := addStoredCookieFromLogin(cookie, "guest", acc.DisplayName, true)
			saveMPayAccountForRequest(r, cookie, "guest", acc.DisplayName, uid)
			log.Printf("[MPAY] 游客账号创建成功: uid=%s", uid)
			jsonW(w, r, M{
				"ok": true, "message": "游客账号创建成功，已加入账号列表并设为当前账号",
				"username": acc.Username, "display_name": acc.DisplayName,
				"uid": uid, "cookie": cookie, "cookie_id": sc.ID,
			})
			return
		}
		var verifyErr *mpay.NeedVerifyError
		if errors.As(guestErr, &verifyErr) {
			log.Printf("[MPAY] 需要验证: %s (code=%d)", verifyErr.Reason, verifyErr.Code)
			ticket := extractTicketFromURL(verifyErr.VerifyURL)
			flowID := newGuestFlow(device, ticket, verifyErr.VerifyURL)
			phoneHint := ""
			smsCode, smsPhone, smsPhoneBak := extractSmsInfo(verifyErr.VerifyURL)
			if verifyErr.VerifyURL != "" {
				if info, fErr := fetchVerifyPageInfo(verifyErr.VerifyURL); fErr == nil {
					phoneHint = info
				}
			}
			jsonW(w, r, M{
				"ok": false, "need_verify": true, "flow_id": flowID,
				"verify_url": verifyErr.VerifyURL,
				"ticket":     ticket,
				"phone_hint": phoneHint,
				"code":       smsCode,
				"phone":      smsPhone,
				"phone_bak":  smsPhoneBak,
				"message":    "需要安全验证。请使用任意手机号发送验证码后点击已完成",
			})
			return
		}
		jsonW(w, r, M{"ok": false, "error": fmt.Sprintf("游客创建失败: %v", guestErr)})
	})

	http.HandleFunc("/api/mpay/guest/send-verify-sms", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			return
		}
		if !guestSmsLimiter.Allow(requestIP(r)) {
			jsonW(w, r, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
			return
		}
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var body struct {
			FlowID string `json:"flow_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.FlowID == "" {
			jsonW(w, r, M{"ok": false, "error": "缺少 flow_id"})
			return
		}
		flow := getGuestFlow(body.FlowID)
		if flow == nil {
			jsonW(w, r, M{"ok": false, "error": "flow 已过期或无效，请重新创建游客账号"})
			return
		}
		log.Println("[MPAY-GUEST] 尝试获取验证手机号提示...")
		phoneHint := "请打开验证页面查看接收验证码的手机号"
		if flow.VerifyURL != "" {
			if info, err := fetchVerifyPageInfo(flow.VerifyURL); err == nil {
				phoneHint = info
			}
		}
		jsonW(w, r, M{"ok": true, "phone_hint": phoneHint, "message": "请在浏览器中打开验证页面完成短信验证"})
	})

	// 短信验证"已完成"步骤：调用 upload_sms/result 用 ticket 完成设备验证。
	http.HandleFunc("/api/mpay/guest/finish-sms", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			return
		}
		if !guestSmsLimiter.Allow(requestIP(r)) {
			jsonW(w, r, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
			return
		}
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var body struct {
			FlowID string `json:"flow_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.FlowID == "" {
			jsonW(w, r, M{"ok": false, "error": "缺少 flow_id"})
			return
		}
		flow := getGuestFlow(body.FlowID)
		if flow == nil {
			jsonW(w, r, M{"ok": false, "error": "flow 已过期或无效，请重新创建游客账号"})
			return
		}
		if flow.Verified {
			jsonW(w, r, M{"ok": true, "message": "设备已验证"})
			return
		}
		if flow.Ticket == "" {
			jsonW(w, r, M{"ok": false, "error": "缺少 ticket，请重新创建游客账号"})
			return
		}
		log.Println("[MPAY-GUEST] 完成短信验证(finish-sms)...")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		token, userID, err := mpay.FinishSMS(ctx, PickOneTimeProxy(), flow.Ticket, mpay.FixedAppVersionCode)
		cancel()
		if err != nil {
			log.Printf("[MPAY-GUEST] 短信验证失败: %v", err)
			jsonW(w, r, M{"ok": false, "error": "手机号被限制或没有发送短信"})
			return
		}
		flow.Verified = true
		// 验证阶段已消耗 ticket 创建了首个账号：先入待实名(实名由用户输入/库存随机抽取)
		acc := createGuestAccount(r, flow.Device, token, userID, "guest_verify")
		flow.PendingRealname = append(flow.PendingRealname, acc)
		log.Printf("[MPAY-GUEST] 短信验证完成，首个账号 uid=%s (待实名=%d): flow=%s",
			userID, len(flow.PendingRealname), body.FlowID)
		jsonW(w, r, M{
			"ok": true, "message": "验证完成，首个账号已处理，可开始批量注册",
			"first_account": map[string]string{"uid": acc.UID, "cookie": acc.Cookie, "nickname": acc.Nickname},
			"to_realname": len(flow.PendingRealname),
		})
	})

	http.HandleFunc("/api/mpay/guest/batch", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			return
		}
		if !guestContinueLimiter.Allow(requestIP(r)) {
			jsonW(w, r, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
			return
		}
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var body struct {
			FlowID   string `json:"flow_id"`
			RealName string `json:"real_name"`
			IDNum    string `json:"id_num"`
			MaxBatch int    `json:"max_batch"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.FlowID == "" {
			jsonW(w, r, M{"ok": false, "error": "缺少 flow_id"})
			return
		}
		flow := getGuestFlow(body.FlowID)
		if flow == nil {
			jsonW(w, r, M{"ok": false, "error": "flow 已过期或无效，请重新创建游客账号"})
			return
		}
		if !flow.Verified {
			jsonW(w, r, M{"ok": false, "error": "请先完成短信验证后再开始批量注册"})
			return
		}
		log.Printf("[MPAY-GUEST] 批量注册")

		maxBatch := body.MaxBatch
		if maxBatch <= 0 {
			maxBatch = 500
		}

		// 处理单个账号注册：先建号，全部进待实名(实名由用户输入/库存随机抽取)
		processAccount := func(token, sdkuid string) {
			acc := createGuestAccount(r, flow.Device, token, sdkuid, "guest_batch")
			flow.PendingRealname = append(flow.PendingRealname, acc)
		}

		// 用 flow.Device.Guest() 循环创建账号（设备已验证，不再需要 SMS）
		for i := 0; i < maxBatch; i++ {
			loopCtx, loopCancel := context.WithTimeout(context.Background(), 20*time.Second)
			user, gErr := flow.Device.Guest(loopCtx)
			loopCancel()
			if gErr != nil {
				var verifyErr *mpay.NeedVerifyError
				if errors.As(gErr, &verifyErr) {
					log.Printf("[MPAY-GUEST] 批量: 第 %d 个需要验证，停止(code=%d)", i, verifyErr.Code)
				} else {
					log.Printf("[MPAY-GUEST] 批量: 第 %d 个失败: %v", i, gErr)
				}
				break
			}
			processAccount(user.Sauth.SessionID, user.Sauth.SDKUID)
			time.Sleep(800 * time.Millisecond)
		}

		// 随机抽一个库存身份证尝试实名；剩余待用户输入
		flow.PendingRealname = tryStockRealname(flow, flow.PendingRealname)

		toRealname := len(flow.PendingRealname)
		total := toRealname + len(flow.PendingRename)
		if total == 0 {
			log.Printf("[MPAY-GUEST] 批量注册失败：未完成短信验证或设备不可用")
			guestFlowMu.Lock()
			delete(guestFlows, body.FlowID)
			guestFlowMu.Unlock()
			jsonW(w, r, M{"ok": false, "error": "设备尚未完成短信验证或无法批量创建，请重新创建游客账号"})
			return
		}

		// 有待实名或待改名账号时保留 flow 供后续使用，否则清理
		if toRealname == 0 && len(flow.PendingRename) == 0 {
			guestFlowMu.Lock()
			delete(guestFlows, body.FlowID)
			guestFlowMu.Unlock()
		}

		log.Printf("[MPAY-GUEST] 批量注册完成: 待实名 %d 个, 待改名 %d 个", toRealname, len(flow.PendingRename))
		jsonW(w, r, M{
			"ok":          true,
			"stage":       "batch_done",
			"to_realname": toRealname,
			"to_rename":   len(flow.PendingRename),
			"total":       total,
			"message":     fmt.Sprintf("完成：待实名 %d 个，待改名 %d 个。", toRealname, len(flow.PendingRename)),
		})
	})

	// 用户自输身份证完成待实名账号的实名认证（库存用尽或预设失败后的兜底）。
	http.HandleFunc("/api/mpay/guest/realname", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			return
		}
		if !guestRealnameLimiter.Allow(requestIP(r)) {
			jsonW(w, r, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
			return
		}
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var body struct {
			FlowID string `json:"flow_id"`
			Name   string `json:"name"`
			IDNum  string `json:"id_num"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.FlowID == "" {
			jsonW(w, r, M{"ok": false, "error": "缺少 flow_id"})
			return
		}
		if body.Name == "" || body.IDNum == "" {
			jsonW(w, r, M{"ok": false, "error": "请填写姓名和身份证号"})
			return
		}
		flow := getGuestFlow(body.FlowID)
		if flow == nil {
			jsonW(w, r, M{"ok": false, "error": "flow 已过期或无效，请重新创建游客账号"})
			return
		}
		if len(flow.PendingRealname) == 0 {
			jsonW(w, r, M{"ok": false, "error": "没有待实名的账号"})
			return
		}
		idMask := body.IDNum
		if len(idMask) > 4 {
			idMask = body.IDNum[:4]
		}
		log.Printf("[MPAY-GUEST] 用户自输实名: %s %s****, 待实名 %d 个", body.Name, idMask, len(flow.PendingRealname))
		// 逐个实名；首个失败则整体失败，让用户换身份证重新输入
		for _, acc := range flow.PendingRealname {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			err := mpay.AuthRealNameFixed(ctx, PickOneTimeProxy(), flow.Device.ID, acc.UID, acc.Token, body.Name, body.IDNum)
			cancel()
			if err != nil {
				log.Printf("[MPAY-GUEST] 自输实名失败 uid=%s: %v", acc.UID, err)
				jsonW(w, r, M{"ok": false, "error": "该身份证实名验证失败，请换一个"})
				return
			}
		}
		// 全部成功：身份证入库补充库存(去重)
		dup := false
		if all, err := db.GetAllRealnamePresets(); err == nil {
			for _, p := range all {
				if p.IDNumber == body.IDNum {
					dup = true
					break
				}
			}
		}
		if !dup {
			if _, err := db.AddRealnamePreset(body.Name, body.IDNum, true); err != nil {
				log.Printf("[MPAY-GUEST] 身份证入库失败: %v", err)
			} else {
				log.Printf("[MPAY-GUEST] 身份证已入库: %s", body.Name)
			}
		}
		// 路由：前2个待改名，超出入未发送
		unsent := routeRealnamed(flow, flow.PendingRealname)
		if len(unsent) > 0 {
			writeGuestFile(guestUnsentDir, unsent)
		}
		guestFlowMu.Lock()
		n := len(flow.PendingRealname)
		flow.PendingRealname = nil
		if len(flow.PendingRename) == 0 {
			delete(guestFlows, body.FlowID)
		}
		guestFlowMu.Unlock()
		log.Printf("[MPAY-GUEST] 用户实名完成 %d 个, 身份证入库 %s, 待改名 %d 个", n, body.Name, len(flow.PendingRename))
		jsonW(w, r, M{
			"ok": true, "count": n,
			"to_rename":   len(flow.PendingRename),
			"to_realname": 0,
			"message":     fmt.Sprintf("实名完成 %d 个账号。%s", n, map[bool]string{true: "还有待改名的账号。", false: "全部完成。"}[len(flow.PendingRename) > 0]),
		})
	})

	// 用户放弃实名：未实名账号移入未实名文件夹。
	http.HandleFunc("/api/mpay/guest/realname-abandon", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			return
		}
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": "POST only"})
			return
		}
		var body struct {
			FlowID string `json:"flow_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.FlowID == "" {
			jsonW(w, r, M{"ok": false, "error": "缺少 flow_id"})
			return
		}
		flow := getGuestFlow(body.FlowID)
		if flow != nil {
			var lines []string
			for _, acc := range flow.PendingRealname {
				lines = append(lines, acc.Cookie)
			}
			writeGuestFile(guestUnverifiedDir, lines)
			guestFlowMu.Lock()
			delete(guestFlows, body.FlowID)
			guestFlowMu.Unlock()
			log.Printf("[MPAY-GUEST] 放弃实名，%d 个账号已入未实名", len(lines))
		}
		jsonW(w, r, M{"ok": true, "message": "已放弃，账号已存入未实名"})
	})

	// 逐个给待改名账号改名：成功才入库(发放)，失败返回原因供前端重试/换名。
	http.HandleFunc("/api/mpay/guest/rename", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			return
		}
		if !guestNicknameLimiter.Allow(requestIP(r)) {
			jsonW(w, r, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
			return
		}
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var body struct {
			FlowID string `json:"flow_id"`
			Name   string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.FlowID == "" {
			jsonW(w, r, M{"ok": false, "error": "缺少 flow_id"})
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		if body.Name == "" {
			jsonW(w, r, M{"ok": false, "error": "请输入昵称"})
			return
		}
		flow := getGuestFlow(body.FlowID)
		if flow == nil {
			jsonW(w, r, M{"ok": false, "error": "flow 已过期或无效，请重新创建游客账号"})
			return
		}
		if len(flow.PendingRename) == 0 {
			jsonW(w, r, M{"ok": false, "error": "没有待改名的账号"})
			return
		}
		acc := flow.PendingRename[0]
		log.Printf("[MPAY-GUEST] 改名 uid=%s -> %s", acc.UID, body.Name)
		// 复用已验证可用的 g79client 改名链路：认证 cookie → UpdateNickname(内部用动态 token)
		gc, gErr := g79.NewClientWithHTTPClient(PickOneTimeProxy())
		if gErr != nil {
			jsonW(w, r, M{"ok": false, "error": "创建客户端失败"})
			return
		}
		if err := gc.G79AuthenticateWithCookie(acc.Cookie); err != nil {
			log.Printf("[MPAY-GUEST] 改名认证失败 uid=%s: %v", acc.UID, err)
			jsonW(w, r, M{"ok": false, "error": "请先登录"})
			return
		}
		if err := gc.UpdateNickname(body.Name); err != nil {
			errMsg := err.Error()
			log.Printf("[MPAY-GUEST] 改名失败 uid=%s: %v", acc.UID, errMsg)
			if strings.Contains(errMsg, "占用") || strings.Contains(errMsg, "已存在") {
				jsonW(w, r, M{"ok": false, "error": "该昵称已被占用"})
			} else if strings.Contains(errMsg, "合规") || strings.Contains(errMsg, "敏感") || strings.Contains(errMsg, "非法") {
				jsonW(w, r, M{"ok": false, "error": "昵称不合规，请修改"})
			} else {
				jsonW(w, r, M{"ok": false, "error": "修改失败: " + errMsg})
			}
			return
		}
		// 改名成功 → 入库(发放)
		flow.GivenCount++
		saveMPayAccountForRequest(r, acc.Cookie, "guest_rename", body.Name, acc.UID)
		guestFlowMu.Lock()
		flow.PendingRename = flow.PendingRename[1:]
		remaining := len(flow.PendingRename)
		if remaining == 0 {
			delete(guestFlows, body.FlowID)
		}
		guestFlowMu.Unlock()
		log.Printf("[MPAY-GUEST] 改名成功并发放 uid=%s (%s), 剩余待改名 %d", acc.UID, body.Name, remaining)
		jsonW(w, r, M{
			"ok": true, "renamed": 1, "remaining": remaining, "given": flow.GivenCount,
			"message": fmt.Sprintf("改名成功并发放：%s。%s", body.Name, map[bool]string{true: "全部改名完成。", false: fmt.Sprintf("还有 %d 个待改名。", remaining)}[remaining == 0]),
		})
	})

	// 用户放弃改名或超时：把待改名账号移入未发送文件夹。
	http.HandleFunc("/api/mpay/guest/rename-abandon", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			return
		}
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": "POST only"})
			return
		}
		var body struct {
			FlowID string `json:"flow_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.FlowID == "" {
			jsonW(w, r, M{"ok": false, "error": "缺少 flow_id"})
			return
		}
		flow := getGuestFlow(body.FlowID)
		if flow != nil {
			var lines []string
			for _, acc := range flow.PendingRename {
				lines = append(lines, acc.Cookie)
			}
			writeGuestFile(guestUnsentDir, lines)
			guestFlowMu.Lock()
			delete(guestFlows, body.FlowID)
			guestFlowMu.Unlock()
			log.Printf("[MPAY-GUEST] 放弃改名，%d 个待改名账号已入未发送", len(lines))
		}
		jsonW(w, r, M{"ok": true, "message": "已放弃，账号已存入未发送"})
	})
	http.HandleFunc("/api/mpay/login", func(w http.ResponseWriter, r *http.Request) {
		if !mpayLoginLimiter.Allow(requestIP(r)) {
			jsonW(w, r, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
			return
		}
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Email == "" || body.Password == "" {
			jsonW(w, r, M{"ok": false, "error": "请提供 email 和 password"})
			return
		}
		log.Printf("[MPAY] 邮箱登录: %s", body.Email)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		device, err := mpay.GenerateDeviceWithHTTPClient(ctx, PickOneTimeProxy())
		if err != nil {
			jsonW(w, r, M{"ok": false, "error": fmt.Sprintf("生成设备失败: %v", err)})
			return
		}
		var user *mpay.User
		for {
			ctx2, cancel2 := context.WithTimeout(context.Background(), 15*time.Second)
			user, err = device.LoginEmail(ctx2, body.Email, body.Password)
			cancel2()
			if err == nil {
				break
			}
			var verifyErr *mpay.NeedVerifyError
			if !errors.As(err, &verifyErr) {
				jsonW(w, r, M{"ok": false, "error": fmt.Sprintf("登录失败: %v", err)})
				return
			}
			if verifyErr.Code == 1301 || strings.TrimSpace(verifyErr.VerifyURL) == "" {
				jsonW(w, r, M{"ok": false, "error": verifyErr.Reason})
				return
			}
			log.Printf("[MPAY] 需要验证: %s (code=%d)", verifyErr.Reason, verifyErr.Code)
			jsonW(w, r, M{
				"ok": false, "need_verify": true,
				"code": verifyErr.Code, "reason": verifyErr.Reason,
				"verify_url": verifyErr.VerifyURL,
				"message":    "需要安全验证，请打开 verify_url 完成验证后重试",
			})
			return
		}
		cookie, _ := user.CookieString()
		uid := user.Sauth.SDKUID
		acc := &Account{
			Username: body.Email, DisplayName: body.Email,
			UID: uid, Cookie: cookie,
			Token:   user.Sauth.SessionID,
			IsGuest: false, DeviceID: user.Sauth.DeviceID,
			CreatedAt: time.Now().Unix(),
		}
		accountMu.Lock()
		accounts[body.Email] = acc
		accountMu.Unlock()
		saveAccounts()
		sc := addStoredCookieFromLogin(cookie, "email", body.Email, true)
		saveMPayAccountForRequest(r, cookie, "email", body.Email, uid)
		_ = db.AddAuditLog(nil, nil, "mpay_email_login", "game_account", fmt.Sprintf("邮箱登录: %s uid=%s", body.Email, uid), requestIP(r))
		log.Printf("[MPAY] 登录成功: email=%s uid=%s", body.Email, uid)
		jsonW(w, r, M{
			"ok": true, "message": "登录成功，已加入账号列表并设为当前账号",
			"username": body.Email, "display_name": body.Email,
			"uid": uid, "cookie": cookie, "cookie_id": sc.ID,
		})
	})

	// ── 手机号登录 ──
	phoneDevMu := sync.Mutex{}
	phoneDevices := map[string]*mpay.Device{} // phone number → device

	http.HandleFunc("/api/mpay/phone/sms", func(w http.ResponseWriter, r *http.Request) {
		if !phoneSmsLimiter.Allow(requestIP(r)) {
			jsonW(w, r, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
			return
		}
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var body struct {
			Phone string `json:"phone"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Phone == "" {
			jsonW(w, r, M{"ok": false, "error": "请提供手机号"})
			return
		}
		log.Printf("[MPAY-SMS] 发送验证码到 %s", body.Phone)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		device, err := mpay.GenerateDeviceWithHTTPClient(ctx, PickOneTimeProxy())
		if err != nil {
			jsonW(w, r, M{"ok": false, "error": fmt.Sprintf("生成设备失败: %v", err)})
			return
		}
		phoneDevMu.Lock()
		phoneDevices[body.Phone] = device
		phoneDevMu.Unlock()
		err = mpaySendSMS(device, body.Phone)
		if err != nil {
			var verifyErr *mpay.NeedVerifyError
			if errors.As(err, &verifyErr) {
				jsonW(w, r, M{"ok": false, "need_verify": true, "code": verifyErr.Code, "reason": verifyErr.Reason, "verify_url": verifyErr.VerifyURL, "message": "需要安全验证"})
				return
			}
			jsonW(w, r, M{"ok": false, "error": sysErr(err)})
			return
		}
		log.Printf("[MPAY-SMS] 验证码已发送到 %s", body.Phone)
		jsonW(w, r, M{"ok": true, "message": "验证码已发送"})
	})

	http.HandleFunc("/api/mpay/phone/verify", func(w http.ResponseWriter, r *http.Request) {
		if !phoneVerifyLimiter.Allow(requestIP(r)) {
			jsonW(w, r, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
			return
		}
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var body struct {
			Phone string `json:"phone"`
			Code  string `json:"code"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Phone == "" || body.Code == "" {
			jsonW(w, r, M{"ok": false, "error": "请提供手机号和验证码"})
			return
		}
		phoneDevMu.Lock()
		device, ok := phoneDevices[body.Phone]
		phoneDevMu.Unlock()
		if !ok {
			jsonW(w, r, M{"ok": false, "error": "请先发送验证码"})
			return
		}
		log.Printf("[MPAY-VERIFY] 验证手机号 %s", body.Phone)
		ticket, err := mpayVerifySMS(device, body.Phone, body.Code)
		if err != nil {
			var verifyErr *mpay.NeedVerifyError
			if errors.As(err, &verifyErr) {
				jsonW(w, r, M{"ok": false, "need_verify": true, "code": verifyErr.Code, "reason": verifyErr.Reason, "verify_url": verifyErr.VerifyURL, "message": "需要安全验证"})
				return
			}
			jsonW(w, r, M{"ok": false, "error": sysErr(err)})
			return
		}
		// Use ticket to complete phone login
		cookie, uid, token, displayName, err := mpayPhoneLogin(device, body.Phone, ticket)
		if err != nil {
			var verifyErr *mpay.NeedVerifyError
			if errors.As(err, &verifyErr) {
				jsonW(w, r, M{"ok": false, "need_verify": true, "code": verifyErr.Code, "reason": verifyErr.Reason, "verify_url": verifyErr.VerifyURL, "message": "需要安全验证"})
				return
			}
			jsonW(w, r, M{"ok": false, "error": fmt.Sprintf("手机登录失败: %v", err)})
			return
		}
		acc := &Account{
			Username: body.Phone, DisplayName: displayName,
			UID: uid, Cookie: cookie, Token: token,
			IsGuest: false, DeviceID: device.ID,
			CreatedAt: time.Now().Unix(),
		}
		accountMu.Lock()
		accounts[body.Phone] = acc
		accountMu.Unlock()
		saveAccounts()
		delete(phoneDevices, body.Phone)
		sc := addStoredCookieFromLogin(cookie, "phone", displayName, true)
		saveMPayAccountForRequest(r, cookie, "phone", displayName, uid)
		_ = db.AddAuditLog(nil, nil, "mpay_phone_login", "game_account", fmt.Sprintf("手机登录: %s uid=%s", body.Phone, uid), requestIP(r))
		log.Printf("[MPAY-VERIFY] 手机登录成功: phone=%s uid=%s", body.Phone, uid)
		jsonW(w, r, M{
			"ok": true, "message": "手机号登录成功，已加入账号列表并设为当前账号",
			"username": body.Phone, "display_name": displayName,
			"uid": uid, "cookie": cookie, "cookie_id": sc.ID,
		})
	})

	http.HandleFunc("/api/mpay/accounts", func(w http.ResponseWriter, r *http.Request) {
		accountMu.Lock()
		defer accountMu.Unlock()
		list := []M{}
		for _, a := range accounts {
			list = append(list, M{
				"username": a.Username, "display_name": a.DisplayName,
				"uid": a.UID, "is_guest": a.IsGuest,
				"cookie": a.Cookie, "created_at": a.CreatedAt,
			})
		}
		jsonW(w, r, M{"ok": true, "accounts": list})
	})

	http.HandleFunc("/api/cookies/add", auth.SessionAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		body, _ := io.ReadAll(r.Body)
		cookieValue, source, err := extractCookieValue(body)
		if err != nil {
			jsonW(w, r, M{"ok": false, "error": sysErr(err), "stage": "parse"})
			return
		}
		log.Printf("cookies/add: cookie_len=%d source=%s", len(cookieValue), source)
		sc := newStoredCookie(cookieValue, source, "")
		validateMessage := ""
		if valid, err := validateCookieRecord(cookieValue, source, ""); err == nil {
			valid.ID = sc.ID
			valid.CreatedAt = sc.CreatedAt
			sc = valid
		} else {
			jsonW(w, r, M{"ok": false, "error": sysErr(err), "stage": "validate", "parsed": true, "source": source})
			return
		}
		cookieStore.Add(sc)
		cookieStore.SetActive(sc.ID)
		cookieStore.Save()
		sessionMu.Lock()
		client = nil
		cookieStr = sc.Cookie
		sessionMu.Unlock()
		os.WriteFile("cookie.json", []byte(sc.Cookie), 0644)
		resp := cookieResponse(sc)
		resp["ok"] = true
		if validateMessage != "" {
			resp["message"] = "Cookie 已保存并设为当前账号，但暂时无法获取账号详情"
			resp["warning"] = validateMessage
		} else {
			resp["message"] = "Cookie 解析成功，已设为当前账号"
		}
		resp["is_active"] = true
		jsonW(w, r, resp)
	}))
	http.HandleFunc("/api/cookies", auth.SessionAuth(func(w http.ResponseWriter, r *http.Request) {
		cookieStore.mu.RLock()
		defer cookieStore.mu.RUnlock()
		list := make([]M, 0, len(cookieStore.Cookies))
		for id, sc := range cookieStore.Cookies {
			entry := cookieResponse(sc)
			entry["is_active"] = id == cookieStore.ActiveID
			list = append(list, entry)
		}
		jsonW(w, r, M{"ok": true, "cookies": list, "active_id": cookieStore.ActiveID})
	}))
	http.HandleFunc("/api/cookies/switch", auth.SessionAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var req struct{ ID string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.ID == "" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("missing_id", "糟糕,缺少必要信息,请刷新后重试~")})
			return
		}
		if !cookieStore.SetActive(req.ID) {
			jsonW(w, r, M{"ok": false, "error": "糟糕,该账号没有 Cookie,请先添加 Cookie 再操作~"})
			return
		}
		cookieStore.Save()
		sessionMu.Lock()
		client = nil
		cookieStr = ""
		sessionMu.Unlock()
		if sc, ok := cookieStore.Get(req.ID); ok {
			os.WriteFile("cookie.json", []byte(sc.Cookie), 0644)
			cookieStr = sc.Cookie
		}
		jsonW(w, r, M{"ok": true, "message": "已切换账号"})
	}))
	http.HandleFunc("/api/cookies/refresh", auth.SessionAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var req struct{ ID string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.ID == "" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("missing_id", "糟糕,缺少必要信息,请刷新后重试~")})
			return
		}
		sc, ok := cookieStore.Get(req.ID)
		if !ok {
			jsonW(w, r, M{"ok": false, "error": "糟糕,该账号没有 Cookie,请先添加 Cookie 再操作~"})
			return
		}
		if strings.TrimSpace(sc.Cookie) == "" && repairStoredCookie(sc) {
			cookieStore.Save()
		}
		if strings.TrimSpace(sc.Cookie) == "" {
			jsonW(w, r, M{"ok": false, "error": "这个账号缺少 Cookie，无法重新登录；请重新登录或重新粘贴 Cookie", "stage": "refresh", "id": sc.ID})
			return
		}
		if ok, elapsed := cookieStore.CanRefresh(req.ID); !ok {
			jsonW(w, r, M{"ok": false, "error": fmt.Sprintf("刷新太快，请 %d 秒后再试", 4-elapsed), "cooldown": true, "retry_after": 4 - elapsed})
			return
		}
		updated, err := validateCookieRecord(sc.Cookie, sc.Source, firstNonEmpty(sc.DisplayName, sc.Name))
		if err != nil {
			jsonW(w, r, M{"ok": false, "error": sysErr(err), "stage": "refresh", "id": sc.ID})
			return
		}
		updated.ID = sc.ID
		updated.CreatedAt = sc.CreatedAt
		*sc = *updated
		sc.LastUsed = time.Now().Unix()
		cookieStore.Save()
		resp := cookieResponse(sc)
		resp["ok"] = true
		resp["message"] = "账号信息已刷新"
		jsonW(w, r, resp)
	}))

	http.HandleFunc("/api/cookies/active", auth.SessionAuth(func(w http.ResponseWriter, r *http.Request) {
		cookieStore.mu.RLock()
		sc, ok := cookieStore.Cookies[cookieStore.ActiveID]
		cookieStore.mu.RUnlock()
		if !ok {
			jsonW(w, r, M{"ok": false, "error": "没有活跃的 Cookie"})
			return
		}
		resp := cookieResponse(sc)
		resp["ok"] = true
		jsonW(w, r, resp)
	}))
	http.HandleFunc("/api/cookies/delete", auth.SessionAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var req struct{ ID string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.ID == "" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("missing_id", "糟糕,缺少必要信息,请刷新后重试~")})
			return
		}
		cookieStore.Delete(req.ID)
		cookieStore.Save()
		jsonW(w, r, M{"ok": true, "message": "已删除"})
	}))
	http.HandleFunc("/api/cookies/rotate", auth.SessionAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		if r.URL.Query().Get("key") != "x19rotate" {
			jsonW(w, r, M{"ok": false, "error": "密钥无效"})
			return
		}
		sc, err := cookieStore.Rotate()
		if err != nil {
			jsonW(w, r, M{"ok": false, "error": sysErr(err)})
			return
		}
		cookieStore.Save()
		os.WriteFile("cookie.json", []byte(sc.Cookie), 0644)
		sessionMu.Lock()
		client = nil
		cookieStr = sc.Cookie
		sessionMu.Unlock()
		resp := cookieResponse(sc)
		resp["ok"] = true
		resp["message"] = "已随机切换到下一个账号"
		resp["is_active"] = true
		jsonW(w, r, resp)
	}))

	http.HandleFunc("/api/mpay/use-cookie", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			jsonW(w, r, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
			return
		}
		var body struct{ Username string }
		json.NewDecoder(r.Body).Decode(&body)
		accountMu.Lock()
		acc, ok := accounts[body.Username]
		accountMu.Unlock()
		if !ok {
			jsonW(w, r, M{"ok": false, "error": msg.Get("account_not_found", "糟糕,账号不存在或已移除,请刷新后重试~")})
			return
		}
		os.WriteFile("cookie.json", []byte(acc.Cookie), 0644)
		cookieStr = acc.Cookie
		sessionMu.Lock()
		client = nil
		sessionMu.Unlock()
		log.Printf("[MPAY] 切换 cookie 为: %s (uid=%s)", acc.Username, acc.UID)
		jsonW(w, r, M{"ok": true, "message": fmt.Sprintf("已切换为账号 %s", acc.Username)})
	})

	// ── Serve frontend from external dist dir ──
	frontendDir := "../../../prism-web/dist"
	if _, err := os.Stat(frontendDir); os.IsNotExist(err) {
		exe, _ := os.Executable()
		frontendDir = filepath.Join(filepath.Dir(exe), "../../../prism-web/dist")
	}
	absDir, _ := filepath.Abs(frontendDir)
	fs := http.FileServer(http.Dir(absDir))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			jsonW(w, r, M{"success": true})
			return
		}
		reqPath := filepath.Clean(r.URL.Path)
		fullPath := filepath.Join(absDir, reqPath)
		if !strings.HasPrefix(fullPath, absDir) {
			http.NotFound(w, r)
			return
		}
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			http.ServeFile(w, r, filepath.Join(absDir, "index.html"))
			return
		}
		// 带 hash 的构建资产 → 强缓存；SPA 入口/其它 → 不缓存
		if strings.HasPrefix(r.URL.Path, "/assets/") || strings.Contains(filepath.Base(r.URL.Path), ".") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fs.ServeHTTP(w, r)
	})

	log.Printf("Prism on :8081 (MPay integrated)")
	// Global security middleware chain.
	// 命中限流必须返回 429，而不是 panic(http.ErrAbortHandler)——panic 会在不写任何响应
	// 的情况下掐断连接，反向代理(NX)会把上游这种异常关闭判定为请求失败并触发重连/重试，
	// 造成连接风暴、放大内存占用。recoverPanic 亦覆盖其它 handler panic。
	var wrapped http.Handler = http.DefaultServeMux
	wrapped = globalRateLimit(wrapped)
	// 注意：gzip 中间件已移除。http.FileServer 为静态资源预设 Content-Length，
	// 经 gzip 压缩后字节数变小但 Content-Length 未清除，造成 Content-Length 与实际
	// 响应体不匹配 → 走 HTTP/2（CDN）时报 ERR_HTTP2_PROTOCOL_ERROR，白屏。见
	// security_middleware.go gzip 实现。如需压缩需正确处理 Content-Length/Range。
	wrapped = securityHeaders(wrapped)
	wrapped = limitBody(maxRequestBodyBytes, wrapped)
	wrapped = realIPMiddleware(wrapped)
	wrapped = recoverPanic(wrapped)
	srv := &http.Server{
		Addr:              ":8081",
		Handler:           wrapped,
		ReadHeaderTimeout: 5 * time.Second, // 防 Slowloris 慢请求头
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 16, // 64KB
	}
	log.Fatal(srv.ListenAndServe())
}

func loadChallengeOverrides() {
	userIDs, err := db.GetChallengeOverrideUserIDs()
	if err != nil {
		log.Printf("[AUTH] Failed to load challenge overrides: %v", err)
		return
	}
	g79OverrideMu.Lock()
	for _, id := range userIDs {
		g79OverrideMap[id] = true
		log.Printf("[AUTH] Challenge override loaded for userID=%d", id)
	}
	g79OverrideMu.Unlock()
	log.Printf("[AUTH] Loaded %d challenge overrides", len(userIDs))
}

func autoRefreshLoop() {
	nextRefresh := make(map[int64]time.Time)
	checkTicker := time.NewTicker(18 * time.Second)
	defer checkTicker.Stop()

	// 启动时错开刷新时间，避免所有账号同时登录触发异地登录通知
	initDone := false
	staggerInit := func() {
		accs, err := db.ListAllAccounts()
		if err != nil || len(accs) == 0 {
			return
		}
		now := time.Now()
		delay := 0
		for _, acc := range accs {
			if acc.CookieData == "" || acc.IsServerOwner || acc.Disabled {
				continue
			}
			// 每个号间隔 12~24 秒，分散到几分钟内
			delay += 12 + rand.Intn(13)
			nextRefresh[acc.ID] = now.Add(time.Duration(delay) * time.Second)
		}
		log.Printf("[AUTO-REFRESH] 启动刷新调度: %d 个账号将在 %d 秒内分批完成", len(nextRefresh), delay)
	}

	for range checkTicker.C {
		if !initDone {
			// 启动后先静默 3 分钟，避免刚重启就刷新触发异地登录通知
			time.Sleep(3 * time.Minute)
			staggerInit()
			initDone = true
		}
		accs, err := db.ListAllAccounts()
		if err != nil || len(accs) == 0 {
			continue
		}
		// Clean up stale refresh entries for deleted accounts
		validIDs := make(map[int64]bool, len(accs))
		for _, acc := range accs {
			validIDs[acc.ID] = true
		}
		for id := range nextRefresh {
			if !validIDs[id] {
				delete(nextRefresh, id)
			}
			for id := range accountClient {
				if !validIDs[id] {
					ReleaseAccountProxy(id)
				}
			}

		}
		now := time.Now()
		for _, acc := range accs {
			if acc.CookieData == "" || acc.IsServerOwner || acc.Disabled {
				continue
			}
			if !acc.AutoRefreshEnabled {
				continue
			}
			if next, ok := nextRefresh[acc.ID]; ok && now.Before(next) {
				continue
			}
			log.Printf("[AUTO-REFRESH] 刷新 #%d %s (%s)", acc.ID, acc.DisplayName, acc.UID)
			proxyClient := AssignProxy(acc.ID)
			client, err := g79.NewClientWithHTTPClient(proxyClient)
			if err != nil {
				log.Printf("[AUTO-REFRESH] 客户端失败 #%d: %v", acc.ID, err)
				nextRefresh[acc.ID] = now.Add(1 * time.Minute)
				continue
			}
			if err := client.G79AuthenticateWithCookie(acc.CookieData); err != nil {
				errStr := err.Error()
				log.Printf("[AUTO-REFRESH] 认证失败 #%d: %v", acc.ID, err)
				// code 2001 不是错误：不判离线，直接重试
				if strings.Contains(errStr, "code: 2001") {
					nextRefresh[acc.ID] = now.Add(1 * time.Minute)
					continue
				}
				// code 32：IP被封/服务器维护，是临时性问题，非账号问题，不判离线，长退避
				if strings.Contains(errStr, "code: 32") {
					ReportAccountCode32(acc.ID)
					nextRefresh[acc.ID] = now.Add(30 * time.Minute)
					continue
				}
				// code 29 = 永久封禁：共享账号直接删除，私有账号标记封禁
				if strings.Contains(errStr, "code: 29") || strings.Contains(errStr, "禁止登录") {
					db.HandleAccountBanned(acc.ID)
					nextRefresh[acc.ID] = now.Add(2 * time.Minute)
					log.Printf("[AUTO-REFRESH] #%d code 29/禁止登录, 处理封禁(共享删除/私有标记), 2分钟后重试", acc.ID)
					continue
				}
				// 其他错误：标记离线并短退避
				db.UpdateAccountStatus(acc.ID, "offline")
				nextRefresh[acc.ID] = now.Add(1 * time.Minute)
				continue
			}
			info := db.AccountInfo{
				DisplayName: acc.DisplayName, UID: acc.UID, Status: "normal",
				GrowthLevel: acc.GrowthLevel, Score: acc.Score,
				SkinNumber: acc.SkinNumber, CapeNumber: acc.CapeNumber,
				AvatarImageURL: acc.AvatarImageURL, IsVip: acc.IsVip, Source: acc.Source,
			}
			fillAccountInfoFromClient(client, &info)
			db.UpdateAccountFull(acc.ID, info)
			delayMin := 60 + rand.Intn(300)
			nextRefresh[acc.ID] = now.Add(time.Duration(delayMin) * time.Minute)
			// 启动客户端模拟（模拟好友/通知/任务轮询等）
			log.Printf("[AUTO-REFRESH] #%d 完成, 下次 %d 分钟后, 启动模拟保活", acc.ID, delayMin)
		}
	}
}

type M map[string]any

// ──手机号登录辅助函数────
func mpaySendSMS(device *mpay.Device, phone string) error {
	client := &http.Client{Timeout: 900 * time.Millisecond}
	form := phoneBaseForm(device)
	form.Set("mobile", phone)
	form.Set("device_id", device.ID)
	req, _ := http.NewRequest("POST", "https://service.mkey.163.com/mpay/api/users/login/mobile/get_sms", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "com.netease.x19/840268037 NeteaseMobileGame/a5.2.0 (23117RK66C;32)")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		Code      int    `json:"code"`
		Reason    string `json:"reason"`
		VerifyURL string `json:"verify_url"`
	}
	json.Unmarshal(body, &r)
	if resp.StatusCode != 200 || r.Code != 0 {
		if r.VerifyURL != "" {
			return &mpay.NeedVerifyError{Code: r.Code, Reason: r.Reason, VerifyURL: r.VerifyURL}
		}
		return fmt.Errorf("发送验证码失败: %s (code=%d, status=%d)", r.Reason, r.Code, resp.StatusCode)
	}
	return nil
}

func mpayVerifySMS(device *mpay.Device, phone, code string) (string, error) {
	client := &http.Client{Timeout: 900 * time.Millisecond}
	form := phoneBaseForm(device)
	form.Set("mobile", phone)
	form.Set("smscode", code)
	form.Set("login_for", "1")
	form.Set("device_id", device.ID)
	req, _ := http.NewRequest("POST", "https://service.mkey.163.com/mpay/api/users/login/mobile/verify_sms", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "com.netease.x19/840268037 NeteaseMobileGame/a5.2.0 (23117RK66C;32)")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		Ticket    string `json:"ticket"`
		Code      int    `json:"code"`
		Reason    string `json:"reason"`
		VerifyURL string `json:"verify_url"`
	}
	json.Unmarshal(body, &r)
	if r.VerifyURL != "" {
		return "", &mpay.NeedVerifyError{Code: r.Code, Reason: r.Reason, VerifyURL: r.VerifyURL}
	}
	if r.Ticket == "" {
		return "", fmt.Errorf("验证失败: %s (code=%d)", r.Reason, r.Code)
	}
	return r.Ticket, nil
}

func mpayPhoneLogin(device *mpay.Device, phone, ticket string) (cookie, uid, token, displayName string, err error) {
	client := &http.Client{Timeout: 900 * time.Millisecond}
	form := phoneBaseForm(device)
	form.Set("mobile", phone)
	form.Set("ticket", ticket)
	form.Set("device_id", device.ID)
	form.Set("opt_fields", "nickname,avatar,realname_status,mobile_bind_status,exit_popup_info,mask_related_mobile,related_login_status,detect_is_new_user")
	req, _ := http.NewRequest("POST", "https://service.mkey.163.com/mpay/api/users/login/mobile/finish", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "com.netease.x19/840268037 NeteaseMobileGame/a5.2.0 (23117RK66C;32)")
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		User struct {
			ID       string `json:"id"`
			Token    string `json:"token"`
			Nickname string `json:"nickname"`
		} `json:"user"`
		Code      int    `json:"code"`
		Reason    string `json:"reason"`
		VerifyURL string `json:"verify_url"`
	}
	json.Unmarshal(body, &r)
	if r.VerifyURL != "" {
		err = &mpay.NeedVerifyError{Code: r.Code, Reason: r.Reason, VerifyURL: r.VerifyURL}
		return
	}
	if r.User.Token == "" {
		err = fmt.Errorf("手机登录失败: %s (code=%d)", r.Reason, r.Code)
		return
	}
	uid = r.User.ID
	token = r.User.Token
	if r.User.Nickname != "" {
		displayName = r.User.Nickname
	} else {
		displayName = phone
	}
	sauth := map[string]interface{}{
		"aim_info":    "{\"aim\":\"127.0.0.1\",\"country\":\"CN\",\"tz\":\"+0800\",\"tzid\":\"\"}",
		"app_channel": "netease", "client_login_sn": fmt.Sprintf("%x", rand.Uint64()),
		"deviceid": device.ID, "gameid": "x19", "gas_token": "",
		"get_access_token": "1", "ip": "127.0.0.1", "is_unisdk_guest": 0,
		"login_channel": "netease", "platform": "pc", "sdk_version": "3.9.0",
		"sdkuid": uid, "sessionid": token,
		"source_app_channel": "netease", "source_platform": "pc", "udid": device.UDID,
	}
	sauthJSON, _ := json.Marshal(sauth)
	cookiePayload, _ := json.Marshal(M{"sauth_json": string(sauthJSON)})
	cookie = string(cookiePayload)
	return
}

func extractUIDFromChecknumData(data string) string {
	var arr []any
	if err := json.Unmarshal([]byte(data), &arr); err != nil || len(arr) < 3 {
		return ""
	}
	f, ok := arr[2].(float64)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d", int64(f))
}

func fetchVerifyPageInfo(verifyURL string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", verifyURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "com.netease.x19/840268037 NeteaseMobileGame/a5.2.0 (23117RK66C;32)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	text := string(body)
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`1\d{2}[*＊xX\s-]{2,8}\d{2,4}`),
		regexp.MustCompile(`1\d{10}`),
		regexp.MustCompile(`(?:手机号|手机|号码|验证码)[^<\n]{0,80}`),
	}
	seen := map[string]bool{}
	var hints []string
	for _, re := range patterns {
		for _, match := range re.FindAllString(text, 5) {
			match = strings.TrimSpace(match)
			if match != "" && !seen[match] {
				seen[match] = true
				hints = append(hints, match)
			}
		}
	}
	if len(hints) == 0 {
		return "请打开验证页面查看接收验证码的手机号", nil
	}
	return strings.Join(hints, "；"), nil
}

func extractTicketFromURL(verifyURL string) string {
	if verifyURL == "" {
		return ""
	}
	u, err := url.Parse(verifyURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("ticket")
}

// extractSmsInfo 从 verify_url 提取动态验证码与发送目标短号，供前端展示"发送 CODE 至 PHONE"并打开短信应用。
// code 来自 verify_url 的 code 查询参数；phone/phoneBak 为固定发送短号(收件人)，验证码作为短信正文。
func extractSmsInfo(verifyURL string) (code, phone, phoneBak string) {
	phone = "1069016373035"
	phoneBak = "10698163016373035"
	if verifyURL == "" {
		return "367550", phone, phoneBak
	}
	if u, err := url.Parse(verifyURL); err == nil {
		code = u.Query().Get("code")
	}
	if code == "" {
		code = "367550"
	}
	return code, phone, phoneBak
}

func phoneBaseForm(device *mpay.Device) url.Values {
	form := url.Values{}
	form.Set("game_id", "aecfrxodyqaaaajp-g-x19")
	form.Set("gv", "840287970")
	form.Set("gvn", "3.7.15.287970")
	form.Set("cv", "a5.16.0")
	form.Set("sv", "33")
	form.Set("app_type", "games")
	form.Set("app_mode", "2")
	form.Set("app_channel", "netease")
	form.Set("sc", "1")
	form.Set("jf_game_id", "x19")
	form.Set("pkg_channel", "netease")
	form.Set("mcount_app_key", "EEkEEXLymcNjM42yLY3Bn6AO15aGy4yq")
	form.Set("mcount_transaction_id", device.MCountTransactionID)
	form.Set("transid", device.TransID)
	form.Set("urs_udid", fmt.Sprintf("%x", md5.Sum([]byte(device.UDID+device.Mac))))
	form.Set("_cloud_extra_base64", "e30=")
	return form
}

// GET /api/owner/servers 获取当前用户所有服主账号的服务器列表
func handleOwnerServers(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	owners, err := db.GetServerOwnerAccounts(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}

	type rentalServer struct {
		ServerID    string `json:"server_id"`
		Name        string `json:"name"`
		Status      int    `json:"status"`
		PlayerCount int    `json:"player_count"`
		Capacity    int    `json:"capacity"`
		MCVersion   string `json:"mc_version"`
	}
	type domainServer struct {
		Sid         string `json:"sid"`
		Name        string `json:"name"`
		Status      int    `json:"status"`
		OnlineCount int    `json:"online_count"`
		Capacity    int    `json:"capacity"`
	}
	type ownerResp struct {
		ID            int64          `json:"id"`
		DisplayName   string         `json:"display_name"`
		RentalServers []rentalServer `json:"rental_servers"`
		DomainServers []domainServer `json:"domain_servers"`
	}

	var result []ownerResp
	for _, owner := range owners {
		if owner.CookieData == "" {
			continue
		}
		resp := ownerResp{ID: owner.ID, DisplayName: owner.DisplayName}

		// 优先使用缓存的客户端，避免重复认证导致 code 22
		client := getCachedClient(owner.ID)
		if client == nil {
			// 没有缓存时不认证，避免影响用户在客户端的在线状态
			continue
		}

		// 查询租赁服列表
		if rentalResp, err := client.SearchMyRentalServers(50, 0); err == nil {
			for _, s := range rentalResp.Entities {
				resp.RentalServers = append(resp.RentalServers, rentalServer{
					ServerID: s.EntityID, Name: s.Name, Status: s.Status,
					PlayerCount: s.PlayerCount, Capacity: s.Capacity, MCVersion: s.MCVersion,
				})
			}
		}

		// 查询山头服列表（已加入的）
		if domainResp, err := client.GetOtherDomainServers(); err == nil {
			for _, s := range domainResp.Entities {
				resp.DomainServers = append(resp.DomainServers, domainServer{
					Sid: s.Sid, Name: s.Name,
					Status: int(s.Status.Int64()), OnlineCount: 0, Capacity: 0,
				})
			}
		}

		result = append(result, resp)
	}

	jsonResp(w, M{"ok": true, "accounts": result})
}

// POST /api/owner/servers/refresh 强制刷新服主账号的服务器列表（重新认证并缓存）
func handleOwnerServersRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}

	owners, err := db.GetServerOwnerAccounts(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}

	type rentalServer struct {
		ServerID    string `json:"server_id"`
		Name        string `json:"name"`
		Status      int    `json:"status"`
		PlayerCount int    `json:"player_count"`
		Capacity    int    `json:"capacity"`
		MCVersion   string `json:"mc_version"`
	}
	type domainServer struct {
		Sid         string `json:"sid"`
		Name        string `json:"name"`
		Status      int    `json:"status"`
		OnlineCount int    `json:"online_count"`
		Capacity    int    `json:"capacity"`
	}
	type ownerResp struct {
		ID            int64          `json:"id"`
		DisplayName   string         `json:"display_name"`
		RentalServers []rentalServer `json:"rental_servers"`
		DomainServers []domainServer `json:"domain_servers"`
	}

	var result []ownerResp
	for _, owner := range owners {
		if owner.CookieData == "" {
			continue
		}
		resp := ownerResp{ID: owner.ID, DisplayName: owner.DisplayName}

		client, err := newG79ClientWithProxy(owner.ID)
		if err != nil {
			continue
		}
		if err := client.G79AuthenticateWithCookie(owner.CookieData); err != nil {
			continue
		}
		setCachedClient(owner.ID, client)

		// 查询租赁服列表
		if rentalResp, err := client.SearchMyRentalServers(50, 0); err == nil {
			for _, s := range rentalResp.Entities {
				resp.RentalServers = append(resp.RentalServers, rentalServer{
					ServerID: s.EntityID, Name: s.Name, Status: s.Status,
					PlayerCount: s.PlayerCount, Capacity: s.Capacity, MCVersion: s.MCVersion,
				})
			}
		}

		// 查询山头服列表（已加入的）
		if domainResp, err := client.GetOtherDomainServers(); err == nil {
			for _, s := range domainResp.Entities {
				resp.DomainServers = append(resp.DomainServers, domainServer{
					Sid: s.Sid, Name: s.Name,
					Status: int(s.Status.Int64()), OnlineCount: 0, Capacity: 0,
				})
			}
		}

		result = append(result, resp)
	}

	jsonResp(w, M{"ok": true, "accounts": result})
}
