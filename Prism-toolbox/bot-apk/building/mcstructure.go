package building

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/nbt"
	"github.com/OmineDev/flowers-for-machines/utils"
)

// ParseMCStructure parses a .mcstructure file (little-endian NBT format).
func ParseMCStructure(data []byte) (*StructureData, error) {
	buf := bytes.NewBuffer(data)
	decoder := nbt.NewDecoderWithEncoding(buf, nbt.LittleEndian)

	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("ParseMCStructure: decode NBT: %w", err)
	}

	var sx, sy, sz int
	switch s := root["size"].(type) {
	case []any:
		if len(s) < 3 {
			return nil, fmt.Errorf("ParseMCStructure: size array too short")
		}
		sx, sy, sz = int(toInt32(s[0])), int(toInt32(s[1])), int(toInt32(s[2]))
	case []int32:
		if len(s) < 3 {
			return nil, fmt.Errorf("ParseMCStructure: size array too short")
		}
		sx, sy, sz = int(s[0]), int(s[1]), int(s[2])
	default:
		return nil, fmt.Errorf("ParseMCStructure: unexpected size type %T", root["size"])
	}

	if err := checkVolume(sx, sy, sz); err != nil {
		return nil, fmt.Errorf("ParseMCStructure: %w", err)
	}

	structure, ok := root["structure"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("ParseMCStructure: missing structure field")
	}

	// Parse palette — iterate all palette names (not just "default")
	paletteData, _ := structure["palette"].(map[string]any)
	var blockPaletteRaw []any
	blockPosData := make(map[string]any)
	for _, pv := range paletteData {
		pm, ok := pv.(map[string]any)
		if !ok { continue }
		if bp, ok := pm["block_palette"].([]any); ok && len(bp) > len(blockPaletteRaw) {
			blockPaletteRaw = bp
		}
		if bpd, ok := pm["block_position_data"].(map[string]any); ok {
			// 合并所有 palette 组件的 block_position_data，不遗漏命令方块 NBT
			for k, v := range bpd {
				blockPosData[k] = v
			}
		}
	}
	if len(blockPaletteRaw) == 0 {
		return nil, fmt.Errorf("ParseMCStructure: no block palette found (palette keys: %v)", paletteKeys(paletteData))
	}

	palette := make([]string, len(blockPaletteRaw))
	paletteNames := make([]string, len(blockPaletteRaw))
	paletteStates := make([]string, len(blockPaletteRaw))
	for i, bp := range blockPaletteRaw {
		bpMap, ok := bp.(map[string]any)
		if !ok {
			continue
		}
		name, _ := bpMap["name"].(string)
		states, _ := bpMap["states"].(map[string]any)
		ps := formatBlockStates(states)
		palette[i] = name + " " + ps
		paletteNames[i] = name
		paletteStates[i] = ps
	}

	// Parse block indices — supports both []any and bare []int32 layers
	blockIndices, _ := structure["block_indices"].([]any)
	if len(blockIndices) < 1 {
		return nil, fmt.Errorf("ParseMCStructure: no block_indices layers")
	}
	layer0 := toIntArray(blockIndices[0])
	var layer1 []int32
	if len(blockIndices) >= 2 {
		layer1 = toIntArray(blockIndices[1])
	} else {
		// Single-layer file: fill layer1 with -1 (air)
		layer1 = make([]int32, len(layer0))
		for i := range layer1 {
			layer1[i] = -1
		}
	}
	if layer0 == nil {
		return nil, fmt.Errorf("ParseMCStructure: failed to read block_indices layer 0 (type %T)", blockIndices[0])
	}

	// Extract block entity data
	blockEntityMap := make(map[int]map[string]any)
	for key, value := range blockPosData {
		valMap, ok := value.(map[string]any)
		if !ok {
			continue
		}
		entityData, ok := valMap["block_entity_data"].(map[string]any)
		if !ok {
			continue
		}
		var keyInt int
		fmt.Sscanf(key, "%d", &keyInt)
		blockEntityMap[keyInt] = entityData
	}

	// Release the NBT tree BEFORE the main loop.
	// Once we have the block indices and block entity data, the tree is no longer needed.
	blockPosData = nil
	blockIndices = nil
	paletteData = nil
	root = nil
	structure = nil

	// Build BlockInfo palette and release the intermediate string slices
	blockPalette := make([]BlockInfo, len(paletteNames))
	for i := range paletteNames {
		blockPalette[i] = BlockInfo{Name: paletteNames[i], States: paletteStates[i]}
		computeBlockFlags(&blockPalette[i])
	}
	paletteNames = nil
	paletteStates = nil
	palette = nil

	// Build the result structure.
	// mcstructure stores block_indices in x-major (idx = x * sy * sz + y * sz + z).
	// 只用稀疏 Blocks map（内存红线，§11.1）：对大建筑不再额外建一份 SizeX*SizeY*SizeZ 的全量
	// blocksFlat 数组（那是 OOM 峰值的主要来源）。GetIndex/Get/FreezePalette/AllBlocks 均有
	// 稀疏 fallback，去掉 blocksFlat 是安全的，只损失一点 flat-array 访问速度。
	{
		blocksMap := make(map[uint64]uint32, len(layer0)/10)
		for z := 0; z < sz; z++ {
			for y := 0; y < sy; y++ {
				for x := 0; x < sx; x++ {
					srcIdx := x*sy*sz + y*sz + z
					fg := layer0[srcIdx]
					bg := layer1[srcIdx]
					if fg >= 0 && int(fg) < len(blockPalette) {
						blocksMap[packPos(int32(x), int32(y), int32(z))] = uint32(fg) + 1
					} else if bg >= 0 && int(bg) < len(blockPalette) {
						blocksMap[packPos(int32(x), int32(y), int32(z))] = uint32(bg) + 1
					}
				}
			}
		}

		// Prepend air to palette (index 0 = air)
		paletteWithAir := make([]BlockInfo, 1, len(blockPalette)+1)
		paletteWithAir[0] = BlockInfo{Name: "minecraft:air", States: "[]"}
		computeBlockFlags(&paletteWithAir[0])
		blockPalette = append(paletteWithAir, blockPalette...)

		result := &StructureData{
			SizeX:      sx,
			SizeY:      sy,
			SizeZ:      sz,
			Palette:    blockPalette,
			Blocks:     blocksMap,
		}

		// Handle block entity data (linear index → packed pos)
		if len(blockEntityMap) > 0 {
			result.NBTBlocks = make(map[uint64]map[string]any, len(blockEntityMap))
			for idx, nbt := range blockEntityMap {
				z := idx % sz
				y := (idx / sz) % sy
				x := idx / (sy * sz)
				if x < sx && y < sy && z < sz {
					pos := packPos(int32(x), int32(y), int32(z))
					result.NBTBlocks[pos] = nbt
				}
			}
			result.HasCommand = true
		}

		
		// 释放 layer0/layer1
		layer0 = nil
		layer1 = nil

		return result, nil
	}
}

