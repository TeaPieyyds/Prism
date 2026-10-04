package building

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ChunkSize = 16

// Bedrock 的 fill 存在体积上限。使用 32766 作为安全上限。
const maxFillVolume = 32766

const (
	placeNormal    = 0 // 普通方块（跳过重力和依赖）
	placeGravity   = 1 // 仅重力方块
	placeDependent = 2 // 仅依赖方块
)

type SetBlockFunc func(x, y, z int32, name, states string) error
type FillFunc func(x1, y1, z1, x2, y2, z2 int32, name, states string) error
type TPFunc func(x, y, z int) error

type ImportTask struct {
	Data               *StructureData
	BaseX              int
	BaseY              int
	BaseZ              int
	Speed              int
	StartCX            int
	StartCZ            int
	SetBlock           SetBlockFunc
	FillRegion         FillFunc
	Teleport           TPFunc
	OnNBT              func(x, y, z int32, nbt map[string]any, blockName, blockStates string)
	OnProgress         func(cx, cz, chunk, total, blocks int)
	OnSubProgress      func(cx, cz, placed, totalSolid int)
	OnRegionProgress   func(ri, totalRegions, rx, rz int)
	ChunkLoaded        func(cx, cz int) bool // 区块加载检测，返回 true=已加载
	StopCh             <-chan struct{}
	ImportCommands     bool
	CmdDisabled        bool
	ExcludeWater       bool
	ExcludeWaterlogged bool
	ExcludeLava        bool

	// RegionMode: 1=16×16(单区块), 2=32×32(2×2), 3=64×64(4×4)
	RegionMode int

	// Deny layer: placed at baseY+DenyYOffset before each region
	DenyEnable  bool
	DenyYOffset int

	// Border: at edge positions, placed as part of region import
	BorderEnable bool
	BorderBlock  string
	BorderYRel   int // relative to BaseY

	// Pre-clear: clear area before import
	// 0=off  1=building-box  2=base-to-sky  3=full-chunks
	PreClearMode int
	Dimension    string // "overworld" / "nether" / "the_end"

	// Callbacks for single-row clear progress
	OnClearProgress func(cleared, totalBlocks int)
	OnClearDone     func()

	// Pre-computed counts for accurate progress
	TotalSolidBlocks int
	chunkSolid       map[[2]int]int
	regionSolid      map[[2]int]int
	totalRegions     int

	// StopOnError: 当 SetBlock/FillRegion/Teleport 返回错误时停止导入
	StopOnError bool

	// importErr 记录 StopOnError 模式下的第一个错误
	importErr error

	// sleepAccum 累积子毫秒睡眠时间，用于精确速率控制
	sleepAccum time.Duration

	// ── 修补模式 ──
	// 修补模式用于导入完成后发现缺失方块的场景，复用首次导入的缓存文件，
	// 只重新放置指定区域内的方块。修补模式不预清空、不铺 deny、不铺 border。
	RepairMode bool
	// RepairNBTOnly 指示仅修补 NBT 方块（命令方块/容器/告示牌等），
	// 跳过无 NBT 数据的普通方块。仅 RepairMode 为真时生效。
	RepairNBTOnly bool
	// RepairRect 指示矩形修补范围（世界坐标）。当 RepairUseCircle 为假时生效。
	// 格式: [minX, minY, minZ, maxX, maxY, maxZ]
	RepairRect [6]int32
	// RepairUseCircle 指示是否使用圆形（区块对齐）修补模式。
	RepairUseCircle bool
	// RepairCenterX / RepairCenterZ 指示圆形修补的中心（世界坐标）。
	RepairCenterX int32
	RepairCenterZ int32
	// RepairRadius 指示圆形修补的半径（方块数）。
	RepairRadius int32

	// ── 多机器人单元模式（S2） ──
	// PlaceOnly 非 nil 时：只放置这些 16×16 区块列（单元模式，强制单区块粒度）。
	PlaceOnly map[[2]int]bool
	// SkipPositions 非 nil 时：跳过这些位置（普通单元需跳过被抠出的工作区NBT和告示牌）。
	SkipPositions map[[3]int]bool
	// SkipPreClear 为 true 时跳过预清空（多机器人预清空由一台机器人先整图做一次，S2/S7）。
	SkipPreClear bool
	// IgnoreNBTInUnit 单元模式：普通单元里带 NBT 的方块（磁石、花盆、蜂巢等简单方块实体）
	// 直接按纯普通方块放置——不触发 OnNBT 传送补发、允许 fill 合并。多机器人下逐块传送
	// 补发会阻塞大工作区（每块 TP+发包极慢，整单元久拖不完）；命令/结构方块已抠出进
	// 独立单元，不受影响。
	IgnoreNBTInUnit bool
}

// RunUnit 让导入只跑一个任务单元：只放该单元的区块列，跳过被抠出的位置（§8.1），
// 不做预清空（多机器人预清空由一台机器人先整图做一次）。单元共享同一份 data（§11.1）。
func (t *ImportTask) RunUnit(u *Unit) error {
	t.PlaceOnly = make(map[[2]int]bool, len(u.Chunks))
	for _, c := range u.Chunks {
		t.PlaceOnly[c] = true
	}
	if len(u.Skipped) > 0 {
		t.SkipPositions = make(map[[3]int]bool, len(u.Skipped))
		for _, p := range u.Skipped {
			t.SkipPositions[p] = true
		}
	}
	t.SkipPreClear = true
	// 普通单元里的带 NBT 方块（磁石等）直接当普通方块放，不逐块 TP 补发（见字段注释）
	t.IgnoreNBTInUnit = true
	return t.Run()
}

// placeable 判断结构局部坐标 (wx,y,wz) 是否属于本单元该放的位置：
// 所在区块列必须在 PlaceOnly（若有），且位置不在 SkipPositions（被抠出的方块）。
// 单机模式（PlaceOnly/SkipPositions 为 nil）恒为 true，行为不变。
func (t *ImportTask) placeable(wx, y, wz int) bool {
	if t.PlaceOnly != nil {
		if !t.PlaceOnly[[2]int{wx / ChunkSize, wz / ChunkSize}] {
			return false
		}
	}
	if t.SkipPositions != nil {
		if t.SkipPositions[[3]int{wx, y, wz}] {
			return false
		}
	}
	return true
}

// rateLimit 控制放置速率。time.Sleep 在多数平台上有 ~1ms 的最小精度，
// 当目标间隔小于 1ms 时累积后批量睡眠，避免实际速率远低于目标值。
func (t *ImportTask) rateLimit(d time.Duration) {
	if d >= time.Millisecond {
		time.Sleep(d)
		return
	}
	t.sleepAccum += d
	if t.sleepAccum >= time.Millisecond {
		time.Sleep(t.sleepAccum)
		t.sleepAccum = 0
	}
}

// checkOpError 在 StopOnError 模式下记录第一个错误，阻止后续放置
func (t *ImportTask) checkOpError(err error) {
	if err != nil && t.StopOnError && t.importErr == nil {
		t.importErr = err
	}
}

// clampInt32 将 v 限制在 [min, max] 范围内。
func clampInt32(v, min, max int32) int32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// isChunkCoveredByCircle 判断 world 坐标下的区块 (chunkX, chunkZ) 是否与修补圆形相交。
// 区块按 16×16 对齐，只要区块与圆形有交集（含相切）即视为被覆盖。
func (t *ImportTask) isChunkCoveredByCircle(chunkX, chunkZ int32) bool {
	// 区块的 XZ 范围（世界坐标）
	chunkMinX := chunkX * ChunkSize
	chunkMinZ := chunkZ * ChunkSize
	chunkMaxX := chunkMinX + ChunkSize - 1
	chunkMaxZ := chunkMinZ + ChunkSize - 1

	// 圆心到区块矩形最近点的距离
	closestX := clampInt32(t.RepairCenterX, chunkMinX, chunkMaxX)
	closestZ := clampInt32(t.RepairCenterZ, chunkMinZ, chunkMaxZ)
	dx := t.RepairCenterX - closestX
	dz := t.RepairCenterZ - closestZ
	return dx*dx+dz*dz <= t.RepairRadius*t.RepairRadius
}

