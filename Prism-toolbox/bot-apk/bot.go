package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/OmineDev/flowers-for-machines/client"
	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
)

type botConn struct {
	conn    *minecraft.Conn
	display string
	xuid    string
	eid     int64
	idStr   string
	pos     mgl32.Vec3

	cmdCh  chan string
	logCh  chan string
	stopCh chan struct{}

	chatMu       sync.Mutex // 聊天限流
	lastChatTime time.Time
}

// sendChat 发送普通聊天消息，限流（每2秒1条）+截断（最长50字符）
func (bc *botConn) sendChat(msg string) {
	bc.chatMu.Lock()
	defer bc.chatMu.Unlock()

	runes := []rune(msg)
	if len(runes) > 50 {
		runes = runes[:50]
		msg = string(runes)
	}
	elapsed := time.Since(bc.lastChatTime)
	if elapsed < 2*time.Second {
		time.Sleep(2*time.Second - elapsed)
	}
	bc.conn.WritePacket(&packet.Text{
		TextType:   packet.TextTypeChat,
		Message:    msg,
		SourceName: bc.display,
		XUID:       bc.xuid,
	})
	bc.lastChatTime = time.Now()
}

func runBot(token, server, auth string, logCh chan string, cmdCh chan string, stopCh chan struct{}) {
	cfg := client.Config{
		AuthServerAddress:    auth,
		RentalServerCode:     server,
		RentalServerPasscode: "",
		AuthServerToken:      token,
	}

	logCh <- fmt.Sprintf("[*] 连接 %s ...", server)

	c, err := client.LoginRentalServer(cfg)
	if err != nil {
		logCh <- fmt.Sprintf("[-] %v", err)
		return
	}

	conn := c.Conn()
	id := conn.IdentityData()
	logCh <- fmt.Sprintf("[+] 连接成功!")
	logCh <- fmt.Sprintf("[+] 玩家: %s", id.DisplayName)

	bc := &botConn{
		conn:    conn,
		display: id.DisplayName,
		xuid:    id.XUID,
		eid:     int64(conn.GameData().EntityUniqueID),
		idStr:   id.Identity,
		pos:     mgl32.Vec3{0, 64, 0},
		cmdCh:   cmdCh,
		logCh:   logCh,
		stopCh:  stopCh,
	}

	// heartbeat
	go func() {
		tk := time.NewTicker(200 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-tk.C:
				conn.WritePacket(&packet.PlayerAuthInput{
					Position:         bc.pos,
					Pitch:            0,
					Yaw:              0,
					HeadYaw:          0,
					MoveVector:       mgl32.Vec2{},
					InputData:        protocol.NewBitset(packet.PlayerAuthInputBitsetSize),
					InputMode:        packet.InputModeMouse,
					PlayMode:         packet.PlayModeScreen,
					InteractionModel: packet.InteractionModelCrosshair,
					Tick:             0,
					Delta:            mgl32.Vec3{},
				})
			}
		}
	}()

	// cmd handler
	go func() {
		for {
			select {
			case <-stopCh:
				return
			case line := <-cmdCh:
				if line == "" {
					continue
				}
				if line[0] == '!' {
					handleBotAction(conn, bc, line[1:], logCh)
				} else if line[0] == '/' {
					conn.WritePacket(&packet.CommandRequest{
						CommandLine: line[1:],
						CommandOrigin: protocol.CommandOrigin{
							Origin:         protocol.CommandOriginPlayer,
							UUID:           uuid.MustParse(bc.idStr),
							PlayerUniqueID: bc.eid,
						},
						Version: 105,
					})
					logCh <- fmt.Sprintf("[命令] %s", line)
				} else {
					bc.sendChat(line)
					logCh <- fmt.Sprintf("[发送] %s", line)
				}
			}
		}
	}()

	logCh <- "[*] 就绪"

	for {
		pk, err := conn.ReadPacket()
		if err != nil {
			logCh <- fmt.Sprintf("[-] %v", err)
			return
		}
		select {
		case <-stopCh:
			return
		default:
		}
		processPacket(pk, logCh, bc)
	}
}

var cmdLastTs time.Time

func processPacket(pk packet.Packet, logCh chan string, bc *botConn) {
	switch p := pk.(type) {
	case *packet.Text:
		logCh <- fmt.Sprintf("[聊天] %s: %s", p.SourceName, p.Message)

	case *packet.Disconnect:
		logCh <- fmt.Sprintf("[-] 被踢: %s", p.Message)

	case *packet.Respawn:
		bc.pos = p.Position
		logCh <- fmt.Sprintf("[重生] 位置: (%.1f, %.1f, %.1f)", p.Position[0], p.Position[1], p.Position[2])

	case *packet.PlayerList:
		if p.ActionType == packet.PlayerListActionAdd {
			for _, e := range p.Entries {
				logCh <- fmt.Sprintf("[加入] %s", e.Username)
			}
		} else if p.ActionType == packet.PlayerListActionRemove {
			for _, e := range p.Entries {
				logCh <- fmt.Sprintf("[退出] EID=%d", e.EntityUniqueID)
			}
		}

	case *packet.AddPlayer:
		logCh <- fmt.Sprintf("[加入] %s 位置:(%.0f,%.0f,%.0f)",
			p.Username, p.Position[0], p.Position[1], p.Position[2])

	case *packet.CommandOutput:
		// 服务器命令广播，对用户无意义，不输出日志
		_ = cmdLastTs
	}
}
