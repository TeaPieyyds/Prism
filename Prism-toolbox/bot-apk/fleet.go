package main

import (
	"fmt"
	"sync"
	"time"

	"bot-apk/config"
)

// FleetBotStatus 单个机器人的连接状态，供前端展示。
type FleetBotStatus struct {
	Index     int    `json:"index"`
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
	IsOP      bool   `json:"is_op"`
	Error     string `json:"error,omitempty"`
}

// Fleet 多机器人舰队管理器。
type Fleet struct {
	mu   sync.Mutex
	Main *BotManager        // 主机器人（使用主令牌）
	Subs []*BotManager      // 子机器人
	stat []FleetBotStatus
	// subNames 已连接的子机器人显示名集合，用于主机器人识别"请给予权限"请求来自哪个子机器人。
	// 与 Subs 分开维护：子机器人可能在加入 Subs 前就发送权限请求，此时按名字也能匹配。
	subNames map[string]bool
}

var fleet Fleet

// FleetConnect 连接所有子机器人（主机器人应已先连接好）。
// 每个子机器人的连接进度通过 hub.Emit("fleet_progress", ...) 推送。
// 主机器人自动 /op 每个成功连接的子机器人。
func FleetConnect(cfg *config.AppConfig) {
	fleet.mu.Lock()
	fleet.Main = botMgr
	fleet.Subs = nil
	fleet.subNames = make(map[string]bool)
	total := len(cfg.BotTokens)
	fleet.stat = make([]FleetBotStatus, total)
	for i := range fleet.stat {
		fleet.stat[i].Index = i
	}
	fleet.mu.Unlock()

	hub.Emit("fleet_progress", mustJSON(map[string]any{
		"total": total, "connected": 0, "msg": "准备连接子机器人...",
	}))

	// 进服进度：以 onReady（真正权限确认）为准累计已加入数。多子机器人 onReady 由各自
	// goroutine 触发，可能乱序，用互斥保护。主机器人负责游戏内 actionbar + 消息。
	var jmu sync.Mutex
	joined := 0
	joinedPlus := func() int {
		jmu.Lock()
		defer jmu.Unlock()
		joined++
		return joined
	}
	joinedCount := func() int {
		jmu.Lock()
		defer jmu.Unlock()
		return joined
	}
	fleetActionbar := func() {
		main := fleet.Main
		if main == nil || !main.IsConnected() {
			return
		}
		cur := joinedCount()
		pct := 0
		if total > 0 {
			pct = cur * 100 / total
		}
		if pct > 99 {
			pct = 99
		}
		bar := buildParBar(pct*14/100, 14, "a", "7")
		main.SendWOCmd(fmt.Sprintf(`titleraw @a actionbar {"rawtext":[{"text":"§r§c导入中  §dprism  §7┃  §f子机器人进服\n§l§d> §7已加入 %s  §a§l%d%%\n§r§7%d/%d（还要 %d 个）"}]}`, bar, pct, cur, total, total-cur))
	}
	fleetTell := func(msg string) {
		main := fleet.Main
		if main != nil && main.IsConnected() {
			main.SendTellraw(msg)
		}
	}

	for i, token := range cfg.BotTokens {
		bm := NewBotManager()
		bm.BotIndex = i + 1

		// 转发子机器人连接日志到终端，方便排查。
		// 只报连接阶段的关键日志，过滤掉游戏内消息（聊天/加入/退出等）——
		// 那些由主机器人统一报，子机器人不重复打印。
		go func(bot *BotManager, idx int) {
			for msg := range bot.logCh {
				if isGameInternalLog(msg) {
					continue
				}
				hub.Emit("bot_log", mustJSON(map[string]any{
					"msg": fmt.Sprintf("[机器人%d] %s", idx, msg),
				}))
			}
		}(bm, i+1)

		// 设置 onReady：子机器人权限确认后，更新状态 + 主机器人游戏内报"第N个已加入" + 更新进度条
		botIdx := i
		bm.onReady = func() {
			n := joinedPlus()
			fleet.mu.Lock()
			if botIdx < len(fleet.stat) {
				fleet.stat[botIdx].IsOP = true
			}
			fleet.mu.Unlock()
			hub.Emit("fleet_progress", mustJSON(map[string]any{
				"total": total, "connected": n, "msg": fmt.Sprintf("子机器人 %d 权限已确认", botIdx+1),
			}))
			name := ""
			if bm.conn != nil {
				name = bm.conn.IdentityData().DisplayName
			}
			fleetTell(fmt.Sprintf("§a第%d个子机器人 §f%s §a已成功加入§r", n, name))
			fleetActionbar()
		}

		// 连接 + 外层重试：登录失败时主机器人报"第X次重试"，3 次后仍失败则报失败原因。
		const retries = 3
		var err error
		for attempt := 1; attempt <= retries; attempt++ {
			hub.Emit("fleet_progress", mustJSON(map[string]any{
				"total": total, "connected": i, "msg": fmt.Sprintf("连接子机器人 %d/%d（第 %d 次尝试）...", i+1, total, attempt),
			}))
			fleetTell(fmt.Sprintf("§e第%d个子机器人，第%d次尝试加入...§r", i+1, attempt))
			err = bm.Connect(token, cfg.ServerCode, cfg.AuthURL, cfg.ServerPass)
			if err == nil {
				break
			}
			if attempt < retries {
				bm.Disconnect()
				fleetTell(fmt.Sprintf("§c第%d个子机器人加入失败：%v，进行第%d次重试§r", i+1, err, attempt))
				time.Sleep(1 * time.Second)
			}
		}
		if err != nil {
			fleet.mu.Lock()
			if i < len(fleet.stat) {
				fleet.stat[i].Error = err.Error()
			}
			fleet.mu.Unlock()
			fleetTell(fmt.Sprintf("§c第%d个子机器人重试%d次后仍失败：%v§r", i+1, retries, err))
			hub.Emit("fleet_progress", mustJSON(map[string]any{
				"total": total, "connected": i, "msg": fmt.Sprintf("子机器人 %d 连接失败: %v", i+1, err),
			}))
			continue
		}

		// 记录连接状态
		fleet.mu.Lock()
		fleet.Subs = append(fleet.Subs, bm)
		if fleet.subNames == nil {
			fleet.subNames = make(map[string]bool)
		}
		fleet.subNames[bm.conn.IdentityData().DisplayName] = true
		fleet.stat[i].Connected = true
		fleet.stat[i].Name = bm.conn.IdentityData().DisplayName
		fleet.mu.Unlock()

		// 主机器人给子机器人 OP
		grantOPToFleet()

		hub.Emit("fleet_progress", mustJSON(map[string]any{
			"total": total, "connected": i + 1, "msg": fmt.Sprintf("子机器人 %d %s 已进服", i+1, bm.conn.IdentityData().DisplayName),
		}))
	}

	// 全部完成
	fleet.mu.Lock()
	mainOP := fleet.Main != nil && fleet.Main.IsOP()
	connected := fleetConnectedCountLocked()
	fleet.mu.Unlock()

	if mainOP {
		// 主机器人已有权限，再补发一次 OP 给所有子机器人（保证刚连上的也能拿到）
		fleet.mu.Lock()
		for _, bm := range fleet.Subs {
			if bm != nil && bm.IsConnected() {
				fleet.Main.SendCommand(fmt.Sprintf("/op %s", bm.conn.IdentityData().DisplayName))
			}
		}
		fleet.mu.Unlock()
	}

	// 全部接入完成：主机器人游戏内报完成，并清除进服进度条（避免残留覆盖后续导入 actionbar）。
	fleetTell(fmt.Sprintf("§a全部 %d 个子机器人接入完成（成功 %d）§r", total, connected))
	main := fleet.Main
	if main != nil && main.IsConnected() {
		main.SendWOCmd(`titleraw @a actionbar {"rawtext":[{"text":""}]}`)
	}

	hub.Emit("fleet_ready", mustJSON(map[string]any{
		"total": total, "connected": connected,
	}))
}