// shouldRepairBlock 判断世界坐标 (x, y, z) 处的方块是否属于修补范围。
// 非修补模式下始终返回 true。
//
// 圆形模式按区块判断：只要方块所在区块被圆形覆盖，该区块内所有方块都修补。
// 矩形模式按方块坐标逐格判断。
func (t *ImportTask) shouldRepairBlock(x, y, z int32) bool {
	if !t.RepairMode {
		return true
	}
	if t.RepairUseCircle {
		return t.isChunkCoveredByCircle(x/ChunkSize, z/ChunkSize)
	}
	return x >= t.RepairRect[0] && x <= t.RepairRect[3] &&
		y >= t.RepairRect[1] && y <= t.RepairRect[4] &&
		z >= t.RepairRect[2] && z <= t.RepairRect[5]
}

type ImportCheckpointData struct {
	LastRX       int `json:"last_rx,omitempty"`
	LastRZ       int `json:"last_rz,omitempty"`
	LastCX       int `json:"last_cx"`
	LastCZ       int `json:"last_cz"`
	BlocksPlaced int `json:"blocks_placed"`
}

func (t *ImportTask) regionChunks() int {
	switch t.RegionMode {
	case 2:
		return 2
	case 3:
		return 4
	default:
		return 1
	}
}

func (t *ImportTask) Run() error {
	if t.Data == nil {
		return fmt.Errorf("no structure data loaded")
	}
	sleeptime := time.Second / time.Duration(t.Speed)
	totalChunksX := (t.Data.SizeX + ChunkSize - 1) / ChunkSize
	totalChunksZ := (t.Data.SizeZ + ChunkSize - 1) / ChunkSize
	totalChunks := totalChunksX * totalChunksZ
	rc := t.regionChunks()
	if t.PlaceOnly != nil {
		// 单元模式强制单区块粒度：单元以 16×16 区块列为单位，regionMode 不再适用。
		rc = 1
	}
	totalRegionsX := (totalChunksX + rc - 1) / rc
	totalRegionsZ := (totalChunksZ + rc - 1) / rc
	t.totalRegions = totalRegionsX * totalRegionsZ

	// ── 预清空（在导入之前，每个填充前自己 TP 加载区块） ──
	// 修补模式不预清空，避免破坏已有建筑；单元模式也不预清空（多机器人由一台先整图清一次）
	if !t.RepairMode && !t.SkipPreClear {
		if err := t.doPreClear(t.Dimension); err != nil {
			return err
		}
		if t.OnClearDone != nil && t.PreClearMode != 0 {
			t.OnClearDone()
		}
	}

	t.chunkSolid = make(map[[2]int]int)
	t.regionSolid = make(map[[2]int]int)
	t.TotalSolidBlocks = 0
	rc2 := t.regionChunks()
	for pos, idx := range t.Data.Blocks {
		block := t.Data.Palette[idx]
		if block.IsAir {
			continue
		}
		skip := false
		if t.ExcludeWater && block.IsWater {
			skip = true
		}
		if t.ExcludeLava && block.IsLava {
			skip = true
		}
		if skip {
			continue
		}
		x, _, z := unpackPos(pos)
		cx := int(x) / ChunkSize
		cz := int(z) / ChunkSize
		t.chunkSolid[[2]int{cx, cz}]++
		t.TotalSolidBlocks++
		t.regionSolid[[2]int{cx / rc2, cz / rc2}]++
	}

	blocksPlaced := 0
	skippedRegions := 0
	startRegionX := t.StartCX / rc
	startRegionZ := t.StartCZ / rc

	for rx := 0; rx < totalRegionsX; rx++ {
		var rzStart, rzEnd, rzStep int
		if rx%2 == 0 {
			rzStart, rzEnd, rzStep = 0, totalRegionsZ, 1
		} else {
			rzStart, rzEnd, rzStep = totalRegionsZ-1, -1, -1
		}

		for rz := rzStart; rz != rzEnd; rz += rzStep {
			if rx < startRegionX || (rx == startRegionX && rz < startRegionZ) {
				skippedRegions++
				continue
			}
			select {
			case <-t.StopCh:
				return fmt.Errorf("cancelled at region (%d,%d)", rx, rz)
			default:
			}
			if t.importErr != nil {
				return t.importErr
			}

			chunkStartX := rx * rc
			chunkEndX := min(chunkStartX+rc, totalChunksX)
			chunkStartZ := rz * rc
			chunkEndZ := min(chunkStartZ+rc, totalChunksZ)

			// 单元模式：只放属于本单元的区块列。rc==1 时一个区域即一个区块列。
			if t.PlaceOnly != nil && !t.PlaceOnly[[2]int{chunkStartX, chunkStartZ}] {
				skippedRegions++
				continue
			}

			// TP 到小区中心
			regionCX := chunkStartX*ChunkSize + (chunkEndX-chunkStartX)*ChunkSize/2
			regionCZ := chunkStartZ*ChunkSize + (chunkEndZ-chunkStartZ)*ChunkSize/2
			if t.Teleport != nil {
				if err := t.Teleport(t.BaseX+min(regionCX, t.Data.SizeX-1), t.BaseY, t.BaseZ+min(regionCZ, t.Data.SizeZ-1)); err != nil && t.StopOnError && t.importErr == nil {
					t.importErr = err
					return err
				}
				time.Sleep(30 * time.Millisecond)
			}

			// 计算小区在建筑内的结构坐标范围（chunk 索引 → 方块坐标）
			// 供后面 deny 铺底和 border 边缘共用，避免重复计算
			blockStartX := chunkStartX * ChunkSize                  // 小区起始 X（结构坐标）
			blockStartZ := chunkStartZ * ChunkSize                  // 小区起始 Z（结构坐标）
			blockEndX := min(chunkEndX*ChunkSize, t.Data.SizeX) - 1 // 小区结束 X（含）
			blockEndZ := min(chunkEndZ*ChunkSize, t.Data.SizeZ) - 1 // 小区结束 Z（含）

			// 放置前强制等待区块加载（多机器人模式 ChunkLoaded 已注入）：
			// 区块未加载时 setblock/fill 会静默失败（SendAICommand 只返回发包是否成功，
			// 检测不到服务器是否执行），导致整片区域缺失——大地图画区块多、机器人快速遍历，
			// 加载跟不上尤为明显。用 testforblock 探测，确认当前列已加载再放置（§10 注意 5）。
			// 单机 ChunkLoaded 为 nil 时 waitChunksLoaded 直接返回，不影响单机行为。
			// 注：ChunkLoaded 内部以"区块列×16+8"当世界坐标探测，故必须传世界坐标（含 BaseX/BaseZ）。
			t.waitChunksLoaded(int32(t.BaseX+blockStartX), int32(t.BaseZ+blockStartZ),
				int32(t.BaseX+blockEndX), int32(t.BaseZ+blockEndZ))

			// 修补模式不铺 deny、不铺 border
			if !t.RepairMode {
				// ── 拒绝方块层：在小区底部铺一层 deny ──
				if t.DenyEnable && t.FillRegion != nil {
					denyWorldY := t.BaseY + t.DenyYOffset
					fillX1 := int32(t.BaseX + blockStartX)
					fillZ1 := int32(t.BaseZ + blockStartZ)
					fillX2 := int32(t.BaseX + blockEndX)
					fillZ2 := int32(t.BaseZ + blockEndZ)
					t.FillRegion(fillX1, int32(denyWorldY), fillZ1, fillX2, int32(denyWorldY), fillZ2, "minecraft:deny", "[]")
					time.Sleep(10 * time.Millisecond)
				}

				// ── 边界方块：小区在建筑边缘的话，沿边放一圈 border_block ──
				if t.BorderEnable && t.FillRegion != nil {
					bWorldY := t.BaseY + t.BorderYRel
					// 只有小区在建筑边缘时才需要放置
					if rx == 0 || rx == totalRegionsX-1 || rz == 0 || rz == totalRegionsZ-1 {
						if rx == 0 {
							t.FillRegion(int32(t.BaseX-1), int32(bWorldY), int32(t.BaseZ+blockStartZ), int32(t.BaseX-1), int32(bWorldY), int32(t.BaseZ+blockEndZ), t.BorderBlock, "[]")
							time.Sleep(10 * time.Millisecond)
						}
						if rx == totalRegionsX-1 {
							wx := t.BaseX + t.Data.SizeX
							t.FillRegion(int32(wx), int32(bWorldY), int32(t.BaseZ+blockStartZ), int32(wx), int32(bWorldY), int32(t.BaseZ+blockEndZ), t.BorderBlock, "[]")
							time.Sleep(10 * time.Millisecond)
						}
						if rz == 0 {
							t.FillRegion(int32(t.BaseX+blockStartX), int32(bWorldY), int32(t.BaseZ-1), int32(t.BaseX+blockEndX), int32(bWorldY), int32(t.BaseZ-1), t.BorderBlock, "[]")
							time.Sleep(10 * time.Millisecond)
						}
						if rz == totalRegionsZ-1 {
							wz := t.BaseZ + t.Data.SizeZ
							t.FillRegion(int32(t.BaseX+blockStartX), int32(bWorldY), int32(wz), int32(t.BaseX+blockEndX), int32(bWorldY), int32(wz), t.BorderBlock, "[]")
							time.Sleep(10 * time.Millisecond)
						}
					}
				}

			}
			// 放置方块：区域模式（跨区块合并）或逐区块模式
			if rc > 1 {
				p := t.placeRegion(rx, rz, rc, sleeptime)
				blocksPlaced += p
				// 报告区域内所有区块完成，用于进度条和断点
				for cx := chunkStartX; cx < chunkEndX; cx++ {
					for cz := chunkStartZ; cz < chunkEndZ; cz++ {
						chunkNum := cx*totalChunksZ + cz + 1
						if t.OnProgress != nil {
							t.OnProgress(cx, cz, chunkNum, totalChunks, blocksPlaced)
						}
					}
				}
			} else {
				for cx := chunkStartX; cx < chunkEndX; cx++ {
					var cz2Start, cz2End, cz2Step int
					if cx%2 == 0 {
						cz2Start, cz2End, cz2Step = chunkStartZ, chunkEndZ, 1
					} else {
						cz2Start, cz2End, cz2Step = chunkEndZ-1, chunkStartZ-1, -1
					}
					for cz := cz2Start; cz != cz2End; cz += cz2Step {
						select {
						case <-t.StopCh:
							return fmt.Errorf("cancelled at chunk (%d,%d)", cx, cz)
						default:
						}
						if t.importErr != nil {
							return t.importErr
						}
						cxs := min(ChunkSize, t.Data.SizeX-cx*ChunkSize)
						czs := min(ChunkSize, t.Data.SizeZ-cz*ChunkSize)
						p := t.placeChunk(cx, cz, cxs, czs, sleeptime)
						blocksPlaced += p
						chunkNum := cx*totalChunksZ + cz + 1
						if t.OnProgress != nil {
							t.OnProgress(cx, cz, chunkNum, totalChunks, blocksPlaced)
						}
					}
				}
			}

			regionIdx := rx*totalRegionsZ + rz + 1
			if t.OnRegionProgress != nil {
				t.OnRegionProgress(regionIdx-skippedRegions, t.totalRegions-skippedRegions, rx, rz)
			}
		}
	}
	return nil
}

