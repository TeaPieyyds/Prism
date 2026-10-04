package authsvc

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	g79 "github.com/Yeah114/g79client"
)

var randomNickname = rand.New(rand.NewSource(time.Now().UnixNano()))

func Login(ctx context.Context, cli *g79.Client, p LoginParams, pmCookieClientProvider func(context.Context) (*g79.Client, error)) (LoginResult, error) {
	var result LoginResult

	if cli == nil {
		return result, fmt.Errorf("nil client")
	}
	if err := ensureUserDetail(cli); err != nil {
		return result, err
	}
	if p.ServerCode == "" {
		return result, fmt.Errorf("server code is empty")
	}

	p.ServerPassword = strings.ReplaceAll(p.ServerPassword, "000000", "")

	switch {
	case hasPrefixValue(p.ServerCode, "LobbyGame:"), hasPrefixValue(p.ServerCode, "#"):
		roomCode := strings.TrimPrefix(p.ServerCode, "LobbyGame:")
		roomCode = strings.TrimPrefix(roomCode, "#")
		ipAddress, chainInfo, err := loginLobbyGame(cli, roomCode, p.ServerPassword, p.ClientPublicKey)
		if err != nil {
			return result, err
		}
		result.IP = ipAddress
		result.ChainInfo = chainInfo
	case hasPrefixValue(p.ServerCode, "PCLobbyGame:"):
		if pmCookieClientProvider == nil {
			return result, fmt.Errorf("PC Lobby 缺少客户端提供器")
		}
		roomCode := strings.TrimPrefix(p.ServerCode, "PCLobbyGame:")
		ipAddress, chainInfo, err := loginPCLobbyGame(ctx, cli, roomCode, p.ServerPassword, p.ClientPublicKey, pmCookieClientProvider)
		if err != nil {
			return result, err
		}
		result.IP = ipAddress
		result.ChainInfo = chainInfo
		result.IsPC = true
	case hasPrefixValue(p.ServerCode, "NetworkGame:"):
		gameCode := strings.TrimPrefix(p.ServerCode, "NetworkGame:")
		ipAddress, chainInfo, err := loginNetworkGame(cli, gameCode, p.ClientPublicKey)
		if err != nil {
			return result, err
		}
		result.IP = ipAddress
		result.ChainInfo = chainInfo
	case p.ServerCode == "MainCity":
		ipAddress, chainInfo, err := loginMainCity(cli, p.ClientPublicKey)
		if err != nil {
			return result, err
		}
		result.IP = ipAddress
		result.ChainInfo = chainInfo
	case hasPrefixValue(p.ServerCode, "DomainGame:"):
		inviteCode := strings.TrimPrefix(p.ServerCode, "DomainGame:")
		ipAddress, chainInfo, err := loginDomainGame(cli, inviteCode, p.ClientPublicKey, false)
		if err != nil {
			return result, err
		}
		result.IP = ipAddress
		result.ChainInfo = chainInfo
	case hasPrefixValue(p.ServerCode, "PCDomainGame:"):
		inviteCode := strings.TrimPrefix(p.ServerCode, "PCDomainGame:")
		ipAddress, chainInfo, err := loginDomainGame(cli, inviteCode, p.ClientPublicKey, true)
		if err != nil {
			return result, err
		}
		result.IP = ipAddress
		result.ChainInfo = chainInfo
		result.IsPC = true
	case hasPrefixValue(p.ServerCode, "@"):
		// 本地联机（TAN）房间：必须先剥离 @ 标识符，用纯房间号查询进入。
		// 否则会把 "@45678" 整体当作房间号传给查询接口，导致"找不到房间"。
		roomID := strings.TrimSpace(strings.TrimPrefix(p.ServerCode, "@"))
		if roomID == "" {
			return result, fmt.Errorf("本地联机房间号为空")
		}
		tanResult, err := TanLobbyLogin(ctx, cli, TanLobbyLoginParams{RoomID: roomID})
		if err != nil {
			return result, err
		}
		result.TanLobby = &tanResult
		// TAN 房间没有租赁服的 IP/链信息，用信令地址占位；
		// 上层响应携带完整 TAN 字段（raknet/signaling 地址与密钥）。
		result.IP = tanResult.SignalingServerAddress
	default:
		ipAddress, chainInfo, err := loginRentalGame(cli, p.ServerCode, p.ServerPassword, p.ClientPublicKey)
		if err != nil {
			return result, err
		}
		result.IP = ipAddress
		result.ChainInfo = chainInfo
	}

	result.UID = cli.UserID
	if cli.UserDetail != nil {
		result.EntityID = cli.UserDetail.EntityID
		result.MasterName = cli.UserDetail.Name
		result.BotLevel = int(cli.UserDetail.Level.Int64())
	}
	if result.MasterName == "" {
		result.MasterName = cli.UserID
	}
	result.EngineVersion = cli.EngineVersion
	result.PatchVersion = cli.G79LatestVersion

	return result, nil
}

