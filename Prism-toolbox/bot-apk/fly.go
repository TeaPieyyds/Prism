package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/go-gl/mathgl/mgl32"
)

// ============================================================
// 游戏心跳
// 每 50ms 发送 PlayerAuthInput。
// 包含当前 pos/pitch/yaw，以及 moveVec（持续移动时）。
// 飞行时维持飞行标志位，确保服务器不取消飞行状态。
// ============================================================

func (bm *BotManager) startHeartbeat() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.heartbeatStopCh != nil {
		close(bm.heartbeatStopCh)
	}

	stopCh := make(chan struct{})
	bm.heartbeatStopCh = stopCh

	go func() {
		// 防御：心跳 goroutine 绝不能 panic 崩溃整个进程
		defer func() {
			if r := recover(); r != nil {
				debugLog("heartbeat goroutine panic: %v", r)
			}
		}()
		tk := time.NewTicker(50 * time.Millisecond)
		defer tk.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-tk.C:
				bm.mu.Lock()
				conn := bm.conn
				pos := bm.pos
				pitch := bm.pitch
				yaw := bm.yaw
				flying := bm.isFlying
				flyReq := bm.flyRequested
				mFlags := bm.moveFlags
				mVec := bm.moveVec
				jump := bm.jumpPressed
				sneak := bm.sneakPressed
				sprint := bm.sprintPressed
				lastReported := bm.lastReportedPos

				// 使用 flyRequested 而非 isFlying 判断是否发 StartFlying，
				// 避免 UpdateAbilities 包覆盖 isFlying 导致飞行状态被意外取消/恢复。
				inputData := protocol.NewBitset(packet.PlayerAuthInputBitsetSize)
				if flyReq {
					inputData.Set(packet.InputFlagStartFlying)
				}
				bm.mu.Unlock()

				if conn == nil {
					continue
				}

				// If Pathfinder has active control states, override movement flags
				if HasPFControl() {
					pfCS := GetPFControlState()
					pfMV := pfCS.MoveVector()
					// MoveVector: MCBE 约定 Y=+1=向前, 但 pfCS.MoveVector() 返回 Y=-1=向前
					mVec = mgl32.Vec2{pfMV[0], -pfMV[1]}
					// Set directional flags individually
					flags := uint32(0)
					if pfCS.Forward {
						flags |= 1 << packet.InputFlagUp
					}
					if pfCS.Back {
						flags |= 1 << packet.InputFlagDown
					}
					if pfCS.Left {
						flags |= 1 << packet.InputFlagLeft
					}
					if pfCS.Right {
						flags |= 1 << packet.InputFlagRight
					}
					mFlags = flags
					jump = pfCS.Jump
					sneak = pfCS.Sneak
					sprint = pfCS.Sprint
				}

				// 跳跃键 / 上升
				if jump {
					if flying {
						// 飞行时：跳跃 = 上升
						inputData.Set(packet.InputFlagWantUp)
						inputData.Set(packet.InputFlagAscend)
					} else {
						// 地面时：跳跃 = 真实跳跃
						inputData.Set(packet.InputFlagJumping)
						inputData.Set(packet.InputFlagJumpDown)
						inputData.Set(packet.InputFlagJumpPressedRaw)
						inputData.Set(packet.InputFlagJumpCurrentRaw)
					}
				}

				// 潜行键 / 下降
				if sneak {
					if flying {
						// 飞行时：潜行 = 下降
						inputData.Set(packet.InputFlagWantDown)
						inputData.Set(packet.InputFlagDescend)
					} else {
						// 地面时：潜行 = 真实潜行
						inputData.Set(packet.InputFlagStartSneaking)
						inputData.Set(packet.InputFlagSneaking)
					}
				}

				// 奔跑键已禁用（潜行冲突）
				// 保留 sprint 变量读取但不使用，避免编译错误
				_ = sprint

				// 设置移动输入标志（WASD方向）
				if mFlags != 0 {
					for i := 0; i < 64; i++ {
						if mFlags&(1<<i) != 0 {
							inputData.Set(i)
						}
					}
				}

				// 只在 前端面板打开 + 无任务运行中 时发送 PlayerAuthInput
				// 有任务运行时心跳不参与，避免干扰导入/导出等操作
				// SSE 位置推送始终工作，让前端能看到机器人的位置
				if bm.heartbeatEnabled && (activeTask == nil || !activeTask.Running()) {
					if bm.heartbeatWriteMu.TryLock() {
						conn.WritePacket(&packet.PlayerAuthInput{
							Position:         pos,
							Pitch:            pitch,
							Yaw:              yaw,
							HeadYaw:          yaw,
							MoveVector:       mVec,
							InputData:        inputData,
							InputMode:        packet.InputModeMouse,
							PlayMode:         packet.PlayModeScreen,
							InteractionModel: packet.InteractionModelCrosshair,
							Tick:             0,
							Delta:            mgl32.Vec3{},
						})
						bm.heartbeatWriteMu.Unlock()
					}
				}

				// 位置变化时推送 SSE 事件
				if pos != lastReported && hub != nil {
					bm.mu.Lock()
					bm.lastReportedPos = pos
					bm.mu.Unlock()
					data, _ := json.Marshal(map[string]any{
						"x": pos[0], "y": pos[1], "z": pos[2],
						"pitch": pitch, "yaw": yaw,
					})
					hub.Emit("pos", string(data))
				}
			}
		}
	}()
}

