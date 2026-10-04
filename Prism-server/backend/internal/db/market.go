package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ────────────────────────────────────────────────────────────
// 数据模型
// ────────────────────────────────────────────────────────────

// MarketUploadSession 两阶段上传的临时会话令牌(TTS)记录。
type MarketUploadSession struct {
	ID         int64          `json:"id"`
	TTSHash    string         `json:"-"`
	UserID     int64          `json:"user_id"`
	FileMD5    string         `json:"file_md5"`
	FileSize   int64          `json:"file_size"`
	PreviewsMD string         `json:"previews_md5"`
	PricePts   int            `json:"price_pts"`
	BlockCount int            `json:"block_count"`
	NbtCount   int            `json:"nbt_count"`
	BlockDims  map[string]int `json:"block_dims"`
	ExpiresAt  string         `json:"expires_at"`
	Used       bool           `json:"used"`
	CreatedAt  string         `json:"created_at"`
}

// MarketFile 建筑商品主表。
type MarketFile struct {
	ID            int64          `json:"id"`
	UploaderID    int64          `json:"uploader_id"`
	UploaderName  string         `json:"uploader_name,omitempty"`
	Title         string         `json:"title"`
	Description   string         `json:"description,omitempty"`
	Categories    []string       `json:"categories"`
	Tags          []string       `json:"tags"`
	PricePts      int            `json:"price_pts"`
	AllowAnon     bool           `json:"allow_anonymous"`
	MD5           string         `json:"-"`
	BlockCount    int            `json:"block_count"`
	NbtCount      int            `json:"nbt_count"`
	FileSize      int64          `json:"file_size"`
	BlockDims     map[string]int `json:"block_dims,omitempty"`
	PreviewCam    map[string]int `json:"preview_cam,omitempty"`
	DownloadCount int            `json:"download_count"`
	PartDir       string         `json:"-"`
	Flagged       bool           `json:"flagged"`
	ReportCount   int            `json:"report_count"`
	Status        int            `json:"status"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
	CoverPartID   int64          `json:"cover_part_id"`
}

// MarketCategory 分类。
type MarketCategory struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

// MarketPart 解包出的实体文件。
type MarketPart struct {
	ID         int64  `json:"id"`
	FileID     int64  `json:"file_id"`
	PartType   string `json:"part_type"` // structure | preview_img | preview_gif
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

// MarketReport 举报记录。
type MarketReport struct {
	ID         int64  `json:"id"`
	FileID     int64  `json:"file_id"`
	ReporterID int64  `json:"reporter_id"`
	Reason     string `json:"reason"`
	CreatedAt  string `json:"created_at"`
}

// MarketConfig 后台可配置项（单行 id=1）。
type MarketConfig struct {
	UploadRewardNuts   int `json:"upload_reward_nuts"`
	ReportAutoTakedown int `json:"report_auto_takedown"`
}

// ────────────────────────────────────────────────────────────
// 会话（临时令牌）
// ────────────────────────────────────────────────────────────

func CreateMarketUploadSession(ttsHash string, userID int64, fileMD5 string, fileSize int64, previewsMD string, price int, blockCount, nbtCount int, blockDims map[string]int) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	exp := time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339)
	dims, _ := json.Marshal(blockDims)
	res, err := DB.Exec(`INSERT INTO market_upload_sessions (tts_hash, user_id, file_md5, file_size, previews_md5, price_pts, block_count, nbt_count, block_dims, expires_at, used, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,0,?)`,
		ttsHash, userID, fileMD5, fileSize, previewsMD, price, blockCount, nbtCount, string(dims), exp, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetMarketUploadSession 按 TTS 哈希查会话（未用且未过期）。
func GetMarketUploadSession(ttsHash string) (*MarketUploadSession, error) {
	row := DB.QueryRow(`SELECT id, user_id, file_md5, file_size, previews_md5, price_pts, block_count, nbt_count, block_dims, expires_at, used, created_at FROM market_upload_sessions WHERE tts_hash = ?`, ttsHash)
	var s MarketUploadSession
	var used int
	var dims string
	if err := row.Scan(&s.ID, &s.UserID, &s.FileMD5, &s.FileSize, &s.PreviewsMD, &s.PricePts, &s.BlockCount, &s.NbtCount, &dims, &s.ExpiresAt, &used, &s.CreatedAt); err != nil {
		return nil, err
	}
	s.BlockDims = map[string]int{}
	json.Unmarshal([]byte(dims), &s.BlockDims)
	s.TTSHash = ttsHash
	s.Used = used != 0
	return &s, nil
}

// MarkMarketUploadSessionUsed 标记会话已用（防重放）。
func MarkMarketUploadSessionUsed(id int64) error {
	_, err := DB.Exec(`UPDATE market_upload_sessions SET used = 1 WHERE id = ?`, id)
	return err
}

// DeleteMarketUploadSession 删除会话记录。
func DeleteMarketUploadSession(id int64) error {
	_, err := DB.Exec(`DELETE FROM market_upload_sessions WHERE id = ?`, id)
	return err
}

// PruneMarketUploadSessions 清理过期/已用会话。
func PruneMarketUploadSessions() error {
	_, err := DB.Exec(`DELETE FROM market_upload_sessions WHERE used = 1 OR expires_at < ?`, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ────────────────────────────────────────────────────────────
// 配置
// ────────────────────────────────────────────────────────────

func GetMarketConfig() (*MarketConfig, error) {
	var c MarketConfig
	err := DB.QueryRow(`SELECT upload_reward_nuts, report_auto_takedown FROM market_config WHERE id = 1`).Scan(&c.UploadRewardNuts, &c.ReportAutoTakedown)
	if err != nil {
		return &MarketConfig{UploadRewardNuts: 10, ReportAutoTakedown: 3}, nil
	}
	return &c, nil
}

func UpdateMarketConfig(c *MarketConfig) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := DB.Exec(`INSERT INTO market_config (id, upload_reward_nuts, report_auto_takedown, updated_at) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET upload_reward_nuts=excluded.upload_reward_nuts, report_auto_takedown=excluded.report_auto_takedown, updated_at=excluded.updated_at`,
		c.UploadRewardNuts, c.ReportAutoTakedown, now)
	return err
}

// ────────────────────────────────────────────────────────────
// 文件（商品）
// ────────────────────────────────────────────────────────────

// CreateMarketFile 创建商品 + 关联 parts（单事务）。返回 file id。
func CreateMarketFile(f *MarketFile, parts []MarketPart) (int64, error) {
	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	tags, _ := json.Marshal(f.Tags)
	dims, _ := json.Marshal(f.BlockDims)
	cam, _ := json.Marshal(f.PreviewCam)
	anon := 0
	if f.AllowAnon {
		anon = 1
	}
	res, err := tx.Exec(`INSERT INTO market_files (uploader_id, title, description, category, tags, price_pts, allow_anonymous, md5, block_count, nbt_count, file_size, block_dims, preview_cam, download_count, cover_part_id, part_dir, flagged, report_count, status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,0,?,?,0,0,1,?,?)`,
		f.UploaderID, f.Title, f.Description, "", string(tags), f.PricePts, anon, f.MD5, f.BlockCount, f.NbtCount, f.FileSize, string(dims), string(cam), f.CoverPartID, f.PartDir, now, now)
	if err != nil {
		return 0, err
	}
	fileID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	// 分类关联(多对多)
	if err := insertFileCategoriesTx(tx, fileID, f.Categories); err != nil {
		return 0, err
	}
	for _, p := range parts {
		if _, err := tx.Exec(`INSERT INTO market_file_parts (file_id, part_type, orig_name, stored_name, ext, rel_path, md5, width, height, size, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			fileID, p.PartType, p.OrigName, p.StoredName, p.Ext, p.RelPath, p.MD5, p.Width, p.Height, p.Size, now); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return fileID, nil
}

