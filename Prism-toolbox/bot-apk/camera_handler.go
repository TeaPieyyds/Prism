package main

import (
	"encoding/json"
	"net/http"
	"fmt"
	"strings"
	"time"
)

// ========== API 处理函数 ==========

func handlePlayersList(w http.ResponseWriter, r *http.Request) {
	type playerSummary struct {
		Name      string  `json:"name"`
		PosX      float32 `json:"pos_x"`
		PosY      float32 `json:"pos_y"`
		PosZ      float32 `json:"pos_z"`
		Pitch     float32 `json:"pitch"`
		Yaw       float32 `json:"yaw"`
		IsOP      bool    `json:"is_op"`
		Dimension int     `json:"dimension"`
	}
	var list []playerSummary
	seen := make(map[string]bool)

	// 主数据源：从 PlayerInfoManager 获取（含完整位置/角度）
	piList := playerInfoMgr.GetAllPlayers()
	for _, p := range piList {
		seen[p.Name] = true
		list = append(list, playerSummary{
			Name:      p.Name,
			PosX:      p.Position[0],
			PosY:      p.Position[1],
			PosZ:      p.Position[2],
			Pitch:     p.Pitch,
			Yaw:       p.Yaw,
			IsOP:      p.IsOP,
			Dimension: p.Dimension,
		})
	}

	// 补充：从 bot_op 全局 players 表补全缺失的玩家
	opMu.Lock()
	for name, eid := range players {
		_ = eid
		if seen[name] {
			continue
		}
		seen[name] = true
		isOP := false
		if _, ok := opPlayers[name]; ok {
			isOP = true
		}
		// 尝试从 PlayerInfoManager 补充位置数据
		posX, posY, posZ := float32(0), float32(0), float32(0)
		pitch, yaw := float32(0), float32(0)
		dim := 0
		if pi := playerInfoMgr.GetPlayer(name); pi != nil {
			posX, posY, posZ = pi.Position[0], pi.Position[1], pi.Position[2]
			pitch, yaw = pi.Pitch, pi.Yaw
			dim = pi.Dimension
		} else if opInfo, ok := opPlayers[name]; ok {
			posX, posY, posZ = opInfo.Pos[0], opInfo.Pos[1], opInfo.Pos[2]
		}
		list = append(list, playerSummary{
			Name:      name,
			PosX:      posX,
			PosY:      posY,
			PosZ:      posZ,
			Pitch:     pitch,
			Yaw:       yaw,
			IsOP:      isOP,
			Dimension: dim,
		})
	}
	opMu.Unlock()

	writeJSON(w, map[string]any{"ok": true, "players": list})
}

func handleCameraStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持 POST"})
		return
	}
	var req struct {
		Player string `json:"player"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "参数错误: " + err.Error()})
		return
	}
	if req.Player == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "未指定玩家"})
		return
	}

	cr, err := StartRecording(req.Player)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{
		"ok":     true,
		"player": cr.player,
	})
}

func handleCameraStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持 POST"})
		return
	}

	shot, err := StopRecording()
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{
		"ok":   true,
		"shot": shot,
	})
}

func handleCameraStatus(w http.ResponseWriter, r *http.Request) {
	status := GetRecordingStatus()
	status["ok"] = true
	writeJSON(w, status)
}

func handleCameraPivot(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持 POST"})
		return
	}

	var req struct {
		ShotID int `json:"shot_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "参数错误: " + err.Error()})
		return
	}

	shots := GetShots()
	for _, shot := range shots {
		if shot.ID == req.ShotID {
			writeJSON(w, map[string]any{
				"ok":    true,
				"pivot": shot.Pivot,
			})
			return
		}
	}

	writeJSON(w, map[string]any{"ok": false, "error": "镜头未找到"})
}

func handleCameraShots(w http.ResponseWriter, r *http.Request) {
	shots := GetShots()
	writeJSON(w, map[string]any{
		"ok":    true,
		"shots": shots,
	})
}

func handleCameraShotUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持 POST"})
		return
	}

	var req struct {
		ShotID  int            `json:"shot_id"`
		Updates map[string]any `json:"updates"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "参数错误: " + err.Error()})
		return
	}

	shot, err := UpdateShot(req.ShotID, req.Updates)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{
		"ok":   true,
		"shot": shot,
	})
}

func handleCameraShotDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持 POST"})
		return
	}

	var req struct {
		ShotID int `json:"shot_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "参数错误: " + err.Error()})
		return
	}

	if err := DeleteShot(req.ShotID); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

func handleCameraGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持 POST"})
		return
	}

	var req struct {
		Player string `json:"player"`
		Format string `json:"format"` // "text" or "mcfunction"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "参数错误: " + err.Error()})
		return
	}

	shots := GetShots()
	if len(shots) == 0 {
		writeJSON(w, map[string]any{"ok": false, "error": "没有镜头"})
		return
	}

	if req.Format == "mcfunction" {
		path, err := ExportMcfunction(shots, req.Player)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{
			"ok":   true,
			"path": path,
		})
		return
	}

	// 默认：纯文本
	cmds := GenerateCommands(shots, req.Player)
	text := ""
	for _, cmd := range cmds {
		text += cmd + "\n"
	}
	writeJSON(w, map[string]any{
		"ok":   true,
		"text": text,
	})
}

func handleCameraTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持 POST"})
		return
	}

	var req struct {
		ShotID int    `json:"shot_id"`
		Player string `json:"player"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "参数错误: " + err.Error()})
		return
	}

	shots := GetShots()
	var targetShot *CameraShot
	for _, shot := range shots {
		if shot.ID == req.ShotID {
			targetShot = &shot
			break
		}
	}

	if targetShot == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "镜头未找到"})
		return
	}

	player := req.Player
	if player == "" {
		player = targetShot.Player
	}

	bm := getBotManager()
	if bm == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "机器人未连接"})
		return
	}

	// 单独测试：TP + 等待2秒 + 播放 + 清除
	startYaw, startPitch := calcStartDir(targetShot.Samples)
	tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f %.1f %.1f", player, targetShot.Start.X, targetShot.Start.Y, targetShot.Start.Z, startYaw, startPitch)
	if err := bm.SendAICommand(tpCmd); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": fmt.Sprintf("TP失败: %v", err)})
		return
	}
	time.Sleep(2 * time.Second)

	cmds := GenerateCommands([]CameraShot{*targetShot}, player)
	var errs []string
	for _, cmd := range cmds {
		line := cmd
		if line == "" || line[0] == '#' {
			continue
		}
		line = strings.ReplaceAll(line, "@a", player)
		if err := bm.SendAICommand(line); err != nil {
			errs = append(errs, fmt.Sprintf("命令失败: %s → %v", line, err))
		}
	}

	// 等待镜头播放完成后再清除，避免玩家卡在自由相机模式
	time.Sleep(time.Duration(targetShot.Duration) * time.Millisecond)
	bm.SendAICommand(fmt.Sprintf("camera %s clear", player))
	if len(errs) > 0 {
		writeJSON(w, map[string]any{"ok": false, "errors": errs})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleCameraTestAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持 POST"})
		return
	}

	shots := GetShots()
	if len(shots) == 0 {
		writeJSON(w, map[string]any{"ok": false, "error": "没有镜头"})
		return
	}

	bm := getBotManager()
	if bm == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "机器人未连接"})
		return
	}

	player := shots[0].Player
	if player == "" {
		player = "@p"
	}

	var errs []string
	for i, shot := range shots {
		// 第1个镜头前 TP 到起点 + 等待2秒
		if i == 0 {
			startYaw, startPitch := calcStartDir(shot.Samples)
			tpCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f %.1f %.1f", player, shot.Start.X, shot.Start.Y, shot.Start.Z, startYaw, startPitch)
			if err := bm.SendAICommand(tpCmd); err != nil {
				errs = append(errs, fmt.Sprintf("TP失败: %s → %v", tpCmd, err))
				break
			}
			time.Sleep(2 * time.Second)
		}

		cmds := GenerateCommands([]CameraShot{shot}, player)
		for _, cmd := range cmds {
			line := cmd
			if line == "" || line[0] == '#' {
				continue
			}
			line = strings.ReplaceAll(line, "@a", player)
			if err := bm.SendAICommand(line); err != nil {
				errs = append(errs, fmt.Sprintf("镜头#%d 失败: %s → %v", shot.ID, line, err))
			}
		}

		waitMs := shot.Duration + 1000
		time.Sleep(time.Duration(waitMs) * time.Millisecond)
	}

	bm.SendAICommand(fmt.Sprintf("camera %s clear", player))

	if len(errs) > 0 {
		writeJSON(w, map[string]any{"ok": false, "errors": errs})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleCameraClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, map[string]any{"ok": false, "error": "仅支持 POST"})
		return
	}
	ClearShots()
	writeJSON(w, map[string]any{"ok": true})
}
