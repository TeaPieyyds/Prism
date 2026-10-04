package main

import (
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"bot-apk/building"
	"bot-apk/state"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/utils"
)

// ── 工作区NBT方块执行器（从 OnNBT 抽取，单机与多机共用） ──

// placeOneNBTBlock 对一个工作区NBT方块执行完整的"制作→保存→加载→删除"流程。
// 与单机导入 OnNBT 里的 needsNBT 分支完全一致，保证单机/多机行为一致（§8.1）。
func placeOneNBTBlock(bot *BotManager, x, y, z int32, blockName, blockStates string, nbtData map[string]any, consoleCenter protocol.BlockPos, nbtStructSeq *int) error {
	statesMap := utils.ParseBlockStatesString(blockStates)
	cleanName := strings.ReplaceAll(strings.TrimPrefix(blockName, "minecraft:"), ":", "_")

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		bot.cleanupNBTWorkspace()
		// 先 TP 到工作区并短暂等待，主动触发目标区块加载，降低首次进服/区块未加载时
		// initConsole 的 structure save 轮询超时概率（超时会整块跳过 NBT）。
		bot.TP(int(consoleCenter[0]), int(consoleCenter[1]+1), int(consoleCenter[2]))
		time.Sleep(500 * time.Millisecond)
		assigner, aErr := bot.getNBTAssigner(consoleCenter)
		if aErr != nil {
			lastErr = aErr
			debugLog("NBT: getNBTAssigner 失败(第%d次): %v", attempt+1, aErr)
			time.Sleep(600 * time.Millisecond)
			continue
		}
		bot.TP(int(consoleCenter[0]), int(consoleCenter[1]+1), int(consoleCenter[2]))
		time.Sleep(50 * time.Millisecond)

		_, _, _, nbtErr := assigner.PlaceNBTBlock(blockName, statesMap, nbtData)
		if nbtErr != nil {
			lastErr = nbtErr
			debugLog("NBT: PlaceNBTBlock 失败(第%d次): %v", attempt+1, nbtErr)
			time.Sleep(600 * time.Millisecond)
			continue
		}

		structID := building.NBTStructName(bot.BotIndex, cleanName, *nbtStructSeq)
		*nbtStructSeq++

		saveCmd := fmt.Sprintf("structure save \"%s\" %d %d %d %d %d %d",
			structID, consoleCenter[0], consoleCenter[1], consoleCenter[2],
			consoleCenter[0], consoleCenter[1], consoleCenter[2])
		if err := bot.SendPlayerCommand(saveCmd); err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		time.Sleep(100 * time.Millisecond)

		bot.TP(int(x), int(y+1), int(z))
		time.Sleep(100 * time.Millisecond)

		loadCmd := fmt.Sprintf("structure load \"%s\" %d %d %d", structID, x, y, z)
		if err := bot.SendAICommand(loadCmd); err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		time.Sleep(100 * time.Millisecond)

		bot.SendPlayerCommand(fmt.Sprintf("structure delete \"%s\"", structID))
		return nil
	}
	return fmt.Errorf("NBT 方块放置失败（已重试 5 次）: %v", lastErr)
}

// placeOneSign 放置一个告示牌，调用 BotManager.PlaceSign。
func placeOneSign(bot *BotManager, x, y, z int32, blockName, blockStates string, nbtData map[string]any) error {
	bot.TP(int(x), int(y+1), int(z))
	return bot.PlaceSign(x, y, z, blockName, blockStates, nbtData)
}

// placeOneLightNBT 处理一个不需要工作台的轻NBT方块（命令方块/结构方块）：
// 传送到目标位置后，发送 CommandBlockUpdate / StructureBlockUpdate 包。
func placeOneLightNBT(bot *BotManager, x, y, z int32, blockName, blockStates string, nbtData map[string]any, cmdDisabled bool) error {
	if bot == nil || bot.conn == nil {
		return nil
	}
	bot.TP(int(x), int(y+1), int(z))
	// 先放置方块本体（命令方块/结构方块），否则只发包不会创建方块 → 方块丢失。
	// 单机导入在放置引擎里会先 setblock 再 OnNBT；轻NBT单元被独立出来后，
	// 这里的 setblock 就是补上"放置方块本体"这一步（§8.1 正确性底线）。
	nm, st := normalizeBlockName(blockName), normalizeBlockStates(blockName, blockStates)
	setCmd := fmt.Sprintf("setblock %d %d %d %s", x, y, z, nm)
	if st != "" && st != "[]" {
		setCmd = fmt.Sprintf("setblock %d %d %d %s %s", x, y, z, nm, st)
	}
	if err := bot.SendBlockCmd(setCmd); err != nil {
		return err
	}
	// 命令方块
	if cmd, ok := nbtData["Command"].(string); ok {
		mode := uint32(0)
		if strings.Contains(blockName, "repeating_command_block") {
			mode = 1
		} else if strings.Contains(blockName, "chain_command_block") {
			mode = 2
		}
		needsRS := false
		if m := nbtByte(nbtData, "LPRedstoneMode"); m != 0 {
			needsRS = true
		}
		if m := nbtByte(nbtData, "NeedsRedstone"); m != 0 {
			needsRS = true
		}
		if m := nbtByte(nbtData, "auto"); m == 0 {
			needsRS = true
		}
		if cmdDisabled && strings.Contains(blockName, "repeating_command_block") {
			needsRS = true
		}
		cond := nbtByte(nbtData, "conditionalMode") != 0
		if c := nbtByte(nbtData, "Conditional"); c != 0 {
			cond = true
		}
		if c := nbtByte(nbtData, "conditional_bit"); c != 0 {
			cond = true
		}
		trackOut := nbtByte(nbtData, "TrackOutput") != 0
		exeFirst := nbtByte(nbtData, "ExecuteOnFirstTick") != 0
		tickDelay := int32(nbtInt(nbtData, "TickDelay"))
		name, _ := nbtData["CustomName"].(string)
		bot.conn.WritePacket(&packet.CommandBlockUpdate{
			Block: true, Position: protocol.BlockPos{x, y, z},
			Mode: mode, NeedsRedstone: needsRS, Conditional: cond,
			Command: cmd, Name: name,
			ShouldTrackOutput: trackOut, ExecuteOnFirstTick: exeFirst,
			TickDelay: tickDelay,
		})
		return nil
	}
	// 结构方块
	if strings.Contains(blockName, "structure_block") {
		upd := buildStructureBlockUpdate(nbtData)
		upd.Position = protocol.BlockPos{x, y, z}
		bot.conn.WritePacket(&upd)
		return nil
	}
	return nil
}