// fleetLog 向主机器人 logCh 安全地发一条日志：非阻塞、不 panic。
// 避免 logCh 已关闭或阻塞时，舰队逻辑卡死或崩溃主机器人。
func fleetLog(main *BotManager, msg string) {
	if main == nil || main.logCh == nil {
		return
	}
	defer func() { recover() }()
	select {
	case main.logCh <- msg:
	default:
	}
}

// grantOPToFleet 主机器人给所有已连接子机器人发送 /op。
// 在主机器人 onReady（拿到权限）时和 FleetConnect 结束后调用，确保子机器人能拿到权限。
func grantOPToFleet() {
	fleet.mu.Lock()
	main := fleet.Main
	var subs []*BotManager
	subs = append(subs, fleet.Subs...)
	fleet.mu.Unlock()

	if main == nil || !main.IsConnected() || !main.IsOP() {
		fleetLog(main, "[舰队] 主机器人暂无权限，暂不授予子机器人 OP")
		return
	}
	for _, bm := range subs {
		if bm != nil && bm.IsConnected() && bm.conn != nil {
			main.SendCommand(fmt.Sprintf("/op %s", bm.conn.IdentityData().DisplayName))
			fleetLog(main, fmt.Sprintf("[舰队] 已授予子机器人 %s OP", bm.conn.IdentityData().DisplayName))
		}
	}
}

