package main

import (
	"fmt"
	"math"
	"sync"
	"time"

	"bot-apk/media"
)

// MusicNote represents a single note event for music playback
type MusicNote struct {
	Instrument string  `json:"instrument"`
	Pitch      float64 `json:"pitch"`
	Volume     float64 `json:"volume"`
	DelayMs    int     `json:"delay_ms"`
}

// LoopMode 循环模式
type LoopMode string

const (
	LoopModeNone   LoopMode = "none"   // 单次播放，播完结束
	LoopModeSingle LoopMode = "single" // 单曲循环
	LoopModeList   LoopMode = "list"   // 列表循环
	LoopModeShuffle LoopMode = "shuffle" // 列表随机
)

// QueueItem 播放队列中的单曲
type QueueItem struct {
	CacheKey string `json:"cache_key"`
	Title    string `json:"title"`
	Duration string `json:"duration"`
	Notes    int    `json:"notes"`
}

// PlaylistManager 播放列表管理器
type PlaylistManager struct {
	mu       sync.Mutex
	queue    []QueueItem
	current  int       // -1 = none
	loopMode LoopMode
	target   string
	speed    float64
	active   bool
	shuffleOrder []int // 随机模式下的播放顺序
	shufflePos    int  // 随机顺序中的当前位置
}

// MusicPlayer handles MIDI music playback through the bot connection
type MusicPlayer struct {
	mu      sync.Mutex
	playing bool
	paused  bool

	notes    []MusicNote
	current  int
	target   string
	speed    float64
	started  time.Time
	pausedAt time.Duration
	title    string    // 当前曲目名
	totalMs  int       // 总时长(ms)
	elapsedMs int      // 已播时长(ms)

	stopCh   chan struct{}
	pauseCh  chan struct{}
	resumeCh chan struct{}
	doneCh   chan struct{}

	onProgress func(current, total int)
	onDone     func()
}

var (
	musicPlayer   *MusicPlayer
	musicPlayerMu sync.Mutex
)

var (
	playlistManager   *PlaylistManager
	playlistManagerMu sync.Mutex
)

func NewMusicPlayer(notes []MusicNote, target string, speed float64, onProgress func(int, int), title string, totalMs int) *MusicPlayer {
	return &MusicPlayer{
		notes:      notes,
		target:     target,
		speed:      speed,
		title:      title,
		totalMs:    totalMs,
		stopCh:     make(chan struct{}, 1),
		pauseCh:    make(chan struct{}, 1),
		resumeCh:   make(chan struct{}, 1),
		doneCh:     make(chan struct{}),
		onProgress: onProgress,
	}
}

func (mp *MusicPlayer) Play() {
	mp.mu.Lock()
	if mp.playing && !mp.paused {
		mp.mu.Unlock()
		return
	}
	if mp.paused {
		mp.paused = false
		mp.mu.Unlock()
		select {
		case mp.resumeCh <- struct{}{}:
		default:
		}
		return
	}
	mp.playing = true
	mp.mu.Unlock()

	go mp.run()
}

func (mp *MusicPlayer) Pause() {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	if !mp.playing || mp.paused {
		return
	}
	mp.paused = true
	select {
	case mp.pauseCh <- struct{}{}:
	default:
	}
}

func (mp *MusicPlayer) Stop() {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	if !mp.playing {
		return
	}
	mp.playing = false
	mp.paused = false
	select {
	case mp.stopCh <- struct{}{}:
	default:
	}
}

func (mp *MusicPlayer) IsPlaying() bool {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	return mp.playing && !mp.paused
}

func (mp *MusicPlayer) IsPaused() bool {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	return mp.playing && mp.paused
}

func (mp *MusicPlayer) Progress() (current, total int) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	return mp.current, len(mp.notes)
}

func (mp *MusicPlayer) Wait() {
	<-mp.doneCh
}