// placeRegion 对整片区域应用与 placeChunk 相同的立体优化（跨区块）。
// 与 placeChunk 的区别：visited 覆盖区域大小而非 16×16，fill 可跨区块边界合并。
func (t *ImportTask) placeRegion(rx, rz, rc int, sleeptime time.Duration) int {
	regionStartX := rx * rc
	regionStartZ := rz * rc
	regionXSize := min(rc*ChunkSize, t.Data.SizeX-regionStartX*ChunkSize)
	regionZSize := min(rc*ChunkSize, t.Data.SizeZ-regionStartZ*ChunkSize)
	bx, by, bz := int32(t.BaseX), int32(t.BaseY), int32(t.BaseZ)
	placed := 0
	regionSolid := t.regionSolid[[2]int{rx, rz}]

	visited := make([][][]bool, t.Data.SizeY)
	for y := 0; y < t.Data.SizeY; y++ {
		vy := make([][]bool, regionXSize)
		for x := 0; x < regionXSize; x++ {
			vy[x] = make([]bool, regionZSize)
		}
		visited[y] = vy
	}

	// 第一遍：放置非依附方块（立体优化）
	for y := 0; y < t.Data.SizeY; y++ {
		for z := 0; z < regionZSize; z++ {
			for x := 0; x < regionXSize; x++ {
				select {
				case <-t.StopCh:
					return placed
				default:
				}
				if t.importErr != nil {
					return placed
				}
				if visited[y][x][z] {
					continue
				}
				wx := regionStartX*ChunkSize + x
				wz := regionStartZ*ChunkSize + z

				// Fast path: palette index (uint32) instead of Get()
				paletteIdx := t.Data.GetIndex(wx, y, wz)
				if paletteIdx == 0 {
					visited[y][x][z] = true
					continue
				}
				block := t.Data.Palette[paletteIdx]
				if block.IsDependent || block.IsGravity {
					visited[y][x][z] = true
					continue
				}

				if t.ExcludeWater && block.IsWater {
					visited[y][x][z] = true
					continue
				}
				if t.ExcludeLava && block.IsLava {
					visited[y][x][z] = true
					continue
				}
				if t.ExcludeWaterlogged && strings.Contains(block.States, "waterlogged") {
					block.States = stripWaterlogged(block.States)
				}
				// 修补模式：跳过修补范围外的方块
				if !t.shouldRepairBlock(bx+int32(wx), by+int32(y), bz+int32(wz)) {
					visited[y][x][z] = true
					continue
				}

				blockNBT := t.Data.GetNBT(wx, y, wz)
				// 单元模式（IgnoreNBTInUnit）：带 NBT 方块（磁石/空容器等）按普通方块放置
				hasNBT := blockNBT != nil && !t.IgnoreNBTInUnit
				skipNBT := !t.ImportCommands && blockNBT != nil && block.IsCommand
				// 仅修补NBT：跳过无 NBT 数据的普通方块
				if t.RepairNBTOnly && !hasNBT {
					visited[y][x][z] = true
					continue
				}

				if hasNBT {
					visited[y][x][z] = true
					if err := t.SetBlock(bx+int32(wx), by+int32(y), bz+int32(wz), block.Name, block.States); err != nil {
						t.checkOpError(err)
					}
					if t.OnNBT != nil && !skipNBT {
						t.OnNBT(bx+int32(wx), by+int32(y), bz+int32(wz), blockNBT, block.Name, block.States)
					}
					placed++
					if t.OnSubProgress != nil && regionSolid > 0 {
						t.OnSubProgress(regionStartX, regionStartZ, placed, regionSolid)
					}
					t.rateLimit(sleeptime)
					continue
				}

				targetIdx := t.Data.getOrCreatePaletteIndex(block.Name, block.States)
				rw, rd := findBestRectInSlice(t.Data, y, regionStartX, regionStartZ, x, z,
					regionXSize, regionZSize, targetIdx, visited[y], t.IgnoreNBTInUnit)
				if rw*rd == 0 {
					rw, rd = 1, 1
				}

				// Try to extend rectangle vertically into a cuboid, capped by maxFillVolume
				maxH := maxFillVolume / (rw * rd)
				if maxH < 1 {
					maxH = 1
				}
				cuboidH := 1
				for y+cuboidH < t.Data.SizeY && cuboidH < maxH {
					nextY := y + cuboidH
					if rectExistsInSlice(t.Data, nextY, regionStartX, regionStartZ, x, z,
						rw, rd, targetIdx, visited[nextY], t.IgnoreNBTInUnit) {
						cuboidH++
					} else {
						break
					}
				}

				maxY := y + cuboidH - 1
				for dy := 0; dy < cuboidH; dy++ {
					cy := y + dy
					for dz := 0; dz < rd; dz++ {
						for dx := 0; dx < rw; dx++ {
							visited[cy][x+dx][z+dz] = true
						}
					}
				}

				totalBlocks := rw * rd * cuboidH
				if totalBlocks >= 3 && t.FillRegion != nil && totalBlocks <= maxFillVolume {
					if err := t.FillRegion(
						bx+int32(wx), by+int32(y), bz+int32(wz),
						bx+int32(wx+rw-1), by+int32(maxY), bz+int32(wz+rd-1),
						block.Name, block.States,
					); err != nil {
						t.checkOpError(err)
					}
				} else {
					for dy := 0; dy < cuboidH; dy++ {
						cy := y + dy
						for dz := 0; dz < rd; dz++ {
							for dx := 0; dx < rw; dx++ {
								if err := t.SetBlock(bx+int32(wx+dx), by+int32(cy), bz+int32(wz+dz), block.Name, block.States); err != nil {
									t.checkOpError(err)
								}
							}
						}
					}
				}
				placed += totalBlocks
				if t.OnSubProgress != nil && regionSolid > 0 {
					t.OnSubProgress(regionStartX, regionStartZ, placed, regionSolid)
				}
				t.rateLimit(sleeptime)
			}
		}
	}

	// 第二遍：放置重力方块（立体优化，在普通方块之后、依赖方块之前确保支撑）
	visitedG := make([][][]bool, t.Data.SizeY)
	for y := 0; y < t.Data.SizeY; y++ {
		vy := make([][]bool, regionXSize)
		for x := 0; x < regionXSize; x++ {
			vy[x] = make([]bool, regionZSize)
		}
		visitedG[y] = vy
	}
	for y := 0; y < t.Data.SizeY; y++ {
		for z := 0; z < regionZSize; z++ {
			for x := 0; x < regionXSize; x++ {
				select {
				case <-t.StopCh:
					return placed
				default:
					if t.importErr != nil {
						return placed
					}
				}
				if visitedG[y][x][z] {
					continue
				}
				wx := regionStartX*ChunkSize + x
				wz := regionStartZ*ChunkSize + z

				// Fast path: palette index (uint32) instead of Get()
				paletteIdx := t.Data.GetIndex(wx, y, wz)
				if paletteIdx == 0 {
					visitedG[y][x][z] = true
					continue
				}
				block := t.Data.Palette[paletteIdx]
				if !block.IsGravity {
					visitedG[y][x][z] = true
					continue
				}

				// Fluid filtering
				if t.ExcludeWater && block.IsWater {
					visitedG[y][x][z] = true
					continue
				}
				if t.ExcludeLava && block.IsLava {
					visitedG[y][x][z] = true
					continue
				}
				// 修补模式：跳过修补范围外的方块
				if !t.shouldRepairBlock(bx+int32(wx), by+int32(y), bz+int32(wz)) {
					visitedG[y][x][z] = true
					continue
				}

				blockNBT := t.Data.GetNBT(wx, y, wz)
				// 单元模式（IgnoreNBTInUnit）：带 NBT 方块（磁石/空容器等）按普通方块放置
				hasNBT := blockNBT != nil && !t.IgnoreNBTInUnit
				skipNBT := !t.ImportCommands && blockNBT != nil && block.IsCommand
				// 仅修补NBT：跳过无 NBT 数据的普通方块
				if t.RepairNBTOnly && !hasNBT {
					visitedG[y][x][z] = true
					continue
				}

				if hasNBT {
					visitedG[y][x][z] = true
					if err := t.SetBlock(bx+int32(wx), by+int32(y), bz+int32(wz), block.Name, block.States); err != nil {
						t.checkOpError(err)
					}
					if t.OnNBT != nil && !skipNBT {
						t.OnNBT(bx+int32(wx), by+int32(y), bz+int32(wz), blockNBT, block.Name, block.States)
					}
					placed++
					t.rateLimit(sleeptime)
					continue
				}

				// Gravity pass has no waterlogged stripping, so paletteIdx is the target
				rw, rd := findBestRectInSlice(t.Data, y, regionStartX, regionStartZ, x, z,
					regionXSize, regionZSize, paletteIdx, visitedG[y], t.IgnoreNBTInUnit)
				if rw*rd == 0 {
					rw, rd = 1, 1
				}
				// Try to extend rectangle vertically into a cuboid, capped by maxFillVolume
				maxH := maxFillVolume / (rw * rd)
				if maxH < 1 {
					maxH = 1
				}
				cuboidH := 1
				for y+cuboidH < t.Data.SizeY && cuboidH < maxH {
					nextY := y + cuboidH
					if rectExistsInSlice(t.Data, nextY, regionStartX, regionStartZ, x, z,
						rw, rd, paletteIdx, visitedG[nextY], t.IgnoreNBTInUnit) {
						cuboidH++
					} else {
						break
					}
				}
				maxY := y + cuboidH - 1
				for dy := 0; dy < cuboidH; dy++ {
					cy := y + dy
					for dz := 0; dz < rd; dz++ {
						for dx := 0; dx < rw; dx++ {
							visitedG[cy][x+dx][z+dz] = true
						}
					}
				}
				totalBlocks := rw * rd * cuboidH
				if totalBlocks >= 3 && t.FillRegion != nil && totalBlocks <= maxFillVolume {
					if err := t.FillRegion(
						bx+int32(wx), by+int32(y), bz+int32(wz),
						bx+int32(wx+rw-1), by+int32(maxY), bz+int32(wz+rd-1),
						block.Name, block.States,
					); err != nil {
						t.checkOpError(err)
					}
				} else {
					for dy := 0; dy < cuboidH; dy++ {
						cy := y + dy
						for dz := 0; dz < rd; dz++ {
							for dx := 0; dx < rw; dx++ {
								if err := t.SetBlock(bx+int32(wx+dx), by+int32(cy), bz+int32(wz+dz), block.Name, block.States); err != nil {
									t.checkOpError(err)
								}
							}
						}
					}
				}
				placed += totalBlocks
				if t.OnSubProgress != nil && regionSolid > 0 {
					t.OnSubProgress(regionStartX, regionStartZ, placed, regionSolid)
				}
				t.rateLimit(sleeptime)
			}
		}
	}

	// 第三遍：放置依附方块（逐线）
	for y := 0; y < t.Data.SizeY; y++ {
		for z := 0; z < regionZSize; z++ {
			select {
			case <-t.StopCh:
				return placed
			default:
			}
			if t.importErr != nil {
				return placed
			}
			wz := regionStartZ*ChunkSize + z
			x := 0
			for x < regionXSize {
				wx := regionStartX*ChunkSize + x
				block := t.Data.Get(wx, y, wz)
				if block == nil || block.IsAir || !block.IsDependent {
					x++
					continue
				}
				// 修补模式：跳过修补范围外的方块
				if !t.shouldRepairBlock(bx+int32(wx), by+int32(y), bz+int32(wz)) {
					x++
					continue
				}

				runEnd := x + 1
				for runEnd < regionXSize {
					nb := t.Data.Get(regionStartX*ChunkSize+runEnd, y, wz)
					if nb == nil || nb.Name != block.Name || nb.States != block.States {
						break
					}
					runEnd++
				}
				n := runEnd - x

				// 仅修补NBT：跳过无 NBT 数据的依赖方块
				if t.RepairNBTOnly && !(block.NBT != nil && len(block.NBT) > 0) {
					x = runEnd
					continue
				}

				if n >= 3 && t.FillRegion != nil && !(block.NBT != nil && len(block.NBT) > 0 && !t.IgnoreNBTInUnit) {
					if err := t.FillRegion(
						bx+int32(wx), by+int32(y), bz+int32(wz),
						bx+int32(regionStartX*ChunkSize+runEnd-1), by+int32(y), bz+int32(wz),
						block.Name, block.States,
					); err != nil {
						t.checkOpError(err)
					}
				} else {
					skipNBT := !t.ImportCommands && block.NBT != nil && block.IsCommand
					for i := 0; i < n; i++ {
						if err := t.SetBlock(bx+int32(wx+i), by+int32(y), bz+int32(wz), block.Name, block.States); err != nil {
							t.checkOpError(err)
						}
						// 单元模式（IgnoreNBTInUnit）：不逐块 TP 补发 NBT（磁石/空容器等按普通方块放）
						if t.OnNBT != nil && block.NBT != nil && len(block.NBT) > 0 && !skipNBT && !t.IgnoreNBTInUnit {
							t.OnNBT(bx+int32(wx+i), by+int32(y), bz+int32(wz), block.NBT, block.Name, block.States)
						}
					}
				}
				placed += n
				if t.OnSubProgress != nil && regionSolid > 0 {
					t.OnSubProgress(regionStartX, regionStartZ, placed, regionSolid)
				}
				t.rateLimit(sleeptime)
				x = runEnd
			}
		}
	}

	return placed
}

