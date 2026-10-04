package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bot-apk/state"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

// ═══════════════════════════════════════════════════════════════
// 命令方块工程 — 导入引擎
// ═══════════════════════════════════════════════════════════════

// CBParsedBlock 解析后的单个命令方块
type CBParsedBlock struct {
	Command     string `json:"command"`
	Facing      string `json:"facing"`       // 朝向
	Type        string `json:"type"`         // impulse / repeating / chain
	Conditional bool   `json:"conditional"`
	NeedsRS     bool   `json:"needs_rs"`     // 需要红石
	TickDelay   int    `json:"tick_delay"`
	ExeFirst    bool   `json:"exe_first"`    // 首次执行（仅循环）
	CustomName  string `json:"custom_name,omitempty"`
	ChainBreak  bool   `json:"chain_break"`  // 断链标记
}

// CBParsedProject 解析后的工程文件
type CBParsedProject struct {
	Name        string           `json:"name"`
	Version     string           `json:"version"`
	Author      string           `json:"author"`
	Description string           `json:"description"`
	Scoreboards []string         `json:"scoreboards"`
	Tags        []string         `json:"tags"`
	Blocks      []CBParsedBlock  `json:"blocks"`
	Warnings    []string         `json:"warnings"`
}

// CBImportRequest 导入请求
type CBImportRequest struct {
	Content string `json:"content"`
	StartX  int    `json:"start_x"`
	StartY  int    `json:"start_y"`
	StartZ  int    `json:"start_z"`
}

// ========== 解析引擎 ==========

// parseCBProject 解析 .mcfunction 工程文件内容
func parseCBProject(content string) *CBParsedProject {
	proj := &CBParsedProject{
		Blocks:   make([]CBParsedBlock, 0),
		Warnings: make([]string, 0),
	}

	lines := strings.Split(content, "\n")

	// 解析 // 元数据（文件头部）
	metaEnd := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "//") {
			metaEnd = i
			break
		}
		metaEnd = i + 1
		parseCBMetaLine(trimmed, proj)
	}

	// 当前属性（继承用）
	curFacing := "z+"
	curType := "impulse"
	curConditional := false
	curNeedsRS := false
	curDelay := 0
	curExeFirst := false

	// 解析 # 属性和命令行
	for i := metaEnd; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "//") {
			continue // 元数据已经解析完，跳过
		}

		if strings.HasPrefix(trimmed, "#") {
			// 检查是否是断链
			afterHash := strings.TrimSpace(trimmed[1:])
			if afterHash == "断链" {
				// 断链标记
				proj.Blocks = append(proj.Blocks, CBParsedBlock{
					Facing:     curFacing,
					Type:       curType,
					ChainBreak: true,
				})
				continue
			}

			// 解析属性行
			parseCBAttributes(afterHash, &curFacing, &curType, &curConditional, &curNeedsRS, &curDelay, &curExeFirst)
			continue
		}

		// 命令行（可能有行末注释）
		cmd, customName := parseCBLineComment(trimmed)
		if cmd == "" {
			continue
		}

		proj.Blocks = append(proj.Blocks, CBParsedBlock{
			Command:     cmd,
			Facing:      curFacing,
			Type:        curType,
			Conditional: curConditional,
			NeedsRS:     curNeedsRS,
			TickDelay:   curDelay,
			ExeFirst:    curExeFirst,
			CustomName:  customName,
		})
	}

	// 验证
	if len(proj.Blocks) > 0 && proj.Name == "" {
		proj.Name = "未命名工程"
	}
	if len(proj.Blocks) == 0 {
		proj.Warnings = append(proj.Warnings, "工程中未找到命令方块")
	}

	return proj
}

// parseCBMetaLine 解析单行 // 元数据
func parseCBMetaLine(line string, proj *CBParsedProject) {
	// 去掉 // 前缀
	content := strings.TrimSpace(strings.TrimPrefix(line, "//"))

	// 支持 : 和 = 分隔符
	sep := ":"
	if strings.Contains(content, "=") && !strings.Contains(content, ":") {
		sep = "="
	}

	idx := strings.Index(content, sep)
	if idx < 0 {
		return // 普通注释，忽略
	}

	key := strings.TrimSpace(content[:idx])
	val := strings.TrimSpace(content[idx+len(sep):])

	switch key {
	case "工程", "project":
		proj.Name = val
	case "版本", "version":
		proj.Version = val
	case "作者", "author":
		proj.Author = val
	case "说明", "description", "desc":
		proj.Description = val
	case "积分板", "scoreboards":
		parts := strings.Split(val, ",")
		for _, p := range parts {
			s := strings.TrimSpace(p)
			if s != "" {
				proj.Scoreboards = append(proj.Scoreboards, s)
			}
		}
	case "标签", "tags":
		parts := strings.Split(val, ",")
		for _, p := range parts {
			s := strings.TrimSpace(p)
			if s != "" {
				proj.Tags = append(proj.Tags, s)
			}
		}
	}
}

