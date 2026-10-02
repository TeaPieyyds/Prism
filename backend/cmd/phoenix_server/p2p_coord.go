// p2p_coord — 自写 P2P 反向隧道协调者（替代 gost 桥方案）。
//
// 与 gost 方案的本质区别：
//   - 协调者直接内嵌进 phoenix_server（同一进程），不再依赖 systemd 服务 / 重写
//     systemd unit / 重启协调者来登记或移除节点。节点上下线只动内存里的表，不打断其他节点。
//   - 只暴露【一个】本地 SOCKS5 给 proxy_pool（不再每节点一桥一端口），由协调者按
//     轮询 + 健康度在在线节点间选一台出口。
//   - 节点用隧道号(tunnel_id)鉴权拨入，目标地址按 G79 域名白名单限制（防开放代理滥用）。
//
// 协议：多路复用帧 [1B type][2B big-endian len][payload]
//   type=1 控制(JSON) | type=2 数据(4B cid + data) | type=3 关断(4B cid)
//   一条节点隧道承载多个并发中继，任一可独立关断。
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/adb-lanlu/prism-oss/internal/db"
)

const (
	frameControl = 1
	frameData    = 2
	frameClose   = 3

	maxFrameLen     = 16 * 1024 * 1024 // 单帧上限，防恶意节点 OOM
	coordHBTimeout  = 45 * time.Second // 节点心跳超时即踢
	coordHBSweep    = 10 * time.Second
	connectTimeout  = 10 * time.Second // 节点拨目标地址超时
	relayBufSize    = 16384
	maxRelayPerNode = 200 // 单节点并发中继上限
)

// p2p 协调者端口：与 gost 时期保持一致，便于复用 160:19205 前台跳板转发到 :8443，
// 以及复用 proxy_pool 已认识 socks5h://127.0.0.1:19003 的惯例。用变量以便测试注入。
var (
	p2pTunPort   = 8443 // 协调者隧道监听端口（节点经前台跳板 160:19205 转发至此）
	p2pSocksPort = 19003 // 协调者本地 SOCKS5 入口（proxy_pool 消费）
)

// g79Suffixes 节点允许拨出的目标域名后缀（网易 G79 认证域名，见 node_handlers.go 的 g79Domains）。
var g79Suffixes = []string{
	".minecraft.cn", ".nie.netease.com", ".netease.com", ".163.com", ".16163.com",
}

// coordTestAllowAllTargets 仅测试用：放开白名单以便中继到本地测试目标。生产恒为 false。
var coordTestAllowAllTargets = false

// whitelistedTarget 校验目标 host:port 是否属于 G79 白名单（host 用后缀匹配，port 仅 443/80）。
func whitelistedTarget(target string) bool {
	if coordTestAllowAllTargets {
		return true
	}
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return false
	}
	if port != "443" && port != "80" {
		return false
	}
	h := strings.ToLower(host)
	for _, s := range g79Suffixes {
		if strings.HasSuffix(h, s) {
			return true
		}
	}
	return false
}

// ---------- 帧编解码 ----------

