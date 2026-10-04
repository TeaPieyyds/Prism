package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ========== 7x7 ASCII 大字字符映射表 ==========

var zd = map[string][]string{
	"A": {"  ███  ", " █   █ ", "█     █", "███████", "█     █", "█     █", "█     █"},
	"B": {"██████ ", "█     █", "█     █", "██████ ", "█     █", "█     █", "██████ "},
	"C": {" █████ ", "█     █", "█      ", "█      ", "█      ", "█     █", " █████ "},
	"D": {"██████ ", "█     █", "█     █", "█     █", "█     █", "█     █", "██████ "},
	"E": {"███████", "█      ", "█      ", "██████ ", "█      ", "█      ", "███████"},
	"F": {"███████", "█      ", "█      ", "██████ ", "█      ", "█      ", "█      "},
	"G": {" █████ ", "█     █", "█      ", "█  ████", "█     █", "█     █", " █████ "},
	"H": {"█     █", "█     █", "█     █", "███████", "█     █", "█     █", "█     █"},
	"I": {"███████", "   █   ", "   █   ", "   █   ", "   █   ", "   █   ", "███████"},
	"J": {"      █", "      █", "      █", "      █", "█     █", "█     █", " █████ "},
	"K": {"█    █ ", "█   █  ", "█  █   ", "███    ", "█  █   ", "█   █  ", "█    █ "},
	"L": {"█      ", "█      ", "█      ", "█      ", "█      ", "█      ", "███████"},
	"M": {"█     █", "██   ██", "█ █ █ █", "█  █  █", "█     █", "█     █", "█     █"},
	"N": {"█     █", "██    █", "█ █   █", "█  █  █", "█   █ █", "█    ██", "█     █"},
	"O": {" █████ ", "█     █", "█     █", "█     █", "█     █", "█     █", " █████ "},
	"P": {"██████ ", "█     █", "█     █", "██████ ", "█      ", "█      ", "█      "},
	"Q": {" █████ ", "█     █", "█     █", "█     █", "█   █ █", "█    ██", " ██████"},
	"R": {"██████ ", "█     █", "█     █", "██████ ", "█   █  ", "█    █ ", "█     █"},
	"S": {" █████ ", "█     █", " █     ", "  ███  ", "     █ ", "█     █", " █████ "},
	"T": {"███████", "   █   ", "   █   ", "   █   ", "   █   ", "   █   ", "   █   "},
	"U": {"█     █", "█     █", "█     █", "█     █", "█     █", "█     █", " █████ "},
	"V": {"█     █", "█     █", "█     █", "█     █", " █   █ ", "  █ █  ", "   █   "},
	"W": {"█     █", "█     █", "█  █  █", "█  █  █", "█ █ █ █", "██   ██", "█     █"},
	"X": {"█     █", " █   █ ", "  █ █  ", "   █   ", "  █ █  ", " █   █ ", "█     █"},
	"Y": {"█     █", " █   █ ", "  █ █  ", "   █   ", "   █   ", "   █   ", "   █   "},
	"Z": {"███████", "      █", "     █ ", "    █  ", "   █   ", "  █    ", "███████"},
	"0": {" █████ ", "█   █ █", "█  █  █", "█ █   █", "██    █", "█     █", " █████ "},
	"1": {"   █   ", "  ██   ", " █ █   ", "   █   ", "   █   ", "   █   ", "███████"},
	"2": {" █████ ", "█     █", "      █", " █████ ", "█      ", "█      ", "███████"},
	"3": {" █████ ", "█     █", "      █", " █████ ", "      █", "█     █", " █████ "},
	"4": {"█      ", "█     █", "█     █", "███████", "      █", "      █", "      █"},
	"5": {"███████", "█      ", "██████ ", "      █", "      █", "█     █", " █████ "},
	"6": {" █████ ", "█     █", "█      ", "██████ ", "█     █", "█     █", " █████ "},
	"7": {"███████", "      █", "     █ ", "    █  ", "   █   ", "  █    ", "  █    "},
	"8": {" █████ ", "█     █", "█     █", " █████ ", "█     █", "█     █", " █████ "},
	"9": {" █████ ", "█     █", "█     █", " ██████", "      █", "█     █", " █████ "},
	":": {"       ", "   █   ", "   █   ", "       ", "   █   ", "   █   ", "       "},
	".": {"       ", "       ", "       ", "       ", "       ", "   █   ", "   █   "},
	"!": {"   █   ", "   █   ", "   █   ", "   █   ", "   █   ", "       ", "   █   "},
	"?": {" █████ ", "█     █", "      █", "    █  ", "   █   ", "       ", "   █   "},
	" ": {"       ", "       ", "       ", "       ", "       ", "       ", "       "},
}

