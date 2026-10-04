package minecraft

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptoRand "crypto/rand"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"errors"
	"fmt"
	"log/slog"
	"image/png"
	"math"
	mathRand "math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/internal"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/login"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
)

// PhoenixBuilder specific interface.
// Author: LNSSPsd, Liliya233, Happy2018new
type Authenticator interface {
	GetAccess(ctx context.Context, publicKey []byte) (auth.AuthResponse, error)
}

// Dialer allows specifying specific settings for connection to a Minecraft server.
// The zero value of Dialer is used for the package level Dial function.
type Dialer struct {
	// ErrorLog is a log.Logger that errors that occur during packet handling of
	// servers are written to. By default, errors are not logged.
	ErrorLog *slog.Logger

	// ClientData is the client data used to login to the server with. It includes fields such as the skin,
	// locale and UUIDs unique to the client. If empty, a default is sent produced using defaultClientData().
	ClientData login.ClientData
	// IdentityData is the identity data used to login to the server with. It includes the username, UUID and
	// XUID of the player.
	// The IdentityData object is obtained using Minecraft auth if Email and Password are set. If not, the
	// object provided here is used, or a default one if left empty.
	IdentityData login.IdentityData

	// PhoenixBuilder specific changes.
	// Author: LNSSPsd
	//
	// Authenticator towards netease's server
	Authenticator

	// PacketFunc is called whenever a packet is read from or written to the connection returned when using
	// Dialer.Dial(). It includes packets that are otherwise covered in the connection sequence, such as the
	// Login packet. The function is called with the header of the packet and its raw payload, the address
	// from which the packet originated, and the destination address.
	PacketFunc func(header packet.Header, payload []byte, src, dst net.Addr)

	// DownloadResourcePack is called individually for every texture and behaviour pack sent by the connection when
	// using Dialer.Dial(), and can be used to stop the pack from being downloaded. The function is called with the UUID
	// and version of the resource pack, the number of the current pack being downloaded, and the total amount of packs.
	// The boolean returned determines if the pack will be downloaded or not.
	DownloadResourcePack func(id uuid.UUID, version string, current, total int) bool

	// DisconnectOnUnknownPackets specifies if the connection should disconnect if packets received are not present
	// in the packet pool. If true, such packets lead to the connection being closed immediately.
	// If set to false, the packets will be returned as a packet.Unknown.
	DisconnectOnUnknownPackets bool

	// DisconnectOnInvalidPackets specifies if invalid packets (either too few bytes or too many bytes) should be
	// allowed. If true, such packets lead to the connection being closed immediately. If false,
	// packets with too many bytes will be returned while packets with too few bytes will be skipped.
	DisconnectOnInvalidPackets bool

	// Protocol is the Protocol version used to communicate with the target server. By default, this field is
	// set to the current protocol as implemented in the minecraft/protocol package. Note that packets written
	// to and read from the Conn are always any of those found in the protocol/packet package, as packets
	// are converted from and to this Protocol.
	Protocol Protocol

	// FlushRate is the rate at which packets sent are flushed. Packets are buffered for a duration up to
	// FlushRate and are compressed/encrypted together to improve compression ratios. The lower this
	// time.Duration, the lower the latency but the less efficient both network and cpu wise.
	// The default FlushRate (when set to 0) is time.Second/20. If FlushRate is set negative, packets
	// will not be flushed automatically. In this case, calling `(*Conn).Flush()` is required after any
	// calls to `(*Conn).Write()` or `(*Conn).WritePacket()` to send the packets over network.
	FlushRate time.Duration

	// EnableClientCache, if set to true, enables the client blob cache for the client. This means that the
	// server will send chunks as blobs, which may be saved by the client so that chunks don't have to be
	// transmitted every time, resulting in less network transmission.
	EnableClientCache bool

	// KeepXBLIdentityData, if set to true, enables passing XUID and title ID to the target server
	// if the authentication token is not set. This is technically not valid and some servers might kick
	// the client when an XUID is present without logging in.
	// For getting this to work with BDS, authentication should be disabled.
	KeepXBLIdentityData bool

	// EnableLegacyAuth, if set to true, will use the legacy authentication behavior
	// (pre-1.21.90) when connecting to the server. This should only be used for outdated
	// servers, as enabling it will cause compatibility issues with updated servers.
	EnableLegacyAuth bool
}

