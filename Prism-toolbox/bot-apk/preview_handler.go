package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bot-apk/building"
)

// ScanRequest 扫描请求参数
type ScanRequest struct {
	Path      string      `json:"path"`
	Recursive bool        `json:"recursive"`
	Filters   ScanFilters `json:"filters"`
}

// ScanFilters 扫描筛选条件
type ScanFilters struct {
	NameInclude  []string `json:"name_include"`
	NameExclude  []string `json:"name_exclude"`
	NameMatchMode string  `json:"name_match_mode"` // "and" or "or"
	LangFilter   string   `json:"lang_filter"`      // "none", "chinese_only", "no_chinese"
	SizeMin      int64    `json:"size_min"`
	SizeMax      int64    `json:"size_max"`
	Formats      []string `json:"formats"`
}

// ScanEntry 扫描结果条目
type ScanEntry struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Format string `json:"format"`
}

// 支持的建筑文件格式
var supportedFormats = map[string]bool{
	".mcstructure": true,
	".schematic":   true,
	".schem":       true,
	".mcworld":     true,
	
}

func handlePreviewScan(w http.ResponseWriter, r *http.Request) {
	var req ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid request"})
		return
	}

	// 默认格式过滤
	formatFilter := req.Filters.Formats
	if len(formatFilter) == 0 {
		for f := range supportedFormats {
			formatFilter = append(formatFilter, f)
		}
	}
	formatSet := make(map[string]bool)
	for _, f := range formatFilter {
		formatSet[strings.ToLower(f)] = true
	}

	var entries []ScanEntry
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !formatSet[ext] {
			return nil
		}

		// 文件大小过滤
		if req.Filters.SizeMin > 0 && info.Size() < req.Filters.SizeMin {
			return nil
		}
		if req.Filters.SizeMax > 0 && info.Size() > req.Filters.SizeMax {
			return nil
		}

		name := filepath.Base(path)
		nameOnly := strings.TrimSuffix(name, ext)

		// 文件名包含过滤
		if len(req.Filters.NameInclude) > 0 {
			match := false
			for _, kw := range req.Filters.NameInclude {
				has := strings.Contains(nameOnly, kw)
				if req.Filters.NameMatchMode == "and" {
					if !has {
						return nil
					}
					match = true
				} else {
					if has {
						match = true
						break
					}
				}
			}
			if !match {
				return nil
			}
		}

		// 文件名不包含过滤
		for _, kw := range req.Filters.NameExclude {
			if strings.Contains(nameOnly, kw) {
				return nil
			}
		}

		// 语言过滤
		if req.Filters.LangFilter == "chinese_only" {
			if !containsChinese(nameOnly) {
				return nil
			}
		} else if req.Filters.LangFilter == "no_chinese" {
			if containsChinese(nameOnly) {
				return nil
			}
		}

		entries = append(entries, ScanEntry{
			Name:   name,
			Path:   path,
			Size:   info.Size(),
			Format: ext,
		})
		return nil
	}

	if req.Recursive {
		filepath.Walk(req.Path, walkFn)
	} else {
		dir, err := os.Open(req.Path)
		if err == nil {
			defer dir.Close()
			entries2, _ := dir.Readdir(-1)
			for _, info := range entries2 {
				walkFn(filepath.Join(req.Path, info.Name()), info, nil)
			}
		}
	}

	writeJSON(w, map[string]any{
		"ok":    true,
		"files": entries,
		"total": len(entries),
	})
}

func containsChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// PreviewGenerateRequest 生成请求
type PreviewGenerateRequest struct {
	Files        []string      `json:"files"`
	Angles       []AngleConfig `json:"angles"`
	Output       OutputConfig  `json:"output"`
	GenTopDown   bool          `json:"gen_topdown"`
	GenElevation bool          `json:"gen_elevation"`
	GenGIF       bool          `json:"gen_gif"`
	SaveConfig   bool          `json:"save_config"`
}

// AngleConfig 镜头角度配置
type AngleConfig struct {
	Type  string `json:"type"`  // "preset" or "custom"
	Value string `json:"value,omitempty"`
	Label string `json:"label"`
	Yaw   int    `json:"yaw,omitempty"`
	Pitch int    `json:"pitch,omitempty"`
	Dist  int    `json:"dist,omitempty"`
}

