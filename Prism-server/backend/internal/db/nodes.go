package db

import (
	"database/sql"
	"time"
)

// Node 是一台贡献的 P2P 出口节点。
// 每个节点有一个 gost 隧道 tunnel_id 和一个协调者上的 SOCKS5 桥端口。
type Node struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	TokenID    *int64 `json:"token_id,omitempty"` // api_tokens.id；nil=主令牌贡献
	IsMaster   bool   `json:"is_master"`          // true=主令牌贡献
	TunnelID   string `json:"tunnel_id"`
	SocksPort  int    `json:"socks_port"`
	Status     string `json:"status"` // online/offline
	LastSeen   string `json:"last_seen,omitempty"`
	CreatedAt  string `json:"created_at"`
	_isMaster  int
	_tokenID   sql.NullInt64
}

func scanNode(s interface {
	Scan(dest ...any) error
}) (*Node, error) {
	var n Node
	err := s.Scan(&n.ID, &n.UserID, &n._tokenID, &n._isMaster, &n.TunnelID, &n.SocksPort, &n.Status, &n.LastSeen, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	n.IsMaster = n._isMaster != 0
	if n._tokenID.Valid {
		n.TokenID = &n._tokenID.Int64
	}
	return &n, nil
}

const nodeCols = `id, user_id, token_id, is_master, tunnel_id, socks_port, status, last_seen, created_at`

// CreateNode 登记一台新节点。tokenID 为 nil 表示主令牌贡献。
func CreateNode(userID int64, tokenID *int64, isMaster bool, tunnelID string, socksPort int) (*Node, error) {
	master := 0
	if isMaster {
		master = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := DB.Exec(
		`INSERT INTO nodes (user_id, token_id, is_master, tunnel_id, socks_port, status, last_seen, created_at)
		 VALUES (?, ?, ?, ?, ?, 'online', ?, ?)`,
		userID, tokenID, master, tunnelID, socksPort, now, now)
	if err != nil {
		return nil, err
	}
	return GetNodeByTunnelID(tunnelID)
}

// GetNodeByID 按主键取节点。
func GetNodeByID(id int64) (*Node, error) {
	return scanNode(DB.QueryRow(`SELECT `+nodeCols+` FROM nodes WHERE id = ?`, id))
}

// GetNodeByTunnelID 按 gost 隧道 UUID 取节点。
func GetNodeByTunnelID(tunnelID string) (*Node, error) {
	return scanNode(DB.QueryRow(`SELECT `+nodeCols+` FROM nodes WHERE tunnel_id = ?`, tunnelID))
}

// GetAllOnlineNodes 返回全部 online 节点。
func GetAllOnlineNodes() ([]*Node, error) {
	rows, err := DB.Query(`SELECT `+nodeCols+` FROM nodes WHERE status = 'online'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// GetOnlineNodesByUser 返回某用户 online 的节点。
func GetOnlineNodesByUser(userID int64) ([]*Node, error) {
	rows, err := DB.Query(`SELECT `+nodeCols+` FROM nodes WHERE user_id = ? AND status = 'online'`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// GetNodeByTokenID 返回某子令牌贡献的节点（子令牌一个令牌一台节点）。
func GetNodeByTokenID(tokenID int64) (*Node, error) {
	return scanNode(DB.QueryRow(`SELECT `+nodeCols+` FROM nodes WHERE token_id = ? AND status = 'online' LIMIT 1`, tokenID))
}

// GetLatestNodeByUser 返回该用户最近一台 master 节点（任意状态）。
// 供重注册时复用 tunnel_id：UUID 不变则协调者 ingress 无需变化，避免重启断开全部隧道。
func GetLatestNodeByUser(userID int64, isMaster bool) (*Node, error) {
	m := 0
	if isMaster {
		m = 1
	}
	return scanNode(DB.QueryRow(`SELECT `+nodeCols+` FROM nodes WHERE user_id = ? AND is_master = ? ORDER BY id DESC LIMIT 1`, userID, m))
}

// GetLatestNodeByTokenID 返回该子令牌最近一台节点（任意状态），供复用。
func GetLatestNodeByTokenID(tokenID int64) (*Node, error) {
	return scanNode(DB.QueryRow(`SELECT `+nodeCols+` FROM nodes WHERE token_id = ? ORDER BY id DESC LIMIT 1`, tokenID))
}

// UpdateNodePort 更新节点桥端口（复用节点时旧端口可能已被分配，需换新端口）。
func UpdateNodePort(tunnelID string, port int) error {
	_, err := DB.Exec(`UPDATE nodes SET socks_port = ? WHERE tunnel_id = ?`, port, tunnelID)
	return err
}

// ListAllNodePorts 返回全部已占用的桥端口（用于分配新端口）。
func ListAllNodePorts() ([]int, error) {
	rows, err := DB.Query(`SELECT socks_port FROM nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// ListOnlineNodePorts 只返回在线节点占用的桥端口（离线节点的端口可复用）。
func ListOnlineNodePorts() ([]int, error) {
	rows, err := DB.Query(`SELECT socks_port FROM nodes WHERE status = 'online'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// SetNodeStatus 更新节点 online/offline 状态。
func SetNodeStatus(tunnelID, status string) error {
	_, err := DB.Exec(`UPDATE nodes SET status = ? WHERE tunnel_id = ?`, status, tunnelID)
	return err
}

// TouchNodeSeen 刷新心跳时间。
func TouchNodeSeen(tunnelID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := DB.Exec(`UPDATE nodes SET last_seen = ?, status = 'online' WHERE tunnel_id = ?`, now, tunnelID)
	return err
}

// DeleteOfflineNodes 删除超过保留期（24 小时）的离线节点记录。
// 近期离线记录保留，供 App 重注册时复用 tunnel_id（避免换新 UUID 触发协调者重启）。
func DeleteOfflineNodes() (int64, error) {
	cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	res, err := DB.Exec(`DELETE FROM nodes WHERE status = 'offline' AND last_seen < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// GetOnlineNodeServingAccount 返回应服务指定账号的在线节点（affinity）。
// 规则：
//   - 账号 owner 有 online 主令牌节点 → 返回之（服务该用户全部账号）
//   - 否则该账号绑定了某个子令牌，且该子令牌有 online 节点 → 返回之
// 都不满足返回 nil（无亲和，走普通分配）。
func GetOnlineNodeServingAccount(accountID int64) (*Node, error) {
	acc, err := GetAccountByID(accountID)
	if err != nil {
		return nil, err
	}
	if acc.OwnerID == nil {
		return nil, nil // 共享账号无 owner，无亲和
	}
	ownerID := *acc.OwnerID
	nodes, err := GetOnlineNodesByUser(ownerID)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}
	// 主令牌节点优先（服务该用户全部账号）
	for _, n := range nodes {
		if n.IsMaster {
			return n, nil
		}
	}
	// 否则匹配账号绑定的子令牌
	for _, n := range nodes {
		if n.TokenID == nil {
			continue
		}
		var bound sql.NullInt64
		err := DB.QueryRow(`SELECT active_account_id FROM api_tokens WHERE id = ?`, *n.TokenID).Scan(&bound)
		if err != nil {
			continue
		}
		if bound.Valid && bound.Int64 == accountID {
			return n, nil
		}
	}
	return nil, nil
}
