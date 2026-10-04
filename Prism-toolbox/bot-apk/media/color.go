package media

import (
	"fmt"
	"github.com/lucasb-eyer/go-colorful"
)

type colorBlock struct {
	Name string
	C    colorful.Color
}

var colorBlocks = []colorBlock{
	{"minecraft:white_concrete", colorful.Color{R: 207, G: 213, B: 214}},
	{"minecraft:orange_concrete", colorful.Color{R: 224, G: 97, B: 1}},
	{"minecraft:magenta_concrete", colorful.Color{R: 169, G: 48, B: 159}},
	{"minecraft:light_blue_concrete", colorful.Color{R: 35, G: 137, B: 199}},
	{"minecraft:yellow_concrete", colorful.Color{R: 241, G: 175, B: 21}},
	{"minecraft:lime_concrete", colorful.Color{R: 94, G: 169, B: 24}},
	{"minecraft:pink_concrete", colorful.Color{R: 214, G: 101, B: 143}},
	{"minecraft:gray_concrete", colorful.Color{R: 54, G: 57, B: 61}},
	{"minecraft:light_gray_concrete", colorful.Color{R: 125, G: 125, B: 115}},
	{"minecraft:cyan_concrete", colorful.Color{R: 21, G: 119, B: 136}},
	{"minecraft:purple_concrete", colorful.Color{R: 100, G: 32, B: 156}},
	{"minecraft:blue_concrete", colorful.Color{R: 45, G: 47, B: 143}},
	{"minecraft:brown_concrete", colorful.Color{R: 96, G: 60, B: 32}},
	{"minecraft:green_concrete", colorful.Color{R: 73, G: 91, B: 36}},
	{"minecraft:red_concrete", colorful.Color{R: 142, G: 33, B: 33}},
	{"minecraft:black_concrete", colorful.Color{R: 8, G: 10, B: 15}},

	{"minecraft:white_wool", colorful.Color{R: 234, G: 236, B: 236}},
	{"minecraft:orange_wool", colorful.Color{R: 241, G: 118, B: 19}},
	{"minecraft:magenta_wool", colorful.Color{R: 190, G: 68, B: 179}},
	{"minecraft:light_blue_wool", colorful.Color{R: 58, G: 175, B: 217}},
	{"minecraft:yellow_wool", colorful.Color{R: 249, G: 198, B: 40}},
	{"minecraft:lime_wool", colorful.Color{R: 112, G: 185, B: 25}},
	{"minecraft:pink_wool", colorful.Color{R: 237, G: 141, B: 172}},
	{"minecraft:gray_wool", colorful.Color{R: 62, G: 68, B: 71}},
	{"minecraft:light_gray_wool", colorful.Color{R: 141, G: 141, B: 134}},
	{"minecraft:cyan_wool", colorful.Color{R: 21, G: 139, B: 145}},
	{"minecraft:purple_wool", colorful.Color{R: 121, G: 42, B: 172}},
	{"minecraft:blue_wool", colorful.Color{R: 53, G: 57, B: 157}},
	{"minecraft:brown_wool", colorful.Color{R: 113, G: 74, B: 42}},
	{"minecraft:green_wool", colorful.Color{R: 84, G: 110, B: 27}},
	{"minecraft:red_wool", colorful.Color{R: 161, G: 39, B: 35}},
	{"minecraft:black_wool", colorful.Color{R: 20, G: 22, B: 26}},

	{"minecraft:snow", colorful.Color{R: 241, G: 245, B: 250}},
	{"minecraft:clay", colorful.Color{R: 159, G: 164, B: 177}},
	{"minecraft:dirt", colorful.Color{R: 111, G: 85, B: 52}},
	{"minecraft:stone", colorful.Color{R: 125, G: 125, B: 125}},
	{"minecraft:cobblestone", colorful.Color{R: 119, G: 119, B: 119}},
	{"minecraft:sand", colorful.Color{R: 219, G: 207, B: 160}},
	{"minecraft:obsidian", colorful.Color{R: 15, G: 10, B: 24}},
	{"minecraft:grass_block", colorful.Color{R: 92, G: 120, B: 42}},
	{"minecraft:oak_planks", colorful.Color{R: 162, G: 131, B: 79}},
	{"minecraft:brick_block", colorful.Color{R: 149, G: 103, B: 86}},
	{"minecraft:glowstone", colorful.Color{R: 145, G: 117, B: 69}},
	{"minecraft:netherrack", colorful.Color{R: 111, G: 54, B: 53}},
	{"minecraft:soul_sand", colorful.Color{R: 75, G: 57, B: 46}},
	{"minecraft:bookshelf", colorful.Color{R: 120, G: 91, B: 57}},
	{"minecraft:end_stone", colorful.Color{R: 219, G: 222, B: 158}},
	{"minecraft:quartz_block", colorful.Color{R: 236, G: 233, B: 227}},
	{"minecraft:coal_block", colorful.Color{R: 16, G: 16, B: 16}},
	{"minecraft:iron_block", colorful.Color{R: 219, G: 219, B: 219}},
	{"minecraft:gold_block", colorful.Color{R: 247, G: 233, B: 79}},
	{"minecraft:diamond_block", colorful.Color{R: 98, G: 219, B: 209}},
	{"minecraft:emerald_block", colorful.Color{R: 87, G: 211, B: 68}},
	{"minecraft:redstone_block", colorful.Color{R: 171, G: 28, B: 9}},
	{"minecraft:lapis_block", colorful.Color{R: 35, G: 67, B: 163}},
	{"minecraft:packed_ice", colorful.Color{R: 141, G: 181, B: 251}},
}

// BlockHexColor returns the hex color string for a given block name (e.g. "#cfd5d6").
// 优先使用 colorBlocks 精确值，fallback 到 colors.json 的中色值。
func BlockHexColor(name string) string {
	for _, cb := range colorBlocks {
		if cb.Name == name {
			return fmt.Sprintf("#%02x%02x%02x", uint8(cb.C.R), uint8(cb.C.G), uint8(cb.C.B))
		}
	}
	if c, ok := BlockMidColor(name); ok {
		return fmt.Sprintf("#%02x%02x%02x", uint8(c.R), uint8(c.G), uint8(c.B))
	}
	return "#e94560"
}

// ClosestBlockForColor finds the best Minecraft block for an RGB color
// using CIELAB distance for perceptually accurate matching.
// 优先匹配 colors.json 中的完整颜色表（~210色），fallback 到 colorBlocks（72色）。
func ClosestBlockForColor(r, g, b uint8) string {
	target := colorful.Color{R: float64(r), G: float64(g), B: float64(b)}
	best := "minecraft:stone"
	bestDist := 1e10
	// 先用 colors.json 的完整表匹配
	if len(mapColorTable) > 0 {
		for _, m := range mapColorTable {
			dist := target.DistanceLab(m.c)
			if dist < bestDist {
				bestDist = dist
				best = m.name
			}
		}
		return best
	}
	// fallback 到 colorBlocks
	for _, cb := range colorBlocks {
		dist := target.DistanceLab(cb.C)
		if dist < bestDist {
			bestDist = dist
			best = cb.Name
		}
	}
	return best
}