func (bm *BotManager) stopHeartbeat() {
	if bm.heartbeatStopCh != nil {
		close(bm.heartbeatStopCh)
		bm.heartbeatStopCh = nil
	}
}

// SetHeartbeatEnabled 开启/关闭心跳发送 PlayerAuthInput。
// 前端打开飞行/寻路面板时开启，关闭面板时关闭以节省资源。
func (bm *BotManager) SetHeartbeatEnabled(enabled bool) {
	bm.mu.Lock()
	bm.heartbeatEnabled = enabled
	bm.mu.Unlock()
}

// ============================================================
// 移动
// MoveRelative/TeleportTo：MovePlayer 传送（快速跳转）
// 持续移动：通过心跳的 MoveVector + 输入标志让服务器驱动移动
// 位置由服务器通过 PlayerLocation/MovePlayer 包回传，本地不模拟
// ============================================================

// MoveRelative 相对移动（传送跳转）。
func (bm *BotManager) MoveRelative(dx, dy, dz float32) error {
	bm.mu.Lock()
	conn := bm.conn
	eid := bm.eid
	if !bm.connected || conn == nil {
		bm.mu.Unlock()
		return fmt.Errorf("未连接")
	}
	newPos := mgl32.Vec3{
		bm.pos[0] + dx,
		bm.pos[1] + dy,
		bm.pos[2] + dz,
	}
	bm.pos = newPos
	bm.mu.Unlock()

	return conn.WritePacket(&packet.MovePlayer{
		EntityRuntimeID: uint64(eid),
		Position:        newPos,
		Pitch:           0,
		Yaw:             0,
		HeadYaw:         0,
		Mode:            packet.MoveModeTeleport,
		TeleportCause:   packet.TeleportCauseCommand,
		OnGround:        true,
		Tick:            uint64(time.Now().UnixMilli()),
	})
}

// TeleportTo 传送到指定坐标。
func (bm *BotManager) TeleportTo(x, y, z float32) error {
	bm.mu.Lock()
	conn := bm.conn
	eid := bm.eid
	if !bm.connected || conn == nil {
		bm.mu.Unlock()
		return fmt.Errorf("未连接")
	}
	bm.pos = mgl32.Vec3{x, y, z}
	bm.mu.Unlock()

	return conn.WritePacket(&packet.MovePlayer{
		EntityRuntimeID: uint64(eid),
		Position:        mgl32.Vec3{x, y, z},
		Pitch:           0,
		Yaw:             0,
		HeadYaw:         0,
		Mode:            packet.MoveModeTeleport,
		TeleportCause:   packet.TeleportCauseCommand,
		OnGround:        true,
		Tick:            uint64(time.Now().UnixMilli()),
	})
}

// ============================================================
// 持续移动
// 设置 moveVec → 心跳自动携带 MoveVector + 输入标志 → 服务器驱动移动
// 位置由服务器回传，本地不模拟步进（避免显示坐标漂移）
// ============================================================

