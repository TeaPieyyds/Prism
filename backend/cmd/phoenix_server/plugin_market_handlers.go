package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
	"github.com/adb-lanlu/prism-oss/internal/pluginscan"
)

// ────────────────────────────────────────────────────────────
// 插件市场上传（两阶段：预检 → 上传）
// ────────────────────────────────────────────────────────────

const (
	pluginMarketDataRoot = "data/plugin_market"
	maxPluginPricePts    = 100000
)

var pluginUploadLimiter = auth.NewRateLimiter(3, time.Minute)

func pluginUploadLimiterKey(uid int64) string { return "plugin_upload:" + strconv.FormatInt(uid, 10) }

type pluginPrecheckReq struct {
	File struct {
		Size int64  `json:"size"`
		MD5  string `json:"md5"`
	} `json:"file"`
	PricePts int `json:"price_pts"`
}

func handlePluginMarketPrecheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "请先登录~")})
		return
	}
	if !pluginUploadLimiter.Allow(pluginUploadLimiterKey(user.ID)) {
		jsonResp(w, M{"ok": false, "error": "上传太频繁,请稍后再试~"})
		return
	}
	var req pluginPrecheckReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, M{"ok": false, "error": "无效的请求~"})
		return
	}
	if req.File.Size <= 0 || req.File.Size > 20<<20 {
		jsonResp(w, M{"ok": false, "error": "插件包大小需在 1B~20MB 之间~"})
		return
	}
	if req.File.MD5 == "" {
		jsonResp(w, M{"ok": false, "error": "缺少文件 MD5~"})
		return
	}
	if req.PricePts < 0 || req.PricePts > maxPluginPricePts {
		jsonResp(w, M{"ok": false, "error": "价格超出范围~"})
		return
	}
	tts := randHex(12)
	id, err := db.CreatePluginMarketUploadSession(sha256Hex(tts), user.ID, req.File.MD5, req.File.Size, req.PricePts)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	log.Printf("[PMARKET] %s 预检通过,签发 TTS session=%d md5=%s", user.Username, id, req.File.MD5)
	jsonResp(w, M{"ok": true, "tts": tts, "session_id": id, "expires_in": 300})
}

type pluginUploadMeta struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Type         string   `json:"type"`
	Description  string   `json:"description"`
	Docs         string   `json:"docs"`
	Categories   []string `json:"categories"`
	Tags         []string `json:"tags"`
	CapsEnc      string   `json:"caps_enc"`
	PricePts     int      `json:"price_pts"`
	AllowAnon    bool     `json:"allow_anonymous"`
	CardColor    string   `json:"card_color"`
	Changelog    string   `json:"changelog"`
}

func handlePluginMarketUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
		return
	}
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authz, "Bearer ") {
		jsonResp(w, M{"ok": false, "error": "缺少临时令牌~"})
		return
	}
	tts := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
	if tts == "" {
		jsonResp(w, M{"ok": false, "error": "缺少临时令牌~"})
		return
	}
	sess, err := db.GetPluginMarketUploadSession(sha256Hex(tts))
	if err != nil || sess == nil || sess.Used {
		jsonResp(w, M{"ok": false, "error": "临时令牌无效或已使用~"})
		return
	}
	if sess.ExpiresAt < time.Now().UTC().Format(time.RFC3339) {
		jsonResp(w, M{"ok": false, "error": "临时令牌已过期,请重新预检~"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 22<<20)
	if err := r.ParseMultipartForm(22 << 20); err != nil {
		jsonResp(w, M{"ok": false, "error": "上传数据过大~"})
		return
	}
	_ = db.MarkPluginMarketUploadSessionUsed(sess.ID)
	cleanup := func() { db.DeletePluginMarketUploadSession(sess.ID) }

	_, fh, err := r.FormFile("file")
	if err != nil {
		cleanup()
		jsonResp(w, M{"ok": false, "error": "缺少插件包~"})
		return
	}
	fileData, err := readUploadedFile(fh)
	if err != nil {
		cleanup()
		jsonResp(w, M{"ok": false, "error": "读取插件包失败~"})
		return
	}
	if int64(len(fileData)) != sess.FileSize || md5Hex(fileData) != sess.FileMD5 {
		cleanup()
		jsonResp(w, M{"ok": false, "error": "插件包与预检不一致,请重新预检~"})
		return
	}
	meta, err := pluginscan.ParsePluginZip(fileData)
	if err != nil {
		cleanup()
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	// 上传即校验：必须是能运行的 Lua，且不能是 Python 冒充 / Python 转 Lua 失败的假插件。
	// 先反 Python 检测（剥离注释字符串后命中 2 个及以上 Python 特征即拒绝），再做语法校验。
	if py := pluginscan.LooksLikePython(meta.MainLua); py != "" {
		cleanup()
		jsonResp(w, M{"ok": false, "error": "检测到非 Lua 代码（疑似 Python，特征：" + py + "），拒绝上传"})
		return
	}
	if luaErr := pluginscan.CheckLuaSyntax(meta.MainLua); luaErr != nil {
		cleanup()
		jsonResp(w, M{"ok": false, "error": "Lua 语法错误，无法运行：" + luaErr.Error()})
		return
	}
	scanned := pluginscan.ScanCapabilities(meta.MainLua)

	var um pluginUploadMeta
	if raw := r.FormValue("meta"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &um); err != nil {
			cleanup()
			jsonResp(w, M{"ok": false, "error": "描述信息格式错误~"})
			return
		}
	}
	um.Name = meta.Name
	um.Version = meta.Version
	um.Type = meta.Type
	if len(um.Docs) == 0 {
		um.Docs = meta.Docs
	}
	if um.PricePts != sess.PricePts {
		cleanup()
		jsonResp(w, M{"ok": false, "error": "价格与预检不一致,请重新预检~"})
		return
	}
	// 能力声明由客户端扫描并加密上传(caps_enc)，此处解密；再与服务端独立扫描交叉核对
	declared := decryptPluginCaps(um.CapsEnc)
	if len(declared) == 0 && len(scanned) > 0 {
		// 客户端未上报加密能力（旧客户端/扫描为空）时，用服务端独立扫描结果兜底
		declared = scanned
	}
	missing, _ := pluginscan.CrossCheck(declared, scanned)
	risk := pluginscan.RiskLevel(declared, scanned)

	dedupFlag, flagReason, isUpdate, existing := pluginDedup(meta, sess.UserID)
	reviewState := 0
	if isUpdate {
		partDir := existing.PartDir
		if partDir == "" {
			partDir = filepath.Join(pluginMarketDataRoot, randHex(8))
			os.MkdirAll(partDir, 0o700)
		}
		stored := randHex(8) + ".zip"
		if err := os.WriteFile(filepath.Join(partDir, stored), fileData, 0o600); err != nil {
			cleanup()
			jsonResp(w, M{"ok": false, "error": "保存文件失败~"})
			return
		}
		if err := db.AppendPluginMarketVersion(existing.ID, um.Version, sanitizeText(um.Changelog, 2000), stored, meta.ZipHash, meta.NormHash, meta.MD5, meta.FileSize, dedupFlag, flagReason); err != nil {
			cleanup()
			jsonResp(w, M{"ok": false, "error": sysErr(err)})
			return
		}
		jsonResp(w, M{"ok": true, "id": existing.ID, "updated": true, "risk_level": risk, "missing_caps": missing})
		return
	}
	partDir := filepath.Join(pluginMarketDataRoot, randHex(8))
	os.MkdirAll(partDir, 0o700)
	stored := randHex(8) + ".zip"
	if err := os.WriteFile(filepath.Join(partDir, stored), fileData, 0o600); err != nil {
		os.RemoveAll(partDir)
		cleanup()
		jsonResp(w, M{"ok": false, "error": "保存文件失败~"})
		return
	}
	coverPartID := int64(0)
	if phs := r.MultipartForm.File["previews"]; len(phs) > 0 {
		coverPartID = storePluginPreviews(partDir, phs)
	}
	f := &db.PluginMarketFile{
		PluginID: meta.ID, UploaderID: sess.UserID, Name: um.Name, Version: meta.Version, Type: um.Type,
		Description: sanitizeText(um.Description, 4000), Docs: um.Docs,
		Categories: um.Categories, Tags: um.Tags, DeclaredCaps: declared, ScanResult: scanned,
		RiskLevel: risk, PricePts: sess.PricePts, AllowAnon: um.AllowAnon,
		CardColor: sanitizeHexColor(um.CardColor), ZipHash: meta.ZipHash, NormHash: meta.NormHash,
		MD5: meta.MD5, FileSize: meta.FileSize, ReviewState: reviewState,
		DedupFlag: dedupFlag, FlagReason: flagReason, CoverPartID: coverPartID, PartDir: partDir,
	}
	parts := []db.PluginMarketPart{
		{PartType: "plugin_zip", OrigName: filepath.Base(fh.Filename), StoredName: stored, Ext: ".zip", RelPath: stored, MD5: meta.MD5, Size: meta.FileSize},
	}
	version := db.PluginMarketVersion{Version: meta.Version, Changelog: sanitizeText(um.Changelog, 2000), ZipHash: meta.ZipHash, NormHash: meta.NormHash, MD5: meta.MD5, FileSize: meta.FileSize, ReviewState: reviewState, DedupFlag: dedupFlag}
	fileID, err := db.CreatePluginMarketFile(f, parts, version)
	if err != nil {
		os.RemoveAll(partDir)
		cleanup()
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	_ = db.CreditPluginUploadReward(sess.UserID)
	log.Printf("[PMARKET] 上传 file=%d plugin=%s version=%s dedup=%d risk=%d", fileID, meta.ID, meta.Version, dedupFlag, risk)
	jsonResp(w, M{"ok": true, "id": fileID, "updated": false, "risk_level": risk, "missing_caps": missing})
}

// pluginDedup 按 spec §5 判定去重/版本。existing 非 nil 表示命中。
func pluginDedup(meta *pluginscan.PluginMeta, uploaderID int64) (dedupFlag int, flagReason string, isUpdate bool, existing *db.PluginMarketFile) {
	if same, _ := db.FindPluginByPluginID(meta.ID, uploaderID); same != nil {
		return 3, "版本更新 v" + meta.Version, true, same
	}
	if hit, _ := db.FindPluginByNormHash(meta.NormHash); hit != nil {
		if hit.UploaderID != uploaderID {
			return 1, "疑似搬运（内容与已有插件雷同）", false, nil
		}
		return 3, "版本更新 v" + meta.Version, true, hit
	}
	return 0, "", false, nil
}

// pluginCapsKey 插件能力声明解密密钥，由固定短语派生（与客户端 prism-plugin-caps-key-v1-2026 一致）。
var pluginCapsKey = func() []byte {
	h := sha256.Sum256([]byte("prism-plugin-caps-key-v1-2026"))
	return h[:]
}()

// decryptPluginCaps 解密客户端加密上传的能力声明(caps_enc)，返回能力 key 数组。失败返回空。
func decryptPluginCaps(enc string) []string {
	if enc == "" {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil
	}
	block, err := aes.NewCipher(pluginCapsKey)
	if err != nil {
		return nil
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil
	}
	if len(data) < gcm.NonceSize() {
		return nil
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil
	}
	var caps []string
	if json.Unmarshal(plain, &caps) != nil {
		return nil
	}
	return normalizeCaps(caps)
}

func normalizeCaps(caps []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, c := range caps {
		c = strings.TrimSpace(c)
		if c != "" && !seen[c] && pluginscan.CapabilityKeys[c] != "" {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

func sanitizeHexColor(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 7 && s[0] == '#' {
		for _, r := range s[1:] {
			if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
				return ""
			}
		}
		return s
	}
	return ""
}

func storePluginPreviews(partDir string, phs []*multipart.FileHeader) int64 {
	for _, ph := range phs {
		f, err := ph.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(f, 5<<20))
		f.Close()
		if len(data) == 0 {
			continue
		}
		stored := randHex(8) + strings.ToLower(filepath.Ext(ph.Filename))
		if err := os.WriteFile(filepath.Join(partDir, stored), data, 0o600); err != nil {
			continue
		}
		return 0
	}
	return 0
}

// ────────────────────────────────────────────────────────────
// 插件市场 列表 / 详情 / 下载 / 分类 / 标签
// ────────────────────────────────────────────────────────────

func handlePluginMarketCategories(w http.ResponseWriter, r *http.Request) {
	cats, _ := db.ListPluginMarketCategories()
	jsonResp(w, M{"ok": true, "categories": cats})
}

func handlePluginMarketTags(w http.ResponseWriter, r *http.Request) {
	// MVP：由管理后台维护；此处返回空
	jsonResp(w, M{"ok": true, "tags": []string{}})
}

func handlePluginMarketFiles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	order := q.Get("order")
	cat := q.Get("category")
	tag := q.Get("tag")
	kw := q.Get("q")
	var rev *int
	if s := q.Get("review"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			rev = &v
		}
	}
	list, total, err := db.ListPluginMarketFiles(page, size, cat, tag, order, kw, rev)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true, "files": list, "total": total})
}