func (mp *MusicPlayer) run() {
	defer func() {
		mp.mu.Lock()
		mp.playing = false
		mp.paused = false
		mp.mu.Unlock()
		close(mp.doneCh)
	}()

	lastBarRefresh := time.Now()

	for mp.current < len(mp.notes) {
		select {
		case <-mp.stopCh:
			return
		case <-mp.pauseCh:
			select {
			case <-mp.resumeCh:
			case <-mp.stopCh:
				return
			}
		default:
		}

		// 断连检测：机器人断连则等待恢复，超30秒自动停止
		if !isBotConnected() {
			select {
			case <-mp.resumeCh:
			case <-mp.stopCh:
				return
			case <-time.After(30 * time.Second):
				return
			}
		}

		note := mp.notes[mp.current]

		// 发送 playsound 命令
		cmd := fmt.Sprintf("/execute as @a at @s run playsound %s @s ~ ~ ~ %.2f %.3f %.2f",
			note.Instrument, note.Volume, note.Pitch, note.Volume)

		botMgrMu.Lock()
		if botMgr != nil && botMgr.IsConnected() {
			botMgr.SendWOCmd(cmd)
		}
		botMgrMu.Unlock()

		mp.current++
		mp.elapsedMs += note.DelayMs
		if mp.onProgress != nil {
			mp.onProgress(mp.current, len(mp.notes))
		}
		// 每 500ms 刷新一次行动栏进度（统一平行四边形样式，不绑定音符数）
		if time.Since(lastBarRefresh) > 500*time.Millisecond {
			lastBarRefresh = time.Now()
			pct := mp.current * 100 / len(mp.notes)
			const barW = 28
			bar := buildParBar(pct*barW/100, barW, "a", "7")
			elapsedSec := int(float64(mp.elapsedMs) / mp.speed / 1000)
			totalSec := int(float64(mp.totalMs) / mp.speed / 1000)
			timeStr := fmt.Sprintf("%d:%02d/%d:%02d", elapsedSec/60, elapsedSec%60, totalSec/60, totalSec%60)
			title := mp.title
			if len([]rune(title)) > 10 {
				title = string([]rune(title)[:10]) + ".."
			}
			text := fmt.Sprintf("\u00a7d\u266b \u00a7f%s\n\u00a7r%s\n\u00a77%d%%  \u00a78|  \u00a7f%s", title, bar, pct, timeStr)
			botMgrMu.Lock()
			if botMgr != nil && botMgr.IsConnected() {
				botMgr.SendWOCmd(fmt.Sprintf(`titleraw @a actionbar {"rawtext":[{"text":"%s"}]}`, text))
			}
			botMgrMu.Unlock()
		}

		// calculate delay from this note to the next
		delay := float64(note.DelayMs) / mp.speed

		// Concurrent notes (same MIDI tick): skip artificial gap,
		// send the next note immediately for chord playback.
		if delay <= 50 {
			// Still check stop/pause but don't sleep
			select {
			case <-mp.stopCh:
				return
			case <-mp.pauseCh:
				select {
				case <-mp.resumeCh:
				case <-mp.stopCh:
					return
				}
			default:
			}
			continue
		}

		if delay > 500 {
			delay = 500 // cap at 500ms for responsiveness
		}

		select {
		case <-mp.stopCh:
			return
		case <-mp.pauseCh:
			select {
			case <-mp.resumeCh:
			case <-mp.stopCh:
				return
			}
		case <-time.After(time.Duration(delay) * time.Millisecond):
		}
	}
}

// startMusicPlayback starts a new music player with the given notes.
// It stops any currently playing music first.
// onDone is called outside musicPlayerMu after playback finishes.
func startMusicPlayback(notes []MusicNote, target string, speed float64, title string, totalMs int, onDone func()) {
	musicPlayerMu.Lock()
	var oldPlayer *MusicPlayer
	if musicPlayer != nil {
		oldPlayer = musicPlayer
		oldPlayer.Stop()
	}
	musicPlayer = NewMusicPlayer(notes, target, speed, func(current, total int) {
		hub.Emit("music_progress", mustJSON(map[string]any{
			"current": current,
			"total":   total,
		}))
	}, title, totalMs)
	musicPlayer.onDone = onDone
	musicPlayerMu.Unlock()

	// 等待旧播放器完全退出后再启动新播放器，防止旧播放器继续发送 playsound / 进度条
	if oldPlayer != nil {
		oldPlayer.Wait()
	}

	hub.Emit("music_start", mustJSON(map[string]any{
		"total_notes":         len(notes),
		"estimated_duration":  fmt.Sprintf("%d:%02d", totalMs/60000, (totalMs%60000)/1000),
		"title":               title,
	}))

	musicPlayer.Play()

	// Watch for completion — capture local ref so we don't accidentally
	// kill a subsequently started player.
	mp := musicPlayer
	go func() {
		mp.Wait()
		fn := mp.onDone
		musicPlayerMu.Lock()
		if musicPlayer == mp {
			musicPlayer = nil
		}
		musicPlayerMu.Unlock()
		if fn != nil {
			fn()
		} else {
			hub.Emit("music_done", mustJSON(map[string]any{}))
		}
	}()
}

