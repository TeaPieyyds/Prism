package authsvc

import (
	"context"
	cryptoRand "crypto/rand"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	g79 "github.com/Yeah114/g79client"
	"github.com/Yeah114/g79client/utils"
)

func TanLobbyLogin(ctx context.Context, cli *g79.Client, p TanLobbyLoginParams) (TanLobbyLoginResult, error) {
	_ = ctx
	var result TanLobbyLoginResult

	// 兼容携带标识符的房间号（@房间号 / #房间号）：入房查询前统一剥离前缀。
	// 否则房间号带 @ / # 时查询接口会因号码不匹配返回"房间不存在"。
	p.RoomID = strings.TrimSpace(p.RoomID)
	p.RoomID = strings.TrimPrefix(p.RoomID, "@")
	p.RoomID = strings.TrimPrefix(p.RoomID, "#")
	p.RoomID = strings.TrimSpace(p.RoomID)
	if p.RoomID == "" {
		return result, fmt.Errorf("TanLobbyLogin: 房间号为空")
	}

	roomInfo, err := cli.GetTransferRoomWithName(p.RoomID)
	if err != nil {
		return result, fmt.Errorf("TanLobbyLogin: 查询房间(%s)失败: %w", p.RoomID, err)
	}
	if roomInfo.Code != 0 {
		return result, fmt.Errorf("TanLobbyLogin: 查询房间(%s)失败: %s(code=%d)", p.RoomID, roomInfo.Message, roomInfo.Code)
	}
	if len(roomInfo.List) == 0 {
		return result, fmt.Errorf("TanLobbyLogin: 房间(%s)不存在或已离线", p.RoomID)
	}

	if cli.UserDetail == nil {
		detail, err := cli.GetUserDetail()
		if err != nil {
			return result, fmt.Errorf("TanLobbyLogin: 获取玩家信息失败: %w", err)
		}
		cli.UserDetail = &detail.Entity
	}

	encryptedToken := utils.GetEncryptedToken(cli.UserToken)
	raknetRand := make([]byte, 16)
	if _, err = cryptoRand.Read(raknetRand); err != nil {
		return result, fmt.Errorf("TanLobbyLogin: 生成随机数失败: %w", err)
	}
	raknetAESRand, err := utils.AesECBEncrypt(raknetRand, encryptedToken)
	if err != nil {
		return result, fmt.Errorf("TanLobbyLogin: 加密失败: %w", err)
	}
	encryptKeyBytes := append(append(make([]byte, 0, len(encryptedToken)+len(raknetRand)), encryptedToken...), raknetRand...)
	decryptKeyBytes := append(append(make([]byte, 0, len(encryptedToken)+len(raknetRand)), raknetRand...), encryptedToken...)

	seed := make([]byte, 16)
	if _, err = cryptoRand.Read(seed); err != nil {
		return result, fmt.Errorf("TanLobbyLogin: 生成随机数失败: %w", err)
	}
	ticket, err := utils.AesECBEncrypt(seed, []byte(cli.UserToken))
	if err != nil {
		return result, fmt.Errorf("TanLobbyLogin: 加密失败: %w", err)
	}

	target := roomInfo.List[0]
	for _, candidate := range roomInfo.List {
		if candidate.RoomUniqueID == p.RoomID || candidate.RID.String() == p.RoomID {
			target = candidate
			break
		}
	}

	result.RoomOwnerID = uint32(target.HID.Int64())
	if cli.UserDetail != nil {
		result.UserPlayerName = cli.UserDetail.Name
		result.BotLevel = int(cli.UserDetail.Level.Int64())
	}

	userUniqueID, err := strconv.ParseInt(cli.UserID, 10, 64)
	if err != nil {
		return result, fmt.Errorf("TanLobbyLogin: 解析用户ID失败: %w", err)
	}
	result.UserUniqueID = uint32(userUniqueID)

	itemIDs := make([]string, 0, len(target.ItemIDs))
	for _, rawID := range target.ItemIDs {
		id := strings.TrimSpace(rawID.String())
		if id == "" || id == "0" {
			continue
		}
		itemIDs = append(itemIDs, id)
	}

	for _, itemID := range itemIDs {
		info, err := cli.GetDownloadInfo(itemID)
		if err != nil {
			return result, fmt.Errorf("TanLobbyLogin: 获取房间资源(%s)失败: %w", itemID, err)
		}
		result.RoomModDisplayName = append(result.RoomModDisplayName, itemID)
		result.RoomModDownloadURL = append(result.RoomModDownloadURL, info.Entity.ResURL)
		result.RoomModEncryptKey = append(result.RoomModEncryptKey, nil)
	}

	roomTransferServerID := int(target.SRV.Int64())
	if roomTransferServerID != 0 {
		servers, err := g79.GetGlobalG79TransferServers()
		if err != nil {
			return result, fmt.Errorf("TanLobbyLogin: 获取传输服务器列表失败: %w", err)
		}
		for _, entry := range servers {
			if int(entry.ID.Int64()) != roomTransferServerID {
				continue
			}
			if len(entry.Ports) > 0 {
				result.RaknetServerAddress = fmt.Sprintf("%s:%d", entry.IP, entry.Ports[0])
			}
			signalPort := entry.SignalWebPort.Int64()
			if signalPort > 0 {
				result.SignalingServerAddress = fmt.Sprintf("%s:%d", entry.IP, signalPort)
			}
			break
		}
	}

	if result.RaknetServerAddress == "" || result.SignalingServerAddress == "" {
		return result, fmt.Errorf("TanLobbyLogin: 无法解析房间(%s)的传输服务器地址", p.RoomID)
	}

	result.RaknetRand = raknetRand
	result.RaknetAESRand = raknetAESRand
	result.SignalingSeed = seed
	result.SignalingTicket = ticket
	result.EncryptKeyBytes = encryptKeyBytes
	result.DecryptKeyBytes = decryptKeyBytes

	return result, nil
}

