package building

import (
	"fmt"
	"sync"
)

// BlockInfo stores a block's identity. Used as a palette entry.
// NBT data is stored separately in StructureData.NBTBlocks.
type BlockInfo struct {
	Name   string
	States string
	NBT    map[string]any // Deprecated: use StructureData.NBTBlocks instead

	// 预计算标记，避免在导入热路径中重复字符串匹配
	IsAir       bool
	IsDependent bool
	IsGravity   bool
	IsWater     bool
	IsLava      bool
	IsCommand   bool
}

// StructureData holds the parsed structure data in a compact format.
// Blocks are stored as palette-indexed flat array or sparse map.
// Flat array (BlocksFlat) is preferred for performance: it's a single allocation,
// O(1) access without hashing, and cache-friendly iteration.
// Sparse map (Blocks) is used for very large or sparse structures.
type StructureData struct {
	SizeX      int
	SizeY      int
	SizeZ      int
	Palette    []BlockInfo               // shared palette: index → Name + States
	BlocksFlat []uint32                  // flat array: [z*sy*sx + y*sx + x] → palette index, 0 = air
	Blocks     map[uint64]uint32         // packed position → palette index (sparse fallback)
	NBTBlocks  map[uint64]map[string]any // sparse: position → NBT data (only for blocks with NBT)
	Commands   []map[string]any
	HasCommand bool

	// paletteLookup provides O(1) palette index lookup by name+states key.
	// Lazily built; invalidated when Palette is appended to.
	// Key format: name + "\x00" + states
	paletteLookup map[string]uint32
	// paletteMu 保护 Palette/paletteLookup 的并发访问。多机器人共享同一份 StructureData，
	// 各机器人的放置引擎并发调用 getOrCreatePaletteIndex 时若无锁会往 Palette 追加，
	// 导致数据竞争/越界崩溃。锁定后并发安全（§11.1 共享 data 内存红线）。
	paletteMu sync.Mutex
}

// FreezePalette 预生成所有方块（含去水后的状态）的 palette 索引，使执行阶段只读。
// 多机器人共享 data 时，若执行期才生成索引，两台机器会并发往 Palette 追加 → 崩溃。
// 切岛/拆分阶段（单线程）调用一次，把所有可能用到的 (name, states) 都预先登记，
// 之后放置引擎的 getOrCreatePaletteIndex 只会命中已有条目，不再追加。
func (s *StructureData) FreezePalette() {
	if s.BlocksFlat != nil {
		for _, idx := range s.BlocksFlat {
			if idx == 0 {
				continue
			}
			s.getOrCreatePaletteIndex(s.Palette[idx].Name, s.Palette[idx].States)
		}
	} else {
		for _, idx := range s.Blocks {
			if idx == 0 {
				continue
			}
			s.getOrCreatePaletteIndex(s.Palette[idx].Name, s.Palette[idx].States)
		}
	}
}

// packPos packs x,y,z into a uint64 for use as a map key.
// Each axis gets 20 bits (range 0-1,048,575), which is plenty for
// structure-local coordinates (max volume 200M → ~585³ cube).
//
// Layout: [x:20 bits][y:20 bits][z:20 bits][4 unused]
func packPos(x, y, z int32) uint64 {
	return (uint64(uint32(x)) << 40) | (uint64(uint32(y)) << 20) | uint64(uint32(z))
}

// unpackPos extracts x,y,z from a packed uint64.
func unpackPos(key uint64) (x, y, z int32) {
	return int32(key >> 40), int32((key >> 20) & 0xFFFFF), int32(key & 0xFFFFF)
}

// packPosInt is a convenience wrapper for int parameters.
func packPosInt(x, y, z int) uint64 {
	return packPos(int32(x), int32(y), int32(z))
}

// getOrCreatePaletteIndex returns the palette index for a block, adding it if new.
// Uses a hash map for O(1) lookup instead of linear scan.
// Empty names are treated as air (not stored in the Blocks map, so this is a safety net).
func (s *StructureData) getOrCreatePaletteIndex(name, states string) uint32 {
	s.paletteMu.Lock()
	defer s.paletteMu.Unlock()
	if name == "" {
		name = "minecraft:air"
	}
	key := name + "\x00" + states
	// Build lookup map lazily
	if s.paletteLookup == nil {
		s.paletteLookup = make(map[string]uint32, len(s.Palette)+1)
		// 确保索引 0 始终是空气：放置代码用 paletteIdx==0 判断空气，
		// 若第一个被解析的非空气方块占用了索引 0，会被误判为空气而跳过。
		if len(s.Palette) == 0 {
			airInfo := BlockInfo{Name: "minecraft:air", States: "[]"}
			computeBlockFlags(&airInfo)
			s.Palette = append(s.Palette, airInfo)
		}
		for i := range s.Palette {
			entry := &s.Palette[i]
			s.paletteLookup[entry.Name+"\x00"+entry.States] = uint32(i)
			computeBlockFlags(entry) // 确保已有条目的标记也被计算
		}
	}
	// Fast path: O(1) lookup
	if idx, ok := s.paletteLookup[key]; ok {
		return idx
	}
	// Add new entry with pre-computed flags
	idx := uint32(len(s.Palette))
	info := BlockInfo{Name: name, States: states}
	computeBlockFlags(&info)
	s.Palette = append(s.Palette, info)
	s.paletteLookup[key] = idx
	return idx
}

