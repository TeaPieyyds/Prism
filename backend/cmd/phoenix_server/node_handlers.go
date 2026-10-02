package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
)

// ── P2P 节点归属系统（令牌 affinity）──
//
// 只有配置了令牌的用户能贡献节点。贡献后：
//   - 主令牌 → 服务该用户全部账号
//   - 子令牌 → 只服务该令牌绑定的账号
// 节点失联/退出 → 其名下账号重新分配。

const (
	// 以下为部署期配置：按你的协调者环境修改
	gostBinPath    = "/path/to/gost"  // 协调者上的 gost 二进制
	coordTunPort   = 8443             // 协调者隧道端口（本地桥用 127.0.0.1:8443）
	coordFrontIP   = "192.0.2.1"      // 前台跳板 IP（节点拨入，隐藏真实协调者）
	coordFrontPort = 19205            // 前台跳板端口（coordFrontIP:coordFrontPort → 协调者:8443）
	nodeSocksMin   = 19100                    // 每节点 SOCKS5 桥端口范围
	nodeSocksMax   = 19199
	nodeOfflineSec = 180                      // last_seen 超过该秒数判失联（放宽：手机网络不稳时心跳易迟到）
)

// 网易 G79 认证目标域名（节点 rtcp forwarder 覆盖）
var g79Domains = []string{
	"g79apigatewayobt.minecraft.cn", "g79apigatewayobtweixin.minecraft.cn",
	"g79authobt.minecraft.cn", "g79mclobthome.minecraft.cn", "g79mclobt.minecraft.cn",
	"g79mcltransfer.minecraft.cn", "g79obtapigtcoregray.minecraft.cn",
	"drpf-g79.proxima.nie.netease.com", "g79apigatewaygrayobt.nie.netease.com",
	"g79.gdl.netease.com", "g79mclobtgray.nie.netease.com", "g79mclobthomegray.nie.netease.com",
	"g79transfernew.nie.netease.com", "g79.update.netease.com", "impression.update.netease.com",
	"mcrealms.update.netease.com", "mgbsdk.matrix.netease.com", "pub-api.seadra.netease.com",
	"x19apigatewayobt.nie.netease.com", "x19.update.netease.com",
	"mc.163.com", "mcpel-web.16163.com",
}

// bridgeProc 跟踪一个 gost 桥子进程。exited 在进程退出后 close（Wait 已回收，不留僵尸）。
// 不能用 Signal(0) 判断存活：未 Wait 的僵尸进程 Signal(0) 也返回 nil，会把死桥当活桥。
type bridgeProc struct {
	cmd    *exec.Cmd
	exited chan struct{}
}

// 桥管理器：tunnel_id -> gost 桥子进程句柄
var (
	nodeBridgeMu    sync.Mutex
	nodeBridges     = map[string]*bridgeProc{}
	unhealthyNodes  = map[string]bool{} // tunnel_id -> 请求该节点自动停止
	nodeFailCount   = map[string]int{}  // tunnel_id -> 连续出口探测失败次数
	nodeHealthMu    sync.Mutex          // 保护 unhealthyNodes / nodeFailCount

	// 协调者 ingress 限频：限制 gost-coord 重启频率，避免节点频繁上下线反复打断隧道。
	// 重启会断开全部已拨入隧道（手机节点 + 桥），必须严格控制频率。
	ingressMu            sync.Mutex
	lastIngressSync      time.Time
	lastRestartedIngress string
	pendingIngress       string // 已写入 unit 但尚未重启生效的 ingress（冷却结束后补重启）
	ingressDebounce      = 3 * time.Minute
)

const nodeHealthFailThreshold = 3                 // 在线节点出口探测连续失败达到该值判"连不上"（通知 App 停止）
// 下线阈值必须 > 规则冷却（3 分钟）：节点重新上线后要等 ingress 冷却过期才能进规则，
// 进规则前探测必然失败，阈值太短会把真节点在进规则前就踢掉（死锁）。
const nodeHealthOfflineThreshold = 15             // 连续失败达到该值服务器直接下线（≈2.5 分钟，幽灵节点）
const nodeHealthGracePeriod = 60 * time.Second     // 新注册节点宽限期：期间不探测，给 gost 时间连上

// nodeSocksURL 返回某节点桥端口的 SOCKS5 地址（proxy_pool 亲和值，远程 DNS）。
func nodeSocksURL(port int) string {
	return "socks5h://127.0.0.1:" + strconv.Itoa(port)
}

// nodeBridgeListener 返回桥进程的监听地址（gost -L 用 socks5://，非 socks5h）。
func nodeBridgeListener(port int) string {
	return "socks5://127.0.0.1:" + strconv.Itoa(port)
}