func toInt32(v any) int32 {
	switch val := v.(type) {
	case int32:
		return val
	case int16:
		return int32(val)
	case int:
		return int32(val)
	case int64:
		return int32(val)
	case int8:
		return int32(val)
	case uint8:
		return int32(val)
	case uint16:
		return int32(val)
	case uint32:
		return int32(val)
	case uint64:
		return int32(val)
	case float64:
		return int32(val)
	case float32:
		return int32(val)
	}
	return -1 // signal "no block" for unknown types
}

// isBoolState reports whether a block state key represents a boolean value.
// In NBT, booleans are stored as TAG_Byte (0/1), but setblock expects true/false.
func isBoolState(key string) bool {
	switch key {
	case
		"open_bit", "open", "powered_bit", "powered",
		"attached_bit", "hanging", "in_wall", "toggle",
		"update_bit", "button_pressed_bit", "triggered_bit",
		"output_subtract_bit", "output_lit_bit", "dead_bit",
		"occupied_bit", "head_piece_bit", "deprecated",
		"suspended_bit", "allow_underwater_bit",
		"explode_bit", "no_drop_bit", "active",
		"disable_encumbrance", "disable_rotation",
		"stripped_bit", "persistent_bit", "covered_bit",
		"can_spread", "conditional_bit", "drag_down",
		"exit", "extinguished", "in_wall_bit", "infini_burn",
		"item_frame_map_bit", "item_frame_photo_bit",
		"liquid_logged", "locked", "mature",
		"no_extra_sound", "ominous", "passable",
		"persistent", "registered_players_only",
		"response_required", "show_bottom",
		"stability", "stability_check", "supports",
		"ticking", "transfer_cooldown", "unlimited",
		"upside_down_bit", "update", "vibration_damper",
		"waterlogged":
		return true
	}
	return strings.HasSuffix(key, "_bit") || strings.HasSuffix(key, "_pressed_bit")
}

