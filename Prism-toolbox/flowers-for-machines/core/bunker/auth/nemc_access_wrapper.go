package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

type AccessWrapper struct {
	ServerCode     string
	ServerPassword string
	Token          string
	Client         *Client
	Username       string
	Password       string
}

func NewAccessWrapper(Client *Client, ServerCode, ServerPassword, Token, username, password string) *AccessWrapper {
	return &AccessWrapper{
		Client:         Client,
		ServerCode:     ServerCode,
		ServerPassword: ServerPassword,
		Token:          Token,
		Username:       username,
		Password:       password,
	}
}

func (aw *AccessWrapper) GetAccess(ctx context.Context, publicKey []byte) (authResponse AuthResponse, err error) {
	pubKeyData := base64.StdEncoding.EncodeToString(publicKey)
	authResponse, err = aw.Client.Auth(ctx, aw.ServerCode, aw.ServerPassword, pubKeyData, aw.Token, aw.Username, aw.Password)
	if err != nil {
		return AuthResponse{}, err
	}
	if len(authResponse.FBToken) != 0 {
		homedir, err := os.UserHomeDir()
		if err != nil {
			homedir = os.TempDir()
		}
		fbconfigdir := filepath.Join(homedir, ".config", "fastbuilder")
		if err := os.MkdirAll(fbconfigdir, 0755); err != nil {
			fbconfigdir = filepath.Join(os.TempDir(), ".config", "fastbuilder")
			os.MkdirAll(fbconfigdir, 0755)
		}
		ptoken := filepath.Join(fbconfigdir, "fbtoken")
		token_file, err := os.OpenFile(ptoken, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err == nil {
			token_file.WriteString(authResponse.FBToken)
			token_file.Close()
		}
	}
	return
}

// TanLobbyGetAccess 通过认证服务器获取 TanLobby 房间的接入信息。
func (aw *AccessWrapper) TanLobbyGetAccess(roomID string) (TanLobbyLoginResponse, error) {
	response, err := aw.Client.TanLobbyAuth(roomID, aw.Token)
	if err != nil {
		return TanLobbyLoginResponse{}, fmt.Errorf("TanLobbyGetAccess: %v", err)
	}
	return response, nil
}

// TanLobbyGetCreate 通过认证服务器创建 TanLobby 房间。
func (aw *AccessWrapper) TanLobbyGetCreate() (TanLobbyCreateResponse, error) {
	response, err := aw.Client.TanLobbyCreate(aw.Token)
	if err != nil {
		return TanLobbyCreateResponse{}, fmt.Errorf("TanLobbyGetCreate: %v", err)
	}
	return response, nil
}
