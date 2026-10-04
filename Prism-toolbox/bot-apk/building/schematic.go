package building

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/nbt"
)

//go:embed block_ids.json
var blockIDsJSON []byte

// bedockBlockMap: legacy ID -> data value -> bedrock block name
var bedrockBlockMap map[string]map[string]string

func init() {
	bedrockBlockMap = make(map[string]map[string]string)
	json.Unmarshal(blockIDsJSON, &bedrockBlockMap)
}

// ParseSchematic parses a Java Edition .schematic or .schem file.
// Java schematics are gzip-compressed BigEndian NBT.
func ParseSchematic(data []byte) (*StructureData, error) {
	// Detect gzip compression (magic: 1f 8b)
	var reader io.Reader = bytes.NewBuffer(data)
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gz, err := gzip.NewReader(bytes.NewBuffer(data))
		if err != nil {
			return nil, fmt.Errorf("ParseSchematic: gzip decompress: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	// Java Edition uses BigEndian NBT
	decoder := nbt.NewDecoderWithEncoding(reader, nbt.BigEndian)

	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("ParseSchematic: decode NBT: %w", err)
	}

	// Detect format version
	if version, ok := root["Version"].(int32); ok && version == 2 {
		return parseSchematicV2(root)
	}
	return parseSchematicLegacy(root)
}

const maxSchemVolume = 200_000_000 // 1亿体素上限

func checkVolume(sx, sy, sz int) error {
	if sx*sy*sz > maxSchemVolume {
		return fmt.Errorf("建筑过大 (%d×%d×%d = %.0fM 体素，上限200M)", sx, sy, sz, float64(sx*sy*sz)/1e6)
	}
	return nil
}

func parseSchematicV2(root map[string]any) (*StructureData, error) {
	width := int(toSchematicInt(root["Width"]))
	height := int(toSchematicInt(root["Height"]))
	length := int(toSchematicInt(root["Length"]))

	// Sponge v2 使用 BlockData（VarInt 编码的调色板索引数组）
	rawBlockData := getByteSlice(root["BlockData"])
	if rawBlockData == nil {
		rawBlockData = getByteSlice(root["Data"])
	}
	if rawBlockData == nil {
		return nil, fmt.Errorf("parseSchematicV2: missing BlockData/Data, types: BlockData=%T Data=%T", root["BlockData"], root["Data"])
	}

	// Sponge v2 格式的 BlockData 是 VarInt 编码的，不是逐字节索引
	blockIndices, err := decodeVarInts(rawBlockData)
	if err != nil {
		return nil, fmt.Errorf("parseSchematicV2: decode BlockData: %w", err)
	}
	expectedCount := width * height * length
	if len(blockIndices) != expectedCount {
		return nil, fmt.Errorf("parseSchematicV2: BlockData 解码后索引数 %d 与预期 %d (W%d×H%d×L%d) 不匹配",
			len(blockIndices), expectedCount, width, height, length)
	}

	paletteData, ok := root["Palette"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parseSchematicV2: missing Palette")
	}

	reversePalette := make([]string, len(paletteData))
	for name, idx := range paletteData {
		switch v := idx.(type) {
		case int32:
			reversePalette[v] = name
		case int:
			reversePalette[v] = name
		}
	}

	// Parse BlockEntities if present
	nbtMap := make(map[uint64]map[string]any)
	if beList, ok := root["BlockEntities"].([]any); ok {
		for _, beItem := range beList {
			bem, ok := beItem.(map[string]any)
			if !ok { continue }
			x := int32(toSchematicInt(bem["x"]))
			y := int32(toSchematicInt(bem["y"]))
			z := int32(toSchematicInt(bem["z"]))
			nbtMap[packPos(x, y, z)] = bem
		}
	}

	result := &StructureData{
		SizeX:  width,
		SizeY:  height,
		SizeZ:  length,
		Blocks: make(map[uint64]uint32),
	}

	index := 0
	for y := 0; y < height; y++ {
		for z := 0; z < length; z++ {
			for x := 0; x < width; x++ {
				paletteIdx := blockIndices[index]
				index++

				if paletteIdx >= 0 && paletteIdx < len(reversePalette) {
					name := convertJavaBlockName(reversePalette[paletteIdx])
					if name != "minecraft:air" {
						pos := packPos(int32(x), int32(y), int32(z))
						idx := result.getOrCreatePaletteIndex(name, "[]")
						result.Blocks[pos] = idx
						if nbt, ok := nbtMap[pos]; ok {
							result.ensureNBTBlocks()
							result.NBTBlocks[pos] = nbt
						}
					}
				}
			}
		}
	}
	return result, nil
}

// decodeVarInts 将 VarInt 编码的字节数组解码为 int 切片。
// Sponge v2 格式使用此编码存储调色板索引。
// VarInt 编码: 每个字节用低 7 位存数据，最高位是续传标志。
// 小端序（低 7 位在前），与 Minecraft 网络协议 VarInt 一致。
func decodeVarInts(data []byte) ([]int, error) {
	result := make([]int, 0, len(data))
	pos := 0
	for pos < len(data) {
		val := 0
		shift := 0
		for {
			if pos >= len(data) {
				return nil, fmt.Errorf("VarInt 解码: 数据在 %d/%d 处截断", pos, len(data))
			}
			b := data[pos]
			pos++
			val |= int(b&0x7F) << shift
			shift += 7
			if shift > 35 {
				return nil, fmt.Errorf("VarInt 解码: 超过 5 字节上限")
			}
			if (b & 0x80) == 0 {
				break
			}
		}
		result = append(result, val)
	}
	return result, nil
}

func parseSchematicLegacy(root map[string]any) (*StructureData, error) {
	width := int(toSchematicInt(root["Width"]))
	height := int(toSchematicInt(root["Height"]))
	length := int(toSchematicInt(root["Length"]))
	if err := checkVolume(width, height, length); err != nil { return nil, err }

	blocks := getByteSlice(root["Blocks"])
	if blocks == nil {
		return nil, fmt.Errorf("parseSchematicLegacy: missing Blocks, type=%T", root["Blocks"])
	}
	dataValues := getByteSlice(root["Data"])

	// 优先使用 SchematicaMapping（Schematica 模组的精确 ID→名称映射）
	idToName := map[int]string{}
	if mapping, ok := root["SchematicaMapping"].(map[string]any); ok {
		for name, idVal := range mapping {
			switch v := idVal.(type) {
			case int32: idToName[int(v)] = name
			case int16: idToName[int(v)] = name
			case float64: idToName[int(v)] = name
			}
		}
	}

	result := &StructureData{
		SizeX:  width,
		SizeY:  height,
		SizeZ:  length,
		Blocks: make(map[uint64]uint32),
	}

	index := 0
	for y := 0; y < height; y++ {
		for z := 0; z < length; z++ {
			for x := 0; x < width; x++ {
				if index >= len(blocks) { break }
				blockID := int(blocks[index])
				var dataVal int
				if dataValues != nil && len(dataValues) > index {
					dataVal = int(dataValues[index])
				}
				index++
				if blockID != 0 {
					var name, states string
					if n, ok := idToName[blockID]; ok {
						name, states = splitNameStates(n)
					} else {
						name, states = lookupBedrockBlock(blockID, dataVal)
					}
					if name != "" && name != "air" && name != "minecraft:air" {
						if !strings.HasPrefix(name, "minecraft:") {
							name = "minecraft:" + name
						}
						idx := result.getOrCreatePaletteIndex(name, states)
						result.Blocks[packPos(int32(x), int32(y), int32(z))] = idx
					}
				}
			}
		}
	}
	return result, nil
}

func lookupBedrockBlock(id, data int) (name, states string) {
	idStr := fmt.Sprintf("%d", id)
	dataStr := fmt.Sprintf("%d", data)
	if idMap, ok := bedrockBlockMap[idStr]; ok {
		if blockStr, ok := idMap[dataStr]; ok {
			return splitNameStates(blockStr)
		}
		if blockStr, ok := idMap["0"]; ok {
			return splitNameStates(blockStr)
		}
	}
	return fmt.Sprintf("unknown_%d", id), "[]"
}

func splitNameStates(raw string) (name, states string) {
	idx := strings.IndexByte(raw, '[')
	if idx < 0 { return raw, "[]" }
	return strings.TrimSpace(raw[:idx]), strings.TrimSpace(raw[idx:])
}

func getByteSlice(v any) []byte {
	if b, ok := v.([]byte); ok {
		return b
	}
	// Handle fixed-size array via reflection (NBT unmarshal produces arrays not slices)
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Array && rv.Type().Elem().Kind() == reflect.Uint8 {
		result := make([]byte, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = byte(rv.Index(i).Uint())
		}
		return result
	}
	return nil
}

func toSchematicInt(v any) int32 {
	switch val := v.(type) {
	case int16: return int32(val)
	case int32: return val
	case float64: return int32(val)
	}
	return 0
}

var javaToBedrock = map[string]string{
	"grass_block": "grass", "stone": "stone", "dirt": "dirt",
	"bricks": "brick_block", "nether_bricks": "nether_brick",
	"red_nether_bricks": "red_nether_brick",
	"slime_block": "slime", "snow_block": "snow",
	"terracotta": "hardened_clay",
	"light_gray_glazed_terracotta": "silver_glazed_terracotta",
	"nether_quartz_ore": "quartz_ore",
	"note_block": "noteblock",
	"oak_log": "log", "birch_log": "log", "spruce_log": "log",
	"jungle_log": "log", "acacia_log": "log2", "dark_oak_log": "log2",
	"oak_planks": "planks", "birch_planks": "planks", "spruce_planks": "planks",
	"jungle_planks": "planks", "acacia_planks": "planks", "dark_oak_planks": "planks",
	"oak_leaves": "leaves", "birch_leaves": "leaves", "spruce_leaves": "leaves",
	"jungle_leaves": "leaves",
}

func convertJavaBlockName(name string) string {
	// Strip block states
	if idx := strings.IndexByte(name, '['); idx >= 0 {
		name = name[:idx]
	}
	// Strip minecraft: prefix if present
	short := strings.TrimPrefix(name, "minecraft:")
	if bedrock, ok := javaToBedrock[short]; ok {
		return "minecraft:" + bedrock
	}
	return name
}

