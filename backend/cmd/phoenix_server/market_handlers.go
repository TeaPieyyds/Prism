package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"image/gif"
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

	"golang.org/x/image/webp"
)

// ────────────────────────────────────────────────────────────
// 常量
// ────────────────────────────────────────────────────────────

const marketDataRoot = "data/market"

const (
	maxFileBytes = 20 << 20               // 单文件/解压后总量上限 20MB
	maxReqBytes  = (20 << 20) + (1 << 20) // 请求体(含 multipart 边界)
	// 预览规范
	maxWebpEdge   = 2048
	maxWebpArea   = 4_000_000
	maxWebpBytes  = 1 << 20
	maxGifEdge    = 1024
	maxGifFrames  = 72
	maxGifBytes   = 2 << 20
	maxPreviewImg = 5
	maxPreviewGif = 1
	// 文本
	maxTitleLen   = 60
	maxDescLen    = 2000
	maxTags       = 10
	maxTagLen     = 20
	maxPricePts   = 999999
	maxBlockCount = 5_000_000
	maxBlockDim   = 10000
)

// 建筑主体扩展名白名单
var structureExts = map[string]bool{".mcstructure": true, ".mcworld": true, ".schematic": true, ".schem": true}

// 上传限流：每用户 6 次/分钟（预检+上传各计 1,一次完整上传约 2 次）→ 约 3 次完整上传/分,防刷
var marketUploadLimiter = auth.NewRateLimiter(6, 60*time.Second)

// ────────────────────────────────────────────────────────────
// 小工具
// ────────────────────────────────────────────────────────────

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func md5Hex(b []byte) string {
	h := md5.Sum(b)
	return hex.EncodeToString(h[:])
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// 需求8:体方块统计在客户端本地计算后,用本密钥 AES-GCM 加密上传(基础 TLS 之外的自有混淆)。
// 密钥由固定短语经 SHA-256 派生,服务端与 bot-apk 客户端各持一份相同实现。
var marketStatsKey = func() []byte {
	h := sha256.Sum256([]byte("prism-market-stats-key-v1-2026"))
	return h[:]
}()

func encryptMarketStats(plain []byte) (string, error) {
	block, err := aes.NewCipher(marketStatsKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, plain, nil)), nil
}