// OutputConfig 输出配置
type OutputConfig struct {
	Format string `json:"format"` // "png", "jpg", "webp"
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Mode   string `json:"mode"` // "folder", "flat", "zip"
	Dir    string `json:"dir"`
}

// 角度预设映射
var anglePresets = map[string]func() (float64, float64, float64){
	"front":     func() (float64, float64, float64) { return 0, 0, 0 },
	"back":      func() (float64, float64, float64) { return 180, 0, 0 },
	"left":      func() (float64, float64, float64) { return 270, 0, 0 },
	"right":     func() (float64, float64, float64) { return 90, 0, 0 },
	"northeast": func() (float64, float64, float64) { return 45, 30, 200 },
	"southeast": func() (float64, float64, float64) { return 135, 30, 200 },
	"northwest": func() (float64, float64, float64) { return 315, 30, 200 },
	"southwest": func() (float64, float64, float64) { return 225, 30, 200 },
	"top":       func() (float64, float64, float64) { return 0, 90, 100 },
}

func init() {
	// 八角
	for i := 0; i < 8; i++ {
		deg := i * 45
		key := fmt.Sprintf("%d", deg)
		d := deg
		anglePresets[key] = func() (float64, float64, float64) {
			return float64(d), 30, 200
		}
	}
}

var (
	previewTasks     = make(map[string]*PreviewTask)
	previewTasksMu   sync.Mutex
	previewTaskIDSeq int
)

// PreviewTask 生成任务状态
type PreviewTask struct {
	ID        string
	StartTime time.Time
	Status    string // "running", "complete", "cancelled"
	Progress  float64
	Message   string
	Total     int
	Done      int
	Current   string
	Angle     string
	OutputDir string
	Error     string
	Cancel    chan struct{}
}

func handlePreviewGenerate(w http.ResponseWriter, r *http.Request) {
	var req PreviewGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid request"})
		return
	}

	previewTasksMu.Lock()
	previewTaskIDSeq++
	taskID := fmt.Sprintf("pv_%d", previewTaskIDSeq)
	total := len(req.Files) * (len(req.Angles) + boolToInt(req.GenTopDown) + boolToInt(req.GenElevation)*4)
	if total <= 0 {
		total = 1
	}
	task := &PreviewTask{
		ID:        taskID,
		StartTime: time.Now(),
		Status:    "running",
		Total:     total,
		Cancel:    make(chan struct{}),
	}
	previewTasks[taskID] = task
	previewTasksMu.Unlock()

	go previewGenerateRun(taskID, req)

	writeJSON(w, map[string]any{"ok": true, "task_id": taskID})
}

func handlePreviewStatus(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task_id")
	previewTasksMu.Lock()
	task, ok := previewTasks[taskID]
	previewTasksMu.Unlock()
	if !ok {
		writeJSON(w, map[string]any{"ok": false, "error": "task not found"})
		return
	}
	writeJSON(w, map[string]any{
		"ok":         true,
		"status":     task.Status,
		"progress":   task.Progress,
		"message":    task.Message,
		"total":      task.Total,
		"done":       task.Done,
		"current":    task.Current,
		"angle":      task.Angle,
		"error":      task.Error,
		"output_dir": task.OutputDir,
	})
}

func handlePreviewCancel(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task_id")
	previewTasksMu.Lock()
	task, ok := previewTasks[taskID]
	previewTasksMu.Unlock()
	if !ok {
		writeJSON(w, map[string]any{"ok": false, "error": "task not found"})
		return
	}
	select {
	case task.Cancel <- struct{}{}:
	default:
	}
	task.Status = "cancelled"
	writeJSON(w, map[string]any{"ok": true})
}

func handlePreviewZipBuilding(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dir  string `json:"dir"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid request"})
		return
	}
	if req.Dir == "" || req.Name == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "dir and name required"})
		return
	}
	zipPath := filepath.Join(filepath.Dir(req.Dir), req.Name+".zip")
	if err := createZipFromDir(req.Dir, zipPath); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// 删除原始图片
	os.RemoveAll(req.Dir)
	writeJSON(w, map[string]any{"ok": true, "zip_path": zipPath})
}

func handlePreviewZip(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dir string `json:"dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid request"})
		return
	}
	if req.Dir == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "dir required"})
		return
	}
	zipPath := filepath.Join(req.Dir, "预览图_"+time.Now().Format("20060102_150405")+".zip")
	if err := createZipFromDir(req.Dir, zipPath); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "zip_path": zipPath})
}

