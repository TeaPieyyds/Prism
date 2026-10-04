package main

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"bot-apk/building"
	"bot-apk/media"
	"bot-apk/state"
	"bytes"
	"compress/gzip"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/nbt"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/utils"

	"github.com/google/uuid"
)

// brandName 品牌名称，用于进度条标题显示
var brandName = "Prism"

func dispatchTaskWithResume(tc *state.TaskController, taskType string, params json.RawMessage, resumeID string) {
	var p map[string]any
	json.Unmarshal(params, &p)
	switch taskType {
	case "import":
		runImportTask(tc, p, resumeID)
		case "import_multi":
			runMultiImportTask(tc, p, getFleetBots())
	case "repair":
		runRepairTask(tc, p)
	case "export":
		runExportTask(tc, p)
	case "mapart":
		runMapArtTask(tc, p)
	case "skin":
		runSkinTask(tc, p)
	default:
		hub.Emit("task_error", mustJSON(map[string]any{"error": "unknown task type: " + taskType}))
	}
}

func dispatchTask(tc *state.TaskController, taskType string, params json.RawMessage) {
	dispatchTaskWithResume(tc, taskType, params, "")
}

func rawParams(p map[string]any) json.RawMessage {
	b, _ := json.Marshal(p)
	return b
}

func getBotManager() *BotManager {
	botMgrMu.Lock()
	defer botMgrMu.Unlock()
	return botMgr
}

func normalizeBlockName(name string) string {
	return strings.TrimPrefix(name, "minecraft:")
}

func normalizeBlockStates(name, states string) string {
	// 蜂巢：honey_level 由 BlockActorData NBT 控制，不在 setblock 状态中设置
	if strings.Contains(name, "beehive") || strings.Contains(name, "bee_nest") {
		if states != "" && states != "[]" {
			states = stripStateKey(states, "honey_level")
		}
		return states
	}
	if !strings.HasSuffix(name, "command_block") || states == "" || states == "[]" {
		return states
	}
	const key = "facing_direction\"="
	idx := strings.Index(states, key)
	if idx < 0 {
		return ""
	}
	idx += len(key)
	j := idx
	for j < len(states) && states[j] >= '0' && states[j] <= '9' {
		j++
	}
	if j > idx {
		// 网易 setblock 不接受 tileData=0，用默认 facing (down)
		if states[idx:j] == "0" {
			return ""
		}
		return states[idx:j]
	}
	return ""
}

// stripStateKey 从方块状态字符串中移除指定键（例如 honey_level）。
// 格式示例：["honey_level"=5,"direction"=0] → ["direction"=0]
func stripStateKey(states, key string) string {
	pat := key + "\"="
	// 匹配 ,"key"=val 或 ["key"=val
	// 从值开始删到下一个逗号或结尾
	for i := 0; i < len(states); i++ {
		if strings.HasPrefix(states[i:], pat) {
			start := i
			end := start + len(pat)
			// 跳过数字值
			for end < len(states) && states[end] >= '0' && states[end] <= '9' {
				end++
			}
			// 跳过前面的逗号或左括号
			if start > 0 && states[start-1] == ',' {
				start--
			}
			// 跳过后面的逗号
			if end < len(states) && states[end] == ',' {
				end++
			}
			states = states[:start] + states[end:]
			i = start - 1
		}
	}
	// 清理空的方括号
	if states == "[\"\"]" || states == "[]" {
		return "[]"
	}
	return states
}

func shortName(path string) string {
	// 文件名去后缀
	name := filepath.Base(path)
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[:i]
		}
	}
	return name
}

func sendActionbar(msg string) {
	bm := getBotManager()
	if bm != nil && bm.IsConnected() {
		escaped := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r", "\t", "\\t").Replace(msg)
		bm.SendWOCmd(fmt.Sprintf(`titleraw @a actionbar {"rawtext":[{"text":"%s"}]}`, escaped))
	}
}