// startNodeBridge 为该节点 spawn 一个 gost 桥：socks5://127.0.0.1:port -F tunnel://127.0.0.1:8443?tunnel.id=id
// 若已有桥进程但已死，则清理并重起。
func startNodeBridge(n *db.Node) {
	// P2P 节点出口已禁用（2026-08）：不再为节点拉起 gost 桥。
	// 账号分配已不走节点出口（preferredProxyForAccount 恒返回空），直接返回避免残留桥进程。
	// 日后恢复时删掉此 return 即可。
	return
	nodeBridgeMu.Lock()
	defer nodeBridgeMu.Unlock()
	if b, ok := nodeBridges[n.TunnelID]; ok {
		select {
		case <-b.exited:
			// 桥进程已退出（Wait 已回收），清掉重起
			delete(nodeBridges, n.TunnelID)
		default:
			return // 仍在运行
		}
	}
	cmd := exec.Command(gostBinPath,
		"-L", nodeBridgeListener(n.SocksPort),
		"-F", "tunnel://127.0.0.1:"+strconv.Itoa(coordTunPort)+"?tunnel.id="+n.TunnelID)
	if err := cmd.Start(); err != nil {
		log.Printf("[NODE] 启动节点 %s 桥失败: %v", n.TunnelID, err)
		return
	}
	b := &bridgeProc{cmd: cmd, exited: make(chan struct{})}
	go func() {
		cmd.Wait() // 回收子进程，防止僵尸累积
		close(b.exited)
	}()
	nodeBridges[n.TunnelID] = b
	log.Printf("[NODE] 节点 %s 桥已启动 :%d", n.TunnelID, n.SocksPort)
}

// syncCoordIngress 已废弃：自写协调者(p2p_coord.go)内嵌进 phoenix_server，节点上下线
// 只动进程内存表，无需重写 gost systemd unit / 重启协调者。保留空实现避免改动调用点。
func syncCoordIngress() {
}

// stopNodeBridge 停掉该节点的桥进程。
func stopNodeBridge(tunnelID string) {
	nodeBridgeMu.Lock()
	b := nodeBridges[tunnelID]
	delete(nodeBridges, tunnelID)
	nodeBridgeMu.Unlock()
	if b != nil && b.cmd.Process != nil {
		b.cmd.Process.Kill()
		log.Printf("[NODE] 节点 %s 桥已停止", tunnelID)
	}
}

// reassignNodeAccounts 把挂在该节点出口的账号释放并重新分配。
func reassignNodeAccounts(n *db.Node) {
	target := nodeSocksURL(n.SocksPort)
	assignMu.Lock()
	var ids []int64
	for id, p := range accountProxy {
		if p == target {
			ids = append(ids, id)
		}
	}
	assignMu.Unlock()
	for _, id := range ids {
		ReleaseAccountProxy(id)
		AssignProxy(id)
		log.Printf("[NODE] 节点 %s 下线，账号 #%d 重新分配", n.TunnelID, id)
	}
}

