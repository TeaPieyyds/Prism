package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// ═══════════════════════════════════════════════════════════════
// 命令方块工程 — 导出引擎
// ═══════════════════════════════════════════════════════════════

// CBSimpleBlock 表示扫描到的单个命令方块（简化版，从 StructureData 中提取）
type CBSimpleBlock struct {
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Z        int    `json:"z"`
	Command  string `json:"command"`
	Type     string `json:"type"`     // impulse / repeating / chain
	Facing   string `json:"facing"`   // 朝向: z+ / z- / x+ / x- / y+ / y-
	Conditional bool `json:"conditional"`
	Auto     bool   `json:"auto"`     // 是否保持开启（红石控制）
	TickDelay int   `json:"tick_delay"`
	CustomName string `json:"custom_name,omitempty"`
	ExeFirstTick bool `json:"exe_first_tick"`
}

// CBlockChain 表示一条命令方块链
type CBlockChain struct {
	Blocks    []CBSimpleBlock `json:"blocks"`
	StartX    int             `json:"start_x"`
	StartY    int             `json:"start_y"`
	StartZ    int             `json:"start_z"`
	Direction string          `json:"direction"` // 链的总体走向
}

// CBExportResult 导出结果
type CBExportResult struct {
	Chains  []CBlockChain    `json:"chains"`
	Content string           `json:"content"` // 生成的 .mcfunction 内容
	Stats   *CBSimpleStats   `json:"stats"`
}

// CBSimpleStats 简易统计（导出时的预览用）
type CBSimpleStats struct {
	TotalBlocks  int            `json:"total_blocks"`
	ChainCount   int            `json:"chain_count"`
	Pulse        int            `json:"pulse"`
	Repeating    int            `json:"repeating"`
	Chain        int            `json:"chain"`
	Conditional  int            `json:"conditional"`
	Unconditional int           `json:"unconditional"`
	HasDelay     int            `json:"has_delay"`
	Breaks       int            `json:"breaks"`
	Scoreboards  []string       `json:"scoreboards"`
	Tags         []string       `json:"tags"`
}

// 朝向 → 位置偏移映射
var cbFacingOffset = map[string][3]int{
	"z+": {0, 0, 1},
	"z-": {0, 0, -1},
	"x+": {1, 0, 0},
	"x-": {-1, 0, 0},
	"y+": {0, 1, 0},
	"y-": {0, -1, 0},
}

// 方块状态 facing_direction 值 → 朝向名
var cbFacingNames = map[int]string{
	0: "y-", 1: "y+", 2: "z-", 3: "z+", 4: "x-", 5: "x+",
}

// 命令方块类型推導
func cbTypeFromName(name string) string {
	n := strings.ToLower(name)
	if strings.Contains(n, "repeating") {
		return "repeating"
	}
	if strings.Contains(n, "chain") {
		return "chain"
	}
	return "impulse"
}

// 解析朝向 facing_direction 值（0-5）
func cbFacingFromStates(states string) string {
	// 从 states 中提取 facing_direction 值
	// 格式: ["facing_direction"=3,...]
	idx := strings.Index(states, `facing_direction"=`)
	if idx < 0 {
		return "z+" // 默认
	}
	idx += len(`facing_direction"=`)
	end := idx
	for end < len(states) && states[end] >= '0' && states[end] <= '9' {
		end++
	}
	if end > idx {
		fd := 0
		fmt.Sscanf(states[idx:end], "%d", &fd)
		if name, ok := cbFacingNames[fd]; ok {
			return name
		}
	}
	return "z+"
}

// ========== 导出 API ==========

func handleCBExportScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		X1 int `json:"x1"`
		Y1 int `json:"y1"`
		Z1 int `json:"z1"`
		X2 int `json:"x2"`
		Y2 int `json:"y2"`
		Z2 int `json:"z2"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}

	// 软限制检查
	dx := absInt(req.X2-req.X1) + 1
	dy := absInt(req.Y2-req.Y1) + 1
	dz := absInt(req.Z2-req.Z1) + 1
	volume := dx * dy * dz
	if volume > 64*64*64 {
		writeJSON(w, map[string]any{"ok": false, "error": "扫描区域超过 64×64×64 软限制", "over_limit": true})
		return
	}

	// 检查机器人连接
	botMgrMu.Lock()
	bm := botMgr
	connected := bm != nil && bm.IsConnected()
	botMgrMu.Unlock()
	if !connected {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器"})
		return
	}

	// 排序坐标
	sx, sy, sz := minInt(req.X1, req.X2), minInt(req.Y1, req.Y2), minInt(req.Z1, req.Z2)
	ex, ey, ez := maxInt(req.X1, req.X2), maxInt(req.Y1, req.Y2), maxInt(req.Z1, req.Z2)
	sizeX := ex - sx + 1
	sizeY := ey - sy + 1
	sizeZ := ez - sz + 1

	// 先传送到区域中心，确保区块加载
	centerX := (sx + ex) / 2
	centerZ := (sz + ez) / 2
	bm.TP(centerX, sy, centerZ)
	time.Sleep(1 * time.Second) // 等待区块加载

	// 使用 RequestStructure 获取区域数据
	data, err := bm.RequestStructure(
		[3]int32{int32(sx), int32(sy), int32(sz)},
		[3]int32{int32(sizeX), int32(sizeY), int32(sizeZ)},
		30*time.Second,
	)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "扫描失败: " + err.Error()})
		return
	}

	// 解析结构数据，提取命令方块
	blocks := extractCommandBlocks(data, sx, sy, sz)
	if len(blocks) == 0 {
		writeJSON(w, map[string]any{"ok": false, "error": "未找到命令方块"})
		return
	}

	// 追踪链
	chains := traceCommandBlockChains(blocks)
	if len(chains) == 0 {
		// 如果链追踪失败（如只有孤立方块），把每个方块当一条链
		for _, b := range blocks {
			chains = append(chains, CBlockChain{
				Blocks: []CBSimpleBlock{b},
				StartX: b.X, StartY: b.Y, StartZ: b.Z,
				Direction: b.Facing,
			})
		}
	}

	// 生成 .mcfunction 内容
	content := generateCBProject(chains)

	// 统计信息
	stats := computeCBStats(chains, content)

	writeJSON(w, map[string]any{
		"ok":      true,
		"result":  CBExportResult{Chains: chains, Content: content, Stats: stats},
		"stats":   stats,
		"content": content,
	})
}

// extractCommandBlocks 从结构数据中提取所有命令方块
// 数据结构来自 protocol.StructureTemplateDataResponse.StructureTemplate
// 格式: {size:[...], structure:{palette:{default:{block_palette:[...], block_position_data:{...}}}, block_indices:[[...],[...]]}}
func extractCommandBlocks(data map[string]any, originX, originY, originZ int) []CBSimpleBlock {
	var blocks []CBSimpleBlock

	// 获取结构大小
	sizeRaw, _ := data["size"].([]any)
	if len(sizeRaw) < 3 {
		return blocks
	}
	sx := int(toFloat64(sizeRaw[0]))
	sy := int(toFloat64(sizeRaw[1]))
	sz := int(toFloat64(sizeRaw[2]))
	if sx <= 0 || sy <= 0 || sz <= 0 {
		return blocks
	}

	structure, _ := data["structure"].(map[string]any)
	if structure == nil {
		return blocks
	}

	// 解析 palette
	paletteData, _ := structure["palette"].(map[string]any)
	if paletteData == nil {
		return blocks
	}

	var blockPaletteRaw []any
	blockPosData := make(map[string]any)
	for _, pv := range paletteData {
		pm, ok := pv.(map[string]any)
		if !ok {
			continue
		}
		if bp, ok := pm["block_palette"].([]any); ok && len(bp) > len(blockPaletteRaw) {
			blockPaletteRaw = bp
		}
		if bpd, ok := pm["block_position_data"].(map[string]any); ok {
			for k, v := range bpd {
				blockPosData[k] = v
			}
		}
	}
	if len(blockPaletteRaw) == 0 {
		return blocks
	}

	// 解析 block_indices
	blockIndices, _ := structure["block_indices"].([]any)
	if len(blockIndices) < 1 {
		return blocks
	}
	layer0 := toIntArrayFromAny(blockIndices[0])
	if layer0 == nil {
		return blocks
	}

	// 提取 block entity data（用 block_entity_data 下的索引）
	blockEntityMap := make(map[int]map[string]any)
	for key, value := range blockPosData {
		valMap, ok := value.(map[string]any)
		if !ok {
			continue
		}
		entityData, ok := valMap["block_entity_data"].(map[string]any)
		if !ok {
			continue
		}
		var keyInt int
		fmt.Sscanf(key, "%d", &keyInt)
		blockEntityMap[keyInt] = entityData
	}

	// 遍历所有方块，找到命令方块
	stride := sx * sy
	for i, idx := range layer0 {
		if idx < 0 || int(idx) >= len(blockPaletteRaw) {
			continue
		}

		blockEntry, _ := blockPaletteRaw[idx].(map[string]any)
		if blockEntry == nil {
			continue
		}
		name, _ := blockEntry["name"].(string)
		if !strings.Contains(strings.ToLower(name), "command_block") {
			continue
		}

		// 计算坐标（block_indices 使用 x 最快顺序）
		z := i / stride
		rem := i % stride
		y := rem / sx
		x := rem % sx

		absX := originX + x
		absY := originY + y
		absZ := originZ + z

		// 获取方块状态
		states, _ := blockEntry["states"].(map[string]any)
		fd := 3 // 默认 z+
		if v, ok := states["facing_direction"]; ok {
			fd = int(toFloat64(v))
		}
		facing := "z+"
		if f, ok := cbFacingNames[fd]; ok {
			facing = f
		}

		cond := false
		if v, ok := states["conditional_bit"]; ok {
			cond = toFloat64(v) != 0
		}

		// 获取 NBT 数据（block_entity_data 使用 z 最快顺序索引）
		cmd := ""
		customName := ""
		auto := true
		tickDelay := 0
		exeFirst := false
		needsRedstone := false

		// block_entity_data 索引公式: index = x * sy * sz + y * sz + z
		nbtIdx := x*sy*sz + y*sz + z
		if nbt, ok := blockEntityMap[nbtIdx]; ok {
			if c, ok := nbt["Command"].(string); ok {
				cmd = c
			}
			if cn, ok := nbt["CustomName"].(string); ok {
				customName = cn
			}
			if v, ok := nbt["auto"]; ok {
				auto = toFloat64(v) != 0
			}
			if v, ok := nbt["TickDelay"]; ok {
				tickDelay = int(toFloat64(v))
			}
			if v, ok := nbt["ExecuteOnFirstTick"]; ok {
				exeFirst = toFloat64(v) != 0
			}
			if v, ok := nbt["NeedsRedstone"]; ok {
				needsRedstone = toFloat64(v) != 0
			}
			if v, ok := nbt["LPRedstoneMode"]; ok {
				needsRedstone = toFloat64(v) != 0
			}
		}

		needsRedstone = needsRedstone || !auto

		blockType := cbTypeFromName(name)

		blocks = append(blocks, CBSimpleBlock{
			X: absX, Y: absY, Z: absZ,
			Command:      cmd,
			Type:         blockType,
			Facing:       facing,
			Conditional:  cond,
			Auto:         auto,
			TickDelay:    tickDelay,
			CustomName:   customName,
			ExeFirstTick: exeFirst,
		})
	}

	return blocks
}

// toIntArrayFromAny 将任意类型转为 []int32
func toIntArrayFromAny(v any) []int32 {
	switch val := v.(type) {
	case []any:
		out := make([]int32, len(val))
		for i, e := range val {
			out[i] = int32(toFloat64(e))
		}
		return out
	case []int32:
		return val
	default:
		return nil
	}
}

// traceCommandBlockChains 从命令方块列表中追踪链
func traceCommandBlockChains(blocks []CBSimpleBlock) []CBlockChain {
	// 建立 3D 位置索引
	blockMap := make(map[string]*CBSimpleBlock)
	for i, b := range blocks {
		key := fmt.Sprintf("%d,%d,%d", b.X, b.Y, b.Z)
		blockMap[key] = &blocks[i]
	}

	visited := make(map[string]bool)
	var chains []CBlockChain

	// 从脉冲/循环方块开始追踪
	for i := range blocks {
		key := fmt.Sprintf("%d,%d,%d", blocks[i].X, blocks[i].Y, blocks[i].Z)
		if visited[key] {
			continue
		}
		if blocks[i].Type != "impulse" && blocks[i].Type != "repeating" {
			continue
		}

		// 开始追踪这条链
		var chainBlocks []CBSimpleBlock
		cur := &blocks[i]
		visited[key] = true

		for cur != nil {
			chainBlocks = append(chainBlocks, *cur)

			// 沿朝向方向找下一个方块
			off, ok := cbFacingOffset[cur.Facing]
			if !ok {
				break
			}
			nx := cur.X + off[0]
			ny := cur.Y + off[1]
			nz := cur.Z + off[2]

			nextKey := fmt.Sprintf("%d,%d,%d", nx, ny, nz)
			next, exists := blockMap[nextKey]
			if !exists || visited[nextKey] {
				break
			}
			// 下一个必须是连锁方块
			if next.Type != "chain" {
				break
			}
			visited[nextKey] = true
			cur = next
		}

		if len(chainBlocks) > 0 {
			first := chainBlocks[0]
			chains = append(chains, CBlockChain{
				Blocks:    chainBlocks,
				StartX:    first.X,
				StartY:    first.Y,
				StartZ:    first.Z,
				Direction: first.Facing,
			})
		}
	}

	return chains
}

// generateCBProject 从链生成 .mcfunction 内容
func generateCBProject(chains []CBlockChain) string {
	var sb strings.Builder

	// 收集积分板和标签
	sb.WriteString("//工程:命令方块工程\n")
	sb.WriteString("//版本:1.0\n")
	sb.WriteString("//作者:Prism 工具箱\n")
	sb.WriteString("//说明:由 Prism 工具箱自动导出\n")

	// 收集 scoreboards 和 tags
	allScoreboards := make(map[string]bool)
	allTags := make(map[string]bool)
	for _, chain := range chains {
		for _, block := range chain.Blocks {
			extractResources(block.Command, allScoreboards, allTags)
		}
	}
	if len(allScoreboards) > 0 {
		var sbList []string
		for s := range allScoreboards {
			sbList = append(sbList, s)
		}
		sb.WriteString("//积分板:" + strings.Join(sbList, ", ") + "\n")
	}
	if len(allTags) > 0 {
		var tList []string
		for t := range allTags {
			tList = append(tList, t)
		}
		sb.WriteString("//标签:" + strings.Join(tList, ", ") + "\n")
	}

	sb.WriteString("\n")

	for ci, chain := range chains {
		if ci > 0 {
			sb.WriteString("\n")
		}
		prevFacing := ""
		prevType := ""
		prevConditional := false
		prevAuto := false
		prevDelay := 0
		prevExeFirst := false
		hasPrev := false

		for i, block := range chain.Blocks {
			// 检测断链：如果当前位置和上一个不连续，插断链标记
			if hasPrev && i > 0 {
				prev := chain.Blocks[i-1]
				off, ok := cbFacingOffset[prev.Facing]
				if ok {
					expectedX := prev.X + off[0]
					expectedY := prev.Y + off[1]
					expectedZ := prev.Z + off[2]
					if block.X != expectedX || block.Y != expectedY || block.Z != expectedZ {
						sb.WriteString("# 断链\n")
						// 重置 prev 属性，让下一个方块写完整属性
						hasPrev = false
					}
				}
			}

			// 写属性行
			var props []string
			needFull := !hasPrev

			// 朝向
			if !hasPrev || block.Facing != prevFacing {
				props = append(props, "朝向="+block.Facing)
			}
			// 类型
			if !hasPrev || block.Type != prevType {
				props = append(props, "类型="+block.Type)
			}
			// 有条件
			if (!hasPrev && block.Conditional) || (hasPrev && block.Conditional != prevConditional) {
				val := "否"
				if block.Conditional {
					val = "是"
				}
				props = append(props, "有条件="+val)
			}
			// 红石控制：如果 auto=false 则需红石
			needsRS := !block.Auto
			if (!hasPrev && needsRS) || (hasPrev && needsRS != !prevAuto) {
				val := "否"
				if needsRS {
					val = "是"
				}
				props = append(props, "红石控制="+val)
			}
			// 延迟
			if (!hasPrev && block.TickDelay > 0) || (hasPrev && block.TickDelay != prevDelay) {
				props = append(props, fmt.Sprintf("延迟=%d", block.TickDelay))
			}
			// 首次执行（仅循环方块）
			if block.Type == "repeating" {
				if (!hasPrev && block.ExeFirstTick) || (hasPrev && block.ExeFirstTick != prevExeFirst) {
					val := "否"
					if block.ExeFirstTick {
						val = "是"
					}
					props = append(props, "首次执行="+val)
				}
			}

			if needFull || len(props) > 0 {
				sb.WriteString("# " + strings.Join(props, " ") + "\n")
			}

			// 写命令行
			cmd := block.Command
			if block.CustomName != "" {
				cmd = cmd + " #" + block.CustomName
			}
			sb.WriteString(cmd + "\n")

			// 更新 prev
			prevFacing = block.Facing
			prevType = block.Type
			prevConditional = block.Conditional
			prevAuto = block.Auto
			prevDelay = block.TickDelay
			prevExeFirst = block.ExeFirstTick
			hasPrev = true
		}
	}

	return sb.String()
}

// computeCBStats 计算统计信息
func computeCBStats(chains []CBlockChain, content string) *CBSimpleStats {
	stats := &CBSimpleStats{}
	for _, chain := range chains {
		for _, b := range chain.Blocks {
			stats.TotalBlocks++
			switch b.Type {
			case "impulse":
				stats.Pulse++
			case "repeating":
				stats.Repeating++
			case "chain":
				stats.Chain++
			}
			if b.Conditional {
				stats.Conditional++
			} else {
				stats.Unconditional++
			}
			if b.TickDelay > 0 {
				stats.HasDelay++
			}
		}
	}
	stats.ChainCount = len(chains)

	// 统计断链（从 content 中分析）
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "# 断链" {
			stats.Breaks++
		}
	}

	// 从 content 中提取积分板和标签
	stats.Scoreboards = extractScoreboardList(content)
	stats.Tags = extractTagList(content)

	return stats
}

// extractResources 从命令中提取积分板目标和标签名
func extractResources(cmd string, scoreboards, tags map[string]bool) {
	// 提取 scoreboard objectives add ... dummy
	// 提取 scoreboard players add ... <objective>
	// 提取 @a[scores={...}]
	// 提取 tag add/remove ... <tag>

	lower := strings.ToLower(cmd)

	// scoreboard objectives add <name> dummy
	if strings.Contains(lower, "scoreboard") && strings.Contains(lower, "objectives") {
		parts := strings.Fields(cmd)
		for i, p := range parts {
			if strings.ToLower(p) == "objectives" && i+3 < len(parts) {
				if strings.ToLower(parts[i+2]) == "add" || strings.ToLower(parts[i+1]) == "add" {
					// 目标名在 objectives 后面第二个词
					objIdx := i + 2
					if strings.ToLower(parts[i+1]) == "add" {
						objIdx = i + 2
					} else if i+2 < len(parts) && strings.ToLower(parts[i+2]) == "add" {
						objIdx = i + 3
					}
					if objIdx < len(parts) && !strings.HasPrefix(parts[objIdx], "@") && !strings.HasPrefix(parts[objIdx], "[") {
						scoreboards[parts[objIdx]] = true
					}
				}
			}
		}
	}

	// scores 中引用的积分板
	if idx := strings.Index(lower, "scores={"); idx >= 0 {
		// 提取 {...} 内的内容
		braceStart := idx + len("scores={")
		braceEnd := braceStart
		depth := 1
		for braceEnd < len(lower) && depth > 0 {
			if lower[braceEnd] == '{' {
				depth++
			} else if lower[braceEnd] == '}' {
				depth--
			}
			braceEnd++
		}
		inner := cmd[braceStart : braceEnd-1]
		// 按逗号分割键值对
		pairs := splitScorePairs(inner)
		for _, pair := range pairs {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) == 2 {
				scoreboards[strings.TrimSpace(kv[0])] = true
			}
		}
	}

	// scoreboard players set @a <objective> ...
	if strings.Contains(lower, "scoreboard") && strings.Contains(lower, "players") {
		parts := strings.Fields(cmd)
		for i, p := range parts {
			if strings.ToLower(p) == "players" && i+2 < len(parts) {
				// players <target> <operation> <objective> ...
				// 操作: add, remove, set, reset, get, test, operation
				ops := map[string]bool{"add": true, "remove": true, "set": true, "reset": true, "get": true, "test": true, "operation": true}
				if i+3 < len(parts) && ops[strings.ToLower(parts[i+2])] {
					objName := parts[i+3]
					if !strings.HasPrefix(objName, "@") && !strings.HasPrefix(objName, "[") {
						scoreboards[objName] = true
					}
				}
			}
		}
	}

	// tag @a add/remove <tag>
	if strings.Contains(lower, "tag ") {
		parts := strings.Fields(cmd)
		for i, p := range parts {
			if strings.ToLower(p) == "tag" && i+2 < len(parts) {
				op := strings.ToLower(parts[i+2])
				if op == "add" || op == "remove" {
					if i+3 < len(parts) {
						tagName := parts[i+3]
						if !strings.HasPrefix(tagName, "@") && !strings.HasPrefix(tagName, "[") {
							tags[tagName] = true
						}
					}
				}
			}
		}
	}
}

// splitScorePairs 分割 scores={...} 内的键值对
func splitScorePairs(s string) []string {
	var pairs []string
	var current strings.Builder
	depth := 0
	for _, ch := range s {
		switch ch {
		case ',':
			if depth == 0 {
				pairs = append(pairs, current.String())
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		case '{':
			depth++
			current.WriteRune(ch)
		case '}':
			depth--
			current.WriteRune(ch)
		default:
			current.WriteRune(ch)
		}
	}
	if current.Len() > 0 {
		pairs = append(pairs, current.String())
	}
	return pairs
}

// extractScoreboardList 从 .mcfunction 内容中提取积分板列表
func extractScoreboardList(content string) []string {
	seen := make(map[string]bool)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//积分板:") {
			val := strings.TrimPrefix(trimmed, "//积分板:")
			parts := strings.Split(val, ",")
			for _, p := range parts {
				s := strings.TrimSpace(p)
				if s != "" {
					seen[s] = true
				}
			}
		}
		// 也从命令中收集
		if !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "#") && trimmed != "" {
			extractResources(trimmed, seen, nil)
		}
	}
	var result []string
	for s := range seen {
		result = append(result, s)
	}
	return result
}

// extractTagList 从 .mcfunction 内容中提取标签列表
func extractTagList(content string) []string {
	seen := make(map[string]bool)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//标签:") {
			val := strings.TrimPrefix(trimmed, "//标签:")
			parts := strings.Split(val, ",")
			for _, p := range parts {
				s := strings.TrimSpace(p)
				if s != "" {
					seen[s] = true
				}
			}
		}
		if !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "#") && trimmed != "" {
			extractResources(trimmed, nil, seen)
		}
	}
	var result []string
	for s := range seen {
		result = append(result, s)
	}
	return result
}

// 处理 .mcfunction 文件导出（保存到本地）
func handleCBExportSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content  string `json:"content"`
		FilePath string `json:"file_path"`
		FileName string `json:"file_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}

	if req.FilePath == "" {
		// 默认路径
		req.FilePath = "/storage/emulated/0/Download/"
	}
	if req.FileName == "" {
		req.FileName = "cb_project.mcfunction"
	}
	if !strings.HasSuffix(req.FileName, ".mcfunction") {
		req.FileName += ".mcfunction"
	}

	fullPath := req.FilePath
	if !strings.HasSuffix(fullPath, "/") {
		fullPath += "/"
	}
	fullPath += req.FileName

	if err := os.WriteFile(fullPath, []byte(req.Content), 0644); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "保存失败: " + err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "path": fullPath})
}

// ========== 辅助函数 ==========

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}