func debugLog(format string, args ...any) {
	f, err := os.OpenFile("/sdcard/Download/prism_debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, time.Now().Format("15:04:05.000")+" "+format+"\n", args...)
}

func runImportTask(tc *state.TaskController, p map[string]any, resumeFromCheckpoint string) {
	path := getString(p, "path", "")
	if path == "" {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "缺少文件路径"}))
		return
	}

	bm := getBotManager()
	hub.Emit("task_start", mustJSON(map[string]any{"type": "导入: " + shortName(path)}))
	hub.Emit("task_progress", mustJSON(map[string]any{"progress": 0}))
	hub.Emit("progress", mustJSON(map[string]any{"msg": "加载建筑文件: " + shortName(path)}))

	// 输出缓存命中状态
	if building.IsCached(path) {
		hub.Emit("progress", mustJSON(map[string]any{"msg": "OK新鲜的缓存"}))
	} else {
		hub.Emit("progress", mustJSON(map[string]any{"msg": "缓存消失了"}))
	}

	data, err := loadStructureFileCached(path)
	if err == nil && data != nil {
		debugLog("CACHE: import cache %s (%d blocks, %dx%dx%d)", shortName(path), len(data.Blocks), data.SizeX, data.SizeY, data.SizeZ)
	}
	if err != nil {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "加载文件失败: " + err.Error()}))
		return
	}

	// Apply rotation before import
	rotation := getInt(p, "rotation", 0)
	if rotation != 0 {
		hub.Emit("progress", mustJSON(map[string]any{"msg": fmt.Sprintf("旋转建筑: %d°", rotation)}))
		building.RotateBlocksInPlace(data, rotation)
	}

	stats := data.ComputeStats()
	hub.Emit("progress", mustJSON(map[string]any{
		"msg": fmt.Sprintf("建筑: %dx%dx%d, %d 实体方块, %d 种方块", data.SizeX, data.SizeY, data.SizeZ, stats.SolidBlocks, len(stats.BlockCountsList)),
	}))
	if bm != nil && bm.IsOP() {
		bm.SendTellraw(fmt.Sprintf("§a§l▍建筑导入:§r§f%s §7尺寸:%dx%dx%d §7方块:%d §7速度:%d/s §7坐标:(%d,%d,%d)",
			shortName(path), data.SizeX, data.SizeY, data.SizeZ, stats.SolidBlocks, getInt(p, "speed", 9500), getInt(p, "x", 0), getInt(p, "y", 64), getInt(p, "z", 0)))
	}

	bm = getBotManager()
	if bm == nil || !bm.IsConnected() {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "未连接到服务器，请先连接"}))
		return
	}
	// Create checkpoint
	taskID := uuid.New().String()
	serverCode := ""
	if bm != nil {
		serverCode = bm.ServerCode()
	}
	cp := &state.Checkpoint{
		TaskID:     taskID,
		Type:       "import",
		Status:     "running",
		Progress:   0,
		CreatedAt:  time.Now().Unix(),
		Params:     rawParams(p),
		Name:       shortName(path),
		ServerCode: serverCode,
	}
	state.SaveCheckpointNow(cp)

	// Force creative mode before import (silent)
	if bm != nil {
		bm.SendCommand("/gamemode creative " + bm.quotedName())
		time.Sleep(300 * time.Millisecond)
	}

	// Load resume data if any
	startChunkX := 0
	startChunkZ := 0
	if resumeFromCheckpoint != "" {
		if c, err := state.LoadCheckpoint(resumeFromCheckpoint); err == nil {
			var rd building.ImportCheckpointData
			json.Unmarshal(c.Data, &rd)
			startChunkX = rd.LastCX
			startChunkZ = rd.LastCZ
			hub.Emit("progress", mustJSON(map[string]any{
				"msg": fmt.Sprintf("从断点恢复: 区块 (%d,%d)", startChunkX, startChunkZ),
			}))
		}
	}

	importCmds := getBool(p, "import_commands", true)
	cmdDisabled := getBool(p, "cmd_disabled", false)
	excludeFluids := getBool(p, "exclude_fluids", false)
	excludeWater := excludeFluids && getBool(p, "exclude_water", true)
	excludeWaterlogged := excludeFluids && getBool(p, "exclude_waterlogged", true)
	excludeLava := excludeFluids && getBool(p, "exclude_lava", true)
	dimension := getString(p, "dimension", "overworld")
	if bm != nil {
		bm.SetDimension(dimension)
	}
	regionMode := getInt(p, "region_mode", 1)
	if regionMode < 1 {
		regionMode = 1
	}
	if regionMode > 3 {
		regionMode = 3
	}

	hub.Emit("progress", mustJSON(map[string]any{"msg": "正在计算实体方块..."}))
	stats = data.ComputeStats()
	totalSolid := 0
	for _, bi := range data.Blocks {
		if bi != 0 {
			totalSolid++
		}
	}

	var (
		abMu              sync.Mutex
		abTotalBarPct     int
		abSubBarPct       int
		abBlocks          int
		abRegionIdx       int
		abTotalRegions    int
		abClearing        bool
		abClearCleared    int
		abClearTotal      int
		targetImportSpeed int
	)
	// 获取自定义品牌名用于进度条
	importBrandName := "Prism"
	if n := GetEffectiveName(); n != "" {
		if len([]rune(n)) > 8 {
			n = string([]rune(n)[:8])
		}
		importBrandName = n
	}
	importStartTime := time.Now()
	abStopTicker := make(chan struct{})
	var consoleCenter protocol.BlockPos
	// 每个 NBT 方块用唯一结构名，避免固定名字被服务端结构缓存复用导致读到旧数据
	nbtStructSeq := 0
	task := &building.ImportTask{
		Data:         data,
		BaseX:        getInt(p, "x", 0),
		BaseY:        getInt(p, "y", 64),
		BaseZ:        getInt(p, "z", 0),
		Speed:        max(1, min(getInt(p, "speed", 9500), 20000)),
		StartCX:      startChunkX,
		StartCZ:      startChunkZ,
		PreClearMode: getInt(p, "pre_clear_mode", 0),
		Dimension:    dimension,
		RegionMode:   regionMode,
		ChunkLoaded: func(cx, cz int) bool {
			if bm == nil || bm.conn == nil {
				return true
			}
			// 用 testforblock 检测区块中心是否加载：区块已加载时命令能成功返回
			// （无论目标方块是否存在），未加载时命令超时或报错。
			posX := cx*16 + 8
			posZ := cz*16 + 8
			_, isTimeout, err := bm.SendWSCommandWithTimeout(
				fmt.Sprintf("testforblock %d 1 %d minecraft:air", posX, posZ),
				1*time.Second,
			)
			if isTimeout || err != nil {
				return false
			}
			return true
		},
		DenyEnable:         getBool(p, "deny_enable", false),
		DenyYOffset:        getInt(p, "deny_y_offset", -1),
		BorderEnable:       getBool(p, "border_enable", false),
		BorderBlock:        getString(p, "border_block", "border_block"),
		BorderYRel:         getInt(p, "border_y", 0),
		ImportCommands:     importCmds,
		CmdDisabled:        cmdDisabled,
		ExcludeWater:       excludeWater,
		ExcludeWaterlogged: excludeWaterlogged,
		ExcludeLava:        excludeLava,
		StopOnError:        true,
		SetBlock: func(x, y, z int32, name, states string) error {
			nm, st := normalizeBlockName(name), normalizeBlockStates(name, states)
			if st == "" || st == "[]" {
				return bm.SendBlockCmd(fmt.Sprintf("setblock %d %d %d %s", x, y, z, nm))
			}
			return bm.SendBlockCmd(fmt.Sprintf("setblock %d %d %d %s %s", x, y, z, nm, st))
		},
		FillRegion: func(x1, y1, z1, x2, y2, z2 int32, name, states string) error {
			nm, st := normalizeBlockName(name), normalizeBlockStates(name, states)
			if st == "" || st == "[]" {
				return bm.SendBlockCmd(fmt.Sprintf("fill %d %d %d %d %d %d %s", x1, y1, z1, x2, y2, z2, nm))
			}
			return bm.SendBlockCmd(fmt.Sprintf("fill %d %d %d %d %d %d %s %s", x1, y1, z1, x2, y2, z2, nm, st))
		},
		Teleport: func(x, y, z int) error {
			return bm.TP(x, y, z)
		},
		OnClearProgress: func(cleared, total int) {
			abMu.Lock()
			abClearing = true
			abClearCleared = cleared
			abClearTotal = total
			abMu.Unlock()
			hub.Emit("clear_progress", mustJSON(map[string]any{
				"progress": float64(cleared) / float64(total),
				"cleared":  cleared,
				"total":    total,
			}))
		},
		OnClearDone: func() {
			abMu.Lock()
			abClearing = false
			abMu.Unlock()
			hub.Emit("clear_done", mustJSON(map[string]any{"done": true}))
		},
		OnNBT: func(x, y, z int32, nbtData map[string]any, blockName, blockStates string) {
			defer func() {
				if r := recover(); r != nil {
					hub.Emit("progress", mustJSON(map[string]any{
						"msg": fmt.Sprintf("NBT 方块处理异常 (%d,%d,%d %s): %v", x, y, z, blockName, r),
					}))
				}
			}()
			if bm == nil || bm.conn == nil {
				return
			}

			// 等待 setblock 生效后再处理 NBT

			// 告示牌：保持现有路径（自带传送 + 点击打开 + 写文字）
			if strings.Contains(blockName, "sign") {
				bm.TP(int(x), int(y+1), int(z))
				if err := bm.PlaceSign(x, y, z, blockName, blockStates, nbtData); err != nil {
					hub.Emit("progress", mustJSON(map[string]any{
						"msg": fmt.Sprintf("告示牌放置失败 (%d,%d,%d): %v", x, y, z, err),
					}))
				}
				return
			}

			// 指令方块: TP 靠近后发送 CommandBlockUpdate 包
			bm.TP(int(x), int(y+1), int(z))
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
				bm.conn.WritePacket(&packet.CommandBlockUpdate{
					Block: true, Position: protocol.BlockPos{x, y, z},
					Mode: mode, NeedsRedstone: needsRS, Conditional: cond,
					Command: cmd, Name: name,
					ShouldTrackOutput: trackOut, ExecuteOnFirstTick: exeFirst,
					TickDelay: tickDelay,
				})
				return
			}

			// 结构方块: TP 靠近后发送 StructureBlockUpdate 包
			bm.TP(int(x), int(y+1), int(z))
			if strings.Contains(blockName, "structure_block") {
				upd := buildStructureBlockUpdate(nbtData)
				upd.Position = protocol.BlockPos{x, y, z}
				bm.conn.WritePacket(&upd)
				return
			}
			// TP 到目标位置确保区块加载（NBTAssigner 和 BlockActorData 都需要）
			bm.TP(int(x), int(y+1), int(z))
			time.Sleep(5 * time.Millisecond)

			handled := false

			// 需要工作台的判定复用 building.IsWorkspaceNBTBlock（§8.1 单一事实来源，
			// 与切岛分类器一致；空容器/空展示框等无可制作内容 → false，归普通单元）。
			needsNBT := building.IsWorkspaceNBTBlock(blockName, nbtData)

			if !handled {
				if needsNBT {
					statesMap := utils.ParseBlockStatesString(blockStates)
					cleanName := strings.ReplaceAll(strings.TrimPrefix(blockName, "minecraft:"), ":", "_")
					// 重试最多 3 次，每次强制重建操作台，规避授权后首次初始化竞态导致的命令超时
					for attempt := 0; attempt < 3; attempt++ {
						bm.cleanupNBTWorkspace() // 重置 nbtAssigner/nbtConsole，强制重建
						debugLog("NBT: 重试第%d次 getNBTAssigner block=%s pos=(%d,%d,%d)", attempt+1, blockName, x, y, z)
						assigner, aErr := bm.getNBTAssigner(consoleCenter)
						if aErr != nil {
							debugLog("NBT: getNBTAssigner 失败(第%d次): %v", attempt+1, aErr)
							time.Sleep(500 * time.Millisecond)
							continue
						}
						// 先传送到工作区，让 PlaceNBTBlock 能正常操作（铁砧/织布机/容器交互需要物理接近）
						debugLog("NBT: getNBTAssigner OK, TP 到工作区 center=%v", consoleCenter)
						bm.TP(int(consoleCenter[0]), int(consoleCenter[1]+1), int(consoleCenter[2]))
						time.Sleep(50 * time.Millisecond)

						_, _, _, nbtErr := assigner.PlaceNBTBlock(blockName, statesMap, nbtData)
						debugLog("NBT: PlaceNBTBlock 完成 err=%v", nbtErr)
						if nbtErr != nil {
							debugLog("NBT: PlaceNBTBlock 失败(第%d次): %v", attempt+1, nbtErr)
							time.Sleep(500 * time.Millisecond)
							continue
						}
						// 用方块扁平化名做结构名基础（如 chest/jukebox），尾部加序号保证不冲突，
						// 前缀带机器人编号，跨机器人唯一（§8.2）
						structID := building.NBTStructName(bm.BotIndex, cleanName, nbtStructSeq)
						nbtStructSeq++
						saveCmd := fmt.Sprintf("structure save \"%s\" %d %d %d %d %d %d", structID, consoleCenter[0], consoleCenter[1], consoleCenter[2], consoleCenter[0], consoleCenter[1], consoleCenter[2])
						debugLog("save: %s", saveCmd)
						if err := bm.SendPlayerCommand(saveCmd); err != nil {
							debugLog("save error: %v", err)
						}
						time.Sleep(100 * time.Millisecond)
						bm.TP(int(x), int(y+1), int(z))
						time.Sleep(100 * time.Millisecond)
						loadCmd := fmt.Sprintf("structure load \"%s\" %d %d %d", structID, x, y, z)
						debugLog("load: 准备发送")
						if err := bm.SendAICommand(loadCmd); err != nil {
							debugLog("load error: %v", err)
						}
						debugLog("load: 发送完成")
						time.Sleep(100 * time.Millisecond)
						bm.SendPlayerCommand(fmt.Sprintf("structure delete \"%s\"", structID))
						handled = true
						break
					}
				}
			} // 回退：简单方块实体补发 BlockActorData
			if !handled {
				if !strings.Contains(blockName, "command_block") &&
					!strings.Contains(blockName, "structure_block") &&
					!strings.Contains(blockName, "sign") {
					if nbtData["Items"] == nil && nbtData["Command"] == nil {
						norm := building.NormalizeBlockEntityNBT(blockName, nbtData)
						norm["x"] = x
						norm["y"] = y
						norm["z"] = z
						bm.conn.WritePacket(&packet.BlockActorData{
							Position: protocol.BlockPos{x, y, z},
							NBTData:  norm,
						})
						time.Sleep(10 * time.Millisecond)
					}
				}
			}
		},
		OnProgress: func(cx, cz, chunk, total, blocks int) {
			progress := float64(chunk) / float64(total)
			if progress > 1.0 {
				progress = 1.0
			}
			abMu.Lock()
			abTotalBarPct = int(progress * 100)
			abBlocks = blocks
			abMu.Unlock()
			hub.Emit("task_progress", mustJSON(map[string]any{
				"progress":     progress,
				"sub_progress": 1.0,
				"chunk":        chunk,
				"total":        total,
				"blocks":       blocks,
			}))
			hub.Emit("progress", mustJSON(map[string]any{
				"msg": fmt.Sprintf("区块 %d/%d, %d 方块", chunk, total, blocks),
			}))

			seq := chunk - 1
			d := building.ImportCheckpointData{
				LastCX:       seq / ((data.SizeZ + 15) / 16),
				LastCZ:       seq % ((data.SizeZ + 15) / 16),
				BlocksPlaced: blocks,
			}

			cp.Data, _ = json.Marshal(d)
			cp.Message = fmt.Sprintf("区块 %d/%d", chunk, total)
			state.SaveCheckpoint(cp)
			cp.Progress = progress
		},
		OnSubProgress: func(cx, cz, placed, chunkTotal int) {
			sp := float64(placed) / float64(chunkTotal)
			if sp > 1.0 {
				sp = 1.0
			}
			if chunkTotal == 0 {
				sp = 1.0
			}
			abMu.Lock()
			abSubBarPct = int(sp * 100)
			abMu.Unlock()
			hub.Emit("task_progress", mustJSON(map[string]any{
				"sub_progress": sp,
			}))
		},
		OnRegionProgress: func(ri, totalRegions, rx, rz int) {
			abMu.Lock()
			abRegionIdx = ri
			abTotalRegions = totalRegions
			abMu.Unlock()
		},
		StopCh: tc.StopCh(),
	}
	targetImportSpeed = task.Speed
	// 设置 NBT 操作台位置并清空旧缓存（每个导入任务重建，确保位置正确）
	if bm != nil {
		// 工作台坐标按机器人编号分配，跨机器人互不重叠（§8.3）
		wx, wy, wz := building.WorkspaceCenter(bm.BotIndex, getInt(p, "x", 0), getInt(p, "z", 0), data.SizeX, data.SizeZ)
		consoleCenter = protocol.BlockPos{int32(wx), int32(wy), int32(wz)}
		// 异步清理工作区块（fire-and-forget，不阻塞导入启动）
		bmCopy := bm
		centerCopy := consoleCenter
		go func() {
			if bmCopy != nil && bmCopy.conn != nil && centerCopy != (protocol.BlockPos{}) {
				bmCopy.SendPlayerCommand(fmt.Sprintf("execute as @s at @s run fill %d %d %d %d %d %d air",
					centerCopy[0]-5, centerCopy[1]-2, centerCopy[2]-5,
					centerCopy[0]+5, centerCopy[1]+2, centerCopy[2]+5))
			}
		}()
		bm.cleanupNBTWorkspace()
	}

	// 定时刷新 actionbar（每 500ms），让旋转动画连续不卡顿
	abTicker := time.NewTicker(500 * time.Millisecond)
	defer abTicker.Stop()
	go func() {
		var speedLastBlocks int
		var speedLastTime = time.Now()
		var lastRealSpeed int
		for {
			select {
			case <-abTicker.C:
				abMu.Lock()
				clearing := abClearing
				if clearing {
					cCleared := abClearCleared
					cTotal := abClearTotal
					abMu.Unlock()
					sendActionbar(buildClearActionbar(cCleared, cTotal))
				} else {
					blocks := abBlocks
					totalPct := abTotalBarPct
					subPct := abSubBarPct
					rIdx := abRegionIdx
					rTotal := abTotalRegions
					abMu.Unlock()
					elapsed := time.Since(importStartTime).Seconds()
					// 每秒更新一次速度，非更新时刻复用上次速度
					now := time.Now()
					realSpeed := lastRealSpeed
					if now.Sub(speedLastTime) >= time.Second {
						dt := now.Sub(speedLastTime).Seconds()
						if dt >= 0.5 {
							realSpeed = int(float64(blocks-speedLastBlocks) / dt)
							speedLastBlocks = blocks
							speedLastTime = now
							lastRealSpeed = realSpeed
						}
					}
					// 第一秒内用平均速度估算
					if lastRealSpeed == 0 && elapsed > 0.1 {
						realSpeed = int(float64(blocks) / elapsed)
					}
					buildV5Actionbar(sendActionbar, shortName(path), data.SizeX, data.SizeY, data.SizeZ,
						regionMode, totalSolid, targetImportSpeed, realSpeed, blocks, totalPct, subPct,
						rIdx, rTotal, importStartTime, importBrandName)
				}
			case <-abStopTicker:
				return
			}
		}
	}()
	defer close(abStopTicker)

	// 导入期间抑制游戏内消息（聊天/加入/退出等）转发到终端，避免大量提示冲击
	setSuppressGameLogs(true)
	if err := task.Run(); err != nil {
		setSuppressGameLogs(false)
		cp.Status = "failed"
		cp.Message = err.Error()
		state.SaveCheckpoint(cp)
		hub.Emit("task_error", mustJSON(map[string]any{"error": err.Error()}))
		return
	}
	setSuppressGameLogs(false)

	// 完成提示：聊天栏 tellraw + 行动栏强调
	if bm != nil && bm.IsOP() {
		bm.SendTellraw(fmt.Sprintf("§a§l▍导入完成:§r§f%s §7共%d方块", shortName(path), totalSolid))
	}

	buildV5Actionbar(sendActionbar, shortName(path), data.SizeX, data.SizeY, data.SizeZ,
		regionMode, totalSolid, targetImportSpeed, 0, totalSolid, 100, 100, 0, 0, importStartTime, importBrandName)

	cp.Status = "done"
	// 清空工作区方块
	if bm != nil && bm.conn != nil && consoleCenter != (protocol.BlockPos{}) {
		bm.SendPlayerCommand(fmt.Sprintf("fill %d %d %d %d %d %d air",
			consoleCenter[0]-5, consoleCenter[1]-2, consoleCenter[2]-5,
			consoleCenter[0]+5, consoleCenter[1]+2, consoleCenter[2]+5))
	}
	bm.cleanupNBTWorkspace()

	cp.Progress = 1.0
	state.SaveCheckpoint(cp)
	// 记录心跳增量
	recordHeartbeatIncrement(IncrementData{
		BlocksImported:    totalSolid,
		BuildingsImported: 1,
		ImportSessions:    1,
	})
	hub.Emit("task_done", mustJSON(map[string]any{"msg": "导入完成", "task_id": taskID}))
}