func handlePluginMarketFileDetail(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	f, err := db.GetPluginMarketFile(id)
	if err != nil || f == nil {
		jsonResp(w, M{"ok": false, "error": "插件不存在~"})
		return
	}
	user := auth.GetUser(r.Context())
	userID := int64(0)
	if user != nil {
		userID = user.ID
	}
	versions, _ := db.ListPluginVersions(id)
	owned := userID > 0 && f.UploaderID == userID
	jsonResp(w, M{"ok": true, "file": f, "versions": versions, "is_owner": owned})
}

func handlePluginMarketFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	f, err := db.GetPluginMarketFile(id)
	if err != nil || f == nil || (f.ReviewState != 0 && f.ReviewState != 1) {
		jsonResp(w, M{"ok": false, "error": "插件不存在或已下架~"})
		return
	}
	ver, err := db.GetLatestPluginVersion(id)
	if err != nil || ver == nil || ver.ZipPartID == 0 {
		jsonResp(w, M{"ok": false, "error": "插件数据缺失~"})
		return
	}
	part, err := db.GetPluginMarketPart(ver.ZipPartID)
	if err != nil || part == nil {
		jsonResp(w, M{"ok": false, "error": "插件文件缺失~"})
		return
	}
	user := auth.GetUser(r.Context())
	switch {
	case f.PricePts == 0 && f.AllowAnon:
		_ = db.RecordFreePluginDownload(id)
	case f.PricePts == 0 && !f.AllowAnon:
		if user == nil {
			jsonResp(w, M{"ok": false, "error": "该插件需要登录后下载~"})
			return
		}
		_ = db.RecordFreePluginDownload(id)
	default:
		if user == nil {
			jsonResp(w, M{"ok": false, "error": "付费插件需要登录后下载~"})
			return
		}
		if _, err := db.PurchaseOrReusePlugin(id, user.ID, f.PricePts, f.UploaderID); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
	}
	abs := filepath.Join(f.PartDir, filepath.Clean(part.RelPath))
	if _, err := os.Stat(abs); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(f.PluginID+".zip")))
	http.ServeFile(w, r, abs)
}