func formatBlockStates(states map[string]any) string {
	if states == nil || len(states) == 0 {
		return "[]"
	}
	// 按键名排序，保证输出确定性
	keys := make([]string, 0, len(states))
	for k := range states {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	s := "["
	first := true
	for _, k := range keys {
		v := states[k]
		// 跳过运行时状态（由游戏自动设置，setblock 不接受）
		if k == "grass_block" || k == "snowy" {
			continue
		}
		if !first {
			s += ","
		}
		switch val := v.(type) {
		case string:
			s += fmt.Sprintf(`"%s"="%s"`, k, val)
		case byte:
			if isBoolState(k) {
				if val != 0 {
					s += fmt.Sprintf(`"%s"=true`, k)
				} else {
					s += fmt.Sprintf(`"%s"=false`, k)
				}
			} else {
				s += fmt.Sprintf(`"%s"=%d`, k, val)
			}
		default:
			s += fmt.Sprintf(`"%s"=%v`, k, v)
		}
		first = false
	}
	return s + "]"
}

func splitBlockName(full string) (name string, states string) {
	spaceIdx := -1
	for i := len(full) - 1; i >= 0; i-- {
		if full[i] == ' ' {
			spaceIdx = i
			break
		}
	}
	if spaceIdx == -1 {
		return full, "[]"
	}
	return full[:spaceIdx], full[spaceIdx+1:]
}

func paletteKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m { keys = append(keys, k) }
	return keys
}

func toIntArray(arr any) []int32 {
	switch a := arr.(type) {
	case []any:
		result := make([]int32, len(a))
		for i, v := range a {
			result[i] = toInt32(v)
		}
		return result
	case []int32:
		return a
	}
	return nil
}

// ── 方块状态旋转 ──
// 这些映射把单个方块朝向属性按建筑旋转角度换成新朝向。
// 注意：这里的 “90°” 对应建筑位置的变换 old(x,z)→new(z,sx-1-x)，
// 即朝向相对俯视旋转是逆时针（north→west→south→east）。旋转映射即按此推导。

// 字符串 4 向朝向（north/south/west/east）
var cardinalRot = map[int]map[string]string{
	90:  {"north": "west", "south": "east", "west": "south", "east": "north"},
	180: {"north": "south", "south": "north", "west": "east", "east": "west"},
	270: {"north": "east", "east": "south", "south": "west", "west": "north"},
}

// 字符串 6 向朝向（minecraft:facing_direction，down/up 不旋转）
var sixWayRot = map[int]map[string]string{
	90:  {"north": "west", "south": "east", "west": "south", "east": "north"},
	180: {"north": "south", "south": "north", "west": "east", "east": "west"},
	270: {"north": "east", "east": "south", "south": "west", "west": "north"},
}

// facing_direction 整数朝向（命令方块/活塞/发射器/漏斗等）：
// 0=下 1=上 2=北 3=南 4=西 5=东（垂直 0/1 不旋转）
var facingDirRot = map[int]map[int32]int32{
	90:  {2: 4, 3: 5, 4: 3, 5: 2},
	180: {2: 3, 3: 2, 4: 5, 5: 4},
	270: {2: 5, 5: 3, 3: 4, 4: 2},
}

// direction 整数朝向（中继器/比较器）：0=南 1=西 2=北 3=东
var repeaterDirRot = map[int]map[int32]int32{
	90:  {0: 3, 1: 0, 2: 1, 3: 2},
	180: {0: 2, 2: 0, 1: 3, 3: 1},
	270: {0: 1, 1: 2, 2: 3, 3: 0},
}

// weirdo_direction 整数朝向（楼梯）：0=东 1=西 2=南 3=北
var weirdoDirRot = map[int]map[int32]int32{
	90:  {0: 3, 1: 2, 2: 1, 3: 0},
	180: {0: 1, 1: 0, 2: 3, 3: 2},
	270: {0: 2, 1: 3, 2: 1, 3: 0},
}

// torch_facing_direction 整数朝向（火把/梯子）：0=未知 1=东 2=西 3=南 4=北 5=上
var torchFacingRot = map[int]map[int32]int32{
	90:  {1: 4, 2: 3, 3: 2, 4: 1},
	180: {1: 2, 2: 1, 3: 4, 4: 3},
	270: {1: 3, 3: 2, 2: 4, 4: 1},
}

// axis 轴（原木/柱子/石英）：90° 和 270° 时 x↔z，180° 不变，y 恒定
var axisRot = map[int]map[string]string{
	90:  {"x": "z", "z": "x"},
	180: {"x": "x", "z": "z"},
	270: {"x": "z", "z": "x"},
}

