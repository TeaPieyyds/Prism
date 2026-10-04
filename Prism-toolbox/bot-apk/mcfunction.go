package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ========== 数据结构 ==========

type ParsedFile struct {
	Groups     []CommandGroup `json:"groups"`
	Tests      []TestDef      `json:"tests"`
	Executions []ExecRecord   `json:"executions"`
	Warnings   []string       `json:"warnings"`
}

type CommandGroup struct {
	Name      string       `json:"name"`
	Commands  []string     `json:"commands"`
	Config    GroupConfig  `json:"config"`
	LineNum   int          `json:"line_num"`
	HasConfig bool         `json:"has_config"`
}

type GroupConfig struct {
	LoopCount int    `json:"loop_count"`
	Channel   string `json:"channel"`
	Speed     int    `json:"speed"`
	Prefix    string `json:"prefix"`
}

type TestDef struct {
	Name     string            `json:"name"`
	Params   map[string]string `json:"params"`
	Commands []string          `json:"commands"`
	LineNum  int               `json:"line_num"`
}

type ExecRecord struct {
	Name    string            `json:"name"`
	Params  map[string]string `json:"params"`
	LineNum int               `json:"line_num"`
}

// ========== 默认配置 ==========

func defaultGroupConfig() GroupConfig {
	return GroupConfig{
		LoopCount: 1,
		Channel:   "",
		Speed:     0,
		Prefix:    "",
	}
}

// ========== 解析引擎 ==========

func parseMcfunction(content string) ParsedFile {
	var result ParsedFile
	lines := strings.Split(content, "\n")
	var currentGroup *CommandGroup
	var currentTest *TestDef
	ungroupedCount := 0

	// 正则：匹配 #组/群/分区/类别 + 分隔符 + 名字
	groupRe := regexp.MustCompile(`^#\s*[组群分区类别][：: 　]\s*(.+)$`)
	// 正则：匹配 #定义/测试/规定 + 分隔符 + 名字
	testRe := regexp.MustCompile(`^#\s*[定义测试规定][：: 　]\s*(.+)$`)
	// 正则：匹配 #执行/运行 + 分隔符 + 名字
	execRe := regexp.MustCompile(`^#\s*[执行运行][：: 　]\s*(.+)$`)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			// 检查是否是 #组
			if m := groupRe.FindStringSubmatch(trimmed); m != nil {
				// 保存当前组
				if currentGroup != nil {
					result.Groups = append(result.Groups, *currentGroup)
				}
				// 保存当前测试（如果存在）
				if currentTest != nil {
					result.Tests = append(result.Tests, *currentTest)
					currentTest = nil
				}
				raw := strings.TrimSpace(m[1])
				name, config := parseGroupConfig(raw)
				currentGroup = &CommandGroup{
					Name:      name,
					Commands:  []string{},
					Config:    config,
					HasConfig: config != defaultGroupConfig(),
					LineNum:   lineNum,
				}
				continue
			}

			// 检查是否是 #测试/定义/规定
			if m := testRe.FindStringSubmatch(trimmed); m != nil {
				if currentTest != nil {
					result.Tests = append(result.Tests, *currentTest)
				}
				raw := strings.TrimSpace(m[1])
				name, params := parseTestParams(raw)
				currentTest = &TestDef{
					Name:     name,
					Params:   params,
					Commands: []string{},
					LineNum:  lineNum,
				}
				continue
			}

			// 检查是否是 #执行/运行
			if m := execRe.FindStringSubmatch(trimmed); m != nil {
				raw := strings.TrimSpace(m[1])
				name, params := parseExecParams(raw)
				result.Executions = append(result.Executions, ExecRecord{
					Name:    name,
					Params:  params,
					LineNum: lineNum,
				})
				continue
			}

			// 纯注释，跳过
			continue
		}

		// 非注释行：归入当前测试或组
		if currentTest != nil {
			currentTest.Commands = append(currentTest.Commands, trimmed)
		} else if currentGroup != nil {
			currentGroup.Commands = append(currentGroup.Commands, trimmed)
		} else {
			// 前面没有组，自动创建"未分组"
			ungroupedCount++
			if ungroupedCount == 1 {
				currentGroup = &CommandGroup{
					Name:     "未分组",
					Commands: []string{trimmed},
					Config:   defaultGroupConfig(),
					LineNum:  lineNum,
				}
			} else if currentGroup != nil && currentGroup.Name == "未分组" {
				currentGroup.Commands = append(currentGroup.Commands, trimmed)
			}
		}
	}

	// 保存最后一个组/测试
	if currentGroup != nil {
		result.Groups = append(result.Groups, *currentGroup)
	}
	if currentTest != nil {
		result.Tests = append(result.Tests, *currentTest)
	}

	return result
}

