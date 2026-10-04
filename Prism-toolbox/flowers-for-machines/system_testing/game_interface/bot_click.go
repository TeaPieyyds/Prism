package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/game_control/game_interface"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/pterm/pterm"
)

func SystemTestingBotClick() {
	tA := time.Now()

	// ClickBlock
	{
		channel := make(chan struct{})

		doOnce := new(sync.Once)
		uniqueID, err := api.Resources().PacketListener().ListenPacket(
			[]uint32{packet.IDBlockActorData},
			func(p packet.Packet, connCloseErr error) {
				if connCloseErr != nil {
					panic(fmt.Sprintf("SystemTestingBotClick: `ClickBlock` failed due to %v", connCloseErr))
				}
				pk := p.(*packet.BlockActorData)
				if pk.Position != [3]int32{0, 0, 0} {
					return
				}
				_, ok := pk.NBTData["Item"].(map[string]any)
				if ok {
					doOnce.Do(func() { close(channel) })
				}
			},
		)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingBotClick: `ClickBlock` failed due to %v", err))
		}

		api.Commands().SendSettingsCommand("tp 0 0 0", true)
		api.Commands().SendSettingsCommand("clear", true)
		api.Commands().AwaitChangesGeneral()

		api.Commands().SendSettingsCommand("replaceitem entity @s slot.hotbar 2 apple 10", true)
		api.BotClick().ChangeSelectedHotbarSlot(2)
		api.Commands().SendSettingsCommand("setblock 0 0 0 air", true)
		api.Commands().SendSettingsCommand("setblock 0 -1 0 grass", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand(`setblock 0 0 0 glow_frame ["facing_direction"=1]`, true)
		api.Commands().AwaitChangesGeneral()

		api.BotClick().ClickBlock(
			game_interface.UseItemOnBlocks{
				HotbarSlotID: 2,
				BotPos:       mgl32.Vec3{0, 0, 0},
				BlockPos:     [3]int32{0, 0, 0},
				BlockName:    "glow_frame",
				BlockStates: map[string]any{
					"facing_direction":     int32(1),
					"item_frame_map_bit":   byte(0),
					"item_frame_photo_bit": byte(0),
				},
			},
		)

		timer := time.NewTimer(time.Second * 5)
		defer timer.Stop()
		select {
		case <-timer.C:
			panic("SystemTestingBotClick: `ClickBlock` time out")
		case <-channel:
			api.PacketListener().DestroyListener(uniqueID)
		}
	}

	// PickBlock
	{
		api.Commands().SendSettingsCommand("gamemode 1", true)
		api.Commands().SendSettingsCommand("clear", true)
		api.Commands().AwaitChangesGeneral()

		success, resultHotbar, err := api.BotClick().PickBlock([3]int32{0, 0, 0}, true)
		if err != nil {
			panic(fmt.Sprintf("SystemTestingBotClick: `PickBlock` failed due to %v", err))
		}
		if !success {
			panic("SystemTestingBotClick: `PickBlock` failed on test round 1")
		}
		if resultHotbar != 0 {
			panic("SystemTestingBotClick: `PickBlock` failed on test round 2")
		}

		item, _ := api.Resources().Inventories().GetItemStack(0, 0)
		if item == nil {
			panic("SystemTestingBotClick: `PickBlock` failed on test round 3")
		}
		if !strings.Contains(fmt.Sprintf("%#v", item.Stack.NBTData), "(+DATA)") {
			panic("SystemTestingBotClick: `PickBlock` failed on test round 4")
		}
	}

	// PlaceBlockHighLevel
	{
		api.Commands().SendSettingsCommand("clear", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("give @s lime_shulker_box", true)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("tp 30 0 30", true)
		api.Commands().AwaitChangesGeneral()
		api.BotClick().ChangeSelectedHotbarSlot(0)
		api.Commands().AwaitChangesGeneral()
		api.Commands().SendSettingsCommand("setblock 30 0 29 air", true)
		api.Commands().AwaitChangesGeneral()

		api.BotClick().PlaceBlockHighLevel(
			[3]int32{30, 0, 30},
			0,
			2,
		)

		success, err := api.ContainerOpenAndClose().OpenContainer(
			game_interface.UseItemOnBlocks{
				HotbarSlotID: 0,
				BotPos:       mgl32.Vec3{30.5, 1.5, 30.5},
				BlockPos:     [3]int32{30, 0, 30},
				BlockName:    "lime_shulker_box",
				BlockStates:  map[string]any{},
			},
			false,
		)
		if !success || err != nil {
			panic("SystemTestingBotClick: `PlaceBlockHighLevel` failed on test round 1")
		}

		err = api.ContainerOpenAndClose().CloseContainer()
		if err != nil {
			panic("SystemTestingBotClick: `PlaceBlockHighLevel` failed on test round 2")
		}
	}

	pterm.Success.Printfln("SystemTestingBotClick: PASS (Time used = %v)", time.Since(tA))
}