func (t *ImportTask) placeChunk(cx, cz, chunkXSize, chunkZSize int, sleeptime time.Duration) int {
	placed := 0
	chunkTotal := t.chunkSolid[[2]int{cx, cz}]

	// Pass 1: non-gravity, non-dependent blocks (rect optimization)
	if t.FillRegion != nil {
		placed += t.placeChunkRectPass(cx, cz, chunkXSize, chunkZSize, sleeptime, placeNormal)
	} else {
		placed += t.placeChunkLineByLine(cx, cz, chunkXSize, chunkZSize, sleeptime, placed, chunkTotal, placeNormal)
	}

	// Pass 2: gravity blocks (rect optimization, after support blocks exist)
	if t.FillRegion != nil {
		placed += t.placeChunkRectPass(cx, cz, chunkXSize, chunkZSize, sleeptime, placeGravity)
	} else {
		placed += t.placeChunkLineByLine(cx, cz, chunkXSize, chunkZSize, sleeptime, placed, chunkTotal, placeGravity)
	}

	// Pass 3: dependent blocks (line-by-line, last)
	placed += t.placeChunkLineByLine(cx, cz, chunkXSize, chunkZSize, sleeptime, placed, chunkTotal, placeDependent)

	return placed
}

// placeChunkRectPass applies rectangle/cuboid optimization for normal (non-gravity, non-dependent) blocks.
func (t *ImportTask) placeChunkRectPass(cx, cz, chunkXSize, chunkZSize int, sleeptime time.Duration, mode int) int {
	placed := 0
	data := t.Data
	bx, by, bz := int32(t.BaseX), int32(t.BaseY), int32(t.BaseZ)
	chunkTotal := t.chunkSolid[[2]int{cx, cz}]

	// rectangle-based optimization
	// visited[y][x][z] — track already-placed blocks
	visited := make([][][]bool, data.SizeY)
	for y := 0; y < data.SizeY; y++ {
		vy := make([][]bool, chunkXSize)
		for x := 0; x < chunkXSize; x++ {
			vy[x] = make([]bool, chunkZSize)
		}
		visited[y] = vy
	}

	for y := 0; y < data.SizeY; y++ {
		for z := 0; z < chunkZSize; z++ {
			for x := 0; x < chunkXSize; x++ {
				select {
				case <-t.StopCh:
					return placed
				default:
				}
				if t.importErr != nil {
					return placed
				}
				if visited[y][x][z] {
					continue
				}
				wx := cx*ChunkSize + x
				wz := cz*ChunkSize + z

				// Fast path: use palette index (uint32) instead of Get() which copies BlockInfo + NBT lookup
				paletteIdx := data.GetIndex(wx, y, wz)
				if paletteIdx == 0 {
					visited[y][x][z] = true
					continue
				}
				// 单元模式：跳过不属于本单元的位置（其他单元列 / 被抠出的方块）
				if !t.placeable(wx, y, wz) {
					visited[y][x][z] = true
					continue
				}
				block := data.Palette[paletteIdx] // value copy, same flags as Get() but no NBT fetch
				var skip bool
				switch mode {
				case placeNormal:
					skip = block.IsDependent || block.IsGravity
				case placeGravity:
					skip = !block.IsGravity
				}
				if skip {
					visited[y][x][z] = true
					continue
				}

				// Fluid filtering
				if t.ExcludeWater && block.IsWater {
					visited[y][x][z] = true
					continue
				}
				if t.ExcludeLava && block.IsLava {
					visited[y][x][z] = true
					continue
				}
				if t.ExcludeWaterlogged && strings.Contains(block.States, "waterlogged") {
					block.States = stripWaterlogged(block.States)
				}
				// 修补模式：跳过修补范围外的方块
				if !t.shouldRepairBlock(bx+int32(wx), by+int32(y), bz+int32(wz)) {
					visited[y][x][z] = true
					continue
				}
				// NBT lookup is separate from palette — avoids map overhead in the common case
				blockNBT := data.GetNBT(wx, y, wz)
				// 单元模式（IgnoreNBTInUnit）：带 NBT 方块（磁石/空容器等）按普通方块放置，
				// 不逐块 TP 补发（有内容的容器/命令方块已抠出进独立单元）
				hasNBT := blockNBT != nil && !t.IgnoreNBTInUnit
				skipNBT := !t.ImportCommands && blockNBT != nil && block.IsCommand
				// 仅修补NBT：跳过无 NBT 数据的普通方块
				if t.RepairNBTOnly && !hasNBT {
					visited[y][x][z] = true
					continue
				}

				// 只有带 NBT 的方块实体才不能合并、需单独 setblock + OnNBT 传送处理。
				// 半砖/楼梯/玻璃板等字符串状态方块无 NBT，可正常合并填充，不走此慢路径。
				if hasNBT {
					visited[y][x][z] = true
					if err := t.SetBlock(bx+int32(wx), by+int32(y), bz+int32(wz), block.Name, block.States); err != nil {
						t.checkOpError(err)
					}
					if t.OnNBT != nil && !skipNBT {
						t.OnNBT(bx+int32(wx), by+int32(y), bz+int32(wz), blockNBT, block.Name, block.States)
					}
					placed++
					if t.OnSubProgress != nil && chunkTotal > 0 {
						t.OnSubProgress(cx, cz, placed, chunkTotal)
					}
					t.rateLimit(sleeptime)
					continue
				}

				// Compute target palette index once (may differ from paletteIdx if waterlogged was stripped)
				targetIdx := data.getOrCreatePaletteIndex(block.Name, block.States)

				// Find largest rectangle of same block in this Y-slice
				rw, rd := findBestRectInSlice(data, y, cx, cz, x, z, chunkXSize, chunkZSize, targetIdx, visited[y], t.IgnoreNBTInUnit)
				if rw*rd == 0 {
					rw, rd = 1, 1
				}

				// Try to extend rectangle vertically into a cuboid, capped by maxFillVolume
				maxH := maxFillVolume / (rw * rd)
				if maxH < 1 {
					maxH = 1
				}
				cuboidH := 1
				for y+cuboidH < data.SizeY && cuboidH < maxH {
					nextY := y + cuboidH
					if rectExistsInSlice(data, nextY, cx, cz, x, z, rw, rd, targetIdx, visited[nextY], t.IgnoreNBTInUnit) {
						cuboidH++
					} else {
						break
					}
				}

				// Mark as visited
				maxY := y + cuboidH - 1
				for dy := 0; dy < cuboidH; dy++ {
					cy := y + dy
					for dz := 0; dz < rd; dz++ {
						for dx := 0; dx < rw; dx++ {
							visited[cy][x+dx][z+dz] = true
						}
					}
				}

				totalBlocks := rw * rd * cuboidH

				// Gravity blocks: use setblock to prevent falling
				// Normal blocks: use FillRegion for large cuboids
				canFill := mode != placeGravity && totalBlocks >= 3 && t.FillRegion != nil && totalBlocks <= maxFillVolume
				if canFill {
					t.FillRegion(
						bx+int32(cx*ChunkSize+x), by+int32(y), bz+int32(cz*ChunkSize+z),
						bx+int32(cx*ChunkSize+x+rw-1), by+int32(maxY), bz+int32(cz*ChunkSize+z+rd-1),
						block.Name, block.States,
					)
				} else {
					for dy := 0; dy < cuboidH; dy++ {
						cy := y + dy
						for dz := 0; dz < rd; dz++ {
							for dx := 0; dx < rw; dx++ {
								px := cx*ChunkSize + x + dx
								py := cy
								pz := cz*ChunkSize + z + dz
								t.SetBlock(bx+int32(px), by+int32(py), bz+int32(pz), block.Name, block.States)
							}
						}
					}
				}
				placed += totalBlocks
				if t.OnSubProgress != nil && chunkTotal > 0 {
					t.OnSubProgress(cx, cz, placed, chunkTotal)
				}
				t.rateLimit(sleeptime)
			}
		}
	}
	return placed
}

