package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-gl/mathgl/mgl32"
)

// ========== 类型定义 ==========

// CameraSample 单个采样点
type CameraSample struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Z         float64 `json:"z"`
	Pitch     float32 `json:"pitch"`
	Yaw       float32 `json:"yaw"`
	Timestamp int64   `json:"timestamp"` // 距录制开始的 ms
}

// CameraShot 单个镜头
type CameraShot struct {
	ID       int             `json:"id"`
	Player   string          `json:"player"`
	Start    CameraSample    `json:"start"`
	End      CameraSample    `json:"end"`
	Samples  []CameraSample  `json:"samples"`
	Duration int64           `json:"duration"`  // ms
	Distance float64         `json:"distance"`  // 总位移
	Pivot    *CameraPivot    `json:"pivot,omitempty"`
	Easing   string          `json:"easing"`
	EditablePivot bool       `json:"editable_pivot"`
	EditableStart bool       `json:"editable_start"`
	EditableEnd   bool       `json:"editable_end"`
	EditableDuration bool   `json:"editable_duration"`
}

// CameraPivot 围绕点
type CameraPivot struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// CameraRecorder 录制器
type CameraRecorder struct {
	mu       sync.Mutex
	player   string
	samples  []CameraSample
	started  bool        // 是否已触发移动（正式录制）
	running  bool
	stopCh   chan struct{}
	startAt time.Time
	startPos mgl32.Vec3
	lastPos  mgl32.Vec3
	idleCount int // 连续静止采样数
	shots    []CameraShot
	shotID   int
}

// 全局录制器
var (
	cameraRecorder   *CameraRecorder
	cameraRecorderMu sync.Mutex
)

// ========== 调试日志 ==========

var debugLogDir = "/storage/emulated/0/Download/"

// ========== 录制控制 ==========

// StartRecording 开始录制指定玩家
func StartRecording(player string) (*CameraRecorder, error) {
	cameraRecorderMu.Lock()
	defer cameraRecorderMu.Unlock()

	// 检查是否已在录制
	if cameraRecorder != nil && cameraRecorder.running {
		return nil, fmt.Errorf("已有录制正在进行")
	}

	// 获取初始位置
	bm := getBotManager()
	if bm == nil {
		return nil, fmt.Errorf("机器人未连接")
	}
	pos, err := bm.GetPlayerPos(player)
	if err != nil {
		return nil, fmt.Errorf("获取玩家位置失败: %v", err)
	}

	cr := &CameraRecorder{
		player:  player,
		samples: make([]CameraSample, 0),
		running: true,
		stopCh:  make(chan struct{}),
		startAt: time.Now(),
	}

	initialPos := mgl32.Vec3{pos.Position.X, pos.Position.Y, pos.Position.Z}
	cr.startPos = initialPos
	cr.lastPos = initialPos

	cameraRecorder = cr

	// 发送提示消息给玩家
	bm.SendWOCmd(fmt.Sprintf(`tellraw "%s" {"rawtext":[{"text":"§a[录制] 准备就绪，移动后开始记录镜头"}]}`, player))

	// 启动采样 goroutine
	go cr.sampleLoop(bm)

	return cr, nil
}

