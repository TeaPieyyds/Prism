package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/nbt"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
)

// SubChunkExportTask 通过 SubChunkRequest 协议导出区域方块数据，写入 LevelDB .mcworld 文件。
// 参考 NexusEgo 的 ExportMCWorldOptimized 实现，补充了 Prism 缺失的关键逻辑：
//   - 区块半径协商（RequestChunkRadius → ChunkRadiusUpdated）
//   - 机器人移动到目标区域中心（确保区块加载）
//   - ChunkNotFound 子区块重试（不静默丢弃）
//   - 包监听器生命周期隔离（每次请求新建/销毁）
type SubChunkExportTask struct {
	bm         *BotManager
	minX, maxX int32
	minY, maxY int32
	minZ, maxZ int32
	outputPath string
	dimension  int32
	onProgress func(current, total int)
}

// RunSubChunkExport 创建并执行导出任务。
func RunSubChunkExport(bm *BotManager, minX, maxX, minY, maxY, minZ, maxZ int32, outputPath string, dimension int32, onProgress func(int, int)) error {
	t := &SubChunkExportTask{
		bm:         bm,
		minX:       minX, maxX: maxX,
		minY:       minY, maxY: maxY,
		minZ:       minZ, maxZ: maxZ,
		outputPath: outputPath,
		dimension:  dimension,
		onProgress: onProgress,
	}
	return t.Run()
}

func (t *SubChunkExportTask) Run() error {
	chunkMinX, chunkMaxX := t.minX>>4, t.maxX>>4
	chunkMinZ, chunkMaxZ := t.minZ>>4, t.maxZ>>4
	chunksX := chunkMaxX - chunkMinX + 1
	chunksZ := chunkMaxZ - chunkMinZ + 1
	totalChunks := int(chunksX * chunksZ)

	// 确保 gameInterface 已初始化（懒加载）
	if _, err := t.bm.getGameInterface(); err != nil {
		return fmt.Errorf("game interface 未就绪: %w", err)
	}

	// 协商区块半径，确保服务器愿意发送覆盖导出区域的数据
	if err := t.requestChunkRadius(); err != nil {
		return fmt.Errorf("区块半径协商失败: %w", err)
	}

	tmpDir, err := os.MkdirTemp("/data/data/com.prismtool.box/files", "prism_export_*")
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
	ldat.CommandsEnabled = true
	if err := w.UpdateLevelDat(); err != nil {
		w.CloseWorld()
		return fmt.Errorf("level.dat: %w", err)
	}

	dim := define.Dimension(t.dimension)
	dimRange := dim.Range()
	processed := 0

	for cx := int32(0); cx < chunksX; cx++ {
		for cz := int32(0); cz < chunksZ; cz++ {
			absCX := chunkMinX + cx
			absCZ := chunkMinZ + cz

			if err := t.moveToChunkCenter(absCX, absCZ, 500); err != nil {
				return err
			}

			subs, nbtList, err := t.fetchSubChunks(absCX, absCZ, dimRange)
			if err != nil {
				return fmt.Errorf("chunk (%d,%d): %w", absCX, absCZ, err)
			}

			// 保存子区块数据
			for i, sub := range subs {
				if sub == nil {
					continue
				}
				subY := t.minY>>4 + int32(i)
				if subY > t.maxY>>4 {
					break
				}
				subPos := define.SubChunkPos{absCX, subY, absCZ}

				if err := w.SaveSubChunk(dim, subPos, sub); err != nil {
					return fmt.Errorf("save subchunk (%d,%d,%d): %w", absCX, subY, absCZ, err)
				}
			}

			// 保存 NBT 方块实体数据
			if len(nbtList) > 0 {
				chunkPos := define.ChunkPos{absCX, absCZ}
				if err := w.SaveNBT(dim, chunkPos, nbtList); err != nil {
					return fmt.Errorf("save nbt (%d,%d): %w", absCX, absCZ, err)
				}
			}

			processed++
			if t.onProgress != nil {
				t.onProgress(processed, totalChunks)
			}
		}
	}

	if err := w.CloseWorld(); err != nil {
		return err
	}
	return exportZipDir(tmpDir, t.outputPath)
}

