package building

import "sort"

// UnitKind 任务单元的类型。普通/NBT 属于阶段一，告示牌属于阶段二。
type UnitKind int

const (
	// UnitNormal 纯普通方块单元（无 NBT 数据），按 16×16 区块列切岛。
	UnitNormal UnitKind = iota
	// UnitNBT 需要工作台的工作区NBT方块单元（有可制作内容，§8.1 从普通单元抠出）。
	UnitNBT
	// UnitSign 告示牌单元，阶段二放（支持方块就位后）。
	UnitSign
	// UnitLightNBT 不需要工作台的轻NBT方块单元（命令/结构方块），传送贴近后发包。
	UnitLightNBT
)

// Unit 一个相互独立的任务单元。所有单元共享同一份 StructureData（§11.1 内存红线），
// 单元只是指向其中的区块范围 / 方块位置，绝不复制数据。
type Unit struct {
	ID   int
	Kind UnitKind
	// Chunks：普通单元的 16×16 区块列（结构局部坐标），用于限制放置引擎只放本单元。
	Chunks [][2]int
	// Positions：NBT/告示牌单元的具体方块位置（结构局部坐标）。
	Positions [][3]int
	// Skipped：普通单元列内被抠出的工作区NBT位置（§8.1）。一个 16×16 列里可能同时有
	// 普通方块和满箱子，普通单元按列放置时必须**跳过**这些位置，留给 NBT 单元放，
	// 否则普通机器人会用一个普通箱子覆盖 NBT 机器人放好的满箱子。
	Skipped [][3]int
	// BlockCount 该单元负责的实心方块数，用于排序与进度加权。
	BlockCount int
}

// SplitOptions 控制切岛与拆分的阈值。零值字段在 SplitStructure 内填默认值。
type SplitOptions struct {
	// MassiveNBTRatio 海量 NBT 阈值：NBT 方块数 ÷ 全部实心方块数 ≥ 该值 ⇒ 拆多份并行。
	// 默认 0.10（§4.3）。
	MassiveNBTRatio float64
	// MaxUnitChunks 单机/默认模式下每个普通单元最多区块列数（BotCount≤1 时用于把普通任务
	// 切出多个单元以便并行）。多机器人模式下由 BotCount 决定大工作区划分，本字段忽略。
	// 默认 24。
	MaxUnitChunks int
	// NBTUnitTarget 海量 NBT 时每个 NBT 单元的目标方块数（按位置分批）。默认 64。
	NBTUnitTarget int
	// SignBatchSize 每个告示牌单元的目标方块数（阶段二并行放置分批）。默认 64。
	SignBatchSize int
	// BotCount 参与导入的机器人数量。>1 时：
	//   1) 普通任务按机器人数量均分成"蛇形大工作区"（每个普通机器人负责一片连续区域，少传送、区块加载好）；
	//   2) 把 NBT 按机器人数量拆成多单元，让干完普通的机器人能立刻并行援助 NBT（工作窃取，§7）。
	BotCount int
}

// SplitResult 切分结果。
type SplitResult struct {
	// Units 全部单元：阶段一（普通单元按方块数降序 → NBT 单元）在前，阶段二（告示牌）在后。
	Units []*Unit
	// TotalSolid 全部实心方块数（含 NBT 与告示牌），用于海量阈值与进度分母。
	TotalSolid int
	// NormalSolid 普通单元覆盖的实心方块数。
	NormalSolid int
	// NBTBlockCount 工作区NBT方块数。
	NBTBlockCount int
	// SignCount 告示牌方块数。
	SignCount int
	// LightNBTBlockCount 轻NBT（命令/结构）方块数。
	LightNBTBlockCount int
	// IsMassiveNBT 是否达到海量 NBT 阈值（§4.3）。
	IsMassiveNBT bool
}