// findBestRectInSlice does bidirectional search for the largest rectangle of the same block
// in a Y-slice, starting from (startX, startZ). It expands both Z-first and X-first,
// picking the larger result. Dimensions are capped to respect maxFillVolume.
// Uses palette index comparison (uint32) instead of string comparison for speed.
func findBestRectInSlice(data *StructureData, y, cx, cz, startX, startZ int,
	chunkXSize, chunkZSize int, targetIdx uint32, visited [][]bool, ignoreNBT bool) (width, depth int) {

	canUse := func(x, z int) bool {
		if x >= chunkXSize || z >= chunkZSize {
			return false
		}
		if visited[x][z] {
			return false
		}
		// Compare palette indices (uint32) instead of Name+States strings
		if data.GetIndex(cx*ChunkSize+x, y, cz*ChunkSize+z) != targetIdx {
			return false
		}
		// NBT blocks cannot be merged (already handled in outer loop, safety check)
		// 单元模式（ignoreNBT）下允许合并：带 NBT 方块按普通方块放置
		if !ignoreNBT && data.GetNBT(cx*ChunkSize+x, y, cz*ChunkSize+z) != nil {
			return false
		}
		return true
	}

	bestW, bestD := 1, 1

	// Pass 1: extend depth (Z), track min width
	curW := 0
	for dz := 0; startZ+dz < chunkZSize; dz++ {
		rowW := 0
		for wx := startX; wx < chunkXSize; wx++ {
			if !canUse(wx, startZ+dz) {
				break
			}
			rowW++
		}
		if rowW == 0 {
			break
		}
		if dz == 0 || rowW < curW {
			curW = rowW
		}
		w, d := curW, dz+1
		if w*d > maxFillVolume {
			w = maxFillVolume / d
		}
		if w*d > bestW*bestD {
			bestW, bestD = w, d
		}
	}

	// Pass 2: extend width (X), track min depth
	curD := 0
	for dw := 0; startX+dw < chunkXSize; dw++ {
		colD := 0
		for wz := startZ; wz < chunkZSize; wz++ {
			if !canUse(startX+dw, wz) {
				break
			}
			colD++
		}
		if colD == 0 {
			break
		}
		if dw == 0 || colD < curD {
			curD = colD
		}
		w, d := dw+1, curD
		if w*d > maxFillVolume {
			d = maxFillVolume / w
		}
		if w*d > bestW*bestD {
			bestW, bestD = w, d
		}
	}

	return bestW, bestD
}

