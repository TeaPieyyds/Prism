package building

import (
	"math"
	"sort"
	"strings"
)

// StructureStats holds computed statistics for a structure
type StructureStats struct {
	TotalVolume     int            `json:"total_volume"`
	SolidBlocks     int            `json:"solid_blocks"`
	AirBlocks       int            `json:"air_blocks"`
	NBTBlocks       int            `json:"nbt_blocks"`
	CmdBlocks       int            `json:"cmd_blocks"`
	ContainerItems  int            `json:"container_items"`
	WaterBlocks     int            `json:"water_blocks"`
	BlockCounts     map[string]int `json:"block_counts"`
	BlockCountsList []BlockCount   `json:"block_counts_list"`
	SizeX           int            `json:"size_x"`
	SizeY           int            `json:"size_y"`
	SizeZ           int            `json:"size_z"`
}

type BlockCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// VoxelPoint represents a single voxel in the simplified 3D model
type VoxelPoint struct {
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Z     int    `json:"z"`
	Color string `json:"c"`
}

// NBTVoxelInfo holds NBT information for a specific block position in the 3D view
type NBTVoxelInfo struct {
	X       int            `json:"x"`
	Y       int            `json:"y"`
	Z       int            `json:"z"`
	Type    string         `json:"type"` // "command" or "container"
	Command string         `json:"command,omitempty"`
	Items   []NBTItemInfo  `json:"items,omitempty"`
	Extra   map[string]any `json:"extra,omitempty"`
}