// ────────────────────────────────────────────────────────────
// 插件市场 举报 / 评论
// ────────────────────────────────────────────────────────────

func handlePluginMarketFileReport(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "请先登录~")})
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	reason := sanitizeText(req.Reason, 500)
	autoTD, err := db.AddPluginMarketReport(id, user.ID, reason)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if autoTD {
		jsonResp(w, M{"ok": true, "auto_takedown": true, "message": "该插件已收到较多举报,已自动下架,等待管理员处理~"})
		return
	}
	jsonResp(w, M{"ok": true, "auto_takedown": false})
}

// pluginCommentNode 插件市场评论树节点(多级回复)。
type pluginCommentNode struct {
	db.PluginMarketComment
	Replies []*pluginCommentNode `json:"replies"`
}

// buildPluginCommentTree 把扁平插件市场评论列表组建成多级回复树。
func buildPluginCommentTree(list []db.PluginMarketComment) []pluginCommentNode {
	nodes := make(map[int64]*pluginCommentNode, len(list))
	order := make([]int64, 0, len(list))
	for i := range list {
		n := &pluginCommentNode{PluginMarketComment: list[i], Replies: []*pluginCommentNode{}}
		nodes[list[i].ID] = n
		order = append(order, list[i].ID)
	}
	roots := []*pluginCommentNode{}
	for _, id := range order {
		n := nodes[id]
		if n.ParentID > 0 {
			if p, ok := nodes[n.ParentID]; ok {
				p.Replies = append(p.Replies, n)
				continue
			}
		}
		roots = append(roots, n)
	}
	out := make([]pluginCommentNode, 0, len(roots))
	for _, r := range roots {
		out = append(out, *r)
	}
	return out
}