func decryptMarketStats(enc string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(marketStatsKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, fmt.Errorf("统计数据过短")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

// blockStats 客户端加密上传的体方块统计明文。
type blockStats struct {
	BlockCount int            `json:"block_count"`
	NbtCount   int            `json:"nbt_count"`
	BlockDims  map[string]int `json:"block_dims"`
	FileSize   int64          `json:"file_size"`
}

// ── 文件列表应用层加密（非对称，hybrid）──
// 需求：文件列表只能被带私钥的 Toolbox App 解密；服务端用 App 公钥加密，
// App 用内置私钥解密。传输走 TLS 之上再加一层应用层加密，防非 App 客户端
// 直接 curl 读取列表标题/内容。
// 私钥由客户端在构建时注入（keys/market_priv.pem，不进 git），此处只持有公钥。
// 本公钥对应当前 App 内置私钥；更换密钥对时必须同时更新此处与 App 端私钥。

// appListPubKey App 端 RSA 公钥（PKIX/PEM），用于加密文件列表。
const appListPubKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAuN672Ja/QQ/d+Sx7zWtd
pcHUZkSUw60aktJeXU0Zi/kMo7NKx4E7Io0bu6stlzEMhFLz3/XbFow0FBNb4aaJ
rXEZVTYWwsBquhxkhYWYaGFeD+QK02fhwYJTijCt9scAYIizZ45Y/8VyhwsQerqm
JS8h+rRaUTNFU0GBRRUhsPEQ8qzU2NME69KrshXMSvovq5SPbJT286OXgXLwNY6c
pqD1e3ug7eUfGc4Yjvq6QcFm9fmAUk55vZu+zOD9I5es/rPXfPzEUBb1pZ5gKI9I
JEpRqWxYzpvkqVYXbizJeacEr0nZarbkZKMeHUJkkzfmR0ga9mI+zslsO+S9K5gy
iwIDAQAB
-----END PUBLIC KEY-----`

// encryptMarketList 用 App 公钥混合加密一份文件列表：
// 随机 AES-256 key 加密每个文件元数据 JSON，再用 RSA-OAEP 加密该 AES key。
// 返回 (keyEncB64, 每个文件的 metaEncB64, err)。
func encryptMarketList(list []db.MarketFile) (string, []string, error) {
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		return "", nil, err
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", nil, err
	}
	// RSA 加密 AES key
	pubBlock, _ := pem.Decode([]byte(appListPubKey))
	if pubBlock == nil {
		return "", nil, fmt.Errorf("App 公钥解析失败")
	}
	pubAny, err := x509.ParsePKIXPublicKey(pubBlock.Bytes)
	if err != nil {
		return "", nil, err
	}
	rsaPub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return "", nil, fmt.Errorf("公钥不是 RSA")
	}
	keyEnc, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaPub, aesKey, nil)
	if err != nil {
		return "", nil, err
	}
	metas := make([]string, 0, len(list))
	for i := range list {
		f := &list[i]
		meta, _ := json.Marshal(map[string]any{
			"title": f.Title, "description": f.Description, "price_pts": f.PricePts,
			"download_count": f.DownloadCount, "block_count": f.BlockCount, "nbt_count": f.NbtCount,
			"categories": f.Categories, "tags": f.Tags, "flagged": f.Flagged,
			"allow_anonymous": f.AllowAnon, "author": f.UploaderName,
		})
		nonce := make([]byte, gcm.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return "", nil, err
		}
		metas = append(metas, base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, meta, nil)))
	}
	return base64.StdEncoding.EncodeToString(keyEnc), metas, nil
}

// sanitizeText 净化用户文本：去控制字符、限制长度。防存储型 XSS/注入。
func sanitizeText(s string, maxLen int) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	r := []rune(s)
	if len(r) > maxLen {
		r = r[:maxLen]
	}
	return string(r)
}

// sanitizeFilename 清洗下载文件名,防响应头注入。
func sanitizeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		switch r {
		case '"', '\n', '\r', '/', '\\', ':':
			return '_'
		}
		return r
	}, name)
	if name == "" {
		name = "download"
	}
	return name
}

func isWebP(data []byte) bool {
	return len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP"))
}

func isGIF(data []byte) bool {
	return len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a")))
}

// previewMeta 阶段一提交的预览素材元数据。
type previewMeta struct {
	Type string `json:"type"`
	Ext  string `json:"ext"`
	MD5  string `json:"md5"`
	W    int    `json:"w"`
	H    int    `json:"h"`
	Size int64  `json:"size"`
}

// recompressGIF 尽可能压缩 GIF：重新编码 + 去除连续重复帧。
func recompressGIF(data []byte) ([]byte, error) {
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if len(g.Image) == 0 {
		return nil, fmt.Errorf("GIF 无帧")
	}
	// 去连续重复帧(比较上一帧像素是否相同)
	out := gif.GIF{LoopCount: 0}
	var prevRaw []byte
	for i, frame := range g.Image {
		raw := frame.Pix
		dup := false
		if prevRaw != nil && bytes.Equal(prevRaw, raw) {
			dup = true
		}
		if !dup {
			out.Image = append(out.Image, frame)
			delay := 10
			if i < len(g.Delay) && g.Delay[i] > 0 {
				delay = g.Delay[i]
			}
			out.Delay = append(out.Delay, delay)
			prevRaw = raw
		}
	}
	if len(out.Image) == 0 {
		out.Image = g.Image
		out.Delay = g.Delay
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// readUploadedFile 读取上传文件字节,校验上限。
func readUploadedFile(fh *multipart.FileHeader) ([]byte, error) {
	if fh.Size > maxFileBytes {
		return nil, fmt.Errorf("文件过大,上限 %dMB", maxFileBytes>>20)
	}
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// ────────────────────────────────────────────────────────────
// 阶段一：元数据预校验 → 签发 TTS
// ────────────────────────────────────────────────────────────

type precheckFile struct {
	Name      string         `json:"name"`
	Ext       string         `json:"ext"`
	Size      int64          `json:"size"`
	MD5       string         `json:"md5"`
	StatsEnc  string         `json:"stats_enc"`  // 客户端加密的 blockStats;优先于明文 BlockCount/BlockDims
	BlockCount int           `json:"block_count"` // 兼容旧客户端明文
	BlockDims map[string]int `json:"block_dims"`
}

type precheckReq struct {
	PricePts int           `json:"price_pts"`
	File     precheckFile  `json:"file"`
	Previews []previewMeta `json:"previews"`
}

func handleMarketPrecheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "请先登录~")})
		return
	}
	if !marketUploadLimiter.Allow(strconv.FormatInt(user.ID, 10)) {
		jsonResp(w, M{"ok": false, "error": "上传太频繁,请稍后再试~"})
		return
	}
	var req precheckReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, M{"ok": false, "error": "无效的请求~"})
		return
	}

	// 文件本体校验
	f := req.File
	ext := strings.ToLower(f.Ext)
	if !structureExts[ext] {
		jsonResp(w, M{"ok": false, "error": "不支持的建筑文件格式~"})
		return
	}
	if f.Size <= 0 || f.Size > maxFileBytes {
		jsonResp(w, M{"ok": false, "error": fmt.Sprintf("文件大小需在 1B~%dMB 之间~", maxFileBytes>>20)})
		return
	}
	if f.MD5 == "" {
		jsonResp(w, M{"ok": false, "error": "缺少文件 MD5~"})
		return
	}
	// 体方块统计:优先解密客户端加密的 stats_enc(需求8),否则兼容旧客户端明文
	blockCount, nbtCount := f.BlockCount, 0
	dims := f.BlockDims
	if f.StatsEnc != "" {
		raw, derr := decryptMarketStats(f.StatsEnc)
		if derr != nil {
			jsonResp(w, M{"ok": false, "error": "体方块统计数据校验失败~"})
			return
		}
		var st blockStats
		if json.Unmarshal(raw, &st) != nil {
			jsonResp(w, M{"ok": false, "error": "体方块统计数据解析失败~"})
			return
		}
		if st.FileSize > 0 && st.FileSize != f.Size {
			jsonResp(w, M{"ok": false, "error": "文件大小与统计不一致~"})
			return
		}
		blockCount, nbtCount, dims = st.BlockCount, st.NbtCount, st.BlockDims
	}
	if blockCount < 0 || blockCount > maxBlockCount || nbtCount < 0 || nbtCount > maxBlockCount {
		jsonResp(w, M{"ok": false, "error": "体方块数据异常~"})
		return
	}
	for _, v := range dims {
		if v <= 0 || v > maxBlockDim {
			jsonResp(w, M{"ok": false, "error": "体方块尺寸异常~"})
			return
		}
	}

	// 预览素材校验
	if len(req.Previews) < 1 {
		jsonResp(w, M{"ok": false, "error": "至少需要 1 张 WebP 预览图~"})
		return
	}
	var imgCount, gifCount int
	previewsJSON, _ := json.Marshal(req.Previews)
	for _, p := range req.Previews {
		ext = strings.ToLower(p.Ext)
		switch p.Type {
		case "preview_img":
			imgCount++
			if ext != ".webp" {
				jsonResp(w, M{"ok": false, "error": "预览图只允许 WebP 格式,禁止混入其他格式~"})
				return
			}
			if p.Size <= 0 || p.Size > maxWebpBytes {
				jsonResp(w, M{"ok": false, "error": "WebP 预览图大小超限~"})
				return
			}
			if p.W <= 0 || p.W > maxWebpEdge || p.H <= 0 || p.H > maxWebpEdge || p.W*p.H > maxWebpArea {
				jsonResp(w, M{"ok": false, "error": "WebP 预览图分辨率超限~"})
				return
			}
		case "preview_gif":
			gifCount++
			if req.PricePts <= 0 {
				jsonResp(w, M{"ok": false, "error": "环绕动画仅付费文件可上传~"})
				return
			}
			if ext != ".gif" {
				jsonResp(w, M{"ok": false, "error": "动画只允许 GIF 格式~"})
				return
			}
			if p.Size <= 0 || p.Size > maxGifBytes {
				jsonResp(w, M{"ok": false, "error": "GIF 大小超限~"})
				return
			}
		default:
			jsonResp(w, M{"ok": false, "error": "未知预览类型~"})
			return
		}
		if p.MD5 == "" {
			jsonResp(w, M{"ok": false, "error": "预览素材缺少 MD5~"})
			return
		}
	}
	if imgCount < 1 || imgCount > maxPreviewImg {
		jsonResp(w, M{"ok": false, "error": fmt.Sprintf("WebP 预览图数量需在 1~%d 之间~", maxPreviewImg)})
		return
	}
	if gifCount > maxPreviewGif {
		jsonResp(w, M{"ok": false, "error": "环绕动画最多 1 个~"})
		return
	}

	// MD5 查重
	if existing, err := db.GetMarketFileByMD5(f.MD5); err == nil && existing != nil {
		jsonResp(w, M{"ok": false, "error": "该文件已存在", "file_id": existing.ID})
		return
	}

	// 签发一次性 TTS
	tts := randHex(12) // 96bit
	id, err := db.CreateMarketUploadSession(sha256Hex(tts), user.ID, f.MD5, f.Size, string(previewsJSON), req.PricePts, blockCount, nbtCount, dims)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	log.Printf("[MARKET] %s 预检通过,签发 TTS session=%d md5=%s", user.Username, id, f.MD5)
	jsonResp(w, M{"ok": true, "tts": tts, "session_id": id, "expires_in": 300})
}

// ────────────────────────────────────────────────────────────
// 阶段二：实体上传（用 TTS 鉴权）
// ────────────────────────────────────────────────────────────

type uploadMeta struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Categories  []string       `json:"categories"`
	Tags        []string       `json:"tags"`
	PricePts    int            `json:"price_pts"`
	AllowAnon   bool           `json:"allow_anonymous"`
	PreviewCam  map[string]int `json:"preview_cam"`
}

func handleMarketUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
		return
	}
	// 用 TTS 鉴权,不用 SessionAuth
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
	sess, err := db.GetMarketUploadSession(sha256Hex(tts))
	if err != nil || sess == nil || sess.Used {
		jsonResp(w, M{"ok": false, "error": "临时令牌无效或已使用~"})
		return
	}
	if sess.ExpiresAt < time.Now().UTC().Format(time.RFC3339) {
		jsonResp(w, M{"ok": false, "error": "临时令牌已过期,请重新预检~"})
		return
	}
	if !marketUploadLimiter.Allow(strconv.FormatInt(sess.UserID, 10)) {
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": "上传太频繁,请稍后再试~"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxReqBytes)
	if err := r.ParseMultipartForm(maxReqBytes); err != nil {
		jsonResp(w, M{"ok": false, "error": "上传数据过大~"})
		return
	}
	// 立即标记会话已用,防重放(失败时整体清理)
	_ = db.MarkMarketUploadSessionUsed(sess.ID)

	_, fh, err := r.FormFile("file")
	if err != nil {
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": "缺少建筑文件~"})
		return
	}
	fileData, err := readUploadedFile(fh)
	if err != nil {
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": "读取建筑文件失败: " + err.Error()})
		return
	}
	if int64(len(fileData)) != sess.FileSize || md5Hex(fileData) != sess.FileMD5 {
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": "文件与预检不一致,请重新预检~"})
		return
	}
	// 内容魔数校验:拒绝伪装成建筑文件的任意二进制
	structExt := strings.ToLower(filepath.Ext(fh.Filename))
	if !structureExts[structExt] {
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": "建筑文件扩展名异常~"})
		return
	}
	if err := validateStructureFile(fileData, structExt); err != nil {
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	if err := checkStructureCompression(fileData, structExt); err != nil {
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}

	var meta uploadMeta
	metaRaw := r.FormValue("meta")
	if metaRaw == "" {
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": "缺少文件描述信息~"})
		return
	}
	if err := json.Unmarshal([]byte(metaRaw), &meta); err != nil {
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": "描述信息格式错误~"})
		return
	}
	if meta.PricePts < 0 || meta.PricePts > maxPricePts {
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": "价格超出范围~"})
		return
	}
	// 预检 price 与上传 meta price 必须一致
	if meta.PricePts != sess.PricePts {
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": "价格与预检不一致,请重新预检~"})
		return
	}

	// 校验并落盘
	partDir := filepath.Join(marketDataRoot, randHex(8))
	os.MkdirAll(partDir, 0o700)

	parts := []db.MarketPart{}
	ext := strings.ToLower(filepath.Ext(meta.Title))
	_ = ext
	// 建筑主体 part（structExt 已在前面校验块计算）
	structName := filepath.Base(fh.Filename)
	if !structureExts[structExt] {
		os.RemoveAll(partDir)
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": "建筑文件扩展名异常~"})
		return
	}
	stored := randHex(8) + structExt
	rel := filepath.Join(partDir, stored)
	if err := os.WriteFile(rel, fileData, 0o600); err != nil {
		os.RemoveAll(partDir)
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": "保存文件失败~"})
		return
	}
	parts = append(parts, db.MarketPart{
		PartType: "structure", OrigName: structName, StoredName: stored,
		Ext: structExt, RelPath: stored, MD5: sess.FileMD5, Size: int64(len(fileData)),
	})

	// 预览素材
	expected, _ := decodePreviewsMeta(sess.PreviewsMD)
	usedIdx := make([]bool, len(expected))
	ph := r.MultipartForm.File["previews"]
	if len(ph) < 1 {
		os.RemoveAll(partDir)
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": "缺少预览素材~"})
		return
	}
	for _, fph := range ph {
		pdata, err := readUploadedFile(fph)
		if err != nil {
			os.RemoveAll(partDir)
			cleanupSession(sess)
			jsonResp(w, M{"ok": false, "error": "预览素材过大~"})
			return
		}
		pmd5 := md5Hex(pdata)
		// 找到匹配的期望元数据
		idx := -1
		for i, e := range expected {
			if !usedIdx[i] && e.MD5 == pmd5 {
				idx = i
				break
			}
		}
		if idx < 0 {
			os.RemoveAll(partDir)
			cleanupSession(sess)
			jsonResp(w, M{"ok": false, "error": "预览素材与预检不一致~"})
			return
		}
		usedIdx[idx] = true
		exp := expected[idx]
		ptype, pstore, perr := storePreview(partDir, exp, pdata)
		if perr != nil {
			os.RemoveAll(partDir)
			cleanupSession(sess)
			jsonResp(w, M{"ok": false, "error": perr.Error()})
			return
		}
		parts = append(parts, db.MarketPart{
			PartType: ptype, OrigName: fph.Filename, StoredName: pstore,
			Ext: strings.ToLower(exp.Ext), RelPath: pstore, MD5: pmd5,
			Width: exp.W, Height: exp.H, Size: int64(len(pdata)),
		})
	}
	// 确认所有期望素材都已上传
	for i, e := range expected {
		if !usedIdx[i] {
			os.RemoveAll(partDir)
			cleanupSession(sess)
			jsonResp(w, M{"ok": false, "error": fmt.Sprintf("缺少预览素材 %s~", e.MD5[:8])})
			return
		}
	}

	// 文本净化
	title := sanitizeText(meta.Title, maxTitleLen)
	if title == "" {
		os.RemoveAll(partDir)
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": "标题不能为空~"})
		return
	}
	desc := sanitizeText(meta.Description, maxDescLen)
	if len(meta.Tags) > maxTags {
		os.RemoveAll(partDir)
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": fmt.Sprintf("标签最多 %d 个~", maxTags)})
		return
	}
	tags := make([]string, 0, len(meta.Tags))
	for _, t := range meta.Tags {
		t = sanitizeText(strings.TrimSpace(t), maxTagLen)
		if t != "" {
			tags = append(tags, t)
		}
	}

	file := &db.MarketFile{
		UploaderID: sess.UserID, Title: title, Description: desc, Categories: meta.Categories,
		Tags: tags, PricePts: meta.PricePts, AllowAnon: meta.AllowAnon, MD5: sess.FileMD5,
		BlockCount: sess.BlockCount, NbtCount: sess.NbtCount, FileSize: sess.FileSize,
		BlockDims: sess.BlockDims, PreviewCam: meta.PreviewCam, PartDir: filepath.ToSlash(partDir),
	}
	fileID, err := db.CreateMarketFile(file, parts)
	if err != nil {
		os.RemoveAll(partDir)
		cleanupSession(sess)
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	_ = db.DeleteMarketUploadSession(sess.ID)

	// 发放上传奖励
	if rwErr := db.CreditUploadReward(sess.UserID); rwErr != nil {
		log.Printf("[MARKET] 上传奖励发放失败 uid=%d: %v", sess.UserID, rwErr)
	}
	log.Printf("[MARKET] 上传成功 file=%d uid=%d title=%q", fileID, sess.UserID, title)
	jsonResp(w, M{"ok": true, "id": fileID})
}

// decodePreviewsMeta 解析阶段一存的预览元数据 JSON。
func decodePreviewsMeta(raw string) ([]previewMeta, error) {
	var list []previewMeta
	if raw == "" {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

// storePreview 校验并落盘一张预览素材,返回 (partType, storedName, err)。GIF 会重新压缩。
func storePreview(partDir string, exp previewMeta, data []byte) (string, string, error) {
	switch exp.Type {
	case "preview_img":
		if !isWebP(data) {
			return "", "", fmt.Errorf("预览图不是合法 WebP~")
		}
		cfg, err := webp.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return "", "", fmt.Errorf("WebP 解码失败~")
		}
		if cfg.Width > maxWebpEdge || cfg.Height > maxWebpEdge || cfg.Width*cfg.Height > maxWebpArea {
			return "", "", fmt.Errorf("WebP 分辨率超限~")
		}
		stored := randHex(8) + ".webp"
		if err := os.WriteFile(filepath.Join(partDir, stored), data, 0o600); err != nil {
			return "", "", err
		}
		return "preview_img", stored, nil
	case "preview_gif":
		if !isGIF(data) {
			return "", "", fmt.Errorf("动画不是合法 GIF~")
		}
		compressed, err := recompressGIF(data)
		if err != nil {
			return "", "", fmt.Errorf("GIF 处理失败~")
		}
		stored := randHex(8) + ".gif"
		if err := os.WriteFile(filepath.Join(partDir, stored), compressed, 0o600); err != nil {
			return "", "", err
		}
		return "preview_gif", stored, nil
	}
	return "", "", fmt.Errorf("未知预览类型~")
}

// validateStructureFile 校验建筑文件内容魔数,防止把任意二进制(改后缀的 exe/so)伪装上传并分发给其他玩家。
func validateStructureFile(data []byte, ext string) error {
	if len(data) < 8 {
		return fmt.Errorf("建筑文件数据异常(过短)")
	}
	// 拒绝明确的非建筑魔数:可执行/脚本/已知媒体格式
	switch {
	case bytes.Equal(data[:2], []byte("MZ")):
		return fmt.Errorf("检测到 Windows 可执行文件,拒绝上传")
	case len(data) >= 4 && bytes.Equal(data[:4], []byte{0x7f, 'E', 'L', 'F'}):
		return fmt.Errorf("检测到 Linux ELF 可执行文件,拒绝上传")
	case len(data) >= 4 && bytes.Equal(data[:4], []byte{0xca, 0xfe, 0xba, 0xbe}):
		return fmt.Errorf("检测到可执行文件,拒绝上传")
	case bytes.Equal(data[:2], []byte("#!")):
		return fmt.Errorf("检测到脚本文件,拒绝上传")
	case bytes.Equal(data[:2], []byte{0xff, 0xd8}):
		return fmt.Errorf("检测到 JPEG 图片,非建筑文件")
	case len(data) >= 4 && bytes.Equal(data[:4], []byte("\x89PNG")):
		return fmt.Errorf("检测到 PNG 图片,非建筑文件")
	case len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a"))):
		return fmt.Errorf("检测到 GIF 图片,非建筑文件")
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")):
		return fmt.Errorf("检测到 RIFF 媒体文件,非建筑文件")
	}
	// 要求符合建筑文件格式魔数
	switch strings.ToLower(ext) {
	case ".mcworld":
		if !bytes.Equal(data[:2], []byte("PK")) {
			return fmt.Errorf("mcworld 应为 ZIP 格式")
		}
	case ".mcstructure":
		// 内容校验:解压(限时限量)后必须是合法 NBT 结构。超时/超内存/非结构一律"不完整",
		// 不泄露具体校验规则,防恶意上传者试探。
		if err := validateMCStructure(data); err != nil {
			return err
		}
	case ".schematic":
		if err := validateSchematic(data); err != nil {
			return err
		}
	case ".schem":
		if !(data[0] == 0x1f && data[1] == 0x8b) && data[0] != 0x0a {
			return fmt.Errorf("schem 格式异常")
		}
	}
	return nil
}

// checkStructureCompression 检查压缩格式的"解压炸弹"。
// 只读压缩元数据,绝不实际解压(服务端只存不解压,解压发生在下载端):
//   - .mcworld(ZIP): 用 zip.NewReader 只读中央目录,校验条目数与声明解压总大小
//   - gzip 结构文件: 读尾部 ISIZE(未压缩大小 mod 2^32)声明
//
// 超限拒收,防止恶意压缩文件。
func checkStructureCompression(data []byte, ext string) error {
	const maxDecompressed = 500 << 20 // 500MB 声明解压上限(远大于 20MB 压缩上限的合理膨胀)
	switch strings.ToLower(ext) {
	case ".mcworld": // ZIP:只读中央目录,不提取
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return fmt.Errorf("mcworld ZIP 结构异常: %v", err)
		}
		if len(zr.File) > 500 {
			return fmt.Errorf("mcworld 内含文件过多(%d),疑似解压炸弹", len(zr.File))
		}
		var total uint64
		for _, f := range zr.File {
			total += f.UncompressedSize64
		}
		if total > maxDecompressed {
			return fmt.Errorf("mcworld 声明解压规模过大(%dMB),疑似解压炸弹", total>>20)
		}
	case ".mcstructure", ".schematic", ".schem":
		if data[0] == 0x1f && data[1] == 0x8b { // gzip:读尾部 ISIZE
			if len(data) >= 4 {
				isize := binary.LittleEndian.Uint32(data[len(data)-4:])
				if isize > maxDecompressed {
					return fmt.Errorf("gzip 声明解压规模过大(%dMB),疑似解压炸弹", isize>>20)
				}
			}
		}
	}
	return nil
}

// 结构校验资源上限:解压上限 200MB、耗时上限 5 秒。
// 超限即判定文件损坏,立即终止,避免恶意压缩包拖垮内存/CPU。
const (
	maxStructDecompressed = 200 << 20
	structDecompressLimit = 5 * time.Second
)

// validateMCStructure 校验 .mcstructure 内容:解压(若 gzip,限时限量)后
// 必须是合法 NBT 结构。任何失败返回通用的"建筑文件不完整",不泄露具体规则。
func validateMCStructure(data []byte) error {
	if len(data) < 1 {
		return fmt.Errorf("建筑文件不完整")
	}
	if data[0] == 0x1f && data[1] == 0x8b { // gzip:限时限量解压
		var err error
		data, err = gunzipBounded(data)
		if err != nil {
			return err
		}
	}
	if len(data) < 1 || data[0] != 0x0a { // NBT TAG_Compound
		return fmt.Errorf("建筑文件不完整")
	}
	// 结构必需字段(ASCII 字段名出现在解压后的 NBT 字节流中)
	if !bytes.Contains(data, []byte("structure")) || !bytes.Contains(data, []byte("block_indices")) {
		return fmt.Errorf("建筑文件不完整")
	}
	return nil
}

// validateSchematic 校验 .schematic(Java Schematic)内容:解压后是合法 NBT 且含 Schematic
// 必需字段(BlockData/Data + Palette)。与 .mcstructure 字段不同,单独校验。
func validateSchematic(data []byte) error {
	if len(data) < 1 {
		return fmt.Errorf("建筑文件不完整")
	}
	if data[0] == 0x1f && data[1] == 0x8b { // gzip:限时限量解压
		var err error
		data, err = gunzipBounded(data)
		if err != nil {
			return err
		}
	}
	if len(data) < 1 || data[0] != 0x0a { // NBT TAG_Compound
		return fmt.Errorf("建筑文件不完整")
	}
	// Schematic 必需字段:v1(Blocks + 尺寸)或 v2(BlockData + Palette + 尺寸)
	if !bytes.Contains(data, []byte("Width")) || !bytes.Contains(data, []byte("Height")) || !bytes.Contains(data, []byte("Length")) {
		return fmt.Errorf("建筑文件不完整")
	}
	if !bytes.Contains(data, []byte("Blocks")) && !bytes.Contains(data, []byte("BlockData")) {
		return fmt.Errorf("建筑文件不完整")
	}
	return nil
}

// gunzipBounded 解压 gzip 数据,同时受字节数上限与耗时上限约束;超限返回"不完整"。
func gunzipBounded(src []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("建筑文件不完整")
	}
	defer gz.Close()
	br := &boundedReader{r: gz, remaining: maxStructDecompressed, deadline: time.Now().Add(structDecompressLimit)}
	out, err := io.ReadAll(br)
	if err != nil {
		return nil, fmt.Errorf("建筑文件不完整")
	}
	return out, nil
}

// boundedReader 包装读取器:同时限制解压字节数(内存)与耗时(CPU)。
type boundedReader struct {
	r         io.Reader
	remaining int64
	deadline  time.Time
}

func (b *boundedReader) Read(p []byte) (int, error) {
	if time.Now().After(b.deadline) {
		return 0, fmt.Errorf("耗时过长")
	}
	if b.remaining <= 0 {
		return 0, fmt.Errorf("解压过大")
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.r.Read(p)
	b.remaining -= int64(n)
	return n, err
}

func cleanupSession(sess *db.MarketUploadSession) {
	if sess != nil {
		_ = db.DeleteMarketUploadSession(sess.ID)
	}
}

// ────────────────────────────────────────────────────────────
// 列表 / 详情 / GIF / 下载
// ────────────────────────────────────────────────────────────

func handleMarketCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := db.ListMarketCategories()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true, "categories": cats})
}

func handleMarketTags(w http.ResponseWriter, r *http.Request) {
	tags, err := db.GetAllMarketTags()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true, "tags": tags})
}

func handleMarketFiles(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	category := r.URL.Query().Get("category")
	tag := r.URL.Query().Get("tag")
	order := r.URL.Query().Get("order")
	q := r.URL.Query().Get("q")
	list, total, err := db.ListMarketFiles(page, size, category, tag, order, q)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	// 文件列表应用层加密：只返回 id/cover 明文，元数据密文，须 App 私钥才能解
	keyEnc, metas, err := encryptMarketList(list)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	out := make([]M, len(list))
	for i := range list {
		out[i] = M{"id": list[i].ID, "cover_part_id": list[i].CoverPartID, "meta_enc": metas[i]}
	}
	jsonResp(w, M{"ok": true, "total": total, "key_enc": keyEnc, "list": out})
}

func handleMarketFileDetail(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id <= 0 {
		jsonResp(w, M{"ok": false, "error": "参数错误~"})
		return
	}
	f, err := db.GetMarketFile(id)
	if err != nil || f == nil || f.Status != 1 {
		jsonResp(w, M{"ok": false, "error": "文件不存在或已下架~"})
		return
	}
	parts, _ := db.GetMarketFileParts(id)
	user := auth.GetUser(r.Context())
	already := false
	canDownload := true
	isOwner := false
	if f.PricePts > 0 {
		canDownload = user != nil
		if user != nil {
			already, _ = db.UserHasPurchased(id, user.ID)
		}
	} else if !f.AllowAnon {
		canDownload = user != nil
	}
	if user != nil {
		isOwner = user.ID == f.UploaderID
	}
	previews := []M{}
	hasGif := false
	for _, p := range parts {
		if p.PartType == "preview_gif" {
			hasGif = true
			continue
		}
		if p.PartType == "preview_img" {
			previews = append(previews, M{
				"id": p.ID, "url": fmt.Sprintf("/api/market/parts/%d/preview", p.ID),
				"w": p.Width, "h": p.Height, "size": p.Size,
			})
		}
	}
	jsonResp(w, M{
		"ok": true, "file": M{
			"id": f.ID, "title": f.Title, "description": f.Description, "categories": f.Categories,
			"tags": f.Tags, "price_pts": f.PricePts, "allow_anonymous": f.AllowAnon,
			"download_count": f.DownloadCount, "block_count": f.BlockCount, "nbt_count": f.NbtCount,
			"file_size": f.FileSize, "block_dims": f.BlockDims, "cover_part_id": f.CoverPartID,
			"preview_cam": f.PreviewCam, "author": f.UploaderName, "created_at": f.CreatedAt,
			"flagged": f.Flagged, "report_count": f.ReportCount,
		},
		"previews": previews, "has_gif": hasGif,
		"can_download": canDownload, "already_purchased": already, "is_owner": isOwner,
	})
}

// handleMarketPartPreview 预览图直出(仅 WebP,公开免费)。
func handleMarketPartPreview(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	part, err := db.GetMarketFilePart(id)
	if err != nil || part == nil || part.PartType != "preview_img" {
		http.NotFound(w, r)
		return
	}
	f, err := db.GetMarketFile(part.FileID)
	if err != nil || f == nil || f.PartDir == "" {
		http.NotFound(w, r)
		return
	}
	abs := filepath.Join(f.PartDir, filepath.Clean(part.RelPath))
	if _, err := os.Stat(abs); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, r, abs)
}

// handleMarketFileGif 环绕动画:必须单独请求才返回。付费文件需已购。
func handleMarketFileGif(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	f, err := db.GetMarketFile(id)
	if err != nil || f == nil || f.Status != 1 {
		jsonResp(w, M{"ok": false, "error": "文件不存在~"})
		return
	}
	parts, _ := db.GetMarketFileParts(id)
	var gifPart *db.MarketPart
	for i := range parts {
		if parts[i].PartType == "preview_gif" {
			gifPart = &parts[i]
			break
		}
	}
	if gifPart == nil {
		jsonResp(w, M{"ok": false, "error": "该文件没有环绕动画~"})
		return
	}
	// 付费文件需已购
	if f.PricePts > 0 {
		user := auth.GetUser(r.Context())
		if user == nil {
			jsonResp(w, M{"ok": false, "error": "付费文件的环绕动画需要购买后才可查看~"})
			return
		}
		ok, _ := db.UserHasPurchased(id, user.ID)
		if !ok && user.Role != "admin" {
			jsonResp(w, M{"ok": false, "error": "购买后才能查看环绕动画~"})
			return
		}
	}
	abs := filepath.Join(f.PartDir, filepath.Clean(gifPart.RelPath))
	if _, err := os.Stat(abs); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, abs)
}

func handleMarketFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	f, err := db.GetMarketFile(id)
	if err != nil || f == nil || f.Status != 1 {
		jsonResp(w, M{"ok": false, "error": "文件不存在或已下架~"})
		return
	}
	parts, _ := db.GetMarketFileParts(id)
	var mainPart *db.MarketPart
	for i := range parts {
		if parts[i].PartType == "structure" {
			mainPart = &parts[i]
			break
		}
	}
	if mainPart == nil {
		jsonResp(w, M{"ok": false, "error": "文件数据缺失~"})
		return
	}
	user := auth.GetUser(r.Context())

	switch {
	case f.PricePts == 0 && f.AllowAnon:
		// 匿名免费
		_ = db.RecordFreeDownload(id)
	case f.PricePts == 0 && !f.AllowAnon:
		if user == nil {
			jsonResp(w, M{"ok": false, "error": "该文件需要登录后下载~"})
			return
		}
		_ = db.RecordFreeDownload(id)
	default: // 付费
		if user == nil {
			jsonResp(w, M{"ok": false, "error": "付费文件需要登录后下载~"})
			return
		}
		if _, err := db.PurchaseOrReuse(id, user.ID, f.PricePts, f.UploaderID); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
	}

	abs := filepath.Join(f.PartDir, filepath.Clean(mainPart.RelPath))
	if _, err := os.Stat(abs); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(mainPart.OrigName)))
	http.ServeFile(w, r, abs)
}

// ────────────────────────────────────────────────────────────
// 举报
// ────────────────────────────────────────────────────────────

func handleMarketReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
		return
	}
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
	autoTD, err := db.AddMarketReport(id, user.ID, reason)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	log.Printf("[MARKET] 举报 file=%d by=%s autoTakedown=%v", id, user.Username, autoTD)
	if autoTD {
		jsonResp(w, M{"ok": true, "auto_takedown": true, "message": "该文件已收到较多举报,已自动下架,等待管理员处理~"})
		return
	}
	jsonResp(w, M{"ok": true, "auto_takedown": false})
}

// ────────────────────────────────────────────────────────────
// 评论
// ────────────────────────────────────────────────────────────

const maxCommentLen = 500

// commentNode 评论树节点(多级回复)。
type commentNode struct {
	db.MarketComment
	Replies []*commentNode `json:"replies"`
}

// buildCommentTree 把扁平评论列表组建成多级回复树。
func buildCommentTree(list []db.MarketComment) []commentNode {
	// 用指针构建:先把所有节点放入 map(指针),再填 Replies,最后收集根。
	// 若根用值拷贝提前保存,后续 append 的回复只会进 map 里的副本,roots 里的
	// 副本 Replies 恒为空,导致"看不到回复"。
	nodes := make(map[int64]*commentNode, len(list))
	order := make([]int64, 0, len(list))
	for i := range list {
		n := &commentNode{MarketComment: list[i], Replies: []*commentNode{}}
		nodes[list[i].ID] = n
		order = append(order, list[i].ID)
	}
	roots := []*commentNode{}
	for _, id := range order {
		n := nodes[id]
		if n.ParentID > 0 {
			if p, ok := nodes[n.ParentID]; ok {
				// 存指针: 后续孙级回复 append 到同一节点,任意深度都能展开
				p.Replies = append(p.Replies, n)
				continue
			}
		}
		roots = append(roots, n)
	}
	out := make([]commentNode, 0, len(roots))
	for _, r := range roots {
		out = append(out, *r)
	}
	return out
}

func handleMarketFileComments(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id <= 0 {
		jsonResp(w, M{"ok": false, "error": "参数错误~"})
		return
	}
	switch r.Method {
	case "GET":
		cu := auth.GetUser(r.Context())
		var cur int64
		if cu != nil {
			cur = cu.ID
		}
		list, err := db.ListMarketComments(id, cur)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": sysErr(err)})
			return
		}
		jsonResp(w, M{"ok": true, "comments": buildCommentTree(list)})
	case "POST":
		user := auth.GetUser(r.Context())
		if user == nil {
			jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "请先登录~")})
			return
		}
		var req struct {
			ParentID int64  `json:"parent_id"`
			Content  string `json:"content"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		content := sanitizeText(strings.TrimSpace(req.Content), maxCommentLen)
		if content == "" {
			jsonResp(w, M{"ok": false, "error": "评论内容不能为空~"})
			return
		}
		cid, err := db.CreateMarketComment(id, req.ParentID, user.ID, content)
		if err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		log.Printf("[MARKET] 评论 file=%d by=%s cid=%d", id, user.Username, cid)
		jsonResp(w, M{"ok": true, "id": cid})
	default:
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
	}
}

func handleMarketCommentDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" && r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "请先登录~")})
		return
	}
	cid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	// 取该评论所属文件的作者
	var fileID int64
	if err := db.DB.QueryRow(`SELECT file_id FROM market_comments WHERE id = ?`, cid).Scan(&fileID); err != nil {
		jsonResp(w, M{"ok": false, "error": "评论不存在~"})
		return
	}
	f, err := db.GetMarketFile(fileID)
	if err != nil || f == nil {
		jsonResp(w, M{"ok": false, "error": "评论不存在~"})
		return
	}
	if err := db.DeleteMarketComment(cid, user.ID, f.UploaderID); err != nil {
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, M{"ok": true})
}

// ────────────────────────────────────────────────────────────
// 上传者编辑自己上传的文件
// ────────────────────────────────────────────────────────────

// coverReqVal 取 owner 编辑请求里显式传入的封面 part id(未传返回 0)。
func coverReqVal(u *db.MarketOwnerUpdate) int64 {
	if u.CoverPartID != nil {
		return *u.CoverPartID
	}
	return 0
}

func handleMarketFileOwnerEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "请先登录~")})
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	log.Printf("[MARKET] owner-edit req file=%d by=%s", id, user.Username)
	var req db.MarketOwnerUpdate
	json.NewDecoder(r.Body).Decode(&req)

	req.Title = sanitizeText(strings.TrimSpace(req.Title), maxTitleLen)
	if req.Title == "" {
		jsonResp(w, M{"ok": false, "error": "标题不能为空~"})
		return
	}
	if req.PricePts < 0 || req.PricePts > maxPricePts {
		jsonResp(w, M{"ok": false, "error": "价格超出范围~"})
		return
	}
	tags := make([]string, 0, len(req.Tags))
	for _, t := range req.Tags {
		t = sanitizeText(strings.TrimSpace(t), maxTagLen)
		if t != "" && len(tags) < maxTags {
			tags = append(tags, t)
		}
	}
	req.Tags = tags
	req.Description = sanitizeText(req.Description, maxDescLen)
	if req.Status != nil && (*req.Status != 0 && *req.Status != 1) {
		jsonResp(w, M{"ok": false, "error": "状态参数错误~"})
		return
	}
	if err := db.UpdateMarketFileOwner(id, user.ID, &req); err != nil {
		log.Printf("[MARKET] 上传者编辑失败 file=%d by=%s err=%v", id, user.Username, err)
		jsonResp(w, M{"ok": false, "error": err.Error()})
		return
	}
	log.Printf("[MARKET] 上传者编辑 file=%d by=%s cover=%d", id, user.Username, coverReqVal(&req))
	jsonResp(w, M{"ok": true})
}

