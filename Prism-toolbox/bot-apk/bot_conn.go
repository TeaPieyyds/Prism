package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"bot-apk/config"

	"github.com/OmineDev/flowers-for-machines/client"
	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/py_rpc"
	cts "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server"
	cts_mc "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft"
	cts_mc_a "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft/ai_command"
	cts_mc_p "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft/preset"
	cts_mc_v "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft/vip_event_system"
	mei "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/interface"
	"github.com/OmineDev/flowers-for-machines/game_control/game_interface"
	"github.com/OmineDev/flowers-for-machines/game_control/resources_control"
	"github.com/OmineDev/flowers-for-machines/nbt_assigner"
	"github.com/OmineDev/flowers-for-machines/nbt_assigner/nbt_console"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
)

// BotManager holds the active Minecraft connection
type BotManager struct {
	mu               sync.Mutex
	client           *client.Client
	conn             *minecraft.Conn
	resources        *resources_control.Resources
	gameInterface    *game_interface.GameInterface
	pktCh            chan packet.Packet
	listenerID       string
	connected        bool
	isOP             bool
	opSent           bool
	verifyPending    bool // true when waiting for gamemode response to confirm OP
	cmdPerm          uint32
	display          string
	xuid             string
	eid              int64
	runtimeID        uint64
	idStr            string
	pos              mgl32.Vec3
	serverCode       string
	botComponent     map[string]*int // 服务器模组列表 (mod UUID → outfit type)
	connectTime      time.Time       // when Connect() completed — used to suppress early false-OP packets
	sayTestMsg       string          // /say 测试消息，非空表示正在等待回显确认权限
	stopCh           chan struct{}
	logCh            chan string
	onReady          func()        // 权限确认后回调，用于直接发 SSE 事件
	readyCh          chan struct{} // closed when onReady fires, signals retry to stop
	nbtAssigner      *nbt_assigner.NBTAssigner
	nbtConsole       *nbt_console.Console
	commandDimension string        // 命令维度（非 overworld 时自动加 execute in 前缀）
	wsCmdRespCh      chan struct{} // WS 命令收包同步

	// 飞行控制状态
	pitch         float32
	yaw           float32
	isFlying      bool
	flyRequested  bool // 用户请求的飞行状态，不被 UpdateAbilities 覆盖
	moveTicker    *time.Ticker
	moveStopCh    chan struct{}
	moveDirection mgl32.Vec3 // 当前持续移动方向
	moveVec       mgl32.Vec2 // 心跳中携带的 MoveVector（持续移动时用）
	moveFlags     uint32     // 心跳中携带的输入标志位（InputFlagUp/Down/Left/Right）
	jumpPressed   bool       // 跳跃键是否按下（心跳持续带跳跃标志）
	sneakPressed  bool       // 潜行键是否按下
	sprintPressed bool       // 奔跑键是否按下

	// 游戏心跳（PlayerAuthInput 定时发送）
	heartbeatStopCh  chan struct{}
	heartbeatWriteMu sync.Mutex // 保护心跳写入，避免并发写导致阻塞
	tick             uint64
	heartbeatEnabled bool // 前端打开飞行/寻路面板时启用，关闭时静默

	// SubChunkRequest 并发限流（最多 4 个同时进行）
	chunkReqSem chan struct{}

	// SSE 位置推送（避免重复发送相同位置）
	lastReportedPos mgl32.Vec3

	// 服务器权威位置
	dimension int32 // 当前维度（0=主世界, 1=下界, 2=末地）

	// 聊天消息限流
	chatMu       sync.Mutex // 保护聊天发送串行化
	lastChatTime time.Time  // 上次发送聊天消息的时间

	// BotIndex 该机器人在多机器人舰队中的编号（0 为主/单机）。用于跨机器人隔离：
	// structure 名前缀（§8.2）与 NBT 工作台坐标分配（§8.3）都依赖它保持唯一。
	BotIndex int
}

var (
	botMgr   *BotManager
	botMgrMu sync.Mutex
)

func NewBotManager() *BotManager {
	return &BotManager{
		stopCh:      make(chan struct{}),
		readyCh:     make(chan struct{}),
		logCh:       make(chan string, 100),
		chunkReqSem: make(chan struct{}, 4),
	}
}