// requestChunkRadius 与服务器协商区块半径，确保导出区域在服务器愿意发送的范围内。
// 请求半径为覆盖导出区域所需的最小值（最小 8，最大 32），实际以服务器返回值为准。
func (t *SubChunkExportTask) requestChunkRadius() error {
	// 计算覆盖导出区域所需的区块半径
	chunkMinX, chunkMaxX := t.minX>>4, t.maxX>>4
	chunkMinZ, chunkMaxZ := t.minZ>>4, t.maxZ>>4
	// 计算以 0,0 为中心时需要的半径来覆盖整个区域
	needX := chunkMaxX
	if -chunkMinX > needX { needX = -chunkMinX }
	needZ := chunkMaxZ
	if -chunkMinZ > needZ { needZ = -chunkMinZ }
	desired := int32(8)
	if needX > desired { desired = needX }
	if needZ > desired { desired = needZ }
	desired += 2 // 留余量
	if desired < 8 { desired = 8 }
	if desired > 32 { desired = 32 }

	gi, err := t.bm.getGameInterface()
	if err != nil {
		return err
	}

	respCh := make(chan int32, 1)
	lid, err := gi.PacketListener().ListenPacket(
		[]uint32{packet.IDChunkRadiusUpdated},
		func(pk packet.Packet, connErr error) {
			if connErr != nil { return }
			if cr, ok := pk.(*packet.ChunkRadiusUpdated); ok {
				select {
				case respCh <- cr.ChunkRadius:
				default:
				}
			}
		},
	)
	if err != nil {
		return fmt.Errorf("监听 ChunkRadiusUpdated: %w", err)
	}
	defer gi.PacketListener().DestroyListener(lid)

	if err := t.bm.WritePacket(&packet.RequestChunkRadius{
		ChunkRadius:    desired,
		MaxChunkRadius: desired,
	}); err != nil {
		return fmt.Errorf("发送 RequestChunkRadius: %w", err)
	}

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case granted := <-respCh:
		t.bm.logCh <- fmt.Sprintf("[导出] 区块半径协商: 请求=%d 授予=%d", desired, granted)
		return nil
	case <-timer.C:
		// 超时不致命——服务器可能不回复这个包，继续导出
		t.bm.logCh <- fmt.Sprintf("[导出] 区块半径协商超时，使用默认值继续")
		return nil
	}
}

// moveToChunkCenter 将机器人 TP 到指定 chunk 中心并等待区块加载。
func (t *SubChunkExportTask) moveToChunkCenter(cx, cz int32, sleepMs int) error {
	centerX := cx*16 + 8
	centerZ := cz*16 + 8
	centerY := (t.minY + t.maxY) / 2
	if err := t.bm.TP(int(centerX), int(centerY), int(centerZ)); err != nil {
		return fmt.Errorf("移动机器人到 (%d,%d,%d): %w", centerX, centerY, centerZ, err)
	}
	time.Sleep(time.Duration(sleepMs) * time.Millisecond)
	return nil
}