// ────────────────────────────────────────────────────────────
// 我的文件(上传者管理)
// ────────────────────────────────────────────────────────────

func handleMarketMyFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "请先登录~")})
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	list, total, err := db.ListMarketFilesByUploader(user.ID, page, size)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true, "total": total, "list": list})
}

// handleMarketFileOwnerDelete 上传者删除自己上传的文件(先删 DB 级联,再删磁盘)。
func handleMarketFileOwnerDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" && r.Method != "DELETE" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "请先登录~")})
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var owner int64
	if err := db.DB.QueryRow(`SELECT uploader_id FROM market_files WHERE id = ?`, id).Scan(&owner); err != nil {
		jsonResp(w, M{"ok": false, "error": "文件不存在~"})
		return
	}
	if owner != user.ID {
		jsonResp(w, M{"ok": false, "error": "无权删除该文件~"})
		return
	}
	dir, err := db.DeleteMarketFile(id)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	if dir != "" {
		os.RemoveAll(filepath.Clean(dir))
	}
	log.Printf("[MARKET] 上传者删除 file=%d by=%s", id, user.Username)
	jsonResp(w, M{"ok": true})
}

// ────────────────────────────────────────────────────────────
// 管理接口（admin）
// ────────────────────────────────────────────────────────────

func handleAdminMarketFiles(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	q := r.URL.Query().Get("q")
	list, total, err := db.ListAdminMarketFiles(page, size, q)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true, "total": total, "list": list})
}