// parseCBAttributes 解析 # 属性行
func parseCBAttributes(line string, curFacing, curType *string, curConditional, curNeedsRS *bool, curDelay *int, curExeFirst *bool) {
	parts := strings.Fields(line)
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			// 无 = 的标记，如 "断链"
			if part == "断链" {
				// 这个在外部处理
			}
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])

		switch key {
		case "朝向", "facing":
			*curFacing = val
			// 验证朝向
			if _, ok := cbFacingOffset[val]; !ok {
				*curFacing = "z+"
			}
		case "类型", "type":
			switch val {
			case "脉冲", "impulse":
				*curType = "impulse"
			case "循环", "repeating":
				*curType = "repeating"
			case "连锁", "chain":
				*curType = "chain"
			default:
				*curType = "impulse"
			}
		case "有条件", "conditional":
			*curConditional = val == "是" || val == "true" || val == "1"
		case "红石控制", "redstone", "needs_rs":
			*curNeedsRS = val == "是" || val == "true" || val == "1"
		case "延迟", "delay", "tick_delay":
			if n, err := strconv.Atoi(val); err == nil && n >= 0 {
				*curDelay = n
			}
		case "首次执行", "execute_first_tick", "exe_first":
			*curExeFirst = val == "是" || val == "true" || val == "1"
		}
	}
}

// parseCBLineComment 解析行末 # 注释
func parseCBLineComment(line string) (cmd, customName string) {
	// 从最后一个 # 开始识别为注释
	// 注意：命令本身可能包含 #，如 /say #1
	hashIdx := -1
	for i := len(line) - 1; i >= 0; i-- {
		if line[i] == '#' {
			// 检查是否是 # 开头（属性行）
			if i == 0 || line[i-1] == ' ' {
				hashIdx = i
				break
			}
		}
	}

	if hashIdx >= 0 {
		cmd = strings.TrimSpace(line[:hashIdx])
		customName = strings.TrimSpace(line[hashIdx+1:])
	} else {
		cmd = strings.TrimSpace(line)
	}
	return
}

// ========== 导入执行 ==========

// 导入命令方块（通过任务系统）
func runCBImportTask(tc *state.TaskController, p map[string]any) {
	content := getString(p, "content", "")
	if content == "" {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "缺少工程内容"}))
		return
	}

	startX := getInt(p, "start_x", 0)
	startY := getInt(p, "start_y", 64)
	startZ := getInt(p, "start_z", 0)

	bm := getBotManager()
	if bm == nil || !bm.IsConnected() {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "未连接到服务器"}))
		return
	}

	proj := parseCBProject(content)

	if len(proj.Warnings) > 0 {
		hub.Emit("progress", mustJSON(map[string]any{
			"msg": fmt.Sprintf("警告: %s", strings.Join(proj.Warnings, "; ")),
		}))
	}

	// 过滤掉断链标记，只保留实际方块
	var placeBlocks []CBParsedBlock
	for _, b := range proj.Blocks {
		if !b.ChainBreak {
			placeBlocks = append(placeBlocks, b)
		}
	}

	total := len(placeBlocks)
	if total == 0 {
		hub.Emit("task_error", mustJSON(map[string]any{"error": "没有需要放置的命令方块"}))
		return
	}

	hub.Emit("task_start", mustJSON(map[string]any{
		"type": fmt.Sprintf("命令方块导入: %s (%d 个方块)", proj.Name, total),
	}))

	hub.Emit("progress", mustJSON(map[string]any{
		"msg": fmt.Sprintf("开始导入 %s: %d 个命令方块", proj.Name, total),
	}))

	// 导入前检查
	if bm.IsOP() {
		bm.SendTellraw(fmt.Sprintf("§a§l▍命令方块导入:§r§f%s §7%d 个方块 §7起点:(%d,%d,%d)",
			proj.Name, total, startX, startY, startZ))
	}

	// 逐个放置
	x, y, z := startX, startY, startZ

	for i, block := range placeBlocks {
		select {
		case <-tc.StopCh():
			hub.Emit("task_error", mustJSON(map[string]any{"error": "导入已取消"}))
			return
		default:
		}

		// 1. 放置命令方块
		blockName := getBlockName(block.Type, block.Conditional)

		// 构建 facing_direction 状态
		fd := facingToDirection(block.Facing)
		condBit := 0
if block.Conditional {
	condBit = 1
}
states := fmt.Sprintf(`["facing_direction"=%d,"conditional_bit"=%d]`, fd, condBit)

		err := bm.SendBlockCmd(fmt.Sprintf("setblock %d %d %d %s %s", x, y, z, blockName, states))
		if err != nil {
			hub.Emit("progress", mustJSON(map[string]any{
				"msg": fmt.Sprintf("放置失败 (%d,%d,%d): %v", x, y, z, err),
			}))
			continue
		}

		// 等待 setblock 生效
		time.Sleep(50 * time.Millisecond)

		// 2. 发送 CommandBlockUpdate 包设置命令和数据
		if bm.conn != nil {
			// TP 靠近确保区块加载
			bm.TP(x, y+1, z)

			mode := uint32(0)
			switch block.Type {
			case "repeating":
				mode = 1
			case "chain":
				mode = 2
			}

			bm.conn.WritePacket(&packet.CommandBlockUpdate{
				Block: true, Position: protocol.BlockPos{int32(x), int32(y), int32(z)},
				Mode:              mode,
				NeedsRedstone:     block.NeedsRS,
				Conditional:       block.Conditional,
				Command:           block.Command,
				Name:              block.CustomName,
				ShouldTrackOutput: true,
				ExecuteOnFirstTick: block.ExeFirst,
				TickDelay:         int32(block.TickDelay),
			})

			time.Sleep(30 * time.Millisecond)
		}

		// 更新进度
		progress := float64(i+1) / float64(total)
		hub.Emit("task_progress", mustJSON(map[string]any{"progress": progress}))
		hub.Emit("progress", mustJSON(map[string]any{
			"msg": fmt.Sprintf("[%d/%d] %s", i+1, total, truncateStr(block.Command, 50)),
		}))

		// 按朝向移动到下一个位置
		off, ok := cbFacingOffset[block.Facing]
		if ok {
			x += off[0]
			y += off[1]
			z += off[2]
		} else {
			// 默认 z+
			z++
		}
	}

	hub.Emit("task_done", mustJSON(map[string]any{
		"msg": fmt.Sprintf("导入完成: %s (%d/%d 个方块)", proj.Name, total, total),
	}))
}