func runRepairTask(tc *state.TaskController, p map[string]any) {
	path := getString(p, "path", "")
	if path == "" {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "缺少文件路径"}))
		return
	}

	// 修补模式必须提供起始坐标，不做默认值
	x := getInt(p, "x", 0)
	y := getInt(p, "y", 0)
	z := getInt(p, "z", 0)
	if x == 0 && y == 0 && z == 0 {
		// 检查是否真的都是 0，还是用户没传
		if _, hasX := p["x"]; !hasX {
			hub.Emit("task_error", mustJSON(map[string]any{"error": "修补模式必须提供起始坐标 (x, y, z)"}))
			return
		}
		if _, hasY := p["y"]; !hasY {
			hub.Emit("task_error", mustJSON(map[string]any{"error": "修补模式必须提供起始坐标 y"}))
			return
		}
		if _, hasZ := p["z"]; !hasZ {
			hub.Emit("task_error", mustJSON(map[string]any{"error": "修补模式必须提供起始坐标 z"}))
			return
		}
	}

	hub.Emit("task_start", mustJSON(map[string]any{"type": "修补: " + shortName(path)}))
	hub.Emit("task_progress", mustJSON(map[string]any{"progress": 0}))
	hub.Emit("progress", mustJSON(map[string]any{"msg": "加载缓存的建筑文件: " + shortName(path)}))

	// 加载缓存（必须已存在，即之前已导入过此文件）
	data, err := loadStructureFileCached(path)
	if err != nil || data == nil {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "无法加载建筑文件缓存，请先正常导入该文件"}))
		return
	}

	rotation := getInt(p, "rotation", 0)
	if rotation != 0 {
		hub.Emit("progress", mustJSON(map[string]any{"msg": fmt.Sprintf("旋转建筑: %d°", rotation)}))
		building.RotateBlocksInPlace(data, rotation)
	}

	stats := data.ComputeStats()
	hub.Emit("progress", mustJSON(map[string]any{
		"msg": fmt.Sprintf("建筑: %dx%dx%d, %d 实体方块, 修补坐标:(%d,%d,%d)", data.SizeX, data.SizeY, data.SizeZ, stats.SolidBlocks, x, y, z),
	}))

	bm := getBotManager()
	if bm == nil || !bm.IsConnected() {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "未连接到服务器，请先连接"}))
		return
	}

	// 修补模式默认速度 20 方块/秒，不跟随全局速度
	speed := getInt(p, "speed", 20)
	if speed < 1 {
		speed = 20
	}

	// 解析修补区域参数
	useCircle := getBool(p, "repair_circle", false)
	repairCX := int32(getInt(p, "repair_cx", 0))
	repairCZ := int32(getInt(p, "repair_cz", 0))
	repairRadius := int32(getInt(p, "repair_radius", 0))
	repairX1 := int32(getInt(p, "repair_x1", 0))
	repairY1 := int32(getInt(p, "repair_y1", 0))
	repairZ1 := int32(getInt(p, "repair_z1", 0))
	repairX2 := int32(getInt(p, "repair_x2", 0))
	repairY2 := int32(getInt(p, "repair_y2", 0))
	repairZ2 := int32(getInt(p, "repair_z2", 0))

	importCmds := getBool(p, "import_commands", true)
	cmdDisabled := getBool(p, "cmd_disabled", false)
	dimension := getString(p, "dimension", "overworld")
	if bm != nil {
		bm.SetDimension(dimension)
	}
	excludeWater := getBool(p, "exclude_water", false)
	excludeWaterlogged := getBool(p, "exclude_waterlogged", false)
	excludeLava := getBool(p, "exclude_lava", false)

	// 强制 creative 模式
	if bm != nil {
		bm.SendCommand("/gamemode creative " + bm.quotedName())
		time.Sleep(300 * time.Millisecond)
	}

	var consoleCenter protocol.BlockPos
	nbtStructSeq := 0

	task := &building.ImportTask{
		Data:               data,
		BaseX:              x,
		BaseY:              y,
		BaseZ:              z,
		Speed:              speed,
		ImportCommands:     importCmds,
		CmdDisabled:        cmdDisabled,
		ExcludeWater:       excludeWater,
		ExcludeWaterlogged: excludeWaterlogged,
		ExcludeLava:        excludeLava,
		StopOnError:        true,
		RepairMode:         true,
		RepairNBTOnly:      getBool(p, "repair_nbt_only", false),
		RepairUseCircle:    useCircle,
		RepairCenterX:      repairCX,
		RepairCenterZ:      repairCZ,
		RepairRadius:       repairRadius,
		RepairRect:         [6]int32{repairX1, repairY1, repairZ1, repairX2, repairY2, repairZ2},

		SetBlock: func(x, y, z int32, name, states string) error {
			nm, st := normalizeBlockName(name), normalizeBlockStates(name, states)
			if st == "" || st == "[]" {
				return bm.SendBlockCmd(fmt.Sprintf("setblock %d %d %d %s", x, y, z, nm))
			}
			return bm.SendBlockCmd(fmt.Sprintf("setblock %d %d %d %s %s", x, y, z, nm, st))
		},
		Teleport: func(x, y, z int) error {
			return bm.TP(x, y, z)
		},
		OnNBT: func(x, y, z int32, nbtData map[string]any, blockName, blockStates string) {
			if bm == nil || bm.conn == nil {
				return
			}
			// 告示牌
			if strings.Contains(blockName, "sign") {
				bm.TP(int(x), int(y+1), int(z))
				if err := bm.PlaceSign(x, y, z, blockName, blockStates, nbtData); err != nil {
					hub.Emit("progress", mustJSON(map[string]any{
						"msg": fmt.Sprintf("告示牌放置失败 (%d,%d,%d): %v", x, y, z, err),
					}))
				}
				return
			}
			// 命令方块
			if cmd, ok := nbtData["Command"].(string); ok {
				bm.TP(int(x), int(y+1), int(z))
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
				bm.conn.WritePacket(&packet.CommandBlockUpdate{
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
				bm.TP(int(x), int(y+1), int(z))
				upd := buildStructureBlockUpdate(nbtData)
				upd.Position = protocol.BlockPos{x, y, z}
				bm.conn.WritePacket(&upd)
				return
			}
			// 需要 NBT 工作区的方块（容器、展示框等），复用 building 分类器（§8.1）
			needsNBT := building.IsWorkspaceNBTBlock(blockName, nbtData)
			if needsNBT {
				assigner, aErr := bm.getNBTAssigner(consoleCenter)
				if aErr == nil {
					bm.TP(int(consoleCenter[0]), int(consoleCenter[1]+1), int(consoleCenter[2]))
					time.Sleep(50 * time.Millisecond)
					statesMap := utils.ParseBlockStatesString(blockStates)
					_, _, _, _ = assigner.PlaceNBTBlock(blockName, statesMap, nbtData)
					cleanName := strings.ReplaceAll(strings.TrimPrefix(blockName, "minecraft:"), ":", "_")
					structID := building.NBTStructName(bm.BotIndex, cleanName, nbtStructSeq)
					nbtStructSeq++
					saveCmd := fmt.Sprintf("structure save \"%s\" %d %d %d %d %d %d", structID, consoleCenter[0], consoleCenter[1], consoleCenter[2], consoleCenter[0], consoleCenter[1], consoleCenter[2])
					bm.SendPlayerCommand(saveCmd)
					time.Sleep(100 * time.Millisecond)
					bm.TP(int(x), int(y+1), int(z))
					time.Sleep(100 * time.Millisecond)
					loadCmd := fmt.Sprintf("structure load \"%s\" %d %d %d", structID, x, y, z)
					bm.SendAICommand(loadCmd)
					time.Sleep(100 * time.Millisecond)
					bm.SendPlayerCommand(fmt.Sprintf("structure delete \"%s\"", structID))
				}
				return
			}
			// 其他方块实体：补发 BlockActorData
			if !strings.Contains(blockName, "command_block") &&
				!strings.Contains(blockName, "structure_block") &&
				!strings.Contains(blockName, "sign") {
				norm := building.NormalizeBlockEntityNBT(blockName, nbtData)
				norm["x"] = x
				norm["y"] = y
				norm["z"] = z
				bm.conn.WritePacket(&packet.BlockActorData{
					Position: protocol.BlockPos{x, y, z},
					NBTData:  norm,
				})
			}
		},
		OnProgress: func(cx, cz, chunk, total, blocks int) {
			progress := float64(chunk) / float64(total)
			if progress > 1.0 {
				progress = 1.0
			}
			hub.Emit("task_progress", mustJSON(map[string]any{"progress": progress}))
			hub.Emit("progress", mustJSON(map[string]any{
				"msg": fmt.Sprintf("区块 %d/%d, %d 方块", chunk, total, blocks),
			}))
		},
		StopCh: tc.StopCh(),
	}

	// 设置 NBT 操作台位置（同正常导入：距离起点最远的位置，按机器人编号分配不重叠 §8.3）
	if bm != nil {
		wx, wy, wz := building.WorkspaceCenter(bm.BotIndex, x, z, data.SizeX, data.SizeZ)
		consoleCenter = protocol.BlockPos{int32(wx), int32(wy), int32(wz)}
		go func() {
			if bm.conn != nil {
				bm.SendPlayerCommand(fmt.Sprintf("execute as @s at @s run fill %d %d %d %d %d %d air",
					consoleCenter[0]-5, consoleCenter[1]-2, consoleCenter[2]-5,
					consoleCenter[0]+5, consoleCenter[1]+2, consoleCenter[2]+5))
			}
		}()
		bm.cleanupNBTWorkspace()
	}

	hub.Emit("progress", mustJSON(map[string]any{"msg": "开始修补..."}))
	setSuppressGameLogs(true)
	err = task.Run()
	if err != nil {
		setSuppressGameLogs(false)
		hub.Emit("task_error", mustJSON(map[string]any{"error": fmt.Sprintf("修补失败: %v", err)}))
		return
	}
	setSuppressGameLogs(false)
	hub.Emit("task_done", mustJSON(map[string]any{"msg": "修补完成"}))
}

func runExportTask(tc *state.TaskController, p map[string]any) {
	sx := getInt(p, "x1", 0)
	sy := getInt(p, "y1", 0)
	sz := getInt(p, "z1", 0)
	ex := getInt(p, "x2", 16)
	ey := getInt(p, "y2", 16)
	ez := getInt(p, "z2", 16)

	bm := getBotManager()
	if bm == nil || !bm.IsConnected() {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "未连接到服务器"}))
		return
	}

	outputPath := getString(p, "output", "/sdcard/Download/export.mcstructure")
	format := getString(p, "format", "mcstructure")

	// Auto-rename if file exists: append (1), (2), ...
	if outputPath != "" {
		base := outputPath
		ext := ""
		if i := strings.LastIndex(base, "."); i >= 0 {
			ext = base[i:]
			base = base[:i]
		}
		for i := 1; i <= 1000; i++ {
			if _, err := os.Stat(outputPath); os.IsNotExist(err) {
				break
			}
			outputPath = fmt.Sprintf("%s(%d)%s", base, i, ext)
		}
	}

	if sx > ex {
		sx, ex = ex, sx
	}
	if sy > ey {
		sy, ey = ey, sy
	}
	if sz > ez {
		sz, ez = ez, sz
	}

	dx := ex - sx + 1
	dy := ey - sy + 1
	dz := ez - sz + 1

	hub.Emit("progress", mustJSON(map[string]any{
		"msg": fmt.Sprintf("导出: (%d,%d,%d)→(%d,%d,%d) %dx%dx%d", sx, sy, sz, ex, ey, ez, dx, dy, dz),
	}))

	if bm.IsOP() {
		bm.SendTellraw(fmt.Sprintf("§a§l▍导出:§r§f格式:%s §7%dx%dx%d", format, dx, dy, dz))
	}

	// Create checkpoint
	taskID := uuid.New().String()
	serverCode := ""
	if bm != nil {
		serverCode = bm.ServerCode()
	}
	cp := &state.Checkpoint{
		TaskID:     taskID,
		Type:       "export",
		Status:     "running",
		Progress:   0,
		CreatedAt:  time.Now().Unix(),
		Params:     rawParams(p),
		Name:       fmt.Sprintf("导出 %dx%dx%d", dx, dy, dz),
		ServerCode: serverCode,
	}
	state.SaveCheckpointNow(cp)

	// mcworld / mcstructure / schematic: 统一走 RequestStructure 获取数据
	// mcworld: 通过 RequestStructure 获取含 NBT 的结构数据，写入 LevelDB 打包
	// mcstructure/schematic: 通过 RequestStructure 获取数据，NBT 编码写入文件
	chunkSize := 16
	chunkCountX := (dx + chunkSize - 1) / chunkSize
	chunkCountZ := (dz + chunkSize - 1) / chunkSize
	totalChunks := chunkCountX * chunkCountZ

	var allChunks []map[string]any
	successChunks := 0

	for cx := 0; cx < chunkCountX; cx++ {
		for cz := 0; cz < chunkCountZ; cz++ {
			select {
			case <-tc.StopCh():
				cp.Status = "failed"
				cp.Message = "导出取消"
				state.SaveCheckpoint(cp)
				hub.Emit("task_error", mustJSON(map[string]any{"error": "导出取消"}))
				return
			default:
			}

			csx := sx + cx*chunkSize
			csz := sz + cz*chunkSize
			csizex := min(chunkSize, dx-cx*chunkSize)
			csizez := min(chunkSize, dz-cz*chunkSize)
			chunkNum := cx*chunkCountZ + cz + 1

			hub.Emit("progress", mustJSON(map[string]any{
				"msg": fmt.Sprintf("区块 %d/%d: (%d,%d) %dx%d", chunkNum, totalChunks, csx, csz, csizex, csizez),
			}))
			hub.Emit("task_progress", mustJSON(map[string]any{"progress": float64(chunkNum-1) / float64(totalChunks)}))

			bm.TP(csx+csizex/2, sy+dy/2, csz+csizez/2)
			time.Sleep(200 * time.Millisecond)

			nbtData, err := bm.RequestStructure(
				[3]int32{int32(csx), int32(sy), int32(csz)},
				[3]int32{int32(csizex), int32(dy), int32(csizez)},
				8*time.Second,
			)
			if err != nil {
				hub.Emit("progress", mustJSON(map[string]any{"msg": fmt.Sprintf("区块 %d 失败: %v，跳过", chunkNum, err)}))
				continue
			}
			// 提取 block_position_data（箱子/命令方块等 NBT 数据）
			extractBlockEntities(nbtData)
			allChunks = append(allChunks, nbtData)
			successChunks++
			cp.Progress = float64(chunkNum) / float64(totalChunks)
			cp.Message = fmt.Sprintf("区块 %d/%d", chunkNum, totalChunks)
			state.SaveCheckpoint(cp)
		}
	}

	if successChunks == 0 {
		cp.Status = "failed"
		cp.Message = "所有区块获取失败"
		state.SaveCheckpoint(cp)
		hub.Emit("task_error", mustJSON(map[string]any{"error": "所有区块获取失败"}))
		return
	}

	hub.Emit("progress", mustJSON(map[string]any{"msg": fmt.Sprintf("收到 %d 个区块数据，正在编码...", successChunks)}))

	var outData []byte
	if format == "mcworld" {
		merged := mergeChunkStructures(allChunks, int32(dx), int32(dy), int32(dz))
		if err := building.WriteMergedStructureToMCWorld(merged, int32(dx), int32(dy), int32(dz), outputPath, int32(sx), int32(sy), int32(sz)); err != nil {
			cp.Status = "failed"
			cp.Message = err.Error()
			state.SaveCheckpoint(cp)
			hub.Emit("task_error", mustJSON(map[string]any{"error": "mcworld 导出失败: " + err.Error()}))
			return
		}
		hub.Emit("progress", mustJSON(map[string]any{"msg": fmt.Sprintf("已写入: %s", outputPath)}))
	} else if format == "mcstructure" {
		mergedStructure := mergeChunkStructures(allChunks, int32(dx), int32(dy), int32(dz))
		var buf bytes.Buffer
		enc := nbt.NewEncoderWithEncoding(&buf, nbt.LittleEndian)
		root := map[string]any{
			"format_version": int32(1),
			"size":           []int32{int32(dx), int32(dy), int32(dz)},
			"structure":      mergedStructure,
		}
		if err := enc.Encode(root); err != nil {
			cp.Status = "failed"
			cp.Message = err.Error()
			state.SaveCheckpoint(cp)
			hub.Emit("task_error", mustJSON(map[string]any{"error": "编码失败: " + err.Error()}))
			return
		}
		outData = buf.Bytes()
	} else {
		if len(allChunks) == 0 {
			cp.Status = "failed"
			cp.Message = "没有区块数据可编码"
			state.SaveCheckpoint(cp)
			hub.Emit("task_error", mustJSON(map[string]any{"error": "没有区块数据可编码"}))
			return
		}
		merged := mergeChunkStructures(allChunks, int32(dx), int32(dy), int32(dz))
		outData = convertToSchematic(map[string]any{"structure": merged}, dx, dy, dz)
	}

	if outputPath != "" && len(outData) > 0 {
		os.MkdirAll(filepath.Dir(outputPath), 0755)
		if err := os.WriteFile(outputPath, outData, 0644); err != nil {
			cp.Status = "failed"
			cp.Message = err.Error()
			state.SaveCheckpoint(cp)
			hub.Emit("task_error", mustJSON(map[string]any{"error": "写入失败: " + err.Error()}))
			return
		}
		hub.Emit("progress", mustJSON(map[string]any{"msg": fmt.Sprintf("已写入: %s (%d 字节)", outputPath, len(outData))}))
	}

	bm.cleanupNBTWorkspace()

	cp.Status = "done"
	cp.Progress = 1.0
	state.SaveCheckpoint(cp)
	recordHeartbeatIncrement(IncrementData{
		BuildingsExported: 1,
		ExportSessions:    1,
	})
	hub.Emit("task_done", mustJSON(map[string]any{
		"msg":    fmt.Sprintf("导出完成: %d/%d 区块, %s", successChunks, totalChunks, outputPath),
		"output": outputPath,
		"format": format,
	}))
}