func handlePreviewRenderIsometric(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path      string  `json:"path"`
		Yaw       float64 `json:"yaw"`
		Pitch     float64 `json:"pitch"`
		Width  int     `json:"width"`
		Output    string  `json:"output"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid request"})
		return
	}
	data, err := loadBuildingFile(req.Path)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err := building.RenderIsometric(data, req.Yaw, req.Pitch, req.Width, req.Output); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "path": req.Output})
}

func previewGenerateRun(taskID string, req PreviewGenerateRequest) {
	previewTasksMu.Lock()
	task := previewTasks[taskID]
	previewTasksMu.Unlock()

	outputDir := req.Output.Dir
	if outputDir == "" {
		outputDir = "/sdcard/Download/Prism预览/"
	}
	task.OutputDir = outputDir

	// 逐个处理建筑文件
	for _, filePath := range req.Files {
		select {
		case <-task.Cancel:
			return
		default:
		}

		buildingName := filepath.Base(filePath)
		buildingName = strings.TrimSuffix(buildingName, filepath.Ext(buildingName))
		task.Current = buildingName
		task.Message = fmt.Sprintf("正在处理 %s...", buildingName)

		// 解析建筑文件
		data, err := loadBuildingFile(filePath)
		if err != nil {
			task.Error = fmt.Sprintf("解析失败 %s: %v", buildingName, err)
			task.Done += len(req.Angles)
			updateTaskProgress(task)
			continue
		}

		// 获取建筑输出子目录
		buildingDir := outputDir
		if req.Output.Mode == "folder" {
			buildingDir = filepath.Join(outputDir, buildingName)
		}

		// 处理每个角度
		for _, angle := range req.Angles {
			select {
			case <-task.Cancel:
				return
			default:
			}

			task.Angle = angle.Label
			updateTaskProgress(task)

			// 生成图片文件名
			angleName := sanitizeFileName(angle.Label)
			imgName := fmt.Sprintf("%s_%s.%s", buildingName, angleName, req.Output.Format)
			_ = filepath.Join(buildingDir, imgName) // 3D 截图由前端完成

			task.Done++
			updateTaskProgress(task)
		}

		// 2D 俯视图
		if req.GenTopDown {
			select {
			case <-task.Cancel:
				return
			default:
			}
			task.Angle = "俯视图"
			updateTaskProgress(task)

			topPath := filepath.Join(buildingDir, buildingName+"_俯视图."+req.Output.Format)
			pixelSize := req.Output.Width / max(data.SizeX, data.SizeZ)
			if pixelSize < 1 {
				pixelSize = 1
			}
			if err := building.RenderTopDown(data, pixelSize, topPath); err != nil {
				task.Error = fmt.Sprintf("俯视图失败 %s: %v", buildingName, err)
			}
			task.Done++
			updateTaskProgress(task)
		}

		// 四向立面图
		if req.GenElevation {
			for _, dir := range []string{"front", "back", "left", "right"} {
				select {
				case <-task.Cancel:
					return
				default:
				}
				task.Angle = dir
				updateTaskProgress(task)

				elevPath := filepath.Join(buildingDir, buildingName+"_"+dir+"."+req.Output.Format)
				pixelSize := req.Output.Width / max(data.SizeX, data.SizeY)
				if pixelSize < 1 {
					pixelSize = 1
				}
				if err := building.RenderElevation(data, dir, pixelSize, elevPath); err != nil {
					task.Error = fmt.Sprintf("立面图失败 %s/%s: %v", buildingName, dir, err)
				}
				task.Done++
				updateTaskProgress(task)
			}
		}

		// 内存回收
		data = nil
		// 不显式调用 runtime.GC()，Go 的 GC 会自动回收
	}

	// ZIP 打包
	if req.Output.Mode == "zip" {
		task.Message = "正在打包 ZIP..."
		zipPath := filepath.Join(outputDir, "预览图_"+time.Now().Format("20060102_150405")+".zip")
		if err := createZipFromDir(outputDir, zipPath); err != nil {
			task.Error = fmt.Sprintf("ZIP 打包失败: %v", err)
		}
		task.OutputDir = zipPath
	}

	task.Status = "complete"
	task.Progress = 1.0
	task.Message = "生成完成"
}

func updateTaskProgress(task *PreviewTask) {
	if task.Total > 0 {
		task.Progress = float64(task.Done) / float64(task.Total)
		if task.Progress > 1.0 {
			task.Progress = 1.0
		}
	}
}

func getAngleParams(angle AngleConfig) (float64, float64, float64) {
	if angle.Type == "custom" {
		return float64(angle.Yaw), float64(angle.Pitch), float64(angle.Dist)
	}
	if fn, ok := anglePresets[angle.Value]; ok {
		return fn()
	}
	return 45, 30, 200
}

func sanitizeFileName(name string) string {
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
		"°", "度", " ", "",
	)
	return replacer.Replace(name)
}

func createZipFromDir(sourceDir, outputPath string) error {
	os.MkdirAll(filepath.Dir(outputPath), 0755)
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	zipWriter := zip.NewWriter(f)
	defer zipWriter.Close()

	filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(sourceDir, path)
		if relPath == "" || strings.HasPrefix(relPath, "预览图_") || strings.HasSuffix(relPath, ".zip") {
			return nil
		}
		w, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})

	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// loadBuildingFile 根据文件路径解析对应的建筑文件
func loadBuildingFile(path string) (*building.StructureData, error) {
	return loadStructureFileCached(path)
}

// handlePreviewSaveImage 接收前端 Three.js 渲染的截图 base64 数据，保存到文件
func handlePreviewGenerateGif(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path   string   `json:"path"`
		Frames []string `json:"frames"` // base64 编码的帧图片列表
		Delay int      `json:"delay"`   // 帧间隔（厘秒，默认 5）
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid request"})
		return
	}
	if len(req.Frames) < 2 {
		writeJSON(w, map[string]any{"ok": false, "error": "need at least 2 frames"})
		return
	}
	if req.Delay <= 0 {
		req.Delay = 5
	}

	// 解析所有帧
	var images []*image.Paletted
	var delays []int
	for _, frameData := range req.Frames {
		// 去掉 data:image/...;base64, 前缀
		dataStr := frameData
		if idx := strings.Index(dataStr, "base64,"); idx >= 0 {
			dataStr = dataStr[idx+7:]
		}
		data, err := base64.StdEncoding.DecodeString(dataStr)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "base64 decode failed: " + err.Error()})
			return
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "image decode failed: " + err.Error()})
			return
		}
		// 转换为 Paletted 用于 GIF（使用 Plan9 256色调色板）
		paletted := image.NewPaletted(img.Bounds(), color.Palette(palette.Plan9))
		draw.FloydSteinberg.Draw(paletted, paletted.Rect, img, img.Bounds().Min)
		images = append(images, paletted)
		delays = append(delays, req.Delay)
	}

	// 创建 GIF
	os.MkdirAll(filepath.Dir(req.Path), 0755)
	f, err := os.Create(req.Path)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer f.Close()
	err = gif.EncodeAll(f, &gif.GIF{
		Image:     images,
		Delay:     delays,
		LoopCount: 0, // 无限循环
	})
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "gif encode failed: " + err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "path": req.Path, "frames": len(images)})
}

func handlePreviewSaveImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
		Data string `json:"data"` // base64 编码的图片数据（不含 data:image/png;base64, 前缀）
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid request"})
		return
	}
	if req.Path == "" || req.Data == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "path and data required"})
		return
	}

	// 如果带 data:image/...;base64, 前缀则去掉
	dataStr := req.Data
	if idx := strings.Index(dataStr, "base64,"); idx >= 0 {
		dataStr = dataStr[idx+7:]
	}

	data, err := base64.StdEncoding.DecodeString(dataStr)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "base64 decode failed: " + err.Error()})
		return
	}

	os.MkdirAll(filepath.Dir(req.Path), 0755)
	if err := os.WriteFile(req.Path, data, 0644); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "path": req.Path, "size": len(data)})
}