// ── 普通单元 ImportTask 构建器 ──

// buildNormalUnitTask 为指定 bot 构建一个可执行普通单元放置的 ImportTask。
// 普通单元不包含工作区NBT方块和告示牌（它们被 RunUnit 的 SkipPositions 跳过），
// 所以 OnNBT 只需处理轻NBT（命令/结构方块）和简单 BlockActorData 补发。
func buildNormalUnitTask(bot *BotManager, data *building.StructureData, params map[string]any) *building.ImportTask {
	importCmds := getBool(params, "import_commands", true)
	cmdDisabled := getBool(params, "cmd_disabled", false)
	excludeFluids := getBool(params, "exclude_fluids", false)
	excludeWater := excludeFluids && getBool(params, "exclude_water", true)
	excludeWaterlogged := excludeFluids && getBool(params, "exclude_waterlogged", true)
	excludeLava := excludeFluids && getBool(params, "exclude_lava", true)
	dimension := getString(params, "dimension", "overworld")
	if bot != nil {
		bot.SetDimension(dimension)
	}
	regionMode := getInt(params, "region_mode", 1)
	if regionMode < 1 {
		regionMode = 1
	}
	if regionMode > 3 {
		regionMode = 3
	}
	return &building.ImportTask{
		Data:   data,
		BaseX:  getInt(params, "x", 0),
		BaseY:  getInt(params, "y", 64),
		BaseZ:  getInt(params, "z", 0),
		Speed:  max(1, min(getInt(params, "speed", 9500), 20000)),
		StartCX: 0,
		StartCZ: 0,
		PreClearMode: 0,
		Dimension:    dimension,
		RegionMode:   regionMode,
		ChunkLoaded: func(cx, cz int) bool {
			if bot == nil || bot.conn == nil {
				return true
			}
			posX := cx*16 + 8
			posZ := cz*16 + 8
			_, isTimeout, err := bot.SendWSCommandWithTimeout(
				fmt.Sprintf("testforblock %d 1 %d minecraft:air", posX, posZ),
				1*time.Second,
			)
			return !(isTimeout || err != nil)
		},
		SetBlock: func(x, y, z int32, name, states string) error {
			nm, st := normalizeBlockName(name), normalizeBlockStates(name, states)
			if st == "" || st == "[]" {
				return bot.SendBlockCmd(fmt.Sprintf("setblock %d %d %d %s", x, y, z, nm))
			}
			return bot.SendBlockCmd(fmt.Sprintf("setblock %d %d %d %s %s", x, y, z, nm, st))
		},
		FillRegion: func(x1, y1, z1, x2, y2, z2 int32, name, states string) error {
			nm, st := normalizeBlockName(name), normalizeBlockStates(name, states)
			if st == "" || st == "[]" {
				return bot.SendBlockCmd(fmt.Sprintf("fill %d %d %d %d %d %d %s", x1, y1, z1, x2, y2, z2, nm))
			}
			return bot.SendBlockCmd(fmt.Sprintf("fill %d %d %d %d %d %d %s %s", x1, y1, z1, x2, y2, z2, nm, st))
		},
		Teleport: func(x, y, z int) error {
			return bot.TP(x, y, z)
		},
		// 普通单元不含工作区NBT和告示牌（被 Skipped 跳过），OnNBT 只处理轻NBT
		OnNBT: func(x, y, z int32, nbtData map[string]any, blockName, blockStates string) {
			if bot == nil || bot.conn == nil {
				return
			}
			bot.TP(int(x), int(y+1), int(z))
			// 命令方块
			if cmd, ok := nbtData["Command"].(string); ok {
				mode := uint32(0)
				if strings.Contains(blockName, "repeating_command_block") {
					mode = 1
				} else if strings.Contains(blockName, "chain_command_block") {
					mode = 2
				}
				needsRS := false
				if m := nbtByte(nbtData, "LPRedstoneMode"); m != 0 {
					needsRS = true
				}
				if m := nbtByte(nbtData, "NeedsRedstone"); m != 0 {
					needsRS = true
				}
				if m := nbtByte(nbtData, "auto"); m == 0 {
					needsRS = true
				}
				if cmdDisabled && strings.Contains(blockName, "repeating_command_block") {
					needsRS = true
				}
				cond := nbtByte(nbtData, "conditionalMode") != 0
				if c := nbtByte(nbtData, "Conditional"); c != 0 {
					cond = true
				}
				if c := nbtByte(nbtData, "conditional_bit"); c != 0 {
					cond = true
				}
				trackOut := nbtByte(nbtData, "TrackOutput") != 0
				exeFirst := nbtByte(nbtData, "ExecuteOnFirstTick") != 0
				tickDelay := int32(nbtInt(nbtData, "TickDelay"))
				name, _ := nbtData["CustomName"].(string)
				bot.conn.WritePacket(&packet.CommandBlockUpdate{
					Block: true, Position: protocol.BlockPos{x, y, z},
					Mode: mode, NeedsRedstone: needsRS, Conditional: cond,
					Command: cmd, Name: name,
					ShouldTrackOutput: trackOut, ExecuteOnFirstTick: exeFirst,
					TickDelay: tickDelay,
				})
				return
			}
			// 结构方块
			if strings.Contains(blockName, "structure_block") {
				upd := buildStructureBlockUpdate(nbtData)
				upd.Position = protocol.BlockPos{x, y, z}
				bot.conn.WritePacket(&upd)
				return
			}
			// 简单方块实体补发 BlockActorData
			time.Sleep(5 * time.Millisecond)
			if nbtData["Items"] == nil && nbtData["Command"] == nil {
				norm := building.NormalizeBlockEntityNBT(blockName, nbtData)
				norm["x"] = x
				norm["y"] = y
				norm["z"] = z
				bot.conn.WritePacket(&packet.BlockActorData{
					Position: protocol.BlockPos{x, y, z},
					NBTData:  norm,
				})
				time.Sleep(10 * time.Millisecond)
			}
		},
		ImportCommands:   importCmds,
		CmdDisabled:      cmdDisabled,
		ExcludeWater:     excludeWater,
		ExcludeWaterlogged: excludeWaterlogged,
		ExcludeLava:      excludeLava,
		StopOnError:      false,
	}
}

