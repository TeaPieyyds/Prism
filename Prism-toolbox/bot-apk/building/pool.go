package building

import (
	"sync"
	"time"
)

// UnitState 任务单元的运行状态（§14.1 断点粒度：pending / in_progress / done / failed）。
type UnitState int

const (
	UnitPending UnitState = iota
	UnitInProgress
	UnitDone
	// UnitFailed 该单元多次放置失败后判失败。方块未真正放置，但为了让工人池能收敛、
	// 不无限重做，把它从 pending 里移出；它不会被标记 done，因此进度永远到不了 100%，
	// 完成上报也会如实反映缺失量（§14.4：宁可重做，也不许"标记了却没放成"）。
	UnitFailed
)

// WorkItem 一个带运行状态的任务单元。
type WorkItem struct {
	Unit  *Unit
	state UnitState
	// fails 记录该单元连续失败的次数。Requeue 时累加；超过上限后判失败（§14.4），
	// 避免某个方块持续失败导致机器人反复重做同一单元（"反复导入"）。
	fails int
	// claimedAt 该单元被哪个机器人认领的时间。超时看门狗据此回收卡死的 in_progress 单元
	// （机器人断线但 WritePacket 未立即报错时，单元会永久卡在 in_progress）。
	claimedAt time.Time
}

// WorkPool 是工人池调度器（工作窃取）的核心：
//   - 持有阶段一（普通 + NBT）与阶段二（告示牌）的单元列表与各自状态；
//   - Claim：空闲机器人原子地领走下一个它够格的单元（工作窃取，快的自然多干）；
//   - 阶段二只在阶段一全部完成后再开放；
//   - 能力不足的工人（没工作台）在普通干完后会被提示"建工作台援助 NBT"（§7 降级/援助）。
//
// 本层只管理"谁领哪个单元、状态怎么流转"，不执行放置；执行由调用方注入的 worker 完成
// （package main 里每个 BotManager 一个 goroutine 循环 Claim→执行→Complete）。
type WorkPool struct {
	mu      sync.Mutex
	normals    []*WorkItem
	nbt        []*WorkItem
	signs      []*WorkItem
	lightNBTs  []*WorkItem

	// 进度聚合（按任务算，§6）：每类单元已完成 / 总数方块。
	totalNormal, doneNormal int
	totalNBT, doneNBT       int
	totalSign, doneSign     int
	totalLight, doneLight   int

	// 判失败单元的总方块数（缺失量）。判失败不累加 done，因此进度不会虚高；
	// 完成上报时用这些值如实反映"没放成的量"。
	failedNormal, failedNBT, failedSign, failedLight int

	// OnProgress 在每次单元完成时回调各类进度（0~1）。可空。
	OnProgress func(normal, nbt, sign, light float64)
}

// Claim 一次领活的结果。
type Claim struct {
	// Item 非 nil 表示领到一个单元，worker 应执行它并随后 Complete。
	Item *WorkItem
}

// NewWorkPool 从切分结果构建工人池。
func NewWorkPool(res *SplitResult) *WorkPool {
	p := &WorkPool{}
	if res == nil {
		return p
	}
	for _, u := range res.Units {
		wi := &WorkItem{Unit: u}
		switch u.Kind {
		case UnitNormal:
			p.normals = append(p.normals, wi)
			p.totalNormal += u.BlockCount
		case UnitNBT:
			p.nbt = append(p.nbt, wi)
			p.totalNBT += u.BlockCount
		case UnitSign:
			p.signs = append(p.signs, wi)
			p.totalSign += u.BlockCount
		case UnitLightNBT:
			p.lightNBTs = append(p.lightNBTs, wi)
			p.totalLight += u.BlockCount
		}
	}
	return p
}

