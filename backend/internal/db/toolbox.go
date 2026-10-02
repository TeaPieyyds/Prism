package db

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ── App Version Config ──

type AppVersionConfig struct {
	LatestVersion     string `json:"latest_version"`
	LatestVersionCode int    `json:"latest_version_code"`
	ForceUpdate       bool   `json:"force_update"`
	UpdateURL         string `json:"update_url"`
	UpdateMessage     string `json:"update_message"`
}

func GetAppVersionConfig() (*AppVersionConfig, error) {
	var c AppVersionConfig
	var force int
	err := DB.QueryRow(`SELECT latest_version, latest_version_code, force_update, update_url, update_message FROM app_version_config WHERE id = 1`).
		Scan(&c.LatestVersion, &c.LatestVersionCode, &force, &c.UpdateURL, &c.UpdateMessage)
	if err != nil {
		// Return defaults if no row exists
		return &AppVersionConfig{}, nil
	}
	c.ForceUpdate = force != 0
	return &c, nil
}

func UpdateAppVersionConfig(c *AppVersionConfig) error {
	force := 0
	if c.ForceUpdate {
		force = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := DB.Exec(`INSERT INTO app_version_config (id, latest_version, latest_version_code, force_update, update_url, update_message, updated_at) VALUES (1, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET latest_version=excluded.latest_version, latest_version_code=excluded.latest_version_code, force_update=excluded.force_update, update_url=excluded.update_url, update_message=excluded.update_message, updated_at=excluded.updated_at`,
		c.LatestVersion, c.LatestVersionCode, force, c.UpdateURL, c.UpdateMessage, now)
	if err != nil {
		return err
	}
	// Sync changelog to current version's release_notes for history
	if c.UpdateMessage != "" {
		DB.Exec(`UPDATE app_versions SET release_notes = ? WHERE is_current = 1`, c.UpdateMessage)
	}
	return nil
}

// ── App Announcements ──

type Announcement struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Content        string   `json:"content"`
	ContentType    string   `json:"content_type"`
	Severity       string   `json:"severity"`
	DisplayMode    string   `json:"display_mode"`
	CanDismiss     bool     `json:"can_dismiss"`
	TargetVersions []string `json:"target_versions"`
	TargetServers  []string `json:"target_servers"`
	IsActive       bool     `json:"is_active"`
	CreatedAt      string   `json:"created_at"`
	ExpiresAt      string   `json:"expires_at"`
}