func ensureUserDetail(cli *g79.Client) error {
	if cli.UserDetail == nil {
		detail, err := cli.GetUserDetail()
		if err != nil {
			return fmt.Errorf("GetUserDetail: %w", err)
		}
		cli.UserDetail = &detail.Entity
	}
	if cli.UserDetail != nil && cli.UserDetail.Name == "" {
		name := fmt.Sprintf("AE%09d", randomNickname.Intn(1000000000))
		if err := cli.UpdateNickname(name); err != nil {
			return fmt.Errorf("UpdateNickname: %w", err)
		}
		if detail, err := cli.GetUserDetail(); err == nil {
			cli.UserDetail = &detail.Entity
		}
	}
	return nil
}

func loginLobbyGame(cli *g79.Client, roomCode, password, clientPublicKey string) (string, string, error) {
	roomCode, roomInfo, err := resolveLobbyRoom(cli, roomCode)
	if err != nil {
		return "", "", err
	}

	roomMap, err := cli.PurchaseItem(roomInfo.Entity.ResID.String())
	if err != nil {
		return "", "", fmt.Errorf("PurchaseItem: %w", err)
	}
	if !(roomMap.Code == 0 || roomMap.Code == 502 || roomMap.Code == 44) {
		return "", "", fmt.Errorf("PurchaseItem: %s(%d)", roomMap.Message, roomMap.Code)
	}

	var enterResp *g79.OnlineLobbyRoomEnterResponse
	for attempt := 1; attempt <= 3; attempt++ {
		enterResp, err = cli.EnterOnlineLobbyRoom(roomCode, password)
		if err != nil {
			return "", "", fmt.Errorf("EnterOnlineLobbyRoom: %w", err)
		}
		if enterResp.Code != 501 {
			break
		}
		if attempt < 3 {
			_, _ = cli.PurchaseItem(roomInfo.Entity.ResID.String())
			time.Sleep(500 * time.Millisecond)
		}
	}
	if enterResp.Code != 0 {
		return "", "", fmt.Errorf("EnterOnlineLobbyRoom: %s(%d)", enterResp.Message, enterResp.Code)
	}

	gameEnter, err := cli.OnlineLobbyGameEnter()
	if err != nil {
		return "", "", fmt.Errorf("OnlineLobbyGameEnter: %w", err)
	}
	if gameEnter.Code != 0 {
		return "", "", fmt.Errorf("OnlineLobbyGameEnter: %s(%d)", gameEnter.Message, gameEnter.Code)
	}

	authv2Data, err := cli.GenerateLobbyGameAuthV2(roomCode, clientPublicKey)
	if err != nil {
		return "", "", fmt.Errorf("GenerateLobbyGameAuthV2: %w", err)
	}
	chainInfo, err := cli.SendAuthV2Request(authv2Data)
	if err != nil {
		return "", "", fmt.Errorf("SendAuthV2Request: %w", err)
	}

	return gameEnter.Entity.BestAddr(), string(chainInfo), nil
}