// ── MultiBotManager ──

// MultiBotManager 持有 N 个 BotManager 和一个 WorkPool，驱动所有机器人工人循环。
type MultiBotManager struct {
	mu sync.Mutex
	bots   []*BotManager
	pool   *building.WorkPool
	data   *building.StructureData
	params map[string]any

	consoleCenters []protocol.BlockPos
	nbtStructSeqs  []int

	// 放置基准坐标：NBT/轻NBT/告示牌单元存的是结构局部坐标，执行时必须加上
	// BaseX/BaseY/BaseZ 才得到世界坐标（普通单元由 ImportTask 内部加）。
	baseX, baseY, baseZ int

	// 每台机器人已完成的方块数，用于计算"最慢机器人进度"（瓶颈驱动）。
	blocksPlaced []int
	// speedBlocks 每台机器人实时放置的方块数，用于算"上一秒总方块数"速度。
	// 普通单元由 OnProgress 实时累加，其他单元完成时累加 BlockCount。
	speedBlocks []int
	// botActiveKind 每台机器人当前正在处理的单元类型（-1=空闲），
	// 用于 actionbar 显示"该类型下有几台机器人"。
	botActiveKind []int
	// lastProg 每台机器人上一次普通单元 OnProgress 的方块数，用于差分累加速度。
	lastProg []int

	// botPref 每台机器人的主类型：认领时优先领该类型的任务（保持工作连续性），
	// 该类型没活时自动领其他类型（工作窃取 = 升降级/援助，无需显式状态机）。
	botPref []building.UnitKind
	// botPos 每台机器人当前所在区块列（质心），认领时"距离最近"优先，减少传送、保证区块加载。
	botPos [][2]int
	// botStarted 每台机器人是否已在首个工作区起点等过区块加载。首次认领时 TP 到起点
	// 等待约 1.5s，让区块加载后再放置；否则并发各去不同区域、区块未加载时 setblock/fill
	// 会失败，导致方块丢失、机器人看似来回传送却不导入。
	botStarted []bool

	// stopCh 停止信号：前端"停止导入"时关闭，worker 循环和单元执行都会检查。
	stopCh <-chan struct{}
}

// SetStopCh 设置停止信号通道。前端点击停止时关闭该通道，导入会尽快停止。
func (m *MultiBotManager) SetStopCh(stopCh <-chan struct{}) {
	m.stopCh = stopCh
}

// stopped 检查是否收到停止信号。
func (m *MultiBotManager) stopped() bool {
	if m.stopCh == nil {
		return false
	}
	select {
	case <-m.stopCh:
		return true
	default:
		return false
	}
}

// NewMultiBotManager 构建多机器人管理器，为每个 bot 分配工作台坐标（§8.3）并设置 BotIndex。
func NewMultiBotManager(bots []*BotManager, data *building.StructureData, params map[string]any) *MultiBotManager {
	n := len(bots)
	m := &MultiBotManager{
		bots:           bots,
		data:           data,
		params:         params,
		consoleCenters: make([]protocol.BlockPos, n),
		nbtStructSeqs:  make([]int, n),
	}
	m.baseX = getInt(params, "x", 0)
	m.baseY = getInt(params, "y", 64)
	m.baseZ = getInt(params, "z", 0)
	for i := range bots {
		wx, wy, wz := building.WorkspaceCenter(i, m.baseX, m.baseZ, data.SizeX, data.SizeZ)
		m.consoleCenters[i] = protocol.BlockPos{int32(wx), int32(wy), int32(wz)}
		if bots[i] != nil {
			bots[i].BotIndex = i
		}
	}
	return m
}

// SetPool 设置工人池。
func (m *MultiBotManager) SetPool(pool *building.WorkPool) {
	m.mu.Lock()
	m.pool = pool
	m.mu.Unlock()
}