// sampleLoop 每 500ms 用 querytarget 查询一次玩家位置
// 静止超过 3 次采样（1.5s）自动结束当前镜头，等待下次移动
func (cr *CameraRecorder) sampleLoop(bm *BotManager) {
	tk := time.NewTicker(500 * time.Millisecond)
	defer tk.Stop()

	for {
		select {
		case <-cr.stopCh:
			return
		case <-tk.C:
			cr.mu.Lock()

			pos, err := bm.GetPlayerPos(cr.player)
			if err != nil {
				cr.mu.Unlock()
				continue
			}

			now := time.Now()
			ts := now.Sub(cr.startAt).Milliseconds()

			// 从 PlayerInfoManager 获取俯仰角和头部朝向（querytarget 不返回 pitch，仅返回身体朝向）
			pitch := float32(0)
			yaw := pos.YRot
			if pi := playerInfoMgr.GetPlayer(cr.player); pi != nil {
				pitch = pi.Pitch
				// 使用头部朝向而非身体朝向：玩家围绕点旋转时身体朝向移动方向（切线），
				// 头部朝向注视点（圆心），用身体朝向算出的围绕点会成镜像关系
				yaw = pi.HeadYaw
			}
			sample := CameraSample{
				X: float64(pos.Position.X), Y: float64(pos.Position.Y), Z: float64(pos.Position.Z),
				Pitch: pitch, Yaw: yaw,
				Timestamp: ts,
			}

			cr.samples = append(cr.samples, sample)

			dx := math.Abs(sample.X - float64(cr.lastPos[0]))
			dy := math.Abs(sample.Y - float64(cr.lastPos[1]))
			dz := math.Abs(sample.Z - float64(cr.lastPos[2]))
			moved := dx > 0.5 || dy > 0.5 || dz > 0.5

			if moved {
				if !cr.started {
					// 第1次移动 → 开始新镜头
					cr.started = true
					cr.startAt = now
					sample.Timestamp = 0
					cr.samples = []CameraSample{sample}
					bm.SendWOCmd(fmt.Sprintf(`tellraw "%s" {"rawtext":[{"text":"§a[录制] 开始记录镜头！"}]}`, cr.player))
				}
				cr.idleCount = 0
			} else {
				cr.idleCount++
				// 静止超过 3 次采样 → 自动结束当前镜头
				if cr.started && cr.idleCount >= 3 && len(cr.samples) >= 3 {
					cr.finalizeShot(bm, now)
					cr.started = false
					cr.idleCount = 0
					bm.SendWOCmd(fmt.Sprintf(`tellraw "%s" {"rawtext":[{"text":"§a[录制] 镜头 #%d 已自动保存，等待下次移动..."}]}`, cr.player, cr.shotID))
				}
			}

			cr.lastPos = mgl32.Vec3{pos.Position.X, pos.Position.Y, pos.Position.Z}
			cr.mu.Unlock()
		}
	}
}

// finalizeShot 将当前采样数据保存为一个镜头
func (cr *CameraRecorder) finalizeShot(bm *BotManager, endTime time.Time) {
	if len(cr.samples) < 3 {
		return
	}

	// 去掉末尾的静止采样（3次 × 500ms = 1.5s 空闲检测）
	idleSamples := 3
	if len(cr.samples) > idleSamples {
		cr.samples = cr.samples[:len(cr.samples)-idleSamples]
	}

	if len(cr.samples) < 3 {
		return
	}

	start := cr.samples[0]
	end := cr.samples[len(cr.samples)-1]
	duration := end.Timestamp - start.Timestamp
	if duration < 500 {
		return
	}

	var totalDist float64
	for i := 1; i < len(cr.samples); i++ {
		s := cr.samples[i]
		sp := cr.samples[i-1]
		dx := s.X - sp.X
		dy := s.Y - sp.Y
		dz := s.Z - sp.Z
		totalDist += math.Sqrt(dx*dx + dy*dy + dz*dz)
	}

	pivot := calcPivot(cr.samples)
	easing := recommendEasing(cr.samples)

	shot := &CameraShot{
		ID:       cr.shotID + 1,
		Player:   cr.player,
		Start:    start,
		End:      end,
		Samples:  cr.samples,
		Duration: duration,
		Distance: totalDist,
		Pivot:    pivot,
		Easing:   easing,
		EditablePivot:  true,
		EditableStart:  true,
		EditableEnd:    true,
		EditableDuration: true,
	}

	cr.shots = append(cr.shots, *shot)
	cr.shotID++

	// 重置采样，等待下次移动
	cr.samples = nil
	cr.startAt = endTime
}

// StopRecording 停止录制并返回镜头数据
func StopRecording() (*CameraShot, error) {
	cameraRecorderMu.Lock()
	defer cameraRecorderMu.Unlock()

	if cameraRecorder == nil || !cameraRecorder.running {
		return nil, fmt.Errorf("没有正在进行的录制")
	}

	cr := cameraRecorder
	close(cr.stopCh)
	cr.running = false

	cr.mu.Lock()
	defer cr.mu.Unlock()

	// 如果有正在进行的镜头，先保存
	if cr.started && len(cr.samples) >= 3 {
		cr.finalizeShot(nil, time.Now())
	}

	// 返回最后一个镜头
	if len(cr.shots) > 0 {
		last := cr.shots[len(cr.shots)-1]
		return &last, nil
	}

	return nil, fmt.Errorf("没有录制到任何镜头")
}

// ========== 围绕点计算 ==========

