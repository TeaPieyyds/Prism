package main

import (
	"encoding/binary"
	"math"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

// ============================================================
// SubChunk — 子区块块存储解析器
// 每个 SubChunk 是 16×16×16 的方块，使用调色板压缩
// 方块顺序: x + z*16 + y*256 (XZY 顺序)
// ============================================================

const (
	SubChunkBlocks        = 4096 // 16*16*16
	SubChunkSectionHeight = 16
)

// SubChunk 存储 4096 个方块的运行时 ID
type SubChunk struct {
	Blocks       [SubChunkBlocks]uint32
	Waterlogging *SubChunk // 第二层（含水）
}

// GetBlock 获取本地坐标的方块运行时 ID
func (s *SubChunk) GetBlock(x, y, z int) uint32 {
	idx := (x & 0xF) + ((z & 0xF) << 4) + ((y & 0xF) << 8)
	return s.Blocks[idx]
}

// parseBlockStorage 解析一个方块存储层
// 格式:
//
//	byte 0: (bitsPerBlock << 1) | isRuntime
//	后续: 压缩的方块索引 (小端 32-bit words)
//	最后: 调色板 (varint 数量 + varint 条目)
func parseBlockStorage(data []byte, offset int) (*SubChunk, int) {
	sub := &SubChunk{}
	pos := offset

	if pos >= len(data) {
		return sub, 0
	}

	header := data[pos]
	pos++
	isRuntime := (header & 1) != 0
	bitsPerBlock := header >> 1

	if bitsPerBlock == 0 {
		// 所有方块相同
		if pos+4 <= len(data) {
			runtimeID, n := readVarInt(data, pos)
			pos = n
			for i := range sub.Blocks {
				sub.Blocks[i] = uint32(runtimeID)
			}
		}
		return sub, pos - offset
	}

	if bitsPerBlock < 1 || bitsPerBlock > 16 {
		return sub, 0
	}

	blocksPerWord := int(32 / bitsPerBlock)
	wordCount := (SubChunkBlocks + blocksPerWord - 1) / blocksPerWord
	mask := uint32((1 << bitsPerBlock) - 1)

	// 读取压缩的方块索引
	indices := make([]uint32, SubChunkBlocks)
	blockIdx := 0

	for w := 0; w < wordCount && pos+4 <= len(data); w++ {
		word := binary.LittleEndian.Uint32(data[pos:])
		pos += 4

		for b := 0; b < int(blocksPerWord) && blockIdx < SubChunkBlocks; b++ {
			indices[blockIdx] = (word >> (b * int(bitsPerBlock))) & mask
			blockIdx++
		}
	}

	// 读取调色板
	paletteSize, n := readVarInt(data, pos)
	pos = n
	palette := make([]uint32, paletteSize)
	for i := range palette {
		val, n := readVarInt(data, pos)
		palette[i] = uint32(val)
		pos = n
	}

	// 通过调色板映射索引到运行时 ID
	for i := 0; i < SubChunkBlocks && i < len(indices); i++ {
		idx := indices[i]
		if int(idx) < len(palette) {
			sub.Blocks[i] = palette[idx]
		} else {
			sub.Blocks[i] = 0
		}
		// 如果是旧版 ID（非 runtime），保留原样
		if !isRuntime {
			sub.Blocks[i] = idx // 旧版直接存 ID
		}
	}

	return sub, pos - offset
}

// readVarInt 读取无符号 VarInt
func readVarInt(data []byte, offset int) (uint32, int) {
	var value uint32
	var shift uint
	pos := offset

	for pos < len(data) {
		b := data[pos]
		pos++
		value |= uint32(b&0x7F) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
		if shift > 35 {
			break
		}
	}

	return value, pos
}

// ============================================================
// ChunkColumn — 16×384×16 的方块列
// 世界 Y 范围: -64 到 319 (24 个 SubChunk)
// ============================================================

const (
	WorldMinY      = -64
	WorldMaxY      = 319
	WorldSubChunks = 24 // (319 - (-64) + 1) / 16
)

// ChunkPos 区块坐标 (使用 [2]int32 匹配 protocol.ChunkPos)
type ChunkPos = [2]int32

// ChunkColumn 存储一个区块列的所有方块数据
type ChunkColumn struct {
	Pos      ChunkPos
	Sections [WorldSubChunks]*SubChunk
}

// sectionIndex 计算世界 Y 坐标对应的 SubChunk 索引
func (c *ChunkColumn) sectionIndex(y int) int {
	if y < WorldMinY || y > WorldMaxY {
		return -1
	}
	idx := (y - WorldMinY) / SubChunkSectionHeight
	if idx >= len(c.Sections) {
		return -1
	}
	return idx
}

// GetBlock 获取世界坐标的方块运行时 ID
func (c *ChunkColumn) GetBlock(x, y, z int) uint32 {
	si := c.sectionIndex(y)
	if si < 0 {
		return 0
	}
	sec := c.Sections[si]
	if sec == nil {
		return 0
	}
	return sec.GetBlock(x&0xF, (y-WorldMinY)&0xF, z&0xF)
}

// SetBlock 设置世界坐标的方块运行时 ID
func (c *ChunkColumn) SetBlock(x, y, z int, runtimeID uint32) {
	si := c.sectionIndex(y)
	if si < 0 {
		return
	}
	if c.Sections[si] == nil {
		c.Sections[si] = &SubChunk{}
	}
	idx := (x & 0xF) + ((z & 0xF) << 4) + ((y-WorldMinY)&0xF)<<8
	c.Sections[si].Blocks[idx] = runtimeID
}

// ============================================================
// World — 世界数据管理器
// ============================================================

// World 管理所有已加载的区块
// 限制最大区块数防止内存爆炸（每个区块约 400KB）
// 默认不启用，只有前端打开寻路/飞行控制面板时才收集区块数据
type World struct {
	columns map[ChunkPos]*ChunkColumn
	keys    []ChunkPos // 插入顺序，用于 FIFO 淘汰
	enabled bool       // 是否收集区块数据
}

const MaxWorldChunks = 256 // 256 区块 × 400KB ≈ 100MB 上限

func NewWorld() *World {
	return &World{
		columns: make(map[ChunkPos]*ChunkColumn, MaxWorldChunks),
		keys:    make([]ChunkPos, 0, MaxWorldChunks),
		enabled: false,
	}
}

// SetEnabled 开启/关闭世界数据收集。
// 关闭时自动清空所有已缓存的区块数据，释放内存。
func (w *World) SetEnabled(enabled bool) {
	if enabled == w.enabled {
		return
	}
	w.enabled = enabled
	if !enabled {
		w.Reset()
	}
}

// IsEnabled 返回世界数据收集是否开启。
func (w *World) IsEnabled() bool {
	return w.enabled
}

func chunkPosKey(x, z int32) ChunkPos {
	return ChunkPos{x, z}
}

// GetChunk 获取指定坐标的区块
func (w *World) GetChunk(cx, cz int32) *ChunkColumn {
	return w.columns[chunkPosKey(cx, cz)]
}

// SetChunk 设置指定坐标的区块
// 超过最大数量时淘汰最旧的区块（FIFO），防止内存无限增长
func (w *World) SetChunk(cx, cz int32, col *ChunkColumn) {
	key := chunkPosKey(cx, cz)
	if _, exists := w.columns[key]; exists {
		w.columns[key] = col
		return
	}
	// 淘汰：超过上限时删除最旧的 1/4
	if len(w.columns) >= MaxWorldChunks {
		evict := len(w.keys) / 4
		if evict < 1 {
			evict = 1
		}
		for _, old := range w.keys[:evict] {
			delete(w.columns, old)
		}
		w.keys = w.keys[evict:]
	}
	w.columns[key] = col
	w.keys = append(w.keys, key)
}

// BlockAt 获取世界坐标的方块运行时 ID
// 世界坐标转区块坐标: cx = x >> 4, cz = z >> 4
func (w *World) BlockAt(x, y, z int) uint32 {
	cx := x >> 4
	cz := z >> 4
	col := w.GetChunk(int32(cx), int32(cz))
	if col == nil {
		return 0 // 区块未加载 = 空气
	}
	return col.GetBlock(x, y, z)
}

// IsSolid 检查方块是否实心（不可通过）
// 0 = 空气, 其它大部分非 0 的是实心方块
func (w *World) IsSolid(x, y, z int) bool {
	rtid := w.BlockAt(x, y, z)
	return rtid != 0 // 非 air 即为实心
}

// IsAir 检查方块是否为空气
func (w *World) IsAir(x, y, z int) bool {
	return w.BlockAt(x, y, z) == 0
}

// IsWalkable 检查方块是否可通行（空气或非实心）
func (w *World) IsWalkable(x, y, z int) bool {
	return !w.IsSolid(x, y, z)
}

// IsSafe 检查位置是否安全可站
// 要求: 脚下实心, 脚下方块可通行, 头上方块可通行
func (w *World) IsSafe(x, y, z int) bool {
	belowSolid := w.IsSolid(x, y-1, z)
	feetFree := w.IsWalkable(x, y, z)
	headFree := w.IsWalkable(x, y+1, z)
	return belowSolid && feetFree && headFree
}

// ParseLevelChunk 解析 LevelChunk 数据包
// 格式: 连续 N 个 SubChunk (版本 8/9/10+), 然后是高度图+生物群系
func (w *World) ParseLevelChunk(cx, cz int32, subChunkCount uint32, data []byte) {
	pfLog("ParseLevelChunk: chunk=(%d,%d) subChunks=%d dataLen=%d", cx, cz, subChunkCount, len(data))

	col := &ChunkColumn{Pos: ChunkPos{cx, cz}}
	offset := 0

	// SubChunkRequestModeLimitless = math.MaxUint32 - 1 = 4294967294
	// SubChunkRequestModeLimited = math.MaxUint32 = 4294967295
	// 这两种模式下 LevelChunk 不含方块数据，只有高度图+生物群系
	if subChunkCount > 0 && subChunkCount < math.MaxUint32-1 {
		for i := uint32(0); i < subChunkCount && offset < len(data); i++ {
			if offset >= len(data) {
				pfLog("  WARN: chunk(%d,%d) subChunk %d: offset overflow", cx, cz, i)
				break
			}
			version := data[offset]
			offset++

			n := 0
			switch {
			case version == 0:
				// 全空气
				col.Sections[i] = &SubChunk{}
				n = 0
				pfLog("  subChunk[%d]: version=0 (all air)", i)

			case version == 1:
				// 旧版格式：4096 字节方块 ID
				sub := &SubChunk{}
				for b := 0; b < SubChunkBlocks && offset < len(data); b++ {
					sub.Blocks[b] = uint32(data[offset])
					offset++
				}
				col.Sections[i] = sub
				n = SubChunkBlocks
				pfLog("  subChunk[%d]: version=1 (legacy)", i)

			case version == 8 || version == 9:
				// 新版调色板格式
				layerCount := 1
				if version == 9 {
					layerCount = int(data[offset])
					offset++
				}
				for layer := 0; layer < layerCount && offset < len(data); layer++ {
					sub, read := parseBlockStorage(data, offset)
					if read == 0 {
						break
					}
					if layer == 0 {
						col.Sections[i] = sub
					} else if layer == 1 {
						// 含水层
						if col.Sections[i] != nil {
							col.Sections[i].Waterlogging = sub
						}
					}
					offset += read
					n += read
				}
				pfLog("  subChunk[%d]: version=%d layers=%d bytes=%d", i, version, layerCount, n)

			case version >= 10:
				// 新版格式（版本 10+）
				if version >= 11 && offset < len(data) {
					offset++ // 跳过缓存标志
					n++
				}
				for offset < len(data) {
					sub, read := parseBlockStorage(data, offset)
					if read == 0 {
						break
					}
					if col.Sections[i] == nil {
						col.Sections[i] = sub
					} else if col.Sections[i].Waterlogging == nil {
						col.Sections[i].Waterlogging = sub
					}
					offset += read
					n += read
				}
				pfLog("  subChunk[%d]: version=%d bytes=%d", i, version, n)

			default:
				pfLog("  WARN: chunk(%d,%d) subChunk %d: unknown version %d", cx, cz, i, version)
				// 尝试跳过未知格式
				if offset+1 < len(data) {
					// 尝试读取下一个版本字节
					continue
				}
			}
		}

		// 跳过高度图（512 字节）
		if offset+512 <= len(data) {
			offset += 512
		}

		// 跳过生物群系（256 或 4096 字节）
		if offset+4096 <= len(data) {
			offset += 4096
		} else if offset+256 <= len(data) {
			offset += 256
		}
	} else {
		pfLog("  subchunk request mode (subChunkCount=%d), skipping block data", subChunkCount)
	}

	// 方块实体数据在剩余部分，暂时忽略

	w.SetChunk(cx, cz, col)
	pfLog("  -> chunk(%d,%d) stored, total sections=%d", cx, cz, subChunkCount)
}

// ParseSubChunk 解析 SubChunk 响应包，更新世界方块数据
func (w *World) ParseSubChunk(pk *packet.SubChunk) {
	for _, entry := range pk.SubChunkEntries {
		if entry.Result != protocol.SubChunkResultSuccess && entry.Result != protocol.SubChunkResultSuccessAllAir {
			continue
		}
		// 计算绝对子区块坐标
		cx := pk.Position[0] + int32(entry.Offset[0])
		cy := pk.Position[1] + int32(entry.Offset[1])
		cz := pk.Position[2] + int32(entry.Offset[2])

		col := w.GetChunk(cx, cz)
		if col == nil {
			col = &ChunkColumn{Pos: ChunkPos{cx, cz}}
			w.SetChunk(cx, cz, col)
		}

		if entry.Result == protocol.SubChunkResultSuccessAllAir {
			// 全空气，清空该子区块
			si := col.sectionIndex(int(cy)*16 + WorldMinY)
			if si >= 0 && si < len(col.Sections) {
				col.Sections[si] = &SubChunk{}
			}
			continue
		}

		// 解析方块数据
		if len(entry.RawPayload) == 0 {
			continue
		}
		version := entry.RawPayload[0]
		data := entry.RawPayload
		offset := 1

		var sub *SubChunk
		switch {
		case version == 0:
			sub = &SubChunk{}
		case version == 1:
			sub = &SubChunk{}
			for b := 0; b < SubChunkBlocks && offset < len(data); b++ {
				sub.Blocks[b] = uint32(data[offset])
				offset++
			}
		case version == 8 || version == 9:
			layerCount := 1
			if version == 9 && offset < len(data) {
				layerCount = int(data[offset])
				offset++
			}
			for layer := 0; layer < layerCount && offset < len(data); layer++ {
				parsed, read := parseBlockStorage(data, offset)
				if read == 0 {
					break
				}
				if layer == 0 {
					sub = parsed
				} else if layer == 1 && sub != nil {
					sub.Waterlogging = parsed
				}
				offset += read
			}
		case version >= 10:
			if version >= 11 && offset < len(data) {
				offset++ // 跳过缓存标志
			}
			for offset < len(data) {
				parsed, read := parseBlockStorage(data, offset)
				if read == 0 {
					break
				}
				if sub == nil {
					sub = parsed
				} else if sub.Waterlogging == nil {
					sub.Waterlogging = parsed
				}
				offset += read
			}
		default:
			pfLog("ParseSubChunk: unknown version %d at (%d,%d,%d)", version, cx, cy, cz)
		}

		if sub != nil {
			// cy 是绝对子区块索引，转换为 section index
			si := col.sectionIndex(int(cy)*16 + WorldMinY)
			if si >= 0 && si < len(col.Sections) {
				col.Sections[si] = sub
				pfLog("ParseSubChunk: chunk(%d,%d) section[%d] updated (result=%d)", cx, cz, si, entry.Result)
			}
		}
	}
}

// Reset 清空所有区块数据，释放内存。
// 在断开连接或重新连接时调用。
func (w *World) Reset() {
	w.columns = make(map[ChunkPos]*ChunkColumn, MaxWorldChunks)
	w.keys = make([]ChunkPos, 0, MaxWorldChunks)
}

// 全局世界实例
var globalWorld = NewWorld()
