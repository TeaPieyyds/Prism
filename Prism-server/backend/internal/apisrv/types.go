package apisrv

type LoginRequest struct {
	FBToken         string `json:"login_token,omitempty"`
	UserName        string `json:"username,omitempty"`
	Password        string `json:"password,omitempty"`
	ServerCode      string `json:"server_code"`
	ServerPassword  string `json:"server_passcode"`
	ClientPublicKey string `json:"client_public_key"`
}

type SkinInfo struct {
	ItemID          string `json:"entity_id"`
	SkinDownloadURL string `json:"res_url"`
	SkinIsSlim      bool   `json:"is_slim"`
}

type Message struct {
	Information string `json:"message,omitempty"`
	Translation int    `json:"translation,omitempty"`
}

type LoginResponse struct {
	SuccessStates bool   `json:"success"`
	ServerMessage string `json:"server_msg,omitempty"`
	Message
	BotLevel       int             `json:"growth_level"`
	BotSkin        SkinInfo        `json:"skin_info,omitempty"`
	BotComponent   map[string]*int `json:"outfit_info,omitempty"`
	FBToken        string          `json:"token"`
	MasterName     string          `json:"respond_to,omitempty"`
	RentalServerIP string          `json:"ip_address"`
	ChainInfo      string          `json:"chainInfo"`

	// ── 本地联机（TAN）房间专用字段：仅 @房间号 登录时填充 ──
	RaknetServerAddress    string   `json:"raknet_server_address,omitempty"`
	SignalingServerAddress string   `json:"signaling_server_address,omitempty"`
	RaknetRand             string   `json:"raknet_rand,omitempty"`
	RaknetAESRand          string   `json:"raknet_aes_rand,omitempty"`
	EncryptKeyBytes        string   `json:"encrypt_key_bytes,omitempty"`
	DecryptKeyBytes        string   `json:"decrypt_key_bytes,omitempty"`
	SignalingSeed          string   `json:"signaling_seed,omitempty"`
	SignalingTicket        string   `json:"signaling_ticket,omitempty"`
	RoomModDisplayName     []string `json:"room_mod_display_name,omitempty"`
	RoomModDownloadURL     []string `json:"room_mod_download_url,omitempty"`
	RoomOwnerID            uint32   `json:"room_owner_id,omitempty"`
	UserUniqueID           uint32   `json:"user_unique_id,omitempty"`
}

type TransferCheckNumRequest struct {
	Data          string `json:"data"`
	EngineVersion string `json:"engine_version,omitempty"`
	PatchVersion  string `json:"patch_version,omitempty"`
	IsPC          *bool  `json:"is_pc,omitempty"`
}

type TransferCheckNumResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Value   string `json:"value,omitempty"`
}

type TransferStartTypeResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type TanLobbyTransferServersResponse struct {
	Success          bool     `json:"success"`
	ErrorInfo        string   `json:"error_info"`
	RaknetServers    []string `json:"raknet_servers"`
	WebsocketServers []string `json:"websocket_servers"`
}

type TanLobbyLoginRequest struct {
	FBToken string `json:"login_token"`
	RoomID  string `json:"room_id"`
}

type TanLobbyLoginResponse struct {
	Success   bool   `json:"success"`
	ErrorInfo string `json:"error_info"`

	UserUniqueID   uint32          `json:"user_unique_id"`
	UserPlayerName string          `json:"user_player_name"`
	BotLevel       int             `json:"growth_level"`
	BotSkin        SkinInfo        `json:"skin_info"`
	BotComponent   map[string]*int `json:"outfit_info,omitempty"`

	RoomOwnerID        uint32   `json:"room_owner_id"`
	RoomModDisplayName []string `json:"room_mod_display_name,omitempty"`
	RoomModDownloadURL []string `json:"room_mod_download_url,omitempty"`
	RoomModEncryptKey  [][]byte `json:"room_mod_encrypt_key,omitempty"`

	RaknetServerAddress    string `json:"raknet_server_address"`
	RaknetRand             []byte `json:"raknet_rand"`
	RaknetAESRand          []byte `json:"raknet_aes_rand"`
	EncryptKeyBytes        []byte `json:"encrypt_key_bytes"`
	DecryptKeyBytes        []byte `json:"decrypt_key_bytes"`
	SignalingServerAddress string `json:"signaling_server_address"`
	SignalingSeed          []byte `json:"signaling_seed"`
	SignalingTicket        []byte `json:"signaling_ticket"`
}

type TanLobbyCreateRequest struct {
	FBToken string `json:"login_token"`
}

type TanLobbyCreateResponse struct {
	Success   bool   `json:"success"`
	ErrorInfo string `json:"error_info"`

	UserUniqueID           uint32 `json:"user_unique_id"`
	UserPlayerName         string `json:"user_player_name"`
	RaknetServerAddress    string `json:"raknet_server_address"`
	RaknetRand             []byte `json:"raknet_rand"`
	RaknetAESRand          []byte `json:"raknet_aes_rand"`
	EncryptKeyBytes        []byte `json:"encrypt_key_bytes"`
	DecryptKeyBytes        []byte `json:"decrypt_key_bytes"`
	SignalingServerAddress string `json:"signaling_server_address"`
	SignalingSeed          []byte `json:"signaling_seed"`
	SignalingTicket        []byte `json:"signaling_ticket"`
}
