package db

import (
	"database/sql"
	"strings"
	"time"
)

type AuditLog struct {
	ID          int64  `json:"id"`
	UserID      *int64 `json:"user_id,omitempty"`
	Username    string `json:"username,omitempty"`
	AccountID   *int64 `json:"account_id,omitempty"`
	AccountName string `json:"account_name,omitempty"`
	Action      string `json:"action"`
	Target      string `json:"target"`
	Detail      string `json:"detail"`
	IP          string `json:"ip"`
	CreatedAt   string `json:"created_at"`
}

type SystemLog struct {
	ID        int64  `json:"id"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

func AddAuditLog(userID *int64, accountID *int64, action, target, detail, ip string, tokenID ...int64) error {
	tid := int64(0)
	if len(tokenID) > 0 {
		tid = tokenID[0]
	}
	_, err := DB.Exec(
		`INSERT INTO audit_logs (user_id, account_id, token_id, action, target, detail, ip, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, accountID, tid, action, target, detail, ip, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// LogFilter holds optional search/filter criteria for log listing.
type LogFilter struct {
	Keyword  string // fuzzy match against detail/target/username/account/ip/message
	Action   string // exact action (audit) or level (system)
	DateFrom string // RFC3339 inclusive lower bound on created_at
	DateTo   string // RFC3339 inclusive upper bound on created_at
	UserID   *int64 // restrict to a single user (audit only)
}

func ListAuditLogs(userID *int64, limit int, offset int, f LogFilter) ([]AuditLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	base := `SELECT l.id, l.user_id, COALESCE(u.username, ''), l.account_id, COALESCE(a.display_name, ''), l.action, l.target, l.detail, l.ip, l.created_at
		FROM audit_logs l
		LEFT JOIN users u ON u.id = l.user_id
		LEFT JOIN game_accounts a ON a.id = l.account_id`

	where := []string{}
	args := []any{}
	if userID != nil {
		where = append(where, "l.user_id = ?")
		args = append(args, *userID)
	}
	if f.UserID != nil {
		where = append(where, "l.user_id = ?")
		args = append(args, *f.UserID)
	}
	if f.Action != "" {
		where = append(where, "l.action = ?")
		args = append(args, f.Action)
	}
	if f.DateFrom != "" {
		where = append(where, "l.created_at >= ?")
		args = append(args, f.DateFrom)
	}
	if f.DateTo != "" {
		where = append(where, "l.created_at <= ?")
		args = append(args, f.DateTo)
	}
	if f.Keyword != "" {
		kw := "%" + f.Keyword + "%"
		where = append(where, `(l.detail LIKE ? OR l.target LIKE ? OR u.username LIKE ? OR a.display_name LIKE ? OR l.ip LIKE ? OR l.action LIKE ?)`)
		args = append(args, kw, kw, kw, kw, kw, kw)
	}

	query := base
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY l.id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []AuditLog
	for rows.Next() {
		var l AuditLog
		var uid, aid sql.NullInt64
		if err := rows.Scan(&l.ID, &uid, &l.Username, &aid, &l.AccountName, &l.Action, &l.Target, &l.Detail, &l.IP, &l.CreatedAt); err != nil {
			return nil, err
		}
		if uid.Valid {
			l.UserID = &uid.Int64
		}
		if aid.Valid {
			l.AccountID = &aid.Int64
		}
		logs = append(logs, l)
	}
	return logs, nil
}

// CountAuditLogs returns the total number of audit logs matching the filter,
// used for pagination.
func CountAuditLogs(userID *int64, f LogFilter) (int, error) {
	from := `FROM audit_logs l
		LEFT JOIN users u ON u.id = l.user_id
		LEFT JOIN game_accounts a ON a.id = l.account_id`
	where := []string{}
	args := []any{}
	if userID != nil {
		where = append(where, "l.user_id = ?")
		args = append(args, *userID)
	}
	if f.UserID != nil {
		where = append(where, "l.user_id = ?")
		args = append(args, *f.UserID)
	}
	if f.Action != "" {
		where = append(where, "l.action = ?")
		args = append(args, f.Action)
	}
	if f.DateFrom != "" {
		where = append(where, "l.created_at >= ?")
		args = append(args, f.DateFrom)
	}
	if f.DateTo != "" {
		where = append(where, "l.created_at <= ?")
		args = append(args, f.DateTo)
	}
	if f.Keyword != "" {
		kw := "%" + f.Keyword + "%"
		where = append(where, `(l.detail LIKE ? OR l.target LIKE ? OR u.username LIKE ? OR a.display_name LIKE ? OR l.ip LIKE ? OR l.action LIKE ?)`)
		args = append(args, kw, kw, kw, kw, kw, kw)
	}
	query := "SELECT COUNT(*) " + from
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	var n int
	if err := DB.QueryRow(query, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// HasRecentJoin checks if user joined a specific server within the cutoff time,
// keyed on the same identity (account + api token) so that switching account or
// token expires the free window.
func HasRecentJoin(userID, accountID, tokenID int64, serverCode, cutoff string) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM audit_logs WHERE user_id = ? AND account_id = ? AND action = 'join_server' AND target = ? AND created_at >= ?`
	args := []any{userID, accountID, serverCode, cutoff}
	if tokenID == 0 {
		query += ` AND (token_id = 0 OR token_id IS NULL)`
	} else {
		query += ` AND token_id = ?`
		args = append(args, tokenID)
	}
	err := DB.QueryRow(query, args...).Scan(&count)
	return count > 0, err
}

// HasRecentOtherJoin checks if user joined any OTHER server within the cutoff time,
// keyed on the same identity (account + api token).
func HasRecentOtherJoin(userID, accountID, tokenID int64, serverCode, cutoff string) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM audit_logs WHERE user_id = ? AND account_id = ? AND action = 'join_server' AND target != ? AND created_at >= ?`
	args := []any{userID, accountID, serverCode, cutoff}
	if tokenID == 0 {
		query += ` AND (token_id = 0 OR token_id IS NULL)`
	} else {
		query += ` AND token_id = ?`
		args = append(args, tokenID)
	}
	err := DB.QueryRow(query, args...).Scan(&count)
	return count > 0, err
}

func PruneSystemLogs(keepDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -keepDays).UTC().Format(time.RFC3339)
	result, err := DB.Exec("DELETE FROM system_logs WHERE created_at < ?", cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return n, nil
}

// PruneAPICallLogs 清理超过 keepDays 天的 API 调用日志。这些数据驱动统计/繁忙时段图,
// 保留天数应与统计回看窗口匹配(如 30 天),避免清掉统计仍在展示的历史。
func PruneAPICallLogs(keepDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -keepDays).UTC().Format(time.RFC3339)
	result, err := DB.Exec("DELETE FROM api_call_logs WHERE created_at < ?", cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return n, nil
}

func AddSystemLog(level, message, detail string) error {
	_, err := DB.Exec(
		`INSERT INTO system_logs (level, message, detail, created_at) VALUES (?, ?, ?, ?)`,
		level, message, detail, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func ListSystemLogs(limit int, offset int, f LogFilter) ([]SystemLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	where := []string{}
	args := []any{}
	if f.Action != "" {
		where = append(where, "level = ?")
		args = append(args, f.Action)
	}
	if f.DateFrom != "" {
		where = append(where, "created_at >= ?")
		args = append(args, f.DateFrom)
	}
	if f.DateTo != "" {
		where = append(where, "created_at <= ?")
		args = append(args, f.DateTo)
	}
	if f.Keyword != "" {
		kw := "%" + f.Keyword + "%"
		where = append(where, `(message LIKE ? OR detail LIKE ?)`)
		args = append(args, kw, kw)
	}
	query := "SELECT id, level, message, detail, created_at FROM system_logs"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []SystemLog
	for rows.Next() {
		var l SystemLog
		if err := rows.Scan(&l.ID, &l.Level, &l.Message, &l.Detail, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}

// CountSystemLogs returns the total number of system logs matching the filter.
func CountSystemLogs(f LogFilter) (int, error) {
	where := []string{}
	args := []any{}
	if f.Action != "" {
		where = append(where, "level = ?")
		args = append(args, f.Action)
	}
	if f.DateFrom != "" {
		where = append(where, "created_at >= ?")
		args = append(args, f.DateFrom)
	}
	if f.DateTo != "" {
		where = append(where, "created_at <= ?")
		args = append(args, f.DateTo)
	}
	if f.Keyword != "" {
		kw := "%" + f.Keyword + "%"
		where = append(where, `(message LIKE ? OR detail LIKE ?)`)
		args = append(args, kw, kw)
	}
	query := "SELECT COUNT(*) FROM system_logs"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	var n int
	if err := DB.QueryRow(query, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