// ========== 配置 ==========

type MarqueeConfig struct {
	Text      string `json:"text"`
	Speed     int    `json:"speed"`
	Width     int    `json:"width"`
	Color     string `json:"color"`
	TextChar  string `json:"text_char"`
	FillChar  string `json:"fill_char"`
	Direction string `json:"direction"`
	Rainbow   bool   `json:"rainbow"`
}

func (c *MarqueeConfig) fillDefaults() {
	if c.Speed <= 0 {
		c.Speed = 200
	}
	if c.Width <= 0 {
		c.Width = 40
	}
	if c.Color == "" {
		c.Color = "a"
	}
	if c.TextChar == "" {
		c.TextChar = "█"
	}
	if c.Direction == "" {
		c.Direction = "left"
	}
}

const mcColorPrefix = "§"

var rainbowColors = []string{"c", "6", "e", "a", "b", "d", "5"}

// ========== 状态 ==========

var (
	marqueeMu      sync.Mutex
	marqueeRunning bool
	marqueeStopCh  chan struct{}
	marqueeCfg     MarqueeConfig
)

// ========== ASCII 大字构建 ==========

// buildMarqueeRows 构建 7 行 ASCII 大字画布。
// 每行中的 █ 替换为 textChar，空格替换为 fillChar（支持任意长度）。
func buildMarqueeRows(text string, textChar, fillChar string) ([]string, int) {
	upper := strings.ToUpper(text)
	rows := make([]string, 7)
	for _, ch := range upper {
		art, ok := zd[string(ch)]
		if !ok {
			art = zd[" "]
		}
		for i := 0; i < 7; i++ {
			row := art[i]
			// 先替换 █ 为 textChar，再替换空格为 fillChar
			row = strings.ReplaceAll(row, "█", textChar)
			row = strings.ReplaceAll(row, " ", fillChar)
			if rows[i] != "" {
				rows[i] += fillChar + fillChar
			}
			rows[i] += row
		}
	}
	if len(rows) == 0 || len([]rune(rows[0])) == 0 {
		return rows, 0
	}
	// 确保所有行等长
	w := len([]rune(rows[0]))
	for i := 0; i < 7; i++ {
		if len([]rune(rows[i])) != w {
			runes := []rune(rows[i])
			for len(runes) < w {
				runes = append(runes, ' ')
			}
			rows[i] = string(runes)
		}
	}
	return rows, w
}

func scrollWindow(row string, pos int, width int) string {
	runes := []rune(row)
	n := len(runes)
	if n <= 0 {
		return strings.Repeat(" ", width)
	}
	if n <= width {
		pad := (width - n) / 2
		if pad < 0 {
			pad = 0
		}
		return strings.Repeat(" ", pad) + row + strings.Repeat(" ", width-pad-n)
	}
	doubled := append(runes, runes...)
	return string(doubled[pos : pos+width])
}

func applyColor(line string, color string) string {
	return mcColorPrefix + color + line
}

func applyRainbow(line string, offset int) string {
	var b strings.Builder
	for i, ch := range line {
		ci := (i + offset) % len(rainbowColors)
		b.WriteString(mcColorPrefix + rainbowColors[ci] + string(ch))
	}
	return b.String()
}

// ========== 滚动循环 ==========

