package minecraft

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptoRand "crypto/rand"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/internal"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/login"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

var dnetLogFile = "/storage/emulated/0/Download/prism_debug.log"

func dnetLog(format string, args ...any) {
	f, err := os.OpenFile(dnetLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	line := fmt.Sprintf("[%s] [DialNetConn] %s\n", ts, fmt.Sprintf(format, args...))
	_, _ = f.WriteString(line)
}

// DialNetConnContext 在已有的 net.Conn（如 TanLobby WebRTC 隧道）上进行 Minecraft 登录。
// 与 DialContext 不同，它不需要认证服务器返回的 chainInfo，
// 而是直接用离线登录（EncodeOffline）在隧道上完成握手。
func DialNetConnContext(ctx context.Context, netConn net.Conn, identityData login.IdentityData, clientData login.ClientData) (*Conn, error) {
	var d Dialer
	return d.DialNetConnContext(ctx, netConn, identityData, clientData)
}

func (d Dialer) DialNetConnContext(ctx context.Context, netConn net.Conn, identityData login.IdentityData, clientData login.ClientData) (*Conn, error) {
	dnetLog("=== DialNetConnContext 开始 ===")
	dnetLog("identityData: Uid=%d, DisplayName=%s", identityData.Uid, identityData.DisplayName)
	dnetLog("netConn type: %T, addr: %s", netConn, netConn.RemoteAddr())

	key, _ := ecdsa.GenerateKey(elliptic.P384(), cryptoRand.Reader)
	dnetLog("ECDSA key generated")

	if d.ErrorLog == nil {
		d.ErrorLog = slog.New(internal.DiscardHandler{})
	}
	d.ErrorLog = d.ErrorLog.With("src", "dialer")
	if d.Protocol == nil {
		d.Protocol = DefaultProtocol
	}
	if d.FlushRate == 0 {
		d.FlushRate = time.Second / 20
	}
	dnetLog("Protocol ID: %d", d.Protocol.ID())

	conn := newConn(netConn, key, d.ErrorLog, d.Protocol, d.FlushRate, false)
	conn.pool = conn.proto.Packets(false)
	conn.identityData = identityData
	conn.clientData = clientData
	conn.packetFunc = d.PacketFunc
	conn.downloadResourcePack = d.DownloadResourcePack
	conn.cacheEnabled = d.EnableClientCache
	conn.disconnectOnInvalidPacket = d.DisconnectOnInvalidPackets
	conn.disconnectOnUnknownPacket = d.DisconnectOnUnknownPackets
	conn.maxDecompressedLen = math.MaxInt

	defaultIdentityData(&conn.identityData)
	if conn.clientData.ServerAddress == "" {
		conn.clientData.ServerAddress = netConn.RemoteAddr().String()
	}
	defaultClientData(&conn.clientData, auth.AuthResponse{RentalServerIP: conn.clientData.ServerAddress})
	setAndroidData(&conn.clientData)

	dnetLog("ClientData: ServerAddress=%s, SkinID=%s", conn.clientData.ServerAddress, conn.clientData.SkinID)

	request := login.EncodeOffline(conn.identityData, conn.clientData, key, false)
	dnetLog("EncodeOffline 完成, request len=%d", len(request))

	parsedIdentityData, _, _, err := login.Parse(request)
	if err != nil {
		dnetLog("Parse identity data 失败: %v", err)
		return nil, fmt.Errorf("dial net conn: parse identity data: %w", err)
	}
	conn.identityData = parsedIdentityData
	dnetLog("Parse identity data 成功, DisplayName=%s", parsedIdentityData.DisplayName)

	readyForLogin, connected := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancelCause(ctx)
	go listenConn(conn, readyForLogin, connected, cancel)
	dnetLog("listenConn goroutine 已启动")

	conn.expect(packet.IDNetworkSettings, packet.IDPlayStatus)
	dnetLog("发送 RequestNetworkSettings (Protocol=%d)...", d.Protocol.ID())
	if err := conn.WritePacket(&packet.RequestNetworkSettings{ClientProtocol: d.Protocol.ID()}); err != nil {
		dnetLog("发送 RequestNetworkSettings 失败: %v", err)
		return nil, conn.wrap(fmt.Errorf("send request network settings: %w", err), "dial net conn")
	}
	_ = conn.Flush()
	dnetLog("RequestNetworkSettings 已发送, 等待 NetworkSettings 响应...")

	select {
	case <-ctx.Done():
		dnetLog("ctx.Done: %v", context.Cause(ctx))
		return nil, conn.wrap(context.Cause(ctx), "dial net conn")
	case <-conn.ctx.Done():
		dnetLog("conn.ctx.Done: %v", conn.closeErr("dial net conn"))
		return nil, conn.closeErr("dial net conn")
	case <-readyForLogin:
		dnetLog("收到 readyForLogin, 发送 Login 包...")
		conn.expect(packet.IDServerToClientHandshake, packet.IDPlayStatus)
		if err := conn.WritePacket(&packet.Login{ConnectionRequest: request, ClientProtocol: d.Protocol.ID()}); err != nil {
			dnetLog("发送 Login 包失败: %v", err)
			return nil, conn.wrap(fmt.Errorf("send login: %w", err), "dial net conn")
		}
		_ = conn.Flush()
		dnetLog("Login 包已发送, 等待 connected 信号...")

		select {
		case <-ctx.Done():
			dnetLog("ctx.Done (等待connected): %v", context.Cause(ctx))
			return nil, conn.wrap(context.Cause(ctx), "dial net conn")
		case <-conn.ctx.Done():
			dnetLog("conn.ctx.Done (等待connected): %v", conn.closeErr("dial net conn"))
			return nil, conn.closeErr("dial net conn")
		case <-connected:
			dnetLog("connected! EntityUniqueID=%d, EntityRuntimeID=%d", conn.GameData().EntityUniqueID, conn.GameData().EntityRuntimeID)
			return conn, nil
		}
	}
}