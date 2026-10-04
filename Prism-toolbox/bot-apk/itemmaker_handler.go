package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/nbt"
)

// ItemMakerItem 物品定义
type ItemMakerItem struct {
	Name string         `json:"名称"`
	ID   string         `json:"ID"`
	Data int            `json:"特殊值"`
	Tag  map[string]any `json:"标签属性"`
}

// handleItemMakerGenerate 生成包含木桶+物品的 .mcstructure 文件
// POST /api/itemmaker/generate
func handleItemMakerGenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []ItemMakerItem `json:"items"`
		X     int             `json:"x,omitempty"`
		Y     int             `json:"y,omitempty"`
		Z     int             `json:"z,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "无效JSON: " + err.Error()})
		return
	}
	if len(req.Items) == 0 {
		writeJSON(w, map[string]any{"ok": false, "error": "物品列表为空"})
		return
	}
	if len(req.Items) > 27 {
		req.Items = req.Items[:27]
	}

	// 构建 barrel 的 block_entity_data (Items 列表)
	itemsList := make([]any, 0, len(req.Items))
	for i, item := range req.Items {
		if item.ID == "" {
			continue
		}

		itemCompound := map[string]any{
			"Slot":   byte(i),
			"Name":   item.ID,
			"Count":  byte(1),
			"Damage": int16(item.Data),
		}

		// 构建 tag（display.Name 是纯字符串，不是 JSON 文本组件）
		// 参考房主神器盒.mcstructure 的格式
		if item.Name != "" || item.Tag != nil {
			tag := make(map[string]any)
			if item.Tag != nil {
				for k, v := range item.Tag {
					// 递归转换 float64 → int32（json.Unmarshal 把数字解成 float64）
					tag[k] = fixJSONNumbers(v)
				}
			}
			if item.Name != "" {
				display, _ := tag["display"].(map[string]any)
				if display == nil {
					display = make(map[string]any)
					tag["display"] = display
				}
				// display.Name 是纯字符串，不是 {"text":"..."}
				display["Name"] = item.Name
			}
			itemCompound["tag"] = tag
		}

		itemsList = append(itemsList, itemCompound)
	}

	blockEntityData := map[string]any{
		"id":    "Barrel",
		"Items": itemsList,
	}

	blockPositionData := map[string]any{
		"0": map[string]any{
			"block_entity_data": blockEntityData,
		},
	}

	// 构建 palette
	blockPalette := []any{
		map[string]any{"name": "minecraft:air", "states": map[string]any{}},
		map[string]any{
			"name": "minecraft:barrel",
			"states": map[string]any{
				"facing_direction": int32(2), // 2=north
				"open_bit":         false,
			},
		},
	}

	// 构建完整 structure
	structure := map[string]any{
		"block_indices": []any{
			[]int32{1},  // layer0: palette index 1 = barrel
			[]int32{-1}, // layer1: empty
		},
		"palette": map[string]any{
			"default": map[string]any{
				"block_palette":      blockPalette,
				"block_position_data": blockPositionData,
			},
		},
		"entities": []any{},
	}

	// 编码为 NBT
	root := map[string]any{
		"format_version": int32(1),
		"size":           []int32{1, 1, 1},
		"structure":      structure,
	}

	var buf bytes.Buffer
	enc := nbt.NewEncoderWithEncoding(&buf, nbt.LittleEndian)
	if err := enc.Encode(root); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "编码失败: " + err.Error()})
		return
	}

	// 写入临时文件
	outputDir := "/data/data/com.prismtool.box/files/itemmaker"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		outputDir = "/tmp/itemmaker"
		os.MkdirAll(outputDir, 0755)
	}
	outputPath := filepath.Join(outputDir, "itemmaker_barrel.mcstructure")
	if err := os.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "写入文件失败: " + err.Error()})
		return
	}

	writeJSON(w, map[string]any{
		"ok":   true,
		"path": outputPath,
		"size": len(req.Items),
	})
}

// fixJSONNumbers 递归遍历，把 float64 转成 int16。
// json.Unmarshal 把 JSON 数字全解成 float64，但 NBT 需要 int16（ench 的 id/lvl、Damage 等）。
// int16 范围 -32768~32767 足够覆盖所有附魔等级和 damage 值。
func fixJSONNumbers(v any) any {
	switch val := v.(type) {
	case float64:
		return int16(val)
	case map[string]any:
		for k, v2 := range val {
			val[k] = fixJSONNumbers(v2)
		}
		return val
	case []any:
		for i, v2 := range val {
			val[i] = fixJSONNumbers(v2)
		}
		return val
	default:
		return v
	}
}