func writeFrame(w io.Writer, typ byte, payload []byte) error {
	if len(payload) > maxFrameLen {
		return errors.New("frame too large")
	}
	hdr := []byte{typ, byte(len(payload) >> 8), byte(len(payload))}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func readFrame(r *bufio.Reader) (byte, []byte, error) {
	hdr := make([]byte, 3)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return 0, nil, err
	}
	n := int(hdr[1])<<8 | int(hdr[2])
	if n > maxFrameLen {
		return 0, nil, errors.New("frame too large")
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return hdr[0], payload, nil
}

func be32(b []byte) uint32     { return binary.BigEndian.Uint32(b) }
func put32(b []byte, v uint32) { binary.BigEndian.PutUint32(b, v) }

// coordMsg 节点与协调者间的控制消息。
type coordMsg struct {
	Op     string `json:"op"`
	Tunnel string `json:"tunnel,omitempty"` // register: 节点隧道号
	Cid    uint32 `json:"cid,omitempty"`
	Target string `json:"target,omitempty"`
	OK     bool   `json:"ok,omitempty"`
}

// nodeConn 一台已拨入的在线节点。
type nodeConn struct {
	id     string // tunnel_id
	conn   net.Conn
	br     *bufio.Reader
	wmu    sync.Mutex        // 序列化对该节点隧道帧的写入
	cids   map[uint32]net.Conn // 活跃中继 cid -> 协调者侧 socks 客户端
	lastHB time.Time
	mu     sync.Mutex
}

// work 一次待确认的 socks 连接请求。
type work struct {
	client net.Conn
	target string
	done   chan struct{}
}

// coordinator 节点目录 + 盲中继。
type coordinator struct {
	mu      sync.Mutex
	nodes   []*nodeConn
	pending map[uint32]*work
	nextCID uint32
}

func newCoordinator() *coordinator {
	return &coordinator{pending: map[uint32]*work{}}
}

// p2pCoord 协调者全局单例：main() 启动，proxy_pool 据此决定是否走节点出口。
var p2pCoord = newCoordinator()

// onlineCount 返回当前真正拨入的在隧道节点数。
func (c *coordinator) onlineCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.nodes)
}

// sendNodeFrame 写一帧到节点隧道（wmu 串行化，避免并发写坏流）。
func (c *coordinator) sendNodeFrame(n *nodeConn, typ byte, payload []byte) error {
	n.wmu.Lock()
	defer n.wmu.Unlock()
	return writeFrame(n.conn, typ, payload)
}

func (c *coordinator) removeNode(n *nodeConn) {
	c.mu.Lock()
	for i, x := range c.nodes {
		if x == n {
			c.nodes = append(c.nodes[:i], c.nodes[i+1:]...)
			break
		}
	}
	// 释放该节点上的所有挂起中继
	for cid, cl := range n.cids {
		cl.Close()
		delete(n.cids, cid)
		if w := c.pending[cid]; w != nil {
			close(w.done)
		}
		delete(c.pending, cid)
	}
	c.mu.Unlock()
}

// pickNode 轮询选一台在线节点。返回 nil 表示无可用节点。
func (c *coordinator) pickNode(round *uint32) *nodeConn {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.nodes) == 0 {
		return nil
	}
	*round++
	start := int(*round % uint32(len(c.nodes)))
	for i := 0; i < len(c.nodes); i++ {
		n := c.nodes[(start+i)%len(c.nodes)]
		n.mu.Lock()
		cnt := len(n.cids)
		n.mu.Unlock()
		if cnt < maxRelayPerNode {
			return n
		}
	}
	return nil
}

// handleNode 处理一台节点的整个生命周期。
func (c *coordinator) handleNode(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)

	// 首帧必须是 register
	typ, payload, err := readFrame(br)
	if err != nil || typ != frameControl {
		log.Printf("[COORD] 拒绝未注册节点 %s", conn.RemoteAddr())
		return
	}
	var m coordMsg
	if json.Unmarshal(payload, &m) != nil || m.Op != "register" || m.Tunnel == "" {
		log.Printf("[COORD] 拒绝未注册节点 %s", conn.RemoteAddr())
		return
	}
	// 鉴权：隧道号必须存在于 nodes 表（在线 / 属于某用户），防止陌生人白嫖出口
	if _, err := db.GetNodeByTunnelID(m.Tunnel); err != nil {
		log.Printf("[COORD] 拒绝未知隧道号 %s", m.Tunnel)
		return
	}
	n := &nodeConn{id: m.Tunnel, conn: conn, br: br, cids: map[uint32]net.Conn{}, lastHB: time.Now()}
	c.mu.Lock()
	c.nodes = append(c.nodes, n)
	c.mu.Unlock()
	log.Printf("[COORD] 节点 %s 上线 (%s)", m.Tunnel, conn.RemoteAddr())

	for {
		typ, payload, err := readFrame(br)
		if err != nil {
			c.removeNode(n)
			log.Printf("[COORD] 节点 %s 下线", m.Tunnel)
			return
		}
		switch typ {
		case frameControl:
			var msg coordMsg
			if json.Unmarshal(payload, &msg) != nil {
				continue
			}
			switch msg.Op {
			case "heartbeat":
				n.mu.Lock()
				n.lastHB = time.Now()
				n.mu.Unlock()
			case "bye":
				c.removeNode(n)
				log.Printf("[COORD] 节点 %s 主动下线", m.Tunnel)
				return
			case "connect_res":
				c.mu.Lock()
				w := c.pending[msg.Cid]
				delete(c.pending, msg.Cid)
				c.mu.Unlock()
				if w == nil {
					continue
				}
				if msg.OK {
					n.mu.Lock()
					n.cids[msg.Cid] = w.client
					n.mu.Unlock()
					socksReply(w.client, 0)
					log.Printf("[COORD] 节点 %s 出口成功 → %s", n.id, w.target)
					go c.relayClientToNode(n, w.client, msg.Cid)
				} else {
					log.Printf("[COORD] 节点 %s 拨 %s 失败(connect_res !ok)", n.id, w.target)
					socksReply(w.client, 5)
					w.client.Close()
				}
				close(w.done)
			}
		case frameData:
			if len(payload) < 4 {
				continue
			}
			cid := be32(payload[:4])
			n.mu.Lock()
			cl := n.cids[cid]
			n.mu.Unlock()
			if cl != nil {
				cl.Write(payload[4:])
			}
		case frameClose:
			if len(payload) < 4 {
				continue
			}
			cid := be32(payload[:4])
			n.mu.Lock()
			cl := n.cids[cid]
			delete(n.cids, cid)
			n.mu.Unlock()
			if cl != nil {
				cl.Close()
			}
		}
	}
}