func loginPCLobbyGame(ctx context.Context, cli *g79.Client, roomCode, password, clientPublicKey string, provider func(context.Context) (*g79.Client, error)) (string, string, error) {
	newCli, err := provider(ctx)
	if err != nil {
		return "", "", fmt.Errorf("NewClient: %w", err)
	}
	for {
		time.Sleep(time.Second)
		err = newCli.X19AuthenticateWithCookie(cli.Cookie)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "操作过于频繁") {
			return "", "", fmt.Errorf("X19AuthenticateWithCookie: %w", err)
		}
	}
	cli = newCli

	roomCode, roomInfo, err := resolveLobbyRoom(cli, roomCode)
	if err != nil {
		return "", "", err
	}

	roomMap, err := cli.UserItemPurchase(roomInfo.Entity.ResID.String())
	if err != nil {
		return "", "", fmt.Errorf("UserItemPurchase: %w", err)
	}
	if !(roomMap.Code == 0 || roomMap.Code == 502 || roomMap.Code == 44) {
		return "", "", fmt.Errorf("UserItemPurchase: %s(%d)", roomMap.Message, roomMap.Code)
	}

	var enterResp *g79.OnlineLobbyRoomEnterResponse
	for attempt := 1; attempt <= 5; attempt++ {
		enterResp, err = cli.EnterOnlineLobbyRoom(roomCode, password)
		if err != nil {
			return "", "", fmt.Errorf("EnterOnlineLobbyRoom: %w", err)
		}
		if enterResp.Code != 501 {
			break
		}
		if attempt < 5 {
			_, _ = cli.UserItemPurchase(roomInfo.Entity.ResID.String())
			time.Sleep(time.Second)
		}
	}
	if enterResp.Code != 0 {
		return "", "", fmt.Errorf("EnterOnlineLobbyRoom: %s(%d)", enterResp.Message, enterResp.Code)
	}

	gameEnter, err := cli.OnlineLobbyGameEnter()
	if err != nil {
		return "", "", fmt.Errorf("OnlineLobbyGameEnter: %w", err)
	}
	if gameEnter.Code != 0 {
		return "", "", fmt.Errorf("OnlineLobbyGameEnter: %s(%d)", gameEnter.Message, gameEnter.Code)
	}

	authv2Data, err := cli.GeneratePCLobbyGameAuthV2(roomInfo.Entity.ResID.String(), clientPublicKey)
	if err != nil {
		return "", "", fmt.Errorf("GeneratePCLobbyGameAuthV2: %w", err)
	}
	chainInfo, err := cli.SendAuthV2Request(authv2Data)
	if err != nil {
		return "", "", fmt.Errorf("SendAuthV2Request: %w", err)
	}

	return gameEnter.Entity.BestAddr(), string(chainInfo), nil
}

func loginNetworkGame(cli *g79.Client, gameCode, clientPublicKey string) (string, string, error) {
	serverAddress, err := cli.GetPeGameServerAddress(gameCode)
	if err != nil {
		return "", "", fmt.Errorf("GetPeGameServerAddress: %w", err)
	}
	if serverAddress.Code != 0 {
		return "", "", fmt.Errorf("GetPeGameServerAddress: %s(%d)", serverAddress.Message, serverAddress.Code)
	}

	authv2Data, err := cli.GenerateNetworkGameAuthV2(gameCode, clientPublicKey)
	if err != nil {
		return "", "", fmt.Errorf("GenerateNetworkGameAuthV2: %w", err)
	}
	chainInfo, err := cli.SendAuthV2Request(authv2Data)
	if err != nil {
		return "", "", fmt.Errorf("SendAuthV2Request: %w", err)
	}

	return fmt.Sprintf("%s:%d", serverAddress.Entity.IP, serverAddress.Entity.Port.Int64()), string(chainInfo), nil
}