func (bm *BotManager) Connect(token, server, auth, password string) error {
	// 检测本地联机前缀：@房间号 → 走 TanLobby 中继加入
	// #房间号 → 普通联机房间，走租赁服通道
	if strings.HasPrefix(server, "@") {
		roomID := strings.TrimPrefix(server, "@")
		return bm.connectTanLobby(token, roomID, auth, password)
	}

	bm.mu.Lock()
	if bm.connected {
		bm.disconnectLocked()
	}
	bm.mu.Unlock()

	cfg := client.Config{
		AuthServerAddress:    auth,
		RentalServerCode:     server,
		RentalServerPasscode: password,
		AuthServerToken:      token,
		OnLoginProgress: func(msg string) {
			select {
			case bm.logCh <- fmt.Sprintf("  ├ %s", msg):
			default:
			}
		},
	}

	bm.logCh <- fmt.Sprintf("⛏ 正在连接 %s ...", server)

	const maxRetries = 3
	var c *client.Client
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-bm.stopCh:
			bm.logCh <- fmt.Sprintf("  └→ 连接已取消")
			return fmt.Errorf("connection cancelled")
		default:
		}
		bm.logCh <- fmt.Sprintf("  └→ 第%d次尝试连接...", attempt)
		c, err = client.LoginRentalServer(cfg)
		if err == nil {
			break
		}
		if attempt < maxRetries {
			bm.logCh <- fmt.Sprintf("  └→ 失败: %v，1秒后重试", err)
			time.Sleep(1 * time.Second)
		}
	}
	if err != nil {
		select {
		case <-bm.stopCh:
			bm.logCh <- fmt.Sprintf("  └→ 连接已取消")
			return fmt.Errorf("connection cancelled")
		default:
		}
		bm.logCh <- fmt.Sprintf("✗ 登录失败(已重试%d次): %v", maxRetries, err)
		return fmt.Errorf("login: %w", err)
	}

	conn := c.Conn()
	id := conn.IdentityData()
	bm.logCh <- fmt.Sprintf("✓ 已连接到服务器! 玩家: %s", id.DisplayName)
	bm.logCh <- fmt.Sprintf("  ├ 服务器代码: %s", server)
	bm.logCh <- fmt.Sprintf("  ├ EID: %d", conn.GameData().EntityUniqueID)
	bm.logCh <- fmt.Sprintf("  └ XUID: %s", id.XUID)

	bm.mu.Lock()
	bm.client = c
	bm.conn = conn
	bm.display = id.DisplayName
	bm.xuid = id.XUID
	bm.eid = int64(conn.GameData().EntityUniqueID)
	bm.runtimeID = conn.GameData().EntityRuntimeID
	bm.idStr = id.Identity
	bm.pos = conn.GameData().PlayerPosition
	bm.serverCode = server
	bm.botComponent = c.BotComponent
	bm.connectTime = time.Now()
	bm.stopCh = make(chan struct{})
	bm.connected = true
	bm.mu.Unlock()

	// 把自己注册到 players map
	opMu.Lock()
	players[bm.display] = bm.eid
	opMu.Unlock()

	bm.logCh <- "[*] 启动包监听器..."
	res := resources_control.NewResourcesControl(c) // 自动排空登录缓存 + 自动回复网易心跳/传送/重生
	bm.mu.Lock()
	bm.resources = res
	bm.pktCh = make(chan packet.Packet, 256)
	bm.mu.Unlock()
	go bm.consumePackets()
	bm.registerPacketHandlers()
	bm.logCh <- "  └→ 监听器已启动"

	// OP 确认轮询：检测到权限后发送就绪通知，无权限时定期请求授权
	// Wrap onReady to signal readyCh on initialization complete
	origReady := bm.onReady
	bm.onReady = func() {
		close(bm.readyCh)
		// 权限就绪后立即初始化动作接口（切创造+飞行），提前暴露问题。
		// 同步执行确保任务开始前已完成，避免任务中触发 getGameInterface 卡住心跳。
		// getGameInterface 首次失败时带重试（ensureGameInterfaceWithRetry），
		// 避免授权后首次初始化竞态导致动作接口缺失、NBT/告示牌被静默跳过。
		// 仍失败则放行（不阻塞连接），仅需动作接口的方块会跳过。
		bm.ensureGameInterfaceWithRetry()
		// 启动玩家列表刷新（网易租赁服不发 PlayerList 包，用命令替代）
		go func() {
			time.Sleep(2 * time.Second)
			playerInfoMgr.RefreshPlayerList(bm)
			bm.logCh <- fmt.Sprintf("[玩家] 已刷新玩家列表: %d 人", playerInfoMgr.GetOnlineCount())
		}()
		if origReady != nil {
			origReady()
		}
	}

	go bm.opConfirmLoop(conn)

	// 山头服（服务器号含非数字）需要额外模组同步
	if hasNonDigit(server) {
		bm.logCh <- "[*] 检测到山头服，发送模组同步..."
		go bm.sendModSync(conn)
	}

	// 启动游戏心跳（PlayerAuthInput 定时发送）
	// 在服务器权威移动模式下，客户端必须定期发送 PlayerAuthInput
	// 否则服务器会忽略位置变化或回退位置
	bm.startHeartbeat()
	bm.heartbeatEnabled = true // 默认开启，前端关面板时再关闭

	// 初始化寻路器
	InitPathfinder(bm)

	// 欢迎语（聊天消息）— 异步发送，不阻塞 Connect 返回
	// 尝试使用工具箱自定义提示词，如无则使用默认文本
	go func() {
		time.Sleep(500 * time.Millisecond)
		prompt := GetEffectivePrompt()
		if prompt == "" {
			prompt = fmt.Sprintf("§e§l▍prism 工具箱 §fv%s\n§7网站: §bprism.adblanlu.qzz.io §7Q群: §b569823467\n§8©2026 @adb_lanlu", shortVersion())
		}
		bm.sendChatMessage(prompt)

		// 匿名模式警告
		cfg := config.ReloadConfig()
		if cfg == nil || cfg.Token == "" {
			time.Sleep(600 * time.Millisecond)
			bm.sendChatMessage("§c§l▍ 匿名模式 §r§7功能受限 · §e谨慎给权 §a· 推荐登录")
		}
	}()

	// Wait for ready or disconnect. If disconnected before ready, retry.
	// 子机器人（BotIndex>0）无权限时不能永久阻塞等待 OP——那会让 FleetConnect 卡在
	// 第一个子机器人、后面的永远连不上。子机器人超时后先返回，OP 由主机器人稍后授予
	// （授予后 opConfirmLoop 仍在后台运行，检测到权限会再触发 onReady）。
	if bm.BotIndex > 0 {
		select {
		case <-bm.readyCh:
			return nil
		case <-bm.stopCh:
			bm.logCh <- fmt.Sprintf("  └→ 初始化阶段断开，重试")
			bm.mu.Lock()
			bm.disconnectLocked()
			bm.mu.Unlock()
			return fmt.Errorf("disconnected during init")
		case <-time.After(3 * time.Second):
			bm.logCh <- "  └→ 子机器人无权限，先返回（等待主机器人授予 OP）"
			return nil
		}
	}
	// 主机器人：等待 OP（无超时，防止无权限时误报连接失败）
	select {
	case <-bm.readyCh:
		return nil
	case <-bm.stopCh:
		bm.logCh <- fmt.Sprintf("  └→ 初始化阶段断开，重试")
		bm.mu.Lock()
		bm.disconnectLocked()
		bm.mu.Unlock()
		return fmt.Errorf("disconnected during init")
	}
}