func GetActiveAnnouncements(versionCode int, serverCode string) ([]Announcement, error) {
	rows, err := DB.Query(`SELECT id, title, content, content_type, severity, display_mode, can_dismiss, target_versions, target_servers, created_at, expires_at FROM app_announcements WHERE is_active = 1 ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now().UTC()
	var all []Announcement
	for rows.Next() {
		var a Announcement
		var canDismiss int
		var tvJSON, tsJSON string
		if err := rows.Scan(&a.ID, &a.Title, &a.Content, &a.ContentType, &a.Severity, &a.DisplayMode, &canDismiss, &tvJSON, &tsJSON, &a.CreatedAt, &a.ExpiresAt); err != nil {
			continue
		}
		a.CanDismiss = canDismiss != 0
		json.Unmarshal([]byte(tvJSON), &a.TargetVersions)
		json.Unmarshal([]byte(tsJSON), &a.TargetServers)

		// Filter by version
		if len(a.TargetVersions) > 0 {
			verStr := fmt.Sprintf("%d", versionCode)
			found := false
			for _, v := range a.TargetVersions {
				if v == verStr {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		// Filter by server
		if len(a.TargetServers) > 0 && serverCode != "" {
			found := false
			for _, s := range a.TargetServers {
				if s == serverCode {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		// Check expiry
		if a.ExpiresAt != "" {
			t, err := time.Parse(time.RFC3339, a.ExpiresAt)
			if err == nil && now.After(t) {
				continue
			}
		}

		all = append(all, a)
	}
	return all, nil
}

func ListAllAnnouncements() ([]Announcement, error) {
	rows, err := DB.Query(`SELECT id, title, content, content_type, severity, display_mode, can_dismiss, target_versions, target_servers, is_active, created_at, expires_at FROM app_announcements ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Announcement
	for rows.Next() {
		var a Announcement
		var canDismiss, isActive int
		var tvJSON, tsJSON string
		if err := rows.Scan(&a.ID, &a.Title, &a.Content, &a.ContentType, &a.Severity, &a.DisplayMode, &canDismiss, &tvJSON, &tsJSON, &isActive, &a.CreatedAt, &a.ExpiresAt); err != nil {
			continue
		}
		a.CanDismiss = canDismiss != 0
		a.IsActive = isActive != 0
		json.Unmarshal([]byte(tvJSON), &a.TargetVersions)
		json.Unmarshal([]byte(tsJSON), &a.TargetServers)
		list = append(list, a)
	}
	return list, nil
}

func CreateAnnouncement(a *Announcement) error {
	a.ID = fmt.Sprintf("ann_%s_%03d", time.Now().Format("20060102"), time.Now().UnixNano()%1000)
	if a.ContentType == "" {
		a.ContentType = "markdown"
	}
	if a.Severity == "" {
		a.Severity = "info"
	}
	if a.DisplayMode == "" {
		a.DisplayMode = "modal"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	a.CreatedAt = now
	if a.ExpiresAt == "" {
		a.ExpiresAt = time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	}

	canDismiss := 0
	if a.CanDismiss {
		canDismiss = 1
	}
	isActive := 0
	if a.IsActive {
		isActive = 1
	}
	tvJSON, _ := json.Marshal(a.TargetVersions)
	tsJSON, _ := json.Marshal(a.TargetServers)

	_, err := DB.Exec(`INSERT INTO app_announcements (id, title, content, content_type, severity, display_mode, can_dismiss, target_versions, target_servers, is_active, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Title, a.Content, a.ContentType, a.Severity, a.DisplayMode, canDismiss, string(tvJSON), string(tsJSON), isActive, a.CreatedAt, a.ExpiresAt)
	return err
}

func UpdateAnnouncement(a *Announcement) error {
	canDismiss := 0
	if a.CanDismiss {
		canDismiss = 1
	}
	isActive := 0
	if a.IsActive {
		isActive = 1
	}
	tvJSON, _ := json.Marshal(a.TargetVersions)
	tsJSON, _ := json.Marshal(a.TargetServers)

	_, err := DB.Exec(`UPDATE app_announcements SET title=?, content=?, content_type=?, severity=?, display_mode=?, can_dismiss=?, target_versions=?, target_servers=?, is_active=?, expires_at=? WHERE id=?`,
		a.Title, a.Content, a.ContentType, a.Severity, a.DisplayMode, canDismiss, string(tvJSON), string(tsJSON), isActive, a.ExpiresAt, a.ID)
	return err
}

func DeleteAnnouncement(id string) error {
	_, err := DB.Exec(`DELETE FROM app_announcements WHERE id = ?`, id)
	return err
}

func ToggleAnnouncementActive(id string) (bool, error) {
	var current int
	err := DB.QueryRow(`SELECT is_active FROM app_announcements WHERE id = ?`, id).Scan(&current)
	if err != nil {
		return false, err
	}
	next := 1
	if current == 1 {
		next = 0
	}
	_, err = DB.Exec(`UPDATE app_announcements SET is_active = ? WHERE id = ?`, next, id)
	return next == 1, err
}

// ── App Signatures ──

type AppSignature struct {
	ID            int64  `json:"id"`
	PackageName   string `json:"package_name"`
	SignatureHash string `json:"signature_hash"`
	Label         string `json:"label"`
	CreatedAt     string `json:"created_at"`
}

func AddSignature(packageName, hash, label string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := DB.Exec(`INSERT INTO app_signatures (package_name, signature_hash, label, created_at) VALUES (?, ?, ?, ?)`,
		packageName, strings.TrimSpace(hash), label, now)
	return err
}

func RemoveSignature(id int64) error {
	_, err := DB.Exec(`DELETE FROM app_signatures WHERE id = ?`, id)
	return err
}

func ListSignatures() ([]AppSignature, error) {
	rows, err := DB.Query(`SELECT id, package_name, signature_hash, label, created_at FROM app_signatures ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []AppSignature
	for rows.Next() {
		var s AppSignature
		if err := rows.Scan(&s.ID, &s.PackageName, &s.SignatureHash, &s.Label, &s.CreatedAt); err != nil {
			continue
		}
		list = append(list, s)
	}
	return list, nil
}

func VerifySignature(packageName, hash string) (bool, string, string) {
	// 一个包名可能对应多个官方签名（历史上换过密钥），只要命中任意一个即通过。
	rows, err := DB.Query(`SELECT signature_hash, label FROM app_signatures WHERE package_name = ?`, packageName)
	if err != nil {
		return true, "", "" // DB 查询失败 → 放行（不阻断正常使用）
	}
	defer rows.Close()
	known := make([]string, 0, 4)
	labels := make(map[string]string)
	for rows.Next() {
		var h, l string
		if err := rows.Scan(&h, &l); err != nil {
			continue
		}
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		known = append(known, h)
		labels[h] = l
	}
	if len(known) == 0 {
		return true, "", "" // 尚未配置白名单 → 放行
	}
	target := strings.TrimSpace(hash)
	for _, h := range known {
		if strings.EqualFold(target, h) {
			return true, h, ""
		}
	}
	return false, strings.Join(known, ","), "APK 签名不匹配，可能已被篡改。请从官方渠道重新下载。"
}

// ── App Versions ──

type AppVersion struct {
	ID           int64  `json:"id"`
	Version      string `json:"version"`
	VersionCode  int    `json:"version_code"`
	ReleaseNotes string `json:"release_notes"`
	FileName     string `json:"file_name"`
	FileSize     int64  `json:"file_size"`
	IsCurrent    bool   `json:"is_current"`
	UploadedAt   string `json:"uploaded_at"`
}

func AddVersion(version string, versionCode int, releaseNotes, fileName string, fileSize int64) (*AppVersion, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := DB.Exec(`INSERT INTO app_versions (version, version_code, release_notes, file_name, file_size, is_current, uploaded_at) VALUES (?, ?, ?, ?, ?, 0, ?)`,
		version, versionCode, releaseNotes, fileName, fileSize, now)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return &AppVersion{
		ID: id, Version: version, VersionCode: versionCode,
		ReleaseNotes: releaseNotes, FileName: fileName, FileSize: fileSize,
		UploadedAt: now,
	}, nil
}

func ListAppVersions() ([]AppVersion, error) {
	rows, err := DB.Query(`SELECT id, version, version_code, release_notes, file_name, file_size, is_current, uploaded_at FROM app_versions ORDER BY version_code DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []AppVersion
	for rows.Next() {
		var v AppVersion
		var isCur int
		if err := rows.Scan(&v.ID, &v.Version, &v.VersionCode, &v.ReleaseNotes, &v.FileName, &v.FileSize, &isCur, &v.UploadedAt); err != nil {
			continue
		}
		v.IsCurrent = isCur != 0
		list = append(list, v)
	}
	return list, nil
}

func SwitchCurrentVersion(versionID int64) (*AppVersion, error) {
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE app_versions SET is_current = 0`)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(`UPDATE app_versions SET is_current = 1 WHERE id = ?`, versionID)
	if err != nil {
		return nil, err
	}

	var v AppVersion
	var isCur int
	err = tx.QueryRow(`SELECT id, version, version_code, release_notes, file_name, file_size, is_current, uploaded_at FROM app_versions WHERE id = ?`, versionID).
		Scan(&v.ID, &v.Version, &v.VersionCode, &v.ReleaseNotes, &v.FileName, &v.FileSize, &isCur, &v.UploadedAt)
	if err != nil {
		return nil, err
	}
	v.IsCurrent = isCur != 0

	// Sync only version string and URL — don't overwrite admin's version_code/force/message
	updateURL := "/dl/" + v.FileName
	now := time.Now().UTC().Format(time.RFC3339)
	tx.Exec(`INSERT INTO app_version_config (id, latest_version, latest_version_code, force_update, update_url, update_message, updated_at) VALUES (1, ?, 0, 0, ?, '', ?) ON CONFLICT(id) DO UPDATE SET latest_version=excluded.latest_version, update_url=excluded.update_url, updated_at=excluded.updated_at`,
		v.Version, updateURL, now)
	// Record this version's changelog
	if v.ReleaseNotes != "" {
		tx.Exec(`UPDATE app_versions SET release_notes = ? WHERE id = ?`, v.ReleaseNotes, v.ID)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &v, nil
}

func DeleteVersion(versionID int64) error {
	var fileName string
	var isCurrent int
	err := DB.QueryRow(`SELECT file_name, is_current FROM app_versions WHERE id = ?`, versionID).Scan(&fileName, &isCurrent)
	if err != nil {
		return err
	}
	if isCurrent != 0 {
		return fmt.Errorf("不能删除当前生效的版本，请先切换到其他版本")
	}
	_, err = DB.Exec(`DELETE FROM app_versions WHERE id = ?`, versionID)
	return err
}

func GetCurrentVersion() (*AppVersion, error) {
	var v AppVersion
	var isCur int
	err := DB.QueryRow(`SELECT id, version, version_code, release_notes, file_name, file_size, is_current, uploaded_at FROM app_versions WHERE is_current = 1 ORDER BY version_code DESC LIMIT 1`).
		Scan(&v.ID, &v.Version, &v.VersionCode, &v.ReleaseNotes, &v.FileName, &v.FileSize, &isCur, &v.UploadedAt)
	if err != nil {
		return nil, err
	}
	v.IsCurrent = isCur != 0
	return &v, nil
}


// ── Toolbox User Config ──

type ToolboxUserConfig struct {
    UserID         int64  `json:"user_id"`
    Enabled        bool   `json:"enabled"`
    ExpiresAt      string `json:"expires_at"`
    PromptPurchased bool  `json:"prompt_purchased"`
    NamePurchased  bool   `json:"name_purchased"`
    CustomPrompt   string `json:"custom_prompt"`
    CustomName     string `json:"custom_name"`
    TrialUsed      bool   `json:"trial_used"`
    CreatedAt      string `json:"created_at"`
    UpdatedAt      string `json:"updated_at"`
    Email          string `json:"email,omitempty"`
    Username       string `json:"username,omitempty"`
}

type ToolboxDefaults struct {
    DefaultPrompt string `json:"default_prompt"`
    DefaultName   string `json:"default_name"`
    UpdatedAt     string `json:"updated_at"`
}

// Toolbox pricing constants (nuts)
const (
    ToolboxExtensionPricePer30Days = 100
    ToolboxPromptPrice            = 500
    ToolboxNamePrice              = 300
    ToolboxTrialDays              = 90
)

func GetToolboxConfig(userID int64) (*ToolboxUserConfig, error) {
    var c ToolboxUserConfig
    var enabled, promptPurchased, namePurchased, trialUsed int
    err := DB.QueryRow(`SELECT user_id, enabled, expires_at, prompt_purchased, name_purchased, custom_prompt, custom_name, trial_used, created_at, updated_at FROM toolbox_user_config WHERE user_id = ?`, userID).
        Scan(&c.UserID, &enabled, &c.ExpiresAt, &promptPurchased, &namePurchased, &c.CustomPrompt, &c.CustomName, &trialUsed, &c.CreatedAt, &c.UpdatedAt)
    if err != nil {
        return nil, err
    }
    c.Enabled = enabled != 0
    c.PromptPurchased = promptPurchased != 0
    c.NamePurchased = namePurchased != 0
    c.TrialUsed = trialUsed != 0
    return &c, nil
}

func UpsertToolboxConfig(c *ToolboxUserConfig) error {
    now := time.Now().UTC().Format(time.RFC3339)
    enabled := 0
    if c.Enabled { enabled = 1 }
    promptPur := 0
    if c.PromptPurchased { promptPur = 1 }
    namePur := 0
    if c.NamePurchased { namePur = 1 }
    trialUsed := 0
    if c.TrialUsed { trialUsed = 1 }

    _, err := DB.Exec(`INSERT INTO toolbox_user_config (user_id, enabled, expires_at, prompt_purchased, name_purchased, custom_prompt, custom_name, trial_used, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(user_id) DO UPDATE SET enabled=excluded.enabled, expires_at=excluded.expires_at, prompt_purchased=excluded.prompt_purchased, name_purchased=excluded.name_purchased, custom_prompt=excluded.custom_prompt, custom_name=excluded.custom_name, trial_used=excluded.trial_used, updated_at=excluded.updated_at`,
        c.UserID, enabled, c.ExpiresAt, promptPur, namePur, c.CustomPrompt, c.CustomName, trialUsed, now, now)
    return err
}

func ListAllToolboxConfigs() ([]*ToolboxUserConfig, error) {
    rows, err := DB.Query(`SELECT t.user_id, t.enabled, t.expires_at, t.prompt_purchased, t.name_purchased, t.custom_prompt, t.custom_name, t.trial_used, t.created_at, t.updated_at, u.email, u.username FROM toolbox_user_config t JOIN users u ON u.id = t.user_id ORDER BY t.updated_at DESC`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var list []*ToolboxUserConfig
    for rows.Next() {
        var c ToolboxUserConfig
        var enabled, promptPurchased, namePurchased, trialUsed int
        var email, username string
        if err := rows.Scan(&c.UserID, &enabled, &c.ExpiresAt, &promptPurchased, &namePurchased, &c.CustomPrompt, &c.CustomName, &trialUsed, &c.CreatedAt, &c.UpdatedAt, &email, &username); err != nil {
            continue
        }
        c.Enabled = enabled != 0
        c.PromptPurchased = promptPurchased != 0
        c.NamePurchased = namePurchased != 0
        c.TrialUsed = trialUsed != 0
        c.Email = email
        c.Username = username
        list = append(list, &c)
    }
    return list, nil
}

// GetToolboxDefaults returns system-wide defaults. Returns empty strings if no row.
func GetToolboxDefaults() (*ToolboxDefaults, error) {
    var d ToolboxDefaults
    err := DB.QueryRow(`SELECT COALESCE(default_prompt,''), COALESCE(default_name,'') FROM toolbox_defaults WHERE id = 1`).Scan(&d.DefaultPrompt, &d.DefaultName)
    if err != nil {
        return &ToolboxDefaults{}, nil
    }
    return &d, nil
}

func UpsertToolboxDefaults(d *ToolboxDefaults) error {
    now := time.Now().UTC().Format(time.RFC3339)
    _, err := DB.Exec(`INSERT INTO toolbox_defaults (id, default_prompt, default_name, updated_at) VALUES (1, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET default_prompt=excluded.default_prompt, default_name=excluded.default_name, updated_at=excluded.updated_at`,
        d.DefaultPrompt, d.DefaultName, now)
    return err
}

// StartTrial activates a one-time 3-month trial for a user
func StartTrial(userID int64) (*ToolboxUserConfig, error) {
    now := time.Now().UTC()
    // Check if trial already used
    existing, err := GetToolboxConfig(userID)
    if err == nil && existing.TrialUsed {
        return nil, fmt.Errorf("试用只能使用一次")
    }

    expiresAt := now.AddDate(0, 0, ToolboxTrialDays).Format(time.RFC3339)
    c := &ToolboxUserConfig{
        UserID:   userID,
        Enabled:  true,
        ExpiresAt: expiresAt,
        TrialUsed: true,
        CreatedAt: now.Format(time.RFC3339),
        UpdatedAt: now.Format(time.RFC3339),
    }
    if err := UpsertToolboxConfig(c); err != nil {
        return nil, err
    }
    return c, nil
}

// PurchaseToolboxFeature purchases a feature using nuts. feature: "prompt" or "name"
func PurchaseToolboxFeature(userID int64, feature string) error {
    var price int
    var column string
    switch feature {
    case "prompt":
        price = ToolboxPromptPrice
        column = "prompt_purchased"
    case "name":
        price = ToolboxNamePrice
        column = "name_purchased"
    default:
        return fmt.Errorf("未知功能: %s", feature)
    }

    tx, err := DB.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // Check if already purchased (idempotency guard)
    var alreadyPurchased int
    tx.QueryRow(
        fmt.Sprintf(`SELECT %s FROM toolbox_user_config WHERE user_id = ?`, column),
        userID,
    ).Scan(&alreadyPurchased)
    if alreadyPurchased != 0 {
        return fmt.Errorf("该功能已购买，无需重复购买")
    }

    // 订阅期内免费：跳过余额检查与扣费，直接授权
    subActive, _, _ := IsSubscriptionActive(userID)
    now := time.Now().UTC().Format(time.RFC3339)
    if !subActive {
        // Check balance
        var balance int
        err = tx.QueryRow(`SELECT nuts_balance FROM users WHERE id = ?`, userID).Scan(&balance)
        if err != nil {
            return fmt.Errorf("用户不存在")
        }
        if balance < price {
            return fmt.Errorf("板栗不足，需要 %d，当前 %d", price, balance)
        }

        // Deduct nuts
        err = tx.QueryRow(`UPDATE users SET nuts_balance = nuts_balance - ? WHERE id = ? RETURNING nuts_balance`, price, userID).Scan(&balance)
        if err != nil {
            return err
        }

        // Record transaction
        _, err = tx.Exec(`INSERT INTO nuts_transactions (user_id, amount, reason, balance_after, created_at) VALUES (?, ?, ?, ?, ?)`,
            userID, -price, fmt.Sprintf("购买工具箱功能: %s", feature), balance, now)
        if err != nil {
            return err
        }
    }

    // Grant feature
    _, err = tx.Exec(
        fmt.Sprintf(
            `INSERT INTO toolbox_user_config (user_id, %s, created_at, updated_at) VALUES (?, 1, ?, ?) ON CONFLICT(user_id) DO UPDATE SET %s=excluded.%s, updated_at=excluded.updated_at`,
            column, column, column,
        ),
        userID, now, now,
    )
    if err != nil {
        return err
    }

    return tx.Commit()
}

// ExtendToolboxAccess extends toolbox access by N days using nuts
func ExtendToolboxAccess(userID int64, days int) error {
    if days <= 0 {
        return fmt.Errorf("天数必须大于0")
    }
    // Price: ceil(days/30) * pricePer30Days
    units := (days + 29) / 30
    price := units * ToolboxExtensionPricePer30Days

    tx, err := DB.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // 订阅期内免费：跳过余额检查与扣费，直接续期
    subActive, _, _ := IsSubscriptionActive(userID)
    now := time.Now().UTC()
    nowStr := now.Format(time.RFC3339)
    if !subActive {
        // Check balance
        var balance int
        err = tx.QueryRow(`SELECT nuts_balance FROM users WHERE id = ?`, userID).Scan(&balance)
        if err != nil {
            return fmt.Errorf("用户不存在")
        }
        if balance < price {
            return fmt.Errorf("板栗不足，需要 %d，当前 %d", price, balance)
        }

        // Deduct nuts
        err = tx.QueryRow(`UPDATE users SET nuts_balance = nuts_balance - ? WHERE id = ? RETURNING nuts_balance`, price, userID).Scan(&balance)
        if err != nil {
            return err
        }

        // Record transaction
        _, err = tx.Exec(`INSERT INTO nuts_transactions (user_id, amount, reason, balance_after, created_at) VALUES (?, ?, ?, ?, ?)`,
            userID, -price, fmt.Sprintf("续期工具箱 %d 天", days), balance, nowStr)
        if err != nil {
            return err
        }
    }

    // Get current expiry
    var currentExpires string
    var enabled int
    row := tx.QueryRow(`SELECT enabled, expires_at FROM toolbox_user_config WHERE user_id = ?`, userID)
    row.Scan(&enabled, &currentExpires)

    var newExpires time.Time
    if enabled != 0 && currentExpires != "" {
        t, parseErr := time.Parse(time.RFC3339, currentExpires)
        if parseErr == nil && t.After(now) {
            // Extend from current expiry
            newExpires = t.AddDate(0, 0, days)
        } else {
            newExpires = now.AddDate(0, 0, days)
        }
    } else {
        newExpires = now.AddDate(0, 0, days)
    }

    // Hard cap: toolbox access never extends past 2099-12-31. Guards against
    // huge "days" values overflowing the time and producing an unparseable
    // expires_at (previously rendered as "Invalid Date" / 0 days remaining).
    if newExpires.Year() > 2099 {
        newExpires = time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)
    }
    expiresStr := newExpires.Format(time.RFC3339)
    _, err = tx.Exec(`INSERT INTO toolbox_user_config (user_id, enabled, expires_at, created_at, updated_at) VALUES (?, 1, ?, ?, ?) ON CONFLICT(user_id) DO UPDATE SET enabled=1, expires_at=excluded.expires_at, updated_at=excluded.updated_at`,
        userID, expiresStr, nowStr, nowStr)
    if err != nil {
        return err
    }

    return tx.Commit()
}


// ── Heartbeat + Push Notification ──

type UserStats struct {
	UserID             int64  `json:"user_id"`
	BlocksImported     int    `json:"blocks_imported"`
	BuildingsImported  int    `json:"buildings_imported"`
	BuildingsExported  int    `json:"buildings_exported"`
	MapartCompleted    int    `json:"mapart_completed"`
	SkinCompleted      int    `json:"skin_completed"`
	ImportSessions     int    `json:"import_sessions"`
	ExportSessions     int    `json:"export_sessions"`
	TotalOnlineMinutes int    `json:"total_online_minutes"`
	LastHeartbeatAt    string `json:"last_heartbeat_at"`
	LastServerCode     string `json:"last_server_code"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type PushMessage struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	Action       string `json:"action"`
	ActionData   string `json:"action_data"`
	DisplayMode  string `json:"display_mode"`
	TargetUserID *int64 `json:"target_user_id,omitempty"`
	TargetVersion string `json:"target_version"`
	Priority     int    `json:"priority"`
	CreatedAt    string `json:"created_at"`
	ExpiresAt    string `json:"expires_at"`
}

func HandleHeartbeat(userID int64, version, deviceID, deviceModel, serverCode string, online bool, inc IncrementData) (*HeartbeatResponse, error) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	// 计算在线时长：上次心跳到本次心跳的分钟数
	var prevHb string
	var prevMinutes int
	DB.QueryRow(`SELECT last_heartbeat_at, total_online_minutes FROM user_stats WHERE user_id = ?`, userID).Scan(&prevHb, &prevMinutes)
	onlineMinutes := prevMinutes
	if prevHb != "" {
		if t, err := time.Parse(time.RFC3339, prevHb); err == nil {
			diff := int(now.Sub(t).Minutes())
			if diff > 0 && diff < 60 { // 超过 1 小时不累加（防止异常值）
				onlineMinutes += diff
			}
		}
	}

	// Upsert user_stats
	_, err := DB.Exec(`INSERT INTO user_stats (user_id, blocks_imported, buildings_imported, buildings_exported, mapart_completed, skin_completed, import_sessions, export_sessions, total_online_minutes, last_heartbeat_at, last_server_code, device_model, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(user_id) DO UPDATE SET
		blocks_imported = blocks_imported + excluded.blocks_imported,
		buildings_imported = buildings_imported + excluded.buildings_imported,
		buildings_exported = buildings_exported + excluded.buildings_exported,
		mapart_completed = mapart_completed + excluded.mapart_completed,
		skin_completed = skin_completed + excluded.skin_completed,
		import_sessions = import_sessions + excluded.import_sessions,
		export_sessions = export_sessions + excluded.export_sessions,
		total_online_minutes = excluded.total_online_minutes,
		last_heartbeat_at = excluded.last_heartbeat_at,
		last_server_code = excluded.last_server_code,
		device_model = excluded.device_model,
		updated_at = excluded.updated_at`,
		userID, inc.BlocksImported, inc.BuildingsImported, inc.BuildingsExported,
		inc.MapartCompleted, inc.SkinCompleted, inc.ImportSessions, inc.ExportSessions,
		onlineMinutes, nowStr, serverCode, deviceModel, nowStr, nowStr)
	if err != nil {
		return nil, err
	}

	// Log heartbeat
	DB.Exec(`INSERT INTO heartbeat_logs (user_id, server_code, device_model, app_version, blocks_imported, buildings_imported, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, serverCode, deviceModel, version, inc.BlocksImported, inc.BuildingsImported, nowStr)

	// Get current stats
	stats := &UserStats{}
	row := DB.QueryRow(`SELECT blocks_imported, buildings_imported, buildings_exported, mapart_completed, skin_completed, import_sessions, export_sessions, total_online_minutes FROM user_stats WHERE user_id = ?`, userID)
	row.Scan(&stats.BlocksImported, &stats.BuildingsImported, &stats.BuildingsExported, &stats.MapartCompleted, &stats.SkinCompleted, &stats.ImportSessions, &stats.ExportSessions, &stats.TotalOnlineMinutes)

	// Get pending push messages
	msgs, _ := GetPendingPushMessages(userID)

	return &HeartbeatResponse{
		ServerTime:      now.Unix(),
		HeartbeatInterval: 15,
		PendingMessages: msgs,
		Stats:           stats,
	}, nil
}

type IncrementData struct {
	BlocksImported    int `json:"blocks_imported"`
	BuildingsImported int `json:"buildings_imported"`
	BuildingsExported int `json:"buildings_exported"`
	MapartCompleted   int `json:"mapart_completed"`
	SkinCompleted     int `json:"skin_completed"`
	ImportSessions    int `json:"import_sessions"`
	ExportSessions    int `json:"export_sessions"`
}

type HeartbeatResponse struct {
	ServerTime       int64         `json:"server_time"`
	HeartbeatInterval int          `json:"heartbeat_interval"`
	PendingMessages  []PushMessage `json:"pending_messages"`
	Stats            *UserStats    `json:"stats"`
}

func GetPendingPushMessages(userID int64) ([]PushMessage, error) {
	rows, err := DB.Query(`SELECT id, type, title, body, action, action_data, display_mode, target_user_id, target_version, priority, created_at, expires_at FROM push_messages WHERE (target_user_id IS NULL OR target_user_id = ?) AND (expires_at = '' OR expires_at > ?) AND id NOT IN (SELECT msg_id FROM user_push_reads WHERE user_id = ?) ORDER BY priority DESC, created_at DESC LIMIT 50`,
		userID, time.Now().UTC().Format(time.RFC3339), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PushMessage
	for rows.Next() {
		var m PushMessage
		rows.Scan(&m.ID, &m.Type, &m.Title, &m.Body, &m.Action, &m.ActionData, &m.DisplayMode, &m.TargetUserID, &m.TargetVersion, &m.Priority, &m.CreatedAt, &m.ExpiresAt)
		list = append(list, m)
	}
	return list, nil
}

func CreatePushMessage(m *PushMessage) error {
	_, err := DB.Exec(`INSERT INTO push_messages (id, type, title, body, action, action_data, display_mode, target_user_id, target_version, priority, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.Type, m.Title, m.Body, m.Action, m.ActionData, m.DisplayMode, m.TargetUserID, m.TargetVersion, m.Priority, m.CreatedAt, m.ExpiresAt)
	return err
}

func DeletePushMessage(id string) error {
	_, err := DB.Exec(`DELETE FROM push_messages WHERE id = ?`, id)
	return err
}

func ListPushMessages(limit, offset int) ([]PushMessage, error) {
	rows, err := DB.Query(`SELECT id, type, title, body, action, action_data, display_mode, target_user_id, target_version, priority, created_at, expires_at FROM push_messages ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PushMessage
	for rows.Next() {
		var m PushMessage
		rows.Scan(&m.ID, &m.Type, &m.Title, &m.Body, &m.Action, &m.ActionData, &m.DisplayMode, &m.TargetUserID, &m.TargetVersion, &m.Priority, &m.CreatedAt, &m.ExpiresAt)
		list = append(list, m)
	}
	return list, nil
}

func AckPushMessages(userID int64, msgIDs []string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, id := range msgIDs {
		_, err := DB.Exec(`INSERT OR IGNORE INTO user_push_reads (user_id, msg_id, read_at) VALUES (?, ?, ?)`, userID, id, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func GetUserStatsOverview() (map[string]int64, error) {
	result := map[string]int64{}
	var totalUsers, activeToday int64
	DB.QueryRow(`SELECT COUNT(*) FROM user_stats`).Scan(&totalUsers)
	DB.QueryRow(`SELECT COUNT(*) FROM user_stats WHERE last_heartbeat_at > ?`, time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339)).Scan(&activeToday)
	result["total_users"] = totalUsers
	result["active_today"] = activeToday

	var bi, bdi, bde, mc, sc int64
	DB.QueryRow(`SELECT COALESCE(SUM(blocks_imported),0), COALESCE(SUM(buildings_imported),0), COALESCE(SUM(buildings_exported),0), COALESCE(SUM(mapart_completed),0), COALESCE(SUM(skin_completed),0) FROM user_stats`).Scan(&bi, &bdi, &bde, &mc, &sc)
	result["blocks_imported"] = bi
	result["buildings_imported"] = bdi
	result["buildings_exported"] = bde
	result["mapart_completed"] = mc
	result["skin_completed"] = sc
	return result, nil
}
func GetOnlineUsers() ([]map[string]interface{}, error) {
	rows, err := DB.Query(`SELECT s.user_id, COALESCE(u.email,''), COALESCE(u.username,''), s.blocks_imported, s.buildings_imported, s.buildings_exported, s.mapart_completed, s.skin_completed, s.import_sessions, s.export_sessions, s.total_online_minutes, s.last_heartbeat_at, s.last_server_code, s.device_model FROM user_stats s LEFT JOIN users u ON u.id = s.user_id WHERE s.last_heartbeat_at > ? ORDER BY s.last_heartbeat_at DESC`, time.Now().UTC().Add(-30*time.Second).Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []map[string]interface{}
	for rows.Next() {
		var uID int64
		var email, username, lastHb, lastSvr string
		var bi, bdi, bde, mc, sc, ims, exs, tom int
		var deviceModel string
		rows.Scan(&uID, &email, &username, &bi, &bdi, &bde, &mc, &sc, &ims, &exs, &tom, &lastHb, &lastSvr, &deviceModel)
		displayName := username
		if uID < 0 || (username == "" && email == "") {
			displayName = fmt.Sprintf("匿名设备 #%d", -uID)
		}
		list = append(list, map[string]interface{}{
			"user_id": uID, "email": email, "username": displayName,
			"blocks_imported": bi, "buildings_imported": bdi, "buildings_exported": bde,
			"mapart_completed": mc, "skin_completed": sc,
			"import_sessions": ims, "export_sessions": exs,
			"total_online_minutes": tom,
			"last_heartbeat_at": lastHb, "last_server_code": lastSvr, "device_model": deviceModel,
		})
	}
	return list, nil
}

type UserStatsDetail struct {
	UserID             int64  `json:"user_id"`
	Email              string `json:"email"`
	Username           string `json:"username"`
	BlocksImported     int    `json:"blocks_imported"`
	BuildingsImported  int    `json:"buildings_imported"`
	BuildingsExported  int    `json:"buildings_exported"`
	MapartCompleted    int    `json:"mapart_completed"`
	SkinCompleted      int    `json:"skin_completed"`
	ImportSessions     int    `json:"import_sessions"`
	ExportSessions     int    `json:"export_sessions"`
	TotalOnlineMinutes int    `json:"total_online_minutes"`
	LastHeartbeatAt    string `json:"last_heartbeat_at"`
	LastServerCode     string `json:"last_server_code"`
	DeviceModel        string `json:"device_model"`
}

func GetUserStatsDetail(userID int64) (*UserStatsDetail, error) {
	d := &UserStatsDetail{}
	// 匿名设备（负数 user_id）没有 users 记录，直接查 user_stats
	if userID < 0 {
		err := DB.QueryRow(`SELECT s.user_id, '', '', COALESCE(s.blocks_imported,0), COALESCE(s.buildings_imported,0), COALESCE(s.buildings_exported,0), COALESCE(s.mapart_completed,0), COALESCE(s.skin_completed,0), COALESCE(s.import_sessions,0), COALESCE(s.export_sessions,0), COALESCE(s.total_online_minutes,0), COALESCE(s.last_heartbeat_at,''), COALESCE(s.last_server_code,''), COALESCE(s.device_model,'') FROM user_stats s WHERE s.user_id = ?`, userID).
			Scan(&d.UserID, &d.Email, &d.Username, &d.BlocksImported, &d.BuildingsImported, &d.BuildingsExported, &d.MapartCompleted, &d.SkinCompleted, &d.ImportSessions, &d.ExportSessions, &d.TotalOnlineMinutes, &d.LastHeartbeatAt, &d.LastServerCode, &d.DeviceModel)
		if err != nil {
			return nil, err
		}
		d.Username = fmt.Sprintf("匿名设备 #%d", -userID)
		return d, nil
	}
	d = &UserStatsDetail{}
	err := DB.QueryRow(`SELECT u.id, u.email, u.username, COALESCE(s.blocks_imported,0), COALESCE(s.buildings_imported,0), COALESCE(s.buildings_exported,0), COALESCE(s.mapart_completed,0), COALESCE(s.skin_completed,0), COALESCE(s.import_sessions,0), COALESCE(s.export_sessions,0), COALESCE(s.total_online_minutes,0), COALESCE(s.last_heartbeat_at,''), COALESCE(s.last_server_code,''), COALESCE(s.device_model,'') FROM users u LEFT JOIN user_stats s ON s.user_id = u.id WHERE u.id = ?`, userID).
		Scan(&d.UserID, &d.Email, &d.Username, &d.BlocksImported, &d.BuildingsImported, &d.BuildingsExported, &d.MapartCompleted, &d.SkinCompleted, &d.ImportSessions, &d.ExportSessions, &d.TotalOnlineMinutes, &d.LastHeartbeatAt, &d.LastServerCode, &d.DeviceModel)
	if err != nil {
		return nil, err
	}
	return d, nil
}

