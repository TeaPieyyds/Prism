package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type AppConfig struct {
	Token        string `json:"token"`
	AuthURL      string `json:"auth_url"`
	ServerCode   string `json:"server_code"`
	ServerPass   string `json:"server_pass"`
	BotName      string `json:"bot_name"`
	ImportSpeed  int    `json:"import_speed"`
	ExportSpeed  int    `json:"export_speed"`
	MediaDisplay string `json:"media_display"`
	MediaTarget  string `json:"media_target"`
	ShowProgress bool   `json:"show_progress"`
	DataDir      string `json:"data_dir"`

	ImportCommands     bool `json:"import_commands"`
	CmdDisabled        bool `json:"cmd_disabled"`
	ExcludeFluids      bool `json:"exclude_fluids"`
	ExcludeWater       bool `json:"exclude_water"`
	ExcludeWaterlogged bool `json:"exclude_waterlogged"`
	ExcludeLava        bool `json:"exclude_lava"`
	UseGravityBlocks   bool `json:"use_gravity_blocks"`
	GravityPlatform    bool `json:"gravity_platform"`

	UseNewProtocol bool   `json:"use_new_protocol"` // 1.21.120 新版协议
	RegionMode     string `json:"region_mode"`       // 导入粒度: 1=16×16, 2=32×32, 3=64×64

	PreClearMode int `json:"pre_clear_mode"` // 0=关闭 1=一格 2=竖柱 3=区块

	CustomDNS string `json:"custom_dns"` // 自定义 DNS 服务器，空=默认 223.5.5.5

	TLSSkipVerify *bool `json:"tls_skip_verify,omitempty"` // 跳过 TLS 证书验证，nil=默认 true

	// ── 多机器人（S8） ──
	BotTokens []string `json:"bot_tokens"` // 子机器人令牌（不包括主令牌），配置即启用多机器人
}

var cfgPath string
var current *AppConfig

func InitConfig(dataDir string) {
	os.MkdirAll(dataDir, 0755)
	cfgPath = filepath.Join(dataDir, "config.json")
}

func LoadConfig() *AppConfig {
	if current != nil {
		return current
	}
	current = &AppConfig{
		AuthURL:            "https://prism.adblanlu.qzz.io",
		ImportSpeed:        500,
		ExportSpeed:        100,
		MediaDisplay:       "actionbar",
		MediaTarget:        "@a",
		ShowProgress:       true,
		ImportCommands:     true,
		ExcludeWater:       true,
		ExcludeWaterlogged: true,
		ExcludeLava:        true,
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return current
	}
	json.Unmarshal(data, current)
	return current
}

// ReloadConfig re-reads config from disk, bypassing the cache.
func ReloadConfig() *AppConfig {
	if cfgPath == "" {
		return LoadConfig()
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return LoadConfig()
	}
	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return LoadConfig()
	}
	// Ensure defaults for zero fields
	if cfg.AuthURL == "" {
		cfg.AuthURL = "https://prism.adblanlu.qzz.io"
	}
	current = &cfg
	return current
}

func SaveConfig(cfg *AppConfig) error {
	current = cfg
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, data, 0644)
}