/*
PhoenixBuilder specific changes.
Author: Happy2018new

Dial dials a Minecraft connection to the address passed over the network passed. The network is typically
"raknet". A Conn is returned which may be used to receive packets from and send packets to.

A zero value of a Dialer struct is used to initiate the connection. A custom Dialer may be used to specify
additional behaviour.
*/
func Dial(network string) (*Conn, auth.AuthResponse, error) {
	// func Dial(network, address string) (*Conn, error) {
	var d Dialer
	return d.Dial(network)
}

// PhoenixBuilder specific changes.
// Author: Happy2018new
//
// DialTimeout dials a Minecraft connection to the address passed over the network passed. The network is
// typically "raknet". A Conn is returned which may be used to receive packets from and send packets to.
// If a connection is not established before the timeout ends, DialTimeout returns an error.
// DialTimeout uses a zero value of Dialer to initiate the connection.
func DialTimeout(network string, timeout time.Duration) (*Conn, auth.AuthResponse, error) {
	// DialTimeout(network, address string, timeout time.Duration) (*Conn, error)
	var d Dialer
	return d.DialTimeout(network, timeout)
}

// PhoenixBuilder specific changes.
// Author: Happy2018new
//
// DialContext dials a Minecraft connection to the address passed over the network passed. The network is
// typically "raknet". A Conn is returned which may be used to receive packets from and send packets to.
// If a connection is not established before the context passed is cancelled, DialContext returns an error.
// DialContext uses a zero value of Dialer to initiate the connection.
func DialContext(ctx context.Context, network string) (*Conn, auth.AuthResponse, error) {
	// func DialContext(ctx context.Context, network, address string) (*Conn, error) {
	var d Dialer
	return d.DialContext(ctx, network)
}

// PhoenixBuilder specific changes.
// Author: Happy2018new
//
// Dial dials a Minecraft connection to the address passed over the network passed. The network is typically
// "raknet". A Conn is returned which may be used to receive packets from and send packets to.
func (d Dialer) Dial(network string) (*Conn, auth.AuthResponse, error) {
	// func (d Dialer) Dial(network, address string) (*Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	return d.DialContext(ctx, network)
}

// PhoenixBuilder specific changes.
// Author: Happy2018new
//
// DialTimeout dials a Minecraft connection to the address passed over the network passed. The network is
// typically "raknet". A Conn is returned which may be used to receive packets from and send packets to.
// If a connection is not established before the timeout ends, DialTimeout returns an error.
func (d Dialer) DialTimeout(network string, timeout time.Duration) (*Conn, auth.AuthResponse, error) {
	// func (d Dialer) DialTimeout(network, address string, timeout time.Duration) (*Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return d.DialContext(ctx, network)
}