func TanLobbyCreate(ctx context.Context, cli *g79.Client) (TanLobbyCreateResult, error) {
	_ = ctx
	var result TanLobbyCreateResult

	if cli == nil {
		return result, fmt.Errorf("TanLobbyCreate: nil client")
	}
	if cli.UserToken == "" {
		return result, fmt.Errorf("TanLobbyCreate: missing user token")
	}
	if cli.UserDetail == nil {
		detail, err := cli.GetUserDetail()
		if err != nil {
			return result, fmt.Errorf("TanLobbyCreate: GetUserDetail: %w", err)
		}
		cli.UserDetail = &detail.Entity
	}

	raknetAddr, signalingAddr, err := selectTransferServer(cli)
	if err != nil {
		return result, fmt.Errorf("TanLobbyCreate: %w", err)
	}

	encryptedToken := utils.GetEncryptedToken(cli.UserToken)
	raknetRand := make([]byte, 16)
	if _, err = cryptoRand.Read(raknetRand); err != nil {
		return result, fmt.Errorf("TanLobbyCreate: rand read: %w", err)
	}
	raknetAESRand, err := utils.AesECBEncrypt(raknetRand, encryptedToken)
	if err != nil {
		return result, fmt.Errorf("TanLobbyCreate: aes encrypt: %w", err)
	}
	if len(raknetAESRand) >= 16 {
		raknetAESRand = raknetAESRand[:16]
	}
	encryptKeyBytes := append(append(make([]byte, 0, len(encryptedToken)+len(raknetRand)), encryptedToken...), raknetRand...)
	decryptKeyBytes := append(append(make([]byte, 0, len(encryptedToken)+len(raknetRand)), raknetRand...), encryptedToken...)

	signalingSeed := make([]byte, 16)
	if _, err = cryptoRand.Read(signalingSeed); err != nil {
		return result, fmt.Errorf("TanLobbyCreate: rand read: %w", err)
	}
	signalingTicket, err := utils.AesECBEncrypt(signalingSeed, []byte(cli.UserToken))
	if err != nil {
		return result, fmt.Errorf("TanLobbyCreate: aes encrypt: %w", err)
	}
	if len(signalingTicket) >= 16 {
		signalingTicket = signalingTicket[:16]
	}

	uid, err := cli.GetUserIDInt()
	if err != nil {
		return result, fmt.Errorf("TanLobbyCreate: parse user id: %w", err)
	}
	playerName := cli.UserID
	if cli.UserDetail != nil && cli.UserDetail.Name != "" {
		playerName = cli.UserDetail.Name
	}

	result.UserUniqueID = uint32(uid)
	result.UserPlayerName = playerName
	result.RaknetServerAddress = raknetAddr
	result.RaknetRand = raknetRand
	result.RaknetAESRand = raknetAESRand
	result.EncryptKeyBytes = encryptKeyBytes
	result.DecryptKeyBytes = decryptKeyBytes
	result.SignalingServerAddress = signalingAddr
	result.SignalingSeed = signalingSeed
	result.SignalingTicket = signalingTicket
	return result, nil
}

func TransferServerList() ([]string, []string, error) {
	servers, err := g79.GetGlobalG79TransferServers()
	if err != nil {
		return nil, nil, err
	}
	var raknetServers []string
	var websocketServers []string
	for _, server := range servers {
		for _, port := range server.Ports {
			raknetServers = append(raknetServers, fmt.Sprintf("%s:%d", server.IP, port))
		}
		websocketServers = append(websocketServers, fmt.Sprintf("%s:%d", server.IP, server.SignalWebPort.Int64()))
	}
	return raknetServers, websocketServers, nil
}

type transferServerEntry struct {
	Status         int
	ServerIP       string
	SignalWebPort  int
	WebsocketPorts []int
}

func selectTransferServer(cli *g79.Client) (string, string, error) {
	_ = cli
	servers, err := g79.GetGlobalG79TransferServers()
	if err != nil {
		return "", "", fmt.Errorf("SelectTransferServer: %w", err)
	}
	var list []transferServerEntry
	for _, s := range servers {
		list = append(list, transferServerEntry{Status: int(s.Status.Int64()), ServerIP: s.IP, SignalWebPort: int(s.SignalWebPort.Int64()), WebsocketPorts: s.Ports})
	}
	available := make([]transferServerEntry, 0, len(list))
	for _, entry := range list {
		if len(entry.WebsocketPorts) == 0 || entry.ServerIP == "" || entry.SignalWebPort == 0 {
			continue
		}
		available = append(available, entry)
	}
	if len(available) == 0 {
		return "", "", fmt.Errorf("SelectTransferServer: no available server")
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	selected := available[rng.Intn(len(available))]
	port := selected.WebsocketPorts[rng.Intn(len(selected.WebsocketPorts))]
	return fmt.Sprintf("%s:%d", selected.ServerIP, port), fmt.Sprintf("%s:%d", selected.ServerIP, selected.SignalWebPort), nil
}