// calcPivot 从采样点计算视线交点（围绕点）
// 用采样点的 pitch/yaw 作为视线方向，计算所有视线的最近交汇点
// 没有交汇点时返回 nil（无焦点），终点权重最大
func calcPivot(samples []CameraSample) *CameraPivot {
	if len(samples) < 4 {
		return nil
	}

	n := len(samples)
	step := 1
	if n > 50 {
		step = n / 50
	}

	type ray struct {
		ox, oy, oz float64
		dx, dy, dz float64
	}

	var rays []ray
	for i := 0; i < n; i += step {
		s := samples[i]
		pitch := float64(s.Pitch) * math.Pi / 180
		yaw := float64(s.Yaw) * math.Pi / 180
		dx := -math.Sin(yaw) * math.Cos(pitch)
		dy := -math.Sin(pitch)
		dz := math.Cos(yaw) * math.Cos(pitch)
		rays = append(rays, ray{s.X, s.Y + 1.62, s.Z, dx, dy, dz})
	}

	// 按位置加权：越靠后的点权重越大
	// 每个点根据其索引重复加入，终点权重最大但所有点都被考虑
	for w := 0; w < n; w += step {
		weight := (w / step) + 1
		if weight > 5 {
			weight = 5
		}
		for r := 0; r < weight; r++ {
			s := samples[w]
			pitch := float64(s.Pitch) * math.Pi / 180
			yaw := float64(s.Yaw) * math.Pi / 180
			dx := -math.Sin(yaw) * math.Cos(pitch)
			dy := -math.Sin(pitch)
			dz := math.Cos(yaw) * math.Cos(pitch)
			rays = append(rays, ray{s.X, s.Y + 1.62, s.Z, dx, dy, dz})
		}
	}

	if len(rays) < 2 {
		return nil
	}

	var sumX, sumY, sumZ float64
	count := 0

	for i := 0; i < len(rays); i++ {
		bestDist := math.MaxFloat64
		bestX, bestY, bestZ := rays[i].ox, rays[i].oy, rays[i].oz
		for j := 0; j < len(rays); j++ {
			if i == j {
				continue
			}
			A := []float64{rays[i].ox, rays[i].oy, rays[i].oz}
			B := []float64{rays[j].ox, rays[j].oy, rays[j].oz}
			D := []float64{rays[i].dx, rays[i].dy, rays[i].dz}
			E := []float64{rays[j].dx, rays[j].dy, rays[j].dz}

			AB := []float64{A[0] - B[0], A[1] - B[1], A[2] - B[2]}
			dDotD := D[0]*D[0] + D[1]*D[1] + D[2]*D[2]
			dDotE := D[0]*E[0] + D[1]*E[1] + D[2]*E[2]
			eDotE := E[0]*E[0] + E[1]*E[1] + E[2]*E[2]
			abDotD := AB[0]*D[0] + AB[1]*D[1] + AB[2]*D[2]
			abDotE := AB[0]*E[0] + AB[1]*E[1] + AB[2]*E[2]

			denom := dDotD*eDotE - dDotE*dDotE
			if math.Abs(denom) < 1e-10 {
				continue
			}
			t := (abDotD*eDotE - abDotE*dDotE) / denom

			px := A[0] + t*D[0]
			py := A[1] + t*D[1]
			pz := A[2] + t*D[2]

			dist := math.Sqrt((px-B[0])*(px-B[0]) + (py-B[1])*(py-B[1]) + (pz-B[2])*(pz-B[2]))
			if dist < bestDist {
				bestDist = dist
				bestX, bestY, bestZ = px, py, pz
			}
		}
		if bestDist < 50 {
			sumX += bestX
			sumY += bestY
			sumZ += bestZ
			count++
		}
	}

	if count == 0 {
		return nil
	}

	return &CameraPivot{
		X: math.Round(sumX/float64(count)*100) / 100,
		Y: math.Round(sumY/float64(count)*100) / 100,
		Z: math.Round(sumZ/float64(count)*100) / 100,
	}
}

// ========== 缓动推荐 ==========