// StartMoveDirection 开始朝指定方向持续移动。
// 设置输入标志位，心跳每 50ms 发送 PlayerAuthInput 带这些标志，
// 服务器会像处理真实玩家输入一样驱动移动。
func (bm *BotManager) StartMoveDirection(dx, dy, dz float32) error {
	bm.mu.Lock()
	if !bm.connected || bm.conn == nil {
		bm.mu.Unlock()
		return fmt.Errorf("未连接")
	}

	// 停止已有移动
	if bm.moveStopCh != nil {
		close(bm.moveStopCh)
	}
	bm.moveDirection = mgl32.Vec3{dx, dy, dz}
	stopCh := make(chan struct{})
	bm.moveStopCh = stopCh

	// 根据方向设置输入标志位
	flags := uint32(0)
	if dx < -0.5 {
		flags |= 1 << packet.InputFlagLeft
	} else if dx > 0.5 {
		flags |= 1 << packet.InputFlagRight
	}
	if dz < -0.5 {
		flags |= 1 << packet.InputFlagUp // forward
	} else if dz > 0.5 {
		flags |= 1 << packet.InputFlagDown // backward
	}
	if dy > 0.5 {
		flags |= 1 << packet.InputFlagWantUp
	} else if dy < -0.5 {
		flags |= 1 << packet.InputFlagWantDown
	}
	bm.moveFlags = flags
	// MoveVector: 玩家输入空间 X/Z
	// MCBE 约定: Y=+1=向前, Y=-1=向后, X=-1=左, X=+1=右
	// 前端传 dz=-1 为前进，所以 {dx, -dz} 映射为 {0, 1} = 向前
	bm.moveVec = mgl32.Vec2{dx, -dz}
	bm.mu.Unlock()

	return nil
}

// StopMovement 停止持续移动。
func (bm *BotManager) StopMovement() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if bm.moveStopCh != nil {
		close(bm.moveStopCh)
		bm.moveStopCh = nil
	}
	bm.moveDirection = mgl32.Vec3{}
	bm.moveFlags = 0
	bm.moveVec = mgl32.Vec2{}
	return nil
}

// ============================================================
// 视角控制
// ============================================================

// SetRotation 设置机器人的视角方向。
// 只更新本地状态，心跳会在 20ms 内将新视角通过 PlayerAuthInput 发送到服务器。
// 不单独发 MovePlayer 旋转包，避免与心跳的 PlayerAuthInput 冲突。
func (bm *BotManager) SetRotation(pitch, yaw float32) error {
	bm.mu.Lock()
	bm.pitch = pitch
	bm.yaw = yaw
	bm.mu.Unlock()
	return nil
}

// ============================================================
// 跳跃 / 上升 / 下降 / 潜行
// 飞行态：跳跃=上升(通过心跳 WantUp+Ascend)，下降=下降(通过心跳 WantDown+Descend)
// 地面态：跳跃=真实跳跃，下降=潜行(通过心跳 Sneaking)
// 所有操作都通过心跳发送，支持与方向键同时按下
// ============================================================

// JumpStart 按下跳跃键（心跳持续带跳跃/上升标志）。
func (bm *BotManager) JumpStart() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if !bm.connected || bm.conn == nil {
		return fmt.Errorf("未连接")
	}
	bm.jumpPressed = true
	return nil
}

// JumpStop 松开跳跃键。
func (bm *BotManager) JumpStop() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.jumpPressed = false
	return nil
}

// SneakStart 按下潜行键（心跳持续带潜行/下降标志）。
func (bm *BotManager) SneakStart() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if !bm.connected || bm.conn == nil {
		return fmt.Errorf("未连接")
	}
	bm.sneakPressed = true
	return nil
}

// SneakStop 松开潜行键。
func (bm *BotManager) SneakStop() error {
	bm.mu.Lock()
	conn := bm.conn
	bm.sneakPressed = false
	bm.mu.Unlock()
	if conn != nil {
		inputData := protocol.NewBitset(packet.PlayerAuthInputBitsetSize)
		inputData.Set(packet.InputFlagStopSneaking)
		conn.WritePacket(&packet.PlayerAuthInput{
			Position:         bm.pos,
			InputData:        inputData,
			InputMode:        packet.InputModeMouse,
			PlayMode:         packet.PlayModeScreen,
			InteractionModel: packet.InteractionModelCrosshair,
			Tick:             0,
		})
	}
	return nil
}

// SprintStart 按下奔跑键（心跳持续带奔跑标志）。
func (bm *BotManager) SprintStart() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if !bm.connected || bm.conn == nil {
		return fmt.Errorf("未连接")
	}
	bm.sprintPressed = true
	return nil
}

// SprintStop 松开奔跑键。
func (bm *BotManager) SprintStop() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.sprintPressed = false
	return nil
}

// Jump 执行一次跳跃或上升（取决于飞行状态）。
// 通过心跳机制与方向键同时生效，而非单独发包。
func (bm *BotManager) Jump() error {
	bm.mu.Lock()
	conn := bm.conn
	if !bm.connected || conn == nil {
		bm.mu.Unlock()
		return fmt.Errorf("未连接")
	}
	bm.jumpPressed = true
	bm.mu.Unlock()

	// 200ms 后自动释放，模拟一次按键
	go func() {
		time.Sleep(200 * time.Millisecond)
		bm.JumpStop()
	}()

	return nil
}