// grantOPIfSubBot 当主机器人收到子机器人的"请给予权限"请求时，授予该子机器人 OP。
// senderName 是发送请求的玩家名。
func grantOPIfSubBot(senderName string) {
	fleet.mu.Lock()
	main := fleet.Main
	isSub := fleet.subNames != nil && fleet.subNames[senderName]
	// 也检查 Subs 里是否匹配（子机器人可能在加入 subNames 前就发送了请求）
	if !isSub {
		for _, bm := range fleet.Subs {
			if bm != nil && bm.conn != nil && bm.conn.IdentityData().DisplayName == senderName {
				isSub = true
				break
			}
		}
	}
	fleet.mu.Unlock()

	if main == nil || !main.IsConnected() || !main.IsOP() {
		return
	}
	if isSub {
		main.SendCommand(fmt.Sprintf("/op %s", senderName))
		fleetLog(main, fmt.Sprintf("[舰队] 已授予子机器人 %s OP", senderName))
	}
}

// FleetDisconnect 断开所有子机器人。
func FleetDisconnect() {
	fleet.mu.Lock()
	defer fleet.mu.Unlock()
	for _, bm := range fleet.Subs {
		bm.Disconnect()
	}
	fleet.Subs = nil
	fleet.stat = nil
	hub.Emit("fleet_progress", mustJSON(map[string]any{
		"total": 0, "connected": 0, "msg": "子机器人已断开",
	}))
}

// FleetStatus 返回所有子机器人的状态。
func FleetStatus() []FleetBotStatus {
	fleet.mu.Lock()
	defer fleet.mu.Unlock()
	result := make([]FleetBotStatus, len(fleet.stat))
	copy(result, fleet.stat)
	return result
}

// fleetConnectedCount 返回已连接的子机器人数量（需持有锁）。
func fleetConnectedCountLocked() int {
	n := 0
	for _, s := range fleet.stat {
		if s.Connected {
			n++
		}
	}
	return n
}

// GetFleetBots 返回所有已连接的机器人（主 + 子），用于 MultiBotManager。
func GetFleetBots() []*BotManager {
	fleet.mu.Lock()
	defer fleet.mu.Unlock()
	var bots []*BotManager
	if fleet.Main != nil && fleet.Main.IsConnected() {
		bots = append(bots, fleet.Main)
	}
	for _, bm := range fleet.Subs {
		if bm.IsConnected() {
			bots = append(bots, bm)
		}
	}
	return bots
}