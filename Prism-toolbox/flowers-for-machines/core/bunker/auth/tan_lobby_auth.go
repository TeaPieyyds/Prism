package auth

import (
	"fmt"

	tanauth "github.com/Happy2018new/nemc-tan-lobby-solver/bunker"
)

// tanLobbyAuthenticator 实现了 nemc-tan-lobby-solver/bunker.Authenticator 接口，
// 用于 tanservice.DialContext 调用。
type tanLobbyAuthenticator struct {
	aw *AccessWrapper
}

// NewTanLobbyAuthenticator 创建一个 TanLobby 认证器，将 AccessWrapper 适配为
// nemc-tan-lobby-solver 所需的 Authenticator 接口。
func NewTanLobbyAuthenticator(aw *AccessWrapper) tanauth.Authenticator {
	return &tanLobbyAuthenticator{aw: aw}
}

func (a *tanLobbyAuthenticator) GetAccess(roomID string) (tanauth.TanLobbyLoginResponse, error) {
	response, err := a.aw.TanLobbyGetAccess(roomID)
	if err != nil {
		return tanauth.TanLobbyLoginResponse{}, err
	}
	return tanauth.TanLobbyLoginResponse{
		Success:   response.Success,
		ErrorInfo: response.ErrorInfo,

		UserUniqueID:   response.UserUniqueID,
		UserPlayerName: response.UserPlayerName,
		BotSkin: tanauth.PhoenixSkinInfo{
			ItemID:          response.BotSkin.ItemID,
			SkinDownloadURL: response.BotSkin.SkinDownloadURL,
			SkinIsSlim:      response.BotSkin.SkinIsSlim,
		},
		BotComponent: response.BotComponent,

		RoomOwnerID:        response.RoomOwnerID,
		RoomModDisplayName: response.RoomModDisplayName,
		RoomModDownloadURL: response.RoomModDownloadURL,
		RoomModEncryptKey:  response.RoomModEncryptKey,

		RaknetServerAddress: response.RaknetServerAddress,
		RaknetRand:          response.RaknetRand,
		RaknetAESRand:       response.RaknetAESRand,
		EncryptKeyBytes:     response.EncryptKeyBytes,
		DecryptKeyBytes:     response.DecryptKeyBytes,

		SignalingServerAddress: response.SignalingServerAddress,
		SignalingSeed:          response.SignalingSeed,
		SignalingTicket:        response.SignalingTicket,
	}, nil
}

func (a *tanLobbyAuthenticator) GetCreate() (tanauth.TanLobbyCreateResponse, error) {
	response, err := a.aw.TanLobbyGetCreate()
	if err != nil {
		return tanauth.TanLobbyCreateResponse{}, err
	}
	return tanauth.TanLobbyCreateResponse{
		Success:   response.Success,
		ErrorInfo: response.ErrorInfo,

		UserUniqueID:   response.UserUniqueID,
		UserPlayerName: response.UserPlayerName,

		RaknetServerAddress: response.RaknetServerAddress,
		RaknetRand:          response.RaknetRand,
		RaknetAESRand:       response.RaknetAESRand,
		EncryptKeyBytes:     response.EncryptKeyBytes,
		DecryptKeyBytes:     response.DecryptKeyBytes,

		SignalingServerAddress: response.SignalingServerAddress,
		SignalingSeed:          response.SignalingSeed,
		SignalingTicket:        response.SignalingTicket,
	}, nil
}

func (a *tanLobbyAuthenticator) GetDebug(loginResponse string, raknetRand []byte) (tanauth.TanLobbyDebugResponse, error) {
	return tanauth.TanLobbyDebugResponse{}, fmt.Errorf("TanLobbyDebug not implemented")
}