// Get returns the BlockInfo at position (x,y,z), or nil if air/not placed.
// The returned BlockInfo includes NBT data if present at this position.
func (s *StructureData) Get(x, y, z int) *BlockInfo {
	// Prefer flat array (fast path)
	if s.BlocksFlat != nil {
		idx := z*s.SizeY*s.SizeX + y*s.SizeX + x
		if idx < len(s.BlocksFlat) && s.BlocksFlat[idx] > 0 {
			info := s.Palette[s.BlocksFlat[idx]]
			if s.NBTBlocks != nil {
				if nbt, ok := s.NBTBlocks[packPosInt(x, y, z)]; ok {
					info.NBT = nbt
				}
			}
			return &info
		}
		return nil
	}
	// Fall back to sparse map
	if idx, ok := s.Blocks[packPosInt(x, y, z)]; ok {
		info := s.Palette[idx]
		if s.NBTBlocks != nil {
			if nbt, ok := s.NBTBlocks[packPosInt(x, y, z)]; ok {
				info.NBT = nbt
			}
		}
		return &info
	}
	return nil
}

// GetNBT returns the NBT data at position (x,y,z), or nil.
func (s *StructureData) GetNBT(x, y, z int) map[string]any {
	if s.NBTBlocks == nil {
		return nil
	}
	return s.NBTBlocks[packPosInt(x, y, z)]
}

// ensureNBTBlocks lazily initializes the NBTBlocks map.
func (s *StructureData) ensureNBTBlocks() {
	if s.NBTBlocks == nil {
		s.NBTBlocks = make(map[uint64]map[string]any)
	}
}

// Set stores a block at position (x,y,z) using palette indexing.
func (s *StructureData) Set(x, y, z int, info *BlockInfo) {
	if info == nil {
		return
	}
	idx := s.getOrCreatePaletteIndex(info.Name, info.States)
	pos := packPosInt(x, y, z)
	s.Blocks[pos] = idx
	if info.NBT != nil && len(info.NBT) > 0 {
		s.ensureNBTBlocks()
		s.NBTBlocks[pos] = info.NBT
		s.HasCommand = true
	}
}

// Delete removes a block at position (x,y,z).
func (s *StructureData) Delete(x, y, z int) {
	pos := packPosInt(x, y, z)
	delete(s.Blocks, pos)
	delete(s.NBTBlocks, pos)
}

// Len returns the number of non-air blocks.
func (s *StructureData) Len() int {
	if s.BlocksFlat != nil {
		return len(s.Blocks) // deprecated, use Blocks for Len with flat array
	}
	return len(s.Blocks)
}

// BlockPos represents a single block position with its palette index.
type BlockPos struct {
	X, Y, Z int32
	Index   uint32
}

// AllBlocks returns a flat slice of all non-air blocks for iteration.
func (s *StructureData) AllBlocks() []BlockPos {
	if s.BlocksFlat != nil {
		result := make([]BlockPos, 0, len(s.BlocksFlat)/10)
		stride := s.SizeY * s.SizeX
		for z := 0; z < s.SizeZ; z++ {
			for y := 0; y < s.SizeY; y++ {
				base := z*stride + y*s.SizeX
				for x := 0; x < s.SizeX; x++ {
					idx := s.BlocksFlat[base+x]
					if idx > 0 {
						result = append(result, BlockPos{X: int32(x), Y: int32(y), Z: int32(z), Index: idx})
					}
				}
			}
		}
		return result
	}
	result := make([]BlockPos, 0, len(s.Blocks))
	for pos, idx := range s.Blocks {
		x, y, z := unpackPos(pos)
		result = append(result, BlockPos{X: x, Y: y, Z: z, Index: idx})
	}
	return result
}

// computeBlockFlags 预计算 BlockInfo 的布尔标记，避免在导入热路径中重复字符串匹配。
func computeBlockFlags(info *BlockInfo) {
	name := info.Name
	info.IsAir = IsAirBlock(name)
	info.IsWater = IsWaterBlock(name)
	info.IsLava = IsLavaBlock(name)
	info.IsCommand = IsCommandBlock(name)
	info.IsDependent = isDependentBlock(name)
	info.IsGravity = isGravityBlock(name)
}

// GetIndex 返回 (x,y,z) 处的调色板索引，0=空气。
// 相比 Get() 避免了创建 BlockInfo 结构体，专为热路径优化。
func (s *StructureData) GetIndex(x, y, z int) uint32 {
	if s.BlocksFlat != nil {
		idx := z*s.SizeY*s.SizeX + y*s.SizeX + x
		if idx < len(s.BlocksFlat) {
			return s.BlocksFlat[idx]
		}
		return 0
	}
	return s.Blocks[packPosInt(x, y, z)]
}

// Validate checks that the structure data is internally consistent.
func (s *StructureData) Validate() error {
	for pos, idx := range s.Blocks {
		if int(idx) >= len(s.Palette) {
			x, y, z := unpackPos(pos)
			return fmt.Errorf("block at (%d,%d,%d): palette index %d out of range (palette size %d)",
				x, y, z, idx, len(s.Palette))
		}
	}
	return nil
}