package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 点歌台音乐数据目录（与 ToolDelta 点歌台插件共享同一份数据）
const (
	// 部署时按需修改为你的点歌台插件数据目录
	musicListDir      = "./data/music/音乐列表"
	musicCountFile    = "./data/music/play_count.json"
	musicCooldownFile = "./data/music/heat_cooldown.json"
)

// 每首歌热度冷却窗口：10 分钟内无论播放/下载多少次，热度最多 +1
const musicHeatCooldown = 10 * time.Minute

// 音乐文件扩展名（对外统一以 .mmu 传输/命名）
var musicFileExts = map[string]bool{".midseq": true, ".mmu": true}

func readMusicPlayCounts() map[string]int {
	counts := map[string]int{}
	data, err := os.ReadFile(musicCountFile)
	if err != nil {
		return counts
	}
	_ = json.Unmarshal(data, &counts)
	return counts
}

func writeMusicPlayCounts(counts map[string]int) error {
	data, err := json.MarshalIndent(counts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(musicCountFile, data, 0o644)
}

// 每首歌上次加热时刻（unix 秒）
func readMusicCooldowns() map[string]int64 {
	cd := map[string]int64{}
	data, err := os.ReadFile(musicCooldownFile)
	if err != nil {
		return cd
	}
	_ = json.Unmarshal(data, &cd)
	return cd
}

func writeMusicCooldowns(cd map[string]int64) error {
	data, err := json.MarshalIndent(cd, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(musicCooldownFile, data, 0o644)
}

// 扫描音乐目录，返回 {歌名: 热度}
func scanMusicSongs() map[string]int {
	counts := readMusicPlayCounts()
	songs := map[string]int{}
	entries, err := os.ReadDir(musicListDir)
	if err != nil {
		return songs
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !musicFileExts[ext] {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if name == "" {
			continue
		}
		songs[name] = counts[name]
	}
	return songs
}

// GET /api/music/hot —— 按热度降序返回热门歌曲列表
func handleMusicHot(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
		return
	}
	songs := scanMusicSongs()
	type song struct {
		Name string `json:"name"`
		Heat int    `json:"heat"`
	}
	list := make([]song, 0, len(songs))
	for name, heat := range songs {
		list = append(list, song{Name: name, Heat: heat})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Heat != list[j].Heat {
			return list[i].Heat > list[j].Heat
		}
		return list[i].Name < list[j].Name
	})
	jsonResp(w, M{"ok": true, "list": list})
}

// GET /api/music/download?name=X —— 下载歌曲（热度+1），以 <歌名>.mmu 返回
func handleMusicDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		jsonResp(w, M{"ok": false, "error": "参数错误~"})
		return
	}

	// 定位音乐文件（.midseq 优先，其次 .mmu）
	var abs string
	for _, ext := range []string{".midseq", ".mmu"} {
		p := filepath.Join(musicListDir, name+ext)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			abs = p
			break
		}
	}
	if abs == "" {
		jsonResp(w, M{"ok": false, "error": "歌曲不存在~"})
		return
	}

	// 每首歌 10 分钟冷却：距上次加热 ≥10 分钟（且本次有下载/播放）才热度+1；否则不变
	now := time.Now().Unix()
	counts := readMusicPlayCounts()
	cd := readMusicCooldowns()
	if now-cd[name] >= int64(musicHeatCooldown/time.Second) {
		counts[name] = counts[name] + 1
		cd[name] = now
		if err := writeMusicPlayCounts(counts); err != nil {
			log.Printf("[MUSIC] 热度写入失败 name=%s err=%v", name, err)
		} else if err := writeMusicCooldowns(cd); err != nil {
			log.Printf("[MUSIC] 冷却写入失败 name=%s err=%v", name, err)
		}
	}

	filename := sanitizeFilename(name) + ".mmu"
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	http.ServeFile(w, r, abs)
}
