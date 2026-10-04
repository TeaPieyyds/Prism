package db

import (

	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

type GameAccount struct {
	ID             int64  `json:"id"`
	OwnerID        *int64 `json:"owner_id,omitempty"` // NULL = shared pool
	CreatedBy      *int64 `json:"created_by,omitempty"`
	DisplayName    string `json:"display_name"`
	UID            string `json:"uid"`
	CookieData     string `json:"-"`
	Status         string `json:"status"`
	GrowthLevel    string `json:"growth_level,omitempty"`
	GrowthExp      string `json:"growth_exp,omitempty"`
	GrowthNeedExp  string `json:"growth_need_exp,omitempty"`
	Score          string `json:"score,omitempty"`
	SkinNumber     string `json:"skin_number,omitempty"`
	SkinURL        string `json:"skin_url,omitempty"`
	CapeNumber     string `json:"cape_number,omitempty"`
	AvatarImageURL string `json:"avatar_image_url,omitempty"`
	IsVip          bool   `json:"is_vip"`
	Disabled           bool   `json:"disabled"`
	Source             string `json:"source,omitempty"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
	AutoRefreshEnabled bool   `json:"auto_refresh_enabled"`
	_autoRefreshRaw    int    // SQLite workaround: scan INTEGER then convert
	IsServerOwner      bool   `json:"is_server_owner"`
	_serverOwnerRaw    int    // SQLite workaround: scan INTEGER then convert
	// Extra info not stored in DB but returned in API
	IsActive bool   `json:"is_active"`
	LastUsed string `json:"last_used,omitempty"`
}

type AccountInfo struct {
	ID                 int64  `json:"id"`
	CreatedBy          *int64 `json:"created_by,omitempty"`
	DisplayName        string `json:"display_name"`
	UID                string `json:"uid"`
	Status             string `json:"status"`
	GrowthLevel        string `json:"growth_level"`
	GrowthExp          string `json:"growth_exp,omitempty"`
	GrowthNeedExp      string `json:"growth_need_exp,omitempty"`
	Score              string `json:"score"`
	SkinNumber         string `json:"skin_number"`
	SkinURL            string `json:"skin_url"`
	CapeNumber         string `json:"cape_number"`
	AvatarImageURL     string `json:"avatar_image_url"`
	IsVip              bool   `json:"is_vip"`
	Disabled           bool   `json:"disabled"`
	Source             string `json:"source"`
	IsShared           bool   `json:"is_shared"`
	IsActive           bool   `json:"is_active"`
	AccessGameFlag     string `json:"access_game_flag,omitempty"`
	RealnameStatus     string `json:"realname_status,omitempty"`
	AntiAdditionStatus string `json:"anti_addition_status,omitempty"`
	Signature          string `json:"signature,omitempty"`
}

// For JSON parsing of cookie data
type cookieOuter struct {
	SauthJSON string `json:"sauth_json"`
}

type sauthData struct {
	AccessGameFlag     string `json:"access_game_flag"`
	RealnameStatus     string `json:"realname_status"`
	AntiAdditionStatus string `json:"anti_addition_status"`
	Signature          string `json:"signature"`
	DisplayName        string `json:"display_name"`
	UID                string `json:"uid"`
	SDKUID             string `json:"sdkuid"`
}

// GetAccountsExcludeSource 返回排除了指定来源的非禁用账号列表。
// GetAccountsWithAutoRefresh 返回启用了自动刷新的账号（含 phone/email）
func GetAccountsWithAutoRefresh() ([]*GameAccount, error) {
	rows, err := DB.Query(
		`SELECT id, owner_id, created_by, display_name, uid, cookie_data, status,
		        growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url,
		        cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled,
		        is_server_owner, created_at, updated_at
		 FROM game_accounts WHERE auto_refresh_enabled=1 AND disabled=0 AND cookie_data != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*GameAccount
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, nil
}

func GetAccountsExcludeSource(excludes ...string) ([]*GameAccount, error) {
	if len(excludes) == 0 {
		rows, err := DB.Query(
			`SELECT id, owner_id, created_by, display_name, uid, cookie_data, status,
			        growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url,
			        cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled, is_server_owner,
			        created_at, updated_at
			 FROM game_accounts WHERE disabled=0 AND cookie_data != ''`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var result []*GameAccount
		for rows.Next() {
			a, err := scanAccount(rows)
			if err != nil {
				return nil, err
			}
			result = append(result, a)
		}
		return result, nil
	}
	placeholders := make([]string, len(excludes))
	args := make([]any, len(excludes))
	for i, s := range excludes {
		placeholders[i] = "?"
		args[i] = s
	}
	rows, err := DB.Query(
		`SELECT id, owner_id, created_by, display_name, uid, cookie_data, status,
		        growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url,
		        cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled, is_server_owner,
		        created_at, updated_at
		 FROM game_accounts
		 WHERE source NOT IN (`+strings.Join(placeholders, ",")+`) AND disabled=0 AND cookie_data != ''`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*GameAccount
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, nil
}

func GetUserAccounts(userID int64) ([]*GameAccount, error) {
	rows, err := DB.Query(
		`SELECT id, owner_id, created_by, display_name, uid, cookie_data, status,
		        growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url, cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled, is_server_owner,
		        created_at, updated_at
		 FROM game_accounts WHERE owner_id = ? ORDER BY display_name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

func GetSharedAccounts() ([]*GameAccount, error) {
	rows, err := DB.Query(
		`SELECT id, owner_id, created_by, display_name, uid, cookie_data, status,
		        growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url, cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled, is_server_owner,
		        created_at, updated_at
		 FROM game_accounts WHERE owner_id IS NULL ORDER BY display_name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

func GetSharedAccountsByCreator(userID int64) ([]*GameAccount, error) {
	rows, err := DB.Query(
		`SELECT id, owner_id, created_by, display_name, uid, cookie_data, status,
		        growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url, cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled, is_server_owner,
		        created_at, updated_at
		 FROM game_accounts WHERE owner_id IS NULL AND created_by = ? ORDER BY display_name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

// GetNamedSharedAccounts returns shared accounts (owner_id IS NULL) that have
// a non-empty display_name — used for the guest/anonymous pool.
func GetNamedSharedAccounts() ([]*GameAccount, error) {
	rows, err := DB.Query(
		`SELECT id, owner_id, created_by, display_name, uid, cookie_data, status,
		        growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url, cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled, is_server_owner,
		        created_at, updated_at
		 FROM game_accounts WHERE owner_id IS NULL AND disabled=0 AND cookie_data != '' AND display_name != ''
		 ORDER BY RANDOM()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

func AddAccount(ownerID *int64, cookieData string, info AccountInfo) (*GameAccount, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	createdBy := info.CreatedBy
	if createdBy == nil {
		createdBy = ownerID
	}
	// Default: auto-refresh on for cookie/guest/web, off for phone/email
	autoRefreshVal := 1
	if info.Source == "phone" || info.Source == "email" {
		autoRefreshVal = 0
		if ownerID == nil { // 共享账号强制开启
			autoRefreshVal = 1
		}
	}
	// Upsert: same owner + same UID → update instead of duplicate
	var existingID int64
	row := DB.QueryRow(`SELECT id FROM game_accounts WHERE uid = ? AND ((owner_id IS NULL AND ? IS NULL) OR owner_id = ?) LIMIT 1`,
		info.UID, ownerID, ownerID)
	if row.Scan(&existingID) == nil && existingID > 0 {
		// Update existing
		_, err := DB.Exec(
			`UPDATE game_accounts SET display_name=?, cookie_data=?, status=?, growth_level=?, growth_exp=?, growth_need_exp=?, score=?,
			 skin_number=?, skin_url=?, cape_number=?, avatar_image_url=?, is_vip=?, disabled=?, source=?, updated_at=? WHERE id=?`,
			info.DisplayName, cookieData, info.Status, info.GrowthLevel, info.GrowthExp, info.GrowthNeedExp, info.Score,
			info.SkinNumber, info.SkinURL, info.CapeNumber, info.AvatarImageURL, info.IsVip, info.Disabled, info.Source, now, existingID)
		if err != nil {
			return nil, err
		}
		return GetAccountByID(existingID)
	}
	// Insert new
	if ownerID != nil {
		// 该用户私有账号池上限
		var n int
		if err := DB.QueryRow(`SELECT COUNT(*) FROM game_accounts WHERE owner_id = ?`, *ownerID).Scan(&n); err == nil && n >= 40 {
			return nil, fmt.Errorf("你的私有账号池已达上限（40 个），无法继续添加")
		}
	}
	result, err := DB.Exec(
		`INSERT INTO game_accounts (owner_id, created_by, display_name, uid, cookie_data, status,
		 growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url, cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled, is_server_owner, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ownerID, createdBy, info.DisplayName, info.UID, cookieData, info.Status,
		info.GrowthLevel, info.GrowthExp, info.GrowthNeedExp, info.Score, info.SkinNumber, info.SkinURL, info.CapeNumber, info.AvatarImageURL, info.IsVip, info.Disabled, info.Source, autoRefreshVal, 0,
		now, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return GetAccountByID(id)
}

func GetAccountByID(id int64) (*GameAccount, error) {
	row := DB.QueryRow(
		`SELECT id, owner_id, created_by, display_name, uid, cookie_data, status,
		        growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url, cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled, is_server_owner,
		        created_at, updated_at
		 FROM game_accounts WHERE id = ?`, id)
	return scanAccount(row)
}

func HasSharedCopy(uid string) (bool, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM game_accounts WHERE uid = ? AND owner_id IS NULL`, uid).Scan(&count)
	return count > 0, err
}

func GetSharedAccountByUID(uid string) (*GameAccount, error) {
	row := DB.QueryRow(
		`SELECT id, owner_id, created_by, display_name, uid, cookie_data, status,
		        growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url,
		        cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled, is_server_owner,
		        created_at, updated_at
		 FROM game_accounts WHERE uid = ? AND owner_id IS NULL LIMIT 1`, uid)
	return scanAccount(row)
}

func LookupAccountByUID(uid string) (*GameAccount, error) {
	row := DB.QueryRow(
		`SELECT id, owner_id, created_by, display_name, uid, cookie_data, status,
		        growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url, cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled, is_server_owner,
		        created_at, updated_at
		 FROM game_accounts WHERE uid = ? LIMIT 1`, uid)
	return scanAccount(row)
}

// GetChallengeOverrideUIDs returns all game account UIDs whose owner has challenge_override enabled.
func GetChallengeOverrideUserIDs() ([]int64, error) {
	rows, err := DB.Query(
		`SELECT u.id FROM users u
		 WHERE u.challenge_override = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func DeleteAccount(id int64) error {
	DB.Exec(`UPDATE users SET active_account_id = NULL WHERE active_account_id = ?`, id)
	_, err := DB.Exec(`DELETE FROM game_accounts WHERE id = ?`, id)
	return err
}

// HandleAccountBanned 账号被网易永久封禁(code 29)时调用：
// 共享账号(owner_id IS NULL)直接删除,避免占用资源;私有账号标记 banned。
func HandleAccountBanned(accountID int64) {
	var owner sql.NullInt64
	if err := DB.QueryRow(`SELECT owner_id FROM game_accounts WHERE id = ?`, accountID).Scan(&owner); err != nil {
		log.Printf("[ACCOUNTS] 封禁处理 #%d: 查询失败 %v", accountID, err)
		return
	}
	if !owner.Valid {
		if err := DeleteAccount(accountID); err != nil {
			log.Printf("[ACCOUNTS] 共享账号 #%d 封禁删除失败: %v", accountID, err)
			return
		}
		log.Printf("[ACCOUNTS] 共享账号 #%d 封禁,已删除", accountID)
		return
	}
	if err := UpdateAccountStatus(accountID, "banned"); err != nil {
		log.Printf("[ACCOUNTS] 私有账号 #%d 封禁标记失败: %v", accountID, err)
	}
}

// DeleteAccountCascade deletes the account and all other copies with the same UID.
func DeleteAccountCascade(id int64) error {
	var uid string
	if err := DB.QueryRow(`SELECT uid FROM game_accounts WHERE id = ?`, id).Scan(&uid); err == nil && uid != "" {
		rows, err := DB.Query(`SELECT id FROM game_accounts WHERE uid = ? AND id != ?`, uid, id)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var otherID int64
				if rows.Scan(&otherID) == nil {
					DB.Exec(`UPDATE users SET active_account_id = NULL WHERE active_account_id = ?`, otherID)
					DB.Exec(`DELETE FROM game_accounts WHERE id = ?`, otherID)
				}
			}
		}
	}
	DB.Exec(`UPDATE users SET active_account_id = NULL WHERE active_account_id = ?`, id)
	_, err := DB.Exec(`DELETE FROM game_accounts WHERE id = ?`, id)
	return err
}

func SetAccountShared(id int64, shared bool, userID int64) error {
	var owner sql.NullInt64
	if shared {
		owner.Valid = false
		// 共享账号自动启用刷新，确保 cookie 不过期
		DB.Exec(`UPDATE game_accounts SET auto_refresh_enabled = 1, updated_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), id)
	} else {
		owner.Int64 = userID
		owner.Valid = true
		// Clear other users who had this shared account as active
		DB.Exec(`UPDATE users SET active_account_id = NULL WHERE active_account_id = ? AND id != ?`, id, userID)
	}
	_, err := DB.Exec(`UPDATE game_accounts SET owner_id = ?, updated_at = ? WHERE id = ?`, owner, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func SetAutoRefresh(id int64, enabled bool) error {
	v := 0
	if enabled { v = 1 }
	_, err := DB.Exec(`UPDATE game_accounts SET auto_refresh_enabled = ?, updated_at = ? WHERE id = ?`, v, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func UpdateAccountName(id int64, displayName string) error {
	_, err := DB.Exec(`UPDATE game_accounts SET display_name = ?, updated_at = ? WHERE id = ?`, displayName, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func SetAccountDisabled(id int64, disabled bool) error {
	v := 0
	if disabled {
		v = 1
	}
	_, err := DB.Exec(`UPDATE game_accounts SET disabled = ?, updated_at = ? WHERE id = ?`, v, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func ListAllAccounts() ([]*GameAccount, error) {
	rows, err := DB.Query(
		`SELECT id, owner_id, created_by, display_name, uid, cookie_data, status,
		        growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url, cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled, is_server_owner,
		        created_at, updated_at
		 FROM game_accounts ORDER BY display_name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all, err := scanAccounts(rows)
	if err != nil {
		return nil, err
	}
	// Dedupe: for same UID, prefer owned accounts with valid cookie
	seen := make(map[string]*GameAccount)
	for _, a := range all {
		existing, ok := seen[a.UID]
		if !ok {
			seen[a.UID] = a
			continue
		}
		// Keep the better one: owned > shared, has cookie > no cookie, newer > older
		if a.OwnerID != nil && existing.OwnerID == nil {
			seen[a.UID] = a
		} else if a.OwnerID != nil && existing.OwnerID != nil && a.CookieData != "" && existing.CookieData == "" {
			seen[a.UID] = a
		} else if a.CookieData != "" && existing.CookieData == "" {
			seen[a.UID] = a
		}
	}
	var deduped []*GameAccount
	for _, a := range seen {
		deduped = append(deduped, a)
	}
	return deduped, nil
}

// ListAllAccountsForAdmin returns ALL account records without UID dedup.
// Admin panel needs to see every account, including duplicates with the same UID.
func ListAllAccountsForAdmin() ([]*GameAccount, error) {
	rows, err := DB.Query(
		`SELECT id, owner_id, created_by, display_name, uid, cookie_data, status,
		        growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url, cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled, is_server_owner,
		        created_at, updated_at
		 FROM game_accounts ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

func ListGuestAccounts() ([]*GameAccount, error) {
	rows, err := DB.Query(
		`SELECT id, owner_id, created_by, display_name, uid, cookie_data, status,
		        growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url, cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled, is_server_owner,
		        created_at, updated_at
		 FROM game_accounts
		 WHERE source = 'guest' AND status = 'normal' AND cookie_data != ''
		 ORDER BY display_name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

func UpdateAccountStatus(id int64, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := DB.Exec(`UPDATE game_accounts SET status = ?, updated_at = ? WHERE id = ?`, status, now, id)
	return err
}

func UpdateAccountFull(id int64, info AccountInfo) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if info.Source == "" {
		// preserve existing source
		var s string
		DB.QueryRow(`SELECT source FROM game_accounts WHERE id=?`, id).Scan(&s)
		info.Source = s
	}
	_, err := DB.Exec(
		`UPDATE game_accounts SET display_name=?, uid=?, status=?, growth_level=?, growth_exp=?, growth_need_exp=?, score=?,
			 skin_number=?, skin_url=?, cape_number=?, avatar_image_url=?, is_vip=?, source=?, updated_at=? WHERE id=?`,
		info.DisplayName, info.UID, info.Status, info.GrowthLevel, info.GrowthExp, info.GrowthNeedExp, info.Score,
		info.SkinNumber, info.SkinURL, info.CapeNumber, info.AvatarImageURL, info.IsVip, info.Source, now, id)
	return err
}
func ParseAccountInfo(cookieData string) (*AccountInfo, error) {
	var outer cookieOuter
	if err := json.Unmarshal([]byte(cookieData), &outer); err != nil {
		_ = json.Unmarshal([]byte(cookieData), &outer) // try nested
	}
	var sauth sauthData
	if outer.SauthJSON != "" {
		if err := json.Unmarshal([]byte(outer.SauthJSON), &sauth); err != nil {
			return nil, err
		}
	} else {
		_ = json.Unmarshal([]byte(cookieData), &sauth)
	}
	uid := sauth.UID
	if uid == "" {
		uid = sauth.SDKUID
	}
	if uid == "" {
		return nil, fmt.Errorf("无法从cookie中解析到UID，cookie可能无效")
	}
	return &AccountInfo{
		DisplayName:        sauth.DisplayName,
		UID:                uid,
		AccessGameFlag:     sauth.AccessGameFlag,
		RealnameStatus:     sauth.RealnameStatus,
		AntiAdditionStatus: sauth.AntiAdditionStatus,
		Signature:          sauth.Signature,
	}, nil
}

func scanAccounts(rows *sql.Rows) ([]*GameAccount, error) {
	var accs []*GameAccount
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accs = append(accs, a)
	}
	return accs, nil
}

func scanAccount(s interface {
	Scan(dest ...any) error
}) (*GameAccount, error) {
	var a GameAccount
	var ownerID, createdBy sql.NullInt64
	err := s.Scan(&a.ID, &ownerID, &createdBy, &a.DisplayName, &a.UID, &a.CookieData,
		&a.Status, &a.GrowthLevel, &a.GrowthExp, &a.GrowthNeedExp, &a.Score, &a.SkinNumber, &a.SkinURL, &a.CapeNumber,
		&a.AvatarImageURL, &a.IsVip, &a.Disabled, &a.Source, &a._autoRefreshRaw, &a._serverOwnerRaw, &a.CreatedAt, &a.UpdatedAt)
		a.AutoRefreshEnabled = a._autoRefreshRaw != 0
		a.IsServerOwner = a._serverOwnerRaw != 0
	if err != nil {
		return nil, err
	}
	if ownerID.Valid {
		a.OwnerID = &ownerID.Int64
	}
	if createdBy.Valid {
		a.CreatedBy = &createdBy.Int64
	}
	return &a, nil
}

func boolToInt(b bool) int {
	if b { return 1 }
	return 0
}

// SetServerOwner 设置/取消账号的服主标记。
// enabled=true 时同时关闭自动刷新。
func SetServerOwner(id int64, enabled bool) error {
	v := 0
	if enabled { v = 1 }
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := DB.Exec(`UPDATE game_accounts SET is_server_owner = ?, auto_refresh_enabled = ?, updated_at = ? WHERE id = ?`,
		v, boolToInt(!enabled), now, id)
	return err
}

// GetServerOwnerAccounts 获取指定用户的所有服主账号。
func GetServerOwnerAccounts(userID int64) ([]*GameAccount, error) {
	rows, err := DB.Query(
		`SELECT id, owner_id, created_by, display_name, uid, cookie_data, status,
		        growth_level, growth_exp, growth_need_exp, score, skin_number, skin_url,
		        cape_number, avatar_image_url, is_vip, disabled, source, auto_refresh_enabled, is_server_owner,
		        created_at, updated_at
		 FROM game_accounts WHERE owner_id = ? AND is_server_owner = 1 ORDER BY display_name ASC`, userID)
	if err != nil { return nil, err }
	defer rows.Close()
	return scanAccounts(rows)
}

// HasOtherUserWithSameUID 检查除当前用户外是否有其他人使用同 UID 的账号。
func HasOtherUserWithSameUID(uid string, excludeUserID int64) (bool, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM game_accounts WHERE uid = ? AND (owner_id IS NOT NULL AND owner_id != ?)`, uid, excludeUserID).Scan(&count)
	return count > 0, err
}