// getBlockName 获取命令方块类型名
func getBlockName(blockType string, conditional bool) string {
	switch blockType {
	case "repeating":
		if conditional {
			return "repeating_command_block"
		}
		return "repeating_command_block"
	case "chain":
		if conditional {
			return "chain_command_block"
		}
		return "chain_command_block"
	default:
		if conditional {
			return "command_block"
		}
		return "command_block"
	}
}

// facingToDirection 将朝向名转为 facing_direction 值（0-5）
func facingToDirection(facing string) int {
	for fd, name := range cbFacingNames {
		if name == facing {
			return fd
		}
	}
	return 3 // 默认 z+
}

// truncateStr 截断字符串
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ========== API 端点 ==========

// handleCBImportPreview 导入预览（解析文件并返回统计）
func handleCBImportPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}

	proj := parseCBProject(req.Content)

	// 统计
	total := 0
	pulse := 0
	repeating := 0
	chain := 0
	cond := 0
	uncond := 0
	hasDelay := 0
	breaks := 0

	for _, b := range proj.Blocks {
		if b.ChainBreak {
			breaks++
			continue
		}
		total++
		switch b.Type {
		case "impulse":
			pulse++
		case "repeating":
			repeating++
		case "chain":
			chain++
		}
		if b.Conditional {
			cond++
		} else {
			uncond++
		}
		if b.TickDelay > 0 {
			hasDelay++
		}
	}

	// 链结构预览
	chainCount := 0
	chainSizes := make([]int, 0)
	currentChainSize := 0
	for _, b := range proj.Blocks {
		if b.ChainBreak {
			if currentChainSize > 0 {
				chainSizes = append(chainSizes, currentChainSize)
				chainCount++
				currentChainSize = 0
			}
			continue
		}
		currentChainSize++
		if b.Type == "impulse" || b.Type == "repeating" {
			if currentChainSize > 1 {
				chainSizes = append(chainSizes, currentChainSize)
				chainCount++
				currentChainSize = 0
			}
			currentChainSize = 1
		}
	}
	if currentChainSize > 0 {
		chainSizes = append(chainSizes, currentChainSize)
		chainCount++
		_ = chainSizes
	}

	writeJSON(w, map[string]any{
		"ok":   true,
		"project": proj,
		"stats": map[string]any{
			"total":     total,
			"pulse":     pulse,
			"repeating": repeating,
			"chain":     chain,
			"conditional":    cond,
			"unconditional":  uncond,
			"has_delay": hasDelay,
			"breaks":    breaks,
			"chain_count": chainCount,
		},
	})
}

// handleCBImportStart 开始导入
func handleCBImportStart(w http.ResponseWriter, r *http.Request) {
	var req CBImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}

	if req.Content == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "缺少工程内容"})
		return
	}

	// 检查机器人连接
	botMgrMu.Lock()
	connected := botMgr != nil && botMgr.IsConnected()
	botMgrMu.Unlock()
	if !connected {
		writeJSON(w, map[string]any{"ok": false, "error": "未连接到服务器，请先连接"})
		return
	}

	// 通过任务系统启动导入
	taskMu.Lock()
	defer taskMu.Unlock()
	if activeTask != nil && activeTask.Running() {
		writeJSON(w, map[string]any{"ok": false, "error": "已有任务运行中"})
		return
	}

	tc := state.NewTaskController(hub)
	activeTask = tc

	params := map[string]any{
		"content":  req.Content,
		"start_x":  req.StartX,
		"start_y":  req.StartY,
		"start_z":  req.StartZ,
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				hub.Emit("task_error", mustJSON(map[string]any{"error": fmt.Sprintf("导入异常崩溃: %v", r)}))
			}
			taskMu.Lock()
			if activeTask == tc {
				activeTask = nil
			}
			taskMu.Unlock()
		}()
		runCBImportTask(tc, params)
	}()

	writeJSON(w, map[string]any{"ok": true})
}