// PhoenixBuilder specific func, which modified from orgin version.
// Author: LNSSPsd, CMA2401PT, Liliya233, Happy2018new
//
// DialContext dials a Minecraft connection to the address passed over the network passed. The network is
// typically "raknet". A Conn is returned which may be used to receive packets from and send packets to.
// If a connection is not established before the context passed is cancelled, DialContext returns an error.
func (d Dialer) DialContext(ctx context.Context, network string) (conn *Conn, authResponse auth.AuthResponse, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), cryptoRand.Reader)
	if err != nil {
		return nil, auth.AuthResponse{}, &net.OpError{Op: "dial", Net: "minecraft", Err: fmt.Errorf("generating ECDSA key: %w", err)}
	}
	armoured_key, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, auth.AuthResponse{}, &net.OpError{Op: "dial", Net: "minecraft", Err: fmt.Errorf("marshalling PKIX public key: %w", err)}
	}

	authResponse, err = d.Authenticator.GetAccess(ctx, armoured_key)
	if err != nil {
		return nil, auth.AuthResponse{}, err
	}
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

	n, ok := networkByID(network, d.ErrorLog)
	if !ok {
		return nil, auth.AuthResponse{}, &net.OpError{Op: "dial", Net: "minecraft", Err: fmt.Errorf("dial: no network under id %v", network)}
	}

	/*
		Delete by Liliya233.

		var pong []byte
		var netConn net.Conn
		if pong, err = n.PingContext(ctx, address); err == nil {
			netConn, err = n.DialContext(ctx, addressWithPongPort(pong, address))
		} else {
			netConn, err = n.DialContext(ctx, address)
		}
	*/

	// PhoenixBuilder specific changes.
	// Author: CoozillaX
	//
	// Netease: pharos speed up
	var netConn net.Conn
	if addr, err := getPharosSpeedUpIP(authResponse.RentalServerIP); err == nil {
		authResponse.RentalServerIP = addr
	}
	netConn, err = n.DialContext(ctx, authResponse.RentalServerIP)
	if err != nil {
		return nil, auth.AuthResponse{}, err
	}

	conn = newConn(netConn, key, d.ErrorLog, d.Protocol, d.FlushRate, false)
	conn.pool = conn.proto.Packets(false)
	conn.identityData = d.IdentityData
	conn.clientData = d.ClientData
	conn.packetFunc = d.PacketFunc
	conn.downloadResourcePack = d.DownloadResourcePack
	conn.cacheEnabled = d.EnableClientCache
	conn.disconnectOnInvalidPacket = d.DisconnectOnInvalidPackets
	conn.disconnectOnUnknownPacket = d.DisconnectOnUnknownPackets
	conn.maxDecompressedLen = math.MaxInt

	defaultIdentityData(&conn.identityData)
	defaultClientData(&conn.clientData, authResponse)

	var request []byte
	// We login as an Android device and this will show up in the 'titleId' field in the JWT chain, which
	// we can't edit. We just enforce Android data for logging in.
	setAndroidData(&conn.clientData)

	request = login.Encode(authResponse.ChainInfo, conn.clientData, key, d.EnableLegacyAuth)
	identityData, _, _, err := login.Parse(request)
	if err != nil {
		fmt.Printf("WARNING: Identity data parsing error: %v\n", err)
	}
	// If we got the identity data from Minecraft auth, we need to make sure we set it in the Conn too, as
	// we are not aware of the identity data ourselves yet.
	conn.identityData = identityData

	readyForLogin, connected := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancelCause(ctx)
	go listenConn(conn, readyForLogin, connected, cancel)

	conn.expect(packet.IDNetworkSettings, packet.IDPlayStatus)
	if err := conn.WritePacket(&packet.RequestNetworkSettings{ClientProtocol: d.Protocol.ID()}); err != nil {
		return nil, auth.AuthResponse{}, conn.wrap(fmt.Errorf("send request network settings: %w", err), "dial")
	}
	_ = conn.Flush()

	select {
	case <-ctx.Done():
		return nil, auth.AuthResponse{}, conn.wrap(context.Cause(ctx), "dial")
	case <-conn.ctx.Done():
		return nil, auth.AuthResponse{}, conn.closeErr("dial")
	case <-readyForLogin:
		// We've received our network settings, so we can now send our login request.
		conn.expect(packet.IDServerToClientHandshake, packet.IDPlayStatus)
		if err := conn.WritePacket(&packet.Login{ConnectionRequest: request, ClientProtocol: d.Protocol.ID()}); err != nil {
			return nil, auth.AuthResponse{}, conn.wrap(fmt.Errorf("send login: %w", err), "dial")
		}
		_ = conn.Flush()

		select {
		case <-ctx.Done():
			return nil, auth.AuthResponse{}, conn.wrap(context.Cause(ctx), "dial")
		case <-conn.ctx.Done():
			return nil, auth.AuthResponse{}, conn.closeErr("dial")
		case <-connected:
			// We've connected successfully. We return the connection and no error.
			return conn, auth.AuthResponse{}, nil
		}
	}
}

