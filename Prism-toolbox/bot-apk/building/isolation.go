package building

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// WorkspaceY NBT 工作台（铁砧/织布机操作台）所在的 Y 高度。
const WorkspaceY = 310

// NBTStructName 生成跨机器人唯一、跨任务唯一的 structure 名（§8.2）。
//
// 多台机器人同时 structure save/load，若存同名结构会互相读到旧数据。旧代码用每任务
// 独立的 nbtStructSeq 防撞，但多机器人各自独立计数会撞名。改为"机器人编号 + 序号 +
// 纯随机后缀"：前缀保证不同机器人/不同任务确定性不同，随机后缀进一步防止并发瞬间撞名。
func NBTStructName(botIdx int, cleanName string, seq int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 10)
	max := big.NewInt(int64(len(chars)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			b[i] = 'a'
			continue
		}
		b[i] = chars[n.Int64()]
	}
	return fmt.Sprintf("bot%d_%d_%s", botIdx, seq, string(b))
}

// WorkspaceCenter 分配第 botIdx 个机器人的 NBT 工作台中心坐标（§8.3），互不重叠。
//
// 工作台放在建筑东北角外（baseX+sizeX+3 起），按编号错开 ≥16 格铺开；每个工作台
// 占约 11×11（runImportTask 里会以中心 ±5 清理），16 格间距保证两两不重叠。返回
// (x, WorkspaceY, z)。
func WorkspaceCenter(botIdx, baseX, baseZ, sizeX, sizeZ int) (int, int, int) {
	const spacing = 16
	col := botIdx % 8 // 每行 8 个，避免一条线拉太长
	row := botIdx / 8
	x := baseX + sizeX + 3 + col*spacing
	z := baseZ + sizeZ + 3 + row*spacing
	return x, WorkspaceY, z
}