// stopMusicPlayback stops any currently playing music and waits for it to finish
func stopMusicPlayback() {
	musicPlayerMu.Lock()
	oldPlayer := musicPlayer
	if oldPlayer != nil {
		oldPlayer.Stop()
		musicPlayer = nil
	}
	musicPlayerMu.Unlock()
	if oldPlayer != nil {
		oldPlayer.Wait() // 等待旧播放器 goroutine 完全退出
	}
}

// ========== PlaylistManager ==========

func NewPlaylistManager(target string, speed float64) *PlaylistManager {
	return &PlaylistManager{
		current: -1,
		loopMode: LoopModeNone,
		target:   target,
		speed:    speed,
	}
}

// AddToQueue 添加曲目到队列末尾
func (pm *PlaylistManager) AddToQueue(item QueueItem) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.queue = append(pm.queue, item)
	if pm.active && pm.current < 0 {
		pm.current = 0
	}
	pm.emitQueueUpdate()
}

// RemoveFromQueue 从队列移除指定索引的曲目
func (pm *PlaylistManager) RemoveFromQueue(index int) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	defer pm.emitQueueUpdate()
	if index < 0 || index >= len(pm.queue) {
		return fmt.Errorf("索引越界")
	}
	if pm.active && pm.current >= index {
		pm.current--
		if pm.current < -1 {
			pm.current = -1
		}
	}
	pm.queue = append(pm.queue[:index], pm.queue[index+1:]...)
	if len(pm.queue) == 0 {
		pm.current = -1
		pm.active = false
	}
	return nil
}

// ClearQueue 清空队列并停止播放
func (pm *PlaylistManager) ClearQueue() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.active {
		stopMusicPlayback()
	}
	pm.queue = nil
	pm.current = -1
	pm.active = false
	pm.shuffleOrder = nil
	pm.shufflePos = 0
	pm.emitQueueUpdate()
}

// GetQueue 返回队列快照
func (pm *PlaylistManager) GetQueue() ([]QueueItem, int, LoopMode, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	q := make([]QueueItem, len(pm.queue))
	copy(q, pm.queue)
	return q, pm.current, pm.loopMode, pm.active
}

// SetLoopMode 设置循环模式
func (pm *PlaylistManager) SetLoopMode(mode LoopMode) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.loopMode = mode
	if mode == LoopModeShuffle {
		pm.genShuffleOrder()
	}
	pm.emitQueueUpdate()
}

// GetLoopMode 获取循环模式
func (pm *PlaylistManager) GetLoopMode() LoopMode {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.loopMode
}

// genShuffleOrder 生成随机播放顺序
func (pm *PlaylistManager) genShuffleOrder() {
	n := len(pm.queue)
	pm.shuffleOrder = make([]int, n)
	for i := 0; i < n; i++ {
		pm.shuffleOrder[i] = i
	}
	// Fisher-Yates shuffle
	for i := n - 1; i > 0; i-- {
		j := int(time.Now().UnixNano()) % (i + 1)
		if j < 0 {
			j = -j
		}
		pm.shuffleOrder[i], pm.shuffleOrder[j] = pm.shuffleOrder[j], pm.shuffleOrder[i]
	}
	pm.shufflePos = 0
}