// Run 启动所有机器人工人循环，等待全部完成（阻塞）。
func (m *MultiBotManager) Run() {
	if m.pool == nil {
		return
	}
	m.blocksPlaced = make([]int, len(m.bots))
	m.speedBlocks = make([]int, len(m.bots))
	m.botActiveKind = make([]int, len(m.bots))
	m.lastProg = make([]int, len(m.bots))
	for i := range m.botActiveKind {
		m.botActiveKind[i] = -1
	}

	// ── 角色分配：命令、NBT 各最多 1 台（若该类型存在且有机器人可用），其余全做普通 ──
	// 机器人数量 <3 或缺少某类型时，相应角色不分配，全部做普通（空池自然跳过）。
	m.botPref = make([]building.UnitKind, len(m.bots))
	m.botPos = make([][2]int, len(m.bots))
	m.botStarted = make([]bool, len(m.bots))
	for i := range m.botPref {
		m.botPref[i] = building.UnitNormal
	}
	maxRole := len(m.bots) - 1 // 至少保留 1 台普通（普通是大头）
	if maxRole < 0 {
		maxRole = 0
	}
	roleAssigned := 0
	assignRole := func(kind building.UnitKind) {
		if roleAssigned >= maxRole {
			return
		}
		for i := range m.botPref {
			if m.botPref[i] == building.UnitNormal {
				m.botPref[i] = kind
				roleAssigned++
				return
			}
		}
	}
	if m.pool.HasLightNBT() {
		assignRole(building.UnitLightNBT)
	}
	if m.pool.HasNBT() {
		assignRole(building.UnitNBT)
	}
	debugLog("MULTI ROLE: 主类型分配 = %v", m.botPref)

	// actionbar 进度条：由活最少的机器人发送（不挤占主机器人的指令队列）
	stopTicker := make(chan struct{})
	defer close(stopTicker)
	go m.actionbarLoop(stopTicker)

	var wg sync.WaitGroup
	connected := 0
	for i, bot := range m.bots {
		if bot == nil || !bot.IsConnected() {
			debugLog("MULTI RUN: bot[%d] 未连接，跳过", i)
			continue
		}
		connected++
		wg.Add(1)
		go func(idx int, b *BotManager) {
			defer wg.Done()
			m.runWorker(idx, b)
		}(i, bot)
	}
	debugLog("MULTI RUN: 共 %d 台机器人，其中 %d 台已连接启动 worker", len(m.bots), connected)
	wg.Wait()
	// 导入彻底完成：清空所有机器人工作台区域（§10 残留物清理）
	m.cleanupAllWorkspaces()
}

// cleanupAllWorkspaces 清空所有机器人的 NBT 工作台区域（fill air + 清理操作台），
// 避免导入完成后在建筑外残留铁砧/织布机等。
func (m *MultiBotManager) cleanupAllWorkspaces() {
	for i, bot := range m.bots {
		if bot == nil || bot.conn == nil {
			continue
		}
		if i < len(m.consoleCenters) {
			m.setupWorkspace(bot, m.consoleCenters[i])
		}
	}
}

// actionbarLoop 每隔 500ms 由活最少的机器人发一次 actionbar 进度。
// 速度：所有机器人 speedBlocks 之和的每秒增量 = "上一秒总方块数"（实时，不等单元完成）。
func (m *MultiBotManager) actionbarLoop(stop chan struct{}) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	start := time.Now()
	var lastTotal, lastSpeed int
	var lastSample time.Time

	// 文件名：去后缀、去 @[/[ 段、截断（与单机 buildV5Actionbar 一致），不带 {} 包裹。
	name := shortName(getString(m.params, "path", ""))
	if idx := strings.IndexAny(name, "@["); idx >= 0 {
		name = name[:idx]
	}
	if len([]rune(name)) > 6 {
		name = string([]rune(name)[:6])
	}

	for {
		select {
		case <-ticker.C:
			bot := m.idlestBot()
			if bot == nil {
				continue
			}
			// 速度：所有机器人 speedBlocks 之和的每秒增量
			total := 0
			m.mu.Lock()
			for i := range m.speedBlocks {
				total += m.speedBlocks[i]
			}
			m.mu.Unlock()
			speed := lastSpeed
			now := time.Now()
			if lastSample.IsZero() {
				lastTotal, lastSample = total, now
			} else if now.Sub(lastSample) >= time.Second {
				if dt := now.Sub(lastSample).Seconds(); dt >= 0.5 {
					speed = int(float64(total-lastTotal) / dt)
					lastTotal, lastSample, lastSpeed = total, now, speed
				}
			}

			nd, nt, nbd, nbt, sd, st, ld, lt := m.pool.ProgressBlocks()
			counts := m.activeBotCounts()
			totalBots := m.connectedBots()
			msg := buildMultiActionbar(name, nd, nt, nbd, nbt, sd, st, ld, lt,
				counts, totalBots, m.pool.Phase1Done(), speed, start)
			bot.SendWOCmd(fmt.Sprintf(`titleraw @a actionbar {"rawtext":[{"text":"%s"}]}`, msg))
		case <-stop:
			return
		}
	}
}

// padFraction 把 done/total 补零对齐到相同宽度（如 1/100 → "001/100"），
// 保证进度数字长度固定，不随任务进行出现两位变三位。
func padFraction(done, total int) string {
	wd, wt := len(fmt.Sprintf("%d", done)), len(fmt.Sprintf("%d", total))
	w := wd
	if wt > w {
		w = wt
	}
	return fmt.Sprintf("%0*d/%0*d", w, done, w, total)
}

// padBotCount 机器人数量补零：总数>10 且当前<10 时显示 0X（文档第 2 点）。
func padBotCount(v, totalBots int) string {
	if totalBots > 10 && v < 10 {
		return fmt.Sprintf("%02d", v)
	}
	return fmt.Sprintf("%d", v)
}