func convertToSchematic(nbtData map[string]any, w, h, l int) []byte {
	// Build Java Edition schematic (Sponge v2 format, BigEndian NBT + gzip)
	// 注意：mcstructure 索引 = x*h*l + y*l + z，Sponge v2 索引 = y*w*l + z*w + x
	// 需要做坐标重映射
	structure, _ := nbtData["structure"].(map[string]any)
	blockIndices, _ := structure["block_indices"].([]any)
	paletteData, _ := structure["palette"].(map[string]any)

	// Build simplified palette and block data
	palette := map[string]int32{"minecraft:air": 0}
	nextID := int32(1)
	total := w * h * l
	blockData := make([]int32, total)

	if blockIndices != nil && len(blockIndices) > 0 {
		layer0 := toIntSlice(blockIndices[0])
		for x := 0; x < w; x++ {
			for y := 0; y < h; y++ {
				for z := 0; z < l; z++ {
					// mcstructure 索引
					mcIdx := x*h*l + y*l + z
					if mcIdx >= len(layer0) {
						continue
					}
					idx := layer0[mcIdx]
					if idx < 0 {
						continue // air
					}
					// Get block name from palette
					blockName := "minecraft:stone"
					if pal, ok := paletteData["default"].(map[string]any); ok {
						if bp, ok := pal["block_palette"].([]any); ok && idx < len(bp) {
							if bm, ok := bp[idx].(map[string]any); ok {
								if n, ok := bm["name"].(string); ok {
									blockName = n
								}
							}
						}
					}
					pid, ok := palette[blockName]
					if !ok {
						pid = nextID
						palette[blockName] = nextID
						nextID++
					}
					// Sponge v2 索引 = y*w*l + z*w + x
					spongeIdx := y*w*l + z*w + x
					blockData[spongeIdx] = pid
				}
			}
		}
	}

	// schematic 格式不支持 NBT 方块实体数据，跳过 block_position_data

	root := map[string]any{
		"Version": int32(2),
		"Width":   int16(w), "Height": int16(h), "Length": int16(l),
		"BlockData": blockData, "Palette": palette,
		"Offset": [3]int32{0, 0, 0},
	}

	var buf bytes.Buffer
	enc := nbt.NewEncoderWithEncoding(&buf, nbt.BigEndian)
	if err := enc.Encode(root); err != nil {
		return nil
	}
	// Gzip
	var gzbuf bytes.Buffer
	gw := gzip.NewWriter(&gzbuf)
	gw.Write(buf.Bytes())
	gw.Close()
	return gzbuf.Bytes()
}

func toIntSlice(v any) []int {
	switch arr := v.(type) {
	case []any:
		r := make([]int, len(arr))
		for i, x := range arr {
			switch n := x.(type) {
			case int32:
				r[i] = int(n)
			case int:
				r[i] = n
			case float64:
				r[i] = int(n)
			}
		}
		return r
	case []int32:
		r := make([]int, len(arr))
		for i, n := range arr {
			r[i] = int(n)
		}
		return r
	}
	return nil
}

func makeUUIDSafe(id string) string {
	// Make UUID safe for Minecraft structure names (avoid censorship)
	result := make([]byte, 0, len(id)*2)
	for _, c := range id {
		if c == '-' {
			result = append(result, '_')
		} else {
			result = append(result, byte(c))
		}
	}
	return string(result)
}

// mapArtBlock 存储像素画单个像素对应的方块信息和高度偏移。
// heightOff 值：-1（暗色）/ 0（中色）/ +1（亮色），用于地图上产生立体起伏。
type mapArtBlock struct {
	info      *building.BlockInfo
	heightOff int
}

var gravityReplacements = map[string]string{
	"minecraft:sand":     "minecraft:sandstone",
	"minecraft:red_sand": "minecraft:red_sandstone",
	"minecraft:gravel":   "minecraft:stone",
}