func recommendEasing(samples []CameraSample) string {
	if len(samples) < 5 {
		return "out_sine"
	}

	// 计算速度曲线
	velocities := make([]float64, 0)
	for i := 1; i < len(samples); i++ {
		s := samples[i]
		sp := samples[i-1]
		dt := float64(s.Timestamp - sp.Timestamp)
		if dt < 1 {
			dt = 1
		}
		dx := s.X - sp.X
		dy := s.Y - sp.Y
		dz := s.Z - sp.Z
		v := math.Sqrt(dx*dx+dy*dy+dz*dz) / dt
		velocities = append(velocities, v)
	}

	if len(velocities) < 3 {
		return "out_sine"
	}

	// 分析速度变化趋势
	firstHalf := velocities[:len(velocities)/2]
	secondHalf := velocities[len(velocities)/2:]

	var avgFirst, avgSecond float64
	for _, v := range firstHalf {
		avgFirst += v
	}
	for _, v := range secondHalf {
		avgSecond += v
	}
	avgFirst /= float64(len(firstHalf))
	avgSecond /= float64(len(secondHalf))

	ratio := avgSecond / (avgFirst + 0.001)

	switch {
	case ratio < 0.5:
		// 后半段比前半段慢很多 → 减速运动 → 缓出（ease out）
		return "out_cubic"
	case ratio > 1.5:
		// 后半段比前半段快很多 → 加速运动 → 缓入（ease in）
		return "in_cubic"
	case math.Abs(ratio-1) < 0.2:
		return "linear" // 匀速
	default:
		return "out_sine" // 默认
	}
}

// ========== 指令生成 ==========

// calcStartDir 从采样点计算朝向（水平 yaw, 垂直 pitch）
// 以终点权重最大：用最后两个采样点计算方向
// calcStartDir 返回第一个采样点的朝向（用于 TP）
func calcStartDir(samples []CameraSample) (yaw, pitch float64) {
	if len(samples) == 0 {
		return 0, 0
	}
	return float64(samples[0].Yaw), float64(samples[0].Pitch)
}

// calcEndDir 返回最后一个采样点的朝向（用于 camera rot）
func calcEndDir(samples []CameraSample) (yaw, pitch float64) {
	if len(samples) == 0 {
		return 0, 0
	}
	return float64(samples[len(samples)-1].Yaw), float64(samples[len(samples)-1].Pitch)
}

// GenerateCommands 生成 camera 指令
func GenerateCommands(shots []CameraShot, player string) []string {
	var cmds []string
	for _, shot := range shots {
		cmds = append(cmds, fmt.Sprintf("# camera_shot_%d - %.1fs", shot.ID, float64(shot.Duration)/1000))

		// 计算朝向
		dirYaw, dirPitch := calcEndDir(shot.Samples)

		duration := float64(shot.Duration) / 1000
		if duration < 0.01 {
			duration = 0.01
		}

		if shot.Pivot != nil {
			cmds = append(cmds, fmt.Sprintf("# pivot: (%.1f, %.1f, %.1f)", shot.Pivot.X, shot.Pivot.Y, shot.Pivot.Z))
			cmds = append(cmds, fmt.Sprintf(
				"execute as @a at @s run camera @a set minecraft:free ease %.3f %s pos %.1f %.1f %.1f facing %.1f %.1f %.1f",
				duration, shot.Easing,
				shot.End.X, shot.End.Y, shot.End.Z,
				shot.Pivot.X, shot.Pivot.Y, shot.Pivot.Z,
			))
		} else {
			cmds = append(cmds, fmt.Sprintf(
				"execute as @a at @s run camera @a set minecraft:free ease %.3f %s pos %.1f %.1f %.1f rot %.1f %.1f",
				duration, shot.Easing,
				shot.End.X, shot.End.Y, shot.End.Z,
				dirPitch, dirYaw,
			))
		}
		cmds = append(cmds, "")
	}
	return cmds
}

// ExportMcfunction 导出为 .mcfunction 文件
func ExportMcfunction(shots []CameraShot, player string) (string, error) {
	cmds := GenerateCommands(shots, player)
	content := ""
	for _, cmd := range cmds {
		content += cmd + "\n"
	}

	ts := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("camera_shot_%s.mcfunction", ts)
	path := filepath.Join(debugLogDir, filename)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %v", err)
	}

	return path, nil
}

// ========== 状态查询 ==========

