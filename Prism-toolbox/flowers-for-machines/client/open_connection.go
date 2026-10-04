package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/google/uuid"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/py_rpc"
	cts "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server"
	cts_mc "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft"
	cts_mc_p "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft/preset"
	cts_mc_v "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/client_to_server/minecraft/vip_event_system"
	mei "github.com/OmineDev/flowers-for-machines/core/py_rpc/mod_event/interface"
)

// openConnection 通过 authenticator 连接到租赁服，
// 并初始化 Minecraft 连接。返回 BotComponent 模组列表供后续模组同步使用。
func openConnection(
	ctx context.Context,
	authenticator minecraft.Authenticator,
) (conn *minecraft.Conn, botComponent map[string]*int, err error) {
	// prepare
	var dialer minecraft.Dialer
	var authResponse auth.AuthResponse

	// create connection
	dialer = minecraft.Dialer{
		Authenticator: authenticator,
		DownloadResourcePack: func(id uuid.UUID, version string, current, total int) bool { return false },
	}
	conn, authResponse, err = dialer.DialContext(ctx, "raknet")
	if err != nil {
		return nil, nil, err
	}

	// capture bot component for return — used later by sendModSync
	botComponent = authResponse.BotComponent

	// 发送登录后的初始化包序列
	postLoginPackets(conn, authResponse.BotComponent, authResponse.BotSkin.ItemID)

	// return
	return
}

// sendBotSkin 发送 PlayerSkin 包，确保其他玩家能看到机器人的皮肤。
func sendBotSkin(conn *minecraft.Conn, skinID string) error {
	identity := conn.IdentityData()
	clientData := conn.ClientData()

	// 使用传入的 skinID 作为 SkinID，与 entity_id 保持一致
	if skinID == "" {
		skinID = clientData.SkinID
	}

	// 解码皮肤数据
	skinData, _ := base64.StdEncoding.DecodeString(clientData.SkinData)
	skinGeometry, _ := base64.StdEncoding.DecodeString(clientData.SkinGeometry)
	skinResourcePatch, _ := base64.StdEncoding.DecodeString(clientData.SkinResourcePatch)
	geometryDataEngineVersion, _ := base64.StdEncoding.DecodeString(clientData.SkinGeometryVersion)
	animationData, _ := base64.StdEncoding.DecodeString(clientData.SkinAnimationData)
	capeData, _ := base64.StdEncoding.DecodeString(clientData.CapeData)

	capeID := clientData.CapeID
	if capeID == "" {
		capeID = uuid.New().String()
	}

	skin := protocol.Skin{
		SkinID:               skinID,
		PlayFabID:            clientData.PlayFabID,
		SkinResourcePatch:    skinResourcePatch,
		SkinImageWidth:       uint32(clientData.SkinImageWidth),
		SkinImageHeight:      uint32(clientData.SkinImageHeight),
		SkinData:             skinData,
		Animations:           make([]protocol.SkinAnimation, 0),
		CapeImageWidth:       uint32(clientData.CapeImageWidth),
		CapeImageHeight:      uint32(clientData.CapeImageHeight),
		CapeData:             capeData,
		SkinGeometry:         skinGeometry,
		GeometryDataEngineVersion: geometryDataEngineVersion,
		AnimationData:        animationData,
		CapeID:               capeID,
		FullID:               skinID,
		ArmSize:              clientData.ArmSize,
		SkinColour:           clientData.SkinColour,
		PremiumSkin:          clientData.PremiumSkin,
		PersonaSkin:          clientData.PersonaSkin,
		PersonaCapeOnClassicSkin: clientData.CapeOnClassicSkin,
		PrimaryUser:          true,
		OverrideAppearance:   true,
		Trusted:              true,
		PersonaPieces:        make([]protocol.PersonaPiece, 0),
		PieceTintColours:     make([]protocol.PersonaPieceTintColour, 0),
	}

	pk := &packet.PlayerSkin{
		UUID: uuid.MustParse(identity.Identity),
		Skin: skin,
	}
	return conn.WritePacket(pk)
}

// postLoginPackets 发送登录后的初始化包序列（网易租赁服/TanLobby 通用）。
// botComponent 为模组列表，skinItemID 为皮肤资源 ID（entity_id）。
func postLoginPackets(conn *minecraft.Conn, botComponent map[string]*int, skinItemID string) {
	runtimeid := fmt.Sprintf("%d", conn.GameData().EntityUniqueID)

	// 发送 LOGIN_UID
	conn.WritePacket(&packet.NeteaseJson{
		Data: []byte(
			fmt.Sprintf(
				`{"eventName":"LOGIN_UID","resid":"","uid":"%d"}`,
				conn.IdentityData().Uid,
			),
		),
	})

	// 发送 ClientCacheStatus（网易需要此包声明缓存状态）
	conn.WritePacket(&packet.ClientCacheStatus{
		Enabled: false,
	})

	// 发送 PlayerSkin（完整皮肤数据）
	if err := sendBotSkin(conn, skinItemID); err != nil {
		fmt.Fprintf(os.Stderr, "[skin] 发送 PlayerSkin 失败: %v\n", err)
	}

	// 模组同步 — SyncUsingMod
	{
		modUUIDs := make([]any, 0)
		botComp := make(map[string]int64, 0)
		for modUUID, outfitType := range botComponent {
			modUUIDs = append(modUUIDs, modUUID)
			if outfitType != nil {
				botComp[modUUID] = int64(*outfitType)
			}
		}
		conn.WritePacket(&packet.PyRpc{
			Value: py_rpc.Marshal(&py_rpc.SyncUsingMod{
				modUUIDs,
				conn.ClientData().SkinID,
				skinItemID,
				true,
				botComp,
			}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	}

	// ClientLoadAddonsFinishedFromGac
	conn.WritePacket(&packet.PyRpc{
		Value:         py_rpc.Marshal(&py_rpc.ClientLoadAddonsFinishedFromGac{}),
		OperationType: packet.PyRpcOperationTypeSend,
	})

	// GetLoadedInstances
	{
		event := cts_mc_p.GetLoadedInstances{PlayerRuntimeID: runtimeid}
		module := cts_mc.Preset{Module: &mei.DefaultModule{Event: &event}}
		park := cts.Minecraft{Default: mei.Default{Module: &module}}
		conn.WritePacket(&packet.PyRpc{
			Value: py_rpc.Marshal(&py_rpc.ModEvent{
				Package: &park,
				Type:    py_rpc.ModEventClientToServer,
			}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	}

	// ArenaGamePlayerFinishLoad
	conn.WritePacket(&packet.PyRpc{
		Value:         py_rpc.Marshal(&py_rpc.ArenaGamePlayerFinishLoad{}),
		OperationType: packet.PyRpcOperationTypeSend,
	})

	// PlayerUiInit
	{
		event := cts_mc_v.PlayerUiInit{RuntimeID: runtimeid}
		module := cts_mc.VIPEventSystem{Module: &mei.DefaultModule{Event: &event}}
		park := cts.Minecraft{Default: mei.Default{Module: &module}}
		conn.WritePacket(&packet.PyRpc{
			Value: py_rpc.Marshal(&py_rpc.ModEvent{
				Package: &park,
				Type:    py_rpc.ModEventClientToServer,
			}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	}

	// ClientInitUIFinishedEventFromGac
	conn.WritePacket(&packet.PyRpc{
		Value: py_rpc.Marshal(&py_rpc.Default{
			NAME: "ClientInitUIFinishedEventFromGac",
			Data: []any{},
		}),
		OperationType: packet.PyRpcOperationTypeSend,
	})
}