// insertFileCategoriesTx 把分类名解析为 ID 并写入关联表(忽略不存在的分类名)。
func insertFileCategoriesTx(tx *sql.Tx, fileID int64, names []string) error {
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		var cid int64
		err := tx.QueryRow(`SELECT id FROM market_categories WHERE name = ?`, n).Scan(&cid)
		if err != nil {
			continue // 分类不存在则忽略
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO market_file_categories (file_id, category_id) VALUES (?,?)`, fileID, cid); err != nil {
			return err
		}
	}
	return nil
}

// GetMarketFileByMD5 查重。
func GetMarketFileByMD5(md5 string) (*MarketFile, error) {
	return scanMarketFile(DB.QueryRow(`SELECT ` + marketFileSel + `,'' FROM market_files f WHERE f.md5 = ?`, md5))
}

// GetMarketFile 按 id 取（含 uploader 名、分类、封面）。
func GetMarketFile(id int64) (*MarketFile, error) {
	f, err := scanMarketFile(DB.QueryRow(`
		SELECT ` + marketFileSel + `,COALESCE(u.username, '') FROM market_files f
		LEFT JOIN users u ON u.id = f.uploader_id WHERE f.id = ?`, id))
	if err != nil {
		return nil, err
	}
	cats, _ := GetMarketFileCategories(id)
	f.Categories = cats
	if f.CoverPartID == 0 {
		if pid, err := marketCoverPartID(id); err == nil {
			f.CoverPartID = pid
		}
	}
	return f, nil
}

// ListMarketFiles 分页 + 分类/标签/关键词搜索。order: new|hot。
// 分类经 junction 表(market_file_categories)过滤;"其他"聚合无分类文件与明确归"其他"的文件。
func ListMarketFiles(page, size int, category, tag, order, q string) ([]MarketFile, int, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 20
	}
	where := []string{"f.status = 1"}
	args := []any{}
	if category != "" {
		// "其他" = 无任何分类 或 明确标为"其他"
		if category == "其他" {
			where = append(where, `(EXISTS(SELECT 1 FROM market_file_categories fc JOIN market_categories c ON c.id=fc.category_id WHERE fc.file_id=f.id AND c.name='其他')
				OR NOT EXISTS(SELECT 1 FROM market_file_categories fc WHERE fc.file_id=f.id))`)
		} else {
			where = append(where, `EXISTS(SELECT 1 FROM market_file_categories fc JOIN market_categories c ON c.id=fc.category_id WHERE fc.file_id=f.id AND c.name=?)`)
			args = append(args, category)
		}
	}
	if tag != "" {
		where = append(where, "f.tags LIKE ?")
		args = append(args, "%\""+escapeLike(tag)+"\"%")
	}
	if q != "" {
		lq := "%" + escapeLike(q) + "%"
		where = append(where, "(f.title LIKE ? OR f.description LIKE ? OR f.tags LIKE ?)")
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
	if err := DB.QueryRow(`SELECT COUNT(*) FROM market_files f `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	off := (page - 1) * size
	args = append(args, size, off)
	rows, err := DB.Query(`SELECT ` + marketFileSel + `,COALESCE(u.username,'') FROM market_files f LEFT JOIN users u ON u.id=f.uploader_id `+cond+` ORDER BY `+orderBy+` LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []MarketFile{}
	for rows.Next() {
		f, err := scanMarketFile(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *f)
	}
	if err := enrichMarketFiles(list); err != nil {
		return nil, 0, err
	}
	return list, total, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// GetAllMarketTags 聚合所有标签去重（按出现次数降序）。
func GetAllMarketTags() ([]string, error) {
	rows, err := DB.Query(`SELECT tags FROM market_files WHERE status = 1 AND tags != '[]' AND tags != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	count := map[string]int{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var tags []string
		if json.Unmarshal([]byte(raw), &tags) == nil {
			for _, t := range tags {
				count[t]++
			}
		}
	}
	tags := make([]string, 0, len(count))
	for t := range count {
		tags = append(tags, t)
	}
	// 简单稳定排序：按出现次数降序
	for i := 0; i < len(tags); i++ {
		for j := i + 1; j < len(tags); j++ {
			if count[tags[j]] > count[tags[i]] {
				tags[i], tags[j] = tags[j], tags[i]
			}
		}
	}
	return tags, nil
}

// marketFileSel 显式列出 market_files 列(固定顺序),与 scanMarketFile 严格对应。
// 不能用 f.*: 对已存在的旧库,ALTER ADD COLUMN 会把新列追加到表尾,导致 f.* 顺序
// 与 CREATE 定义的顺序不一致,scan 时列错位(曾把 block_dims 读到 int 列报错)。
const marketFileSel = "f.id, f.uploader_id, f.title, f.description, f.category, f.tags, f.price_pts, f.allow_anonymous, f.md5, f.block_count, f.nbt_count, f.file_size, f.block_dims, f.preview_cam, f.download_count, f.cover_part_id, f.part_dir, f.flagged, f.report_count, f.status, f.created_at, f.updated_at"

func scanMarketFile(row interface{ Scan(...any) error }) (*MarketFile, error) {
	var f MarketFile
	var tags, dims, cam, legacyCat string
	var anon, flagged int
	err := row.Scan(
		&f.ID, &f.UploaderID, &f.Title, &f.Description, &legacyCat, &tags, &f.PricePts,
		&anon, &f.MD5, &f.BlockCount, &f.NbtCount, &f.FileSize, &dims, &cam, &f.DownloadCount,
		&f.CoverPartID, &f.PartDir, &flagged, &f.ReportCount, &f.Status, &f.CreatedAt, &f.UpdatedAt,
		&f.UploaderName,
	)
	if err != nil {
		return nil, err
	}
	f.AllowAnon = anon != 0
	f.Flagged = flagged != 0
	json.Unmarshal([]byte(tags), &f.Tags)
	json.Unmarshal([]byte(dims), &f.BlockDims)
	json.Unmarshal([]byte(cam), &f.PreviewCam)
	// 兼容:历史单分类数据导入到 categories
	if f.Categories == nil && legacyCat != "" {
		f.Categories = []string{legacyCat}
	}
	return &f, nil
}

// GetMarketFileCategories 取文件关联的分类名列表。
func GetMarketFileCategories(fileID int64) ([]string, error) {
	rows, err := DB.Query(`SELECT c.name FROM market_file_categories fc JOIN market_categories c ON c.id = fc.category_id WHERE fc.file_id = ? ORDER BY c.sort_order`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		list = append(list, n)
	}
	return list, rows.Err()
}

// enrichMarketFiles 为文件列表批量填充分类与封面预览 part id。
func enrichMarketFiles(files []MarketFile) error {
	for i := range files {
		cats, err := GetMarketFileCategories(files[i].ID)
		if err != nil {
			return err
		}
		files[i].Categories = cats
		// 封面:第一张 preview_img part
		if files[i].CoverPartID == 0 {
			if pid, err := marketCoverPartID(files[i].ID); err == nil {
				files[i].CoverPartID = pid
			}
		}
	}
	return nil
}

// marketCoverPartID 取建筑第一张 WebP 预览 part id。
func marketCoverPartID(fileID int64) (int64, error) {
	var pid int64
	err := DB.QueryRow(`SELECT id FROM market_file_parts WHERE file_id = ? AND part_type = 'preview_img' ORDER BY id LIMIT 1`, fileID).Scan(&pid)
	if err != nil {
		return 0, err
	}
	return pid, nil
}

// ────────────────────────────────────────────────────────────
// Parts
// ────────────────────────────────────────────────────────────

func GetMarketFileParts(fileID int64) ([]MarketPart, error) {
	rows, err := DB.Query(`SELECT id, file_id, part_type, orig_name, stored_name, ext, rel_path, md5, width, height, size, created_at FROM market_file_parts WHERE file_id = ?`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []MarketPart{}
	for rows.Next() {
		var p MarketPart
		if err := rows.Scan(&p.ID, &p.FileID, &p.PartType, &p.OrigName, &p.StoredName, &p.Ext, &p.RelPath, &p.MD5, &p.Width, &p.Height, &p.Size, &p.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func GetMarketFilePart(id int64) (*MarketPart, error) {
	var p MarketPart
	err := DB.QueryRow(`SELECT id, file_id, part_type, orig_name, stored_name, ext, rel_path, md5, width, height, size, created_at FROM market_file_parts WHERE id = ?`, id).
		Scan(&p.ID, &p.FileID, &p.PartType, &p.OrigName, &p.StoredName, &p.Ext, &p.RelPath, &p.MD5, &p.Width, &p.Height, &p.Size, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ────────────────────────────────────────────────────────────
// 下载（权限 + 付费）
// ────────────────────────────────────────────────────────────

// PurchaseOrReuse 付费下载事务：幂等防重复扣费。
// 已购(UNIQUE(file_id,user_id)命中)→直接返回;否则扣买家、卖家入账、计数、写流水。
// sellerID==buyerID 时跳过双方记账,仅计数。返回是否本次新增付费。
func PurchaseOrReuse(fileID, buyerID int64, price int, sellerID int64) (bool, error) {
	tx, err := DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)

	// 幂等：命中唯一约束 → 已购，不再扣费
	res, err := tx.Exec(`INSERT OR IGNORE INTO market_downloads (file_id, user_id, price, created_at) VALUES (?,?,?,?)`, fileID, buyerID, price, now)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, tx.Commit()
	}

	if price > 0 {
		// 订阅买家：免费下载，买卖双方无板栗变动（仍写 market_downloads 记录）
		if subActive, _, _ := IsSubscriptionActive(buyerID); subActive {
			// 跳过扣费块，但继续执行下方 download_count 递增
		} else {
			// 扣买家
			var bal int
			err = tx.QueryRow(`UPDATE users SET nuts_balance = nuts_balance - ? WHERE id = ? RETURNING nuts_balance`, price, buyerID).Scan(&bal)
			if err != nil || bal < 0 {
				return false, fmt.Errorf("板栗不足,需要 %d", price)
			}
			if _, err := tx.Exec(`INSERT INTO nuts_transactions (user_id, amount, reason, ref_user_id, balance_after, created_at) VALUES (?,?,?,?,?,?)`,
				buyerID, -price, "下载建筑", &sellerID, bal, now); err != nil {
				return false, err
			}
			// 卖家入账（买自己跳过）
			if sellerID != buyerID {
				var sbal int
				if err := tx.QueryRow(`UPDATE users SET nuts_balance = nuts_balance + ? WHERE id = ? RETURNING nuts_balance`, price, sellerID).Scan(&sbal); err != nil {
					return false, err
				}
				if _, err := tx.Exec(`INSERT INTO nuts_transactions (user_id, amount, reason, ref_user_id, balance_after, created_at) VALUES (?,?,?,?,?,?)`,
					sellerID, price, "售出建筑", &buyerID, sbal, now); err != nil {
					return false, err
				}
			}
		}
	}
	if _, err := tx.Exec(`UPDATE market_files SET download_count = download_count + 1 WHERE id = ?`, fileID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// RecordFreeDownload 免费下载（匿名/免费）：仅计数，不写 market_downloads。
func RecordFreeDownload(fileID int64) error {
	_, err := DB.Exec(`UPDATE market_files SET download_count = download_count + 1 WHERE id = ?`, fileID)
	return err
}

// UserHasPurchased 查询用户是否已购。
func UserHasPurchased(fileID, userID int64) (bool, error) {
	var n int
	err := DB.QueryRow(`SELECT COUNT(*) FROM market_downloads WHERE file_id = ? AND user_id = ?`, fileID, userID).Scan(&n)
	return n > 0, err
}

// CreditUploadReward 上传成功后发放奖励板栗（按后台配置）。
func CreditUploadReward(userID int64) error {
	c, err := GetMarketConfig()
	if err != nil {
		return err
	}
	if c.UploadRewardNuts <= 0 {
		return nil
	}
	_, err = AddNuts(userID, c.UploadRewardNuts, "上传建筑", nil, nil)
	return err
}

// ────────────────────────────────────────────────────────────
// 举报
// ────────────────────────────────────────────────────────────

// AddMarketReport 记录举报并更新文件标记。返回(autoTakedown bool, err)。
// 一人对一文件只举报一次(UNIQUE)。举报后 file.flagged=1(对所有用户可见);
// report_count > 阈值时自动下架(status=0),但保留文件供管理员决定。
func AddMarketReport(fileID, reporterID int64, reason string) (autoTakedown bool, err error) {
	tx, err := DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := tx.Exec(`INSERT OR IGNORE INTO market_reports (file_id, reporter_id, reason, created_at) VALUES (?,?,?,?)`, fileID, reporterID, reason, now)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, fmt.Errorf("你已经举报过这个文件了~")
	}

	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM market_reports WHERE file_id = ?`, fileID).Scan(&count); err != nil {
		return false, err
	}
	c, _ := GetMarketConfig()
	threshold := c.ReportAutoTakedown
	if threshold < 1 {
		threshold = 3
	}
	autoTakedown = count > threshold
	status := 1
	if autoTakedown {
		status = 0
	}
	if _, err := tx.Exec(`UPDATE market_files SET flagged = 1, report_count = ?, status = ? WHERE id = ?`, count, status, fileID); err != nil {
		return false, err
	}
	return autoTakedown, tx.Commit()
}

// MarketReportedFiles 管理：受标记文件列表。
func MarketReportedFiles() ([]MarketFile, int, error) {
	rows, err := DB.Query(`SELECT ` + marketFileSel + `,COALESCE(u.username,'') FROM market_files f LEFT JOIN users u ON u.id=f.uploader_id WHERE f.flagged = 1 ORDER BY f.report_count DESC`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []MarketFile{}
	for rows.Next() {
		f, err := scanMarketFile(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *f)
	}
	return list, len(list), rows.Err()
}

// ────────────────────────────────────────────────────────────
// 分类管理
// ────────────────────────────────────────────────────────────

func ListMarketCategories() ([]MarketCategory, error) {
	rows, err := DB.Query(`SELECT id, name, sort_order FROM market_categories ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []MarketCategory{}
	for rows.Next() {
		var c MarketCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.SortOrder); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func CreateMarketCategory(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("分类名不能为空")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := DB.Exec(`INSERT INTO market_categories (name, sort_order, created_at) VALUES (?, 0, ?)`, name, now)
	return err
}

func RenameMarketCategory(id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("分类名不能为空")
	}
	res, err := DB.Exec(`UPDATE market_categories SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("分类不存在")
	}
	return nil
}

// DeleteMarketCategory 删除分类(级联删关联)。被删分类下仅剩此分类的文件归"未分类"(分类为空)。
func DeleteMarketCategory(id int64) error {
	res, err := DB.Exec(`DELETE FROM market_categories WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("分类不存在")
	}
	return nil
}

// ────────────────────────────────────────────────────────────
// 管理
// ────────────────────────────────────────────────────────────

// UpdateMarketFileDetail 后台编辑文件详情：价格/匿名/标签/说明/分类。分类用分类名数组。
func UpdateMarketFileDetail(id int64, price int, allowAnon bool, tags, description []string, categories []string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	anon := 0
	if allowAnon {
		anon = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tagsJSON, _ := json.Marshal(tags)
	desc := ""
	if len(description) > 0 {
		desc = description[0]
	}
	if _, err := tx.Exec(`UPDATE market_files SET price_pts = ?, allow_anonymous = ?, tags = ?, description = ?, updated_at = ? WHERE id = ?`,
		price, anon, string(tagsJSON), desc, now, id); err != nil {
		return err
	}
	// 重建分类关联
	if _, err := tx.Exec(`DELETE FROM market_file_categories WHERE file_id = ?`, id); err != nil {
		return err
	}
	if err := insertFileCategoriesTx(tx, id, categories); err != nil {
		return err
	}
	return tx.Commit()
}

func ListAdminMarketFiles(page, size int, q string) ([]MarketFile, int, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	cond := ""
	args := []any{}
	if q != "" {
		cond = " WHERE f.title LIKE ?"
		args = append(args, "%"+escapeLike(q)+"%")
	}
	var total int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM market_files f`+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, size, (page-1)*size)
	rows, err := DB.Query(`SELECT ` + marketFileSel + `,COALESCE(u.username,'') FROM market_files f LEFT JOIN users u ON u.id=f.uploader_id`+cond+` ORDER BY f.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []MarketFile{}
	for rows.Next() {
		f, err := scanMarketFile(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *f)
	}
	if err := enrichMarketFiles(list); err != nil {
		return nil, 0, err
	}
	return list, total, rows.Err()
}

// SetMarketFileStatus 上架/下架。
func SetMarketFileStatus(id int64, status int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := DB.Exec(`UPDATE market_files SET status = ?, updated_at = ? WHERE id = ?`, status, now, id)
	return err
}

// ClearMarketFileFlag 管理员处理后清除举报标记。
func ClearMarketFileFlag(id int64) error {
	_, err := DB.Exec(`UPDATE market_files SET flagged = 0, report_count = 0 WHERE id = ?`, id)
	return err
}

// ────────────────────────────────────────────────────────────
// 评论
// ────────────────────────────────────────────────────────────

// MarketComment 文件评论(多级回复)。IsAuthor=是否文件作者;Mine=是否为当前登录用户。
type MarketComment struct {
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

// CreateMarketComment 新增评论。parent_id=0 为根评论;>0 校验父评论属于同一文件。
func CreateMarketComment(fileID, parentID, userID int64, content string) (int64, error) {
	if parentID > 0 {
		var fid int64
		if err := DB.QueryRow(`SELECT file_id FROM market_comments WHERE id = ?`, parentID).Scan(&fid); err != nil || fid != fileID {
			return 0, fmt.Errorf("回复的评论不存在~")
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := DB.Exec(`INSERT INTO market_comments (file_id, parent_id, user_id, content, created_at) VALUES (?,?,?,?,?)`,
		fileID, parentID, userID, content, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListMarketComments 列出某文件全部评论(含作者标记/是否本人)。返回按时间升序的扁平列表,由调用方组树。
func ListMarketComments(fileID, currentUserID int64) ([]MarketComment, error) {
	var uploaderID int64
	DB.QueryRow(`SELECT uploader_id FROM market_files WHERE id = ?`, fileID).Scan(&uploaderID)
	rows, err := DB.Query(`SELECT c.id, c.file_id, c.parent_id, c.user_id, COALESCE(u.username,''), c.content, c.created_at
		FROM market_comments c LEFT JOIN users u ON u.id = c.user_id WHERE c.file_id = ? ORDER BY c.id`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []MarketComment{}
	for rows.Next() {
		var c MarketComment
		if err := rows.Scan(&c.ID, &c.FileID, &c.ParentID, &c.UserID, &c.Username, &c.Content, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.IsAuthor = c.UserID == uploaderID
		c.Mine = currentUserID > 0 && c.UserID == currentUserID
		list = append(list, c)
	}
	return list, rows.Err()
}

// DeleteMarketComment 删除评论及其全部子孙(级联)。允许:评论作者本人 或 文件作者。
// 删除父评论时,其下所有子评论/回复一并删除,防止"子评论失去父级变成根评论"。
func DeleteMarketComment(id, userID, fileUploaderID int64) error {
	var owner int64
	if err := DB.QueryRow(`SELECT user_id FROM market_comments WHERE id = ?`, id).Scan(&owner); err != nil {
		return fmt.Errorf("评论不存在~")
	}
	if userID != owner && userID != fileUploaderID {
		return fmt.Errorf("无权删除该评论~")
	}
	_, err := DB.Exec(`WITH RECURSIVE sub(id) AS (
		SELECT id FROM market_comments WHERE id = ?
		UNION ALL
		SELECT c.id FROM market_comments c JOIN sub s ON c.parent_id = s.id
	)
	DELETE FROM market_comments WHERE id IN (SELECT id FROM sub)`, id)
	return err
}

// ────────────────────────────────────────────────────────────
// 上传者编辑自己上传的文件
// ────────────────────────────────────────────────────────────

// MarketOwnerUpdate 上传者可编辑的字段。Status/CoverPartID 用指针区分"不改"与"改为0"。
type MarketOwnerUpdate struct {
	Title         string   `json:"title"`
	PricePts      int      `json:"price_pts"`
	AllowAnon     bool     `json:"allow_anonymous"`
	Tags          []string `json:"tags"`
	Description   string   `json:"description"`
	Categories    []string `json:"categories"`
	Status        *int     `json:"status"`
	CoverPartID   *int64   `json:"cover_part_id"`
}

// UpdateMarketFileOwner 校验归属后更新(价格/匿名/标签/说明/分类/上下架/封面)。分类重建关联。
func UpdateMarketFileOwner(id, uploaderID int64, u *MarketOwnerUpdate) error {
	var owner int64
	if err := DB.QueryRow(`SELECT uploader_id FROM market_files WHERE id = ?`, id).Scan(&owner); err != nil || owner != uploaderID {
		return fmt.Errorf("无权编辑该文件~")
	}
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)

	// 封面必须属于本文件且为 preview_img
	if u.CoverPartID != nil && *u.CoverPartID > 0 {
		var pt, fid int64
		err := tx.QueryRow(`SELECT id, file_id FROM market_file_parts WHERE id = ? AND part_type = 'preview_img'`, *u.CoverPartID).Scan(&pt, &fid)
		if err != nil || fid != id {
			return fmt.Errorf("封面预览图不合法~")
		}
	}

	tagsJSON, _ := json.Marshal(u.Tags)
	// 封面: 仅当显式传入 cover_part_id 时才更新,否则保持原封面(防止"改别的字段把封面清掉")
	coverSet := 0
	cov := int64(0)
	if u.CoverPartID != nil {
		coverSet = 1
		cov = *u.CoverPartID
	}
	statusSet := 0
	statusVal := 0
	if u.Status != nil {
		statusSet = 1
		statusVal = *u.Status
	}
	if _, err := tx.Exec(`UPDATE market_files SET title = ?, price_pts = ?, allow_anonymous = ?, tags = ?, description = ?,
		cover_part_id = CASE WHEN ? = 1 THEN ? ELSE cover_part_id END,
		status = CASE WHEN ? = 1 THEN ? ELSE status END, updated_at = ? WHERE id = ?`,
		u.Title, u.PricePts, b2i(u.AllowAnon), string(tagsJSON), u.Description, coverSet, cov, statusSet, statusVal, now, id); err != nil {
		return err
	}
	// 重建分类关联
	if _, err := tx.Exec(`DELETE FROM market_file_categories WHERE file_id = ?`, id); err != nil {
		return err
	}
	if err := insertFileCategoriesTx(tx, id, u.Categories); err != nil {
		return err
	}
	return tx.Commit()
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ListMarketFilesByUploader 某用户上传的文件(用于"我的文件"编辑管理)。含下架。
func ListMarketFilesByUploader(uploaderID int64, page, size int) ([]MarketFile, int, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 20
	}
	var total int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM market_files WHERE uploader_id = ?`, uploaderID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := DB.Query(`SELECT ` + marketFileSel + `,COALESCE(u.username,'') FROM market_files f LEFT JOIN users u ON u.id=f.uploader_id WHERE f.uploader_id = ? ORDER BY f.id DESC LIMIT ? OFFSET ?`,
		uploaderID, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []MarketFile{}
	for rows.Next() {
		f, err := scanMarketFile(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *f)
	}
	if err := enrichMarketFiles(list); err != nil {
		return nil, 0, err
	}
	return list, total, rows.Err()
}

// DeleteMarketFile 删除商品(DB 级联 parts/downloads/reports)。返回 part_dir 供调用方清磁盘。
func DeleteMarketFile(id int64) (string, error) {
	var dir string
	err := DB.QueryRow(`SELECT part_dir FROM market_files WHERE id = ?`, id).Scan(&dir)
	if err != nil {
		return "", err
	}
	if _, err := DB.Exec(`DELETE FROM market_files WHERE id = ?`, id); err != nil {
		return "", err
	}
	return dir, nil
}