// GetRecordingStatus 获取录制状态
func GetRecordingStatus() map[string]any {
	cameraRecorderMu.Lock()
	defer cameraRecorderMu.Unlock()

	if cameraRecorder == nil || !cameraRecorder.running {
		return map[string]any{
			"running": false,
			"shots":   getShotSummaries(cameraRecorder),
		}
	}

	cr := cameraRecorder
	cr.mu.Lock()
	defer cr.mu.Unlock()

	return map[string]any{
		"running":    true,
		"player":     cr.player,
		"started":    cr.started,
		"samples":    len(cr.samples),
		"idle_count": cr.idleCount,
		"shots":      getShotSummaries(cr),
		"last_sample": len(cr.samples) > 0,
	}
}

func getShotSummaries(cr *CameraRecorder) []map[string]any {
	if cr == nil {
		return nil
	}
	var summaries []map[string]any
	for _, shot := range cr.shots {
		summaries = append(summaries, map[string]any{
			"id":       shot.ID,
			"player":   shot.Player,
			"duration": shot.Duration,
			"distance": shot.Distance,
			"easing":   shot.Easing,
			"pivot":    shot.Pivot,
		})
	}
	return summaries
}

// UpdateShot 更新镜头参数（围绕点、起始终止坐标、时长、缓动）
func UpdateShot(shotID int, updates map[string]any) (*CameraShot, error) {
	cameraRecorderMu.Lock()
	defer cameraRecorderMu.Unlock()

	if cameraRecorder == nil {
		return nil, fmt.Errorf("没有录制数据")
	}

	for i := range cameraRecorder.shots {
		shot := &cameraRecorder.shots[i]
		if shot.ID != shotID {
			continue
		}

		if pivot, ok := updates["pivot"].(map[string]any); ok {
			shot.Pivot = &CameraPivot{
				X: toFloat64(pivot["x"]),
				Y: toFloat64(pivot["y"]),
				Z: toFloat64(pivot["z"]),
			}
		}
		if start, ok := updates["start"].(map[string]any); ok {
			if x, ok := start["x"]; ok {
				shot.Start.X = toFloat64(x)
			}
			if y, ok := start["y"]; ok {
				shot.Start.Y = toFloat64(y)
			}
			if z, ok := start["z"]; ok {
				shot.Start.Z = toFloat64(z)
			}
			if pitch, ok := start["pitch"]; ok {
				shot.Start.Pitch = float32(toFloat64(pitch))
			}
			if yaw, ok := start["yaw"]; ok {
				shot.Start.Yaw = float32(toFloat64(yaw))
			}
		}
		if end, ok := updates["end"].(map[string]any); ok {
			if x, ok := end["x"]; ok {
				shot.End.X = toFloat64(x)
			}
			if y, ok := end["y"]; ok {
				shot.End.Y = toFloat64(y)
			}
			if z, ok := end["z"]; ok {
				shot.End.Z = toFloat64(z)
			}
			if pitch, ok := end["pitch"]; ok {
				shot.End.Pitch = float32(toFloat64(pitch))
			}
			if yaw, ok := end["yaw"]; ok {
				shot.End.Yaw = float32(toFloat64(yaw))
			}
		}
		if easing, ok := updates["easing"].(string); ok {
			shot.Easing = easing
		}
		if duration, ok := updates["duration"]; ok {
			shot.Duration = int64(toFloat64(duration))
		}

		return shot, nil
	}

	return nil, fmt.Errorf("镜头 #%d 未找到", shotID)
}

// GetShots 获取所有镜头
func GetShots() []CameraShot {
	cameraRecorderMu.Lock()
	defer cameraRecorderMu.Unlock()
	if cameraRecorder == nil {
		return nil
	}
	shots := make([]CameraShot, len(cameraRecorder.shots))
	copy(shots, cameraRecorder.shots)
	return shots
}

// DeleteShot 删除指定镜头
func DeleteShot(shotID int) error {
	cameraRecorderMu.Lock()
	defer cameraRecorderMu.Unlock()
	if cameraRecorder == nil {
		return fmt.Errorf("没有录制数据")
	}
	for i, shot := range cameraRecorder.shots {
		if shot.ID == shotID {
			cameraRecorder.shots = append(cameraRecorder.shots[:i], cameraRecorder.shots[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("镜头 #%d 未找到", shotID)
}

// ClearShots 清空所有镜头
func ClearShots() {
	cameraRecorderMu.Lock()
	defer cameraRecorderMu.Unlock()
	if cameraRecorder != nil {
		cameraRecorder.shots = nil
		cameraRecorder.shotID = 0
	}
}

// ========== 工具函数 ==========

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	default:
		return 0
	}
}