func runMapArtTask(tc *state.TaskController, p map[string]any) {
	path := getString(p, "path", "")
	if path == "" {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "缺少图片路径"}))
		return
	}

	bm := getBotManager()
	if bm == nil || !bm.IsConnected() {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "未连接到服务器"}))
		return
	}

	hub.Emit("progress", mustJSON(map[string]any{"msg": "加载图片: " + filepath.Base(path)}))
	if bm.IsOP() {
		bm.SendTellraw(fmt.Sprintf("§a§l▍地图画:§r§f%s §7坐标:(%d,%d,%d)", "像素画", getInt(p, "x", 0), getInt(p, "y", 64), getInt(p, "z", 0)))
	}

	f, err := os.Open(path)
	if err != nil {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "无法打开图片: " + err.Error()}))
		return
	}
	defer f.Close()

	srcImg, _, err := image.Decode(f)
	if err != nil {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "图片解码失败: " + err.Error()}))
		return
	}

	orientation := getString(p, "orientation", "vertical")
	speed := max(1, min(getInt(p, "speed", 9500), 20000))
	useGravityBlocks := getBool(p, "use_gravity_blocks", false)
	gravityPlatform := getBool(p, "gravity_platform", false)
	useRelief := getBool(p, "use_relief", false)
	targetW := getInt(p, "target_w", 128)
	targetH := getInt(p, "target_h", 128)
	// Clamp to 128 multiples, max 1024
	if targetW < 128 {
		targetW = 128
	}
	if targetH < 128 {
		targetH = 128
	}
	if targetW > 1024 {
		targetW = 1024
	}
	if targetH > 1024 {
		targetH = 1024
	}

	baseX := getInt(p, "x", 0)
	baseY := getInt(p, "y", 64)
	baseZ := getInt(p, "z", 0)

	// 解析色彩空间和抖动算法参数
	colorSpaceStr := getString(p, "color_space", "lab")
	ditherStr := getString(p, "dither", "floyd_steinberg")
	cs := parseColorSpace(colorSpaceStr)
	dither := parseDitherAlgo(ditherStr)

	srcW, srcH := srcImg.Bounds().Dx(), srcImg.Bounds().Dy()
	hub.Emit("progress", mustJSON(map[string]any{"msg": fmt.Sprintf("图片缩放: %dx%d → %dx%d, 色彩空间: %s, 抖动: %s", srcW, srcH, targetW, targetH, colorSpaceStr, ditherStr)}))

	// 使用新引擎处理（含抖动、色彩空间匹配，不旋转）
	grid := media.ProcessMapArt(srcImg, targetW, targetH, cs, dither, media.Rotation0)

	// Convert grid to block map
	var sx, sy, sz int
	blocks := make(map[[3]int]*mapArtBlock)

	sleeptime := time.Second / time.Duration(speed)
	count := 0

	for y := 0; y < targetH; y++ {
		for x := 0; x < targetW; x++ {
			block := grid[y][x]
			if block.Name == "minecraft:air" {
				continue // 跳过透明像素
			}
			blockName := block.Name
			heightOff := block.HeightOff
			if !useGravityBlocks {
				if repl, ok := gravityReplacements[blockName]; ok {
					blockName = repl
				}
			}
			// 三级明暗偏移：亮色 +1，中色 0，暗色 -1
			// 让地图画在地图上产生立体起伏效果
			if orientation == "vertical" {
				blocks[[3]int{baseX, baseY + y, baseZ + x}] = &mapArtBlock{info: &building.BlockInfo{Name: blockName, States: "[]"}, heightOff: heightOff}
				if y+baseY+1 > sy {
					sy = y + baseY + 1
				}
				if x+baseZ+1 > sz {
					sz = x + baseZ + 1
				}
			} else {
				blocks[[3]int{baseX + x, baseY, baseZ + y}] = &mapArtBlock{info: &building.BlockInfo{Name: blockName, States: "[]"}, heightOff: heightOff}
				if x+baseX+1 > sx {
					sx = x + baseX + 1
				}
				if y+baseZ+1 > sz {
					sz = y + baseZ + 1
				}
			}
		}
	}

	hub.Emit("progress", mustJSON(map[string]any{"msg": fmt.Sprintf("像素画: %dx%d → %d 方块, 开始放置...", targetW, targetH, len(blocks))}))

	// 32×32区域优化：在每个32×32区域内做2D矩形填充优化
	regionSize := 32
	regionsW := (targetW + regionSize - 1) / regionSize
	regionsH := (targetH + regionSize - 1) / regionSize

	for rx := 0; rx < regionsW; rx++ {
		var ryStart, ryEnd, ryStep int
		if rx%2 == 0 {
			ryStart, ryEnd, ryStep = 0, regionsH, 1
		} else {
			ryStart, ryEnd, ryStep = regionsH-1, -1, -1
		}

		for ry := ryStart; ry != ryEnd; ry += ryStep {
			select {
			case <-tc.StopCh():
				hub.Emit("task_error", mustJSON(map[string]any{"error": "已取消"}))
				return
			default:
			}

			if gravityPlatform && orientation == "horizontal" && useGravityBlocks {
				rx0 := baseX + rx*regionSize
				rz0 := baseZ + ry*regionSize
				rx1 := rx0 + min(regionSize, targetW-rx*regionSize) - 1
				rz1 := rz0 + min(regionSize, targetH-ry*regionSize) - 1
				bm.SendBlockCmd(fmt.Sprintf("fill %d %d %d %d %d %d glass",
					rx0, baseY-1, rz0, rx1, baseY-1, rz1))
				time.Sleep(100 * time.Millisecond)
			}

			rStartW := rx * regionSize
			rStartH := ry * regionSize
			rSizeW := min(regionSize, targetW-rStartW)
			rSizeH := min(regionSize, targetH-rStartH)
			var midX, midY, midZ int
			if orientation == "vertical" {
				midX = baseX
				midY = baseY + rStartH + rSizeH/2
				midZ = baseZ + rStartW + rSizeW/2
			} else {
				midX = baseX + rStartW + rSizeW/2
				midY = baseY
				midZ = baseZ + rStartH + rSizeH/2
			}
			bm.TP(midX, midY, midZ)
			time.Sleep(30 * time.Millisecond)

			regVisited := make([][]bool, rSizeH)
			for i := 0; i < rSizeH; i++ {
				regVisited[i] = make([]bool, rSizeW)
			}

			for ly := 0; ly < rSizeH; ly++ {
				for lx := 0; lx < rSizeW; lx++ {
					if regVisited[ly][lx] {
						continue
					}
					imgX := rStartW + lx
					imgY := rStartH + ly
					var key [3]int
					if orientation == "vertical" {
						key = [3]int{baseX, baseY + imgY, baseZ + imgX}
					} else {
						key = [3]int{baseX + imgX, baseY, baseZ + imgY}
					}
					mb, ok := blocks[key]
					if !ok {
						regVisited[ly][lx] = true
						continue
					}
					block := mb.info
					curHeightOff := mb.heightOff
					if orientation == "vertical" || (orientation == "horizontal" && !useRelief) {
						curHeightOff = 0
					}

					rw := 0
					for dx := 0; lx+dx < rSizeW; dx++ {
						var nk [3]int
						if orientation == "vertical" {
							nk = [3]int{baseX, baseY + imgY, baseZ + rStartW + lx + dx}
						} else {
							nk = [3]int{baseX + rStartW + lx + dx, baseY, baseZ + imgY}
						}
						nb, nok := blocks[nk]
						if !nok || nb.info.Name != block.Name || nb.info.States != block.States || nb.heightOff != curHeightOff || regVisited[ly][lx+dx] {
							break
						}
						rw++
					}
					if rw == 0 {
						rw = 1
					}

					bestW, bestH := rw, 1
					for dy := 1; ly+dy < rSizeH; dy++ {
						cw := 0
						for dx := 0; dx < bestW; dx++ {
							var nk [3]int
							if orientation == "vertical" {
								nk = [3]int{baseX, baseY + rStartH + ly + dy, baseZ + rStartW + lx + dx}
							} else {
								nk = [3]int{baseX + rStartW + lx + dx, baseY, baseZ + rStartH + ly + dy}
							}
							nb, nok := blocks[nk]
							if !nok || nb.info.Name != block.Name || nb.info.States != block.States || nb.heightOff != curHeightOff || regVisited[ly+dy][lx+dx] {
								break
							}
							cw++
						}
						if cw == 0 {
							break
						}
						if cw < bestW {
							bestW = cw
						}
						bestH = dy + 1
					}

					for dy := 0; dy < bestH; dy++ {
						for dx := 0; dx < bestW; dx++ {
							regVisited[ly+dy][lx+dx] = true
						}
					}

					total := bestW * bestH
					if total >= 3 {
						var x1, y1, z1, x2, y2, z2 int
						if orientation == "vertical" {
							x1, y1, z1 = baseX+curHeightOff, baseY+rStartH+ly, baseZ+rStartW+lx
							x2, y2, z2 = baseX+curHeightOff, baseY+rStartH+ly+bestH-1, baseZ+rStartW+lx+bestW-1
						} else {
							x1, y1, z1 = baseX+rStartW+lx, baseY+curHeightOff, baseZ+rStartH+ly
							x2, y2, z2 = baseX+rStartW+lx+bestW-1, baseY+curHeightOff, baseZ+rStartH+ly+bestH-1
						}
						bm.SendBlockCmd(fmt.Sprintf("fill %d %d %d %d %d %d %s %s",
							x1, y1, z1, x2, y2, z2, normalizeBlockName(block.Name), block.States))
					} else {
						for dy := 0; dy < bestH; dy++ {
							for dx := 0; dx < bestW; dx++ {
								var baseKey [3]int
								var setX, setY, setZ int
								if orientation == "vertical" {
									baseKey = [3]int{baseX, baseY + rStartH + ly + dy, baseZ + rStartW + lx + dx}
									setX, setY, setZ = baseX+curHeightOff, baseY+rStartH+ly+dy, baseZ+rStartW+lx+dx
								} else {
									baseKey = [3]int{baseX + rStartW + lx + dx, baseY, baseZ + rStartH + ly + dy}
									setX, setY, setZ = baseX+rStartW+lx+dx, baseY+curHeightOff, baseZ+rStartH+ly+dy
								}
								b := blocks[baseKey]
								if b == nil {
									continue
								}
								bm.SendBlockCmd(fmt.Sprintf("setblock %d %d %d %s %s", setX, setY, setZ, normalizeBlockName(b.info.Name), b.info.States))
							}
						}
					}
					count += total
					time.Sleep(sleeptime * time.Duration(total))
				}
			}

			pct := float64(count) / float64(len(blocks))
			const mapBarW = 14
			bar := buildParBar(int(pct*100)*mapBarW/100, mapBarW, "d", "7")
			sendActionbar(fmt.Sprintf("§r§d%s §fv%s §7|  §dMapArt\n§f §r{%s}\n§r%s\n§b%.0f%%  §7%d/%d", brandName,
				shortVersion(), filepath.Base(path),
				bar, pct*100, count, len(blocks)))
			hub.Emit("task_progress", mustJSON(map[string]any{"progress": pct}))
			hub.Emit("progress", mustJSON(map[string]any{"msg": fmt.Sprintf("像素画: %d/%d (%.0f%%)", count, len(blocks), pct*100)}))
		}
	}

	// 记录心跳增量
	recordHeartbeatIncrement(IncrementData{
		MapArtCompleted: 1,
	})
	hub.Emit("task_done", mustJSON(map[string]any{"msg": fmt.Sprintf("像素画完成: %d 方块", count)}))
}