// fetchSubChunks 发送 SubChunkRequest 获取一个 chunk 列中所有子区块的数据。
// 对 ChunkNotFound 的子区块自动重试，直到全部获取或达到重试上限。
func (t *SubChunkExportTask) fetchSubChunks(chunkX, chunkZ int32, dimRange define.Range) ([]*chunk.SubChunk, []map[string]any, error) {
	subMinY := t.minY >> 4
	subMaxY := t.maxY >> 4

	// 构建请求位置列表
	var positions []protocol.SubChunkPos
	for y := subMinY; y <= subMaxY; y++ {
		positions = append(positions, protocol.SubChunkPos{chunkX, y, chunkZ})
	}
	if len(positions) == 0 {
		return nil, nil, nil
	}

	// 首次请求
	subs, allNBTs, missing, err := t.fetchSubChunksOnce(chunkX, chunkZ, subMinY, subMaxY, positions, dimRange)
	if err != nil {
		return nil, nil, err
	}

	// 对 ChunkNotFound 的子区块重试（最多 5 轮，有收敛检测）
	retryCount := 0
	prevMissing := len(missing)
	for len(missing) > 0 && retryCount < 5 {
		// 收敛检测：缺失数量没减少则退出
		if len(missing) == prevMissing && retryCount > 0 {
			t.bm.logCh <- fmt.Sprintf("[导出] chunk (%d,%d): %d 个子区块持续缺失，跳过", chunkX, chunkZ, len(missing))
			break
		}
		prevMissing = len(missing)

		retryCount++
		time.Sleep(300 * time.Millisecond)

		// 重试时重新移动机器人到中心
		t.moveToChunkCenter(chunkX, chunkZ, 400)

		retrySubs, retryNBTs, stillMissing, retryErr := t.fetchSubChunksOnce(chunkX, chunkZ, subMinY, subMaxY, missing, dimRange)
		if retryErr != nil {
			t.bm.logCh <- fmt.Sprintf("[导出] chunk (%d,%d) 重试%d失败: %v", chunkX, chunkZ, retryCount, retryErr)
			break
		}

		// 合并结果
		for i, sub := range retrySubs {
			if sub != nil {
				subs[i] = sub
			}
		}
		allNBTs = append(allNBTs, retryNBTs...)
		missing = stillMissing
	}

	if len(missing) > 0 {
		t.bm.logCh <- fmt.Sprintf("[导出] chunk (%d,%d): 最终 %d 个子区块未获取（空气）", chunkX, chunkZ, len(missing))
	}

	return subs, allNBTs, nil
}

// fetchSubChunksOnce 发送一次 SubChunkRequest 并解析响应。
// 返回已解码的子区块列表、NBT 数据列表、和 ChunkNotFound 的位置列表。
func (t *SubChunkExportTask) fetchSubChunksOnce(chunkX, chunkZ, subMinY, subMaxY int32, positions []protocol.SubChunkPos, dimRange define.Range) ([]*chunk.SubChunk, []map[string]any, []protocol.SubChunkPos, error) {
	if len(positions) == 0 {
		return nil, nil, nil, nil
	}

	base := positions[0]
	var offsets []protocol.SubChunkOffset
	for _, pos := range positions {
		dx := int8(pos.X() - base.X())
		dy := int8(pos.Y() - base.Y())
		dz := int8(pos.Z() - base.Z())
		offsets = append(offsets, protocol.SubChunkOffset{dx, dy, dz})
	}

	if len(offsets) > 256 {
		return nil, nil, nil, fmt.Errorf("too many sub-chunks (%d)", len(offsets))
	}

	gi, err := t.bm.getGameInterface()
	if err != nil {
		return nil, nil, nil, err
	}

	// 每次请求创建全新的监听器，用完即销毁，避免跨请求串包
	respCh := make(chan *subChunkResult, 1)
	listenerID, err := gi.PacketListener().ListenPacket(
		[]uint32{packet.IDSubChunk},
		func(pk packet.Packet, connErr error) {
			if connErr != nil {
				select {
				case respCh <- &subChunkResult{err: connErr}:
				default:
				}
				return
			}
			sub, ok := pk.(*packet.SubChunk)
			if !ok {
				return
			}
			if sub.Dimension != t.dimension {
				return
			}
			if sub.Position != base {
				return
			}
			select {
			case respCh <- &subChunkResult{pk: sub}:
			default:
			}
		},
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("listener: %w", err)
	}
	defer gi.PacketListener().DestroyListener(listenerID)

	if err := t.bm.WritePacket(&packet.SubChunkRequest{
		Dimension: t.dimension,
		Position:  base,
		Offsets:   offsets,
	}); err != nil {
		return nil, nil, nil, err
	}

	// 等待响应
	var resp *packet.SubChunk
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case r := <-respCh:
		if r.err != nil {
			return nil, nil, nil, r.err
		}
		resp = r.pk
	case <-timer.C:
		return nil, nil, nil, fmt.Errorf("subchunk timeout at (%d,%d)", chunkX, chunkZ)
	}

	if resp == nil {
		return nil, nil, nil, fmt.Errorf("no response for (%d,%d)", chunkX, chunkZ)
	}

	return decodeSubs(resp, base, subMinY, subMaxY, dimRange)
}