// NBTItemInfo holds info about a single item in a container
type NBTItemInfo struct {
	Slot  int    `json:"slot"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// VoxelData holds the simplified 3D representation
type VoxelData struct {
	Points     []VoxelPoint   `json:"points"`
	NBTInfo    []NBTVoxelInfo `json:"nbt_info,omitempty"`
	SizeX      int            `json:"size_x"`
	SizeY      int            `json:"size_y"`
	SizeZ      int            `json:"size_z"`
	Step       int            `json:"step"`
	TooLarge   bool           `json:"too_large"`
	TotalSolid int            `json:"total_solid"`
}

const (
	maxVoxelPoints  = 80000  // target max points for 3D preview
	maxTotalVolume  = 1000000000 // 1B blocks volume threshold
)

// ComputeStats analyzes the structure and returns statistics
func (s *StructureData) ComputeStats() *StructureStats {
	stats := &StructureStats{
		TotalVolume: s.SizeX * s.SizeY * s.SizeZ,
		SizeX:       s.SizeX,
		SizeY:       s.SizeY,
		SizeZ:       s.SizeZ,
		BlockCounts: make(map[string]int),
	}

	for pos, idx := range s.Blocks {
		entry := s.Palette[idx]
		if IsAirBlock(entry.Name) {
			continue
		}
		stats.SolidBlocks++
		if s.NBTBlocks != nil {
			if nbt, ok := s.NBTBlocks[pos]; ok {
				stats.NBTBlocks++
				if cmd, ok := nbt["Command"].(string); ok && cmd != "" {
					stats.CmdBlocks++
				}
				if items, ok := nbt["Items"].([]any); ok {
					stats.ContainerItems += len(items)
				}
			}
		}
		if isWaterBlock(entry.Name) {
			stats.WaterBlocks++
		}
		stats.BlockCounts[entry.Name]++
	}

	// Convert map to sorted list
	stats.BlockCountsList = make([]BlockCount, 0, len(stats.BlockCounts))
	for name, count := range stats.BlockCounts {
		stats.BlockCountsList = append(stats.BlockCountsList, BlockCount{Name: name, Count: count})
	}

	// Air blocks = total volume - non-air blocks
	stats.AirBlocks = stats.TotalVolume - stats.SolidBlocks
	sort.Slice(stats.BlockCountsList, func(i, j int) bool {
		return stats.BlockCountsList[i].Count > stats.BlockCountsList[j].Count
	})

	return stats
}

// GenerateVoxelData creates a simplified 3D voxel representation of the structure
func (s *StructureData) GenerateVoxelData() *VoxelData {
	totalVolume := s.SizeX * s.SizeY * s.SizeZ

	if totalVolume > maxTotalVolume {
		return &VoxelData{
			TooLarge:   true,
			SizeX:      s.SizeX,
			SizeY:      s.SizeY,
			SizeZ:      s.SizeZ,
			TotalSolid: 0,
		}
	}

	// Count solid blocks from sparse map
	solidCount := len(s.Blocks)

	// Adaptive step size: for thin structures use step=1 (pixel art etc.)
	step := 1
	minDim := s.SizeX
	if s.SizeY < minDim {
		minDim = s.SizeY
	}
	if s.SizeZ < minDim {
		minDim = s.SizeZ
	}
	if minDim > 3 && solidCount > maxVoxelPoints {
		step = int(math.Ceil(math.Cbrt(float64(solidCount) / float64(maxVoxelPoints))))
		if step < 1 {
			step = 1
		}
		if step > 8 {
			step = 8
		}
	}

	// Send all solid blocks (with color). Frontend will filter to surface only for rendering.
	var points []VoxelPoint
	var nbtInfos []NBTVoxelInfo
	for pos, idx := range s.Blocks {
		entry := s.Palette[idx]
		if IsAirBlock(entry.Name) {
			continue
		}
		// 跳过不可见方块（屏障、光源块、结构空位等）
		if IsInvisibleBlock(entry.Name) {
			continue
		}
		x, y, z := unpackPos(pos)
		if int(x)%step != 0 || int(y)%step != 0 || int(z)%step != 0 {
			continue
		}
		points = append(points, VoxelPoint{X: int(x), Y: int(y), Z: int(z), Color: blockCategoryColor(entry.Name)})

		// Collect NBT voxel info
		if nbt, ok := s.NBTBlocks[pos]; ok && len(nbt) > 0 {
			info := extractNBTInfo(int(x), int(y), int(z), nbt, entry.Name)
			if info != nil {
				nbtInfos = append(nbtInfos, *info)
			}
		}
		// Also check legacy entry.NBT
		if entry.NBT != nil && len(entry.NBT) > 0 {
			info := extractNBTInfo(int(x), int(y), int(z), entry.NBT, entry.Name)
			if info != nil {
				nbtInfos = append(nbtInfos, *info)
			}
		}
	}

	return &VoxelData{
		Points:     points,
		NBTInfo:    nbtInfos,
		SizeX:      s.SizeX,
		SizeY:      s.SizeY,
		SizeZ:      s.SizeZ,
		Step:       step,
		TooLarge:   false,
		TotalSolid: solidCount,
	}
}

// extractNBTInfo 从方块 NBT 数据中提取前端 3D 预览需要展示的信息。
func extractNBTInfo(x, y, z int, nbtData map[string]any, blockName string) *NBTVoxelInfo {
	if nbtData == nil {
		return nil
	}

	// 检查是否是命令方块
	if cmd, ok := nbtData["Command"].(string); ok && cmd != "" {
		info := &NBTVoxelInfo{
			X:       x,
			Y:       y,
			Z:       z,
			Type:    "command",
			Command: cmd,
		}
		if name, ok := nbtData["CustomName"].(string); ok && name != "" {
			info.Extra = map[string]any{"name": name}
		}
		return info
	}

	// 检查是否是容器（箱子、漏斗、发射器、熔炉等）
	if itemsRaw, ok := nbtData["Items"].([]any); ok && len(itemsRaw) > 0 {
		info := &NBTVoxelInfo{
			X:    x,
			Y:    y,
			Z:    z,
			Type: "container",
		}
		for _, itemRaw := range itemsRaw {
			itemMap, ok := itemRaw.(map[string]any)
			if !ok {
				continue
			}
			item := NBTItemInfo{}
			item.Slot = int(toIntAnyV(itemMap["Slot"]))
			item.Name, _ = itemMap["Name"].(string)
			c := toIntAnyV(itemMap["Count"])
			if c == 0 { c = 1 }
			item.Count = int(c)
			if item.Name != "" {
				info.Items = append(info.Items, item)
			}
		}
		if len(info.Items) > 0 {
			return info
		}
	}

	// 检查是否是告示牌
	if text, ok := nbtData["Text"].(string); ok && text != "" {
		return &NBTVoxelInfo{
			X:    x,
			Y:    y,
			Z:    z,
			Type: "sign",
			Extra: map[string]any{
				"text": text,
			},
		}
	}

	return nil
}

var blockColorMap = map[string]string{
	"white": "#f0f0f0", "orange": "#f09030", "magenta": "#d060d0", "light_blue": "#80b0f0",
	"yellow": "#f0f040", "lime": "#60d020", "pink": "#f0a0b0", "gray": "#606060",
	"light_gray": "#a0a0a0", "silver": "#a0a0a0", "cyan": "#40a0a0", "purple": "#9020d0",
	"blue": "#3030f0", "brown": "#805030", "green": "#408020", "red": "#d02020", "black": "#202020",
}

func blockCategoryColor(name string) string {
	n := strings.ToLower(name)

	// 优先从纹理颜色表查找（更精确）
	if c, ok := blockTextureColor(n); ok {
		return c
	}

	// Try exact color match for colored blocks
	for color, hex := range blockColorMap {
		if strings.Contains(n, "_"+color) {
			return hex
		}
	}
	// Common block colors
	switch {
	case contains(n, "stone", "cobblestone", "bedrock", "gravel", "andesite", "diorite", "granite", "tuff", "deepslate", "basalt", "blackstone"):
		return "#7f7f7f"
	case contains(n, "brick", "bricks", "nether_brick"):
		return "#8b4513"
	case contains(n, "prismarine", "dark_prismarine"):
		return "#508060"
	case contains(n, "obsidian", "crying_obsidian"):
		return "#151018"
	case contains(n, "quartz", "calcite", "diorite", "smooth_stone", "iron_block"):
		return "#d0d0c8"
	case contains(n, "sandstone", "sand", "end_stone", "bone"):
		return "#e0d8b0"
	case contains(n, "purpur"):
		return "#c090c0"
	case contains(n, "log", "planks", "wood", "stem", "hyphae", "bamboo_block", "crafting_table", "bookshelf", "chest"):
		return "#b08050"
	case contains(n, "leaves", "wart_block", "shroomlight", "vine", "moss"):
		return "#408030"
	case contains(n, "water", "ice", "snow", "packed_ice", "blue_ice"):
		return "#6090d0"
	case contains(n, "glass", "ice"):
		return "#c0d8d8"
	case contains(n, "wool", "carpet"):
		return "#e8e8e0"
	case contains(n, "dirt", "mud", "farmland", "grass_path", "podzol", "mycelium", "clay"):
		return "#8b6914"
	case contains(n, "grass_block", "grass "):
		return "#6b8c42"
	case contains(n, "ore", "gold", "diamond", "emerald", "lapis", "redstone", "copper", "nether_quartz_ore"):
		return "#c0a878"
	case contains(n, "gold_block", "diamond_block", "emerald_block", "netherite", "anvil", "chain", "lantern", "iron_bars", "iron_door", "iron_trapdoor"):
		return "#b8b8b0"
	case contains(n, "coal", "coal_block"):
		return "#282828"
	case contains(n, "emerald", "emerald_block"):
		return "#30d030"
	case contains(n, "diamond", "diamond_block"):
		return "#40d0d0"
	case contains(n, "gold_ore", "gold_block", "gilded"):
		return "#f0d030"
	case contains(n, "flower", "sapling", "mushroom", "wheat", "carrot", "potato", "beetroot", "sugar_cane", "cactus", "bamboo", "kelp", "fern", "lily", "grass", "tallgrass", "deadbush", "dandelion", "poppy", "allium", "rose", "tulip", "sunflower", "lilac"):
		return "#40b020"
	case contains(n, "netherrack", "soul", "magma", "crimson", "warped", "nylium", "nether_wart"):
		return "#804040"
	case contains(n, "slime", "slime_block"):
		return "#60d040"
	case contains(n, "honey", "honeycomb"):
		return "#e0a020"
	case contains(n, "pumpkin", "jack_o_lantern", "glowstone", "shroomlight", "sea_lantern"):
		return "#e0c040"
	case contains(n, "glazed_terracotta"):
		return "#c0a080"
	case contains(n, "terracotta", "hardened_clay", "stained_hardened_clay"):
		return "#b06030"
	case contains(n, "concrete_powder"):
		return "#d8d0c0"
	case contains(n, "concrete"):
		return "#c8c0b8"
	case contains(n, "melon", "pumpkin"):
		return "#80b020"
	case contains(n, "torch", "lantern"):
		return "#f0c040"
	case contains(n, "sponge"):
		return "#c0c030"
	case contains(n, "web"):
		return "#e8e8e8"
	case contains(n, "tnt"):
		return "#d03020"
	case contains(n, "dispenser", "dropper", "furnace", "observer", "piston", "sticky_piston"):
		return "#787878"
	case contains(n, "noteblock", "jukebox"):
		return "#805030"
	case contains(n, "rail", "golden_rail", "detector_rail", "activator_rail"):
		return "#886040"
	default:
		return "#e94560"
	}
}

func categorizeBlock(name string) string {
	n := strings.ToLower(name)
	switch {
	case contains(n, "stone", "cobblestone", "bedrock", "andesite", "diorite", "granite", "brick", "prismarine", "obsidian", "quartz", "sandstone", "end_stone", "purpur", "nether_brick", "basalt", "blackstone", "deepslate", "tuff", "calcite"):
		return "stone"
	case contains(n, "log", "planks", "wood", "stem", "hyphae"):
		return "wood"
	case contains(n, "leaves", "wart_block", "shroomlight", "vine"):
		return "leaf"
	case contains(n, "water", "ice", "snow"):
		return "water"
	case contains(n, "sand", "gravel"):
		return "sand"
	case contains(n, "glass", "ice"):
		return "glass"
	case contains(n, "wool", "carpet"):
		return "wool"
	case contains(n, "dirt", "mud", "grass_block", "farmland", "grass_path", "podzol", "mycelium", "clay", "moss"):
		return "dirt"
	case contains(n, "ore", "coal", "iron", "gold", "diamond", "emerald", "lapis", "redstone", "copper"):
		return "ore"
	case contains(n, "iron", "gold_block", "diamond_block", "emerald_block", "netherite", "anvil", "chain", "lantern"):
		return "metal"
	case contains(n, "grass", "flower", "sapling", "mushroom", "wheat", "carrot", "potato", "beetroot", "sugar_cane", "cactus", "bamboo", "kelp", "fern", "lily"):
		return "plant"
	case contains(n, "netherrack", "soul", "magma", "basalt", "crimson", "warped", "nylium"):
		return "nether"
	case contains(n, "concrete"):
		return "concrete"
	case contains(n, "terracotta", "glazed"):
		return "terracotta"
	default:
		return "other"
	}
}

func contains(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func isWaterBlock(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "water") || strings.Contains(n, "lava") || strings.Contains(n, "flowing")
}

func IsAirBlock(name string) bool {
	n := strings.ToLower(name)
	return n == "air" || n == "minecraft:air" || strings.HasSuffix(n, ":air")
}

// IsInvisibleBlock 检测方块是否为不可见方块（预览渲染时跳过）
func IsInvisibleBlock(name string) bool {
	n := strings.ToLower(name)
	return n == "minecraft:barrier" || n == "barrier" ||
		n == "minecraft:light_block" || n == "light_block" ||
		n == "minecraft:structure_void" || n == "structure_void" ||
		strings.Contains(n, "deny") || strings.Contains(n, "allow") ||
		strings.Contains(n, "border_block") || n == "minecraft:invisiblebedrock" || n == "invisiblebedrock"
}

func toIntAnyV(v any) int32 {
	switch val := v.(type) {
	case int32: return val
	case int16: return int32(val)
	case byte: return int32(val)
	case float64: return int32(val)
	case int: return int32(val)
	}
	return 0
}