// ========== 配置解析辅助函数 ==========

// parseGroupConfig 解析 "#组:建造 次数=2 通道=ai 速率=50"
func parseGroupConfig(raw string) (string, GroupConfig) {
	cfg := defaultGroupConfig()
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return "未命名组", cfg
	}
	name := parts[0]
	for _, p := range parts[1:] {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := kv[0]
		val := kv[1]
		switch key {
		case "次数", "loop", "count":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.LoopCount = n
			}
		case "通道", "channel", "ch":
			cfg.Channel = normalizeChannel(val)
		case "速率", "speed", "rate":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.Speed = n
			}
		case "前缀", "prefix", "pre":
			cfg.Prefix = val
		}
	}
	return name, cfg
}

// normalizeChannel 将通道别名转为标准值
// 支持: ai/魔法指令/0, player/玩家/1, console/控制台/2
func normalizeChannel(val string) string {
	switch val {
	case "0", "ai", "魔法指令", "magic":
		return "ai"
	case "1", "player", "玩家":
		return "player"
	case "2", "console", "控制台":
		return "console"
	default:
		return val
	}
}

// parseTestParams 解析 "#测试:高频 坐标:≤~ ~1 ~≥ 次数:≤50≥"
func parseTestParams(raw string) (string, map[string]string) {
	params := make(map[string]string)
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return "未命名", params
	}
	name := parts[0]
	for _, p := range parts[1:] {
		kv := strings.SplitN(p, ":", 2)
		if len(kv) != 2 {
			// 尝试用 = 分割
			kv = strings.SplitN(p, "=", 2)
			if len(kv) != 2 {
				continue
			}
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		// 去掉 ≤≥ 包裹
		val = strings.TrimPrefix(val, "≤")
		val = strings.TrimSuffix(val, "≥")
		params[key] = val
	}
	return name, params
}

// parseExecParams 解析 "#执行:高频 坐标=~ ~2 ~ 次数=100"
func parseExecParams(raw string) (string, map[string]string) {
	params := make(map[string]string)
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return "未命名", params
	}
	name := parts[0]
	for _, p := range parts[1:] {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		params[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}
	return name, params
}

// ========== API 端点 ==========

func handleMcfunctionParse(w http.ResponseWriter, r *http.Request) {
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
	result := parseMcfunction(req.Content)
	writeJSON(w, map[string]any{"ok": true, "parsed": result})
}

// ========== 执行引擎 ==========

type ExecuteRequest struct {
	Content string                `json:"content"`
	Global  GlobalExecuteConfig   `json:"global"`
	Groups  map[string]GroupExecConfig `json:"groups"`
	Tests   []TestExecConfig      `json:"tests"`
}

type GlobalExecuteConfig struct {
	Channel string `json:"channel"`
	Speed   int    `json:"speed"`
	Prefix  string `json:"prefix"`
	Loop    int    `json:"loop"`
}

type GroupExecConfig struct {
	Enabled   bool   `json:"enabled"`
	LoopCount int    `json:"loop_count"`
	Channel   string `json:"channel"`
	Speed     int    `json:"speed"`
	Prefix    string `json:"prefix"`
}

type TestExecConfig struct {
	Name    string            `json:"name"`
	Params  map[string]string `json:"params"`
	Enabled bool              `json:"enabled"`
}