// rectExistsInSlice checks if a specific rectangle position is fully occupied
// by the same block type, not placed yet, and none have NBT data.
//
// The `y` parameter is the NEXT Y level to check; `name` and `states` are the
// CURRENT Y level's block type. The function compares against the current block's
// palette index (not the next Y level's) to prevent incorrect vertical extension
// when different block types exist at different Y levels.
func rectExistsInSlice(data *StructureData, y, cx, cz, startX, startZ, w, d int,
	targetIdx uint32, visited [][]bool, ignoreNBT bool) bool {

	// targetIdx 由调用方传入（基于当前层的 palette 索引），
	// 不能使用 data.GetIndex(..., y, ...) 因为 y 是 nextY，会读到下一层的方块
	if targetIdx == 0 {
		return false
	}
	for dz := 0; dz < d; dz++ {
		z := startZ + dz
		for dx := 0; dx < w; dx++ {
			x := startX + dx
			if visited[x][z] {
				return false
			}
			if data.GetIndex(cx*ChunkSize+x, y, cz*ChunkSize+z) != targetIdx {
				return false
			}
			// 单元模式（ignoreNBT）下允许合并：带 NBT 方块按普通方块放置
			if !ignoreNBT && data.GetNBT(cx*ChunkSize+x, y, cz*ChunkSize+z) != nil {
				return false
			}
		}
	}
	return true
}

// placeChunkLineByLine places blocks line-by-line along X axis (original algorithm).
// mode: placeNormal (skip gravity+dependent), placeGravity (only gravity), placeDependent (only dependent).
func (t *ImportTask) placeChunkLineByLine(cx, cz, chunkXSize, chunkZSize int, sleeptime time.Duration, placed, chunkTotal int, mode int) int {
	data := t.Data
	bx, by, bz := int32(t.BaseX), int32(t.BaseY), int32(t.BaseZ)

	for y := 0; y < data.SizeY; y++ {
		for z := 0; z < chunkZSize; z++ {
			wz := cz*ChunkSize + z
			x := 0
			for x < chunkXSize {
				select {
				case <-t.StopCh:
					return placed
				default:
				}
				if t.importErr != nil {
					return placed
				}
				wx := cx*ChunkSize + x
				block := data.Get(wx, y, wz)
				if block == nil || block.IsAir {
					x++
					continue
				}
				switch mode {
				case placeNormal:
					if block.IsDependent || block.IsGravity {
						x++
						continue
					}
				case placeGravity:
					if !block.IsGravity {
						x++
						continue
					}
				case placeDependent:
					if !block.IsDependent {
						x++
						continue
					}
				}
				// 单元模式：跳过不属于本单元的位置
				if !t.placeable(wx, y, wz) {
					x++
					continue
				}
				// 修补模式：跳过修补范围外的方块
				if !t.shouldRepairBlock(bx+int32(wx), by+int32(y), bz+int32(wz)) {
					x++
					continue
				}

				// Find consecutive same blocks
				runEnd := x + 1
				for runEnd < chunkXSize {
					nb := data.Get(cx*ChunkSize+runEnd, y, wz)
					if nb == nil || nb.Name != block.Name || nb.States != block.States {
						break
					}
					runEnd++
				}
				n := runEnd - x

				hasNBT := block.NBT != nil && len(block.NBT) > 0 && !t.IgnoreNBTInUnit
				// 仅修补NBT：跳过无 NBT 数据的普通方块
				if t.RepairNBTOnly && !hasNBT {
					x = runEnd
					continue
				}
				if n >= 3 && t.FillRegion != nil && !hasNBT {
					t.FillRegion(bx+int32(wx), by+int32(y), bz+int32(wz),
						bx+int32(cx*ChunkSize+runEnd-1), by+int32(y), bz+int32(wz),
						block.Name, block.States)
				} else {
					skipNBT := !t.ImportCommands && block.NBT != nil && block.IsCommand
					for i := 0; i < n; i++ {
						t.SetBlock(bx+int32(cx*ChunkSize+x+i), by+int32(y), bz+int32(wz), block.Name, block.States)
						// 单元模式（IgnoreNBTInUnit）：不逐块 TP 补发 NBT（磁石/空容器等按普通方块放）
						if t.OnNBT != nil && block.NBT != nil && len(block.NBT) > 0 && !skipNBT && !t.IgnoreNBTInUnit {
							t.OnNBT(bx+int32(cx*ChunkSize+x+i), by+int32(y), bz+int32(wz), block.NBT, block.Name, block.States)
						}
					}
				}
				placed += n
				if t.OnSubProgress != nil && chunkTotal > 0 {
					t.OnSubProgress(cx, cz, placed, chunkTotal)
				}
				t.rateLimit(sleeptime)
				x = runEnd
			}
		}
	}
	return placed
}