func loginMainCity(cli *g79.Client, clientPublicKey string) (string, string, error) {
	_ = cli.LeaveEnteredGame()
	_, _ = cli.LeaveMainCity()

	mainCity, err := cli.EnterMainCity()
	if err != nil {
		return "", "", fmt.Errorf("EnterMainCity: %w", err)
	}
	if mainCity.Code != 0 {
		return "", "", fmt.Errorf("EnterMainCity: %s(%d)", mainCity.Message, mainCity.Code)
	}

	authv2Data, err := cli.GenerateLobbyGameAuthV2(fmt.Sprintf("%d", mainCity.Entity.CityNo), clientPublicKey)
	if err != nil {
		return "", "", fmt.Errorf("GenerateLobbyGameAuthV2: %w", err)
	}
	chainInfo, err := cli.SendAuthV2Request(authv2Data)
	if err != nil {
		return "", "", fmt.Errorf("SendAuthV2Request: %w", err)
	}

	return fmt.Sprintf("%s:%d", mainCity.Entity.ServerHost, mainCity.Entity.ServerPort), string(chainInfo), nil
}

func loginDomainGame(cli *g79.Client, inviteCode, clientPublicKey string, isPC bool) (string, string, error) {
	// 记录加入前的已有山头服 sid 集合，用于后续定位新服
	before := map[string]bool{}
	if resp, err := cli.GetOtherDomainServers(); err == nil {
		for _, s := range resp.Entities {
			before[s.Sid] = true
		}
	}

	inviteResp, err := cli.JoinDomainServerWithInviteCode(inviteCode)
	if err != nil {
		return "", "", fmt.Errorf("JoinDomainServerWithInviteCode: %w", err)
	}
	if inviteResp.Code != 0 {
		return "", "", fmt.Errorf("JoinDomainServerWithInviteCode: %s(%d)", inviteResp.Message, inviteResp.Code)
	}

	// 找到刚加入的新服 sid（不在 before 集合中的）
	var serverID string
	for retry := 0; retry < 5; retry++ {
		serversResp, err := cli.GetOtherDomainServers()
		if err != nil {
			if retry < 4 { time.Sleep(500 * time.Millisecond); continue }
			return "", "", fmt.Errorf("GetOtherDomainServers(after join): %w", err)
		}
		for _, s := range serversResp.Entities {
			if !before[s.Sid] {
				serverID = s.Sid
				break
			}
		}
		if serverID != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if serverID == "" {
		return "", "", fmt.Errorf("JoinDomainServerWithInviteCode: %s(%d)", inviteResp.Message, inviteResp.Code)
	}

	enterResp, err := cli.RequestEnterDomainServer(serverID)
	if err != nil {
		return "", "", fmt.Errorf("RequestEnterDomainServer(sid=%s): %w", serverID, err)
	}
	if enterResp == nil || enterResp.Code != 0 {
		msg := ""
		if enterResp != nil { msg = fmt.Sprintf("%s(%d)", enterResp.Message, enterResp.Code) }
		return "", "", fmt.Errorf("RequestEnterDomainServer(sid=%s): %s", serverID, msg)
	}
	ip := fmt.Sprintf("%s:%d", enterResp.Entity.ServerHost, enterResp.Entity.ServerPort.Int64())

	var authv2Data []byte
	if isPC {
		authv2Data, err = cli.GeneratePCDomainGameAuthV2(serverID, clientPublicKey)
		if err != nil {
			return "", "", fmt.Errorf("GeneratePCDomainGameAuthV2: %w", err)
		}
	} else {
		authv2Data, err = cli.GenerateDomainGameAuthV2(serverID, clientPublicKey)
		if err != nil {
			return "", "", fmt.Errorf("GenerateDomainGameAuthV2: %w", err)
		}
	}
	chainInfo, err := cli.SendAuthV2Request(authv2Data)
	if err != nil {
		return "", "", fmt.Errorf("SendAuthV2Request: %w", err)
	}

	_, _ = cli.RequestLeaveDomainServer(serverID)
	_, _ = cli.DeleteOtherDomainServer(serverID)
	return ip, string(chainInfo), nil
}

func loginRentalGame(cli *g79.Client, serverCode, password, clientPublicKey string) (string, string, error) {
	searchResp, err := cli.SearchRentalServerByName(serverCode)
	if err != nil {
		return "", "", fmt.Errorf("SearchRentalServerByName: %w", err)
	}
	if searchResp.Code != 0 {
		return "", "", fmt.Errorf("SearchRentalServerByName: %s(%d)", searchResp.Message, searchResp.Code)
	}
	if len(searchResp.Entities) == 0 {
		return "", "", fmt.Errorf("SearchRentalServerByName: 找不到服务器")
	}
	serverID := searchResp.Entities[0].EntityID

	enterResp, err := cli.EnterRentalServerWorld(serverID.String(), password)
	if err != nil {
		return "", "", fmt.Errorf("EnterRentalServerWorld: %w", err)
	}
	if enterResp.Code != 0 {
		return "", "", fmt.Errorf("EnterRentalServerWorld: %s(%d)", enterResp.Message, enterResp.Code)
	}
	ipAddress := fmt.Sprintf("%s:%d", enterResp.Entity.McserverHost, enterResp.Entity.McserverPort.Int64())

	authv2Data, err := cli.GenerateRentalGameAuthV2(serverID.String(), clientPublicKey)
	if err != nil {
		return "", "", fmt.Errorf("GenerateRentalGameAuthV2: %w", err)
	}
	chainInfo, err := cli.SendAuthV2Request(authv2Data)
	if err != nil {
		return "", "", fmt.Errorf("SendAuthV2Request: %w", err)
	}

	return ipAddress, string(chainInfo), nil
}

// ResolveRentalServerAddress 把租赁服号解析成可拨号的 IP:Port（host:port）。
// 仅取 loginRentalGame 的前两步（SearchRentalServerByName + EnterRentalServerWorld），
// 不做 GenerateRentalGameAuthV2/SendAuthV2Request —— 联机大厅中继只要可拨号地址，
// 不需要房主自身的认证链。
func ResolveRentalServerAddress(cli *g79.Client, serverCode, password string) (string, error) {
	searchResp, err := cli.SearchRentalServerByName(serverCode)
	if err != nil {
		return "", fmt.Errorf("SearchRentalServerByName: %w", err)
	}
	if searchResp.Code != 0 {
		return "", fmt.Errorf("SearchRentalServerByName: %s(%d)", searchResp.Message, searchResp.Code)
	}
	if len(searchResp.Entities) == 0 {
		return "", fmt.Errorf("SearchRentalServerByName: 找不到服务器")
	}
	serverID := searchResp.Entities[0].EntityID

	enterResp, err := cli.EnterRentalServerWorld(serverID.String(), password)
	if err != nil {
		return "", fmt.Errorf("EnterRentalServerWorld: %w", err)
	}
	if enterResp.Code != 0 {
		return "", fmt.Errorf("EnterRentalServerWorld: %s(%d)", enterResp.Message, enterResp.Code)
	}
	return fmt.Sprintf("%s:%d", enterResp.Entity.McserverHost, enterResp.Entity.McserverPort.Int64()), nil
}

func resolveLobbyRoom(cli *g79.Client, roomCode string) (string, *g79.OnlineLobbyRoomGetResponse, error) {
	if len(roomCode) != 19 {
		searchResp, err := cli.SearchOnlineLobbyRoomByKeyword(roomCode, 1, 0)
		if err != nil {
			return "", nil, fmt.Errorf("SearchOnlineLobbyRoomByKeyword: %w", err)
		}
		if searchResp.Code != 0 {
			return "", nil, fmt.Errorf("SearchOnlineLobbyRoomByKeyword: %s(%d)", searchResp.Message, searchResp.Code)
		}
		if len(searchResp.Entities) == 0 {
			return "", nil, fmt.Errorf("SearchOnlineLobbyRoomByKeyword: 找不到房间")
		}
		roomCode = searchResp.Entities[0].EntityID.String()
	}

	roomInfo, err := cli.GetOnlineLobbyRoom(roomCode)
	if err != nil {
		return "", nil, fmt.Errorf("GetOnlineLobbyRoom: %w", err)
	}
	if roomInfo.Code != 0 {
		return "", nil, fmt.Errorf("GetOnlineLobbyRoom: %s(%d)", roomInfo.Message, roomInfo.Code)
	}
	return roomCode, roomInfo, nil
}

func hasPrefixValue(raw, prefix string) bool {
	if !strings.HasPrefix(raw, prefix) {
		return false
	}
	return strings.TrimSpace(strings.TrimPrefix(raw, prefix)) != ""
}