// connectTanLobby 通过 TanLobby 中继加入本地联机房间。
// server 为房间号，password 为房间密码（无密码则空）。
func (bm *BotManager) connectTanLobby(token, roomID, auth, password string) error {
	bm.mu.Lock()
	if bm.connected {
		bm.disconnectLocked()
	}
	bm.mu.Unlock()

	cfg := client.Config{
		AuthServerAddress:    auth,
		RentalServerCode:     roomID,
		RentalServerPasscode: password,
		AuthServerToken:      token,
		OnLoginProgress: func(msg string) {
			select {
			case bm.logCh <- fmt.Sprintf("  ├ %s", msg):
			default:
			}
		},
	}

	bm.logCh <- fmt.Sprintf("[*] 正在加入房间 %s ...", roomID)

	const maxRetries = 5
	var c *client.Client
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-bm.stopCh:
			bm.logCh <- fmt.Sprintf("  └→ 连接已取消")
			return fmt.Errorf("connection cancelled")
		default:
		}
		bm.logCh <- fmt.Sprintf("  └→ 第%d次尝试连接...", attempt)
		c, err = client.LoginTanLobbyRoom(cfg)
		if err == nil {
			break
		}
		if attempt < maxRetries {
			bm.logCh <- fmt.Sprintf("  └→ 失败: %v，1秒后重试", err)
			time.Sleep(1 * time.Second)
		}
	}
	if err != nil {
		select {
		case <-bm.stopCh:
			bm.logCh <- fmt.Sprintf("  └→ 连接已取消")
			return fmt.Errorf("connection cancelled")
		default:
		}
		bm.logCh <- fmt.Sprintf("✗ 加入房间失败(已重试%d次): %v", maxRetries, err)
		return fmt.Errorf("login tanlobby: %w", err)
	}

	conn := c.Conn()
	id := conn.IdentityData()
	bm.logCh <- fmt.Sprintf("✓ 已加入房间! 玩家: %s", id.DisplayName)
	bm.logCh <- fmt.Sprintf("  ├ 房间ID: %s", roomID)
	bm.logCh <- fmt.Sprintf("  ├ EID: %d", conn.GameData().EntityUniqueID)
	bm.logCh <- fmt.Sprintf("  └ XUID: %s", id.XUID)

	bm.mu.Lock()
	bm.client = c
	bm.conn = conn
	bm.display = id.DisplayName
	bm.xuid = id.XUID
	bm.eid = int64(conn.GameData().EntityUniqueID)
	bm.runtimeID = conn.GameData().EntityRuntimeID
	bm.idStr = id.Identity
	bm.pos = conn.GameData().PlayerPosition
	bm.serverCode = roomID
	bm.botComponent = c.BotComponent
	bm.connectTime = time.Now()
	bm.stopCh = make(chan struct{})
	bm.connected = true
	bm.mu.Unlock()

	// 把自己注册到 players map
	opMu.Lock()
	players[bm.display] = bm.eid
	opMu.Unlock()

	bm.logCh <- "[*] 启动包监听器..."
	res := resources_control.NewResourcesControl(c)
	bm.mu.Lock()
	bm.resources = res
	bm.pktCh = make(chan packet.Packet, 256)
	bm.mu.Unlock()
	go bm.consumePackets()
	bm.registerPacketHandlers()
	bm.logCh <- "  └→ 监听器已启动"

	// OP 确认轮询
	origReady := bm.onReady
	bm.onReady = func() {
		close(bm.readyCh)
		// 动作接口初始化：失败时带重试，避免首次初始化竞态导致 NBT/告示牌被静默跳过。
		// 仍失败则放行（不阻塞连接），仅需动作接口的方块会跳过。
		bm.ensureGameInterfaceWithRetry()
		go func() {
			time.Sleep(2 * time.Second)
			playerInfoMgr.RefreshPlayerList(bm)
			bm.logCh <- fmt.Sprintf("[玩家] 已刷新玩家列表: %d 人", playerInfoMgr.GetOnlineCount())
		}()
		if origReady != nil {
			origReady()
		}
	}

	go bm.opConfirmLoop(conn)

	// 山头服（房间号含非数字）需要额外模组同步
	if hasNonDigit(roomID) {
		bm.logCh <- "[*] 检测到山头服，发送模组同步..."
		go bm.sendModSync(conn)
	}

	bm.startHeartbeat()
	bm.heartbeatEnabled = true
	InitPathfinder(bm)

	go func() {
		time.Sleep(500 * time.Millisecond)
		prompt := GetEffectivePrompt()
		if prompt == "" {
			prompt = fmt.Sprintf("§e§l▍prism 工具箱 §fv%s\n§7网站: §bprism.adblanlu.qzz.io §7Q群: §b569823467\n§8©2026 @adb_lanlu", shortVersion())
		}
		bm.sendChatMessage(prompt)
		cfg := config.ReloadConfig()
		if cfg == nil || cfg.Token == "" {
			time.Sleep(600 * time.Millisecond)
			bm.sendChatMessage("§c§l▍ 匿名模式 §r§7功能受限 · §e谨慎给权 §a· 推荐登录")
		}
	}()

	select {
	case <-bm.readyCh:
		return nil
	case <-bm.stopCh:
		bm.logCh <- fmt.Sprintf("  └→ 初始化阶段断开，重试")
		bm.mu.Lock()
		bm.disconnectLocked()
		bm.mu.Unlock()
		return fmt.Errorf("disconnected during init")
	}
}

// registerPacketHandlers 订阅关心的包类型，收到后塞入 pktCh，由 consumePackets 顺序消费。
// 只订阅需要的包，避免海量区块包触发大量回调 goroutine（flowers 每包每监听器起一个 goroutine）。
func (bm *BotManager) registerPacketHandlers() {
	ids := []uint32{
		packet.IDText, packet.IDDisconnect, packet.IDRespawn,
		packet.IDCommandOutput, packet.IDStructureTemplateDataResponse,
		packet.IDUpdateAbilities, packet.IDAdventureSettings,
		packet.IDAddPlayer, packet.IDPlayerList,
		packet.IDLevelChunk, packet.IDSubChunk,
		packet.IDPlayerLocation, packet.IDMovePlayer,
		packet.IDMobEquipment, packet.IDChangeDimension,
	}
	id, err := bm.resources.PacketListener().ListenPacket(ids, func(pk packet.Packet, connCloseErr error) {
		if connCloseErr != nil {
			bm.logCh <- fmt.Sprintf("✗ 连接断开: %v", connCloseErr)
			bm.Disconnect()
			return
		}
		select {
		case bm.pktCh <- pk:
		case <-bm.stopCh:
		}
	})
	if err != nil {
		bm.logCh <- fmt.Sprintf("✗ 注册包监听器失败: %v", err)
		return
	}
	bm.listenerID = id
}

