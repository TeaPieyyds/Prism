package game_interface

import (
	"fmt"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

// DefaultMaintainer 是默认的维护器实现。
// 它确保机器人总是保持在创造模式和飞行状态
var DefaultMaintainer Maintainer = new(BaseMaintainer)

// Maintainer 是适用于 GameInterface 的维护器。
// 这意味着将存在一种永远不会销毁，永远工作的实现。
// 您将可以通过维护器来持久地维护机器人的部分特性，
// 例如将机器人的游戏模式保持在创造模式和飞行状态
type Maintainer interface {
	// TouchMaintainer 提供了一种允许
	// 外部使用者手动触发维护行为的方法
	TouchMaintainer(api *GameInterface) error
	// HandlePacket 处理抵达的数据包。
	// pk 则指示了这样的数据包。
	// api 是底层传入的 API 实现
	HandlePacket(pk packet.Packet, api *GameInterface)
	// PacketToListen 罗列该维护器希望监听的数据包的 ID。
	// 应确保该函数返回的 ID 所构成的集合在任何情况下都不变。
	// 另外，确保 HandlePacket 只会收到该函数列出的数据包
	PacketToListen() map[uint32]bool
}

// BaseMaintainer 是默认的维护器实现。
// 它确保机器人总是保持在创造模式和飞行状态
type BaseMaintainer struct{}

// doMaintain ..
func (b *BaseMaintainer) doMaintain(api *GameInterface) error {
	err := api.Commands().SendSettingsCommand("gamemode 1", true)
	if err != nil {
		return fmt.Errorf("doMaintain: %v", err)
	}
	err = api.Commands().AwaitChangesGeneral()
	if err != nil {
		return fmt.Errorf("doMaintain: %v", err)
	}

	err = api.Movement().StopFlying()
	if err != nil {
		return fmt.Errorf("doMaintain: %v", err)
	}
	err = api.Movement().StartFlying()
	if err != nil {
		return fmt.Errorf("doMaintain: %v", err)
	}

	return nil
}

// TouchMaintainer ..
func (b *BaseMaintainer) TouchMaintainer(api *GameInterface) error {
	err := b.doMaintain(api)
	if err != nil {
		return fmt.Errorf("TouchMaintainer: %v", err)
	}
	return nil
}

// HandlePacket ..
func (b *BaseMaintainer) HandlePacket(pk packet.Packet, api *GameInterface) {
	switch p := pk.(type) {
	case *packet.SetPlayerGameType:
		if p.GameType != packet.GameTypeCreative {
			// 在独立协程中执行维护操作，避免阻塞 listenPacket 协程
			// StartFlying 会等待 UpdateAbilities 包，而该包只能由 listenPacket 协程处理
			go b.doMaintain(api)
		}
	case *packet.UpdatePlayerGameType:
		if p.PlayerUniqueID == api.GetBotInfo().EntityUniqueID && p.GameType != packet.GameTypeCreative {
			go b.doMaintain(api)
		}
	case *packet.Respawn:
		if p.State == packet.RespawnStateReadyToSpawn {
			go b.doMaintain(api)
		}
	}
}

// PacketToListen ..
func (b *BaseMaintainer) PacketToListen() map[uint32]bool {
	return map[uint32]bool{
		packet.IDSetPlayerGameType:    true,
		packet.IDUpdatePlayerGameType: true,
		packet.IDRespawn:              true,
	}
}