// readChainIdentityData reads a login.IdentityData from the Mojang chain
// obtained through authentication.
func readChainIdentityData(chainData []byte) (login.IdentityData, error) {
	chain := struct{ Chain []string }{}
	if err := json.Unmarshal(chainData, &chain); err != nil {
		return login.IdentityData{}, fmt.Errorf("read chain: read json: %w", err)
	}
	data := chain.Chain[1]
	claims := struct {
		ExtraData login.IdentityData `json:"extraData"`
	}{}
	tok, err := jwt.ParseSigned(data, []jose.SignatureAlgorithm{jose.ES384})
	if err != nil {
		return login.IdentityData{}, fmt.Errorf("read chain: parse jwt: %w", err)
	}
	if err := tok.UnsafeClaimsWithoutVerification(&claims); err != nil {
		return login.IdentityData{}, fmt.Errorf("read chain: read claims: %w", err)
	}
	if claims.ExtraData.Identity == "" {
		return login.IdentityData{}, fmt.Errorf("read chain: no extra data found")
	}
	return claims.ExtraData, nil
}

// listenConn listens on the connection until it is closed on another goroutine. The channel passed will
// receive a value once the connection is logged in.
func listenConn(conn *Conn, readyForLogin, connected chan struct{}, cancel context.CancelCauseFunc) {
	defer func() {
		_ = conn.Close()
	}()
	cancelContext := true
	for {
		// We finally arrived at the packet decoding loop. We constantly decode packets that arrive
		// and push them to the Conn so that they may be processed.
		packets, err := conn.dec.Decode()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				if cancelContext {
					cancel(err)
				} else {
					conn.log.Error(err.Error())
				}
			}
			return
		}
		for _, data := range packets {
			loggedInBefore, readyToLoginBefore := conn.loggedIn, conn.readyToLogin
			if err := conn.receive(data); err != nil {
				if cancelContext {
					cancel(err)
				} else {
					conn.log.Error(err.Error())
				}
				return
			}
			if !readyToLoginBefore && conn.readyToLogin {
				// This is the signal that the connection is ready to login, so we put a value in the channel so that
				// it may be detected.
				readyForLogin <- struct{}{}
			}
			if !loggedInBefore && conn.loggedIn {
				// This is the signal that the connection was considered logged in, so we put a value in the channel so
				// that it may be detected.
				cancelContext = false
				connected <- struct{}{}
			}
		}
	}
}

//go:embed skin_resource_patch.json
var skinResourcePatch []byte

//go:embed skin_geometry.json
var skinGeometry []byte