// buildMultiActionbar 渲染多机器人模式的 actionbar 进度条，风格与单机 buildV5Actionbar 统一
// （旋转帧 + Importer 品牌 + §颜色 + buildParBar）。按文档 docs/多机器人导入行动栏进度条要求.md：
//  - 表头：旋转帧 + "导入中" + prism（多机器人固定，不分是否购买命名）+ 文件名（无 {}）
//  - 阶段一：NBT/命令/普通 各一行「类型 机器人数量 进度条 done/total」+「总共 总机器人 进度条 百分比」
//  - 阶段一完成后：去掉 NBT/命令/普通三行，只留告示牌行
//  - 底部：速度（上一秒方块数）+ 方块总数
func buildMultiActionbar(name string,
	normalDone, normalTotal, nbtDone, nbtTotal, signDone, signTotal, lightDone, lightTotal int,
	counts [4]int, totalBots int, phase1Done bool, speed int, start time.Time) string {

	frames := []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
	frame := frames[time.Now().UnixMilli()/500%int64(len(frames))]
	const barWidth = 14
	clamp := func(p int) int {
		if p > barWidth {
			return barWidth
		}
		if p < 0 {
			return 0
		}
		return p
	}
	bar := func(pct int, color string) string {
		return buildParBar(clamp((pct*barWidth+99)/100), barWidth, color, "7")
	}
	// 总进度：全部类型 done/total 之和
	totalDone := normalDone + nbtDone + signDone + lightDone
	totalTotal := normalTotal + nbtTotal + signTotal + lightTotal
	totalPct := 0
	if totalTotal > 0 {
		totalPct = totalDone * 100 / totalTotal
	}
	if totalPct > 99 {
		totalPct = 99
	}

	var rows strings.Builder
	// 表头：旋转帧 + 导入中 + prism + 文件名（无 {}；prism 固定小写，不随是否购买命名变化）
	rows.WriteString(fmt.Sprintf("§r%s  §c导入中  §dprism §fv%s  §7┃  §f%s\n", frame, shortVersion(), name))

	if !phase1Done {
		// 阶段一：NBT / 命令 / 普通（仅显示实际存在的类型）
		if nbtTotal > 0 {
			rows.WriteString(fmt.Sprintf("§l§d> §7NBT  §e%s  %s  §f%s\n",
				padBotCount(counts[1], totalBots), bar(nbtDone*100/nbtTotal, "d"), padFraction(nbtDone, nbtTotal)))
		}
		if lightTotal > 0 {
			rows.WriteString(fmt.Sprintf("§l§d> §7命令 §6%s  %s  §f%s\n",
				padBotCount(counts[3], totalBots), bar(lightDone*100/lightTotal, "6"), padFraction(lightDone, lightTotal)))
		}
		if normalTotal > 0 {
			rows.WriteString(fmt.Sprintf("§l§d> §7普通 §e%s  %s  §f%s\n",
				padBotCount(counts[0], totalBots), bar(normalDone*100/normalTotal, "e"), padFraction(normalDone, normalTotal)))
		}
	} else if signTotal > 0 {
		// 阶段一完成后：只留告示牌一行
		rows.WriteString(fmt.Sprintf("§l§d> §7告示牌 §b%s  %s  §f%s\n",
			padBotCount(counts[2], totalBots), bar(signDone*100/signTotal, "b"), padFraction(signDone, signTotal)))
	}

	// 总共行：总机器人数量 + 总进度条 + 百分比
	rows.WriteString(fmt.Sprintf("§l§d> §7总共 §a%s  %s  §l§a%d%%\n",
		padBotCount(totalBots, totalBots), bar(totalPct, "a"), totalPct))

	// 底部：速度（上一秒方块数）+ 方块总数
	rows.WriteString(fmt.Sprintf("§r%s/s  §7|  §f(%s/%s)  §7|  %s",
		formatSpeedV5(float64(speed)), formatCountV5(totalDone), formatCountV5(totalTotal),
		time.Since(start).Round(time.Second)))
	return rows.String()
}

// idlestBot 返回当前完成方块数最少的机器人（最闲，最有余力发 actionbar）。
func (m *MultiBotManager) idlestBot() *BotManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	minIdx := 0
	minPlaced := -1
	for i, bot := range m.bots {
		if bot == nil || !bot.IsConnected() {
			continue
		}
		placed := 0
		if i < len(m.blocksPlaced) {
			placed = m.blocksPlaced[i]
		}
		if minPlaced < 0 || placed < minPlaced {
			minPlaced = placed
			minIdx = i
		}
	}
	if minPlaced < 0 {
		return nil
	}
	return m.bots[minIdx]
}

