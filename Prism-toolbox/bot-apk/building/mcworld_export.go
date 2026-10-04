package building

import (
	"fmt"
	"os"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
)

// WriteMergedStructureToMCWorld 将 mergeChunkStructures 合并后的结构写入 .mcworld 文件。
// 使用 chunk.Chunk.SetBlock + world.SaveChunk，与 SimpleWorldExporter Python 版一致。
func WriteMergedStructureToMCWorld(mergedStructure map[string]any, totalW, totalH, totalL int32, worldPath string, originX, originY, originZ int32) error {
	if mergedStructure == nil {
		return fmt.Errorf("no structure data to write")
	}

	tmpDir, err := os.MkdirTemp("/data/data/com.prismtool.box/files", "prism_export_")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	w, err := world.Open(tmpDir, nil)
	if err != nil {
		return fmt.Errorf("open world: %w", err)
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
		return fmt.Errorf("level.dat: %w", err)
	}

	blockIndicesRaw, _ := mergedStructure["block_indices"].([]any)
	paletteRaw, _ := mergedStructure["palette"].(map[string]any)
	if blockIndicesRaw == nil || paletteRaw == nil {
		w.CloseWorld()
		return fmt.Errorf("invalid merged structure: missing block_indices or palette")
	}

	// 解析 block_palette
	var paletteNames []string
	var paletteStates []map[string]any
	if pd, ok := paletteRaw["default"].(map[string]any); ok {
		if bp, ok2 := pd["block_palette"].([]any); ok2 {
			for _, entry := range bp {
				if em, ok3 := entry.(map[string]any); ok3 {
					name, _ := em["name"].(string)
					paletteNames = append(paletteNames, name)
					states, _ := em["states"].(map[string]any)
					paletteStates = append(paletteStates, states)
				}
			}
		}
	}

	// 解析 block_position_data（NBT 方块实体数据）
	blockPosData := make(map[int]map[string]any)
	if pd, ok := paletteRaw["default"].(map[string]any); ok {
		if bpd, ok2 := pd["block_position_data"].(map[string]any); ok2 {
			for keyStr, value := range bpd {
				valMap, ok := value.(map[string]any)
				if !ok {
					continue
				}
				entityData, ok := valMap["block_entity_data"].(map[string]any)
				if !ok {
					continue
				}
				var keyInt int
				fmt.Sscanf(keyStr, "%d", &keyInt)
				blockPosData[keyInt] = entityData
			}
		}
	}

	// 计算 air runtime ID
	airRID, _ := block.StateToRuntimeID("minecraft:air", nil)
	dim := define.Dimension(define.DimensionIDOverworld)
	dimRange := dim.Range()

	chunkSize := int32(16)
	chunksX := (totalW + chunkSize - 1) / chunkSize
	chunksZ := (totalL + chunkSize - 1) / chunkSize
	layer0 := toIntSliceLocal(blockIndicesRaw[0])

	// 逐区块处理
	for cx := int32(0); cx < chunksX; cx++ {
		for cz := int32(0); cz < chunksZ; cz++ {
			chunkPos := define.ChunkPos{originX>>4 + cx, originZ>>4 + cz}
			// 计算区块在建筑相对坐标中的范围
			// 建筑原点可能不在区块边界上，需要对齐
			chunkStartX := (originX>>4 + cx) * 16
			chunkStartZ := (originZ>>4 + cz) * 16
			startX := chunkStartX - originX
			startZ := chunkStartZ - originZ
			if startX < 0 { startX = 0 }
			if startZ < 0 { startZ = 0 }
			endX := startX + 16
			endZ := startZ + 16
			if endX > totalW { endX = totalW }
			if endZ > totalL { endZ = totalL }

			// 创建新 chunk
			c := chunk.NewChunk(airRID, dimRange)
			if c == nil {
				continue
			}

			// 设置方块
			for y := int32(0); y < totalH; y++ {
				absY := originY + y
				for z := startZ; z < endZ; z++ {
					for x := startX; x < endX; x++ {
						linearIdx := int(x*totalH*totalL + y*totalL + z)
						if linearIdx >= len(layer0) {
							continue
						}
						pi := layer0[linearIdx]
						if pi < 0 || pi >= len(paletteNames) {
							continue
						}
						name := paletteNames[pi]
						if name == "minecraft:air" || name == "minecraft:structure_void" {
							continue
						}
													rid, found := block.StateToRuntimeID(name, paletteStates[pi])
if !found {
	rid, found = block.StateToRuntimeID(name, nil)
}
						if !found {
							continue
						}
						// chunk.SetBlock 接受绝对 Y 坐标（int16），自动处理子区块映射
						c.SetBlock(uint8(originX+x-chunkStartX), int16(absY), uint8(originZ+z-chunkStartZ), 0, rid)
					}
				}
			}

			// 逐子区块保存（与 RunSubChunkExport 一致）
			for i, sub := range c.Sub() {
				if sub == nil || sub.Empty() {
					continue
				}
				subY := c.SubY(int16(i)) >> 4
				subPos := define.SubChunkPos{chunkPos[0], int32(subY), chunkPos[1]}
				if err := w.SaveSubChunk(dim, subPos, sub); err != nil {
					w.CloseWorld()
					return fmt.Errorf("save subchunk (%d,%d,%d): %w", chunkPos[0], subY, chunkPos[1], err)
				}
			}

			// 收集并保存 NBT 方块实体
			var chunkNBTs []map[string]any
			for linearIdx, entityData := range blockPosData {
				x := linearIdx / int(totalH*totalL)
				rem := linearIdx % int(totalH*totalL)
				y := rem / int(totalL)
				z := rem % int(totalL)
				if int32(x) >= startX && int32(x) < endX && int32(z) >= startZ && int32(z) < endZ {
					blkName := "minecraft:air"
					if linearIdx < len(layer0) {
						pi := layer0[linearIdx]
						if pi >= 0 && pi < len(paletteNames) {
							blkName = paletteNames[pi]
						}
					}
					entity := make(map[string]any)
					entity["x"] = originX + int32(x)
					entity["y"] = originY + int32(y)
					entity["z"] = originZ + int32(z)
					entity["id"] = stripMinecraftPrefix(blkName)
					for k, v := range entityData {
						entity[k] = v
					}
					entity["x"] = originX + int32(x)
					entity["y"] = originY + int32(y)
					entity["z"] = originZ + int32(z)
					chunkNBTs = append(chunkNBTs, entity)
				}
			}
			if len(chunkNBTs) > 0 {
				if err := w.SaveNBT(dim, chunkPos, chunkNBTs); err != nil {
					w.CloseWorld()
					return fmt.Errorf("save nbt (%d,%d): %w", chunkPos[0], chunkPos[1], err)
				}
			}
		}
	}

	os.WriteFile(tmpDir+"/levelname.txt", []byte("prism export"), 0644)

	if err := w.CloseWorld(); err != nil {
		return fmt.Errorf("close world: %w", err)
	}

	return zipDir(tmpDir, worldPath)
}