// Claim 领一个任务。
// prefType 是机器人主类型（优先领取该类型，保持工作连续性）；curCX/curCZ 是机器人当前位置
// （区块列坐标），用于"距离最近优先"，减少传送距离、保证区块能加载、不重复传送。
// 升降级 = 工作窃取的自然结果：主类型没有 pending 时，自动领其他类型
// （按 普通 > 命令 > NBT 的支援顺序），所以"NBT 机器人降级做普通"、"普通机器人升级做 NBT"
// 都不需要显式状态机，由这个规则自动发生。
// 告示牌（阶段二）永远在阶段一全部完成后才开放。
func (p *WorkPool) Claim(prefType UnitKind, curCX, curCZ int) Claim {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.allPhase1DoneLocked() {
		for _, k := range phase1Order(prefType) {
			var list []*WorkItem
			switch k {
			case UnitNormal:
				list = p.normals
			case UnitLightNBT:
				list = p.lightNBTs
			case UnitNBT:
				list = p.nbt
			}
			if it := pickNearestPending(list, curCX, curCZ); it != nil {
				it.state = UnitInProgress
				it.claimedAt = time.Now()
				return Claim{Item: it}
			}
		}
		return Claim{}
	}
	if it := pickNearestPending(p.signs, curCX, curCZ); it != nil {
		it.state = UnitInProgress
		it.claimedAt = time.Now()
		return Claim{Item: it}
	}
	return Claim{}
}

// phase1Order 返回阶段一类型的认领优先级：主类型优先，其余按 普通>命令>NBT 支援顺序。
func phase1Order(pref UnitKind) []UnitKind {
	others := []UnitKind{UnitNormal, UnitLightNBT, UnitNBT}
	order := make([]UnitKind, 0, len(others))
	for _, o := range others {
		if o == pref {
			order = append(order, o)
			break
		}
	}
	for _, o := range others {
		if o != pref {
			order = append(order, o)
		}
	}
	return order
}

// pickNearestPending 从 pending 单元里挑"质心距机器人当前位置最近"的（距离近 → 区块加载好、少传送）。
func pickNearestPending(list []*WorkItem, curCX, curCZ int) *WorkItem {
	var best *WorkItem
	bestD2 := -1
	for _, it := range list {
		if it.state != UnitPending {
			continue
		}
		cx, cz := unitCentroidInt(it.Unit)
		d2 := (cx-curCX)*(cx-curCX) + (cz-curCZ)*(cz-curCZ)
		if bestD2 < 0 || d2 < bestD2 {
			bestD2 = d2
			best = it
		}
	}
	return best
}

// unitCentroidInt 返回单元质心的区块列整数坐标。
func unitCentroidInt(u *Unit) (int, int) {
	cx, cz := unitCentroid(u)
	return int(cx), int(cz)
}

// Complete 标记单元完成，累加已完成方块并触发进度回调。
// blocks 为该单元本次实际完成的方块数（用于进度加权，§6 按任务算）。
func (p *WorkPool) Complete(it *WorkItem, blocks int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	it.state = UnitDone
	switch it.Unit.Kind {
	case UnitNormal:
		p.doneNormal += blocks
	case UnitNBT:
		p.doneNBT += blocks
	case UnitSign:
		p.doneSign += blocks
	case UnitLightNBT:
		p.doneLight += blocks
	}
	if p.OnProgress != nil {
		p.OnProgress(pct(p.doneNormal, p.totalNormal), pct(p.doneNBT, p.totalNBT),
			pct(p.doneSign, p.totalSign), pct(p.doneLight, p.totalLight))
	}
}

// Requeue 把执行失败的单元改回 pending（重做幂等，§14）。不做方块粒度位图。
// 返回 false 表示该单元已连续失败超过 MaxUnitFails 次：此时它被标记为 UnitFailed，
// **不会**被当成完成（§14.4 底线：不许"标记了却没放成"），后续完成上报会如实反映缺失。
// 调用方收到 false 时应如实上报失败，而不是调 Complete。
func (p *WorkPool) Requeue(it *WorkItem) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	it.fails++
	if it.fails >= MaxUnitFails {
		p.markFailedLocked(it)
		return false
	}
	if it.state != UnitDone {
		it.state = UnitPending
	}
	return true
}

// Release 把仍处于 in_progress 的单元改回 pending，用于机器人断线后的单元回收（§14.3）。
// 断线不是方块级失败，因此不累加 fails，也不判失败——其他机器人重新抢做（幂等）。
func (p *WorkPool) Release(it *WorkItem) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if it.state == UnitInProgress {
		it.state = UnitPending
	}
}

