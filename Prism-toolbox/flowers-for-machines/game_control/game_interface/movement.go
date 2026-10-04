package game_interface

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

// 描述切换飞行状态时的最大重试次数
const MaxRetryChangeFlyingStates = 30

var (
	StartFlyingInputData1 = []int{
		packet.InputFlagJumpDown,
		packet.InputFlagJumping,
		packet.InputFlagWantUp,
		packet.InputFlagStartFlying,
		packet.InputFlagJumpPressedRaw,
		packet.InputFlagJumpCurrentRaw,
	}
	StartFlyingInputData2 = []int{
		packet.InputFlagJumpDown,
		packet.InputFlagJumping,
		packet.InputFlagWantUp,
		packet.InputFlagStartFlying,
		packet.InputFlagJumpCurrentRaw,
	}
	StartFlyingInputData3 = []int{
		packet.InputFlagStartFlying,
		packet.InputFlagJumpReleasedRaw,
	}
)

var (
	StopFlyingInputData1 = []int{
		packet.InputFlagJumpDown,
		packet.InputFlagJumping,
		packet.InputFlagWantUp,
		packet.InputFlagStopFlying,
		packet.InputFlagJumpPressedRaw,
		packet.InputFlagJumpCurrentRaw,
	}
	StopFlyingInputData2 = []int{
		packet.InputFlagJumpDown,
		packet.InputFlagJumping,
		packet.InputFlagWantUp,
		packet.InputFlagStartFlying,
		packet.InputFlagJumpCurrentRaw,
	}
	StopFlyingInputData3 = []int{
		packet.InputFlagStartFlying,
		packet.InputFlagJumpReleasedRaw,
	}
)

// Movement 基于 ResourcesWrapper 和 Querytarget 实现了机器人的移动
type Movement struct {
	api         *ResourcesWrapper
	querytarget *Querytarget
}

// NewMovement 根据 api 和 querytarget 返回并创建一个新的 Movement
func NewMovement(api *ResourcesWrapper, querytarget *Querytarget) *Movement {
	return &Movement{
		api:         api,
		querytarget: querytarget,
	}
}

// getBotPos 以 Querytarget 的方式获取机器人的当前位置。
// 注意不能用 @s：命令经 WS 自动化通道（AutomationPlayer 来源）执行时，@s 不指向
// 机器人实体，querytarget @s 会返回空导致位置查询失败。改用 @a 查询所有玩家，
// 再按机器人的 EntityUniqueID 筛出自己的位置（多机器人下其他子机器人也会返回，按 ID 区分）。
func (m *Movement) getBotPos() (pos [3]float32, err error) {
	info, err := m.querytarget.DoQuerytarget("@a")
	if err != nil {
		return [3]float32{}, fmt.Errorf("getBotPos: %v", err)
	}
	for _, it := range info {
		if it.PlayerUniqueID == m.api.EntityUniqueID {
			return [3]float32{it.Position.X, it.Position.Y, it.Position.Z}, nil
		}
	}
	return [3]float32{}, fmt.Errorf("getBotPos: Failed to query the bot position")
}

// sendPlayerAuthInput 向服务器发送 packet.PlayerAuthInput 包。
// pos 指示机器人预期抵达的位置，flags 指示要提交的移动状态控制位。
// 如果该数据包被成功发送，则将确保该函数将会继续阻塞至少一个游戏刻
func (m *Movement) sendPlayerAuthInput(pos [3]float32, flags []int) error {
	inputData := protocol.NewBitset(packet.PlayerAuthInputBitsetSize)
	for _, flag := range flags {
		inputData.Set(flag)
	}

	err := m.api.WritePacket(&packet.PlayerAuthInput{
		Position:  pos,
		InputData: inputData,
	})
	if err != nil {
		return fmt.Errorf("sendPlayerAuthInput: %v", err)
	}
	time.Sleep(time.Second / 20)

	return nil
}