// relayClientToNode 单向转发：socks 客户端 -> 节点隧道（数据帧 cid）。
// 调用后 client 连接归本函数接管，结束或出错时由其关闭。
func (c *coordinator) relayClientToNode(n *nodeConn, client net.Conn, cid uint32) {
	defer client.Close()
	var up int64
	buf := make([]byte, relayBufSize)
	for {
		nread, err := client.Read(buf)
		if nread > 0 {
			up += int64(nread)
			payload := make([]byte, 4+nread)
			put32(payload[:4], cid)
			copy(payload[4:], buf[:nread])
			if werr := c.sendNodeFrame(n, frameData, payload); werr != nil {
				break
			}
		}
		if err != nil {
			break
		}
	}
	log.Printf("[COORD] 中继结束 cid=%d 上行=%d 字节 (节点 %s)", cid, up, n.id)
	cl := make([]byte, 4)
	put32(cl, cid)
	c.sendNodeFrame(n, frameClose, cl)
	n.mu.Lock()
	delete(n.cids, cid)
	n.mu.Unlock()
}

// handleSOCKS 处理本地 SOCKS5 CONNECT 请求，路由到某台在线节点。
func (c *coordinator) handleSOCKS(client net.Conn) {
	handedOff := false // 连接交接给 relayClientToNode 后，由其接管关闭
	defer func() {
		if !handedOff {
			client.Close()
		}
	}()
	r := bufio.NewReader(client)
	if ver, err := r.ReadByte(); err != nil || ver != 5 {
		return
	}
	nmethods, err := r.ReadByte()
	if err != nil {
		return
	}
	if _, err := io.CopyN(io.Discard, r, int64(nmethods)); err != nil {
		return
	}
	if _, err := client.Write([]byte{5, 0}); err != nil { // 无认证
		return
	}
	if ver, err := r.ReadByte(); err != nil || ver != 5 {
		return
	}
	cmd, err := r.ReadByte()
	if err != nil {
		return
	}
	r.ReadByte() // RSV
	atyp, err := r.ReadByte()
	if err != nil {
		return
	}
	if cmd != 1 { // 仅 CONNECT
		socksReply(client, 7)
		return
	}
	var host string
	switch atyp {
	case 1:
		b := make([]byte, 4)
		if _, err := io.ReadFull(r, b); err != nil {
			return
		}
		host = net.IP(b).String()
	case 3:
		l, err := r.ReadByte()
		if err != nil {
			return
		}
		b := make([]byte, l)
		if _, err := io.ReadFull(r, b); err != nil {
			return
		}
		host = string(b)
	case 4:
		b := make([]byte, 16)
		if _, err := io.ReadFull(r, b); err != nil {
			return
		}
		host = net.IP(b).String()
	default:
		socksReply(client, 8)
		return
	}
	pb := make([]byte, 2)
	if _, err := io.ReadFull(r, pb); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(pb)
	target := fmt.Sprintf("%s:%d", host, port)

	if !whitelistedTarget(target) {
		log.Printf("[COORD] 拒绝非白名单目标 %s", target)
		socksReply(client, 2)
		return
	}

	var round uint32
	n := c.pickNode(&round)
	if n == nil {
		// 诊断：打印在线表状态，排查"onlineCount>0 但 pickNode 返回 nil"
		c.mu.Lock()
		nc := len(c.nodes)
		var per []int
		for _, nn := range c.nodes {
			nn.mu.Lock()
			per = append(per, len(nn.cids))
			nn.mu.Unlock()
		}
		c.mu.Unlock()
		log.Printf("[COORD] 无可用节点，拒绝 %s (nodes=%d, cids=%v)", target, nc, per)
		socksReply(client, 5)
		return
	}

	c.mu.Lock()
	c.nextCID++
	cid := c.nextCID
	w := &work{client: client, target: target, done: make(chan struct{})}
	c.pending[cid] = w
	c.mu.Unlock()

	cmdJSON, _ := json.Marshal(coordMsg{Op: "connect", Cid: cid, Target: target})
	if err := c.sendNodeFrame(n, frameControl, cmdJSON); err != nil {
		c.mu.Lock()
		delete(c.pending, cid)
		c.mu.Unlock()
		socksReply(client, 5)
		return
	}

	select {
	case <-w.done: // connect_res 已处理并启动转发；连接由 relayClientToNode 接管关闭
		handedOff = true
	case <-time.After(connectTimeout + 5*time.Second):
		c.mu.Lock()
		delete(c.pending, cid)
		c.mu.Unlock()
		socksReply(client, 5)
	}
}