// consumePackets 单 goroutine 顺序消费 pktCh，保持与原读包循环一致的处理时序。
func (bm *BotManager) consumePackets() {
	defer func() {
		if r := recover(); r != nil {
			select {
			case bm.logCh <- fmt.Sprintf("✗ 数据包处理panic: %v", r):
			default:
			}
		}
	}()
	for {
		var pk packet.Packet
		select {
		case <-bm.stopCh:
			return
		case pk = <-bm.pktCh:
		}

		// 追踪 OP 状态（仅记录）
		func() {
			defer func() { recover() }()
			HandleOPPacket(pk, bm.logCh)
		}()
		// 追踪玩家信息（在线列表、位置、手持物品、权限）
		func() {
			defer func() { recover() }()
			bm.HandlePlayerPacket(pk)
		}()

		switch p := pk.(type) {
		case *packet.Text:
			bm.logCh <- fmt.Sprintf("[聊天] %s: %s", p.SourceName, p.Message)
			// /say 回退检测：如果自己发出了测试消息，清除标记通知 opConfirmLoop
			bm.mu.Lock()
			if bm.sayTestMsg != "" && p.SourceName == bm.display && strings.Contains(p.Message, bm.sayTestMsg) {
				bm.sayTestMsg = ""
				bm.logCh <- "[权限] ✓ /say 回显成功: 拥有命令权限"
			}
			bm.mu.Unlock()

		case *packet.Disconnect:
			bm.logCh <- fmt.Sprintf("✗ 被服务器踢出: %s", p.Message)
			bm.Disconnect()
			return

		case *packet.Respawn:
			bm.pos = p.Position
			bm.logCh <- fmt.Sprintf("[重生] 位置: (%.0f, %.0f, %.0f)", p.Position[0], p.Position[1], p.Position[2])

		case *packet.CommandOutput:
			// 用户不需要服务器命令广播日志
			_ = p.OutputMessages
			// /say 回退检测：CommandOutput 成功时清除标记通知 opConfirmLoop
			bm.mu.Lock()
			if bm.sayTestMsg != "" && p.SuccessCount > 0 {
				bm.sayTestMsg = ""
				bm.logCh <- "[权限] ✓ /say CommandOutput 成功: 拥有命令权限"
			}
			bm.mu.Unlock()
			// OP verification: gamemode command succeeds only if bot has OP
			bm.mu.Lock()
			verifying := bm.verifyPending
			bm.mu.Unlock()
			if verifying {
				bm.mu.Lock()
				bm.verifyPending = false
				bm.mu.Unlock()
				if p.SuccessCount > 0 {
					bm.logCh <- "[权限] ✓ 命令验证通过: 拥有管理员权限！"
					bm.setOP(true, 3, "CmdVerified")
					if bm.onReady != nil {
						bm.onReady()
					}
					if bm.conn != nil {
						bm.conn.WritePacket(&packet.SettingsCommand{
							CommandLine:    "tellraw @a {\"rawtext\":[{\"text\":\"§a§l▍成功:§r§f成功完成初始化，请前往控制台下发任务\"}]}",
							SuppressOutput: true,
						})
					}
				} else {
					bm.logCh <- "[权限] ✗ 命令验证失败: 没有管理员权限"
					bm.mu.Lock()
					bm.isOP = false
					bm.opSent = false
					bm.mu.Unlock()
				}
			} else if p.SuccessCount > 0 {
				bm.checkSelfOP()
			}
		case *packet.StructureTemplateDataResponse:
			if ch, ok := structureCallbacks.Load(p.StructureName); ok {
				select {
				case ch.(chan map[string]any) <- p.StructureTemplate:
				default:
				}
			}

		case *packet.UpdateAbilities:
			isMe := p.AbilityData.EntityUniqueID == bm.eid
			if isMe {
				bm.setOP(p.AbilityData.CommandPermissions >= packet.CommandPermissionLevelHost,
					uint32(p.AbilityData.CommandPermissions), "UpdateAbilities")
				// 同步飞行状态
				bm.mu.Lock()
				bm.isFlying = p.AbilityData.Layers[0].Values&protocol.AbilityFlying != 0
				bm.mu.Unlock()
			}

		case *packet.AdventureSettings:
			isMe := p.PlayerUniqueID == bm.eid
			if isMe {
				bm.setOP(p.CommandPermissionLevel >= packet.CommandPermissionLevelHost,
					p.CommandPermissionLevel, "AdventureSettings")
			}

		case *packet.PlayerList:

		case *packet.AddPlayer:
			if p.Username == bm.display {
				bm.mu.Lock()
				bm.pos = p.Position
				bm.mu.Unlock()
				pfLog("AddPlayer: position=(%.2f,%.2f,%.2f)", p.Position[0], p.Position[1], p.Position[2])
				bm.setOP(p.AbilityData.CommandPermissions >= 3,
					uint32(p.AbilityData.CommandPermissions), "AddPlayer")
			}

		case *packet.SetActorData:

		case *packet.MoveActorDelta:

		case *packet.MoveActorAbsolute:

		case *packet.LevelChunk:
			// 只在世界数据收集开启时解析并存储区块数据（前端打开寻路/飞行面板时启用）
			if globalWorld.IsEnabled() {
				globalWorld.ParseLevelChunk(p.Position.X(), p.Position.Z(), p.SubChunkCount, p.RawPayload)
				// 如果服务器使用 SubChunkRequest 模式，发送 SubChunkRequest 获取方块数据
				if p.SubChunkCount == protocol.SubChunkRequestModeLimited || p.SubChunkCount == protocol.SubChunkRequestModeLimitless {
					select {
					case bm.chunkReqSem <- struct{}{}:
						go func(cx, cz int32, dim int32) {
							defer func() { <-bm.chunkReqSem }()
							base := protocol.SubChunkPos{cx, -4, cz}
							offsets := make([]protocol.SubChunkOffset, 24)
							for dy := int8(0); dy < 24; dy++ {
								offsets[dy] = protocol.SubChunkOffset{0, dy, 0}
							}
							bm.WritePacket(&packet.SubChunkRequest{
								Dimension: dim,
								Position:  base,
								Offsets:   offsets,
							})
						}(p.Position.X(), p.Position.Z(), p.Dimension)
					default:
					}
				}
			}

		case *packet.SubChunk:
			if globalWorld.IsEnabled() {
				globalWorld.ParseSubChunk(p)
			}

		case *packet.ChangeDimension:
			bm.mu.Lock()
			bm.dimension = p.Dimension
			bm.pos = p.Position
			bm.mu.Unlock()
		case *packet.NetworkChunkPublisherUpdate:

		case *packet.SyncActorProperty:

		case *packet.PlayerLocation:
			bm.mu.Lock()
			bm.pos = p.Position
			bm.mu.Unlock()

		case *packet.MovePlayer:
			if p.EntityRuntimeID != bm.runtimeID {
				break
			}
			bm.mu.Lock()
			bm.pos = p.Position
			bm.mu.Unlock()

		default:
			// 忽略其他包
		}
	}
}

func (bm *BotManager) opConfirmLoop(conn *minecraft.Conn) {
	// 快速确认：不等包，直接发 WS 命令看返回值
	// WS 命令（AutomationPlayer origin）的 CommandOutput 可靠
	time.Sleep(1500 * time.Millisecond) // 等登录基本完成

	for attempt := 1; attempt <= 2; attempt++ {
		select {
		case <-bm.stopCh:
			return
		default:
		}

		bm.logCh <- fmt.Sprintf("[权限] 验证权限(第%d次)...", attempt)

		// 用 WS 命令通道发 /gamemode，只有 OP 才能执行成功
		cmd := "gamemode creative " + bm.quotedName()
		resp, isTimeout, err := bm.SendWSCommandWithTimeout(cmd, 3*time.Second)

		if err != nil {
			if isTimeout {
				bm.logCh <- fmt.Sprintf("[权限] 命令超时(第%d次)", attempt)
				continue
			}
			// gameInterface 可能未就绪 → 降级：直接发命令包
			bm.logCh <- fmt.Sprintf("[权限] gameInterface 未就绪，降级到原始包检测")
			bm.detectOPLegacy(conn)
			return
		}

		if resp != nil && resp.SuccessCount > 0 {
			bm.logCh <- "[权限] ✓ 命令验证通过: 拥有管理员权限！"
			bm.setOP(true, 3, "CmdVerified")
			bm.sendReadyMessage(conn)
			if bm.onReady != nil {
				bm.onReady()
			}
			return
		}

		bm.logCh <- fmt.Sprintf("[权限] 命令验证失败(第%d次)", attempt)
		time.Sleep(1 * time.Second)
	}

	// 两次都失败 → 进入等待授权模式：只发两遍提示，之后静默检测
	bm.logCh <- "[权限] 权限验证未通过，进入等待授权模式"
	bm.noPermNotifyLoop(conn)
}