// PhoenixBuilder specific changes.
// Author: Happy2018new
//
// defaultClientData edits the ClientData passed to have defaults set to all fields that were left unchanged.
func defaultClientData(
	// PhoenixBuilder specific changes.
	// Author: Liliya233, Happy2018new
	d *login.ClientData,
	authResponse auth.AuthResponse,
	// address, username string, d *login.ClientData,
) {
	d.ServerAddress = authResponse.RentalServerIP
	d.DeviceOS = protocol.DeviceAndroid
	d.DefaultInputMode = packet.InputModeTouch
	d.CurrentInputMode = packet.InputModeTouch
	d.GameVersion = protocol.CurrentVersion
	d.ClientRandomID = mathRand.Int63()
	d.DeviceID = uuid.New().String()
	d.LanguageCode = "zh_CN" // Netease
	d.AnimatedImageData = make([]login.SkinAnimation, 0)
	d.PersonaPieces = make([]login.PersonaPiece, 0)
	d.PieceTintColours = make([]login.PersonaPieceTintColour, 0)
	d.SelfSignedID = uuid.New().String()
	d.SkinID = uuid.New().String()
	d.ArmSize = "wide"
	d.SkinColour = "#b37b62"
	d.PremiumSkin = false
	d.PersonaSkin = false
	d.TrustedSkin = true
	d.OverrideSkin = true
	d.UIProfile = 0
	d.CapeOnClassicSkin = false
	d.CompatibleWithClientSideChunkGen = true
	d.IsEditorMode = false
	d.GrowthLevel = authResponse.BotLevel
	// 与 NexusEgo 对齐：SkinItemID（entity_id）随登录 JWT ClientData 发送给服务器
	d.SkinItemID = authResponse.BotSkin.ItemID
	// 与 NexusEgo 对齐：设置 SkinGeometryVersion
	d.SkinGeometryVersion = base64.StdEncoding.EncodeToString([]byte("0.0.0"))

	// 从验证服务器下载皮肤，失败时回退到内置默认皮肤
	skinLoaded := false
	if authResponse.BotSkin.SkinDownloadURL != "" {
		url := authResponse.BotSkin.SkinDownloadURL
		isSlim := authResponse.BotSkin.SkinIsSlim
		skinDataStr, _, patch, w, h, err := fetchSkin(url, isSlim)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[skin] 下载失败 %s: %v，回退到默认皮肤\n", url, err)
		} else {
			d.SkinData = skinDataStr
			// 使用完整的 skin_geometry.json，只更新贴图尺寸
			geoJSON := updateGeometryDimensions(string(skinGeometry), w, h)
			d.SkinGeometry = base64.StdEncoding.EncodeToString([]byte(geoJSON))
			d.SkinResourcePatch = patch
			d.SkinImageWidth = w
			d.SkinImageHeight = h
			if isSlim {
				d.ArmSize = "slim"
			}
			skinLoaded = true
		}
	}
	if !skinLoaded {
		// 内置默认皮肤 (64x32 Steve 风格 RGBA)
		d.SkinData = base64.StdEncoding.EncodeToString(generateDefaultSkinRGBA())
		d.SkinImageWidth = 64
		d.SkinImageHeight = 32
		d.SkinGeometry = base64.StdEncoding.EncodeToString(skinGeometry)
		d.SkinResourcePatch = base64.StdEncoding.EncodeToString(skinResourcePatch)
	}
	// 与 NexusEgo 对齐：皮肤下载成功时 PremiumSkin=true，失败时 false
	d.PremiumSkin = skinLoaded

	{
		id := make([]byte, 8)
		_, _ = cryptoRand.Read(id)
		d.PlayFabID = hex.EncodeToString(id)
	}
}

// setAndroidData ensures the login.ClientData passed matches settings you would see on an Android device.
func setAndroidData(data *login.ClientData) {
	data.DeviceOS = protocol.DeviceAndroid
	data.GameVersion = protocol.CurrentVersion
}

// clearXBLIdentityData clears data from the login.IdentityData that is only set when a player is logged into
// XBOX Live.
func clearXBLIdentityData(data *login.IdentityData) {
	data.XUID = ""
	data.TitleID = ""
}

// defaultIdentityData edits the IdentityData passed to have defaults set to all fields that were left
// unchanged.
func defaultIdentityData(data *login.IdentityData) {
	if data.Identity == "" {
		data.Identity = uuid.New().String()
	}
	if data.DisplayName == "" {
		data.DisplayName = "Steve"
	}
}

// splitPong splits the pong data passed by ;, taking into account escaping these.
func splitPong(s string) []string {
	var runes []rune
	var tokens []string
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\\':
			inEscape = true
		case r == ';':
			tokens = append(tokens, string(runes))
			runes = runes[:0]
		case inEscape:
			inEscape = false
			fallthrough
		default:
			runes = append(runes, r)
		}
	}
	return append(tokens, string(runes))
}

// addressWithPongPort parses the redirect IPv4 port from the pong and returns the address passed with the port
// found if present, or the original address if not.
func addressWithPongPort(pong []byte, address string) string {
	frag := splitPong(string(pong))
	if len(frag) > 10 {
		portStr := frag[10]
		port, err := strconv.Atoi(portStr)
		// Vanilla (realms, in particular) will sometimes send port 19132 when you ping a port that isn't 19132 already,
		// but we should ignore that.
		if err != nil || port == 19132 {
			return address
		}
		// Remove the port from the address.
		addressParts := strings.Split(address, ":")
		address = strings.Join(strings.Split(address, ":")[:len(addressParts)-1], ":")
		return address + ":" + portStr
	}
	return address
}