// subChunkResult 封装监听器回调结果。
type subChunkResult struct {
	pk  *packet.SubChunk
	err error
}

// entrySubChunkPos 根据基点和偏移计算子区块绝对位置。
func entrySubChunkPos(base protocol.SubChunkPos, entry protocol.SubChunkEntry) protocol.SubChunkPos {
	return protocol.SubChunkPos{
		base.X() + int32(entry.Offset[0]),
		base.Y() + int32(entry.Offset[1]),
		base.Z() + int32(entry.Offset[2]),
	}
}

// decodeSubs 解码 SubChunk 响应条目。
// 返回：已解码的子区块切片（按 Y 层索引）、NBT 方块实体列表、ChunkNotFound 的子区块位置列表。
func decodeSubs(resp *packet.SubChunk, base protocol.SubChunkPos, subMinY, subMaxY int32, dimRange define.Range) ([]*chunk.SubChunk, []map[string]any, []protocol.SubChunkPos, error) {
	subCount := subMaxY - subMinY + 1
	subs := make([]*chunk.SubChunk, subCount)
	var allNBTs []map[string]any
	var missing []protocol.SubChunkPos

	for _, entry := range resp.SubChunkEntries {
		entryY := base.Y() + int32(entry.Offset[1])
		relY := entryY - subMinY
		if relY < 0 || relY >= subCount {
			continue
		}

		switch entry.Result {
		case protocol.SubChunkResultSuccessAllAir:
			// 全空气，保持 nil（SaveSubChunk 会正确处理）
			continue

		case protocol.SubChunkResultChunkNotFound,
			protocol.SubChunkResultInvalidDimension,
			protocol.SubChunkResultPlayerNotFound,
			protocol.SubChunkResultIndexOutOfBounds:
			// 所有非成功结果都加入重试列表
			missing = append(missing, entrySubChunkPos(base, entry))
			continue

		case protocol.SubChunkResultSuccess:
			if len(entry.RawPayload) == 0 {
				missing = append(missing, entrySubChunkPos(base, entry))
				continue
			}

			// 解码网络编码的子区块
			buf := bytes.NewBuffer(entry.RawPayload)
			sub, _, err := chunk.DecodeSubChunk(buf, dimRange, chunk.NetworkEncoding)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("decode subchunk Y=%d: %w", entryY, err)
			}
			subs[relY] = sub

			// 解码子区块数据末尾附加的 NBT 方块实体数据（NetworkLittleEndian）
			for buf.Len() > 0 {
				var blockData map[string]any
				if err := nbt.NewDecoderWithEncoding(buf, nbt.NetworkLittleEndian).Decode(&blockData); err != nil {
					break
				}
				if blockData != nil {
					allNBTs = append(allNBTs, blockData)
				}
			}
		}
	}

	// 从包末尾的 BlockEntities 中解码 NBT 方块实体数据（NetworkLittleEndian）
	// 这些数据在协议包的所有子区块条目之后，之前被协议库丢弃了
	if len(resp.BlockEntities) > 0 {
		buf := bytes.NewBuffer(resp.BlockEntities)
		for buf.Len() > 0 {
			var blockData map[string]any
			if err := nbt.NewDecoderWithEncoding(buf, nbt.NetworkLittleEndian).Decode(&blockData); err != nil {
				break
			}
			if blockData != nil {
				allNBTs = append(allNBTs, blockData)
			}
		}
	}

	return subs, allNBTs, missing, nil
}

// exportZipDir 将目录打包为 .mcworld 文件。
func exportZipDir(srcDir, dstPath string) error {
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
