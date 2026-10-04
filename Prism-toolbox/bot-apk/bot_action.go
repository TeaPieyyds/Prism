package main

import (
	"fmt"
	"image/color"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"strconv"
	"strings"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/go-gl/mathgl/mgl32"
)

// handleBotAction 解析 !action:key=val,key=val 格式的动作指令
func handleBotAction(conn *minecraft.Conn, bc *botConn, action string, logCh chan string) {
	parts := parseAction(action)
	act := parts["_action"]
	logCh <- fmt.Sprintf("[动作] %s", action)
	switch act {
	case "move":
		x := parseFloat(parts["x"])
		y := parseFloat(parts["y"])
		z := parseFloat(parts["z"])
		conn.WritePacket(&packet.MovePlayer{
			EntityRuntimeID: uint64(bc.eid),
			Position:        mgl32.Vec3{float32(x), float32(y), float32(z)},
			Mode:            packet.MoveModeTeleport,
			TeleportCause:   packet.TeleportCauseCommand,
			OnGround:        true,
			Tick:            uint64(time.Now().UnixMilli()),
		})
		bc.pos = mgl32.Vec3{float32(x), float32(y), float32(z)}
		logCh <- fmt.Sprintf("[移动] → (%.1f,%.1f,%.1f)", x, y, z)
	case "map":
		id := parseInt(parts["id"])
		conn.WritePacket(&packet.MapInfoRequest{MapID: int64(id)})
		logCh <- fmt.Sprintf("[地图] 请求 mapID=%d", id)
	case "mapset":
		id := parseInt(parts["id"])
		idx := parseInt(parts["idx"])
		r := byte(parseInt(parts["r"]))
		g := byte(parseInt(parts["g"]))
		b := byte(parseInt(parts["b"]))
		conn.WritePacket(&packet.MapInfoRequest{
			MapID: int64(id),
			ClientPixels: []protocol.PixelRequest{{Index: uint16(idx), Colour: color.RGBA{R: r, G: g, B: b, A: 255}}},
		})
		logCh <- fmt.Sprintf("[地图] 设置像素 mapID=%d idx=%d rgb=(%d,%d,%d)", id, idx, r, g, b)
	case "hotbar":
		slot := parseInt(parts["slot"])
		conn.WritePacket(&packet.PlayerHotBar{
			SelectedHotBarSlot: uint32(slot - 1), WindowID: 0, SelectHotBarSlot: true,
		})
		logCh <- fmt.Sprintf("[快捷栏] 切换到格 %d", slot)
	case "chunk":
		r := parseInt(parts["radius"])
		conn.WritePacket(&packet.RequestChunkRadius{ChunkRadius: int32(r)})
		logCh <- fmt.Sprintf("[区块] 请求半径 %d", r)
	case "drop":
		conn.WritePacket(&packet.PlayerAction{
			EntityRuntimeID: uint64(bc.eid), ActionType: 4,
		})
		logCh <- "[丢弃] 已丢弃手持物品"
	default:
		logCh <- fmt.Sprintf("[动作] 未知: %s", act)
	}
}

func parseAction(s string) map[string]string {
	m := map[string]string{}
	parts := strings.SplitN(s, ":", 2)
	m["_action"] = parts[0]
	if len(parts) > 1 {
		for _, kv := range strings.Split(parts[1], ",") {
			pair := strings.SplitN(kv, "=", 2)
			if len(pair) == 2 {
				m[pair[0]] = pair[1]
			}
		}
	}
	return m
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}