// updateGeometryDimensions 更新皮肤几何体 JSON 中的贴图尺寸。
func updateGeometryDimensions(geoJSON string, w, h int) string {
	// 替换 texture_height 和 texture_width
	geoJSON = regexp.MustCompile(`"texture_height":\s*\d+`).ReplaceAllString(geoJSON, fmt.Sprintf(`"texture_height":%d`, h))
	geoJSON = regexp.MustCompile(`"texture_width":\s*\d+`).ReplaceAllString(geoJSON, fmt.Sprintf(`"texture_width":%d`, w))
	// 更新 identifier 中的 slim 标记
	return geoJSON
}

func fetchSkin(url string, isSlim bool) (skinData, geometry, resourcePatch string, width, height int, err error) {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Resolver: &net.Resolver{
					PreferGo: true,
					Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
						return (&net.Dialer{}).DialContext(ctx, "udp", "8.8.8.8:53")
					},
				},
			}).DialContext,
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return "", "", "", 0, 0, fmt.Errorf("download skin: %w", err)
	}
	defer resp.Body.Close()

	img, err := png.Decode(resp.Body)
	if err != nil {
		return "", "", "", 0, 0, fmt.Errorf("decode skin PNG: %w", err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	pixels := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			idx := (y*w + x) * 4
			pixels[idx] = byte(r >> 8)
			pixels[idx+1] = byte(g >> 8)
			pixels[idx+2] = byte(b >> 8)
			pixels[idx+3] = byte(a >> 8)
		}
	}
	skinData = base64.StdEncoding.EncodeToString(pixels)

	geomID := "geometry.humanoid"
	if isSlim {
		geomID = "geometry.humanoid.customSlim"
	}
	geometry = base64.StdEncoding.EncodeToString(
		[]byte(fmt.Sprintf(`{"format_version":"1.8.0","minecraft:geometry":[{"description":{"identifier":"%s","texture_height":%d,"texture_width":%d,"visible_bounds_offset":[0,0,0],"visible_bounds_height":1.5,"visible_bounds_width":1},"bones":[{"name":"body","pivot":[0,24,0]},{"name":"leftArm","pivot":[5,22,0],"cubes":[{"origin":[4,12,-2],"size":[4,12,4],"uv":[40,16],"mirror":false}]},{"name":"rightArm","pivot":[-5,22,0],"cubes":[{"origin":[-8,12,-2],"size":[4,12,4],"uv":[32,48],"mirror":false},{"origin":[-8,12,-2],"size":[4,12,4],"uv":[40,16]}]},{"name":"head","pivot":[0,24,0],"cubes":[{"origin":[-4,0,-4],"size":[8,8,8],"uv":[0,0]},{"origin":[-4,0,-4],"size":[8,8,8],"uv":[32,0]}]}%s]}]}`,
			geomID, h, w, slimArmAdjust(isSlim))))

	resourcePatch = base64.StdEncoding.EncodeToString(
		[]byte(fmt.Sprintf(`{"geometry":{"default":"%s"}}`, geomID)))

	return skinData, geometry, resourcePatch, w, h, nil
}

func slimArmAdjust(isSlim bool) string {
	if isSlim {
		return `,{"name":"leftArmSlim","pivot":[5,22,0],"cubes":[{"origin":[4,12,-2],"size":[3,12,4],"uv":[32,48],"mirror":false}]},{"name":"rightArmSlim","pivot":[-5,22,0],"cubes":[{"origin":[-7,12,-2],"size":[3,12,4],"uv":[40,16],"mirror":false}]}`
	}
	return ""
}

// generateDefaultSkinRGBA 生成纯黑 64x32 默认皮肤（与 NexusEgo 对齐）
func generateDefaultSkinRGBA() []byte {
	w, h := 64, 32
	pixels := make([]byte, w*h*4)
	for i := 0; i < len(pixels); i += 4 {
		pixels[i] = 0     // R
		pixels[i+1] = 0   // G
		pixels[i+2] = 0   // B
		pixels[i+3] = 255 // A
	}
	return pixels
}