// handleNodeRegister 登记一台新节点，仅接受带令牌的用户。
// 返回 App 起 gost 节点所需的配置。
func handleNodeRegister(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "未登录或令牌无效")})
		return
	}
	apiTok := auth.GetAPIToken(r.Context()) // nil = 主令牌
	isMaster := apiTok == nil
	var tokenID *int64
	if apiTok != nil {
		tokenID = &apiTok.ID
	}

	// 幂等：该令牌已有一台 online 节点则直接返回现有
	var existing *db.Node
	if isMaster {
		nodes, err := db.GetOnlineNodesByUser(user.ID)
		if err == nil {
			for _, n := range nodes {
				if n.IsMaster {
					existing = n
					break
				}
			}
		}
	} else if apiTok != nil {
		existing, _ = db.GetNodeByTokenID(apiTok.ID)
	}
	if existing != nil {
		startNodeBridge(existing)
		db.TouchNodeSeen(existing.TunnelID) // 重注册即视为心跳，避免刚注册就被判失联
		nodeHealthMu.Lock()
		delete(nodeFailCount, existing.TunnelID) // 重新注册即重新开始计探测失败，避免带着旧失败立即被判死
		nodeHealthMu.Unlock()
		nodeConfig(w, existing)
		return
	}

	// 复用该令牌最近一条历史节点（含 offline）：tunnel_id 不变 → 协调者 ingress
	// 不变 → 不重启协调者、不断开其他节点隧道。端口重新分配（旧端口可能已被占用）。
	var reused *db.Node
	if isMaster {
		reused, _ = db.GetLatestNodeByUser(user.ID, true)
	} else if apiTok != nil {
		reused, _ = db.GetLatestNodeByTokenID(apiTok.ID)
	}
	if reused != nil {
		port, err := allocSocksPort()
		if err != nil {
			jsonResp(w, M{"ok": false, "error": "端口池已满，无法贡献节点"})
			return
		}
		if err := db.UpdateNodePort(reused.TunnelID, port); err != nil {
			jsonResp(w, M{"ok": false, "error": "创建节点失败: " + err.Error()})
			return
		}
		reused.SocksPort = port
		db.SetNodeStatus(reused.TunnelID, "online")
		db.TouchNodeSeen(reused.TunnelID)
		nodeHealthMu.Lock()
		delete(nodeFailCount, reused.TunnelID) // 复用即重新开始计探测失败
		nodeHealthMu.Unlock()
		startNodeBridge(reused)
		syncCoordIngress() // tunnel_id 未变，不会触发重启
		log.Printf("[NODE] 用户 #%d 复用节点 %s (master=%v, 端口=%d)", user.ID, reused.TunnelID, isMaster, port)
		nodeConfig(w, reused)
		return
	}

	tunnelID := uuid.NewString()
	port, err := allocSocksPort()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "端口池已满，无法贡献节点"})
		return
	}
	node, err := db.CreateNode(user.ID, tokenID, isMaster, tunnelID, port)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": "创建节点失败: " + err.Error()})
		return
	}
	startNodeBridge(node)
	syncCoordIngress() // 把新节点 tunnel_id 注册进协调者 ingress
	log.Printf("[NODE] 用户 #%d 贡献节点 %s (master=%v, 端口=%d)", user.ID, tunnelID, isMaster, port)
	nodeConfig(w, node)
}

// nodeConfig 输出 App 起 gost 节点所需配置。
func nodeConfig(w http.ResponseWriter, n *db.Node) {
	jsonResp(w, M{
		"ok":            true,
		"tunnel_id":     n.TunnelID,
		"socks_port":    n.SocksPort,
		"coord_ip":      coordFrontIP,   // 前台跳板 IP，隐藏真实协调者
		"coord_tun_port": coordFrontPort, // 前台跳板端口
		"g79_domains":   g79Domains,
	})
}

// handleNodeHeartbeat 刷新节点心跳。
func handleNodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "未登录或令牌无效")})
		return
	}
	var req struct {
		TunnelID string `json:"tunnel_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TunnelID == "" {
		jsonResp(w, M{"ok": false, "error": "参数错误"})
		return
	}
	n, err := db.GetNodeByTunnelID(req.TunnelID)
	if err != nil || n.UserID != user.ID {
		jsonResp(w, M{"ok": false, "error": "节点不存在或不属于你"})
		return
	}
	db.TouchNodeSeen(req.TunnelID)
	startNodeBridge(n) // 桥进程若意外退出则拉起
	// 若该节点被判定"在线但连不上"，在心跳响应里告知 App 自动关闭
	nodeHealthMu.Lock()
	shouldStop := unhealthyNodes[req.TunnelID]
	if shouldStop {
		delete(unhealthyNodes, req.TunnelID)
	}
	nodeHealthMu.Unlock()
	if shouldStop {
		log.Printf("[NODE] 通知节点 %s 自动停止贡献", req.TunnelID)
	}
	jsonResp(w, M{"ok": true, "should_stop": shouldStop})
}

// handleNodeBye 节点主动退出。
func handleNodeBye(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "未登录或令牌无效")})
		return
	}
	var req struct {
		TunnelID string `json:"tunnel_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TunnelID == "" {
		jsonResp(w, M{"ok": false, "error": "参数错误"})
		return
	}
	n, err := db.GetNodeByTunnelID(req.TunnelID)
	if err != nil || n.UserID != user.ID {
		jsonResp(w, M{"ok": false, "error": "节点不存在或不属于你"})
		return
	}
	offlineNode(n)
	jsonResp(w, M{"ok": true})
}

// offlineNode 统一把节点置为 offline：停桥 + 重分配账号。
func offlineNode(n *db.Node) {
	db.SetNodeStatus(n.TunnelID, "offline")
	stopNodeBridge(n.TunnelID)
	reassignNodeAccounts(n)
	syncCoordIngress() // 从协调者 ingress 移除该节点
	log.Printf("[NODE] 节点 %s 已下线", n.TunnelID)
}