// markFailedLocked 把单元判为失败并累加缺失方块数（需持有锁）。
func (p *WorkPool) markFailedLocked(it *WorkItem) {
	it.state = UnitFailed
	switch it.Unit.Kind {
	case UnitNormal:
		p.failedNormal += it.Unit.BlockCount
	case UnitNBT:
		p.failedNBT += it.Unit.BlockCount
	case UnitSign:
		p.failedSign += it.Unit.BlockCount
	case UnitLightNBT:
		p.failedLight += it.Unit.BlockCount
	}
	if p.OnProgress != nil {
		p.OnProgress(pct(p.doneNormal, p.totalNormal), pct(p.doneNBT, p.totalNBT),
			pct(p.doneSign, p.totalSign), pct(p.doneLight, p.totalLight))
	}
}

// FailedBlocks 返回所有判失败单元的总方块数（真实缺失量，用于完成上报）。
func (p *WorkPool) FailedBlocks() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.failedNormal + p.failedNBT + p.failedSign + p.failedLight
}

// RemainingUnits 返回仍未完成的单元数（pending + in_progress），用于判断是否有遗漏。
func (p *WorkPool) RemainingUnits() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, l := range [][]*WorkItem{p.normals, p.nbt, p.signs, p.lightNBTs} {
		for _, it := range l {
			if it.state == UnitPending || it.state == UnitInProgress {
				n++
			}
		}
	}
	return n
}

// MaxUnitFails 单元连续失败多少次后放弃重做（判失败跳过，不再 Requeue）。
const MaxUnitFails = 3

// HasLightNBT 报告是否有命令/结构方块任务（用于角色分配：有则分一台机器人负责）。
func (p *WorkPool) HasLightNBT() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.lightNBTs) > 0
}

// Content 报告各类型任务是否存在。用于多机器人进度条动态显示：
// 只有普通建筑时只显示普通一行，避免空 NBT/命令/告示牌行占位。
func (p *WorkPool) Content() (hasNormal, hasNBT, hasSign, hasLight bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.normals) > 0, len(p.nbt) > 0, len(p.signs) > 0, len(p.lightNBTs) > 0
}

// HasNBT 报告是否有工作区NBT任务（用于角色分配）。
func (p *WorkPool) HasNBT() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.nbt) > 0
}

// Done 报告全部单元（阶段一 + 阶段二）是否完成。
func (p *WorkPool) Done() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.allPhase1DoneLocked() && p.signsDoneLocked()
}

// Progress 返回各类进度（0~1）。
func (p *WorkPool) Progress() (normal, nbt, sign, light float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return pct(p.doneNormal, p.totalNormal), pct(p.doneNBT, p.totalNBT),
		pct(p.doneSign, p.totalSign), pct(p.doneLight, p.totalLight)
}

// ProgressBlocks 返回各类型已完方块数与总方块数（整数，供 actionbar 补零对齐显示）。
func (p *WorkPool) ProgressBlocks() (normalDone, normalTotal, nbtDone, nbtTotal, signDone, signTotal, lightDone, lightTotal int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.doneNormal, p.totalNormal, p.doneNBT, p.totalNBT,
		p.doneSign, p.totalSign, p.doneLight, p.totalLight
}

// Phase1Done 报告阶段一（普通/NBT/轻NBT）是否全部了结（done 或 failed）。
// 供 actionbar 决定阶段一合并时机（§阶段一完成后去掉普通/NBT/命令行，只留告示牌）。
func (p *WorkPool) Phase1Done() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.allPhase1DoneLocked()
}

func (p *WorkPool) allPhase1DoneLocked() bool {
	for _, it := range p.normals {
		if it.state != UnitDone && it.state != UnitFailed {
			return false
		}
	}
	for _, it := range p.nbt {
		if it.state != UnitDone && it.state != UnitFailed {
			return false
		}
	}
	for _, it := range p.lightNBTs {
		if it.state != UnitDone && it.state != UnitFailed {
			return false
		}
	}
	return true
}

func (p *WorkPool) signsDoneLocked() bool {
	for _, it := range p.signs {
		if it.state != UnitDone && it.state != UnitFailed {
			return false
		}
	}
	return true
}

func pct(done, total int) float64 {
	if total == 0 {
		return 1
	}
	return float64(done) / float64(total)
}