func runSkinTask(tc *state.TaskController, p map[string]any) {
	playerName := getString(p, "player", "")
	path := getString(p, "path", "")

	bm := getBotManager()
	if bm == nil || !bm.IsConnected() {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "未连接到服务器"}))
		return
	}

	var img image.Image
	var displayName string

	if playerName != "" {
		// 在线玩家皮肤：从缓存加载
		skinData, w, h, ok := playerInfoMgr.GetPlayerSkin(playerName)
		if !ok {
			hub.Emit("task_error", mustJSON(map[string]any{"error": "玩家不在线或没有皮肤缓存: " + playerName}))
			return
		}
		img = &image.NRGBA{
			Pix:    skinData,
			Stride: 4 * int(w),
			Rect:   image.Rect(0, 0, int(w), int(h)),
		}
		displayName = playerName + " (在线)"
		hub.Emit("progress", mustJSON(map[string]any{"msg": "加载在线玩家皮肤: " + playerName}))
	} else if path == "" {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "缺少皮肤图片路径或玩家名"}))
		return
	} else {
		// 本地文件皮肤
		displayName = filepath.Base(path)
		hub.Emit("progress", mustJSON(map[string]any{"msg": "加载皮肤图片: " + displayName}))
		f, err := os.Open(path)
		if err != nil {
			hub.Emit("task_error", mustJSON(map[string]any{"error": "无法打开图片: " + err.Error()}))
			return
		}
		defer f.Close()

		img, _, err = image.Decode(f)
		if err != nil {
			hub.Emit("task_error", mustJSON(map[string]any{"error": "图片解码失败: " + err.Error()}))
			return
		}
	}

	opts := media.SkinBuildOptions{
		Scale:          getInt(p, "scale", 2),
		ArmType:        media.SkinArmType(getString(p, "arm_type", "classic")),
		BlockSet:       media.SkinBlockSet(getString(p, "block_set", "mixed")),
		AlphaCutoff:    uint8(getInt(p, "alpha_cutoff", 16)),
		OuterThickness: getInt(p, "outer_thickness", 1),
		Solid:          getBool(p, "solid", false),
		FillBlock:      getString(p, "fill_block", "minecraft:stone"),
		Rotation:       getInt(p, "rotation", 0),
	}

	blocks, info, err := media.BuildSkinStatue(img, opts)
	if err != nil {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "雕像构建失败: " + err.Error()}))
		return
	}

	baseX := int32(getInt(p, "x", 0))
	baseY := int32(getInt(p, "y", -64))
	baseZ := int32(getInt(p, "z", 0))
	speed := max(1, min(getInt(p, "speed", 9500), 20000))
	sleeptime := time.Second / time.Duration(speed)

	hub.Emit("task_start", mustJSON(map[string]any{"type": "皮肤雕像: " + displayName}))
	hub.Emit("task_progress", mustJSON(map[string]any{"progress": 0}))
	if bm.IsOP() {
		bm.SendTellraw(fmt.Sprintf("§a§l▍皮肤雕像:§r§f%s §7尺寸:%dx%dx%d §7方块:%d §7坐标:(%d,%d,%d)",
			displayName, info.Width, info.Height, info.Length, info.BlockCount, baseX, baseY, baseZ))
	}

	hub.Emit("progress", mustJSON(map[string]any{
		"msg": fmt.Sprintf("雕像: %dx%dx%d, %d 方块", info.Width, info.Height, info.Length, info.BlockCount),
	}))

	taskID := uuid.New().String()
	serverCode := bm.ServerCode()
	cp := &state.Checkpoint{
		TaskID:     taskID,
		Type:       "skin",
		Status:     "running",
		Progress:   0,
		CreatedAt:  time.Now().Unix(),
		Params:     rawParams(p),
		Name:       displayName,
		ServerCode: serverCode,
	}
	state.SaveCheckpointNow(cp)

	// 按 Y 从下到上排序，确保雕像从地面开始建造
	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].Y != blocks[j].Y {
			return blocks[i].Y < blocks[j].Y
		}
		if blocks[i].X != blocks[j].X {
			return blocks[i].X < blocks[j].X
		}
		return blocks[i].Z < blocks[j].Z
	})

	// 按 Y 从下到上放置方块
	total := len(blocks)
	lastProgressTime := time.Now()
	importStartTime := time.Now()

	// 皮肤雕像 actionbar 定时刷新
	var (
		skinMu     sync.Mutex
		skinPct    int
		skinBlocks int
	)
	skinStopTicker := make(chan struct{})
	skinTicker := time.NewTicker(500 * time.Millisecond)
	defer skinTicker.Stop()
	go func() {
		var speedLastBlocks int
		var speedLastTime = time.Now()
		var lastRealSpeed int
		for {
			select {
			case <-skinTicker.C:
				skinMu.Lock()
				pct := skinPct
				blocks := skinBlocks
				skinMu.Unlock()

				elapsed := time.Since(importStartTime).Seconds()
				// 每秒更新一次速度，非更新时刻复用上次速度
				now := time.Now()
				realSpeed := lastRealSpeed
				if now.Sub(speedLastTime) >= time.Second {
					dt := now.Sub(speedLastTime).Seconds()
					if dt >= 0.5 {
						realSpeed = int(float64(blocks-speedLastBlocks) / dt)
						speedLastBlocks = blocks
						speedLastTime = now
						lastRealSpeed = realSpeed
					}
				}
				// 第一秒内用平均速度估算
				if lastRealSpeed == 0 && elapsed > 0.1 {
					realSpeed = int(float64(blocks) / elapsed)
				}

				// 显示用百分比，最大值 99%
				displayPct := pct
				if displayPct > 99 {
					displayPct = 99
				}
				// 旋转动画帧
				frames := []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
				frame := frames[time.Now().UnixMilli()/500%int64(len(frames))]
				bar := buildParBar(displayPct*14/100, 14, "d", "7")
				name := shortName(path)
				if len([]rune(name)) > 6 {
					name = string([]rune(name)[:6])
				}
				blockStr := formatCountV5(blocks)
				speedStr := formatSpeedV5(float64(realSpeed))
				eta := time.Since(importStartTime).Round(time.Second)
				msg := fmt.Sprintf("§r%s  §dSkin  §7┃  §d%s §fv%s  §7┃  §f%s\n§r%s\n§d%d%% §7| §f(%s) §7| %s/s §7| %s",
					frame, brandName, shortVersion(), name, bar, displayPct, blockStr, speedStr, eta)
				sendActionbar(msg)
			case <-skinStopTicker:
				return
			}
		}
	}()
	defer close(skinStopTicker)

	for i, b := range blocks {
		select {
		case <-tc.StopCh():
			cp.Status = "cancelled"
			cp.Message = "已取消"
			state.SaveCheckpoint(cp)
			hub.Emit("task_error", mustJSON(map[string]any{"error": "已取消"}))
			return
		default:
		}

		x := baseX + b.X
		y := baseY + b.Y
		z := baseZ + b.Z
		bm.SendBlockCmd(fmt.Sprintf("setblock %d %d %d %s %s", x, y, z, normalizeBlockName(b.BlockName), b.BlockStates))
		time.Sleep(sleeptime)

		if i%100 == 0 || time.Since(lastProgressTime) >= 500*time.Millisecond {
			pct := float64(i+1) / float64(total)
			lastProgressTime = time.Now()

			// 更新 actionbar 共享状态
			skinMu.Lock()
			skinPct = int(pct * 100)
			skinBlocks = i + 1
			skinMu.Unlock()

			hub.Emit("task_progress", mustJSON(map[string]any{
				"progress":     pct,
				"sub_progress": 1.0,
				"blocks":       i + 1,
			}))
			hub.Emit("progress", mustJSON(map[string]any{
				"msg": fmt.Sprintf("雕像: %d/%d", i+1, total),
			}))

			cp.Progress = pct
			cp.Message = fmt.Sprintf("%d/%d", i+1, total)
			state.SaveCheckpoint(cp)
		}
	}
	close(skinStopTicker)

	// 最终进度条
	bar := buildParBar(100, 14, "d", "7")
	name := shortName(path)
	if len([]rune(name)) > 6 {
		name = string([]rune(name)[:6])
	}
	blockStr := formatCountV5(len(blocks))
	eta := time.Since(importStartTime).Round(time.Second)
	msg := fmt.Sprintf("§r⣾ §d%s §fv%s §7|  §dSkin\n§f §a§l{%s}\n§r%s\n§d100%% §7| §f(%s) §7| %s",
		brandName, shortVersion(), name, bar, blockStr, eta)
	sendActionbar(msg)

	bm.cleanupNBTWorkspace()

	cp.Status = "done"
	cp.Progress = 1.0
	state.SaveCheckpoint(cp)

	if bm.IsOP() {
		bm.SendTellraw(fmt.Sprintf("§a§l▍雕像完成:§r§f%s §7共%d方块", displayName, len(blocks)))
	}
	// 记录心跳增量
	recordHeartbeatIncrement(IncrementData{
		SkinCompleted: 1,
	})
	hub.Emit("task_done", mustJSON(map[string]any{"msg": fmt.Sprintf("雕像导入完成: %d 方块", len(blocks)), "task_id": taskID}))
}

func runMediaTask(tc *state.TaskController, p map[string]any) {
	mediaType := getString(p, "media_type", "video")
	target := getString(p, "display", "actionbar")
	fps := getInt(p, "fps", 10)
	path := getString(p, "path", "")
	bm := getBotManager()

	switch mediaType {
	case "video":
		hub.Emit("progress", mustJSON(map[string]any{"msg": "视频播放: " + path}))
		if bm != nil && bm.IsOP() {
			bm.SendTellraw(fmt.Sprintf("§a§l▍音视频:§r§f%s §7类型:视频 §7显示:%s", filepath.Base(path), target))
		}
		hub.Emit("progress", mustJSON(map[string]any{
			"msg": "视频需要先用ffmpeg提取帧为图片文件，放入数据目录的frames/子目录",
		}))

		// Try to load frames from data directory
		framesDir := path
		if framesDir == "" {
			framesDir = "./frames/"
		}
		entries, err := os.ReadDir(framesDir)
		if err != nil {
			hub.Emit("task_error", mustJSON(map[string]any{"error": "无法读取帧目录: " + err.Error()}))
			return
		}

		var frames [][]string
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			f, err := os.Open(framesDir + "/" + entry.Name())
			if err != nil {
				continue
			}
			img, _, err := image.Decode(f)
			f.Close()
			if err != nil {
				continue
			}
			lines, _ := media.ImageToASCII(img, 80)
			frames = append(frames, lines)
		}

		if len(frames) == 0 {
			hub.Emit("task_error", mustJSON(map[string]any{"error": "未找到有效帧文件"}))
			return
		}

		hub.Emit("progress", mustJSON(map[string]any{"msg": fmt.Sprintf("找到 %d 帧，开始播放...", len(frames))}))

		player := media.NewPlayer(func(cmd string) error {
			return bm.SendWOCmd(cmd)
		})
		player.Config.Target = "@a"
		player.Config.Mode = target
		player.Config.FPS = fps
		player.PlayASCIIFrames(frames, tc.StopCh())

		hub.Emit("task_done", mustJSON(map[string]any{"msg": "播放完成"}))

	case "midi":
		hub.Emit("progress", mustJSON(map[string]any{"msg": "MIDI: " + path}))
		if bm != nil && bm.IsOP() {
			bm.SendTellraw(fmt.Sprintf("§a§l▍音视频:§r§f%s §7类型:MIDI", filepath.Base(path)))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			hub.Emit("task_error", mustJSON(map[string]any{"error": "无法读取: " + err.Error()}))
			return
		}
		seq, err := media.ParseMidiSeq(data)
		if err != nil {
			hub.Emit("progress", mustJSON(map[string]any{"msg": "非midseq格式，逐行播放..."}))
			lines := splitLines(string(data))
			st := time.Second / time.Duration(fps)
			for i, line := range lines {
				select {
				case <-tc.StopCh():
					return
				default:
				}
				if len(line) > 0 && line[0] != '#' {
					bm.SendWOCmd(line)
					time.Sleep(st)
				}
				if i%10 == 0 {
					hub.Emit("task_progress", mustJSON(map[string]any{"progress": float64(i) / float64(len(lines))}))
				}
			}
			hub.Emit("task_done", mustJSON(map[string]any{"msg": "播放完成"}))
			return
		}
		hub.Emit("progress", mustJSON(map[string]any{"msg": fmt.Sprintf("乐器:%d 时长:%.0fs 音符:%d", len(seq.Instruments), seq.Duration, len(seq.Notes))}))
		for i, n := range seq.Notes {
			select {
			case <-tc.StopCh():
				return
			default:
			}
			time.Sleep(time.Duration(n.Delay * float64(time.Second)))
			bm.SendWOCmd(fmt.Sprintf("execute as @a at @s run playsound %s @s ~~~ %.2f %.3f", n.Instrument, n.Volume, n.Pitch))
			if i%20 == 0 {
				hub.Emit("task_progress", mustJSON(map[string]any{"progress": float64(i) / float64(len(seq.Notes))}))
			}
		}
		hub.Emit("task_done", mustJSON(map[string]any{"msg": fmt.Sprintf("MIDI完成(%.0fs)", seq.Duration)}))
	}
}