func DetectFormat(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mcworld":
		return "mcworld"
	case ".mcstructure":
		return "mcstructure"
	case ".bdx":
		return "bdx"
	case ".schematic", ".schem":
		return "schematic"
	default:
		return ""
	}
}

func LoadStructureFile(path string, region ...[6]int) (*StructureData, error) {
	format := DetectFormat(path)

	switch format {
	case "mcstructure":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("LoadStructureFile: %w", err)
		}
		return ParseMCStructure(data)

	case "schematic", "schem":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("LoadStructureFile: %w", err)
		}
		return ParseSchematic(data)

	case "bdx":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("LoadStructureFile: %w", err)
		}
		return ParseBDX(data)

	case "mcworld":
		if len(region) > 0 {
			return ParseMCWorldWithRegion(path, region[0])
		}
		return ParseMCWorld(path)
	}

	return nil, fmt.Errorf("LoadStructureFile: unsupported format %q (supported: .mcstructure, .schematic, .schem, .bdx, .mcworld)", filepath.Ext(path))
}

func IsWaterBlock(name string) bool {
	n := strings.ToLower(name)
	return n == "minecraft:water" || n == "minecraft:flowing_water" || n == "water" || n == "flowing_water"
}

func IsLavaBlock(name string) bool {
	n := strings.ToLower(name)
	return n == "minecraft:lava" || n == "minecraft:flowing_lava" || n == "lava" || n == "flowing_lava"
}

func IsCommandBlock(name string) bool {
	return strings.Contains(strings.ToLower(name), "command_block")
}

// isDependentBlock 返回方块是否需要依附在其他方块上（红石、按钮、火把、藤蔓等）。
// 这些方块放在第二遍导入，等支撑方块放置完毕后再放置。
func isDependentBlock(name string) bool {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "redstone_wire"), strings.Contains(n, "redstone_torch"):
		return true
	case strings.Contains(n, "repeater"), strings.Contains(n, "comparator"):
		return true
	case strings.Contains(n, "button"), strings.Contains(n, "lever"):
		return true
	case strings.Contains(n, "pressure_plate"):
		return true
	case strings.Contains(n, "rail"):
		return true // detector_rail, activator_rail, powered_rail, rail
	case strings.Contains(n, "torch") && !strings.Contains(n, "redstone_torch"):
		return true // wall torches
	case strings.Contains(n, "vine"), strings.Contains(n, "ladder"):
		return true
	case strings.Contains(n, "door"), strings.Contains(n, "trapdoor"):
		return true
	case strings.Contains(n, "sign"), strings.Contains(n, "banner"):
		return true
	case strings.Contains(n, "sapling"), strings.Contains(n, "flower"), strings.Contains(n, "tulip"),
		strings.Contains(n, "dandelion"), strings.Contains(n, "rose"), strings.Contains(n, "mushroom"),
		strings.Contains(n, "grass") && n != "minecraft:grass_block" && n != "minecraft:grass_path":
		return true
	case strings.Contains(n, "carpet"), strings.Contains(n, "snow_layer"):
		return true
	case strings.Contains(n, "web"), strings.Contains(n, "cocoa"):
		return true
	case strings.Contains(n, "dead_bush"), strings.Contains(n, "fern"), strings.Contains(n, "sugar_cane"):
		return true
	case strings.Contains(n, "cactus"), strings.Contains(n, "bamboo"):
		return true
	case strings.Contains(n, "kelp"), strings.Contains(n, "seagrass"):
		return true
	case strings.Contains(n, "sculk_vein"):
		return true
	case strings.Contains(n, "glow_lichen"):
		return true
	case strings.Contains(n, "end_rod"), strings.Contains(n, "lightning_rod"):
		return true
	case strings.Contains(n, "coral_fan"), strings.Contains(n, "coral_wall_fan"):
		return true
	case strings.Contains(n, "pointed_dripstone"):
		return true
	case strings.Contains(n, "hanging_roots"):
		return true
	case strings.Contains(n, "candle"):
		return true
	case strings.Contains(n, "pink_petals"):
		return true
	case strings.Contains(n, "frogspawn"):
		return true
	case strings.Contains(n, "big_dripleaf"), strings.Contains(n, "small_dripleaf"):
		return true
	case strings.Contains(n, "spore_blossom"), strings.Contains(n, "moss_carpet"):
		return true
	case strings.Contains(n, "azalea") && !strings.Contains(n, "azalea_leaves"):
		return true
	}
	return false
}

// isGravityBlock 返回方块是否受重力影响（无支撑时坠落）。
func isGravityBlock(name string) bool {
	n := strings.ToLower(name)
	switch {
	case n == "minecraft:sand" || n == "sand" || n == "minecraft:red_sand" || n == "red_sand":
		return true
	case n == "minecraft:gravel" || n == "gravel" || n == "minecraft:suspicious_gravel" || n == "suspicious_gravel":
		return true
	case n == "minecraft:suspicious_sand" || n == "suspicious_sand":
		return true
	case strings.Contains(n, "concrete_powder"):
		return true
	case strings.Contains(n, "anvil"):
		return true
	case n == "minecraft:dragon_egg" || n == "dragon_egg":
		return true
	}
	return false
}

// ── 维度 Y 范围（3 大段） ──
var dimensionBigSegs = map[string][][2]int32{
	"overworld": {{-64, 0}, {0, 128}, {128, 321}},
	"nether":    {{0, 128}},
	"the_end":   {{0, 256}},
}

func getBigSegs(dim string) [][2]int32 {
	if s, ok := dimensionBigSegs[dim]; ok {
		return s
	}
	return dimensionBigSegs["overworld"]
}

// doPreClear 在导入前清空区域，模式：
//
//	1=一格（建筑矩形空间） 2=竖柱（基准点到高度上限） 3=区块（整区块列）
//
// 优化：子区域按 GCD 划分 + 蛇形，减少 TP 次数。
func (t *ImportTask) doPreClear(dim string) error {
	if t.PreClearMode == 0 || t.FillRegion == nil {
		return nil
	}
	bigSegs := getBigSegs(dim)
	subSize := calcSubRegionSize(t.Data.SizeX, t.Data.SizeZ)
	switch t.PreClearMode {
	case 1:
		return t.clearBoundingBox(bigSegs)
	case 2:
		return t.clearChunkColumnsOptimized(bigSegs, subSize, false)
	case 3:
		return t.clearChunkColumnsOptimized(bigSegs, subSize, true)
	}
	return nil
}

