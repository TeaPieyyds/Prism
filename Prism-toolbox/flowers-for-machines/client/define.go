package client

import (
	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

// ------------------------- Config -------------------------

// Config ..
type Config struct {
	AuthServerAddress    string
	AuthServerToken      string
	RentalServerCode     string
	RentalServerPasscode string
	OnLoginPacket        func(packet.Packet) // 登录阶段每收到包时回调
	OnLoginProgress      func(string)        // 登录过程日志回调
}

// ------------------------- Client -------------------------

// Client ..
type Client struct {
	connection            *minecraft.Conn
	authClient            *auth.Client
	getCheckNumEverPassed bool
	cachedPacket          chan packet.Packet
	BotComponent          map[string]*int     // 服务器模组列表 (mod UUID → outfit type)
	OnLoginPacket         func(packet.Packet) // 登录阶段每收到包时回调
	OnLoginProgress       func(string)        // 登录过程进度回调
}

// Conn ..
func (c Client) Conn() *minecraft.Conn {
	return c.connection
}

// CachedPacket ..
func (c Client) CachedPacket() chan packet.Packet {
	return c.cachedPacket
}

func (c *Client) fireCallbacks(pk packet.Packet) {
	if c.OnLoginPacket != nil {
		c.OnLoginPacket(pk)
	}
	if c.OnLoginProgress != nil {
		c.OnLoginProgress("正在完成挑战...")
	}
}

// ------------------------- MCPCheckChallengesSolver -------------------------

// MCPCheckChallengesSolver ..
type MCPCheckChallengesSolver struct {
	client *Client
}

// NewChallengeSolver ..
func NewChallengeSolver(client *Client) *MCPCheckChallengesSolver {
	return &MCPCheckChallengesSolver{client: client}
}