func splitLines(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

var bedrockEnchNames = map[int]string{
	0: "protection", 1: "fire_protection", 2: "feather_falling",
	3: "blast_protection", 4: "projectile_protection", 5: "thorns",
	6: "respiration", 7: "depth_strider", 8: "aqua_affinity",
	9: "sharpness", 10: "smite", 11: "bane_of_arthropods",
	12: "knockback", 13: "fire_aspect", 14: "looting",
	15: "efficiency", 16: "silk_touch", 17: "unbreaking",
	18: "fortune", 19: "power", 20: "punch",
	21: "flame", 22: "infinity", 23: "luck_of_the_sea",
	24: "lure", 25: "frost_walker", 26: "mending",
	27: "curse_of_binding", 28: "curse_of_vanishing", 29: "impaling",
	30: "riptide", 31: "loyalty", 32: "channeling",
	33: "multishot", 34: "piercing", 35: "quick_charge",
	36: "soul_speed", 37: "swift_sneak",
}

// buildSimpleItemComponents 从物品 tag NBT 构建简单 components JSON。
// 仅包含服务端支持的：KeepOnDeath、ItemLock、CanDestroy、CanPlaceOn。
// 不包含 display_name 和 enchantments——它们会导致整个命令失败。
func buildSimpleItemComponents(tag map[string]any) string {
	parts := make([]string, 0)

	// KeepOnDeath — 死亡后不掉落
	if _, ok := tag["KeepOnDeath"]; ok {
		parts = append(parts, `"minecraft:keep_on_death":{}`)
	}

	// ItemLock — 锁定物品（lock_in_container / lock_in_slot）
	if lockVal, ok := tag["item_lock"]; ok {
		switch v := lockVal.(type) {
		case map[string]any:
			if mode, ok := v["mode"].(string); ok {
				parts = append(parts, fmt.Sprintf(`"minecraft:item_lock":{"mode":%q}`, mode))
			}
		case string:
			parts = append(parts, fmt.Sprintf(`"minecraft:item_lock":{"mode":%q}`, v))
		}
	}

	// CanDestroy — 冒险模式可破坏方块
	if cd, ok := tag["can_destroy"].(map[string]any); ok {
		if blocks, ok := cd["blocks"].([]any); ok && len(blocks) > 0 {
			blockStrs := make([]string, len(blocks))
			for i, b := range blocks {
				blockStrs[i] = fmt.Sprintf("%q", b)
			}
			parts = append(parts, fmt.Sprintf(`"minecraft:can_destroy":{"blocks":[%s]}`, strings.Join(blockStrs, ",")))
		}
	}

	// CanPlaceOn — 冒险模式可放置方块
	if cpo, ok := tag["can_place_on"].(map[string]any); ok {
		if blocks, ok := cpo["blocks"].([]any); ok && len(blocks) > 0 {
			blockStrs := make([]string, len(blocks))
			for i, b := range blocks {
				blockStrs[i] = fmt.Sprintf("%q", b)
			}
			parts = append(parts, fmt.Sprintf(`"minecraft:can_place_on":{"blocks":[%s]}`, strings.Join(blockStrs, ",")))
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// itemDamage 从物品 NBT 中提取 Damage/meta 值。
func itemDamage(item map[string]any) int32 {
	switch d := item["Damage"].(type) {
	case int32:
		return d
	case int16:
		return int32(d)
	case int:
		return int32(d)
	case float64:
		return int32(d)
	case byte:
		return int32(d)
	}
	return 0
}

func getBool(p map[string]any, key string, def bool) bool {
	if v, ok := p[key].(bool); ok {
		return v
	}
	return def
}

// parseDimension 解析维度参数，兼容字符串（"overworld"/"nether"/"end"）和数字（0/1/2）。
// 导出时前端传的是 select 控件的字符串值，需要映射为 MCBE 维度 ID。
func parseDimension(p map[string]any, key string, def int) int {
	// 尝试字符串
	if s, ok := p[key].(string); ok && s != "" {
		return dimNameToID(s)
	}
	// 回退到数字
	return getInt(p, key, def)
}

func getString(p map[string]any, key, def string) string {
	if v, ok := p[key].(string); ok {
		return v
	}
	return def
}

func getInt(p map[string]any, key string, def int) int {
	switch v := p[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		var i int
		fmt.Sscanf(v, "%d", &i)
		return i
	}
	return def
}

// extractBlockEntities reads block_position_data from a RequestStructure response
// and attaches it as a flat nbt_blocks list to the chunk map.
func extractBlockEntities(chunk map[string]any) {
	structure, _ := chunk["structure"].(map[string]any)
	if structure == nil {
		return
	}
	paletteData, _ := structure["palette"].(map[string]any)
	if paletteData == nil {
		return
	}

	var blockPosData map[string]any
	for _, pv := range paletteData {
		pm, ok := pv.(map[string]any)
		if !ok {
			continue
		}
		if bpd, ok := pm["block_position_data"].(map[string]any); ok && len(bpd) > len(blockPosData) {
			blockPosData = bpd
		}
	}
	if blockPosData == nil {
		return
	}

	// Build palette name lookup
	var paletteNames []string
	for _, pv := range paletteData {
		pm, ok := pv.(map[string]any)
		if !ok {
			continue
		}
		if bp, ok := pm["block_palette"].([]any); ok {
			for _, entry := range bp {
				em, _ := entry.(map[string]any)
				name, _ := em["name"].(string)
				paletteNames = append(paletteNames, name)
			}
			break
		}
	}

	var nbtBlocks []map[string]any
	for keyStr, value := range blockPosData {
		valMap, ok := value.(map[string]any)
		if !ok {
			continue
		}
		entityData, ok := valMap["block_entity_data"].(map[string]any)
		if !ok {
			continue
		}

		var keyInt int
		fmt.Sscanf(keyStr, "%d", &keyInt)
		blockName := "minecraft:air"
		if keyInt >= 0 && keyInt < len(paletteNames) {
			blockName = paletteNames[keyInt]
		}
		nbtBlocks = append(nbtBlocks, map[string]any{
			"palette_index": keyInt,
			"name":          blockName,
			"nbt":           entityData,
		})
	}
	if len(nbtBlocks) > 0 {
		chunk["nbt_blocks"] = nbtBlocks
	}
}

// mergeChunkStructures combines block_indices
// mergeChunkStructures combines block_indices from multiple 16xN chunks into one.
// Each chunk's block palette must be compatible — we remap indices and merge palettes.
// blockNameForIndex resolves a palette index to a block name.
func blockNameForIndex(idx int, paletteData map[string]any) string {
	for _, pv := range paletteData {
		pm, ok := pv.(map[string]any)
		if !ok {
			continue
		}
		bp, ok := pm["block_palette"].([]any)
		if !ok {
			continue
		}
		if idx >= 0 && idx < len(bp) {
			if bm, ok := bp[idx].(map[string]any); ok {
				if n, ok := bm["name"].(string); ok {
					return n
				}
			}
		}
		break
	}
	return "minecraft:air"
}

func mergeChunkStructures(chunks []map[string]any, totalW, totalH, totalL int32) map[string]any {
	if len(chunks) == 0 {
		return nil
	}
	if len(chunks) == 1 {
		if s, ok := chunks[0]["structure"].(map[string]any); ok {
			return s
		}
		return nil
	}

	// For multi-chunk: build a merged palette and concat block_indices properly.
	// Each chunk covers a 16×totalH×16 region (partial at edges).
	chunkSize := int32(16)
	chunksZ := (totalL + chunkSize - 1) / chunkSize

	// Collect all chunk structures
	type chunkInfo struct {
		structure map[string]any
		cx, cz    int32
		szX, szZ  int32
	}
	var infos []chunkInfo
	for ci, chunk := range chunks {
		s, ok := chunk["structure"].(map[string]any)
		if !ok {
			continue
		}
		cx := int32(ci) / chunksZ
		cz := int32(ci) % chunksZ
		szX := chunkSize
		szZ := chunkSize
		if (cx+1)*chunkSize > totalW {
			szX = totalW - cx*chunkSize
		}
		if (cz+1)*chunkSize > totalL {
			szZ = totalL - cz*chunkSize
		}
		infos = append(infos, chunkInfo{s, cx, cz, szX, szZ})
	}
	if len(infos) == 0 {
		return nil
	}
	if len(infos) == 1 {
		return infos[0].structure
	}

	// Build merged palette from all chunks
	paletteMap := make(map[string]int) // blockName → mergedIndex
	var mergedPalette []map[string]any
	mergedPalette = append(mergedPalette, map[string]any{"name": "minecraft:air"}) // index 0 = air
	paletteMap["minecraft:air"] = 0

	// Remap lists: oldChunkName → newIndex
	type remapKey struct{ ci, oldIdx int }
	remaps := make(map[remapKey]int)

	for ci, info := range infos {
		palRaw, _ := info.structure["palette"].(map[string]any)
		for _, pv := range palRaw {
			pm, ok := pv.(map[string]any)
			if !ok {
				continue
			}
			bp, ok := pm["block_palette"].([]any)
			if !ok {
				continue
			}
			for oldIdx, entry := range bp {
				em, _ := entry.(map[string]any)
				name, _ := em["name"].(string)
				if name == "" {
					name = "minecraft:air"
				}
				key := name
				if states, ok := em["states"].(map[string]any); ok && len(states) > 0 {
					key = name + fmt.Sprintf("%v", states)
				}
				if _, exists := paletteMap[key]; !exists {
					paletteMap[key] = len(mergedPalette)
					mergedPalette = append(mergedPalette, em)
				}
				remaps[remapKey{ci, oldIdx}] = paletteMap[key]
			}
			break // only process first palette name
		}
	}

	// Build merged block_indices layers
	// For each layer, create a totalW×totalH×totalL array
	numLayers := 0
	if len(infos) > 0 {
		if bi, ok := infos[0].structure["block_indices"].([]any); ok {
			numLayers = len(bi)
		}
	}
	mergedLayers := make([][]int32, numLayers)
	for layer := 0; layer < numLayers; layer++ {
		total := int(totalW * totalH * totalL)
		merged := make([]int32, total)
		for i := range merged {
			merged[i] = -1
		}

		for ci, info := range infos {
			bi, _ := info.structure["block_indices"].([]any)
			if layer >= len(bi) {
				continue
			}
			indices := toIntSlice(bi[layer])
			offsetX := int(info.cx * chunkSize)
			offsetZ := int(info.cz * chunkSize)
			for y := int32(0); y < totalH; y++ {
				for z := int32(0); z < info.szZ; z++ {
					for x := int32(0); x < info.szX; x++ {
						srcIdx := int(x*totalH*info.szZ + y*info.szZ + z)
						dstIdx := int((int32(offsetX)+x)*totalH*totalL + y*totalL + (int32(offsetZ) + z))
						if srcIdx < len(indices) && dstIdx < total {
							oldIdx := indices[srcIdx]
							if newIdx, ok := remaps[remapKey{ci, oldIdx}]; ok {
								merged[dstIdx] = int32(newIdx)
							} else if oldIdx >= 0 {
								merged[dstIdx] = int32(oldIdx) // keep as-is (fallback)
							}
						}
					}
				}
			}
		}
		mergedLayers[layer] = merged
	}

	// Build merged block_position_data (remap keys)
	mergedPosData := make(map[string]any)
	for _, info := range infos {
		palRaw, _ := info.structure["palette"].(map[string]any)
		for _, pv := range palRaw {
			pm, ok := pv.(map[string]any)
			if !ok {
				continue
			}
			bpd, ok := pm["block_position_data"].(map[string]any)
			if !ok {
				continue
			}
			for oldKeyStr, value := range bpd {
				var oldIdx int
				fmt.Sscanf(oldKeyStr, "%d", &oldIdx)
				// block_position_data key 是线性方块索引，不是调色板索引
				// 区块索引: x*totalH*szZ + y*szZ + z
				szZ := int(info.szZ)
				h := int(totalH)
				l := int(totalL)
				chunkStride := h * szZ
				offsetX := int(info.cx * chunkSize)
				offsetZ := int(info.cz * chunkSize)
				x := oldIdx / chunkStride
				rem := oldIdx % chunkStride
				y := rem / szZ
				z := rem % szZ
				// 合并后索引: (offsetX+x)*totalH*totalL + y*totalL + (offsetZ+z)
				newIdx := (offsetX+x)*h*l + y*l + (offsetZ + z)
				mergedPosData[fmt.Sprintf("%d", newIdx)] = value
			}
		}
	}

	// Convert mergedLayers to []any for NBT encoding
	mergedIndices := make([]any, numLayers)
	for layer := 0; layer < numLayers; layer++ {
		arr := make([]any, len(mergedLayers[layer]))
		for i, v := range mergedLayers[layer] {
			arr[i] = v
		}
		mergedIndices[layer] = arr
	}

	mergedPaletteAny := make([]any, len(mergedPalette))
	for i, v := range mergedPalette {
		mergedPaletteAny[i] = v
	}

	mergedPaletteData := map[string]any{
		"default": map[string]any{
			"block_palette": mergedPaletteAny,
		},
	}
	if len(mergedPosData) > 0 {
		mergedPaletteData["default"].(map[string]any)["block_position_data"] = mergedPosData
	}

	return map[string]any{
		"block_indices": mergedIndices,
		"palette":       mergedPaletteData,
	}
}

// nbtInt reads an integer value from an NBT map, supporting all numeric NBT types.
func nbtInt(m map[string]any, key string) int {
	v := m[key]
	switch val := v.(type) {
	case int32:
		return int(val)
	case int16:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case byte:
		return int(val)
	case float64:
		return int(val)
	}
	return 0
}

// nbtByte reads a boolean-relevant byte from an NBT map (0=false, everything else=true).
func nbtByte(m map[string]any, key string) byte {
	v := m[key]
	switch val := v.(type) {
	case byte:
		return val
	case int32:
		return byte(val)
	case int16:
		return byte(val)
	case float64:
		return byte(val)
	case int:
		return byte(val)
	}
	return 0
}

// nbtStr returns the first non-empty string among the candidate keys.
// Bedrock block-entity NBT uses lowercase keys (e.g. structureName); pass those first.
func nbtStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// nbtF32 reads a float from an NBT map, tolerating float32/float64/int encodings.
func nbtF32(m map[string]any, key string) float32 {
	switch val := m[key].(type) {
	case float32:
		return val
	case float64:
		return float32(val)
	case int32:
		return float32(val)
	case int64:
		return float32(val)
	case int:
		return float32(val)
	}
	return 0
}

// buildStructureBlockUpdate builds a StructureBlockUpdate packet (Position set by caller) from
// raw block-entity NBT. Bedrock (.mcworld/.mcstructure) stores keys lowercase (structureName,
// dataField, xStructureSize, xStructureOffset, ...); capitalized / Java names are read as a
// fallback. It carries the full structure data — name, size, offset, rotation, mirror,
// integrity, seed — not just the name. Values are copied straight from the NBT, so each mode
// (save/load/corner/data) naturally gets the fields it stored; absent fields default to zero.
// The data→type mapping is preserved from the original code.
func buildStructureBlockUpdate(nbt map[string]any) packet.StructureBlockUpdate {
	typ := int32(1)
	if _, ok := nbt["StructureBlockType"]; ok {
		typ = int32(nbtInt(nbt, "StructureBlockType"))
	}
	if _, ok := nbt["data"]; ok {
		switch nbtInt(nbt, "data") {
		case 0:
			typ = 1
		case 1:
			typ = 2
		case 2:
			typ = 3
		case 3:
			typ = 0
		case 4:
			typ = 4
		}
	}

	// Bedrock NBT stores integrity as a percentage (0..100); the packet expects 0..1.
	integrity := nbtF32(nbt, "integrity")
	if integrity > 1 {
		integrity /= 100
	}

	return packet.StructureBlockUpdate{
		StructureName:      nbtStr(nbt, "structureName", "StructureName", "name"),
		DataField:          nbtStr(nbt, "dataField", "DataField", "metadata"),
		IncludePlayers:     nbtByte(nbt, "includePlayers") != 0 || nbtByte(nbt, "IncludePlayers") != 0,
		ShowBoundingBox:    nbtByte(nbt, "showBoundingBox") != 0 || nbtByte(nbt, "ShowBoundingBox") != 0,
		StructureBlockType: typ,
		RedstoneSaveMode:   int32(nbtInt(nbt, "redstoneSaveMode")),
		Settings: protocol.StructureSettings{
			PaletteName:    "default",
			IgnoreEntities: nbtByte(nbt, "ignoreEntities") != 0,
			Size: protocol.BlockPos{
				int32(nbtInt(nbt, "xStructureSize")),
				int32(nbtInt(nbt, "yStructureSize")),
				int32(nbtInt(nbt, "zStructureSize")),
			},
			Offset: protocol.BlockPos{
				int32(nbtInt(nbt, "xStructureOffset")),
				int32(nbtInt(nbt, "yStructureOffset")),
				int32(nbtInt(nbt, "zStructureOffset")),
			},
			Rotation:          nbtByte(nbt, "rotation"),
			Mirror:            nbtByte(nbt, "mirror"),
			AnimationMode:     nbtByte(nbt, "animationMode"),
			AnimationDuration: nbtF32(nbt, "animationSeconds"),
			Integrity:         integrity,
			Seed:              uint32(nbtInt(nbt, "seed")),
		},
	}
}

// buildParBar 渲染平行四边形进度条。
// 使用 ࡆ (U+0866) 实心字符 + §o(斜体) 实现平行四边形效果。
// 在填充/未填充交界处，当两侧都有足够字符时去掉斜体做视觉过渡；
// 但只剩最后一个字符时（进度快满或仅剩一个未填充），保留斜体保证末端统一。
func buildParBar(filled, width int, color, grayColor string) string {
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	var sb strings.Builder
	sb.Grow(width * 8)
	for i := 0; i < width; i++ {
		if i < filled {
			if i == 0 && filled > 1 {
				sb.WriteString("\u00a7o\u00a7l\u00a7" + color)
			} else if i == filled-1 && filled > 1 && filled+1 < width {
				sb.WriteString("\u00a7r\u00a7l\u00a7" + color)
			} else if i == 0 {
				sb.WriteString("\u00a7o\u00a7l\u00a7" + color)
			}
		} else {
			if i == filled {
				if filled == 0 {
					sb.WriteString("\u00a7o\u00a7l\u00a7" + grayColor)
				} else if filled+1 >= width {
					sb.WriteString("\u00a7o\u00a7l\u00a7" + grayColor)
				} else {
					sb.WriteString("\u00a7r\u00a7" + grayColor)
				}
			} else if i == filled+1 && filled > 0 {
				sb.WriteString("\u00a7o\u00a7l\u00a7" + grayColor)
			}
		}
		sb.WriteString("\u0866")
	}
	return sb.String()
}

// buildClearActionbar 渲染清空区域进度条，使用统一的平行四边形样式。
func buildClearActionbar(cleared, total int) string {
	const barWidth = 32
	pct := 0
	if total > 0 {
		pct = cleared * 100 / total
	}
	bar := buildParBar(pct*barWidth/100, barWidth, "6", "8")
	countTxt := fmt.Sprintf("%d / %d", cleared, total)
	if total >= 1000 {
		countTxt = fmt.Sprintf("%.1fk/%.1fk", float64(cleared)/1000, float64(total)/1000)
	}
	return fmt.Sprintf("\u00a7c\u26cf \u6e05\u7a7a\u533a\u57df\n\u00a7r%s\n\u00a7c%d%%  \u00a7f%s", bar, pct, countTxt)
}

// buildV5Actionbar 渲染 v5 版式的 actionbar 进度条
// 保留 Prism 布局结构，采用 NexusEgo 样式：▓/░ 进度字符、旋转动画、ETA 预估
func buildV5Actionbar(sendFn func(string), name string, sx, sy, sz int,
	regionMode, totalSolid, targetSpeed, realSpeed, blocks, totalPct, subPct int,
	regionIdx, totalRegions int, startTime time.Time, brandName string) {

	// 旋转动画帧
	frames := []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
	frame := frames[time.Now().UnixMilli()/500%int64(len(frames))]

	// 速度格式化（参考 NexusEgo 精度）
	speedStr := formatSpeedV5(float64(realSpeed))

	barWidth := 14

	// 显示用百分比，最大值 99%
	displaySubPct := subPct
	if displaySubPct > 99 {
		displaySubPct = 99
	}
	displayTotalPct := totalPct
	if displayTotalPct > 99 {
		displayTotalPct = 99
	}

	sbw := (displaySubPct*barWidth + 99) / 100
	if sbw > barWidth {
		sbw = barWidth
	}
	subBar := buildParBar(sbw, barWidth, "e", "7")

	tbw := (displayTotalPct*barWidth + 99) / 100
	if tbw > barWidth {
		tbw = barWidth
	}
	totalBar := buildParBar(tbw, barWidth, "a", "7")

	// 方块数量格式化（k 后缀）
	blockStr := formatCountV5(blocks)
	totalStr := formatCountV5(totalSolid)

	// 文件名：取 @ 前或 [ 前的内容，再截取前 6 个字符
	if idx := strings.IndexAny(name, "@["); idx >= 0 {
		name = name[:idx]
	}
	if len([]rune(name)) > 6 {
		name = string([]rune(name)[:6])
	}

	// 耗时与 ETA
	elapsed := time.Since(startTime).Round(time.Second)
	etaStr := "--"
	if blocks > 0 && totalPct > 0 {
		remaining := time.Duration(float64(elapsed) * float64(100-totalPct) / float64(totalPct)).Round(time.Second)
		etaStr = formatDurationV5(remaining)
	}

	msg := fmt.Sprintf("§r%s  §cImporter  §7┃  §d%s §fv%s  §7┃  §f§l{%s}\n"+
		"§l§d> §7小区 %s  §e§l%d%%\n"+
		"§l§d> §7总计 %s  §a§l%d%%\n"+
		"§r§b%d%% §7| §f(%s/%s) §7| %s/s §7| %s",
		frame, brandName, shortVersion(), name,
		subBar, displaySubPct,
		totalBar, displayTotalPct,
		displayTotalPct, blockStr, totalStr, speedStr, etaStr)

	sendFn(msg)
}

func formatSpeedV5(speed float64) string {
	if speed <= 0 {
		return "0"
	}
	switch {
	case speed >= 100:
		return fmt.Sprintf("%.0f", speed)
	case speed >= 10:
		return fmt.Sprintf("%.1f", speed)
	default:
		return fmt.Sprintf("%.2f", speed)
	}
}

func formatCountV5(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func formatDurationV5(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return d.String()
}