func socksReply(c net.Conn, code byte) {
	c.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
}

// livenessSweeper 心跳超时踢掉死节点。
func (c *coordinator) livenessSweeper() {
	for range time.NewTicker(coordHBSweep).C {
		now := time.Now()
		c.mu.Lock()
		for _, n := range c.nodes {
			n.mu.Lock()
			stale := now.Sub(n.lastHB) > coordHBTimeout
			n.mu.Unlock()
			if stale {
				log.Printf("[COORD] 节点 %s 心跳超时，断开", n.id)
				n.conn.Close()
			}
		}
		c.mu.Unlock()
	}
}

// Start 启动协调者：监听节点隧道 + 本地 SOCKS5。tunAddr 例 ":8443"，socksAddr 例 "127.0.0.1:19003"。
func (c *coordinator) Start(tunAddr, socksAddr string) error {
	go c.livenessSweeper()

	ln, err := net.Listen("tcp", tunAddr)
	if err != nil {
		return fmt.Errorf("节点隧道监听失败 %s: %v", tunAddr, err)
	}
	log.Printf("[COORD] 节点隧道监听 %s", tunAddr)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				continue
			}
			if tc, ok := conn.(*net.TCPConn); ok {
				tc.SetKeepAlive(true)
				tc.SetKeepAlivePeriod(30 * time.Second)
			}
			go c.handleNode(conn)
		}
	}()

	s, err := net.Listen("tcp", socksAddr)
	if err != nil {
		return fmt.Errorf("本地 SOCKS5 监听失败 %s: %v", socksAddr, err)
	}
	log.Printf("[COORD] 本地 SOCKS5 %s", socksAddr)
	go func() {
		for {
			conn, err := s.Accept()
			if err != nil {
				continue
			}
			go c.handleSOCKS(conn)
		}
	}()
	return nil
}

// startCoordinator 由 main() 调用，按全局端口启动协调者。
func startCoordinator() error {
	return p2pCoord.Start(fmt.Sprintf(":%d", p2pTunPort), fmt.Sprintf("127.0.0.1:%d", p2pSocksPort))
}