func handlePluginMarketFileComments(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	user := auth.GetUser(r.Context())
	userID := int64(0)
	if user != nil {
		userID = user.ID
	}
	if r.Method == "POST" {
		if user == nil {
			jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "请先登录~")})
			return
		}
		var req struct {
			Content  string `json:"content"`
			ParentID int64  `json:"parent_id"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		content := sanitizeText(req.Content, maxCommentLen)
		if content == "" {
			jsonResp(w, M{"ok": false, "error": "评论内容不能为空~"})
			return
		}
		_, err := db.CreatePluginMarketComment(id, req.ParentID, user.ID, content)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		jsonResp(w, M{"ok": true})
		return
	}
	list, err := db.ListPluginMarketComments(id, userID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true, "comments": buildPluginCommentTree(list)})
}

func handlePluginMarketCommentDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "请先登录~")})
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var fileID int64
	db.DB.QueryRow(`SELECT file_id FROM plugin_market_comments WHERE id = ?`, id).Scan(&fileID)
	var uploader int64
	db.DB.QueryRow(`SELECT uploader_id FROM plugin_market_files WHERE id = ?`, fileID).Scan(&uploader)
	if err := db.DeletePluginMarketComment(id, user.ID, uploader); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true})
}

func handlePluginMarketMyFiles(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "请先登录~")})
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	list, total, err := db.ListPluginMarketFilesByUploader(user.ID, page, size)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true, "files": list, "total": total})
}

func handlePluginMarketFileOwnerEdit(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "请先登录~")})
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var u db.PluginMarketOwnerUpdate
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		jsonResp(w, M{"ok": false, "error": "无效的请求~"})
		return
	}
	if err := db.UpdatePluginMarketFileOwner(id, user.ID, &u); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true})
}

func handlePluginMarketFileOwnerDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "请先登录~")})
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	f, err := db.GetPluginMarketFile(id)
	if err != nil || f == nil || f.UploaderID != user.ID {
		jsonResp(w, M{"ok": false, "error": "无权删除该插件~"})
		return
	}
	dir, err := db.DeletePluginMarketFile(id)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if dir != "" {
		os.RemoveAll(filepath.Join(pluginMarketDataRoot, filepath.Base(dir)))
	}
	jsonResp(w, M{"ok": true})
}

// ────────────────────────────────────────────────────────────
// 插件市场 管理后台
// ────────────────────────────────────────────────────────────

func handleAdminPluginMarketFiles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	var rev *int
	if s := q.Get("review"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			rev = &v
		}
	}
	list, total, err := db.ListAdminPluginMarketFiles(page, size, q.Get("q"), rev)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true, "files": list, "total": total})
}

func handleAdminPluginMarketFlagged(w http.ResponseWriter, r *http.Request) {
	list, _, err := db.ListAdminPluginMarketFiles(1, 100, "", nil)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	flagged := []db.PluginMarketFile{}
	for _, f := range list {
		if f.DedupFlag != 0 {
			flagged = append(flagged, f)
		}
	}
	jsonResp(w, M{"ok": true, "files": flagged, "total": len(flagged)})
}

func handleAdminPluginMarketReview(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var req struct {
		Action string `json:"action"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	switch req.Action {
	case "approve":
		if err := db.SetPluginMarketReviewState(id, 1); err != nil {
			jsonResp(w, M{"ok": false, "error": sysErr(err)})
			return
		}
	case "reject":
		if err := db.SetPluginMarketReviewState(id, 2); err != nil {
			jsonResp(w, M{"ok": false, "error": sysErr(err)})
			return
		}
	default:
		jsonResp(w, M{"ok": false, "error": "未知审核操作~"})
		return
	}
	jsonResp(w, M{"ok": true})
}

func handleAdminPluginMarketFileUnflag(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := db.ClearPluginMarketFlag(id); err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true})
}

func handleAdminPluginMarketFileDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	dir, err := db.DeletePluginMarketFile(id)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if dir != "" {
		os.RemoveAll(filepath.Join(pluginMarketDataRoot, filepath.Base(dir)))
	}
	jsonResp(w, M{"ok": true})
}

func handleAdminPluginMarketConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		c, _ := db.GetPluginMarketConfig()
		jsonResp(w, M{"ok": true, "config": c})
		return
	}
	var c db.PluginMarketConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		jsonResp(w, M{"ok": false, "error": "无效的请求~"})
		return
	}
	if err := db.UpdatePluginMarketConfig(&c); err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true})
}

func handleAdminPluginMarketCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		cats, _ := db.ListPluginMarketCategories()
		jsonResp(w, M{"ok": true, "categories": cats})
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := db.CreatePluginMarketCategory(req.Name); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true})
}

func handleAdminPluginMarketFileUpdate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var req struct {
		PricePts    int      `json:"price_pts"`
		AllowAnon   bool     `json:"allow_anonymous"`
		Tags        []string `json:"tags"`
		Description []string `json:"description"`
		Docs        []string `json:"docs"`
		Categories  []string `json:"categories"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := db.UpdatePluginMarketFileDetail(id, req.PricePts, req.AllowAnon, req.Tags, req.Description, req.Docs, req.Categories); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true})
}