// StartFlying 将机器人切换到悬停的飞行状态。
// 该函数的调用者有责任确保机器人已处于创造模式
func (m *Movement) StartFlying() error {
	// Prepare
	isFlying := new(atomic.Bool)
	isFlying.Store(false)

	// Listen packet
	uniqueID, err := m.api.PacketListener().ListenPacket(
		[]uint32{packet.IDUpdateAbilities},
		func(p packet.Packet, connCloseErr error) {
			if connCloseErr == nil {
				pk := p.(*packet.UpdateAbilities)
				if pk.AbilityData.EntityUniqueID == m.api.BotInfo.EntityUniqueID {
					flying := pk.AbilityData.Layers[0].Values&protocol.AbilityFlying != 0
					isFlying.Store(flying)
				}
			}
		},
	)
	if err != nil {
		return fmt.Errorf("StartFlying: %v", err)
	}
	defer m.api.PacketListener().DestroyListener(uniqueID)

	// Get bot pos
	pos, err := m.getBotPos()
	if err != nil {
		return fmt.Errorf("StartFlying: %v", err)
	}

	// Start flying
	for range MaxRetryChangeFlyingStates {
		if err = m.sendPlayerAuthInput(pos, StartFlyingInputData1); err != nil {
			return fmt.Errorf("StartFlying: %v", err)
		}
		if err = m.sendPlayerAuthInput(pos, StartFlyingInputData2); err != nil {
			return fmt.Errorf("StartFlying: %v", err)
		}
		if err = m.sendPlayerAuthInput(pos, StartFlyingInputData3); err != nil {
			return fmt.Errorf("StartFlying: %v", err)
		}
		if isFlying.Load() {
			break
		}
	}
	if !isFlying.Load() {
		return fmt.Errorf("StartFlying: Failed to switch the flying state")
	}

	// Return
	for range 5 {
		if err = m.sendPlayerAuthInput(pos, []int{packet.InputFlagStartFlying}); err != nil {
			return fmt.Errorf("StartFlying: %v", err)
		}
	}
	return nil
}

// StopFlying 终止机器人的悬停飞行状态
func (m *Movement) StopFlying() error {
	// Prepare
	isFlying := new(atomic.Bool)
	isFlying.Store(true)

	// Listen packet
	uniqueID, err := m.api.PacketListener().ListenPacket(
		[]uint32{packet.IDUpdateAbilities},
		func(p packet.Packet, connCloseErr error) {
			if connCloseErr == nil {
				pk := p.(*packet.UpdateAbilities)
				if pk.AbilityData.EntityUniqueID == m.api.BotInfo.EntityUniqueID {
					flying := pk.AbilityData.Layers[0].Values&protocol.AbilityFlying != 0
					isFlying.Store(flying)
				}
			}
		},
	)
	if err != nil {
		return fmt.Errorf("StopFlying: %v", err)
	}
	defer m.api.PacketListener().DestroyListener(uniqueID)

	// Get bot pos
	pos, err := m.getBotPos()
	if err != nil {
		return fmt.Errorf("StopFlying: %v", err)
	}

	// Stop flying
	for range MaxRetryChangeFlyingStates {
		if err = m.sendPlayerAuthInput(pos, StopFlyingInputData1); err != nil {
			return fmt.Errorf("StopFlying: %v", err)
		}
		if err = m.sendPlayerAuthInput(pos, StopFlyingInputData2); err != nil {
			return fmt.Errorf("StopFlying: %v", err)
		}
		if err = m.sendPlayerAuthInput(pos, StopFlyingInputData3); err != nil {
			return fmt.Errorf("StopFlying: %v", err)
		}
		if !isFlying.Load() {
			break
		}
	}
	if isFlying.Load() {
		return fmt.Errorf("StopFlying: Failed to switch the flying state")
	}

	// Return
	for range 5 {
		if err = m.sendPlayerAuthInput(pos, []int{packet.InputFlagStartFlying}); err != nil {
			return fmt.Errorf("StopFlying: %v", err)
		}
	}
	return nil
}