// shuffleNext 获取随机模式下的下一曲索引
func (pm *PlaylistManager) shuffleNext() int {
	if len(pm.shuffleOrder) == 0 {
		pm.genShuffleOrder()
	}
	idx := pm.shuffleOrder[pm.shufflePos%len(pm.shuffleOrder)]
	pm.shufflePos++
	if pm.shufflePos >= len(pm.shuffleOrder) && pm.loopMode == LoopModeList || pm.loopMode == LoopModeShuffle {
		pm.genShuffleOrder()
	}
	return idx
}

// Play 从指定索引开始播放队列
func (pm *PlaylistManager) Play(index int) error {
	pm.mu.Lock()
	if len(pm.queue) == 0 {
		pm.mu.Unlock()
		return fmt.Errorf("队列为空")
	}
	if index < 0 || index >= len(pm.queue) {
		index = 0
	}
	pm.current = index
	pm.active = true
	if pm.loopMode == LoopModeShuffle {
		pm.genShuffleOrder()
		// 找到当前曲目在随机顺序中的位置
		for i, v := range pm.shuffleOrder {
			if v == index {
				pm.shufflePos = i
				break
			}
		}
	}
	target := pm.target
	pm.mu.Unlock()

	pm.playCurrent(target, pm.speed)
	return nil
}

// playCurrent 播放当前曲目
func (pm *PlaylistManager) playCurrent(target string, speed float64) {
	pm.mu.Lock()
	if !pm.active || pm.current < 0 || pm.current >= len(pm.queue) {
		pm.mu.Unlock()
		return
	}
	item := pm.queue[pm.current]
	pm.mu.Unlock()

	// 从缓存获取 MIDI 数据
	midiCacheMu.Lock()
	midi, ok := midiCache[item.CacheKey]
	midiCacheMu.Unlock()
	if !ok {
		pm.mu.Lock()
		pm.onTrackDoneLocked()
		pm.mu.Unlock()
		return
	}

	// 转换音符
	notes := convertMIDIToMusicNotes(midi)

	// 计算总时长
	totalMs := 0
	for _, n := range midi.Notes {
		d := int(n.Tick) * 50
		if d < 1 {
			d = 1
		}
		totalMs += d
	}

	// 发射 track_change SSE
	hub.Emit("track_change", mustJSON(map[string]any{
		"cache_key": item.CacheKey,
		"title":     item.Title,
		"index":     pm.current,
		"total":     len(pm.queue),
	}))

	// 更新队列状态
	pm.mu.Lock()
	pm.emitQueueUpdateLocked()
	pm.mu.Unlock()

	// 开始播放
	startMusicPlayback(notes, target, speed, item.Title, totalMs, func() {
		pm.onTrackDone()
	})
}

// onTrackDone 曲目播放完成回调
func (pm *PlaylistManager) onTrackDone() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	defer pm.emitQueueUpdate()
	pm.onTrackDoneLocked()
}

// onTrackDoneLocked 持有 pm.mu 时调用
func (pm *PlaylistManager) onTrackDoneLocked() {
	if !pm.active || len(pm.queue) == 0 {
		pm.active = false
		pm.current = -1
		hub.Emit("music_done", mustJSON(map[string]any{"reason": "stopped"}))
		return
	}

	switch pm.loopMode {
	case LoopModeNone:
		// 单次：播完结束
		if pm.current < len(pm.queue)-1 {
			pm.current++
			pm.mu.Unlock()
			pm.playCurrent(pm.target, pm.speed)
			pm.mu.Lock()
		} else {
			pm.active = false
			pm.current = -1
			hub.Emit("music_done", mustJSON(map[string]any{"reason": "finished"}))
		}

	case LoopModeSingle:
		// 单曲循环：重新播放同一首
		pm.mu.Unlock()
		pm.playCurrent(pm.target, pm.speed)
		pm.mu.Lock()

	case LoopModeList:
		// 列表循环：下一首，到尾回到开头
		pm.current++
		if pm.current >= len(pm.queue) {
			pm.current = 0
		}
		pm.mu.Unlock()
		pm.playCurrent(pm.target, pm.speed)
		pm.mu.Lock()

	case LoopModeShuffle:
		// 随机模式：按随机顺序取下一曲
		nextIdx := pm.shuffleNext()
		if nextIdx < 0 || nextIdx >= len(pm.queue) {
			pm.active = false
			pm.current = -1
			hub.Emit("music_done", mustJSON(map[string]any{"reason": "finished"}))
			return
		}
		pm.current = nextIdx
		pm.mu.Unlock()
		pm.playCurrent(pm.target, pm.speed)
		pm.mu.Lock()
	}
}