// Down 执行一次下降或潜行（取决于飞行状态）。
// 直接发包，不依赖心跳机制（心跳机制在创造模式下潜行不可靠）
func (bm *BotManager) Down() error {
	bm.mu.Lock()
	conn := bm.conn
	if !bm.connected || conn == nil {
		bm.mu.Unlock()
		return fmt.Errorf("未连接")
	}
	flying := bm.isFlying
	pos := bm.pos
	bm.mu.Unlock()

	if flying {
		// 飞行时：下降（直接发包，200ms）
		inputData := protocol.NewBitset(packet.PlayerAuthInputBitsetSize)
		inputData.Set(packet.InputFlagDescend)
		inputData.Set(packet.InputFlagWantDown)
		for i := 0; i < 4; i++ {
			conn.WritePacket(&packet.PlayerAuthInput{
				Position:         pos,
				InputData:        inputData,
				InputMode:        packet.InputModeMouse,
				PlayMode:         packet.PlayModeScreen,
				InteractionModel: packet.InteractionModelCrosshair,
				Tick:             0,
			})
			time.Sleep(50 * time.Millisecond)
		}
		return nil
	}

	// 地面时：潜行（直接发包，500ms）
	sneakData := protocol.NewBitset(packet.PlayerAuthInputBitsetSize)
	sneakData.Set(packet.InputFlagSneaking)
	sneakData.Set(packet.InputFlagStartSneaking)
	sneakData.Set(packet.InputFlagSneakDown)
	sneakData.Set(packet.InputFlagSneakPressedRaw)
	sneakData.Set(packet.InputFlagSneakCurrentRaw)

	for i := 0; i < 10; i++ {
		conn.WritePacket(&packet.PlayerAuthInput{
			Position:         pos,
			InputData:        sneakData,
			InputMode:        packet.InputModeMouse,
			PlayMode:         packet.PlayModeScreen,
			InteractionModel: packet.InteractionModelCrosshair,
			Tick:             0,
		})
		time.Sleep(50 * time.Millisecond)
	}

	// 释放潜行
	releaseData := protocol.NewBitset(packet.PlayerAuthInputBitsetSize)
	releaseData.Set(packet.InputFlagStopSneaking)
	releaseData.Set(packet.InputFlagSneakReleasedRaw)

	return conn.WritePacket(&packet.PlayerAuthInput{
		Position:         pos,
		InputData:        releaseData,
		InputMode:        packet.InputModeMouse,
		PlayMode:         packet.PlayModeScreen,
		InteractionModel: packet.InteractionModelCrosshair,
		Tick:             0,
	})
}

// ============================================================
// 飞行控制
// ============================================================

// StartFlying 开启飞行模式。
func (bm *BotManager) StartFlying() error {
	bm.mu.Lock()
	bm.isFlying = true
	bm.flyRequested = true
	bm.mu.Unlock()

	gi, err := bm.getGameInterface()
	if err == nil {
		if err := gi.Movement().StartFlying(); err != nil {
			bm.mu.Lock()
			conn := bm.conn
			pos := bm.pos
			bm.mu.Unlock()
			if conn == nil {
				bm.mu.Lock()
				bm.isFlying = false
				bm.mu.Unlock()
				return fmt.Errorf("未连接")
			}
			return bm.sendFlyingInput(conn, pos, true)
		}
		return nil
	}

	bm.mu.Lock()
	conn := bm.conn
	pos := bm.pos
	bm.mu.Unlock()
	if conn == nil {
		bm.mu.Lock()
		bm.isFlying = false
		bm.mu.Unlock()
		return fmt.Errorf("未连接")
	}
	return bm.sendFlyingInput(conn, pos, true)
}

// StopFlying 关闭飞行模式。
func (bm *BotManager) StopFlying() error {
	bm.mu.Lock()
	bm.isFlying = false
	bm.flyRequested = false
	bm.mu.Unlock()

	gi, err := bm.getGameInterface()
	if err == nil {
		if err := gi.Movement().StopFlying(); err != nil {
			bm.mu.Lock()
			conn := bm.conn
			pos := bm.pos
			bm.mu.Unlock()
			if conn == nil {
				return fmt.Errorf("未连接")
			}
			return bm.sendFlyingInput(conn, pos, false)
		}
		return nil
	}

	bm.mu.Lock()
	conn := bm.conn
	pos := bm.pos
	bm.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("未连接")
	}
	return bm.sendFlyingInput(conn, pos, false)
}