// runWorker 单个机器人工人循环：Claim（主类型+距离最近）→ 执行 → Complete。
// 断线处理（§14.3）：每轮先检查 bot 是否还连着。一旦断线，立即退出本 worker，
// 并把手上还没做完（in_progress）的单元 Release 回队列，交给其他机器人重做（幂等），
// 绝不带着死连接继续领活、把单元一个个判失败标记 done（那会造成空缺）。
func (m *MultiBotManager) runWorker(idx int, bot *BotManager) {
	debugLog("MULTI WORKER %d 启动 (bot=%s)", idx, botDisplayName(bot))
	// 当前手上认领的单元；worker 退出时若它还在 in_progress（如机器人断线但
	// WritePacket 未立即报错），Release 回队列，避免永久卡死（§14.3 回收）。
	var cur *building.WorkItem
	defer func() {
		if cur != nil {
			m.pool.Release(cur)
			debugLog("MULTI WORKER %d 退出时回收手上单元 id=%d", idx, cur.Unit.ID)
		}
	}()

	for {
		if m.stopped() {
			debugLog("MULTI WORKER %d 因停止退出", idx)
			return
		}
		if bot == nil || !bot.IsConnected() {
			debugLog("MULTI WORKER %d 机器人断线，退出并回收单元", idx)
			return // defer 负责 Release cur
		}
		pref := building.UnitNormal
		pos := [2]int{0, 0}
		m.mu.Lock()
		if idx < len(m.botPref) {
			pref = m.botPref[idx]
		}
		if idx < len(m.botPos) {
			pos = m.botPos[idx]
		}
		m.mu.Unlock()

		c := m.pool.Claim(pref, pos[0], pos[1])
		if c.Item == nil {
			if m.pool.Done() {
				debugLog("MULTI WORKER %d 全部完成退出", idx)
				return
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		cur = c.Item
		// 更新当前位置为认领单元的质心（距离最近认领的依据），并记录本机器人当前类型
		cx, cz := c.Item.Unit.CentroidInt()
		m.mu.Lock()
		if idx < len(m.botPos) {
			m.botPos[idx] = [2]int{cx, cz}
		}
		if idx < len(m.botActiveKind) {
			m.botActiveKind[idx] = int(c.Item.Unit.Kind)
		}
		started := idx < len(m.botStarted) && m.botStarted[idx]
		m.mu.Unlock()
		// 每台机器人首次认领：TP 到工作区起点，等待约 1.5s 让区块加载，再开始放置。
		// 否则并发各去不同区域、区块未加载时 setblock/fill 会失败 → 方块丢失、机器人
		// 看似来回传送却不导入。之后区块随放置顺序推进加载，无需再等待。
		if !started && bot != nil && bot.conn != nil {
			bot.TP(m.baseX+cx*16+8, m.baseY, m.baseZ+cz*16+8)
			time.Sleep(1500 * time.Millisecond)
			m.mu.Lock()
			if idx < len(m.botStarted) {
				m.botStarted[idx] = true
			}
			m.mu.Unlock()
			debugLog("MULTI WORKER %d 首次认领，已在工作区起点等待区块加载", idx)
		}
		debugLog("MULTI WORKER %d 认领单元 id=%d kind=%d blocks=%d", idx, c.Item.Unit.ID, c.Item.Unit.Kind, c.Item.Unit.BlockCount)
		err := m.executeUnit(idx, bot, c.Item)
		if err != nil {
			if m.pool.Requeue(c.Item) {
				debugLog("MULTI WORKER %d 单元 id=%d 失败: %v → 重排队", idx, c.Item.Unit.ID, err)
				m.setBotIdle(idx)
				cur = nil
			} else {
				// 连续失败超限：判失败跳过（§14.4：不伪造完成）。如实上报缺失量，避免反复重做。
				debugLog("MULTI WORKER %d 单元 id=%d 连续失败超限，判失败（方块未放）", idx, c.Item.Unit.ID)
				m.reportUnitFailed(idx, c.Item, err)
				m.setBotIdle(idx)
				cur = nil
			}
		} else {
			debugLog("MULTI WORKER %d 单元 id=%d 完成", idx, c.Item.Unit.ID)
			m.pool.Complete(c.Item, c.Item.Unit.BlockCount)
			m.mu.Lock()
			if idx < len(m.blocksPlaced) {
				m.blocksPlaced[idx] += c.Item.Unit.BlockCount
			}
			if idx < len(m.botActiveKind) {
				m.botActiveKind[idx] = -1
			}
			// 非普通单元的速度量在此累加（普通单元由 OnProgress 实时累加，避免双算）
			if c.Item.Unit.Kind != building.UnitNormal && idx < len(m.speedBlocks) {
				m.speedBlocks[idx] += c.Item.Unit.BlockCount
			}
			m.mu.Unlock()
			cur = nil
		}
	}
}

// setBotIdle 把机器人标记为空闲（当前不处理任何类型），供 actionbar 统计各类型机器人数。
func (m *MultiBotManager) setBotIdle(idx int) {
	m.mu.Lock()
	if idx >= 0 && idx < len(m.botActiveKind) {
		m.botActiveKind[idx] = -1
	}
	m.mu.Unlock()
}

// activeBotCounts 统计各类型当前正在处理的机器人数量，返回 [普通,NBT,告示牌,轻NBT]。
func (m *MultiBotManager) activeBotCounts() [4]int {
	var c [4]int
	m.mu.Lock()
	for _, k := range m.botActiveKind {
		if k >= 0 && k < 4 {
			c[k]++
		}
	}
	m.mu.Unlock()
	return c
}

// connectedBots 返回当前已连接的机器人总数（含主机器人）。
func (m *MultiBotManager) connectedBots() int {
	n := 0
	for _, b := range m.bots {
		if b != nil && b.IsConnected() {
			n++
		}
	}
	return n
}

// reportUnitFailed 单元连续失败判失败时，向前端如实上报缺失量，让用户知道这里有缺口、
// 不会误以为全部完成。不把单元标记完成（§14.4 底线）。
func (m *MultiBotManager) reportUnitFailed(idx int, item *building.WorkItem, err error) {
	msg := fmt.Sprintf("! 单元 %d（%s，%d 方块）多次失败，已判失败跳过，方块缺失：%v",
		item.Unit.ID, kindName(item.Unit.Kind), item.Unit.BlockCount, err)
	// 前端 progress 事件以滚动日志显示 msg，不打断任务状态。
	hub.Emit("progress", mustJSON(map[string]any{"msg": msg}))
}

// kindName 返回单元类型的可读名，用于失败/进度提示。
func kindName(k building.UnitKind) string {
	switch k {
	case building.UnitNormal:
		return "普通"
	case building.UnitNBT:
		return "特殊"
	case building.UnitSign:
		return "告示牌"
	case building.UnitLightNBT:
		return "命令/结构"
	}
	return "未知"
}

func (m *MultiBotManager) executeUnit(idx int, bot *BotManager, item *building.WorkItem) (err error) {
	// 防御：任何单元执行的 panic 都不能崩溃整个进程（大文件时尤其要稳）。
	// 打印完整堆栈，便于定位"反复导入"的真实 panic 位置。
	defer func() {
		if r := recover(); r != nil {
			debugLog("MULTI WORKER %d executeUnit panic: %v\n%s", idx, r, debug.Stack())
			err = fmt.Errorf("executeUnit panic: %v", r)
		}
	}()
	switch item.Unit.Kind {
	case building.UnitNormal:
		return m.execNormalUnit(idx, bot, item.Unit)
	case building.UnitNBT:
		return m.execNBTUnit(idx, bot, item.Unit)
	case building.UnitSign:
		return m.execSignUnit(idx, bot, item.Unit)
	case building.UnitLightNBT:
		return m.execLightNBTUnit(idx, bot, item.Unit)
	}
	return nil
}

func (m *MultiBotManager) execLightNBTUnit(idx int, bot *BotManager, u *building.Unit) error {
	cmdDisabled := getBool(m.params, "cmd_disabled", false)
	for _, p := range u.Positions {
		if m.stopped() {
			return nil
		}
		idx := m.data.GetIndex(p[0], p[1], p[2])
		blockName := m.data.Palette[idx].Name
		blockStates := m.data.Palette[idx].States
		nbtData := m.data.GetNBT(p[0], p[1], p[2])
		// 即使 nbtData 为 nil（如空结构方块）也要 setblock 放置本体；placeOneLightNBT
		// 内部先 setblock，nbtData 为 nil 时仅放置不发更新包，不会漏放方块。
		if err := placeOneLightNBT(bot, int32(p[0]+m.baseX), int32(p[1]+m.baseY), int32(p[2]+m.baseZ), blockName, blockStates, nbtData, cmdDisabled); err != nil {
			return err
		}
	}
	return nil
}

func (m *MultiBotManager) execNormalUnit(idx int, bot *BotManager, u *building.Unit) error {
	it := buildNormalUnitTask(bot, m.data, m.params)
	it.StopCh = m.stopCh
	// 每单元开始前重置差分基准，保证 OnProgress 增量从 0 起算
	m.mu.Lock()
	if idx < len(m.lastProg) {
		m.lastProg[idx] = 0
	}
	m.mu.Unlock()
	// 实时上报方块进度 → 用差分累加到 speedBlocks，得到"上一秒总方块数"速度，
	// 而不是等整单元完成才跳一次（§文档第 8 点）。
	it.OnProgress = func(cx, cz, chunkNum, totalChunks, b int) {
		m.mu.Lock()
		if idx < len(m.lastProg) {
			if delta := b - m.lastProg[idx]; delta > 0 && idx < len(m.speedBlocks) {
				m.speedBlocks[idx] += delta
			}
			m.lastProg[idx] = b
		}
		m.mu.Unlock()
	}
	// 普通单元不需要工作台，不调用 setupWorkspace（那是 NBT 机器人才需要的，
	// 每领一个普通单元都 fill air 工作台区域是浪费，且会打断其他机器人的 NBT 工作台）
	return it.RunUnit(u)
}

func (m *MultiBotManager) execNBTUnit(idx int, bot *BotManager, u *building.Unit) error {
	// 普通机器人领到特殊单元 = 升级去帮特殊（§7 工作窃取）；NBT 角色无需提示。
	// 只提示"升级"，不再每单元刷"将作为XX导入任务"。
	if m.botPref[idx] == building.UnitNormal {
		bot.SendTellraw(fmt.Sprintf("§e%s 已升级为特殊，开始制作特殊方块§r", botDisplayName(bot)))
	}
	// 每个机器人都开自己的工作区（§8.3 坐标互不重叠），才能制作 NBT 方块
	m.setupWorkspace(bot, m.consoleCenters[idx])
	// defer 确保无论成功/失败/中途退出，都清掉工作区（否则残留铁砧/织布机/地板方块）
	defer m.setupWorkspace(bot, m.consoleCenters[idx])
	cc := m.consoleCenters[idx]
	seq := m.nbtStructSeqs[idx]
	for _, p := range u.Positions {
		if m.stopped() {
			return nil
		}
		idx := m.data.GetIndex(p[0], p[1], p[2])
		blockName := m.data.Palette[idx].Name
		blockStates := m.data.Palette[idx].States
		nbtData := m.data.GetNBT(p[0], p[1], p[2])
		if nbtData == nil {
			continue
		}
		if err := placeOneNBTBlock(bot, int32(p[0]+m.baseX), int32(p[1]+m.baseY), int32(p[2]+m.baseZ), blockName, blockStates, nbtData, cc, &seq); err != nil {
			return err
		}
	}
	m.nbtStructSeqs[idx] = seq
	bot.SendTellraw(fmt.Sprintf("§a%s 特殊任务完成，已降级为普通机器人§r", botDisplayName(bot)))
	return nil
}

func (m *MultiBotManager) execSignUnit(idx int, bot *BotManager, u *building.Unit) error {
	for _, p := range u.Positions {
		if m.stopped() {
			return nil
		}
		idx := m.data.GetIndex(p[0], p[1], p[2])
		blockName := m.data.Palette[idx].Name
		blockStates := m.data.Palette[idx].States
		nbtData := m.data.GetNBT(p[0], p[1], p[2])
		if nbtData == nil {
			continue
		}
		if err := placeOneSign(bot, int32(p[0]+m.baseX), int32(p[1]+m.baseY), int32(p[2]+m.baseZ), blockName, blockStates, nbtData); err != nil {
			return err
		}
	}
	return nil
}

// botDisplayName 返回机器人在游戏中的显示名，连接未就绪时退回 "BotN"。
func botDisplayName(bot *BotManager) string {
	if bot != nil && bot.conn != nil {
		if n := bot.conn.IdentityData().DisplayName; n != "" {
			return n
		}
	}
	if bot != nil {
		return fmt.Sprintf("Bot%d", bot.BotIndex)
	}
	return "Bot"
}

func (m *MultiBotManager) setupWorkspace(bot *BotManager, cc protocol.BlockPos) {
	if bot == nil || bot.conn == nil {
		return
	}
	bot.SendPlayerCommand(fmt.Sprintf("execute as @s at @s run fill %d %d %d %d %d %d air",
		cc[0]-5, cc[1]-2, cc[2]-5, cc[0]+5, cc[1]+2, cc[2]+5))
	bot.cleanupNBTWorkspace()
}

// getFleetBots 返回当前可用的机器人员（主机器人 + 已连接的子机器人）。
func getFleetBots() []*BotManager {
	return GetFleetBots()
}

// runMultiImportTask 多机器人导入的完整编排：解析(共享 data) → 切岛 → 工人池 → 各机器人工人。
// 与单机 runImportTask 并行，进度按任务聚合（§6），预清空由主机器人先整图做一次。
func runMultiImportTask(tc *state.TaskController, p map[string]any, bots []*BotManager) {
	path := getString(p, "path", "")
	if path == "" {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "缺少文件路径"}))
		return
	}
	if len(bots) == 0 {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "没有可用的机器人，请先连接"}))
		return
	}

	hub.Emit("task_start", mustJSON(map[string]any{"type": "多机器人导入: " + shortName(path)}))
	hub.Emit("task_progress", mustJSON(map[string]any{"progress": 0}))
	hub.Emit("progress", mustJSON(map[string]any{"msg": "加载建筑文件: " + shortName(path)}))

	// 共享一份解析后的 data（§11.1 内存红线：绝不复制给每个机器人）
	data, err := loadStructureFileCached(path)
	if err != nil {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "加载文件失败: " + err.Error()}))
		return
	}
	rotation := getInt(p, "rotation", 0)
	if rotation != 0 {
		hub.Emit("progress", mustJSON(map[string]any{"msg": fmt.Sprintf("旋转建筑: %d°", rotation)}))
		building.RotateBlocksInPlace(data, rotation)
	}
	// 预冻结 palette：多机器人共享同一份 data，执行期必须只读，
	// 否则并发 getOrCreatePaletteIndex 会往共享 Palette 追加导致崩溃/方块跳过（§11.1）
	data.FreezePalette()

	stats := data.ComputeStats()
	hub.Emit("progress", mustJSON(map[string]any{
		"msg": fmt.Sprintf("建筑: %dx%dx%d, %d 实体方块, %d 台机器人", data.SizeX, data.SizeY, data.SizeZ, stats.SolidBlocks, len(bots)),
	}))

	// 切岛
	res, err := building.SplitStructure(data, building.SplitOptions{BotCount: len(bots)})
	if err != nil {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "切岛失败: " + err.Error()}))
		return
	}
	hub.Emit("progress", mustJSON(map[string]any{
		"msg": fmt.Sprintf("切岛完成: %d 个普通单元, %d 个NBT单元, %d 个命令/结构单元, %d 个告示牌单元, 机器人 %d 台",
			countKind(res, building.UnitNormal), countKind(res, building.UnitNBT),
			countKind(res, building.UnitLightNBT), countKind(res, building.UnitSign), len(bots)),
	}))
	debugLog("MULTI: bots=%d normalUnits=%d nbtUnits=%d lightNBTUnits=%d signUnits=%d totalSolid=%d",
		len(bots), countKind(res, building.UnitNormal), countKind(res, building.UnitNBT),
		countKind(res, building.UnitLightNBT), countKind(res, building.UnitSign), res.TotalSolid)

	// 工人池（工作窃取 + 阶段分离）
	pool := building.NewWorkPool(res)
	pool.OnProgress = func(normal, nbt, sign, light float64) {
		hub.Emit("task_progress", mustJSON(map[string]any{
			"progress":     (normal + nbt + light) / 3 * 0.99, // 阶段一占主体，告示牌末位
			"sub_progress": 1.0,
			"multi":        map[string]any{"normal": normal, "nbt": nbt, "sign": sign, "light": light},
		}))
	}

	mbm := NewMultiBotManager(bots, data, p)
	mbm.SetPool(pool)
	if tc != nil {
		mbm.SetStopCh(tc.StopCh())
	}

	// 预清空：由主机器人（bots[0]）整图做一次，各单元 RunUnit 不再单独清（SkipPreClear）
	bots[0].SendCommand("/gamemode creative " + bots[0].quotedName())
	time.Sleep(300 * time.Millisecond)

	hub.Emit("progress", mustJSON(map[string]any{"msg": "开始多机器人导入..."}))
	mbm.Run()

	// 完成后如实上报（§14.4 底线：不伪造完成）。有判失败单元（缺失）或未完成单元时，
	// 必须明确告知用户缺口，而不是笼统报"导入完成"。主机器人/机器人中途断线时，
	// 其未完成单元已被 Release 回收重做，但仍可能留下失败单元 → 在这里暴露。
	failed := pool.FailedBlocks()
	remaining := pool.RemainingUnits()
	if failed > 0 || remaining > 0 {
		msg := fmt.Sprintf("多机器人导入结束，但有 %d 方块缺失、%d 个单元未完成（详见日志）",
			failed, remaining)
		hub.Emit("task_progress", mustJSON(map[string]any{
			"progress": 0.99, "sub_progress": 1.0,
			"multi": map[string]any{"normal": 0.99, "nbt": 0.99, "sign": 0.99, "light": 0.99},
		}))
		hub.Emit("task_done", mustJSON(map[string]any{"msg": msg, "total": res.TotalSolid, "failed": failed, "remaining": remaining}))
		hub.Emit("progress", mustJSON(map[string]any{"msg": msg}))
		return
	}

	hub.Emit("task_progress", mustJSON(map[string]any{"progress": 1.0, "sub_progress": 1.0}))
	hub.Emit("task_done", mustJSON(map[string]any{"msg": "多机器人导入完成", "total": res.TotalSolid}))
}

// countKind 统计切分结果中某类单元的数量。
func countKind(res *building.SplitResult, kind building.UnitKind) int {
	n := 0
	for _, u := range res.Units {
		if u.Kind == kind {
			n++
		}
	}
	return n
}