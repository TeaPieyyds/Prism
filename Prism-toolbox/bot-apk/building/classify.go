package building

import "strings"

// BlockCategory 描述单个实心方块在多机器人导入中的类别。
//
// 它同时是"切岛分类器"和"单元执行"的判定来源：只有 CatWorkspaceNBT 的方块会被
// 从普通单元里抠出（§8.1），其余全部留在普通单元内由任意机器人放置。
// 空容器 / 空展示框 / 空白旗等没有可制作内容的方块归 CatNormal（见
// docs/多机器人作业-详细设计.md §4.1.1），避免空方块被错误地塞进 NBT 单元。
type BlockCategory int

const (
	// CatNormal 纯普通方块（无 NBT 数据、非命令/结构方块），任何机器人可放，无需工作台。
	CatNormal BlockCategory = iota
	// CatWorkspaceNBT 需要工作台（携带可制作内容：有物品/图案/唱片/书等）。
	CatWorkspaceNBT
	// CatSign 告示牌，阶段二放（支持方块就位后）。
	CatSign
	// CatLightNBT 不需要工作台的轻NBT方块（命令方块/结构方块），传送贴近后发包即可。
	CatLightNBT
)

// nbtInt reads an integer value from an NBT map, supporting all numeric NBT types.
// 与 package main 的 nbtInt 逻辑一致；分类器需要独立于此，避免与切岛逻辑耦合。
func nbtInt(m map[string]any, key string) int {
	v := m[key]
	switch val := v.(type) {
	case int32:
		return int(val)
	case int16:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case byte:
		return int(val)
	case float64:
		return int(val)
	}
	return 0
}

// IsWorkspaceNBTBlock 判断该方块是否"需要工作台"（携带可制作内容）。
//
// 这是 §8.1 抠出普通单元的唯一标准，必须与单机导入 OnNBT 的 needsNBT 判定
// 完全一致（task_dispatch.go），否则普通单元与 NBT 单元会重叠或漏放。
// 判定依据是"有没有可制作的内容"，而非"有没有 NBT 数据"：
//   - 空容器（Items 为空或全 air）、空展示框、空白旗、空唱片机 → 返回 false，归普通单元；
//   - 有物品的箱子、有物品的展示框、有图案/底色的旗帜、有唱片的唱片机等 → 返回 true。
func IsWorkspaceNBTBlock(blockName string, nbtData map[string]any) bool {
	if nbtData == nil {
		return false
	}
	// 容器（有物品）→ 需要工作台；空容器/空物品列表 → 否
	if items, ok := nbtData["Items"].([]any); ok {
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				if nbtInt(m, "Count") > 0 && !strings.Contains(stringName(m["Name"]), "air") {
					return true
				}
			}
		}
	}
	switch {
	// 展示框（有物品）
	case strings.Contains(blockName, "frame"):
		_, has := nbtData["Item"]
		return has
	// 唱片机（有唱片）
	case strings.Contains(blockName, "jukebox"):
		_, has := nbtData["RecordItem"]
		return has
	// 旗帜：需工作台织布机的情形必须与工具的 NeedSpecialHandle 一致，否则会"建工作台却保存空气结构"。
	// 工具只处理 Base != 默认色(0/黑) 或有图案(Patterns) 的旗帜；Base==0 的纯黑旗直接 setblock 放即可。
	case strings.Contains(blockName, "banner"):
		if v, ok := nbtData["Base"].(int32); ok {
			if v != 0 {
				return true
			}
		}
		_, has := nbtData["Patterns"]
		return has
	// 讲台：只有带书(book)才需要工作台；空讲台/仅自定义名(CustomName)用 setblock 直接放即可
	case strings.Contains(blockName, "lectern"):
		_, has := nbtData["book"]
		return has
	// 酿造台（有物品）
	case strings.Contains(blockName, "brewing_stand") || strings.Contains(blockName, "brewingstand"):
		items, ok := nbtData["Items"].([]any)
		return ok && len(items) > 0
	// 合成器（有物品）
	case strings.Contains(blockName, "crafter"):
		items, ok := nbtData["Items"].([]any)
		return ok && len(items) > 0
	}
	return false
}

// ClassifyBlock 返回一个实心方块在导入中的类别。
// 告示牌优先（阶段二）；其次判断是否"工作区NBT"（需工作台）；再判断是否"轻NBT"
// （命令/结构方块，不需工作台）；其余归普通。
func ClassifyBlock(blockName string, nbtData map[string]any) BlockCategory {
	if strings.Contains(blockName, "sign") {
		return CatSign
	}
	if IsWorkspaceNBTBlock(blockName, nbtData) {
		return CatWorkspaceNBT
	}
	// 命令方块 / 结构方块：不需要工作台，传送贴近后发包即可，独立成单元
	if strings.Contains(blockName, "command_block") || strings.Contains(blockName, "structure_block") {
		return CatLightNBT
	}
	return CatNormal
}

// stringName 从 any 安全读取字符串值；非字符串返回空串。
func stringName(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
