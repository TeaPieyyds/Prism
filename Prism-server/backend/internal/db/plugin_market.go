package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ────────────────────────────────────────────────────────────
// 插件市场上传会话（TTS）
// ────────────────────────────────────────────────────────────

type PluginMarketUploadSession struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	FileMD5   string `json:"file_md5"`
	FileSize  int64  `json:"file_size"`
	PricePts  int    `json:"price_pts"`
	ExpiresAt string `json:"expires_at"`
	Used      bool   `json:"used"`
	CreatedAt string `json:"created_at"`
}

func CreatePluginMarketUploadSession(ttsHash string, userID int64, fileMD5 string, fileSize int64, price int) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	exp := time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339)
	res, err := DB.Exec(`INSERT INTO plugin_market_upload_sessions (tts_hash, user_id, file_md5, file_size, price_pts, expires_at, used, created_at) VALUES (?,?,?,?,?,?,0,?)`,
		ttsHash, userID, fileMD5, fileSize, price, exp, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func GetPluginMarketUploadSession(ttsHash string) (*PluginMarketUploadSession, error) {
	row := DB.QueryRow(`SELECT id, user_id, file_md5, file_size, price_pts, expires_at, used, created_at FROM plugin_market_upload_sessions WHERE tts_hash = ?`, ttsHash)
	var s PluginMarketUploadSession
	var used int
	if err := row.Scan(&s.ID, &s.UserID, &s.FileMD5, &s.FileSize, &s.PricePts, &s.ExpiresAt, &used, &s.CreatedAt); err != nil {
		return nil, err
	}
	s.Used = used != 0
	return &s, nil
}

func MarkPluginMarketUploadSessionUsed(id int64) error {
	_, err := DB.Exec(`UPDATE plugin_market_upload_sessions SET used = 1 WHERE id = ?`, id)
	return err
}

func DeletePluginMarketUploadSession(id int64) error {
	_, err := DB.Exec(`DELETE FROM plugin_market_upload_sessions WHERE id = ?`, id)
	return err
}

func PrunePluginMarketUploadSessions() error {
	_, err := DB.Exec(`DELETE FROM plugin_market_upload_sessions WHERE used = 1 OR expires_at < ?`, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ────────────────────────────────────────────────────────────
// 配置（单行 id=1）
// ────────────────────────────────────────────────────────────

type PluginMarketConfig struct {
	UploadRewardNuts   int `json:"upload_reward_nuts"`
	ReportAutoTakedown int `json:"report_auto_takedown"`
}

func GetPluginMarketConfig() (*PluginMarketConfig, error) {
	var c PluginMarketConfig
	err := DB.QueryRow(`SELECT upload_reward_nuts, report_auto_takedown FROM plugin_market_config WHERE id = 1`).Scan(&c.UploadRewardNuts, &c.ReportAutoTakedown)
	if err != nil {
		return &PluginMarketConfig{UploadRewardNuts: 10, ReportAutoTakedown: 3}, nil
	}
	return &c, nil
}

func UpdatePluginMarketConfig(c *PluginMarketConfig) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := DB.Exec(`INSERT INTO plugin_market_config (id, upload_reward_nuts, report_auto_takedown, updated_at) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET upload_reward_nuts=excluded.upload_reward_nuts, report_auto_takedown=excluded.report_auto_takedown, updated_at=excluded.updated_at`,
		c.UploadRewardNuts, c.ReportAutoTakedown, now)
	return err
}

// ────────────────────────────────────────────────────────────
// 分类
// ────────────────────────────────────────────────────────────

type PluginMarketCategory struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

func ListPluginMarketCategories() ([]PluginMarketCategory, error) {
	rows, err := DB.Query(`SELECT id, name, sort_order FROM plugin_market_categories ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PluginMarketCategory{}
	for rows.Next() {
		var c PluginMarketCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func CreatePluginMarketCategory(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("分类名不能为空")
	}
	_, err := DB.Exec(`INSERT INTO plugin_market_categories (name, sort_order, created_at) VALUES (?, 0, ?)`, name, time.Now().UTC().Format(time.RFC3339))
	return err
}

func RenamePluginMarketCategory(id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("分类名不能为空")
	}
	res, err := DB.Exec(`UPDATE plugin_market_categories SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("分类不存在")
	}
	return nil
}

func DeletePluginMarketCategory(id int64) error {
	res, err := DB.Exec(`DELETE FROM plugin_market_categories WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("分类不存在")
	}
	return nil
}

// sqlTx 事务接口别名（供后续任务复用）
type sqlTx interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

func insertPluginFileCategoriesTx(tx sqlTx, fileID int64, names []string) error {
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		var cid int64
		if err := tx.QueryRow(`SELECT id FROM plugin_market_categories WHERE name = ?`, n).Scan(&cid); err != nil {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO plugin_market_file_categories (file_id, category_id) VALUES (?,?)`, fileID, cid); err != nil {
			return err
		}
	}
	return nil
}

// ────────────────────────────────────────────────────────────
// 文件 / 版本 / parts
// ────────────────────────────────────────────────────────────

type PluginMarketPart struct {
	ID         int64  `json:"id"`
	FileID     int64  `json:"file_id"`
	PartType   string `json:"part_type"`
	OrigName   string `json:"orig_name"`
	StoredName string `json:"-"`
	Ext        string `json:"ext"`
	RelPath    string `json:"-"`
	MD5        string `json:"-"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	Size       int64  `json:"size"`
	CreatedAt  string `json:"created_at"`
}

type PluginMarketVersion struct {
	ID          int64  `json:"id"`
	FileID      int64  `json:"file_id"`
	Version     string `json:"version"`
	Changelog   string `json:"changelog"`
	ZipPartID   int64  `json:"zip_part_id"`
	ZipHash     string `json:"-"`
	NormHash    string `json:"-"`
	MD5         string `json:"-"`
	FileSize    int64  `json:"file_size"`
	ReviewState int    `json:"review_state"`
	DedupFlag   int    `json:"dedup_flag"`
	CreatedAt   string `json:"created_at"`
}

type PluginMarketFile struct {
	ID            int64          `json:"id"`
	PluginID      string         `json:"plugin_id"`
	UploaderID    int64          `json:"uploader_id"`
	UploaderName  string         `json:"uploader_name,omitempty"`
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	Type          string         `json:"type"`
	Description   string         `json:"description,omitempty"`
	Docs          string         `json:"docs,omitempty"`
	Categories    []string       `json:"categories"`
	Tags          []string       `json:"tags"`
	DeclaredCaps  []string       `json:"declared_caps"`
	ScanResult    []string       `json:"scan_result"`
	RiskLevel     int            `json:"risk_level"`
	PricePts      int            `json:"price_pts"`
	AllowAnon     bool           `json:"allow_anonymous"`
	CardColor     string         `json:"card_color"`
	ZipHash       string         `json:"-"`
	NormHash      string         `json:"-"`
	MD5           string         `json:"-"`
	FileSize      int64          `json:"file_size"`
	DownloadCount int            `json:"download_count"`
	ReviewState   int            `json:"review_state"`
	DedupFlag     int            `json:"dedup_flag"`
	FlagReason    string         `json:"flag_reason,omitempty"`
	CoverPartID   int64          `json:"cover_part_id"`
	PartDir       string         `json:"-"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
}

const pluginMarketFileSel = "f.id, f.plugin_id, f.uploader_id, f.name, f.version, f.type, f.description, f.docs, f.categories, f.tags, f.declared_caps, f.scan_result, f.risk_level, f.price_pts, f.allow_anonymous, f.card_color, f.zip_hash, f.norm_hash, f.md5, f.file_size, f.download_count, f.review_state, f.dedup_flag, f.flag_reason, f.cover_part_id, f.part_dir, f.created_at, f.updated_at"

func scanPluginMarketFile(row interface{ Scan(...any) error }) (*PluginMarketFile, error) {
	var f PluginMarketFile
	var cats, tags, caps, scanRaw string
	var anon int
	err := row.Scan(
		&f.ID, &f.PluginID, &f.UploaderID, &f.Name, &f.Version, &f.Type, &f.Description, &f.Docs,
		&cats, &tags, &caps, &scanRaw, &f.RiskLevel, &f.PricePts, &anon, &f.CardColor,
		&f.ZipHash, &f.NormHash, &f.MD5, &f.FileSize, &f.DownloadCount, &f.ReviewState,
		&f.DedupFlag, &f.FlagReason, &f.CoverPartID, &f.PartDir, &f.CreatedAt, &f.UpdatedAt,
		&f.UploaderName,
	)
	if err != nil {
		return nil, err
	}
	f.AllowAnon = anon != 0
	json.Unmarshal([]byte(cats), &f.Categories)
	json.Unmarshal([]byte(tags), &f.Tags)
	json.Unmarshal([]byte(caps), &f.DeclaredCaps)
	json.Unmarshal([]byte(scanRaw), &f.ScanResult)
	return &f, nil
}

// CreatePluginMarketFile 单事务创建 文件 + 分类关联 + 版本 + parts。
func CreatePluginMarketFile(f *PluginMarketFile, parts []PluginMarketPart, v PluginMarketVersion) (int64, error) {
	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	cats, _ := json.Marshal(f.Categories)
	tags, _ := json.Marshal(f.Tags)
	caps, _ := json.Marshal(f.DeclaredCaps)
	scanRaw, _ := json.Marshal(f.ScanResult)
	anon := 0
	if f.AllowAnon {
		anon = 1
	}
	res, err := tx.Exec(`INSERT INTO plugin_market_files (plugin_id, uploader_id, name, version, type, description, docs, categories, tags, declared_caps, scan_result, risk_level, price_pts, allow_anonymous, card_color, zip_hash, norm_hash, md5, file_size, download_count, review_state, dedup_flag, flag_reason, cover_part_id, part_dir, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,?,?,?,?,?,?,?)`,
		f.PluginID, f.UploaderID, f.Name, f.Version, f.Type, f.Description, f.Docs, string(cats), string(tags), string(caps), string(scanRaw), f.RiskLevel, f.PricePts, anon, f.CardColor, f.ZipHash, f.NormHash, f.MD5, f.FileSize, f.ReviewState, f.DedupFlag, f.FlagReason, f.CoverPartID, f.PartDir, now, now)
	if err != nil {
		return 0, err
	}
	fileID, _ := res.LastInsertId()
	if err := insertPluginFileCategoriesTx(tx, fileID, f.Categories); err != nil {
		return 0, err
	}
	v.FileID = fileID
	if err := insertPluginVersionTx(tx, v, parts); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return fileID, nil
}

func insertPluginVersionTx(tx sqlTx, v PluginMarketVersion, parts []PluginMarketPart) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.Exec(`INSERT INTO plugin_market_versions (file_id, version, changelog, zip_part_id, zip_hash, norm_hash, md5, file_size, review_state, dedup_flag, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		v.FileID, v.Version, v.Changelog, 0, v.ZipHash, v.NormHash, v.MD5, v.FileSize, v.ReviewState, v.DedupFlag, now)
	if err != nil {
		return err
	}
	verID, _ := res.LastInsertId()
	for _, p := range parts {
		if _, err := tx.Exec(`INSERT INTO plugin_market_parts (file_id, part_type, orig_name, stored_name, ext, rel_path, md5, width, height, size, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			v.FileID, p.PartType, p.OrigName, p.StoredName, p.Ext, p.RelPath, p.MD5, p.Width, p.Height, p.Size, now); err != nil {
			return err
		}
	}
	var pid int64
	tx.QueryRow(`SELECT id FROM plugin_market_parts WHERE file_id = ? AND part_type = 'plugin_zip' ORDER BY id LIMIT 1`, v.FileID).Scan(&pid)
	if pid > 0 {
		tx.Exec(`UPDATE plugin_market_versions SET zip_part_id = ? WHERE id = ?`, pid, verID)
	}
	return nil
}

