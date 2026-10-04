package building

import (
	"archive/zip"
	"strings"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
)

func WriteMCWorld(chunks []map[string]any, sizeX, sizeY, sizeZ int32, regionOffset [3]int32, worldPath string) error {
	if len(chunks) == 0 {
		return fmt.Errorf("no chunk data to write")
	}

	tmpDir, err := os.MkdirTemp(filepath.Dir(worldPath), "mcworld_out_")
	if err != nil {
		return fmt.Errorf("WriteMCWorld: temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Use bedrock-world-operator's world.Open (same lib as the reader)
	w, err := world.Open(tmpDir, nil)
	if err != nil {
		return fmt.Errorf("WriteMCWorld: open world: %w", err)
	}

	ldat := w.LevelDat()
	ldat.LevelName = "prism export"
	ldat.GameType = 1
	ldat.RandomSeed = 0
	ldat.SpawnX, ldat.SpawnY, ldat.SpawnZ = 0, 64, 0
	ldat.CommandsEnabled = true
	ldat.HasBeenLoadedInCreative = true
	ldat.MultiPlayerGame = true
	ldat.LANBroadcast = true
	ldat.LANBroadcastIntent = true
	if err := w.UpdateLevelDat(); err != nil {
		w.CloseWorld()
		return fmt.Errorf("WriteMCWorld: level.dat: %w", err)
	}

	chunkSize := int32(16)
	chunksZ := (sizeZ + chunkSize - 1) / chunkSize
	subCountY := (sizeY + chunkSize - 1) / chunkSize
	dim := define.Dimension(define.DimensionIDOverworld)

	for ci, chunkRaw := range chunks {
		structure, _ := chunkRaw["structure"].(map[string]any)
		if structure == nil {
			continue
		}
		blockIndicesRaw, _ := structure["block_indices"].([]any)
		paletteRaw, _ := structure["palette"].(map[string]any)

		cx := int32(ci) / chunksZ
		cz := int32(ci) % chunksZ

		var palette []string
		if pd, ok := paletteRaw["default"].(map[string]any); ok {
			if bp, ok2 := pd["block_palette"].([]any); ok2 {
				for _, entry := range bp {
					if em, ok3 := entry.(map[string]any); ok3 {
						name, _ := em["name"].(string)
						palette = append(palette, name)
					}
				}
			}
		}

		paletteNBT := make(map[int]map[string]any)
		if nbtList, ok := chunkRaw["nbt_blocks"].([]any); ok {
			for _, nbtItem := range nbtList {
				if nm, ok2 := nbtItem.(map[string]any); ok2 {
				pi := nbtIntAny(nm["palette_index"])
					if nbt, ok4 := nm["nbt"].(map[string]any); ok4 {
						paletteNBT[pi] = nbt
					}
				}
			}
		}

		// Write subchunks (collect all first, save once per chunk)
		// Per-chunk dimensions (edge chunks may be smaller than 16)
		chunkSizeX := int32(sizeX - cx*chunkSize)
		if chunkSizeX > chunkSize { chunkSizeX = chunkSize }
		chunkSizeZ := int32(sizeZ - cz*chunkSize)
		if chunkSizeZ > chunkSize { chunkSizeZ = chunkSize }
		chunkStride := chunkSizeX * chunkSizeZ

		subChunks := make([][]byte, subCountY)
		for sy := int32(0); sy < subCountY; sy++ {
			subRTIDs := make([]uint32, 4096)
			hasBlocks := false

			for _, layerRaw := range blockIndicesRaw {
				indices := toIntSliceLocal(layerRaw)
				if indices == nil {
					continue
				}
				for y := int32(0); y < chunkSize; y++ {
					globalY := sy*chunkSize + y
					if globalY >= sizeY {
						break
					}
					for z := int32(0); z < chunkSizeZ; z++ {
						for x := int32(0); x < chunkSizeX; x++ {
							idx := int(x * sizeY * chunkSizeZ + globalY * chunkSizeZ + z)
							if idx >= len(indices) {
								continue
							}
							pi := indices[idx]
							if pi < 0 || pi >= len(palette) {
								continue
							}
							name := palette[pi]
							if name == "minecraft:air" || name == "minecraft:structure_void" {
								continue
							}
							rid, found := block.StateToRuntimeID(name, nil)
							if !found {
								continue
							}
							hasBlocks = true
							subRTIDs[int(y)*256+int(z)*16+int(x)] = rid
						}
					}
				}
			}
			if hasBlocks {
				subChunks[sy] = subChunkPayload(subRTIDs)
			}
		}
		w.SaveChunkPayloadOnly(dim, define.ChunkPos{int32(cx), int32(cz)}, subChunks)

		// Write block entities
		var blockEntities []map[string]any
		for _, layerRaw := range blockIndicesRaw {
			indices := toIntSliceLocal(layerRaw)
			if indices == nil {
				continue
			}
			for y := int32(0); y < sizeY; y++ {
				for z := int32(0); z < chunkSizeZ; z++ {
					for x := int32(0); x < chunkSizeX; x++ {
						idx := int(y*chunkStride + z*chunkSizeX + x)
						if idx >= len(indices) {
							continue
						}
						pi := indices[idx]
						if nbt, ok := paletteNBT[pi]; ok {
							wx := regionOffset[0] + cx*chunkSize + x
							wy := regionOffset[1] + y
							wz := regionOffset[2] + cz*chunkSize + z
							entity := make(map[string]any)
							entity["x"] = wx
							entity["y"] = wy
							entity["z"] = wz
							if pi < len(palette) {
					entity["id"] = stripMinecraftPrefix(palette[pi])
							}
							for k, v := range nbt {
								entity[k] = v
							}
							blockEntities = append(blockEntities, entity)
						}
					}
				}
			}
		}
		if len(blockEntities) > 0 {
			w.SaveNBT(dim, define.ChunkPos{int32(cx), int32(cz)}, blockEntities)
		}
	}

	os.WriteFile(filepath.Join(tmpDir, "levelname.txt"), []byte("prism export"), 0644)

	if err := w.CloseWorld(); err != nil {
		return fmt.Errorf("WriteMCWorld: close: %w", err)
	}

	return zipDir(tmpDir, worldPath)
}

// subChunkPayload encodes a 16x16x16 subchunk of runtime IDs into the LevelDB storage format.
func subChunkPayload(rtIDs []uint32) []byte {
	// Count unique non-air IDs
	idMap := make(map[uint32]int)
	idMap[block.AirRuntimeID] = 0
	palette := []uint32{block.AirRuntimeID}
	for _, rid := range rtIDs {
		if _, ok := idMap[rid]; !ok {
			idMap[rid] = len(palette)
			palette = append(palette, rid)
		}
	}

	bitsPerBlock := 1
	for (1 << bitsPerBlock) < len(palette) {
		bitsPerBlock++
	}

	buf := make([]byte, 0, 512)
	buf = append(buf, 8) // version
	buf = append(buf, 1) // storage count

	// palette header: (bits_per_block << 1) | 1 (use runtime IDs)
	buf = append(buf, byte((bitsPerBlock<<1)|1))

	var i32 [4]byte
	// palette size
	le32(i32[:], uint32(len(palette)))
	buf = append(buf, i32[:]...)
	// palette entries
	for _, rid := range palette {
		le32(i32[:], rid)
		buf = append(buf, i32[:]...)
	}

	// packed block indices
	wordCount := (4096*bitsPerBlock + 31) / 32
	words := make([]uint32, wordCount)
	for i, rid := range rtIDs {
		idx := idMap[rid]
		bitOff := i * bitsPerBlock
		wordIdx := bitOff / 32
		bitInWord := bitOff % 32
		words[wordIdx] |= uint32(idx) << bitInWord
		if bitInWord+bitsPerBlock > 32 {
			words[wordIdx+1] |= uint32(idx) >> (32 - bitInWord)
		}
	}
	for _, w := range words {
		le32(i32[:], w)
		buf = append(buf, i32[:]...)
	}

	return buf
}

func le32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func zipDir(srcDir, dstPath string) error {
	f, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(srcDir, path)
		zf, err := w.Create(rel)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(zf, src)
		return err
	})
}

func toIntSliceLocal(v any) []int {
	switch arr := v.(type) {
	case []any:
		r := make([]int, len(arr))
		for i, x := range arr {
			switch n := x.(type) {
			case int32:
				r[i] = int(n)
			case int:
				r[i] = n
			case float64:
				r[i] = int(n)
			}
		}
		return r
	case []int32:
		r := make([]int, len(arr))
		for i, n := range arr {
			r[i] = int(n)
		}
		return r
	}
	return nil
}



func stripMinecraftPrefix(name string) string {
	// LevelDB block entity id field uses simple names: "Chest", "Sign", etc.
	trimmed := strings.Replace(name, "minecraft:", "", 1)
	parts := strings.Split(trimmed, ":")
	return parts[0]
}

func nbtIntAny(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int32:
		return int(val)
	case float64:
		return int(val)
	}
	return 0
}
