package client

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/py_rpc"
)

// CopeChallenge ..
func (m *MCPCheckChallengesSolver) CopeChallenge() error {
	err := m.solveMCPCheckChallenges()
	if err != nil {
		return fmt.Errorf("CopeChallenge: %v", err)
	}
	return nil
}

// solveMCPCheckChallenges ..
func (m *MCPCheckChallengesSolver) solveMCPCheckChallenges() error {
	var (
		pk               packet.Packet
		err              error
		challengeTimeout bool
		challengeError   = make(chan struct{})
		challengeSolved  = make(chan struct{})
		cachedPkt        = make(chan packet.Packet, 32767)
		levelChunkSeen   = make(chan struct{})
		timer            = time.NewTimer(time.Second * 120)
	)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				// cachedPkt 可能已关闭，忽略恐慌
			}
		}()
		for {
			if challengeTimeout {
				return
			}
			pk, err = m.client.connection.ReadPacket()
			if challengeTimeout {
				return
			}
			if err != nil {
				close(challengeError)
				return
			}

			m.client.fireCallbacks(pk)

			switch p := pk.(type) {
			case *packet.PyRpc:
				olderStates := m.client.getCheckNumEverPassed
				if err = m.onPyRpc(p); err != nil {
					close(challengeError)
					return
				}
				if !olderStates && m.client.getCheckNumEverPassed {
					close(challengeSolved)
				}
			case *packet.LevelChunk:
				select {
				case <-levelChunkSeen:
				default:
					close(levelChunkSeen)
				}
				select {
				case cachedPkt <- pk:
				default:
				}
			default:
				select {
				case cachedPkt <- pk:
				default:
				}
			}
		}
	}()

	select {
	case <-challengeSolved:
		// 发送 RequestChunkRadius 请求区块，等待 LevelChunk 确认游戏世界已启动.
		// 不发 /list 避免被山头服当洪水攻击踢下线.
		m.client.connection.WritePacket(&packet.RequestChunkRadius{ChunkRadius: 12})

		select {
		case <-levelChunkSeen:
		case <-time.After(time.Second * 10):
		}

		challengeTimeout = true
		close(cachedPkt)
		m.client.cachedPacket = cachedPkt
		return nil
	case <-challengeError:
		close(challengeSolved)
		close(cachedPkt)
		return fmt.Errorf("solveMCPCheckChallenges: %v", err)
	case <-timer.C:
		challengeTimeout = true
		return fmt.Errorf("solveMCPCheckChallenges: Failed to pass the MCPC check challenges, please try again later")
	}
}

// onPyRpc ..
func (m *MCPCheckChallengesSolver) onPyRpc(p *packet.PyRpc) error {
	// prepare
	conn := m.client.connection
	client := m.client.authClient
	if p.Value == nil {
		return nil
	}

	// unmarshal
	content, err := py_rpc.Unmarshal(p.Value)
	if err != nil {
		return fmt.Errorf("onPyRpc: %v", err)
	}

	// do some actions for some specific PyRpc packets
	switch c := content.(type) {
	case *py_rpc.StartType:
		// get data and send packet
		c.Content, err = client.TransferData(c.Content)
		if err != nil {
			return fmt.Errorf("onPyRpc: %v", err)
		}
		c.Type = py_rpc.StartTypeResponse
		conn.WritePacket(&packet.PyRpc{
			Value:         py_rpc.Marshal(c),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	case *py_rpc.GetMCPCheckNum:
		// if the challenges has been down,
		// then we do NOTHING
		if m.client.getCheckNumEverPassed {
			break
		}
		// create request to the auth server and get response
		arg, _ := json.Marshal([]any{
			c.FirstArg,
			c.SecondArg.Arg,
			conn.GameData().EntityUniqueID,
		})
		ret, err := client.TransferCheckNum(string(arg))
		if err != nil {
			return fmt.Errorf("onPyRpc: %v", err)
		}
		// unmarshal response and adjust the data included
		ret_p := []any{}
		json.Unmarshal([]byte(ret), &ret_p)
		if len(ret_p) > 7 {
			ret6, ok := ret_p[6].(float64)
			if ok {
				ret_p[6] = int64(ret6)
			}
		}
		// send packet and mark this challenges was finished
		conn.WritePacket(&packet.PyRpc{
			Value:         py_rpc.Marshal(&py_rpc.SetMCPCheckNum{ret_p}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
		m.client.getCheckNumEverPassed = true
	}

	// return
	return nil
}