// Next 下一首
func (pm *PlaylistManager) Next() {
	pm.mu.Lock()
	if !pm.active || len(pm.queue) == 0 {
		pm.mu.Unlock()
		return
	}
	stopMusicPlayback()

	switch pm.loopMode {
	case LoopModeShuffle:
		nextIdx := pm.shuffleNext()
		if nextIdx < 0 || nextIdx >= len(pm.queue) {
			pm.active = false
			pm.current = -1
			pm.mu.Unlock()
			hub.Emit("music_done", mustJSON(map[string]any{}))
			pm.emitQueueUpdate()
			return
		}
		pm.current = nextIdx
	default:
		pm.current++
		if pm.current >= len(pm.queue) {
			if pm.loopMode == LoopModeList {
				pm.current = 0
			} else {
				pm.active = false
				pm.current = -1
				pm.mu.Unlock()
				hub.Emit("music_done", mustJSON(map[string]any{}))
				pm.emitQueueUpdate()
				return
			}
		}
	}
	target := pm.target
	pm.mu.Unlock()
	pm.playCurrent(target, pm.speed)
	pm.emitQueueUpdate()
}

// Prev 上一首
func (pm *PlaylistManager) Prev() {
	pm.mu.Lock()
	if !pm.active || len(pm.queue) == 0 {
		pm.mu.Unlock()
		return
	}
	stopMusicPlayback()

	switch pm.loopMode {
	case LoopModeShuffle:
		pm.shufflePos -= 2
		if pm.shufflePos < 0 {
			pm.shufflePos = 0
		}
		nextIdx := pm.shuffleOrder[pm.shufflePos%len(pm.shuffleOrder)]
		pm.current = nextIdx
		pm.shufflePos++
	default:
		pm.current--
		if pm.current < 0 {
			if pm.loopMode == LoopModeList {
				pm.current = len(pm.queue) - 1
			} else {
				pm.current = 0
			}
		}
	}
	target := pm.target
	pm.mu.Unlock()
	pm.playCurrent(target, pm.speed)
	pm.emitQueueUpdate()
}

// Stop 停止队列播放
func (pm *PlaylistManager) Stop() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.active {
		stopMusicPlayback()
		pm.active = false
		pm.current = -1
		pm.emitQueueUpdate()
	}
}

// IsActive 队列是否活跃
func (pm *PlaylistManager) IsActive() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.active
}

func (pm *PlaylistManager) emitQueueUpdate() {
	pm.emitQueueUpdateLocked()
}

func (pm *PlaylistManager) emitQueueUpdateLocked() {
	items := make([]QueueItem, len(pm.queue))
	copy(items, pm.queue)
	hub.Emit("queue_update", mustJSON(map[string]any{
		"queue":     items,
		"current":   pm.current,
		"loop_mode": string(pm.loopMode),
		"active":    pm.active,
	}))
}

// convertMIDIToMusicNotes 将 MIDI 音符转为游戏播放音符
func convertMIDIToMusicNotes(midi *media.MIDIFile) []MusicNote {
	notes := make([]MusicNote, len(midi.Notes))
	for i, n := range midi.Notes {
		pitch := 1.0
		if n.Sound != "note.snare" && n.Sound != "note.bd" && n.Sound != "note.hat" {
			dev := media.MCInstrumentDeviation[n.Sound]
			pr := int(n.Note) - 60 - dev
			pitch = math.Pow(2, float64(pr)/12.0)
		}
		notes[i] = MusicNote{
			Instrument: n.Sound,
			Pitch:      math.Round(pitch*1000) / 1000,
			Volume:     float64(n.Velocity) / 100.0,
			DelayMs:    int(n.Tick) * 50,
		}
	}
	return notes
}

// isBotConnected 检查机器人是否已连接
func isBotConnected() bool {
	botMgrMu.Lock()
	defer botMgrMu.Unlock()
	return botMgr != nil && botMgr.IsConnected()
}