// sendFlyingInput 发送 PlayerAuthInput 飞行/降落标志。
// 心跳会持续发送 StartFlying 维持飞行状态，这里只需发一次切换信号。
func (bm *BotManager) sendFlyingInput(conn *minecraft.Conn, pos mgl32.Vec3, start bool) error {
	// 模拟双击跳跃：第一次"跳跃"让服务器准备飞行
	input1 := protocol.NewBitset(packet.PlayerAuthInputBitsetSize)
	input1.Set(packet.InputFlagJumpDown)
	input1.Set(packet.InputFlagJumping)
	if start {
		input1.Set(packet.InputFlagStartFlying)
	} else {
		input1.Set(packet.InputFlagStopFlying)
	}
	if err := conn.WritePacket(&packet.PlayerAuthInput{
		Position: pos, InputData: input1,
		InputMode: packet.InputModeMouse, PlayMode: packet.PlayModeScreen,
		InteractionModel: packet.InteractionModelCrosshair, Tick: 0,
	}); err != nil {
		return err
	}
	time.Sleep(time.Second / 20)

	// 第二次发包：持续跳跃中维持飞行
	input2 := protocol.NewBitset(packet.PlayerAuthInputBitsetSize)
	input2.Set(packet.InputFlagJumping)
	input2.Set(packet.InputFlagJumpCurrentRaw)
	if start {
		input2.Set(packet.InputFlagStartFlying)
	}
	if err := conn.WritePacket(&packet.PlayerAuthInput{
		Position: pos, InputData: input2,
		InputMode: packet.InputModeMouse, PlayMode: packet.PlayModeScreen,
		InteractionModel: packet.InteractionModelCrosshair, Tick: 0,
	}); err != nil {
		return err
	}
	time.Sleep(time.Second / 20)

	// 释放跳跃
	input3 := protocol.NewBitset(packet.PlayerAuthInputBitsetSize)
	input3.Set(packet.InputFlagJumpReleasedRaw)
	if start {
		input3.Set(packet.InputFlagStartFlying)
	}
	return conn.WritePacket(&packet.PlayerAuthInput{
		Position: pos, InputData: input3,
		InputMode: packet.InputModeMouse, PlayMode: packet.PlayModeScreen,
		InteractionModel: packet.InteractionModelCrosshair, Tick: 0,
	})
}

// ============================================================
// 位置查询
// ============================================================

// GetFlyStatus 返回当前飞行控制状态。
func (bm *BotManager) GetFlyStatus() map[string]any {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return map[string]any{
		"x":         bm.pos[0],
		"y":         bm.pos[1],
		"z":         bm.pos[2],
		"pitch":     bm.pitch,
		"yaw":           bm.yaw,
		"isFlying":      bm.isFlying,
		"flyRequested":  bm.flyRequested,
		"sprinting":     bm.sprintPressed,
		"moving":        bm.moveStopCh != nil,
		"dimension": bm.dimension,
		"pf_active": globalPF != nil && globalPF.IsActive(),
		"pf_status": func() string {
			if globalPF == nil {
				return "未初始化"
			}
			return globalPF.StatusString()
		}(),
		"world_chunks":      len(globalWorld.columns),
		"world_enabled":     globalWorld.IsEnabled(),
		"heartbeat_enabled": bm.heartbeatEnabled,
	}
}

// GetPosition 返回本地跟踪的玩家位置详细信息。
func (bm *BotManager) GetPosition() map[string]any {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return map[string]any{
		"x":         bm.pos[0],
		"y":         bm.pos[1],
		"z":         bm.pos[2],
		"pitch":     bm.pitch,
		"yaw":       bm.yaw,
		"dimension": bm.dimension,
		"isFlying":     bm.isFlying,
			"flyRequested": bm.flyRequested,
	}
}

// QueryPosition 通过服务器 querytarget 命令获取玩家在服务器端的精确位置。
func (bm *BotManager) QueryPosition() (map[string]any, error) {
	gi, err := bm.getGameInterface()
	if err != nil {
		return nil, fmt.Errorf("获取游戏接口: %v", err)
	}

	info, err := gi.Querytarget().DoQuerytarget("@s")
	if err != nil {
		return nil, fmt.Errorf("querytarget: %v", err)
	}
	if len(info) == 0 {
		return nil, fmt.Errorf("未查询到玩家信息")
	}

	return map[string]any{
		"x":         info[0].Position.X,
		"y":         info[0].Position.Y,
		"z":         info[0].Position.Z,
		"yaw":       info[0].YRot,
		"dimension": info[0].Dimension,
		"id":        info[0].PlayerUniqueID,
		"pitch":     bm.pitch,
		"isFlying":     bm.isFlying,
			"flyRequested": bm.flyRequested,
		"sprinting": bm.sprintPressed,
	}, nil
}