// rotateBlockStates 根据旋转角度重写方块状态字符串里的朝向属性。
// 无法识别或没有朝向属性的状态原样返回。
func rotateBlockStates(blockName, states string, degrees int) string {
	if degrees == 0 || states == "" || states == "[]" {
		return states
	}
	m := utils.ParseBlockStatesString(states)
	if len(m) == 0 {
		return states
	}

	// 字符串 4 向：facing / minecraft:cardinal_direction / horizontal_facing
	for _, key := range []string{"facing", "minecraft:cardinal_direction", "horizontal_facing"} {
		if v, ok := m[key].(string); ok {
			if nv, ok := cardinalRot[degrees][v]; ok {
				m[key] = nv
			}
		}
	}
	// 字符串 6 向：minecraft:facing_direction
	if v, ok := m["minecraft:facing_direction"].(string); ok {
		if nv, ok := sixWayRot[degrees][v]; ok {
			m["minecraft:facing_direction"] = nv
		}
	}
	// 轴：axis
	if v, ok := m["axis"].(string); ok {
		if nv, ok := axisRot[degrees][v]; ok {
			m["axis"] = nv
		}
	}
	// 整数朝向：facing_direction
	if v, ok := m["facing_direction"].(int32); ok {
		if nv, ok := facingDirRot[degrees][v]; ok {
			m["facing_direction"] = nv
		}
	}
	// 整数朝向：direction（中继器/比较器）
	if v, ok := m["direction"].(int32); ok {
		if nv, ok := repeaterDirRot[degrees][v]; ok {
			m["direction"] = nv
		}
	}
	// 整数朝向：weirdo_direction（楼梯）
	if v, ok := m["weirdo_direction"].(int32); ok {
		if nv, ok := weirdoDirRot[degrees][v]; ok {
			m["weirdo_direction"] = nv
		}
	}
	// 整数朝向：torch_facing_direction（火把/梯子）
	if v, ok := m["torch_facing_direction"].(int32); ok {
		if nv, ok := torchFacingRot[degrees][v]; ok {
			m["torch_facing_direction"] = nv
		}
	}

	return utils.MarshalBlockStates(m)
}

// RotateBlocksInPlace 绕 Y 轴旋转建筑数据，直接修改 data。
// 支持角度: 0, 90, 180, 270（顺时针，俯视方向）。
// 不仅旋转方块位置，还会同步旋转方块朝向属性（命令方块朝向、楼梯朝向、中继器方向等）。
func RotateBlocksInPlace(data *StructureData, degrees int) *StructureData {
	degrees = degrees % 360
	if degrees < 0 {
		degrees += 360
	}
	if degrees == 0 {
		return data
	}

	// If using flat array, convert to sparse map for rotation
	if data.BlocksFlat != nil {
		data.Blocks = make(map[uint64]uint32, len(data.BlocksFlat)/10)
		stride := data.SizeY * data.SizeX
		for z := 0; z < data.SizeZ; z++ {
			for y := 0; y < data.SizeY; y++ {
				base := z*stride + y*data.SizeX
				for x := 0; x < data.SizeX; x++ {
					if data.BlocksFlat[base+x] > 0 {
						data.Blocks[packPos(int32(x), int32(y), int32(z))] = data.BlocksFlat[base+x]
					}
				}
			}
		}
		data.BlocksFlat = nil
	}
	oldBlocks := data.Blocks
	type transform func(x, y, z, sx, sz int) (int, int)
	var tr transform
	var newSX, newSZ int

	switch degrees {
	case 90:
		// (x, y, z) → (z, y, SizeX-1-x)，XZ 互换
		tr = func(x, y, z, sx, sz int) (int, int) { return z, sx - 1 - x }
		newSX = data.SizeZ
		newSZ = data.SizeX
	case 180:
		// (x, y, z) → (SizeX-1-x, y, SizeZ-1-z)，XZ 不变
		tr = func(x, y, z, sx, sz int) (int, int) { return sx - 1 - x, sz - 1 - z }
		newSX = data.SizeX
		newSZ = data.SizeZ
	case 270:
		// (x, y, z) → (SizeZ-1-z, y, x)，XZ 互换（逆时针 90°）
		tr = func(x, y, z, sx, sz int) (int, int) { return sz - 1 - z, x }
		newSX = data.SizeZ
		newSZ = data.SizeX
	default:
		return data
	}

	// 旋转每个 palette 条目的方块状态（朝向属性），
	// 确保旋转后命令方块朝向、楼梯方向等与位置一致
	for i := range data.Palette {
		data.Palette[i].States = rotateBlockStates(data.Palette[i].Name, data.Palette[i].States, degrees)
	}
	// 状态变了，palette 查找缓存需重建
	data.paletteLookup = nil

	sx, sz := data.SizeX, data.SizeZ
	newBlocks := make(map[uint64]uint32, len(oldBlocks))
	newNBT := make(map[uint64]map[string]any)
	for pos, idx := range oldBlocks {
		x, y, z := unpackPos(pos)
		nx, nz := tr(int(x), int(y), int(z), sx, sz)
		newPos := packPos(int32(nx), int32(y), int32(nz))
		newBlocks[newPos] = idx
		// Preserve NBT if present
		if nbt, ok := data.NBTBlocks[pos]; ok {
			newNBT[newPos] = nbt
		}
	}
	data.SizeX = newSX
	data.SizeZ = newSZ
	data.Blocks = newBlocks
	data.NBTBlocks = newNBT
	return data
}

