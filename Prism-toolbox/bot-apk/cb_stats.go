package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ═══════════════════════════════════════════════════════════════
// 命令方块工程 — 统计引擎
// ═══════════════════════════════════════════════════════════════

// CBFullStats 完整的统计信息
type CBFullStats struct {
	Overview   *CBOverviewStats   `json:"overview"`
	Resources  *CBResourceStats   `json:"resources"`
	Chains     []CBChainPreview   `json:"chains"`
}

// CBOverviewStats 概览统计
type CBOverviewStats struct {
	TotalBlocks  int `json:"total_blocks"`
	ChainCount   int `json:"chain_count"`
	Pulse        int `json:"pulse"`
	Repeating    int `json:"repeating"`
	Chain        int `json:"chain"`
	Conditional  int `json:"conditional"`
	Unconditional int `json:"unconditional"`
	HasDelay     int `json:"has_delay"`
	Breaks       int `json:"breaks"`
}

// CBResourceStats 资源依赖统计
type CBResourceStats struct {
	Scoreboards []string `json:"scoreboards"`
	Tags        []string `json:"tags"`
}

// CBChainPreview 链结构预览
type CBChainPreview struct {
	Index      int      `json:"index"`
	Size       int      `json:"size"`
	Direction  string   `json:"direction"`
	Steps      []string `json:"steps"` // 每个方块的描述，如 "z+ 脉冲", "连锁(有条件)"
	Breaks     int      `json:"breaks"`
}

// handleCBStats 处理统计请求
func handleCBStats(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}

	if req.Content == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "内容为空"})
		return
	}

	stats := computeCBFullStats(req.Content)
	writeJSON(w, map[string]any{"ok": true, "stats": stats})
}

// computeCBFullStats 计算完整统计信息
func computeCBFullStats(content string) *CBFullStats {
	proj := parseCBProject(content)

	stats := &CBFullStats{
		Overview:  &CBOverviewStats{},
		Resources: &CBResourceStats{},
		Chains:    make([]CBChainPreview, 0),
	}

	// 概览统计
	for _, b := range proj.Blocks {
		if b.ChainBreak {
			stats.Overview.Breaks++
			continue
		}
		stats.Overview.TotalBlocks++
		switch b.Type {
		case "impulse":
			stats.Overview.Pulse++
		case "repeating":
			stats.Overview.Repeating++
		case "chain":
			stats.Overview.Chain++
		}
		if b.Conditional {
			stats.Overview.Conditional++
		} else {
			stats.Overview.Unconditional++
		}
		if b.TickDelay > 0 {
			stats.Overview.HasDelay++
		}
	}

	// 资源依赖
	stats.Resources.Scoreboards = proj.Scoreboards
	if stats.Resources.Scoreboards == nil {
		stats.Resources.Scoreboards = []string{}
	}
	stats.Resources.Tags = proj.Tags
	if stats.Resources.Tags == nil {
		stats.Resources.Tags = []string{}
	}

	// 链结构预览
	stats.Overview.ChainCount = 0
	var currentChain []CBParsedBlock
	chainBreaks := 0

	for _, b := range proj.Blocks {
		if b.ChainBreak {
			chainBreaks++
			if len(currentChain) > 0 {
				// 结束当前链
				stats.Chains = append(stats.Chains, buildChainPreview(len(stats.Chains)+1, currentChain))
				stats.Overview.ChainCount++
				currentChain = nil
			}
			continue
		}

		// 如果是脉冲/循环，且当前链已有内容，则开始新链
		if (b.Type == "impulse" || b.Type == "repeating") && len(currentChain) > 0 {
			stats.Chains = append(stats.Chains, buildChainPreview(len(stats.Chains)+1, currentChain))
			stats.Overview.ChainCount++
			currentChain = nil
		}

		currentChain = append(currentChain, b)
	}

	// 最后一条链
	if len(currentChain) > 0 {
		stats.Chains = append(stats.Chains, buildChainPreview(len(stats.Chains)+1, currentChain))
		stats.Overview.ChainCount++
	}

	// 如果没有链，但方块数 > 0，则算作一条链
	if stats.Overview.ChainCount == 0 && stats.Overview.TotalBlocks > 0 {
		stats.Overview.ChainCount = 1
	}

	return stats
}

// buildChainPreview 构建单条链的预览
func buildChainPreview(index int, blocks []CBParsedBlock) CBChainPreview {
	preview := CBChainPreview{
		Index: index,
		Size:  0,
		Steps: make([]string, 0),
		Direction: "z+",
	}

	for _, b := range blocks {
		preview.Size++
		preview.Direction = b.Facing

		step := b.Facing + " "
		switch b.Type {
		case "impulse":
			step += "脉冲"
		case "repeating":
			step += "循环"
		case "chain":
			step += "连锁"
		}
		if b.Conditional {
			step += "(有条件)"
		}
		preview.Steps = append(preview.Steps, step)
	}

	return preview
}

// ========== 前端 API：统计展示 ==========

// handleCBStatsPreview 返回统计预览 HTML（前端用）
func handleCBStatsPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}

	stats := computeCBFullStats(req.Content)

	// 构建链结构文本描述
	var chainDescs []string
	for _, c := range stats.Chains {
		stepStr := strings.Join(c.Steps, " → ")
		chainDescs = append(chainDescs, stepStr)
	}

	writeJSON(w, map[string]any{
		"ok":    true,
		"stats": stats,
		"chains": chainDescs,
	})
}