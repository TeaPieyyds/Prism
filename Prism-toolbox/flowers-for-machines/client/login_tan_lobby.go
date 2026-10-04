package client

import (
	"context"
	"fmt"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/login"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"

	tanservice "github.com/Happy2018new/nemc-tan-lobby-solver/protocol/service"
)

// LoginTanLobbyRoom 通过 TanLobby 中继加入本地联机房间。
// cfg.RentalServerCode 为房间号，cfg.RentalServerPasscode 为房间密码（无密码则留空）。
// 返回的 Client 与 LoginRentalServer 结构一致，后续流程完全复用。
func LoginTanLobbyRoom(cfg Config) (client *Client, err error) {
	DebugLog("=== LoginTanLobbyRoom 开始 ===")
	DebugLog("房间号: %s, 密码: %s", cfg.RentalServerCode, cfg.RentalServerPasscode)
	DebugLog("认证服务器: %s", cfg.AuthServerAddress)
	defer func() {
		if err != nil {
			DebugLog("LoginTanLobbyRoom 失败: %v", err)
		} else {
			DebugLog("LoginTanLobbyRoom 成功")
		}
	}()

	if cfg.OnLoginProgress != nil {
		cfg.OnLoginProgress("向认证服务器请求令牌...")
	}
	DebugLog("Step 1: 创建 authClient...")
	authClient, err := auth.CreateClient(&auth.ClientOptions{
		AuthServer: cfg.AuthServerAddress,
	})
	if err != nil {
		DebugLog("authClient 创建失败: %v", err)
		return nil, fmt.Errorf("LoginTanLobbyRoom: %v", err)
	}
	DebugLog("authClient 创建成功")

	if cfg.OnLoginProgress != nil {
		cfg.OnLoginProgress("通过中继服务器加入房间...")
	}
	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*600)
	defer cancelFunc()

	// 构造 AccessWrapper（Token 即 FBToken）
	wrapper := auth.NewAccessWrapper(
		authClient,
		cfg.RentalServerCode,      // 房间号
		cfg.RentalServerPasscode,  // 房间密码
		cfg.AuthServerToken,       // FBToken
		"", "",
	)

	DebugLog("Step 2: 调用 TanLobbyGetAccess...")
	loginResp, err := wrapper.TanLobbyGetAccess(cfg.RentalServerCode)
	if err != nil {
		DebugLog("TanLobbyGetAccess 失败: %v", err)
		return nil, fmt.Errorf("LoginTanLobbyRoom: %v", err)
	}
	DebugLog("TanLobbyGetAccess 成功, UserUniqueID=%d, PlayerName=%s", loginResp.UserUniqueID, loginResp.UserPlayerName)
	DebugLog("RaknetAddr: %s, SignalingAddr: %s", loginResp.RaknetServerAddress, loginResp.SignalingServerAddress)
	DebugLog("RaknetRand len=%d, RaknetAESRand len=%d", len(loginResp.RaknetRand), len(loginResp.RaknetAESRand))
	DebugLog("EncryptKey len=%d, DecryptKey len=%d", len(loginResp.EncryptKeyBytes), len(loginResp.DecryptKeyBytes))

	// 连接 TanLobby 房间 → 得到 WebRTC 隧道
	DebugLog("Step 3: 调用 tanservice.DialContext...")
	netConn, tanLobbyLoginResp, err := tanservice.DialContext(
		ctx,
		cfg.RentalServerCode,         // roomID
		cfg.RentalServerPasscode,     // password
		auth.NewTanLobbyAuthenticator(wrapper),
	)
	if err != nil {
		DebugLog("tanservice.DialContext 失败: %v", err)
		return nil, fmt.Errorf("LoginTanLobbyRoom: %v", err)
	}
	DebugLog("tanservice.DialContext 成功, netConn=%T", netConn)
	DebugLog("RemoteAddr=%s", netConn.RemoteAddr().String())
	DebugLog("LocalAddr=%s", netConn.LocalAddr().String())

	if cfg.OnLoginProgress != nil {
		cfg.OnLoginProgress("通过 WebRTC 隧道登录游戏服务器...")
	}

	// 构造身份数据（使用 TanLobby 返回的 UID 和角色名）
	identityData := login.IdentityData{
		Uid:         int64(tanLobbyLoginResp.UserUniqueID),
		DisplayName: tanLobbyLoginResp.UserPlayerName,
	}
	clientData := login.ClientData{
		ServerAddress: tanLobbyLoginResp.RaknetServerAddress,
	}

	// 在 WebRTC 隧道上做 Minecraft 离线登录
	DebugLog("Step 4: 调用 minecraft.DialNetConnContext (30s timeout)...")
	loginCtx, cancelLogin := context.WithTimeout(context.Background(), time.Second*60)
	defer cancelLogin()
	conn, err := minecraft.DialNetConnContext(loginCtx, netConn, identityData, clientData)
	if err != nil {
		DebugLog("DialNetConnContext 失败: %v", err)
		_ = netConn.Close()
		return nil, fmt.Errorf("LoginTanLobbyRoom: %v", err)
	}
	DebugLog("DialNetConnContext 成功, EntityUniqueID=%d", conn.GameData().EntityUniqueID)

	if cfg.OnLoginProgress != nil {
		cfg.OnLoginProgress("发送初始化包...")
	}

	DebugLog("Step 5: 发送 postLoginPackets...")
	// 初始化缓存包通道（关闭状态，NewResourcesControl 的排空循环会立即退出）
	cachedCh := make(chan packet.Packet)
	close(cachedCh)

	client = &Client{
		connection:      conn,
		authClient:      authClient,
		cachedPacket:    cachedCh,
		BotComponent:    tanLobbyLoginResp.BotComponent,
		OnLoginPacket:   cfg.OnLoginPacket,
		OnLoginProgress: cfg.OnLoginProgress,
	}

	// 发送登录后的初始化包序列（同租赁服登录流程）
	postLoginPackets(conn, tanLobbyLoginResp.BotComponent, tanLobbyLoginResp.BotSkin.ItemID)
	DebugLog("postLoginPackets 完成")

	return client, nil
}