func handleAdminMarketFlagged(w http.ResponseWriter, r *http.Request) {
	list, _, err := db.MarketReportedFiles()
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true, "list": list})
}

func handleAdminMarketFileStatus(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var req struct {
		Status int `json:"status"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := db.SetMarketFileStatus(id, req.Status); err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true})
}

func handleAdminMarketFileUnflag(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	// 处理完:清标记 + 重新上架
	if err := db.ClearMarketFileFlag(id); err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	_ = db.SetMarketFileStatus(id, 1)
	jsonResp(w, M{"ok": true})
}

func handleAdminMarketFileDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	dir, err := db.DeleteMarketFile(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			jsonResp(w, M{"ok": false, "error": "文件不存在或已删除"})
			return
		}
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	// 先删 DB 再删磁盘
	if dir != "" {
		os.RemoveAll(filepath.Clean(dir))
	}
	log.Printf("[MARKET] 管理员删除 file=%d", id)
	jsonResp(w, M{"ok": true})
}

func handleAdminMarketConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		c, _ := db.GetMarketConfig()
		jsonResp(w, M{"ok": true, "config": c})
		return
	}
	if r.Method == "POST" {
		var req db.MarketConfig
		json.NewDecoder(r.Body).Decode(&req)
		if err := db.UpdateMarketConfig(&req); err != nil {
			jsonResp(w, M{"ok": false, "error": sysErr(err)})
			return
		}
		jsonResp(w, M{"ok": true})
		return
	}
	jsonResp(w, M{"ok": false, "error": "请求方式不对~"})
}

// ── 分类管理 ──
func handleAdminMarketCategories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		cats, err := db.ListMarketCategories()
		if err != nil {
			jsonResp(w, M{"ok": false, "error": sysErr(err)})
			return
		}
		jsonResp(w, M{"ok": true, "categories": cats})
	case "POST": // 新增
		var req struct {
			Name string `json:"name"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if err := db.CreateMarketCategory(req.Name); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		jsonResp(w, M{"ok": true})
	case "PUT": // 改名
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		var req struct {
			Name string `json:"name"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if err := db.RenameMarketCategory(id, req.Name); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		jsonResp(w, M{"ok": true})
	case "DELETE": // 删除
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err := db.DeleteMarketCategory(id); err != nil {
			jsonResp(w, M{"ok": false, "error": err.Error()})
			return
		}
		jsonResp(w, M{"ok": true})
	default:
		jsonResp(w, M{"ok": false, "error": "请求方式不对~"})
	}
}

// ── 编辑文件详情 ──
func handleAdminMarketFileUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "请求方式不对~")})
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var req struct {
		PricePts   int      `json:"price_pts"`
		AllowAnon  bool     `json:"allow_anonymous"`
		Tags       []string `json:"tags"`
		Description string   `json:"description"`
		Categories []string `json:"categories"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.PricePts < 0 || req.PricePts > maxPricePts {
		jsonResp(w, M{"ok": false, "error": "价格超出范围~"})
		return
	}
	// 净化
	tags := make([]string, 0, len(req.Tags))
	for _, t := range req.Tags {
		t = sanitizeText(strings.TrimSpace(t), maxTagLen)
		if t != "" && len(tags) < maxTags {
			tags = append(tags, t)
		}
	}
	desc := sanitizeText(req.Description, maxDescLen)
	if err := db.UpdateMarketFileDetail(id, req.PricePts, req.AllowAnon, tags, []string{desc}, req.Categories); err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	log.Printf("[MARKET] 管理员编辑 file=%d", id)
	jsonResp(w, M{"ok": true})
}
