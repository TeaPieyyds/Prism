package pluginscan

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

const (
	maxZipEntries  = 200
	maxZipFileSize = int64(10 << 20)
	maxZipTotal    = int64(20 << 20)
)

// PluginMeta describes a parsed plugin zip package.
type PluginMeta struct {
	ID       string
	Name     string
	Version  string
	Type     string
	MainLua  string
	Docs     string
	Config   string
	ZipHash  string
	NormHash string
	MD5      string
	FileSize int64
}

// manifest is the required manifest.json shape.
type manifest struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
}

// ParsePluginZip unpacks a plugin zip bytes, validates it, and returns its meta.
func ParsePluginZip(data []byte) (*PluginMeta, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	byBase := make(map[string][]byte)
	var total int64
	for i, f := range zr.File {
		if i >= maxZipEntries {
			return nil, fmt.Errorf("zip has too many entries (>%d)", maxZipEntries)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if !zipSlipSafe(f.Name) {
			return nil, fmt.Errorf("unsafe entry name %q", f.Name)
		}
		if f.UncompressedSize64 > uint64(maxZipFileSize) {
			return nil, fmt.Errorf("entry %q exceeds max file size", f.Name)
		}
		if total+int64(f.UncompressedSize64) > maxZipTotal {
			return nil, fmt.Errorf("zip exceeds max total size")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open entry %q: %w", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read entry %q: %w", f.Name, err)
		}
		total += int64(len(content))
		byBase[filepath.Base(f.Name)] = content
	}

	mraw, ok := byBase["manifest.json"]
	if !ok {
		return nil, fmt.Errorf("manifest.json not found in zip")
	}
	var m manifest
	if err := json.Unmarshal(mraw, &m); err != nil {
		return nil, fmt.Errorf("parse manifest.json: %w", err)
	}
	if !validPluginID(m.ID) {
		return nil, fmt.Errorf("invalid plugin id %q", m.ID)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("manifest name is empty")
	}
	if m.Version == "" {
		return nil, fmt.Errorf("manifest version is empty")
	}
	if m.Type == "" {
		m.Type = "lua"
	}

	mainLua, ok := byBase["main.lua"]
	if !ok {
		return nil, fmt.Errorf("main.lua not found in zip")
	}
	mainStr := strings.TrimSpace(string(mainLua))
	if mainStr == "" {
		return nil, fmt.Errorf("main.lua is empty")
	}

	meta := &PluginMeta{
		ID:       m.ID,
		Name:     m.Name,
		Version:  m.Version,
		Type:     m.Type,
		MainLua:  mainStr,
		Docs:     string(byBase["docs.md"]),
		Config:   string(byBase["config.json"]),
		ZipHash:  ZipHash(data),
		NormHash: NormHash(mainStr),
		MD5:      md5Hex(data),
		FileSize: int64(len(data)),
	}
	return meta, nil
}

// zipSlipSafe reports whether an entry name does not escape its archive root.
func zipSlipSafe(name string) bool {
	if strings.HasPrefix(name, "/") {
		return false
	}
	clean := filepath.Clean(name)
	return !strings.Contains(clean, "..")
}

// validPluginID reports whether id is a safe plugin identifier.
func validPluginID(id string) bool {
	if id == "" || id == "." || id == ".." {
		return false
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return false
		}
		if strings.ContainsRune("/\\ ?#%=&", r) {
			return false
		}
	}
	return true
}

func md5Hex(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}