// detectOPLegacy 降级到原始包检测 + /say 兜底
func (bm *BotManager) detectOPLegacy(conn *minecraft.Conn) {
	noOPStart := time.Now()
	proactiveDone := false
	sayTestDone := false
	noPermConfirmed := false
	lastNotifyTime := time.Time{}

	for {
		select {
		case <-bm.stopCh:
			return
		default:
		}

		bm.mu.Lock()
		isOP := bm.isOP
		opSent := bm.opSent
		bm.mu.Unlock()

		if isOP && opSent {
			return
		}

		// 从包中检测到 OP 但未验证
		if isOP && !opSent {
			bm.logCh <- "[权限] 检测到OP包，尝试命令验证..."
			cmd := "gamemode creative " + bm.quotedName()
			resp, _, err := bm.SendWSCommandWithTimeout(cmd, 3*time.Second)
			if err == nil && resp != nil && resp.SuccessCount > 0 {
				bm.logCh <- "[权限] ✓ 命令验证通过"
				bm.setOP(true, 3, "CmdVerified")
				bm.sendReadyMessage(conn)
				if bm.onReady != nil {
					bm.onReady()
				}
				return
			}
			bm.mu.Lock()
			bm.isOP = false
			bm.mu.Unlock()
		}

		// 主动验证（无 OP 包时）
		if !proactiveDone && time.Since(noOPStart) > 8*time.Second {
			proactiveDone = true
			bm.logCh <- "[权限] 未检测到OP包，主动发送命令验证..."
			cmd := "gamemode creative " + bm.quotedName()
			resp, _, err := bm.SendWSCommandWithTimeout(cmd, 3*time.Second)
			if err == nil && resp != nil && resp.SuccessCount > 0 {
				bm.logCh <- "[权限] ✓ 主动验证通过"
				bm.setOP(true, 3, "CmdVerified")
				bm.sendReadyMessage(conn)
				if bm.onReady != nil {
					bm.onReady()
				}
				return
			}
			bm.logCh <- "[权限] 主动验证超时，假定无权限"
		}

		// /say 兜底
		if proactiveDone && !sayTestDone && time.Since(noOPStart) > 12*time.Second {
			sayTestDone = true
			bm.logCh <- "[权限] 尝试 /say 测试..."
			testMsg := "Welcome"
			bm.mu.Lock()
			bm.sayTestMsg = testMsg
			bm.mu.Unlock()
			conn.WritePacket(&packet.CommandRequest{
				CommandLine: "say " + testMsg,
				CommandOrigin: protocol.CommandOrigin{
					Origin:         protocol.CommandOriginPlayer,
					UUID:           uuid.MustParse(bm.idStr),
					PlayerUniqueID: bm.eid,
				},
				Version: 105,
			})
			time.Sleep(3 * time.Second)
			bm.mu.Lock()
			stillWaiting := bm.sayTestMsg != ""
			bm.mu.Unlock()
			if !stillWaiting {
				bm.logCh <- "[权限] ✓ /say 回显成功: 拥有基本命令权限"
				bm.setOP(true, 1, "SayTest")
				bm.sendReadyMessage(conn)
				if bm.onReady != nil {
					bm.onReady()
				}
				return
			}
			bm.sayTestMsg = ""
			bm.logCh <- "[权限] /say 未回显，确认无命令权限"
			noPermConfirmed = true
			lastNotifyTime = time.Now()
			bm.sendNoPermNotify(conn)
		}

		// 无权限时：每5秒用权威命令 /say 重新检测权限。
		// 先检测——通过则直接完成初始化，只有未通过才发送求授权消息
		if noPermConfirmed && time.Since(lastNotifyTime) > 5*time.Second {
			lastNotifyTime = time.Now()
			bm.logCh <- "[权限] 重新检测权限..."
			resp, _, err := bm.SendWSCommandWithTimeout("say Welcome", 3*time.Second)
			if err == nil && resp != nil && resp.SuccessCount > 0 {
				bm.logCh <- "[权限] ✓ 命令验证通过: 拥有管理员权限！"
				bm.setOP(true, 3, "CmdVerified")
				bm.sendReadyMessage(conn)
				if bm.onReady != nil {
					bm.onReady()
				}
				return
			}
			bm.sendNoPermNotify(conn)
		}

		time.Sleep(2 * time.Second)
	}
}



// noPermNotifyLoop 无权限等待授权模式：
// 1. 发两遍"本机器人由XXX召唤，请给予权限"（间隔5秒）
// 2. 之后静默，用包检测（UpdateAbilities/AdventureSettings）判断机器人自身 OP，
//    并用原始 /say 回显兜底
// 3. 检测到权限后必定发送"已完成初始化"
func (bm *BotManager) noPermNotifyLoop(conn *minecraft.Conn) {
	// 第一遍提示
	bm.sendNoPermNotify(conn)
	notifyCount := 1

	for {
		select {
		case <-bm.stopCh:
			return
		default:
		}

		time.Sleep(5 * time.Second)

		// 包检测：授予 OP 时服务器一定发 UpdateAbilities/AdventureSettings，
		// HandleOPPacket 已填充 opPlayers[bm.display]，checkSelfOP 据此判断
		bm.checkSelfOP()
		bm.mu.Lock()
		isOP := bm.isOP
		bm.mu.Unlock()
		if isOP {
			bm.logCh <- "[权限] ✓ 包检测到 OP: 拥有管理员权限！"
			bm.setOP(true, 3, "CmdVerified")
			bm.sendReadyMessage(conn)
			if bm.onReady != nil {
				bm.onReady()
			}
			return
		}

		// 原始 /say 回显兜底检测（不依赖 gameInterface）
		bm.logCh <- "[权限] 静默检测权限..."
		testMsg := "Welcome"
		bm.mu.Lock()
		bm.sayTestMsg = testMsg
		bm.mu.Unlock()
		conn.WritePacket(&packet.CommandRequest{
			CommandLine: "say " + testMsg,
			CommandOrigin: protocol.CommandOrigin{
				Origin:         protocol.CommandOriginPlayer,
				UUID:           uuid.MustParse(bm.idStr),
				PlayerUniqueID: bm.eid,
			},
			Version: 105,
		})
		time.Sleep(3 * time.Second)
		bm.mu.Lock()
		stillWaiting := bm.sayTestMsg != ""
		bm.sayTestMsg = ""
		bm.mu.Unlock()

		if !stillWaiting {
			// 检测到权限 → 必定发送"已完成初始化"
			bm.logCh <- "[权限] ✓ /say 回显成功: 拥有命令权限！"
			bm.setOP(true, 1, "SayTest")
			bm.sendReadyMessage(conn)
			if bm.onReady != nil {
				bm.onReady()
			}
			return
		}

		// 未检测到权限，且还没发满两遍
		if notifyCount < 2 {
			bm.sendNoPermNotify(conn)
			notifyCount++
		}
	}
}

func (bm *BotManager) sendReadyMessage(conn *minecraft.Conn) {
	if conn == nil {
		return
	}
	conn.WritePacket(&packet.SettingsCommand{
		CommandLine:    "tellraw @a {\"rawtext\":[{\"text\":\"§a§l▍成功:§r§f成功完成初始化，请前往控制台下发任务\"}]}",
		SuppressOutput: true,
	})
}

