package building

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/df-mc/goleveldb/leveldb"
	"github.com/df-mc/goleveldb/leveldb/opt"
	"github.com/df-mc/goleveldb/leveldb/util"
)

const (
	keyVersion       = byte(',')
	keySubChunkData  = byte('/')
	keyBlockEntities = "1"
)

func ParseMCWorld(path string) (*StructureData, error) {
	return ParseMCWorldWithRegion(path, parseMCWorldRegion(filepath.Base(path)))
}

func ParseMCWorldWithRegion(path string, region [6]int) (*StructureData, error) {

	// Use internal storage temp dir first, fall back to file dir or system tmp
	tmpDir, err := os.MkdirTemp("/data/data/com.prismtool.box/files", "mcworld_")
	if err != nil {
		for _, dir := range []string{filepath.Dir(path)} {
			tmpDir, err = os.MkdirTemp(dir, "mcworld_")
			if err == nil {
				break
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("ParseMCWorld: temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("ParseMCWorld: open zip: %w", err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		dst := filepath.Join(tmpDir, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(dst, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(dst), 0755)
		rc, _ := f.Open()
		out, _ := os.Create(dst)
		io.Copy(out, rc)
		rc.Close()
		out.Close()
	}

	dbPath := filepath.Join(tmpDir, "db")
	if rldb, err := leveldb.RecoverFile(dbPath, &opt.Options{}); err == nil {
		rldb.Close()
	}
	if ldb, err := leveldb.OpenFile(dbPath, &opt.Options{}); err == nil {
		ldb.CompactRange(util.Range{})
		ldb.Close()
	}

	sx, sy, sz := region[0], region[1], region[2]
	ex, ey, ez := region[3], region[4], region[5]
	if sx == 0 && ex == 0 {
		sx, sy, sz = -128, -64, -128
		ex, ey, ez = 128, 256, 128
	}
	if sx > ex {
		sx, ex = ex, sx
	}
	if sy > ey {
		sy, ey = ey, sy
	}
	if sz > ez {
		sz, ez = ez, sz
	}
	dx, dy, dz := ex-sx+1, ey-sy+1, ez-sz+1
	if dx > 1024 {
		dx = 1024
	}
	if dz > 1024 {
		dz = 1024
	}
	if err := checkVolume(dx, dy, dz); err != nil {
		return nil, fmt.Errorf("ParseMCWorld: %w", err)
	}

	result := &StructureData{SizeX: dx, SizeY: dy, SizeZ: dz, Blocks: make(map[uint64]uint32)}

	// readDirect returns map[uint64]uint32 and a local palette, merge into result
	directBlocks, directPalette := readDirect(dbPath, sx, ex, sy, ey, sz, ez, dx, dy, dz)
	for pos, localIdx := range directBlocks {
		ns := directPalette[localIdx]
		idx := result.getOrCreatePaletteIndex(ns.name, ns.states)
		result.Blocks[pos] = idx
	}

	// 第二遍: world.Open 补全 readDirect 漏掉的块 (带 states)
	// 注意: bedrock-world-operator 库的 decodePalette 没有校验 paletteCount，
	// 损坏的 subchunk/biome 数据会导致 make([]uint32, huge) → OOM。
	// 因此先通过 LoadChunkPayloadOnly + LoadBiomes 加载裸数据，校验 paletteCount
	// 合法后才调用安全的 safeDiskDecode 来解码。
	w, err := world.Open(tmpDir, nil)
	if err == nil {
		for cx := sx >> 4; cx <= ex>>4; cx++ {
			for cz := sz >> 4; cz <= ez>>4; cz++ {
				cpos := define.ChunkPos{int32(cx), int32(cz)}

				// 加载裸 subchunk 和 biome 数据（安全，不触发解码）
				payload, exists, _ := w.LoadChunkPayloadOnly(define.DimensionIDOverworld, cpos)
				if !exists { continue }

				// 校验 subchunk 数据
				safe := true
				for _, raw := range payload {
					if len(raw) == 0 { continue }
					if !isValidSubchunkData(raw) {
						safe = false
						break
					}
				}
				if !safe { continue }

				// 校验 biome 数据
				biomes, _ := w.LoadBiomes(define.DimensionIDOverworld, cpos)
				if !isValidBiomeData(biomes) {
					continue
				}

				chunk2, _, _ := w.LoadChunk(define.DimensionIDOverworld, cpos)
				if chunk2 == nil { continue }
				subs := chunk2.Sub()
				rng := chunk2.Range()
				baseSubY := int(rng[0]) >> 4
				for si, sub := range subs {
					if sub == nil || sub.Empty() { continue }
					subY := (baseSubY + si) << 4
					for bx := 0; bx < 16; bx++ {
						for bz := 0; bz < 16; bz++ {
							for by := 0; by < 16; by++ {
								wx, wy, wz := cx<<4+bx, subY+by, cz<<4+bz
								if wx < sx || wx > ex || wy < sy || wy > ey || wz < sz || wz > ez { continue }
								rid := sub.Block(uint8(bx), uint8(by), uint8(bz), 0)
								if rid == 0 { continue }
								name, props, found := block.RuntimeIDToState(rid)
								if name == "minecraft:air" || !found { continue }
								rx, ry, rz := wx-sx, wy-sy, wz-sz
								if rx >= 0 && rx < dx && ry >= 0 && ry < dy && rz >= 0 && rz < dz {
									pos := packPos(int32(rx), int32(ry), int32(rz))
									states := statesToString(props)
									if _, ok := result.Blocks[pos]; ok {
										idx := result.Blocks[pos]
										if result.Palette[idx].Name != name || result.Palette[idx].States != states {
											newIdx := result.getOrCreatePaletteIndex(name, states)
											result.Blocks[pos] = newIdx
										}
									} else {
										idx := result.getOrCreatePaletteIndex(name, states)
										result.Blocks[pos] = idx
									}
								}
							}
						}
					}
				}
			}
		}
		w.CloseWorld()
	}

	// 最后: 挂 NBT 数据 (此时所有块已有正确的 name + states)
	blockEntityMap := readBlockEntities(dbPath, sx, ex, sy, ey, sz, ez, dx, dy, dz)
	if len(blockEntityMap) > 0 {
		result.NBTBlocks = blockEntityMap
		for _, nbtData := range blockEntityMap {
			if cmd, ok := nbtData["Command"].(string); ok && cmd != "" {
				result.HasCommand = true
				break
			}
		}
	}
	return result, nil
}

type nameState struct{ name, states string }

func readDirect(dbPath string, sx, ex, sy, ey, sz, ez, dx, dy, dz int) (map[uint64]uint32, []nameState) {
	ldb, err := leveldb.OpenFile(dbPath, &opt.Options{})
	if err != nil {
		return nil, nil
	}
	defer ldb.Close()

	blocks := make(map[uint64]uint32)
	localPalette := make([]nameState, 0)
	getOrCreateLocal := func(name, states string) uint32 {
		for i, ns := range localPalette {
			if ns.name == name && ns.states == states {
				return uint32(i)
			}
		}
		localPalette = append(localPalette, nameState{name, states})
		return uint32(len(localPalette) - 1)
	}

	prefix := make([]byte, 8)
	for cx := sx >> 4; cx <= ex>>4; cx++ {
		for cz := sz >> 4; cz <= ez>>4; cz++ {
			binary.LittleEndian.PutUint32(prefix[0:4], uint32(cx))
			binary.LittleEndian.PutUint32(prefix[4:8], uint32(cz))
			verKey := append([]byte{}, prefix...)
			verKey = append(verKey, keyVersion)
			if has, _ := ldb.Has(verKey, nil); !has {
				continue
			}
			for si := 0; si < 24; si++ {
				subKey := append([]byte{}, prefix...)
				subKey = append(subKey, keySubChunkData, byte(si))
				subData, err := ldb.Get(subKey, nil)
				if err != nil || len(subData) < 3 {
					continue
				}
				subY := si << 4
				for bidx, rid := range decodeSub(subData) {
					if rid == 0 {
						continue
					}
					bx, bz, by := bidx&15, (bidx>>4)&15, (bidx>>8)&15
					wx, wy, wz := cx<<4+bx, subY+by, cz<<4+bz
					if wx < sx || wx > ex || wy < sy || wy > ey || wz < sz || wz > ez {
						continue
					}
					name, props, found := block.RuntimeIDToState(rid)
					if !found || name == "minecraft:air" {
						continue
					}
					rx, ry, rz := wx-sx, wy-sy, wz-sz
					if rx >= 0 && rx < dx && ry >= 0 && ry < dy && rz >= 0 && rz < dz {
						pos := packPos(int32(rx), int32(ry), int32(rz))
						blocks[pos] = getOrCreateLocal(name, statesToString(props))
					}
				}
			}
		}
	}
	return blocks, localPalette
}

// readBlockEntities reads block entity NBT from LevelDB.
// LevelDB key format: chunk_index(8 bytes LE) + "1"
// Value is N TAG_Compound concatenated (not a TAG_List).
func readBlockEntities(dbPath string, sx, ex, sy, ey, sz, ez, dx, dy, dz int) map[uint64]map[string]any {
	ldb, err := leveldb.OpenFile(dbPath, &opt.Options{})
	if err != nil {
		return nil
	}
	defer ldb.Close()

	result := make(map[uint64]map[string]any)
	prefix := make([]byte, 8)
	for cx := sx >> 4; cx <= ex>>4; cx++ {
		for cz := sz >> 4; cz <= ez>>4; cz++ {
			binary.LittleEndian.PutUint32(prefix[0:4], uint32(cx))
			binary.LittleEndian.PutUint32(prefix[4:8], uint32(cz))
			nbtKey := append([]byte{}, prefix...)
			nbtKey = append(nbtKey, keyBlockEntities...)
			nbtData, err := ldb.Get(nbtKey, nil)
			if err != nil || len(nbtData) < 2 {
				continue
			}

			offset := 0
			for offset < len(nbtData) {
				reader := bytes.NewReader(nbtData[offset:])
				tagType, _ := reader.ReadByte()
				if tagType != 0x0a {
					break
				}
				var nameLen [2]byte
				if _, err := io.ReadFull(reader, nameLen[:]); err != nil {
					break
				}
				nl := int(binary.LittleEndian.Uint16(nameLen[:]))
				if _, err := reader.Seek(int64(nl), io.SeekCurrent); err != nil {
					break
				}

				entity, err := parseNBTCompound(reader)
				if err != nil {
					break
				}
				consumed := len(nbtData[offset:]) - reader.Len()
				offset += consumed

				exx := toIntAny(entity["x"])
				eyy := toIntAny(entity["y"])
				ezz := toIntAny(entity["z"])
				if exx == 0 && eyy == 0 && ezz == 0 {
					continue
				}
				if int(exx) < sx || int(exx) > ex || int(eyy) < sy || int(eyy) > ey || int(ezz) < sz || int(ezz) > ez {
					continue
				}
				rx, ry, rz := int(exx)-sx, int(eyy)-sy, int(ezz)-sz
				result[packPos(int32(rx), int32(ry), int32(rz))] = entity
			}
		}
	}
	return result
}

// ---- Custom LittleEndian NBT parser for LevelDB block entities ----
// Handles all NBT tag types and produces map[string]any with Go native types.
// This avoids the flowers-for-machines NBT decoder which has issues with
// certain LevelDB-specific compound structures.

// maxNBTDepth 限制 NBT 复合标签的嵌套深度，防止损坏文件导致栈溢出。
const maxNBTDepth = 64

func parseNBTCompound(r *bytes.Reader) (map[string]any, error) {
	return parseNBTCompoundDepth(r, 0)
}

func parseNBTCompoundDepth(r *bytes.Reader, depth int) (map[string]any, error) {
	if depth > maxNBTDepth {
		return nil, fmt.Errorf("NBT compound nesting too deep (%d)", depth)
	}
	result := make(map[string]any)
	for {
		tagType, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				// 数据提前结束，缺少 TAG_End 结尾，视为损坏数据
				return result, fmt.Errorf("NBT compound missing TAG_End terminator")
			}
			return result, err
		}
		if tagType == 0 {
			return result, nil
		}

		var nameLen [2]byte
		if _, err := io.ReadFull(r, nameLen[:]); err != nil {
			return result, err
		}
		nl := int(binary.LittleEndian.Uint16(nameLen[:]))
		name := ""
		if nl > 0 {
			if err := checkNBTArrayLength(r, nl, 1); err != nil {
				return result, err
			}
			nameBuf := make([]byte, nl)
			if _, err := io.ReadFull(r, nameBuf); err != nil {
				return result, err
			}
			name = string(nameBuf)
		}

		val, err := parseNBTValueDepth(r, tagType, depth)
		if err != nil {
			return result, err
		}
		if name != "" {
			result[name] = val
		}
	}
}

func parseNBTValue(r *bytes.Reader, tagType byte) (any, error) {
	return parseNBTValueDepth(r, tagType, 0)
}

func parseNBTValueDepth(r *bytes.Reader, tagType byte, depth int) (any, error) {
	if depth > maxNBTDepth {
		return nil, fmt.Errorf("NBT nesting too deep (%d)", depth)
	}
	switch tagType {
	case 0: // TAG_End
		return nil, nil
	case 1: // TAG_Byte
		b, _ := r.ReadByte()
		return b, nil
	case 2: // TAG_Short
		var buf [2]byte
		io.ReadFull(r, buf[:])
		return int16(binary.LittleEndian.Uint16(buf[:])), nil
	case 3: // TAG_Int
		var buf [4]byte
		io.ReadFull(r, buf[:])
		return int32(binary.LittleEndian.Uint32(buf[:])), nil
	case 4: // TAG_Long
		var buf [8]byte
		io.ReadFull(r, buf[:])
		return int64(binary.LittleEndian.Uint64(buf[:])), nil
	case 5: // TAG_Float
		var buf [4]byte
		io.ReadFull(r, buf[:])
		return math.Float32frombits(binary.LittleEndian.Uint32(buf[:])), nil
	case 6: // TAG_Double
		var buf [8]byte
		io.ReadFull(r, buf[:])
		return math.Float64frombits(binary.LittleEndian.Uint64(buf[:])), nil
	case 7: // TAG_ByteArray
		var buf [4]byte
		io.ReadFull(r, buf[:])
		length := int(binary.LittleEndian.Uint32(buf[:]))
		if err := checkNBTArrayLength(r, length, 1); err != nil {
			return nil, err
		}
		data := make([]byte, length)
		io.ReadFull(r, data)
		return data, nil
	case 8: // TAG_String
		var buf [2]byte
		io.ReadFull(r, buf[:])
		length := int(binary.LittleEndian.Uint16(buf[:]))
		if err := checkNBTArrayLength(r, length, 1); err != nil {
			return nil, err
		}
		data := make([]byte, length)
		io.ReadFull(r, data)
		return string(data), nil
	case 9: // TAG_List
		childType, _ := r.ReadByte()
		var buf [4]byte
		io.ReadFull(r, buf[:])
		length := int(binary.LittleEndian.Uint32(buf[:]))
		// List 元素是变长类型，不做剩余字节校验（可能误伤合法数据）
		// 只做绝对上限：make([]any, N) 每个元素 ~8 字节指针
		if length > maxNBTArrayItems {
			return nil, fmt.Errorf("NBT list too large (%d items, max %d)", length, maxNBTArrayItems)
		}
		list := make([]any, length)
		for i := 0; i < length; i++ {
			v, err := parseNBTValueDepth(r, childType, depth)
			if err != nil {
				return nil, err
			}
			list[i] = v
		}
		return list, nil
	case 10: // TAG_Compound
		return parseNBTCompoundDepth(r, depth+1)
	case 11: // TAG_IntArray
		var buf [4]byte
		io.ReadFull(r, buf[:])
		length := int(binary.LittleEndian.Uint32(buf[:]))
		if err := checkNBTArrayLength(r, length, 4); err != nil {
			return nil, err
		}
		arr := make([]int32, length)
		for i := 0; i < length; i++ {
			io.ReadFull(r, buf[:])
			arr[i] = int32(binary.LittleEndian.Uint32(buf[:]))
		}
		return arr, nil
	case 12: // TAG_LongArray
		var buf [4]byte
		io.ReadFull(r, buf[:])
		length := int(binary.LittleEndian.Uint32(buf[:]))
		if err := checkNBTArrayLength(r, length, 8); err != nil {
			return nil, err
		}
		arr := make([]int64, length)
		for i := 0; i < length; i++ {
			var buf8 [8]byte
			io.ReadFull(r, buf8[:])
			arr[i] = int64(binary.LittleEndian.Uint64(buf8[:]))
		}
		return arr, nil
	}
	return nil, fmt.Errorf("unknown NBT tag: %d", tagType)
}

// maxNBTArrayBytes 限制单个 NBT 数组/列表字段的最大字节数，
// 防止损坏文件中的伪造长度字段触发超大内存分配导致 OOM。
const maxNBTArrayBytes = 8 << 20 // 8 MB

// maxNBTArrayItems 限制 NBT 列表的最大元素个数（列表元素为变长类型）。
const maxNBTArrayItems = 500000

// checkNBTArrayLength 校验 NBT 数组长度是否合理：
// 既不能超过读取器剩余字节数（损坏），也不能超过全局上限（防 OOM）。
// 仅用于定长元素数组（ByteArray/IntArray/LongArray），List 用 maxNBTArrayItems。
func checkNBTArrayLength(r *bytes.Reader, length, elemSize int) error {
	if length < 0 {
		return fmt.Errorf("negative NBT array length %d", length)
	}
	if length*elemSize > maxNBTArrayBytes {
		return fmt.Errorf("NBT array too large (%d elems x %d bytes), exceeded %d bytes", length, elemSize, maxNBTArrayBytes)
	}
	// 定长数组：长度不能超过剩余字节
	if length*elemSize > r.Len() {
		return fmt.Errorf("NBT array length %d exceeds remaining %d bytes", length*elemSize, r.Len())
	}
	return nil
}

func toIntAny(v any) int32 {
	switch val := v.(type) {
	case int32:
		return val
	case int16:
		return int32(val)
	case byte:
		return int32(val)
	case float64:
		return int32(val)
	case int:
		return int32(val)
	}
	return 0
}

func decodeSub(data []byte) []uint32 {
	if len(data) < 2 {
		return nil
	}
	pos := 0
	ver := data[pos]
	pos++
	if ver < 1 || ver > 10 {
		return nil
	}
	plen, n := readVu(data[pos:])
	if n <= 0 || plen > 4096 {
		return nil
	}
	pos += n
	if plen == 0 {
		return make([]uint32, 4096)
	}
	pal := make([]uint32, plen)
	for i := 0; i < int(plen); i++ {
		v, n := readVu(data[pos:])
		if n <= 0 {
			if pos < len(data) && i == 0 && plen == 1 {
				pal[0] = uint32(data[pos])
				pos++
				break
			}
			return make([]uint32, 4096)
		}
		pal[i] = uint32(v)
		pos += n
	}
	if plen == 1 {
		out := make([]uint32, 4096)
		for i := range out {
			out[i] = pal[0]
		}
		return out
	}
	bpw := 1
	for (1 << bpw) < int(plen) {
		bpw++
	}
	out := make([]uint32, 4096)
	bp := 0
	for i := 0; i < 4096; i++ {
		bi := pos + (bp >> 3)
		bo := bp & 7
		if bi >= len(data) {
			break
		}
		var idx uint32
		for b := 0; b < bpw; b++ {
			if bi >= len(data) {
				break
			}
			if data[bi]&(1<<(bo&7)) != 0 {
				idx |= 1 << b
			}
			bo++
			if bo&7 == 0 {
				bi++
			}
		}
		bp += bpw
		if int(idx) < len(pal) {
			out[i] = pal[idx]
		}
	}
	return out
}

// isValidSubchunkData 校验 subchunk 数据中的 paletteCount 是否合法，
// 防止 bedrock-world-operator 库的 decodePalette 因损坏的 paletteCount
// 触发 make([]uint32, huge) → OOM。
// 支持 version 1 (单层) 和 version 8/9 (多层) 的 subchunk 格式。
// isValidBiomeData 校验 biome 数据（paletted storage 序列）中的 paletteCount。
// 每个 storage 格式: [blockSize:1][blockData:uint32[]][paletteCount:4][entries:uint32[]]
func isValidBiomeData(data []byte) bool {
	if len(data) == 0 {
		return true // 无 biome 数据，跳过
	}
	pos := 0
	for pos < len(data) {
		if pos+1 > len(data) {
			return false
		}
		bpw := int(data[pos] >> 1) // block size (bits per word)
		pos++
		if bpw == 0x7f {
			// 0x7f = "指向上一个 storage"
			continue
		}
		// 计算 uint32Count
		uint32Count := 0
		if bpw > 0 {
			indicesPerUint32 := 32 / bpw
			uint32Count = 4096 / indicesPerUint32
			if 32%bpw != 0 {
				uint32Count++
			}
		}
		pos += uint32Count * 4
		if bpw == 0 {
			if pos+4 > len(data) {
				return false
			}
			continue // paletteCount 隐式 1，无 paletteCount 字段
		}
		if pos+4 > len(data) {
			return false
		}
		pc := binary.LittleEndian.Uint32(data[pos : pos+4])
		if pc <= 0 || pc > 4096 {
			return false
		}
		if pos+4+int(pc)*4 > len(data) {
			return false
		}
		pos += 4 + int(pc)*4
	}
	return true
}

func isValidSubchunkData(data []byte) bool {
	if len(data) < 2 {
		return false
	}
	ver := data[0]
	pos := 1

	switch {
	case ver == 1:
		// 单层格式: [version:1][blockSize:1][blockData:uint32[]][paletteCount:4][entries:uint32[]]
		if pos+1 >= len(data) {
			return false
		}
		bpw := int(data[pos] >> 1) // block size (bits per word)
		pos++
		if !checkPaletteCount(data, &pos, bpw) {
			return false
		}

	case ver == 8 || ver == 9:
		// 多层格式: [version:1][storageCount:1][subIndex:1(ver9)][storages...]
		if pos >= len(data) {
			return false
		}
		storageCount := int(data[pos])
		pos++
		if ver == 9 {
			pos++ // skip subchunk index byte
		}
		for i := 0; i < storageCount; i++ {
			if pos+1 >= len(data) {
				return false
			}
			bpw := int(data[pos] >> 1)
			pos++
			if !checkPaletteCount(data, &pos, bpw) {
				return false
			}
		}

	default:
		// 未知版本，跳过校验
		return true
	}
	return true
}

// checkPaletteCount 读取并校验 paletteCount。
// 返回 false 如果 paletteCount > 4096 或超出数据边界。
func checkPaletteCount(data []byte, pos *int, bpw int) bool {
	if bpw < 0 || bpw > 32 {
		return false
	}
	// 计算 uint32Count (同 library 的 paletteSize.uint32s())
	uint32Count := 0
	if bpw > 0 {
		indicesPerUint32 := 32 / bpw
		uint32Count = 4096 / indicesPerUint32
		// 非2的幂需要填充 (padded)
		if 32%bpw != 0 {
			uint32Count++
		}
	}
	// 跳过 block data
	*pos += uint32Count * 4
	if bpw == 0 {
		// bpw==0 时 paletteCount 隐式为 1，数据中不写 paletteCount。
		// 只需确保至少还有 1 个 palette entry (4 字节)。
		return *pos+4 <= len(data)
	}
	if *pos+4 > len(data) {
		return false
	}
	// 读取 paletteCount
	pc := int(binary.LittleEndian.Uint32(data[*pos : *pos+4]))
	if pc <= 0 || pc > 4096 {
		return false
	}
	// 校验 palette entries 不超出数据
	if *pos+4+pc*4 > len(data) {
		return false
	}
	return true
}

func readVu(data []byte) (uint32, int) {
	var r uint32
	var s uint
	for i := 0; i < len(data) && i < 5; i++ {
		b := data[i]
		r |= uint32(b&0x7F) << s
		if b&0x80 == 0 {
			return r, i + 1
		}
		s += 7
	}
	return 0, -1
}

var mcRegionRe = regexp.MustCompile(`-?\d+`)

func parseMCWorldRegion(filename string) [6]int {
	var r [6]int
	rest := filename
	for i := 0; i < 2; i++ {
		start := strings.IndexByte(rest, '[')
		end := strings.IndexByte(rest, ']')
		if start < 0 || end <= start {
			return parseByRegex(filename)
		}
		coords := rest[start+1 : end]
		parts := strings.FieldsFunc(coords, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
		if len(parts) != 3 {
			return parseByRegex(filename)
		}
		for j := 0; j < 3; j++ {
			r[i*3+j], _ = strconv.Atoi(strings.TrimSpace(parts[j]))
		}
		rest = rest[end+1:]
	}
	return r
}

func parseByRegex(filename string) [6]int {
	matches := mcRegionRe.FindAllString(filename, -1)
	if len(matches) < 6 {
		return [6]int{}
	}
	nums := matches[len(matches)-6:]
	var r [6]int
	for i := 0; i < 6; i++ {
		r[i], _ = strconv.Atoi(nums[i])
	}
	return r
}

// blockNameFromNBT 从 Bedrock 方块实体的 NBT id 推导 minecraft 方块名。
func blockNameFromNBT(nbt map[string]any) string {
	id, _ := nbt["id"].(string)
	switch id {
	case "Chest": return "minecraft:chest"
	case "EnderChest": return "minecraft:ender_chest"
	case "ShulkerBox": return "minecraft:shulker_box"
	case "CommandBlock": return "minecraft:command_block"
	case "Furnace": return "minecraft:furnace"
	case "BlastFurnace": return "minecraft:blast_furnace"
	case "Smoker": return "minecraft:smoker"
	case "Barrel": return "minecraft:barrel"
	case "Dispenser": return "minecraft:dispenser"
	case "Dropper": return "minecraft:dropper"
	case "Hopper": return "minecraft:hopper"
	case "BrewingStand": return "minecraft:brewing_stand"
	case "Beacon": return "minecraft:beacon"
	case "Sign": return "minecraft:standing_sign"
	case "EnchantTable": return "minecraft:enchanting_table"
	case "MobSpawner": return "minecraft:mob_spawner"
	case "Music": return "minecraft:noteblock"
	case "Jukebox": return "minecraft:jukebox"
	case "Bed": return "minecraft:bed"
	case "Cauldron": return "minecraft:cauldron"
	case "Comparator": return "minecraft:comparator"
	case "DaylightDetector": return "minecraft:daylight_detector"
	case "FlowerPot": return "minecraft:flower_pot"
	case "StructureBlock": return "minecraft:structure_block"
	case "NetherReactor": return "minecraft:netherreactor"
	case "Lodestone": return "minecraft:lodestone"
	case "Beehive": return "minecraft:beehive"
	case "BeeNest": return "minecraft:bee_nest"
	case "Bell": return "minecraft:bell"
	case "Campfire": return "minecraft:campfire"
	case "SoulCampfire": return "minecraft:soul_campfire"
	case "Lectern": return "minecraft:lectern"
	case "MovingBlock": return "minecraft:moving_block"
	case "Conduit": return "minecraft:conduit"
	case "EndGateway": return "minecraft:end_gateway"
	case "ItemFrame": return "minecraft:item_frame"
	case "GlowItemFrame": return "minecraft:glow_item_frame"
	case "PistonArm": return "minecraft:piston"
	case "SuspiciousStew": return "minecraft:suspicious_stew"
	case "ChiseledBookshelf": return "minecraft:chiseled_bookshelf"
	case "DecoratedPot": return "minecraft:decorated_pot"
	case "CalibratedSculkSensor": return "minecraft:calibrated_sculk_sensor"
	case "SculkSensor": return "minecraft:sculk_sensor"
	case "SculkShrieker": return "minecraft:sculk_shrieker"
	case "SculkCatalyst": return "minecraft:sculk_catalyst"
	case "BrushableBlock": return "minecraft:suspicious_sand"
	case "Vault": return "minecraft:vault"
	case "Crafter": return "minecraft:crafter"
	case "TrialSpawner": return "minecraft:trial_spawner"
	default: return ""
	}
}

func statesToString(props map[string]any) string {
	if len(props) == 0 {
		return "[]"
	}
	var parts []string
	for k, v := range props {
		// 跳过运行时状态（由游戏自动设置，setblock 不接受）
		if k == "grass_block" || k == "snowy" {
			continue
		}
		switch val := v.(type) {
		case bool:
			if val {
				parts = append(parts, fmt.Sprintf("%q=true", k))
			} else {
				parts = append(parts, fmt.Sprintf("%q=false", k))
			}
		case byte:
			if isBoolState(k) {
				if val != 0 {
					parts = append(parts, fmt.Sprintf("%q=true", k))
				} else {
					parts = append(parts, fmt.Sprintf("%q=false", k))
				}
			} else {
				parts = append(parts, fmt.Sprintf("%q=%d", k, val))
			}
		case string:
			parts = append(parts, fmt.Sprintf("%q=%q", k, val))
		default:
			parts = append(parts, fmt.Sprintf("%q=%v", k, val))
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func detectMCWorld(path string) bool {
	r, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer r.Close()
	hasLD, hasDB := false, false
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "level.dat") {
			hasLD = true
		}
		if strings.HasPrefix(f.Name, "db/") {
			hasDB = true
		}
	}
	return hasLD && hasDB
}