func marqueeLoop() {
	marqueeMu.Lock()
	cfg := marqueeCfg
	stopCh := marqueeStopCh
	marqueeMu.Unlock()

	rows, canvasWidth := buildMarqueeRows(cfg.Text, cfg.TextChar, cfg.FillChar)
	if canvasWidth <= 0 {
		return
	}

	width := cfg.Width
	if width < 10 {
		width = 10
	}
	if width > 120 {
		width = 120
	}

	// 跑马灯：填充 + 文字 + 填充
	// 填充字符支持任意长度，重复到 width 列
	fillStr := cfg.FillChar
	if fillStr == "" {
		fillStr = " "
	}
	// 构建填充字符串（重复 fillStr 直到 >= width 列）
	var padBuf strings.Builder
	for padBuf.Len() < width {
		padBuf.WriteString(fillStr)
	}
	pad := padBuf.String()
	if len([]rune(pad)) > width {
		pad = string([]rune(pad)[:width])
	}

	paddedRows := make([]string, 7)
	for i := 0; i < 7; i++ {
		paddedRows[i] = pad + rows[i] + pad
	}

	maxFrame := canvasWidth + width
	frame := 0
	ticker := time.NewTicker(time.Duration(cfg.Speed) * time.Millisecond)
	defer ticker.Stop()

	rainbowOffset := 0

	for {
		select {
		case <-stopCh:
			botMgrMu.Lock()
			if botMgr != nil && botMgr.IsConnected() {
				botMgr.SendWOCmd(`titleraw @a actionbar {"rawtext":[{"text":""}]}`)
			}
			botMgrMu.Unlock()
			return

		case <-ticker.C:
			pos := frame % maxFrame

			var lines []string
			for i := 0; i < 7; i++ {
				line := scrollWindow(paddedRows[i], pos, width)
				if cfg.Rainbow {
					line = applyRainbow(line, rainbowOffset)
				} else {
					line = applyColor(line, cfg.Color)
				}
				lines = append(lines, line)
			}

			marqueeText := strings.Join(lines, "\n")
			cmd := fmt.Sprintf(`titleraw @a actionbar {"rawtext":[{"text":%s}]}`, mustJSON(marqueeText))

			botMgrMu.Lock()
			if botMgr != nil && botMgr.IsConnected() {
				botMgr.SendWOCmd(cmd)
			}
			botMgrMu.Unlock()

			frame++
			rainbowOffset++
		}
	}
}

// ========== API 处理函数 ==========

func handleMarqueeStart(w http.ResponseWriter, r *http.Request) {
	var req MarqueeConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON: " + err.Error()})
		return
	}
	if req.Text == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "文字内容不能为空"})
		return
	}
	req.fillDefaults()

	marqueeMu.Lock()
	if marqueeRunning {
		close(marqueeStopCh)
	}
	marqueeCfg = req
	marqueeStopCh = make(chan struct{})
	marqueeRunning = true
	marqueeMu.Unlock()

	go marqueeLoop()

	writeJSON(w, map[string]any{"ok": true})
}

func handleMarqueeStop(w http.ResponseWriter, r *http.Request) {
	marqueeMu.Lock()
	if marqueeRunning {
		close(marqueeStopCh)
		marqueeRunning = false
	}
	marqueeMu.Unlock()

	writeJSON(w, map[string]any{"ok": true})
}

func handleMarqueeStatus(w http.ResponseWriter, r *http.Request) {
	marqueeMu.Lock()
	running := marqueeRunning
	cfg := marqueeCfg
	marqueeMu.Unlock()

	writeJSON(w, map[string]any{
		"ok":      true,
		"running": running,
		"config":  cfg,
	})
}

func handleMarqueePreview(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	textChar := r.URL.Query().Get("text_char")
	fillChar := r.URL.Query().Get("fill_char")
	if text == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "缺少 text 参数"})
		return
	}
	if textChar == "" {
		textChar = "█"
	}
	if fillChar == "" {
		fillChar = " "
	}
	rows, _ := buildMarqueeRows(text, textChar, fillChar)
	writeJSON(w, map[string]any{"ok": true, "rows": rows})
}