// sendChatMessage 发送普通聊天消息，自动限流（每2秒1条）和截断（最长50字符）。
// 只对普通聊天 (packet.TextTypeChat) 生效，指令类不受影响。
func (bm *BotManager) sendChatMessage(msg string) {
	bm.chatMu.Lock()
	defer bm.chatMu.Unlock()

	// 截断到50字符（按 rune 计算，尊重中文等宽字符）
	runes := []rune(msg)
	if len(runes) > 50 {
		runes = runes[:50]
		msg = string(runes)
	}

	// 限流：距离上次发送不足2秒则等待
	elapsed := time.Since(bm.lastChatTime)
	if elapsed < 2*time.Second {
		time.Sleep(2*time.Second - elapsed)
	}

	bm.mu.Lock()
	conn := bm.conn
	display := bm.display
	xuid := bm.xuid
	bm.mu.Unlock()

	if conn == nil {
		return
	}

	conn.WritePacket(&packet.Text{
		TextType:   packet.TextTypeChat,
		Message:    msg,
		SourceName: display,
		XUID:       xuid,
	})
	bm.lastChatTime = time.Now()
}

// sendNoPermNotify 无权限时发送求授权提示消息。受 sendChatMessage 限流保护。
func (bm *BotManager) sendNoPermNotify(conn *minecraft.Conn) {
	if conn == nil {
		return
	}
	creator := getCreatorName()
	bm.sendChatMessage(fmt.Sprintf("§b▍§r本机器人由 %s 召唤，请给予权限", creator))
	bm.logCh <- fmt.Sprintf("[权限] 已发送求授权消息 (召唤者: %s)", creator)
}

func (bm *BotManager) checkSelfOP() {
	opMu.Lock()
	info, ok := opPlayers[bm.display]
	opMu.Unlock()
	if ok && info.CmdPerm >= 3 {
		bm.setOP(true, uint32(info.CmdPerm), "opPlayers")
	}
}

func (bm *BotManager) setOP(op bool, perm uint32, source string) {
	bm.mu.Lock()
	wasOP := bm.isOP
	bm.isOP = op
	bm.cmdPerm = perm
	// opSent 只在命令验证通过(source=CmdVerified)或超时回退(source=TimeoutFallback)时设置
	// 包检测(source=UpdateAbilities/AdventureSettings/opPlayers)不设置 opSent
	markSent := (source == "CmdVerified" || source == "TimeoutFallback")
	if markSent {
		bm.opSent = true
	}
	bm.mu.Unlock()

	if wasOP != op {
		bm.logCh <- fmt.Sprintf("[权限] OP状态变化: isOP=%v cmdPerm=%d (来源:%s)", op, perm, source)
	}
	if op && markSent {
		bm.logCh <- fmt.Sprintf("[权限] ✓ OP确认! cmdPerm=%d (来源:%s)", perm, source)
	}
}

func (bm *BotManager) disconnectLocked() {
	// 0. 先关心跳
	globalWorld.Reset()
	bm.stopHeartbeat()
	// 1. 再关 stopCh → 停止所有生产者（心跳、包监听等）
	if bm.stopCh != nil {
		select {
		case <-bm.stopCh:
		default:
			close(bm.stopCh)
		}
	}
	// 2. 再关 logCh → SSE goroutine 会在排空缓冲后自然退出
	if bm.logCh != nil {
		close(bm.logCh)
		bm.logCh = nil
	}
	if bm.conn != nil {
		bm.conn.Close()
		bm.conn = nil
	}
	bm.connected = false
}

func (bm *BotManager) Disconnect() {
	// 先在锁保护下读取 conn 和必要字段，避免持有锁时调用 WritePacket
	bm.mu.Lock()
	conn := bm.conn
	connected := bm.connected
	display := bm.display
	idStr := bm.idStr
	eid := bm.eid
	bm.mu.Unlock()

	// 发送断开包（不持有 bm.mu，避免 WritePacket 阻塞级联）
	if conn != nil && connected {
		// 1. Tell server we're leaving — no OP required
		conn.WritePacket(&packet.Disconnect{
			Message: "prism工具箱 断开连接",
		})
		// 2. Also try /kick if we have OP — graceful server-side removal
		conn.WritePacket(&packet.CommandRequest{
			CommandLine: "kick " + `"` + display + `"`,
			CommandOrigin: protocol.CommandOrigin{
				Origin:         protocol.CommandOriginPlayer,
				UUID:           uuid.MustParse(idStr),
				PlayerUniqueID: eid,
			},
			Version: 105,
		})
		// Give packets time to flush
		time.Sleep(300 * time.Millisecond)
	}

	// 重新获取锁执行清理
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.disconnectLocked()
}

func (bm *BotManager) IsConnected() bool {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.connected
}

func (bm *BotManager) IsOP() bool {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.isOP
}

func (bm *BotManager) ServerCode() string {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.serverCode
}

// SendConsole sends chat or command input from the console tab.
// Plain text → Text packet. /command → WS Automation channel with response.
// Returns structured result for the HTTP handler to return directly to the frontend,
// avoiding log-channel flooding.
func (bm *BotManager) SendConsole(input string) (map[string]any, error) {
	if len(input) > 0 && input[0] == '/' {
		cmd := input[1:]
		bm.logCh <- fmt.Sprintf("[控制台] %s", input)
		// 用 WS 自动化通道发送命令并等待响应（CommandOriginAutomationPlayer）
		// 返回结果直接走 HTTP 响应，不走 SSE 日志，避免洪水
		resp, isTimeout, err := bm.SendWSCommandWithTimeout(cmd, 10*time.Second)
		if err != nil {
			if isTimeout {
				bm.logCh <- fmt.Sprintf("[控制台] 命令超时: %s", cmd)
				return map[string]any{"ok": true, "note": "命令已发送，等待响应超时（可能已执行）"}, nil
			}
			// gameInterface 未就绪，回退到普通 WS 命令（无响应）
			if err2 := bm.SendWSCommand(cmd); err2 != nil {
				return nil, err2
			}
			bm.logCh <- fmt.Sprintf("[控制台] 命令已发送(无响应模式): %s", cmd)
			return map[string]any{"ok": true, "note": "命令已发送（无响应模式）"}, nil
		}
		// 解析命令输出
		result := map[string]any{"ok": true, "cmd": input}
		messages := make([]string, 0, len(resp.OutputMessages))
		for _, msg := range resp.OutputMessages {
			if msg.Message != "" {
				messages = append(messages, formatCommandMessage(msg.Message, msg.Parameters))
			}
		}
		if len(messages) > 0 {
			result["output"] = messages
		}
		result["success"] = resp.SuccessCount > 0
		result["success_count"] = resp.SuccessCount
		return result, nil
	}
	// 普通聊天消息
	bm.mu.Lock()
	if !bm.connected || bm.conn == nil {
		bm.mu.Unlock()
		return nil, fmt.Errorf("not connected")
	}
	bm.mu.Unlock()
	bm.sendChatMessage(input)
	bm.logCh <- fmt.Sprintf("[聊天] %s", input)
	return map[string]any{"ok": true, "type": "chat"}, nil
}

func (bm *BotManager) SendCommand(cmd string) error {
	return bm.SendAICommand(cmd)
}

var structureCallbacks sync.Map