// allocSocksPort 在范围内找一个未占用的端口（只占在线节点的端口，离线节点的可复用）。
// 离线节点记录会保留 24 小时供复用，其端口必须可被重新分配，否则会白白耗尽端口池。
func allocSocksPort() (int, error) {
	// 端口必须：DB 里无任何在线节点占用 + 当前无进程监听（残留桥没在跑）。
	// 避免多节点共用端口导致的桥链错隧道。
	used := map[int]bool{}
	if rows, err := db.ListOnlineNodePorts(); err == nil {
		for _, p := range rows {
			used[p] = true
		}
	}
	for p := nodeSocksMin; p <= nodeSocksMax; p++ {
		if used[p] {
			continue
		}
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			continue // 有进程监听（残留桥），跳过
		}
		l.Close()
		return p, nil
	}
	return 0, fmt.Errorf("no free port")
}

// nodeHealthCheck 探测每个在线节点的桥能否出口 G79。连续失败达到阈值则标记"需停止"，
// 由 heartbeat 返回给 App 让其自动关闭；继续失败则由服务器直接下线（幽灵节点：
// 注册了但隧道从未拨入，不应一直占着入口规则）。
// 返回需要服务器直接下线的节点，由 sweeper 主循环执行（避免并发调用 offlineNode）。
// 并发探测：串行时每个节点最长 8s 超时，节点一多会拖慢 sweeper 一轮，
// 让心跳超时判定滞后到分钟级。
func nodeHealthCheck() []*db.Node {
	nodes, err := db.GetAllOnlineNodes()
	if err != nil {
		return nil
	}
	now := time.Now()
	sem := make(chan struct{}, 8) // 限制并发，避免瞬间把协调者打满
	var wg sync.WaitGroup
	for _, n := range nodes {
		// 宽限期：新注册节点先给时间连 gost，期间不探测，避免"一上线就被判离线"
		if ct, err := time.Parse(time.RFC3339, n.CreatedAt); err == nil && now.Sub(ct) < nodeHealthGracePeriod {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(n *db.Node) {
			defer wg.Done()
			defer func() { <-sem }()
			if probeNodeBridge(n) {
				nodeHealthMu.Lock()
				delete(nodeFailCount, n.TunnelID)
				nodeHealthMu.Unlock()
				return
			}
			nodeHealthMu.Lock()
			nodeFailCount[n.TunnelID]++
			fail := nodeFailCount[n.TunnelID] >= nodeHealthFailThreshold
			if fail {
				unhealthyNodes[n.TunnelID] = true
			}
			nodeHealthMu.Unlock()
			if fail {
				log.Printf("[NODE] 节点 %s 在线但连不上 G79，标记自动停止", n.TunnelID)
			}
		}(n)
	}
	wg.Wait()

	var toOffline []*db.Node
	for _, n := range nodes {
		nodeHealthMu.Lock()
		fail := nodeFailCount[n.TunnelID]
		if fail >= nodeHealthOfflineThreshold {
			delete(nodeFailCount, n.TunnelID)
			toOffline = append(toOffline, n)
		}
		nodeHealthMu.Unlock()
	}
	return toOffline
}

// probeNodeBridge 通过节点的桥发一个 G79 请求，验证能否真正出口。
func probeNodeBridge(n *db.Node) bool {
	rt, err := transportFor(nodeSocksURL(n.SocksPort))
	if err != nil {
		return false
	}
	c := &http.Client{Transport: rt, Timeout: 8 * time.Second}
	resp, err := c.Get("https://g79authobt.minecraft.cn")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

// nodeSweeper 周期扫描：失联节点置下线，确保在线节点的桥在跑。
func nodeSweeper() {
	for range time.NewTicker(10 * time.Second).C {
		syncCoordIngress() // 周期同步协调者 ingress（覆盖新增/移除）
		// 探测在线节点能否出口；隧道从未拨入的幽灵节点由服务器直接下线，
		// 不依赖 App 响应 should_stop。
		for _, n := range nodeHealthCheck() {
			log.Printf("[NODE] 节点 %s 连续探测失败，服务器直接下线", n.TunnelID)
			offlineNode(n)
		}
		nodes, err := db.GetAllOnlineNodes()
		if err != nil {
			continue
		}
		now := time.Now()
		for _, n := range nodes {
			seen, perr := time.Parse(time.RFC3339, n.LastSeen)
			if perr == nil && now.Sub(seen) > nodeOfflineSec*time.Second {
				log.Printf("[NODE] 节点 %s 心跳超时，判定失联", n.TunnelID)
				offlineNode(n)
				continue
			}
			startNodeBridge(n) // 确保桥在跑
		}
		// 周期清理离线节点，释放端口，防止累积占满端口池
		if _, err := db.DeleteOfflineNodes(); err != nil {
			log.Printf("[NODE] 清理离线节点失败: %v", err)
		}
	}
}
