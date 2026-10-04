package building

import (
	"encoding/json"
	"strings"
)

// NormalizeBlockEntityNBT 接收原始 NBT（可能来自 Java Schematic 或 Bedrock MCWorld），
// 根据 blockName 和 nbt["id"] 识别方块实体类型，返回 Bedrock 兼容的 NBT。
func NormalizeBlockEntityNBT(blockName string, nbtData map[string]any) map[string]any {
	out := make(map[string]any, len(nbtData)+2)

	// 剥离 id/x/y/z，这些由调用者设置
	for k, v := range nbtData {
		if k == "id" || k == "x" || k == "y" || k == "z" {
			continue
		}
		out[k] = cleanNBTValue(v)
	}

	id, _ := nbtData["id"].(string)
	bn := strings.ToLower(blockName)
	idLower := strings.ToLower(id)

	switch {
	case strings.Contains(bn, "sign") || strings.Contains(idLower, "sign"):
		normalizeSignNBT(out)
	case strings.Contains(bn, "beacon") || strings.Contains(idLower, "beacon"):
		normalizeBeaconNBT(out)
	case strings.Contains(bn, "lectern") || strings.Contains(idLower, "lectern"):
		normalizeLecternNBT(out)
	case strings.Contains(bn, "flower_pot") || strings.Contains(idLower, "flowerpot"):
		normalizeFlowerPotNBT(out)
	case strings.Contains(bn, "noteblock") || strings.Contains(bn, "note_block") || strings.Contains(idLower, "music"):
		normalizeNoteBlockNBT(out)
	case strings.Contains(bn, "beehive") || strings.Contains(bn, "bee_nest") || strings.Contains(idLower, "beehive") || strings.Contains(idLower, "beenest"):
		normalizeBeehiveNBT(out)
	}

	return out
}

// ═══ 告示牌 ═══

func normalizeSignNBT(nbt map[string]any) {
	// If FrontText exists, validate it has a real Text field
	if ft, ok := nbt["FrontText"].(map[string]any); ok {
		if txt, _ := ft["Text"].(string); txt != "" {
			ft["TextIgnoreDeletion"] = byte(1)
			nbt["FrontText"] = ft
			return
		}
		// FrontText exists but Text is empty — rebuild
	}
	if text, ok := nbt["Text"].(string); ok && text != "" {
		nbt["FrontText"] = map[string]any{"Text": text, "TextIgnoreDeletion": byte(1)}
		delete(nbt, "Text")
		delete(nbt, "Color")
		delete(nbt, "GlowingText")
		return
	}
	lines := make([]string, 0, 4)
	styles := make([]map[string]any, 0, 4)
	for _, k := range []string{"Text1", "Text2", "Text3", "Text4"} {
		if s, ok := nbt[k].(string); ok && s != "" {
			parsedText, parsedStyle := parseSignTextFull(s)
			if parsedText != "" {
				lines = append(lines, parsedText)
				styles = append(styles, parsedStyle)
			}
		}
		delete(nbt, k)
	}
	if len(lines) > 0 {
		ft := map[string]any{
			"Text":               buildSignTextJSON(strings.Join(lines, "\n")),
			"TextIgnoreDeletion": byte(1),
		}
		for _, s := range styles {
			for k, v := range s {
				ft[k] = v
			}
		}
		nbt["FrontText"] = ft
	}
	delete(nbt, "Color")
	delete(nbt, "GlowingText")
}

func buildSignTextJSON(plain string) string {
	escaped := strings.ReplaceAll(plain, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	return `{"rawtext":[{"text":"` + escaped + `"}]}`
}

// parseSignTextFull parses a Java Edition sign text JSON and returns the plain text
// plus Bedrock-compatible style properties (color, glowing).
func parseSignTextFull(raw string) (string, map[string]any) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return raw, map[string]any{}
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw, map[string]any{}
	}

	style := map[string]any{}

	// Extract color
	if c, ok := v["color"].(string); ok && c != "" {
		style["Color"] = c
	}
	// Extract glowing/bold/italic (stored as strings "true"/"false" in Java)
	for _, sk := range []string{"bold", "italic", "underlined", "strikethrough", "obfuscated"} {
		if sv, ok := v[sk]; ok {
			switch sv := sv.(type) {
			case string:
				if sv == "true" || sv == "1" {
					style[sk] = byte(1)
				}
			case bool:
				if sv {
					style[sk] = byte(1)
				}
			}
		}
	}

	rawtext, _ := v["rawtext"].([]any)
	if rawtext == nil {
		rawtext, _ = v["extra"].([]any)
	}
	if rawtext == nil {
		if t, ok := v["text"].(string); ok {
			return t, style
		}
		return raw, style
	}
	var result strings.Builder
	for _, item := range rawtext {
		if m, ok := item.(map[string]any); ok {
			// If this rawtext element has a color, use it
			if c, ok := m["color"].(string); ok && c != "" {
				style["Color"] = c
			}
			if t, ok := m["text"].(string); ok {
				result.WriteString(t)
			}
		}
	}
	return result.String(), style
}

// ═══ 信标 ═══