func (bm *BotManager) RequestStructure(pos [3]int32, size [3]int32, timeout time.Duration) (map[string]any, error) {
	bm.mu.Lock()
	if !bm.connected || bm.conn == nil {
		bm.mu.Unlock()
		return nil, fmt.Errorf("not connected")
	}

	// First save the area as a structure so NBT data (commands, chests, etc.) is included.
	// /structure save must be called before ExportFromSave or the server won't return block NBT.
	name := uuid.New().String()
	safeName := strings.ReplaceAll(name, "-", "")
	saveCmd := fmt.Sprintf(`structure save "%s" %d %d %d %d %d %d`,
		safeName,
		pos[0], pos[1], pos[2],
		pos[0]+size[0]-1, pos[1]+size[1]-1, pos[2]+size[2]-1,
	)
	bm.mu.Unlock()

	// Use WS command to save and wait for completion
	if bm.gameInterface != nil {
		bm.gameInterface.Commands().SendWSCommandWithTimeout(saveCmd, 8*time.Second)
		bm.gameInterface.Commands().AwaitChangesGeneral()
	} else {
		bm.mu.Lock()
		conn := bm.conn
		bm.mu.Unlock()
		if conn != nil {
			conn.WritePacket(&packet.SettingsCommand{
				CommandLine:    saveCmd,
				SuppressOutput: true,
			})
		}
		time.Sleep(1 * time.Second)
	}

	bm.mu.Lock()
	respCh := make(chan map[string]any, 1)
	structureCallbacks.Store(safeName, respCh)
	conn := bm.conn
	eid := bm.eid
	bm.mu.Unlock()

	if conn != nil {
		conn.WritePacket(&packet.StructureTemplateDataRequest{
			StructureName: safeName,
			Position:      protocol.BlockPos{pos[0], pos[1], pos[2]},
			Settings: protocol.StructureSettings{
				PaletteName:               "default",
				IgnoreEntities:            true,
				IgnoreBlocks:              false,
				Size:                      protocol.BlockPos{size[0], size[1], size[2]},
				Offset:                    protocol.BlockPos{0, 0, 0},
				LastEditingPlayerUniqueID: eid,
				Rotation:                  0, Mirror: 0, Integrity: 100, Seed: 0,
				AllowNonTickingChunks: false,
			},
			RequestType: packet.StructureTemplateRequestExportFromSave,
		})
	}

	select {
	case data := <-respCh:
		structureCallbacks.Delete(safeName)
		bm.cleanupStructure(safeName)
		return data, nil
	case <-time.After(timeout):
		structureCallbacks.Delete(safeName)
		bm.cleanupStructure(safeName)
		return nil, fmt.Errorf("timeout waiting for structure data")
	}
}

// cleanupStructure deletes a saved structure from the server to avoid accumulation.
func (bm *BotManager) cleanupStructure(name string) {
	if bm.gameInterface != nil {
		bm.gameInterface.Commands().SendWSCommand(
			fmt.Sprintf(`structure delete "%s"`, name),
		)
	} else {
		bm.mu.Lock()
		conn := bm.conn
		bm.mu.Unlock()
		if conn != nil {
			conn.WritePacket(&packet.SettingsCommand{
				CommandLine:    fmt.Sprintf(`structure delete "%s"`, name),
				SuppressOutput: true,
			})
		}
	}
}

func (bm *BotManager) SendTellraw(msg string) error {
	return bm.SendWOCmd(fmt.Sprintf(`tellraw @a {"rawtext":[{"text":"%s"}]}`, msg))
}

func (bm *BotManager) SendWOCmd(cmd string) error {
	return bm.SendAICommand(cmd)
}

func (bm *BotManager) SendTitleraw(target, mode, text string) error {
	json := fmt.Sprintf(`{"rawtext":[{"text":"%s"}]}`, text)
	cmd := fmt.Sprintf("titleraw %s %s %s", target, mode, json)
	return bm.SendWOCmd(cmd)
}

func (bm *BotManager) TP(x, y, z int) error {
	bm.mu.Lock()
	if !bm.connected {
		bm.mu.Unlock()
		return fmt.Errorf("not connected")
	}
	bm.mu.Unlock()
	return bm.SendAICommand(fmt.Sprintf("tp @s %d %d %d", x, y, z))
}

// quotedName returns the bot's display name wrapped in double quotes
// for use in commands (e.g. "botname" instead of @s).
// display is set once during Connect and never modified — safe to read without lock.
// SendBlockCmd 发送方块操作命令（setblock/fill），走网易 AI 命令通道（PyRpc）。
// AI 命令通道相比普通 CommandRequest 拥有更高权限，在网易租赁服上更可靠。
func (bm *BotManager) SendBlockCmd(cmd string) error {
	return bm.SendAICommand(cmd)
}

// normalizeDimension 将前端维度名转为 Minecraft execute in 可用的维度名。
// 前端传 "nether"，但 Minecraft 用 "the_nether"。
// DM 是网易自定义维度（如 dm3, dm4），保留原样以便 execute in dm3 run 使用。
func normalizeDimension(name string) string {
	lower := strings.ToLower(name)
	switch lower {
	case "nether", "minecraft:nether", "the_nether", "minecraft:the_nether":
		return "the_nether"
	case "end", "the_end", "minecraft:the_end":
		return "the_end"
	}
	// 检查是否为 DM 维度（dm3, dm4, dm5, ...）
	if strings.HasPrefix(lower, "dm") {
		// 保留原样，execute in dm3 run 在网易租赁服上有效
		return lower
	}
	// 其他自定义维度也保留原样
	return lower
}

// dimNameToID 将维度名映射为 MCBE 维度 ID（0=overworld, 1=nether, 2=end）。
// DM 维度（如 dm3）返回对应的数字 ID（3）。
// 供 export_subchunk.go、sign_place.go、task_dispatch.go 等共享使用。
func dimNameToID(name string) int {
	lower := strings.ToLower(name)
	switch lower {
	case "nether", "minecraft:nether", "the_nether", "minecraft:the_nether":
		return 1
	case "end", "the_end", "minecraft:the_end":
		return 2
	}
	// DM 维度：dm3 → 3, dm4 → 4
	if strings.HasPrefix(lower, "dm") {
		var id int
		if n, _ := fmt.Sscanf(lower, "dm%d", &id); n == 1 && id >= 3 {
			return id
		}
	}
	return 0
}

// SetDimension 设置命令执行的维度。非 overworld 时后续命令自动加 execute in 前缀。
func (bm *BotManager) SetDimension(name string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.commandDimension = normalizeDimension(name)
}

// WrapCommandInDimension 在命令前加 execute in <dim> run 前缀（非 overworld 时）。
func (bm *BotManager) WrapCommandInDimension(cmd string) string {
	bm.mu.Lock()
	dim := bm.commandDimension
	bm.mu.Unlock()
	return wrapCommandInDimension(cmd, dim)
}

// WrapCommandInDimensionWithDim 与 WrapCommandInDimension 相同，但不获取锁。
// 供 SendPlayerCommand/SendWSCommand 在已释放锁后调用，避免自死锁。
func (bm *BotManager) WrapCommandInDimensionWithDim(cmd string, dim string) string {
	return wrapCommandInDimension(cmd, dim)
}

