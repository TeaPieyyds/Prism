package main

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/go-gl/mathgl/mgl32"
)

var (
	players   = map[string]int64{}   // name → PlayerList EntityUniqueID
	opPlayers = map[string]*opInfo{} // OP 追踪 (name → info)
	opMu      sync.Mutex
	botPos    mgl32.Vec3
	lastOPCnt = -1 // 周围 OP 数量，-1 未初始化
)

const opRange = 60.0

type opInfo struct {
	Name       string
	ListEID    int64    // PlayerList EntityUniqueID
	RuntimeEID uint64   // AddPlayer EntityRuntimeID
	Pos        mgl32.Vec3
	CmdPerm    byte
	PlayerPerm byte
}

func dist(a, b mgl32.Vec3) float64 {
	dx := float64(a[0] - b[0])
	dy := float64(a[1] - b[1])
	dz := float64(a[2] - b[2])
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func checkSurroundingOP(logCh chan string) {
	if botPos[0] == 0 && botPos[2] == 0 { return }
	var nearby []string
	for name, op := range opPlayers {
		d := dist(botPos, op.Pos)
		if d <= opRange {
			nearby = append(nearby, fmt.Sprintf("%s(%.0f格)", name, d))
		}
	}
	n := len(nearby)
	if n == lastOPCnt { return }
	if lastOPCnt == -1 {
		if n > 0 {
			logCh <- fmt.Sprintf("[OP检测] 周围%.0f格内 %d个OP: %s", opRange, n, strings.Join(nearby, ", "))
		} else {
			logCh <- "[OP检测] 周围无OP，持续监听..."
		}
		lastOPCnt = n
		return
	}
	if n > 0 && lastOPCnt == 0 {
		logCh <- fmt.Sprintf("[OP检测] ⚠ OP出现！%s", strings.Join(nearby, ", "))
	} else if n == 0 && lastOPCnt > 0 {
		logCh <- fmt.Sprintf("[OP检测] ✅ OP全部离开%.0f格范围", opRange)
	} else {
		logCh <- fmt.Sprintf("[OP检测] OP数量变化: %d→%d %s", lastOPCnt, n, strings.Join(nearby, ", "))
	}
	lastOPCnt = n
}

// HandleOPPacket 处理 OP 检测相关的包，返回 true 表示已处理
func HandleOPPacket(pk packet.Packet, logCh chan string) bool {
	opMu.Lock()
	defer opMu.Unlock()
	switch p := pk.(type) {
	case *packet.Respawn:
		botPos = p.Position
		checkSurroundingOP(logCh)

	case *packet.PlayerList:
		if p.ActionType == packet.PlayerListActionAdd {
			for _, e := range p.Entries {
				players[e.Username] = e.EntityUniqueID
			}
		} else {
			for _, e := range p.Entries {
				for n, eid := range players {
					if eid == e.EntityUniqueID {
						delete(players, n)
						if _, ok := opPlayers[n]; ok {
							delete(opPlayers, n)
							checkSurroundingOP(logCh)
						}
					}
				}
			}
		}

	case *packet.AddPlayer:
		ab := p.AbilityData
		name := p.Username
		players[name] = int64(p.EntityRuntimeID)
		isOP := ab.CommandPermissions >= 3
		if isOP {
			old, existed := opPlayers[name]
			if !existed || old.CmdPerm != ab.CommandPermissions {
				opPlayers[name] = &opInfo{
					Name: name, RuntimeEID: p.EntityRuntimeID,
					Pos: p.Position, CmdPerm: ab.CommandPermissions, PlayerPerm: ab.PlayerPermissions,
				}
				logCh <- fmt.Sprintf("[OP加入] !!OP!! %s cmdPerm=%d playerPerm=%d 位置:(%.1f,%.1f,%.1f)",
					name, ab.CommandPermissions, ab.PlayerPermissions, p.Position[0], p.Position[1], p.Position[2])
				checkSurroundingOP(logCh)
			}
		}
		return true

	case *packet.UpdateAbilities:
		eid := p.AbilityData.EntityUniqueID
		cmd := p.AbilityData.CommandPermissions
		isOP := cmd >= 3
		for name, peid := range players {
			if peid == eid {
				if isOP {
					if old, ok := opPlayers[name]; ok {
						old.CmdPerm = cmd
						old.PlayerPerm = p.AbilityData.PlayerPermissions
					} else {
						opPlayers[name] = &opInfo{Name: name, ListEID: eid, CmdPerm: cmd}
						logCh <- fmt.Sprintf("[OP权限] !!OP!! %s cmdPerm=%d playerPerm=%d", name, cmd, p.AbilityData.PlayerPermissions)
						checkSurroundingOP(logCh)
					}
				}
				break
			}
		}
		return true

	case *packet.AdventureSettings:
		isOP := p.CommandPermissionLevel >= packet.CommandPermissionLevelHost
		for name, pt := range players {
			if pt == p.PlayerUniqueID {
				if isOP {
					if _, ok := opPlayers[name]; !ok {
						opPlayers[name] = &opInfo{Name: name, ListEID: p.PlayerUniqueID, CmdPerm: byte(p.CommandPermissionLevel)}
						logCh <- fmt.Sprintf("[OP冒险] !!OP!! %s cmdPerm=%d", name, p.CommandPermissionLevel)
						checkSurroundingOP(logCh)
					}
				}
				break
			}
		}
		return true

	case *packet.PlayerLocation:
		for name, op := range opPlayers {
			if op.ListEID == p.EntityUniqueID {
				old := op.Pos
				op.Pos = p.Position
				if dist(old, p.Position) > 10 {
					logCh <- fmt.Sprintf("[OP传送] %s (%.0f,%.0f,%.0f)→(%.0f,%.0f,%.0f) 距离:%.0f格",
						name, old[0], old[1], old[2], p.Position[0], p.Position[1], p.Position[2], dist(old, p.Position))
					checkSurroundingOP(logCh)
				}
				break
			}
		}

	case *packet.MovePlayer:
		if p.Mode == packet.MoveModeTeleport {
			for name, op := range opPlayers {
				if op.RuntimeEID == p.EntityRuntimeID {
					old := op.Pos
					op.Pos = p.Position
					if dist(old, p.Position) > 10 {
						logCh <- fmt.Sprintf("[OP传送] %s (%.0f,%.0f,%.0f)→(%.0f,%.0f,%.0f) 距离:%.0f格",
							name, old[0], old[1], old[2], p.Position[0], p.Position[1], p.Position[2], dist(old, p.Position))
						checkSurroundingOP(logCh)
					}
					break
				}
			}
		}

	default:
		return false
	}
	return true
}