// SplitStructure 把解析后的建筑切成相互独立的任务单元（按内容切岛 + 工作区NBT抠出 + 告示牌单独）。
//
// 正确性底线（§8.1）：每个实心方块恰好属于一个单元；工作区NBT方块只进 NBT 单元，
// 普通单元不含任何需要工作台的方块位置，绝无重叠。
func SplitStructure(data *StructureData, opts SplitOptions) (*SplitResult, error) {
	if data == nil {
		return nil, errNoData
	}
	if opts.MassiveNBTRatio <= 0 {
		opts.MassiveNBTRatio = 0.10
	}
	if opts.MaxUnitChunks <= 0 {
		// 降低默认值让建筑切出更多单元，多机器人才能真正并行（每个单元一台机器人负责）。
		// 原 96（≈384×384 方块）太大，普通建筑都只有一个单元 → 只有一台机器人干活。
		opts.MaxUnitChunks = 24
	}
	if opts.NBTUnitTarget <= 0 {
		opts.NBTUnitTarget = 64
	}
	if opts.SignBatchSize <= 0 {
		// 降小让告示牌拆成更多单元，多台机器人并行放置告示牌
		opts.SignBatchSize = 16
	}

	res := &SplitResult{}
	colSolid := make(map[[2]int]int) // 普通方块的 区块列 → 方块数
	var nbtPos, signPos, lightNBTPos [][3]int

	for _, bp := range data.AllBlocks() {
		info := &data.Palette[bp.Index]
		if info.IsAir {
			continue
		}
		nbt := data.GetNBT(int(bp.X), int(bp.Y), int(bp.Z))
		cat := ClassifyBlock(info.Name, nbt)
		pos := [3]int{int(bp.X), int(bp.Y), int(bp.Z)}
		switch cat {
		case CatWorkspaceNBT:
			nbtPos = append(nbtPos, pos)
			res.NBTBlockCount++
		case CatSign:
			signPos = append(signPos, pos)
			res.SignCount++
		case CatLightNBT:
			lightNBTPos = append(lightNBTPos, pos)
			res.LightNBTBlockCount++
		default: // CatNormal 纯普通方块
			cx, cz := pos[0]/ChunkSize, pos[2]/ChunkSize
			colSolid[[2]int{cx, cz}]++
			res.NormalSolid++
		}
		res.TotalSolid++
	}

	// 海量 NBT 判定（§4.3）
	if res.TotalSolid > 0 {
		res.IsMassiveNBT = float64(res.NBTBlockCount)/float64(res.TotalSolid) >= opts.MassiveNBTRatio
	}

	// 建"区块列 → 该列内普通单元要跳过的位置"索引（§8.1）：
	// 普通单元既不放被抠出的工作区NBT、也不放轻NBT（命令/结构）和告示牌，
	// 这些位置各归其独立单元。
	skipByCol := make(map[[2]int][][3]int, len(nbtPos)+len(signPos)+len(lightNBTPos))
	for _, p := range nbtPos {
		c := [2]int{p[0] / ChunkSize, p[2] / ChunkSize}
		skipByCol[c] = append(skipByCol[c], p)
	}
	for _, p := range signPos {
		c := [2]int{p[0] / ChunkSize, p[2] / ChunkSize}
		skipByCol[c] = append(skipByCol[c], p)
	}
	for _, p := range lightNBTPos {
		c := [2]int{p[0] / ChunkSize, p[2] / ChunkSize}
		skipByCol[c] = append(skipByCol[c], p)
	}

	// ── 普通单元：按非空区块列划分蛇形大工作区 ──
	// 每个大工作区包含"相同数量的有价值区域（非空气区块列）"，空间蛇形相连，
	// 让每个普通机器人像单机一样蛇形处理完自己大工作区的所有区块：
	// 有序、传送距离短、区块能加载（局部性，§11.1），避免方块因区块未加载而丢失，
	// 也避免多机器人交替传送到同一区域互相干扰。
	nextID := 0
	cols := make([][2]int, 0, len(colSolid))
	for c := range colSolid {
		cols = append(cols, c)
	}
	sortChunkColumnsSerpentine(cols)

	// 每份（大工作区）的区块列数：BotCount>1 时按机器人数量均分（每台一份），
	// 否则（单机/默认）按 MaxUnitChunks 均分，保证也能切出多个单元并行。
	perWork := opts.MaxUnitChunks
	if opts.BotCount > 1 {
		perWork = (len(cols) + opts.BotCount - 1) / opts.BotCount
	}
	if perWork < 1 {
		perWork = 1
	}
	for start := 0; start < len(cols); start += perWork {
		end := start + perWork
		if end > len(cols) {
			end = len(cols)
		}
		u := &Unit{ID: nextID, Kind: UnitNormal, Chunks: cols[start:end]}
		nextID++
		for _, c := range u.Chunks {
			u.BlockCount += colSolid[c]
			if ps, ok := skipByCol[c]; ok {
				u.Skipped = append(u.Skipped, ps...)
			}
		}
		res.Units = append(res.Units, u)
	}
	// 单元按空间蛇形排序：相邻单元空间相连，机器人沿蛇形路径连续工作（§10）。
	sortUnitsSerpentine(res.Units)

	// ── NBT 单元：按机器人数量拆多份并行援助（§7 工作窃取 = 自动援助）。
	// 只要 NBT 方块数 ≥ 机器人数量，就让每台机器人各领一个 NBT 单元，谁干完普通谁就并行
	// 制作 NBT——避免一台机器人独扛全部 NBT 成为瓶颈、其他机器人却无 NBT 可领（"援助机制失效"）。
	// 若 NBT 很少（< 机器人数）才收敛为少数单元，避免拆太碎。
	if len(nbtPos) > 0 {
		sortNBT(nbtPos)
		batch := opts.NBTUnitTarget
		if batch <= 0 {
			batch = 64
		}
		if opts.BotCount > 1 && len(nbtPos) >= opts.BotCount {
			// 按机器人数量均分，让每台机器人都有 NBT 单元可领（援助）
			batch = (len(nbtPos) + opts.BotCount - 1) / opts.BotCount
			if batch < 1 {
				batch = 1
			}
		}
		for start := 0; start < len(nbtPos); start += batch {
			end := start + batch
			if end > len(nbtPos) {
				end = len(nbtPos)
			}
			u := &Unit{ID: nextID, Kind: UnitNBT, Positions: nbtPos[start:end]}
			nextID++
			u.BlockCount = len(u.Positions)
			res.Units = append(res.Units, u)
		}
	}

	// ── 轻NBT单元（命令/结构方块，不需工作台，按位置分批以便并行） ──
	if len(lightNBTPos) > 0 {
		sortNBT(lightNBTPos)
		for start := 0; start < len(lightNBTPos); start += opts.NBTUnitTarget {
			end := start + opts.NBTUnitTarget
			if end > len(lightNBTPos) {
				end = len(lightNBTPos)
			}
			u := &Unit{ID: nextID, Kind: UnitLightNBT, Positions: lightNBTPos[start:end]}
			nextID++
			u.BlockCount = len(u.Positions)
			res.Units = append(res.Units, u)
		}
	}

	// ── 告示牌单元（阶段二，按位置分批以便并行放置） ──
	if len(signPos) > 0 {
		sortNBT(signPos)
		for start := 0; start < len(signPos); start += opts.SignBatchSize {
			end := start + opts.SignBatchSize
			if end > len(signPos) {
				end = len(signPos)
			}
			u := &Unit{ID: nextID, Kind: UnitSign, Positions: signPos[start:end]}
			nextID++
			u.BlockCount = len(u.Positions)
			res.Units = append(res.Units, u)
		}
	}

	return res, nil
}