type ExecLogEntry struct {
	Group   string `json:"group"`
	Command string `json:"command"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
	Channel string `json:"channel"`
	Loop    int    `json:"loop"`
}

var (
	mcfnMu         sync.Mutex
	mcfnRunning    bool
	mcfnStopCh     chan struct{}
	mcfnLog        []ExecLogEntry
	mcfnTotal      int
	mcfnDone       int
	mcfnCurrentGrp string
	mcfnCurrentCmd string
)

func executeMcfunction(req ExecuteRequest) {
	parsed := parseMcfunction(req.Content)
	config := req.Global
	groupOverrides := req.Groups

	mcfnMu.Lock()
	mcfnRunning = true
	mcfnStopCh = make(chan struct{})
	mcfnLog = nil
	mcfnTotal = 0
	mcfnDone = 0
	mcfnCurrentGrp = ""
	mcfnCurrentCmd = ""

	// 计算总数
	for _, g := range parsed.Groups {
		gc := groupOverrides[g.Name]
		loop := g.Config.LoopCount
		if gc.LoopCount > 0 {
			loop = gc.LoopCount
		}
		enabled := true
		if _, ok := groupOverrides[g.Name]; ok {
			enabled = gc.Enabled
		}
		if enabled {
			mcfnTotal += len(g.Commands) * loop
		}
	}
	for _, t := range req.Tests {
		if t.Enabled {
			for _, td := range parsed.Tests {
				if td.Name == t.Name {
					mcfnTotal += len(td.Commands)
					break
				}
			}
		}
	}
	mcfnMu.Unlock()

	stopCh := mcfnStopCh

	// 发送命令
	sendCmd := func(channel, cmd string) error {
		botMgrMu.Lock()
		defer botMgrMu.Unlock()
		if botMgr == nil || !botMgr.IsConnected() {
			return fmt.Errorf("未连接")
		}
		switch channel {
		case "player":
			_, err := botMgr.SendConsole("/" + cmd)
			return err
		case "console":
			_, err := botMgr.SendConsole("/" + cmd)
			return err
		case "ai", "":
			fallthrough
		default:
			return botMgr.SendWOCmd(cmd)
		}
	}

	// 逐组执行
	for _, g := range parsed.Groups {
		select {
		case <-stopCh:
			mcfnMu.Lock()
			mcfnRunning = false
			mcfnMu.Unlock()
			return
		default:
		}

		gc := groupOverrides[g.Name]
		enabled := true
		if _, ok := groupOverrides[g.Name]; ok {
			enabled = gc.Enabled
		}
		if !enabled {
			continue
		}

		channel := config.Channel
		if gc.Channel != "" {
			channel = gc.Channel
		}
		if channel == "" {
			channel = "ai"
		}

		prefix := config.Prefix
		if gc.Prefix != "" {
			prefix = gc.Prefix
		}

		loop := g.Config.LoopCount
		if gc.LoopCount > 0 {
			loop = gc.LoopCount
		}

		speed := config.Speed
		if gc.Speed > 0 {
			speed = gc.Speed
		}
		if speed <= 0 {
			speed = 50
		}
		if channel == "player" && speed < 100 {
			speed = 100
		}

		for li := 0; li < loop; li++ {
			for _, cmd := range g.Commands {
				select {
				case <-stopCh:
					mcfnMu.Lock()
					mcfnRunning = false
					mcfnMu.Unlock()
					return
				default:
				}

				fullCmd := cmd
				if prefix != "" {
					fullCmd = prefix + " " + cmd
				}

				mcfnMu.Lock()
				mcfnCurrentGrp = g.Name
				mcfnCurrentCmd = cmd
				mcfnMu.Unlock()

				entry := ExecLogEntry{
					Group:   g.Name,
					Command: cmd,
					Channel: channel,
					Loop:    li + 1,
				}

				err := sendCmd(channel, fullCmd)
				if err != nil {
					entry.Status = "fail"
					entry.Error = err.Error()
				} else {
					entry.Status = "ok"
				}

				mcfnMu.Lock()
				mcfnDone++
				mcfnLog = append(mcfnLog, entry)
				mcfnMu.Unlock()

				if speed > 0 {
					time.Sleep(time.Duration(speed) * time.Millisecond)
				}
			}
		}
	}

	// 执行测试命令
	for _, t := range req.Tests {
		if !t.Enabled {
			continue
		}
		for _, td := range parsed.Tests {
			if td.Name != t.Name {
				continue
			}
			channel := config.Channel
			if channel == "" {
				channel = "ai"
			}
			speed := config.Speed
			if speed <= 0 {
				speed = 50
			}

			for _, cmd := range td.Commands {
				select {
				case <-stopCh:
					mcfnMu.Lock()
					mcfnRunning = false
					mcfnMu.Unlock()
					return
				default:
				}

				// 替换占位符
				resolved := cmd
				for k, v := range t.Params {
					resolved = strings.ReplaceAll(resolved, "≤"+k+"≥", v)
				}

				mcfnMu.Lock()
				mcfnCurrentGrp = "测试:" + td.Name
				mcfnCurrentCmd = resolved
				mcfnMu.Unlock()

				entry := ExecLogEntry{
					Group:   "测试:" + td.Name,
					Command: resolved,
					Channel: channel,
					Status:  "ok",
				}

				err := sendCmd(channel, resolved)
				if err != nil {
					entry.Status = "fail"
					entry.Error = err.Error()
				}

				mcfnMu.Lock()
				mcfnDone++
				mcfnLog = append(mcfnLog, entry)
				mcfnMu.Unlock()

				if speed > 0 {
					time.Sleep(time.Duration(speed) * time.Millisecond)
				}
			}
			break
		}
	}

	mcfnMu.Lock()
	mcfnRunning = false
	mcfnMu.Unlock()
}

func handleMcfunctionExecute(w http.ResponseWriter, r *http.Request) {
	var req ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON"})
		return
	}
	if req.Content == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "内容为空"})
		return
	}

	mcfnMu.Lock()
	if mcfnRunning {
		mcfnMu.Unlock()
		writeJSON(w, map[string]any{"ok": false, "error": "已有任务执行中"})
		return
	}
	mcfnMu.Unlock()

	go executeMcfunction(req)

	writeJSON(w, map[string]any{"ok": true})
}

func handleMcfunctionStop(w http.ResponseWriter, r *http.Request) {
	mcfnMu.Lock()
	if mcfnRunning && mcfnStopCh != nil {
		close(mcfnStopCh)
		mcfnRunning = false
	}
	mcfnMu.Unlock()
	writeJSON(w, map[string]any{"ok": true})
}

func handleMcfunctionStatus(w http.ResponseWriter, r *http.Request) {
	mcfnMu.Lock()
	running := mcfnRunning
	total := mcfnTotal
	done := mcfnDone
	grp := mcfnCurrentGrp
	cmd := mcfnCurrentCmd
	log := make([]ExecLogEntry, len(mcfnLog))
	copy(log, mcfnLog)
	mcfnMu.Unlock()

	progress := 0.0
	if total > 0 {
		progress = float64(done) / float64(total)
	}

	writeJSON(w, map[string]any{
		"ok":             running || done > 0,
		"running":        running,
		"total":          total,
		"done":           done,
		"progress":       progress,
		"current_group":  grp,
		"current_command": cmd,
		"log":            log,
	})
}

// handleMcfunctionSyntax 读取 .mtsx 语法文件，提取颜色定义和关键模式
func handleMcfunctionSyntax(w http.ResponseWriter, r *http.Request) {
	// 尝试读取语法文件
	paths := []string{
		"指令语法高亮.mtsx",
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		writeJSON(w, map[string]any{
			"ok": false, "error": "找不到语法文件",
			"colors": map[string]string{
				"0": "#000000","1":"#0000AA","2":"#00AA00","3":"#00AAAA",
				"4":"#AA0000","5":"#AA00AA","6":"#FFAA00","7":"#AAAAAA",
				"8":"#555555","9":"#5555FF","a":"#55FF55","b":"#55FFFF",
				"c":"#FF5555","d":"#FF55FF","e":"#FFFF55","f":"#FFFFFF",
			},
		})
		return
	}

	content := string(data)
	colors := make(map[string]string)

	// 从 styles 数组中提取颜色定义
	re := regexp.MustCompile(`"§([0-9a-gklmnopqrstu])"[,\s]+#([0-9A-Fa-f]{6})`)
	matches := re.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		colors[m[1]] = "#" + m[2]
	}

	// 提取命名样式颜色
	styleRe := regexp.MustCompile(`"(\w+)"[,\s]+#([0-9A-Fa-f]{6})`)
	styleMatches := styleRe.FindAllStringSubmatch(content, -1)
	styles := make(map[string]string)
	for _, m := range styleMatches {
		name := m[1]
		if len(name) == 1 && strings.ContainsAny(name, "0123456789abcdefg") {
			continue
		}
		styles[name] = "#" + m[2]
	}

	writeJSON(w, map[string]any{
		"ok":      true,
		"mtsx":    content,
		"colors":  colors,
		"styles":  styles,
	})
}