func normalizeBeaconNBT(nbt map[string]any) {
	// Java 版大写键 → Bedrock 小写键
	if v, ok := nbt["Primary"]; ok {
		nbt["primary"] = toInt(v)
		delete(nbt, "Primary")
	}
	if v, ok := nbt["Secondary"]; ok {
		nbt["secondary"] = toInt(v)
		delete(nbt, "Secondary")
	}
	delete(nbt, "Levels")
}

func toInt(v any) int32 {
	switch val := v.(type) {
	case int32:
		return val
	case int16:
		return int32(val)
	case int:
		return int32(val)
	case int64:
		return int32(val)
	case byte:
		return int32(val)
	case float64:
		return int32(val)
	}
	return 0
}

// ═══ 讲台 ═══

func normalizeLecternNBT(nbt map[string]any) {
	// 已有 Bedrock 格式
	if _, ok := nbt["hasBook"]; ok {
		return
	}
	// Java Book → Bedrock book
	if book, ok := nbt["Book"].(map[string]any); ok {
		nbt["book"] = cleanNBTValue(book)
		nbt["hasBook"] = byte(1)
		delete(nbt, "Book")
	}
	// Java Page → Bedrock page
	if page, ok := nbt["Page"]; ok {
		nbt["page"] = toInt(page)
		delete(nbt, "Page")
	}
}

// ═══ 花盆 ═══

var javaFlowerToBedrock = map[string]string{
	"oak_sapling":       "minecraft:sapling",
	"spruce_sapling":    "minecraft:spruce_sapling",
	"birch_sapling":     "minecraft:birch_sapling",
	"jungle_sapling":    "minecraft:jungle_sapling",
	"acacia_sapling":    "minecraft:acacia_sapling",
	"dark_oak_sapling":  "minecraft:dark_oak_sapling",
	"dandelion":         "minecraft:yellow_flower",
	"poppy":             "minecraft:red_flower",
	"blue_orchid":       "minecraft:red_flower",
	"allium":            "minecraft:red_flower",
	"azure_bluet":       "minecraft:red_flower",
	"red_tulip":         "minecraft:red_flower",
	"orange_tulip":      "minecraft:red_flower",
	"white_tulip":       "minecraft:red_flower",
	"pink_tulip":        "minecraft:red_flower",
	"oxeye_daisy":       "minecraft:red_flower",
	"cornflower":        "minecraft:red_flower",
	"lily_of_the_valley": "minecraft:red_flower",
	"wither_rose":       "minecraft:red_flower",
	"sunflower":         "minecraft:double_plant",
	"lilac":             "minecraft:double_plant",
	"rose_bush":         "minecraft:double_plant",
	"peony":             "minecraft:double_plant",
	"tall_grass":        "minecraft:tallgrass",
	"large_fern":        "minecraft:double_plant",
	"dead_bush":         "minecraft:deadbush",
	"fern":              "minecraft:tallgrass",
	"cactus":            "minecraft:cactus",
	"bamboo":            "minecraft:bamboo",
	"brown_mushroom":    "minecraft:brown_mushroom",
	"red_mushroom":      "minecraft:red_mushroom",
	"crimson_fungus":    "minecraft:crimson_fungus",
	"warped_fungus":     "minecraft:warped_fungus",
	"crimson_roots":     "minecraft:crimson_roots",
	"warped_roots":      "minecraft:warped_roots",
	"azalea":            "minecraft:azalea",
	"flowering_azalea":  "minecraft:flowering_azalea",
}

func normalizeFlowerPotNBT(nbt map[string]any) {
	// 已有 Bedrock PlantBlock
	if _, ok := nbt["PlantBlock"]; ok {
		return
	}
	// Java Item + Data → Bedrock PlantBlock
	item, _ := nbt["Item"].(string)
	if item == "" {
		return
	}
	// 剥离 minecraft: 前缀
	short := strings.TrimPrefix(item, "minecraft:")
	bedrockName, ok := javaFlowerToBedrock[short]
	if !ok {
		bedrockName = item
		if !strings.HasPrefix(bedrockName, "minecraft:") {
			bedrockName = "minecraft:" + short
		}
	}
	nbt["PlantBlock"] = map[string]any{
		"name": bedrockName,
	}
	delete(nbt, "Item")
	delete(nbt, "Data")
}

// ═══ 音符盒 ═══

func normalizeNoteBlockNBT(nbt map[string]any) {
	// note 键名相同，只需确保类型正确
	if v, ok := nbt["note"]; ok {
		nbt["note"] = int8(toInt(v))
	}
}

// ═══ 蜂巢/蜂箱 ═══

func normalizeBeehiveNBT(nbt map[string]any) {
	// 无论是否有蜜蜂数据，全部清除 → 装饰性蜂巢，无蜜蜂无蜂蜜
	delete(nbt, "bees")
	delete(nbt, "Bees")
	delete(nbt, "Occupants")
	delete(nbt, "honey_level")
	delete(nbt, "HoneyLevel")
	delete(nbt, "max_occupants")
	delete(nbt, "should_refill_flowers")
}

// ═══ 类型清理 ═══

func cleanNBTValue(v any) any {
	switch val := v.(type) {
	case uint8:
		return val // NBT 编码器只认 uint8(byte) 作为 TAG_Byte，不认 int8
	case uint16:
		return int16(val)
	case uint32:
		return int32(val)
	case uint64:
		return int64(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = cleanNBTValue(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[k] = cleanNBTValue(item)
		}
		return out
	default:
		return v
	}
}