var errNoData = newSplitError("no structure data")

type splitError string

func (e splitError) Error() string { return string(e) }

func newSplitError(msg string) error { return splitError(msg) }

// groupChunkColumns 用 BFS 把相邻的非空区块列连成"岛"（4 邻接，共享边）。
// 只返回含至少一个普通方块的区块列；空区域不产生任何组。
func groupChunkColumns(colSolid map[[2]int]int) [][][2]int {
	visited := make(map[[2]int]bool, len(colSolid))
	var islands [][][2]int
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for col := range colSolid {
		if visited[col] {
			continue
		}
		visited[col] = true
		queue := [][2]int{col}
		var group [][2]int
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			group = append(group, cur)
			for _, d := range dirs {
				nb := [2]int{cur[0] + d[0], cur[1] + d[1]}
				if colSolid[nb] > 0 && !visited[nb] {
					visited[nb] = true
					queue = append(queue, nb)
				}
			}
		}
		islands = append(islands, group)
	}
	return islands
}

// sortChunkColumnsSerpentine 按空间蛇形排序区块列：先按 Z 分行，行内 X 蛇形交替方向。
// 相邻大工作区空间相连，机器人沿蛇形路径连续工作、传送距离短、区块能随推进加载。
func sortChunkColumnsSerpentine(cols [][2]int) {
	sort.SliceStable(cols, func(i, j int) bool {
		a, b := cols[i], cols[j]
		if a[1] != b[1] { // z 行
			return a[1] < b[1]
		}
		if a[1]%2 == 0 { // 偶数行：左→右
			return a[0] < b[0]
		}
		return a[0] > b[0] // 奇数行：右→左
	})
}

// sortNBT 按 y→x→z 稳定排序位置，保证确定性。
func sortNBT(positions [][3]int) {
	sort.Slice(positions, func(i, j int) bool {
		a, b := positions[i], positions[j]
		if a[1] != b[1] {
			return a[1] < b[1]
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		return a[2] < b[2]
	})
}

// CentroidInt 返回单元质心的区块列整数坐标。供外部（认领时距离最近）使用。
func (u *Unit) CentroidInt() (int, int) {
	return unitCentroidInt(u)
}

// unitCentroid 返回单元的空间质心（区块列坐标），用于蛇形排序。
func unitCentroid(u *Unit) (cx, cz float64) {
	if len(u.Chunks) > 0 {
		for _, c := range u.Chunks {
			cx += float64(c[0])
			cz += float64(c[1])
		}
		n := float64(len(u.Chunks))
		return cx / n, cz / n
	}
	if len(u.Positions) > 0 {
		for _, p := range u.Positions {
			cx += float64(p[0] / ChunkSize)
			cz += float64(p[2] / ChunkSize)
		}
		n := float64(len(u.Positions))
		return cx / n, cz / n
	}
	return 0, 0
}

// sortUnitsSerpentine 按空间蛇形排序单元：先按 Z 分行，行内按 X 蛇形交替方向。
// 这样相邻单元空间相连，机器人沿蛇形路径连续工作，不跳区域。
func sortUnitsSerpentine(units []*Unit) {
	sort.SliceStable(units, func(i, j int) bool {
		ax, az := unitCentroid(units[i])
		bx, bz := unitCentroid(units[j])
		rowA := int(az)
		rowB := int(bz)
		if rowA != rowB {
			return rowA < rowB
		}
		// 同一条"行"内：偶数行从左到右，奇数行从右到左
		if rowA%2 == 0 {
			return ax < bx
		}
		return ax > bx
	})
}
