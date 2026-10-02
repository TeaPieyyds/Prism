package pluginscan

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
)

// CapabilityKeys maps a capability key to its Chinese display name.
var CapabilityKeys = map[string]string{
	"system_call":  "调用手机系统",
	"notify":       "发送通知",
	"message_rw":   "监听/发送文件消息",
	"game_command": "执行游戏指令",
	"file_rw":      "读写本地文件",
	"network":      "联网请求",
}

var (
	luaBlockCommentRe = regexp.MustCompile(`(?s)--\[\[.*?\]\]`)
	luaLineCommentRe  = regexp.MustCompile(`--[^\n]*`)
	luaStringRe       = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`)
	luaDotCallRe      = regexp.MustCompile(`\b([a-zA-Z_]\w*)\.([a-zA-Z_]\w*)`)
)

// stripLuaNoise 去掉 Lua 的块注释、行注释与字符串字面量，避免注释/字符串里的字干扰能力判定。
func stripLuaNoise(src string) string {
	s := luaBlockCommentRe.ReplaceAllString(src, " ")
	s = luaLineCommentRe.ReplaceAllString(s, " ")
	s = luaStringRe.ReplaceAllString(s, " ")
	return s
}

// luaCallCaps 把一个 命名空间.方法 的点调用映射到能力 key（可映射多个）。
func luaCallCaps(ns, method string) []string {
	var out []string
	switch ns {
	case "system":
		out = append(out, "system_call") // 任何 system.* 都算调用手机系统
		switch method {
		case "notify", "notification", "toast", "alert":
			out = append(out, "notify")
		case "command", "cmd", "runcommand", "execute":
			out = append(out, "game_command")
		}
	case "game":
		if method == "sendlines" || method == "sendtargeted" || method == "sendmsg" || method == "say" {
			out = append(out, "message_rw")
		}
		out = append(out, "game_command")
	case "bot":
		if method == "command" || method == "cmd" {
			out = append(out, "game_command")
		}
		out = append(out, "message_rw")
	case "events", "media":
		out = append(out, "message_rw")
	case "world", "player", "building":
		out = append(out, "game_command")
	case "http", "web", "socket", "net":
		out = append(out, "network")
	case "io", "os", "fs":
		out = append(out, "file_rw")
	case "util":
		if method == "readfile" || method == "writefile" || method == "read" || method == "write" {
			out = append(out, "file_rw")
		}
	case "plugin":
		if method == "file" || method == "readfile" || method == "writefile" {
			out = append(out, "file_rw")
		}
		out = append(out, "message_rw")
	case "notify":
		out = append(out, "notify")
	}
	return out
}

// ScanCapabilities 扫描 Lua 源码：去注释/去字符串 → 提取真实 命名空间.方法 点调用 → 映射能力。
func ScanCapabilities(luaSrc string) []string {
	src := stripLuaNoise(luaSrc)
	set := make(map[string]bool)
	for _, m := range luaDotCallRe.FindAllStringSubmatch(src, -1) {
		ns, method := strings.ToLower(m[1]), strings.ToLower(m[2])
		for _, c := range luaCallCaps(ns, method) {
			set[c] = true
		}
	}
	// 兜底：非点调用的联网（如 fetch(）
	if strings.Contains(strings.ToLower(src), "fetch(") {
		set["network"] = true
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

var (
	blockCommentRe = regexp.MustCompile(`(?s)--\[\[.*?\]\]`)
	lineCommentRe  = regexp.MustCompile(`--[^\r\n]*`)
	spaceRe        = regexp.MustCompile(`\s+`)
)

// NormHash normalizes Lua source by stripping comments and folding all
// whitespace to a single space, then returns its sha256 hex digest.
func NormHash(luaSrc string) string {
	s := blockCommentRe.ReplaceAllString(luaSrc, "")
	s = lineCommentRe.ReplaceAllString(s, "")
	s = spaceRe.ReplaceAllString(s, "")
	return sha256Hex([]byte(s))
}

// ZipHash returns the sha256 hex digest of the entire plugin zip bytes.
func ZipHash(zipBytes []byte) string {
	return sha256Hex(zipBytes)
}

// CrossCheck compares declared capabilities against scanned ones.
// missing = capabilities found by scan but not declared;
// undeclared = capabilities declared but not found by scan.
func CrossCheck(declared, scanned []string) (missing, undeclared []string) {
	declaredSet := make(map[string]bool, len(declared))
	for _, d := range declared {
		declaredSet[d] = true
	}
	scannedSet := make(map[string]bool, len(scanned))
	for _, s := range scanned {
		scannedSet[s] = true
	}
	for _, s := range scanned {
		if !declaredSet[s] {
			missing = append(missing, s)
		}
	}
	for _, d := range declared {
		if !scannedSet[d] {
			undeclared = append(undeclared, d)
		}
	}
	sort.Strings(missing)
	sort.Strings(undeclared)
	return missing, undeclared
}

// RiskLevel grades the plugin risk: 2 = high (scanned capability not declared),
// 1 = medium (3+ declared capabilities), 0 = low.
func RiskLevel(declared, scanned []string) int {
	missing, _ := CrossCheck(declared, scanned)
	if len(missing) > 0 {
		return 2
	}
	if len(declared) >= 3 {
		return 1
	}
	return 0
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