// AppendPluginMarketVersion 为既有插件追加一个版本（版本更新复用 file_id）。
// storedName 为已落盘到 part_dir 的新 zip 文件名。
func AppendPluginMarketVersion(fileID int64, version, changelog, storedName, zipHash, normHash, md5 string, fileSize int64, dedupFlag int, flagReason string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.Exec(`INSERT INTO plugin_market_parts (file_id, part_type, orig_name, stored_name, ext, rel_path, md5, width, height, size, created_at) VALUES (?,?,?,?,?,?,?,0,0,?,?)`,
		fileID, "plugin_zip", storedName, storedName, ".zip", storedName, md5, fileSize, now)
	if err != nil {
		return err
	}
	pid, _ := res.LastInsertId()
	if _, err := tx.Exec(`INSERT INTO plugin_market_versions (file_id, version, changelog, zip_part_id, zip_hash, norm_hash, md5, file_size, review_state, dedup_flag, created_at) VALUES (?,?,?,?,?,?,?,?,0,?,?)`,
		fileID, version, changelog, pid, zipHash, normHash, md5, fileSize, dedupFlag, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE plugin_market_files SET version = ?, zip_hash = ?, norm_hash = ?, md5 = ?, file_size = ?, review_state = 0, dedup_flag = ?, flag_reason = ?, updated_at = ? WHERE id = ?`,
		version, zipHash, normHash, md5, fileSize, dedupFlag, flagReason, now, fileID); err != nil {
		return err
	}
	return tx.Commit()
}

func GetLatestPluginVersion(fileID int64) (*PluginMarketVersion, error) {
	row := DB.QueryRow(`SELECT id, file_id, version, changelog, zip_part_id, zip_hash, norm_hash, md5, file_size, review_state, dedup_flag, created_at FROM plugin_market_versions WHERE file_id = ? ORDER BY id DESC LIMIT 1`, fileID)
	return scanPluginVersion(row)
}

func scanPluginVersion(row interface{ Scan(...any) error }) (*PluginMarketVersion, error) {
	var v PluginMarketVersion
	err := row.Scan(&v.ID, &v.FileID, &v.Version, &v.Changelog, &v.ZipPartID, &v.ZipHash, &v.NormHash, &v.MD5, &v.FileSize, &v.ReviewState, &v.DedupFlag, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func ListPluginVersions(fileID int64) ([]PluginMarketVersion, error) {
	rows, err := DB.Query(`SELECT id, file_id, version, changelog, zip_part_id, zip_hash, norm_hash, md5, file_size, review_state, dedup_flag, created_at FROM plugin_market_versions WHERE file_id = ? ORDER BY id DESC`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PluginMarketVersion{}
	for rows.Next() {
		v, err := scanPluginVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func GetPluginMarketFile(id int64) (*PluginMarketFile, error) {
	f, err := scanPluginMarketFile(DB.QueryRow(`SELECT `+pluginMarketFileSel+`,COALESCE(u.username,'') FROM plugin_market_files f LEFT JOIN users u ON u.id = f.uploader_id WHERE f.id = ?`, id))
	if err != nil {
		return nil, err
	}
	f.Categories = pluginMarketFileCategories(id)
	return f, nil
}

func FindPluginByNormHash(normHash string) (*PluginMarketFile, error) {
	if normHash == "" {
		return nil, nil
	}
	f, err := scanPluginMarketFile(DB.QueryRow(`SELECT `+pluginMarketFileSel+`,COALESCE(u.username,'') FROM plugin_market_files f LEFT JOIN users u ON u.id=f.uploader_id WHERE f.norm_hash = ? LIMIT 1`, normHash))
	if err != nil {
		return nil, err
	}
	return f, nil
}

func FindPluginByPluginID(pluginID string, uploaderID int64) (*PluginMarketFile, error) {
	if pluginID == "" {
		return nil, nil
	}
	f, err := scanPluginMarketFile(DB.QueryRow(`SELECT `+pluginMarketFileSel+`,COALESCE(u.username,'') FROM plugin_market_files f LEFT JOIN users u ON u.id=f.uploader_id WHERE f.plugin_id = ? AND f.uploader_id = ? LIMIT 1`, pluginID, uploaderID))
	if err != nil {
		return nil, err
	}
	return f, nil
}

func pluginMarketFileCategories(fileID int64) []string {
	rows, err := DB.Query(`SELECT c.name FROM plugin_market_file_categories fc JOIN plugin_market_categories c ON c.id = fc.category_id WHERE fc.file_id = ? ORDER BY c.sort_order`, fileID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// ListPluginMarketFiles 分页 + 分类/标签/关键词 + 审核状态过滤。order: new|hot。
func ListPluginMarketFiles(page, size int, category, tag, order, q string, reviewState *int) ([]PluginMarketFile, int, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 20
	}
	where := []string{"f.review_state = 0 OR f.review_state = 1"}
	args := []any{}
	if reviewState != nil {
		where = []string{"f.review_state = ?"}
		args = append(args, *reviewState)
	}
	if category != "" {
		if category == "其他" {
			where = append(where, `(EXISTS(SELECT 1 FROM plugin_market_file_categories fc JOIN plugin_market_categories c ON c.id=fc.category_id WHERE fc.file_id=f.id AND c.name='其他')
				OR NOT EXISTS(SELECT 1 FROM plugin_market_file_categories fc WHERE fc.file_id=f.id))`)
		} else {
			where = append(where, `EXISTS(SELECT 1 FROM plugin_market_file_categories fc JOIN plugin_market_categories c ON c.id=fc.category_id WHERE fc.file_id=f.id AND c.name=?)`)
			args = append(args, category)
		}
	}
	if tag != "" {
		where = append(where, "f.tags LIKE ?")
		args = append(args, "%\""+escapeLike(tag)+"\"%")
	}
	if q != "" {
		lq := "%" + escapeLike(q) + "%"
		where = append(where, "(f.name LIKE ? OR f.description LIKE ? OR f.tags LIKE ?)")
		args = append(args, lq, lq, lq)
	}
	cond := "WHERE " + strings.Join(where, " AND ")
	orderBy := "f.created_at DESC"
	switch order {
	case "hot":
		orderBy = "f.download_count DESC, f.created_at DESC"
	case "old":
		orderBy = "f.created_at ASC"
	case "download_asc":
		orderBy = "f.download_count ASC, f.created_at DESC"
	case "size_desc":
		orderBy = "f.file_size DESC, f.created_at DESC"
	case "size_asc":
		orderBy = "f.file_size ASC, f.created_at DESC"
	}
	var total int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM plugin_market_files f `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	off := (page - 1) * size
	args = append(args, size, off)
	rows, err := DB.Query(`SELECT `+pluginMarketFileSel+`,COALESCE(u.username,'') FROM plugin_market_files f LEFT JOIN users u ON u.id=f.uploader_id `+cond+` ORDER BY `+orderBy+` LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []PluginMarketFile{}
	for rows.Next() {
		f, err := scanPluginMarketFile(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *f)
	}
	for i := range list {
		list[i].Categories = pluginMarketFileCategories(list[i].ID)
		if list[i].CoverPartID == 0 {
			list[i].CoverPartID = pluginMarketCoverPartID(list[i].ID)
		}
	}
	return list, total, rows.Err()
}

func pluginMarketCoverPartID(fileID int64) int64 {
	var pid int64
	DB.QueryRow(`SELECT id FROM plugin_market_parts WHERE file_id = ? AND part_type = 'preview_img' ORDER BY id LIMIT 1`, fileID).Scan(&pid)
	return pid
}

// GetPluginMarketPart 取某 part（下载用）。
func GetPluginMarketPart(id int64) (*PluginMarketPart, error) {
	var p PluginMarketPart
	err := DB.QueryRow(`SELECT id, file_id, part_type, orig_name, stored_name, ext, rel_path, md5, width, height, size, created_at FROM plugin_market_parts WHERE id = ?`, id).
		Scan(&p.ID, &p.FileID, &p.PartType, &p.OrigName, &p.StoredName, &p.Ext, &p.RelPath, &p.MD5, &p.Width, &p.Height, &p.Size, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ────────────────────────────────────────────────────────────
// 下载（付费） / 上传奖励 / 举报
// ────────────────────────────────────────────────────────────

// PurchaseOrReusePlugin 付费下载事务（幂等防重复扣费）。sellerID==buyerID 时跳过记账仅计数。
func PurchaseOrReusePlugin(fileID, buyerID int64, price int, sellerID int64) (bool, error) {
	tx, err := DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.Exec(`INSERT OR IGNORE INTO plugin_market_downloads (file_id, user_id, price, created_at) VALUES (?,?,?,?)`, fileID, buyerID, price, now)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, tx.Commit()
	}
	if price > 0 {
		if subActive, _, _ := IsSubscriptionActive(buyerID); !subActive {
			var bal int
			if err := tx.QueryRow(`UPDATE users SET nuts_balance = nuts_balance - ? WHERE id = ? RETURNING nuts_balance`, price, buyerID).Scan(&bal); err != nil || bal < 0 {
				return false, fmt.Errorf("板栗不足,需要 %d", price)
			}
			if _, err := tx.Exec(`INSERT INTO nuts_transactions (user_id, amount, reason, ref_user_id, balance_after, created_at) VALUES (?,?,?,?,?,?)`,
				buyerID, -price, "下载插件", &sellerID, bal, now); err != nil {
				return false, err
			}
			if sellerID != buyerID {
				var sbal int
				if err := tx.QueryRow(`UPDATE users SET nuts_balance = nuts_balance + ? WHERE id = ? RETURNING nuts_balance`, price, sellerID).Scan(&sbal); err != nil {
					return false, err
				}
				if _, err := tx.Exec(`INSERT INTO nuts_transactions (user_id, amount, reason, ref_user_id, balance_after, created_at) VALUES (?,?,?,?,?,?)`,
					sellerID, price, "售出插件", &buyerID, sbal, now); err != nil {
					return false, err
				}
			}
		}
	}
	if _, err := tx.Exec(`UPDATE plugin_market_files SET download_count = download_count + 1 WHERE id = ?`, fileID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func RecordFreePluginDownload(fileID int64) error {
	_, err := DB.Exec(`UPDATE plugin_market_files SET download_count = download_count + 1 WHERE id = ?`, fileID)
	return err
}

func UserHasPurchasedPlugin(fileID, userID int64) (bool, error) {
	var n int
	err := DB.QueryRow(`SELECT COUNT(*) FROM plugin_market_downloads WHERE file_id = ? AND user_id = ?`, fileID, userID).Scan(&n)
	return n > 0, err
}

func CreditPluginUploadReward(userID int64) error {
	c, err := GetPluginMarketConfig()
	if err != nil {
		return err
	}
	if c.UploadRewardNuts <= 0 {
		return nil
	}
	_, err = AddNuts(userID, c.UploadRewardNuts, "上传插件", nil, nil)
	return err
}

// AddPluginMarketReport 记录举报；超过阈值自动下架（review_state=2）。返回是否自动下架。
func AddPluginMarketReport(fileID, reporterID int64, reason string) (bool, error) {
	tx, err := DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.Exec(`INSERT OR IGNORE INTO plugin_market_reports (file_id, reporter_id, reason, created_at) VALUES (?,?,?,?)`, fileID, reporterID, reason, now)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, fmt.Errorf("你已经举报过这个插件了~")
	}
	var count int
	tx.QueryRow(`SELECT COUNT(*) FROM plugin_market_reports WHERE file_id = ?`, fileID).Scan(&count)
	c, _ := GetPluginMarketConfig()
	th := c.ReportAutoTakedown
	if th < 1 {
		th = 3
	}
	auto := count > th
	state := 0
	if auto {
		state = 2
	}
	if _, err := tx.Exec(`UPDATE plugin_market_files SET dedup_flag = 1, flag_reason = ?, review_state = ? WHERE id = ?`, reason, state, fileID); err != nil {
		return false, err
	}
	return auto, tx.Commit()
}

// ────────────────────────────────────────────────────────────
// 评论
// ────────────────────────────────────────────────────────────

type PluginMarketComment struct {
	ID        int64  `json:"id"`
	FileID    int64  `json:"file_id"`
	ParentID  int64  `json:"parent_id"`
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	IsAuthor  bool   `json:"is_author"`
	Mine      bool   `json:"mine"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

func CreatePluginMarketComment(fileID, parentID, userID int64, content string) (int64, error) {
	if parentID > 0 {
		var fid int64
		if err := DB.QueryRow(`SELECT file_id FROM plugin_market_comments WHERE id = ?`, parentID).Scan(&fid); err != nil || fid != fileID {
			return 0, fmt.Errorf("回复的评论不存在~")
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := DB.Exec(`INSERT INTO plugin_market_comments (file_id, parent_id, user_id, content, created_at) VALUES (?,?,?,?,?)`, fileID, parentID, userID, content, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func ListPluginMarketComments(fileID, currentUserID int64) ([]PluginMarketComment, error) {
	var uploaderID int64
	DB.QueryRow(`SELECT uploader_id FROM plugin_market_files WHERE id = ?`, fileID).Scan(&uploaderID)
	rows, err := DB.Query(`SELECT c.id, c.file_id, c.parent_id, c.user_id, COALESCE(u.username,''), c.content, c.created_at
		FROM plugin_market_comments c LEFT JOIN users u ON u.id = c.user_id WHERE c.file_id = ? ORDER BY c.id`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PluginMarketComment{}
	for rows.Next() {
		var c PluginMarketComment
		if err := rows.Scan(&c.ID, &c.FileID, &c.ParentID, &c.UserID, &c.Username, &c.Content, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.IsAuthor = c.UserID == uploaderID
		c.Mine = currentUserID > 0 && c.UserID == currentUserID
		out = append(out, c)
	}
	return out, rows.Err()
}

func DeletePluginMarketComment(id, userID, fileUploaderID int64) error {
	var owner int64
	if err := DB.QueryRow(`SELECT user_id FROM plugin_market_comments WHERE id = ?`, id).Scan(&owner); err != nil {
		return fmt.Errorf("评论不存在~")
	}
	if userID != owner && userID != fileUploaderID {
		return fmt.Errorf("无权删除该评论~")
	}
	_, err := DB.Exec(`WITH RECURSIVE sub(id) AS (
		SELECT id FROM plugin_market_comments WHERE id = ?
		UNION ALL
		SELECT c.id FROM plugin_market_comments c JOIN sub s ON c.parent_id = s.id
	) DELETE FROM plugin_market_comments WHERE id IN (SELECT id FROM sub)`, id)
	return err
}

// ────────────────────────────────────────────────────────────
// 我上传的
// ────────────────────────────────────────────────────────────

func ListPluginMarketFilesByUploader(uploaderID int64, page, size int) ([]PluginMarketFile, int, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 20
	}
	var total int
	DB.QueryRow(`SELECT COUNT(*) FROM plugin_market_files WHERE uploader_id = ?`, uploaderID).Scan(&total)
	rows, err := DB.Query(`SELECT `+pluginMarketFileSel+`,COALESCE(u.username,'') FROM plugin_market_files f LEFT JOIN users u ON u.id=f.uploader_id WHERE f.uploader_id = ? ORDER BY f.id DESC LIMIT ? OFFSET ?`,
		uploaderID, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []PluginMarketFile{}
	for rows.Next() {
		f, err := scanPluginMarketFile(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *f)
	}
	for i := range list {
		list[i].Categories = pluginMarketFileCategories(list[i].ID)
	}
	return list, total, rows.Err()
}

// ────────────────────────────────────────────────────────────
// 上传者编辑/删除
// ────────────────────────────────────────────────────────────

type PluginMarketOwnerUpdate struct {
	Title       string   `json:"title"`
	PricePts    int      `json:"price_pts"`
	AllowAnon   bool     `json:"allow_anonymous"`
	Tags        []string `json:"tags"`
	Description string   `json:"description"`
	Docs        string   `json:"docs"`
	Categories  []string `json:"categories"`
	CardColor   string   `json:"card_color"`
	Status      *int     `json:"status"`
	CoverPartID *int64   `json:"cover_part_id"`
}

func UpdatePluginMarketFileOwner(id, uploaderID int64, u *PluginMarketOwnerUpdate) error {
	var owner int64
	if err := DB.QueryRow(`SELECT uploader_id FROM plugin_market_files WHERE id = ?`, id).Scan(&owner); err != nil || owner != uploaderID {
		return fmt.Errorf("无权编辑该插件~")
	}
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	tagsJSON, _ := json.Marshal(u.Tags)
	coverSet, cov := 0, int64(0)
	if u.CoverPartID != nil {
		coverSet, cov = 1, *u.CoverPartID
	}
	statusSet, statusVal := 0, 0
	if u.Status != nil {
		statusSet, statusVal = 1, *u.Status
	}
	anon := 0
	if u.AllowAnon {
		anon = 1
	}
	if _, err := tx.Exec(`UPDATE plugin_market_files SET name = ?, price_pts = ?, allow_anonymous = ?, tags = ?, description = ?, docs = ?, card_color = ?,
		cover_part_id = CASE WHEN ? = 1 THEN ? ELSE cover_part_id END,
		review_state = CASE WHEN ? = 1 THEN ? ELSE review_state END, updated_at = ? WHERE id = ?`,
		u.Title, u.PricePts, anon, string(tagsJSON), u.Description, u.Docs, u.CardColor,
		coverSet, cov, statusSet, statusVal, now, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM plugin_market_file_categories WHERE file_id = ?`, id); err != nil {
		return err
	}
	if err := insertPluginFileCategoriesTx(tx, id, u.Categories); err != nil {
		return err
	}
	return tx.Commit()
}

func DeletePluginMarketFile(id int64) (string, error) {
	var dir string
	if err := DB.QueryRow(`SELECT part_dir FROM plugin_market_files WHERE id = ?`, id).Scan(&dir); err != nil {
		return "", err
	}
	if _, err := DB.Exec(`DELETE FROM plugin_market_files WHERE id = ?`, id); err != nil {
		return "", err
	}
	return dir, nil
}

// ────────────────────────────────────────────────────────────
// 管理
// ────────────────────────────────────────────────────────────

func ListAdminPluginMarketFiles(page, size int, q string, reviewState *int) ([]PluginMarketFile, int, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	where := []string{}
	args := []any{}
	if q != "" {
		where = append(where, "f.name LIKE ?")
		args = append(args, "%"+escapeLike(q)+"%")
	}
	if reviewState != nil {
		where = append(where, "f.review_state = ?")
		args = append(args, *reviewState)
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM plugin_market_files f`+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, size, (page-1)*size)
	rows, err := DB.Query(`SELECT `+pluginMarketFileSel+`,COALESCE(u.username,'') FROM plugin_market_files f LEFT JOIN users u ON u.id=f.uploader_id`+cond+` ORDER BY f.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []PluginMarketFile{}
	for rows.Next() {
		f, err := scanPluginMarketFile(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *f)
	}
	for i := range list {
		list[i].Categories = pluginMarketFileCategories(list[i].ID)
	}
	return list, total, rows.Err()
}

func SetPluginMarketReviewState(id int64, state int) error {
	_, err := DB.Exec(`UPDATE plugin_market_files SET review_state = ?, updated_at = ? WHERE id = ?`, state, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func ClearPluginMarketFlag(id int64) error {
	_, err := DB.Exec(`UPDATE plugin_market_files SET dedup_flag = 0, flag_reason = '' WHERE id = ?`, id)
	return err
}

func UpdatePluginMarketFileDetail(id int64, price int, allowAnon bool, tags, description, docs []string, categories []string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	tagsJSON, _ := json.Marshal(tags)
	desc, doc := "", ""
	if len(description) > 0 {
		desc = description[0]
	}
	if len(docs) > 0 {
		doc = docs[0]
	}
	anon := 0
	if allowAnon {
		anon = 1
	}
	if _, err := tx.Exec(`UPDATE plugin_market_files SET price_pts = ?, allow_anonymous = ?, tags = ?, description = ?, docs = ?, updated_at = ? WHERE id = ?`,
		price, anon, string(tagsJSON), desc, doc, now, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM plugin_market_file_categories WHERE file_id = ?`, id); err != nil {
		return err
	}
	if err := insertPluginFileCategoriesTx(tx, id, categories); err != nil {
		return err
	}
	return tx.Commit()
}