// clearBoundingBox 模式1：清空建筑矩形空间（从 Base 到 Base+Size）
func (t *ImportTask) clearBoundingBox(segs [][2]int32) error {
	wx1, wy1, wz1 := int32(t.BaseX), int32(t.BaseY), int32(t.BaseZ)
	wx2 := int32(t.BaseX + t.Data.SizeX - 1)
	wy2 := int32(t.BaseY + t.Data.SizeY - 1)
	wz2 := int32(t.BaseZ + t.Data.SizeZ - 1)

	totalBlocks := int32(t.Data.SizeX * t.Data.SizeY * t.Data.SizeZ)
	var cleared int32

	// TP 到建筑中心加载区块
	cx := (wx1 + wx2) / 2
	cz := (wz1 + wz2) / 2
	if t.Teleport != nil {
		t.Teleport(int(cx), int(wy1), int(cz))
		time.Sleep(50 * time.Millisecond)
	}

	// 只取与建筑 Y 范围相交的段
	for _, seg := range segs {
		sMin := max32(seg[0], wy1)
		sMax := min32(seg[1]-1, wy2)
		if sMin > sMax {
			continue
		}
		// 按 maxFillVolume 切分 Y

		cleared += t.fillSegments(wx1, wz1, wx2, wz2, sMin, sMax, totalBlocks, cleared)
	}
	return nil
}

// clearChunkColumnsOptimized 模式2/3：按 GCD 子区域列清空 + 蛇形
//
//	full=false: 从 BaseY 到世界上限（模式2）
//	full=true:  从维度底到顶（模式3）
func (t *ImportTask) clearChunkColumnsOptimized(segs [][2]int32, subSize int, full bool) error {
	totalCX := (t.Data.SizeX + subSize - 1) / subSize
	totalCZ := (t.Data.SizeZ + subSize - 1) / subSize

	// 先算 total 方块数用于进度
	var totalBlocks int32
	for cx := 0; cx < totalCX; cx++ {
		for cz := 0; cz < totalCZ; cz++ {
			cols := min(subSize, t.Data.SizeX-cx*subSize)
			rows := min(subSize, t.Data.SizeZ-cz*subSize)
			areaPerLayer := int32(cols * rows)
			for _, seg := range segs {
				sMin, sMax := clearYRange(seg, int32(t.BaseY), full)
				if sMin > sMax {
					continue
				}
				totalBlocks += areaPerLayer * (sMax - sMin + 1)
			}
		}
	}

	var cleared int32
	for cx := 0; cx < totalCX; cx++ {
		// 蛇形：偶数行正向，奇数行反向
		var rzStart, rzEnd, rzStep int
		if cx%2 == 0 {
			rzStart, rzEnd, rzStep = 0, totalCZ, 1
		} else {
			rzStart, rzEnd, rzStep = totalCZ-1, -1, -1
		}

		for rz := rzStart; rz != rzEnd; rz += rzStep {
			x1 := int32(t.BaseX + cx*subSize)
			z1 := int32(t.BaseZ + rz*subSize)
			cols := min(subSize, t.Data.SizeX-cx*subSize)
			rows := min(subSize, t.Data.SizeZ-rz*subSize)
			x2 := x1 + int32(cols) - 1
			z2 := z1 + int32(rows) - 1

			// ── TP 到本子区域中心加载区块 ──
			tpX := t.BaseX + cx*subSize + cols/2
			tpZ := t.BaseZ + rz*subSize + rows/2
			if t.Teleport != nil {
				t.Teleport(tpX, t.BaseY, tpZ)
				time.Sleep(50 * time.Millisecond)
			}

			for _, seg := range segs {
				sMin, sMax := clearYRange(seg, int32(t.BaseY), full)
				if sMin > sMax {
					continue
				}
				cleared += t.fillSegments(x1, z1, x2, z2, sMin, sMax, totalBlocks, cleared)
			}
		}
	}
	return nil
}

// clearYRange 计算模式2/3在某个维度段内的 Y 范围
func clearYRange(seg [2]int32, baseY int32, full bool) (int32, int32) {
	sMin := seg[0]
	sMax := seg[1] - 1
	if !full {
		sMin = max32(seg[0], baseY)
	}
	return sMin, sMax
}

// waitChunksLoaded 等待指定 XZ 矩形覆盖的区块全部加载。
// 区块未加载时 fill 命令会失败导致区域清不掉，故清空前需校验。
// ChunkLoaded 未实现时直接返回（不等待）。
func (t *ImportTask) waitChunksLoaded(x1, z1, x2, z2 int32) {
	if t.ChunkLoaded == nil {
		return
	}
	cx1, cx2 := x1/ChunkSize, x2/ChunkSize
	cz1, cz2 := z1/ChunkSize, z2/ChunkSize
	// 最多等待约 3 秒（30 次 × 100ms），防止死循环
	for attempt := 0; attempt < 30; attempt++ {
		allLoaded := true
		for cx := cx1; cx <= cx2; cx++ {
			for cz := cz1; cz <= cz2; cz++ {
				if !t.ChunkLoaded(int(cx), int(cz)) {
					allLoaded = false
					break
				}
			}
			if !allLoaded {
				break
			}
		}
		if allLoaded {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// fillSegments 对指定 XZ 矩形覆盖的 Y 范围分段 fill air
// areaPerLayer 从 x1/x2/z1/z2 推导，无需外部传入。
func (t *ImportTask) fillSegments(x1, z1, x2, z2, yMin, yMax int32, totalBlocks, alreadyCleared int32) int32 {
	// 清空前强制校验区块加载，避免 fill 过快导致区域未清掉
	t.waitChunksLoaded(x1, z1, x2, z2)

	areaPerLayer := (x2 - x1 + 1) * (z2 - z1 + 1)
	maxLayers := maxFillVolume / int(areaPerLayer)
	if maxLayers < 1 {
		maxLayers = 1
	}

	y := yMin
	var segCleared int32
	for y <= yMax {
		endY := min32(y+int32(maxLayers)-1, yMax)
		t.FillRegion(x1, y, z1, x2, endY, z2, "minecraft:air", "[]")
		time.Sleep(10 * time.Millisecond)
		layerCount := endY - y + 1
		segCleared += areaPerLayer * layerCount
		if t.OnClearProgress != nil {
			t.OnClearProgress(int(alreadyCleared+segCleared), int(totalBlocks))
		}
		y = endY + 1
	}
	return segCleared
}

// ── 优化版预清空：结构保存法 ──

// calcSubRegionSize 计算子区域边长 = GCD(SizeX, SizeZ)，不超过64，不低于16。
// 16 保底确保不会出现 GCD=1 时 1×1 的退化情况。
func calcSubRegionSize(sizeX, sizeZ int) int {
	g := gcd(sizeX, sizeZ)
	if g > 64 {
		g = 64
	}
	if g < 16 {
		g = 16
	}
	return g
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func min32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func stripWaterlogged(states string) string {
	if states == "" || states == "[]" {
		return states
	}
	// states format: ["key1"=val1,"waterlogged"=true,"key2"=val2] or ["waterlogged"=true]
	// Remove waterlogged entry entirely so the block places without water
	s := states
	// 1. Remove waterlogged with leading comma (middle/end of state list)
	s = strings.ReplaceAll(s, `,"waterlogged"=true`, "")
	s = strings.ReplaceAll(s, `,"waterlogged"=false`, "")
	s = strings.ReplaceAll(s, `,"waterlogged"=1`, "")
	s = strings.ReplaceAll(s, `,"waterlogged"=0`, "")
	// 2. Remove waterlogged with trailing comma (start of state list)
	s = strings.ReplaceAll(s, `"waterlogged"=true,`, "")
	s = strings.ReplaceAll(s, `"waterlogged"=false,`, "")
	s = strings.ReplaceAll(s, `"waterlogged"=1,`, "")
	s = strings.ReplaceAll(s, `"waterlogged"=0,`, "")
	// 3. Handle sole waterlogged in brackets
	s = strings.ReplaceAll(s, `["waterlogged"=true]`, "[]")
	s = strings.ReplaceAll(s, `["waterlogged"=false]`, "[]")
	s = strings.ReplaceAll(s, `["waterlogged"=1]`, "[]")
	s = strings.ReplaceAll(s, `["waterlogged"=0]`, "[]")
	return s
}
