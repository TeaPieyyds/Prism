package authsvc

type LoginParams struct {
	ServerCode      string
	ServerPassword  string
	ClientPublicKey string
}

type LoginResult struct {
	UID           string
	ChainInfo     string
	IP            string
	BotLevel      int
	MasterName    string
	BotComponent  map[string]*int
	EntityID      string
	EngineVersion string
	PatchVersion  string
	IsPC          bool
	// TanLobby 非空表示本次登录目标是本地联机（TAN）房间（@房间号），
	// 连接信息以 TAN 字段为准（raknet/signaling 地址与密钥）。
	TanLobby *TanLobbyLoginResult
}

type TanLobbyLoginParams struct {
	RoomID string
}

type TanLobbyLoginResult struct {
	RoomOwnerID            uint32
	UserUniqueID           uint32
	UserPlayerName         string
	BotLevel               int
	BotComponent           map[string]*int
	RaknetServerAddress    string
	RoomModDisplayName     []string
	RoomModDownloadURL     []string
	RoomModEncryptKey      [][]byte
	SignalingServerAddress string

	RaknetRand      []byte
	RaknetAESRand   []byte
	EncryptKeyBytes []byte
	DecryptKeyBytes []byte

	SignalingSeed   []byte
	SignalingTicket []byte
}

type TanLobbyCreateResult struct {
	UserUniqueID           uint32
	UserPlayerName         string
	RaknetServerAddress    string
	RaknetRand             []byte
	RaknetAESRand          []byte
	EncryptKeyBytes        []byte
	DecryptKeyBytes        []byte
	SignalingServerAddress string
	SignalingSeed          []byte
	SignalingTicket        []byte
}