func wrapCommandInDimension(cmd string, dim string) string {
	if dim == "" || strings.EqualFold(dim, "overworld") || strings.EqualFold(dim, "minecraft:overworld") {
		return cmd
	}
	trimmed := strings.TrimSpace(cmd)
	if strings.HasPrefix(trimmed, "/") {
		trimmed = trimmed[1:]
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "execute in ") {
		return cmd
	}
	return fmt.Sprintf("execute in %s run %s", dim, trimmed)
}

// SendAICommand 通过网易 AI 命令通道（PyRpc ExecuteCommandEvent）发送命令。
// 自动加 "execute run" 前缀以保证命令执行上下文（匹配 NexusEgo 的 SendSettingsCommand 行为）。
func (bm *BotManager) SendAICommand(cmd string) error {
	bm.mu.Lock()
	conn := bm.conn
	bm.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	cmd = bm.WrapCommandInDimension(cmd)
	if len(cmd) > 0 && cmd[0] == '/' {
		cmd = cmd[1:]
	}
	event := cts_mc_a.ExecuteCommandEvent{
		CommandLine:      "execute run " + cmd,
		CommandRequestID: uuid.New(),
	}
	module := cts_mc.AICommand{Module: &mei.DefaultModule{Event: &event}}
	park := cts.Minecraft{Default: mei.Default{Module: &module}}
	return conn.WritePacket(&packet.PyRpc{
		Value: py_rpc.Marshal(&py_rpc.ModEvent{
			Package: &park,
			Type:    py_rpc.ModEventClientToServer,
		}),
		OperationType: packet.PyRpcOperationTypeSend,
	})
}

// SendWSCommand 通过 WebSocket 自动化通道（CommandRequest + AutomationPlayer origin）发送命令。
// 此通道可获取命令响应，适用于需要确认的操作（如 tickingarea、验证命令）。
func (bm *BotManager) SendWSCommand(cmd string) error {
	bm.mu.Lock()
	if !bm.connected || bm.conn == nil {
		bm.mu.Unlock()
		return fmt.Errorf("not connected")
	}
	conn := bm.conn
	dim := bm.commandDimension
	bm.mu.Unlock()
	cmd = bm.WrapCommandInDimensionWithDim(cmd, dim)
	if len(cmd) > 0 && cmd[0] == '/' {
		cmd = cmd[1:]
	}

	conn.WritePacket(&packet.CommandRequest{
		CommandLine: cmd,
		CommandOrigin: protocol.CommandOrigin{
			Origin:         protocol.CommandOriginAutomationPlayer,
			UUID:           uuid.MustParse(bm.idStr),
			PlayerUniqueID: bm.eid,
		},
		Version: 105,
	})
	return nil
}

// SendPlayerCommand 以玩家身份发送命令（CommandOriginPlayer），如同真实玩家在聊天栏输入。
func (bm *BotManager) SendPlayerCommand(cmd string) error {
	bm.mu.Lock()
	if !bm.connected || bm.conn == nil {
		bm.mu.Unlock()
		return fmt.Errorf("not connected")
	}
	conn := bm.conn
	dim := bm.commandDimension
	bm.mu.Unlock()
	cmd = bm.WrapCommandInDimensionWithDim(cmd, dim)
	if len(cmd) > 0 && cmd[0] == '/' {
		cmd = cmd[1:]
	}

	conn.WritePacket(&packet.CommandRequest{
		CommandLine: cmd,
		CommandOrigin: protocol.CommandOrigin{
			Origin:         protocol.CommandOriginPlayer,
			UUID:           uuid.MustParse(bm.idStr),
			PlayerUniqueID: bm.eid,
		},
		Version: 105,
	})
	return nil
}

// WritePacket writes a raw packet to the Minecraft connection.
func (bm *BotManager) WritePacket(pk packet.Packet) error {
	bm.mu.Lock()
	if !bm.connected || bm.conn == nil {
		bm.mu.Unlock()
		return fmt.Errorf("not connected")
	}
	conn := bm.conn
	bm.mu.Unlock()

	conn.WritePacket(pk)
	return nil
}

func (bm *BotManager) quotedName() string {
	return `"` + bm.display + `"`
}

func hasNonDigit(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return true
		}
	}
	return false
}

// sendModSync 发送模组同步和初始化事件（山头服需要）
func (bm *BotManager) sendModSync(conn *minecraft.Conn) {
	time.Sleep(2 * time.Second) // 等登录包风暴平息

	runtimeid := fmt.Sprintf("%d", conn.GameData().EntityUniqueID)

	// 不发送 SyncUsingMod：验证服务器 outfit_info 不可靠，
	// 发空列表反而告诉服务器"我没模组"导致被踢。
	// 资源包已在登录阶段通过 ResourcePackClientResponse(0x03) 声明"全都有"。
	// 参考 Crow-Brother-Ads 的做法，只发后续初始化事件即可。

	conn.WritePacket(&packet.PyRpc{
		Value:         py_rpc.Marshal(&py_rpc.ClientLoadAddonsFinishedFromGac{}),
		OperationType: packet.PyRpcOperationTypeSend,
	})

	{
		event := cts_mc_p.GetLoadedInstances{PlayerRuntimeID: runtimeid}
		module := cts_mc.Preset{Module: &mei.DefaultModule{Event: &event}}
		park := cts.Minecraft{Default: mei.Default{Module: &module}}
		conn.WritePacket(&packet.PyRpc{
			Value: py_rpc.Marshal(&py_rpc.ModEvent{
				Package: &park,
				Type:    py_rpc.ModEventClientToServer,
			}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	}

	conn.WritePacket(&packet.PyRpc{
		Value:         py_rpc.Marshal(&py_rpc.ArenaGamePlayerFinishLoad{}),
		OperationType: packet.PyRpcOperationTypeSend,
	})

	{
		event := cts_mc_v.PlayerUiInit{RuntimeID: runtimeid}
		module := cts_mc.VIPEventSystem{Module: &mei.DefaultModule{Event: &event}}
		park := cts.Minecraft{Default: mei.Default{Module: &module}}
		conn.WritePacket(&packet.PyRpc{
			Value: py_rpc.Marshal(&py_rpc.ModEvent{
				Package: &park,
				Type:    py_rpc.ModEventClientToServer,
			}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	}
}

func formatCommandMessage(msg string, params []string) string {
	if len(params) == 0 {
		return msg
	}
	// Try Java-style %1$s, %2$s, etc.
	substituted := false
	for i, p := range params {
		ph := fmt.Sprintf("%%%d$s", i+1)
		if strings.Contains(msg, ph) {
			msg = strings.ReplaceAll(msg, ph, p)
			substituted = true
		}
	}
	if substituted {
		return msg
	}
	// Replace C-style %s with actual parameters
	if strings.Contains(msg, "%s") || strings.Contains(msg, "%d") {
		for _, p := range params {
			msg = strings.Replace(msg, "%s", p, 1)
		}
		return msg
	}
	// Localization key — append params
	return msg + ": " + strings.Join(params, ", ")
}
