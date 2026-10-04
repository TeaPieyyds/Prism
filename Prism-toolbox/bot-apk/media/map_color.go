// 包 media 提供地图画（像素画）的三级明暗匹配功能。
//
// Minecraft 地图画是一种特殊建筑结构：将一张图片转换为
// 128×128（或更大）的方块阵列，在地图上渲染时看起来像一幅画。
//
// 本文件实现的核心功能：
// 1. 加载 Minecraft 地图颜色映射表（colors.json）
// 2. 将每个像素的颜色匹配到最近的 Minecraft 方块
// 3. 返回方块名和高度偏移（三级明暗），让地图画有立体感
//
// 三级明暗原理：
// Minecraft 地图上的每个像素可以有 3 种明暗度——亮色/中色/暗色，
// 对应方块放置在不同高度。通过 Y 轴的微小平移（+1/0/-1），
// 地图渲染时会自动产生光照渐变效果，让平坦的地图画看起来有立体起伏。
package media

import (
	_ "embed"
	"encoding/json"
	"image"

	"github.com/lucasb-eyer/go-colorful"
)

// 三级明暗的常量 ID。
// 每个 Minecraft 方块在地图上对应 3 个颜色值，分别表示：
//   - heightHigher: 亮色（方块抬高 1 格，看起来更亮）
//   - heightMiddle: 中色（基准高度，方块原地放置）
//   - heightLower:  暗色（方块降低 1 格，看起来更暗）
const (
	heightHigher = 0
	heightMiddle = 1
	heightLower  = 2
)

//go:embed colors.json
var colorJsonBytes []byte

// mapColorMatch 是展平后的颜色匹配条目。
// 每个 colors.json 中的方块条目会产生 3 个 mapColorMatch，
// 分别对应 higher/middle/lower 三级明暗。
//
// 字段说明：
//   - c:    方块在地图上的 RGB 颜色（使用 go-colorful 的 CIELAB 色彩空间）
//   - name: 方块名，例如 "minecraft:stone"
//   - mode: 明暗模式，值为 heightHigher/heightMiddle/heightLower
type mapColorMatch struct {
	c    colorful.Color
	name string
	mode int
}

// mapColorTable 是整个颜色映射表，运行前从 colors.json 加载。
// colors.json 有约 70 个方块 × 3 级明暗 ≈ 210 种颜色。
// 每次 FindMapColor() 通过 CIELAB 色差在其中找最近匹配。
var mapColorTable []mapColorMatch

// blockMidColor 按方块名索引的中色值，用于3D预览颜色显示。
// 在 init() 中从 colors.json 填充。
var blockMidColor = map[string]colorful.Color{}

// init 在程序启动时自动加载 colors.json 并构建颜色表。
// colors.json 的格式为嵌套数组：
//
//	[
//	  [[r,g,b,a], [r,g,b,a], [r,g,b,a], "block_name"],  // 方块 1
//	  [[r,g,b,a], [r,g,b,a], [r,g,b,a], "block_name"],  // 方块 2
//	  ...
//	]
//
// 每个方块数组包含：
//   - entry[0]: 亮色 RGBA（地图上颜色较亮的那个变种）
//   - entry[1]: 中色 RGBA（基准色）
//   - entry[2]: 暗色 RGBA（较暗变种）
//   - entry[3]: 方块 ID 字符串
//
// 加载时会把每个方块的 3 个颜色拆成独立条目，形成 ~210 条的扁平表，
// 方便后续用线性扫描做最近颜色匹配。
func init() {
	var raw [][]any
	if err := json.Unmarshal(colorJsonBytes, &raw); err != nil {
		panic("map_color: " + err.Error())
	}

	mapColorTable = make([]mapColorMatch, 0, len(raw)*3)
	for _, entry := range raw {
		name, _ := entry[3].(string)
		if name == "" {
			continue
		}
		higher := parseRGBA(entry[0].([]any))
		middle := parseRGBA(entry[1].([]any))
		blockMidColor[name] = middle
		lower := parseRGBA(entry[2].([]any))
		mapColorTable = append(mapColorTable,
			mapColorMatch{c: higher, name: name, mode: heightHigher},
			mapColorMatch{c: middle, name: name, mode: heightMiddle},
			mapColorMatch{c: lower, name: name, mode: heightLower},
		)
	}
}

// parseRGBA 将 JSON 数组 [r, g, b, a] 转换为 colorful.Color（RGB，忽略 Alpha）。
// colors.json 中的颜色取值范围 0~255，colorful.Color 要求 0~255 的 float64。
func parseRGBA(v []any) colorful.Color {
	return colorful.Color{
		R: v[0].(float64),
		G: v[1].(float64),
		B: v[2].(float64),
	}
}

// FindMapColor 将图片像素的 RGB 颜色匹配到最近的 Minecraft 地图色，
// 返回方块名和高度偏移。
//
// 参数：
//   - r, g, b: 像素颜色值（0~255）
//
// 返回值：
//   - name:         匹配到的方块名，如 "minecraft:stone"
//   - heightOffset: 高度偏移，-1 降低 1 格（暗色）/ 0 不动（中色）/ +1 抬高 1 格（亮色）
//
// 匹配算法：
// 使用 CIELAB 色差公式（go-colorful.DistanceLab），比 RGB 欧氏距离更符合人眼感知。
// 扫描全部 ~210 种颜色，取色差最小的那个，同时记录该颜色属于哪一级明暗。
//
// 使用示例（地图画主流程中）：
//
//	blockName, h := media.FindMapColor(r, g, b)
//	posY := baseY + h  // 根据明暗调整 Y 坐标
//	// 在地图画对应位置放置 blockName 方块
// BlockMidColor 返回方块在 colors.json 中的中色值，用于3D预览。
// 如果方块不在 colors.json 中，返回 false。
func BlockMidColor(name string) (colorful.Color, bool) {
	c, ok := blockMidColor[name]
	return c, ok
}

// BlockMidColorMapLen 返回 colors.json 中已索引的方块数。
func BlockMidColorMapLen() int {
	return len(blockMidColor)
}

func FindMapColor(r, g, b uint8) (name string, heightOffset int) {
	target := colorful.Color{R: float64(r), G: float64(g), B: float64(b)}
	bestName := "minecraft:white_concrete"
	bestMode := heightMiddle
	bestDist := 1e10
	for _, m := range mapColorTable {
		// 使用 CIELAB 色差，比 RGB 欧氏距离更精确
		dist := target.DistanceLab(m.c)
		if dist < bestDist {
			bestDist = dist
			bestName = m.name
			bestMode = m.mode
		}
	}
	switch bestMode {
	case heightHigher:
		return bestName, 1
	case heightLower:
		return bestName, -1
	default:
		return bestName, 0
	}
}

// ============================================================
// 以下为 v1.0.5 新增：多色彩空间 + 抖动算法支持
// ============================================================

// ColorSpace 色彩空间类型
type ColorSpace int

const (
	ColorSpaceLab ColorSpace = iota // CIELAB（默认，最符合人眼）
	ColorSpaceRgb                   // RGB 欧氏距离
	ColorSpaceHsv                   // HSV 色相饱和度明度
)

// DitherAlgo 抖动算法类型
type DitherAlgo int

const (
	DitherNone               DitherAlgo = iota // 无抖动
	DitherFloydSteinberg                        // Floyd-Steinberg 误差扩散
	DitherAtkinson                              // Atkinson 误差扩散（轻量）
	DitherBurkes                                // Burkes 误差扩散
	DitherStucki                                // Stucki 误差扩散（广范围）
	DitherJarvisJudiceNinke                     // Jarvis-Judice-Ninke 误差扩散（最细腻）
	DitherBayer2x2                              // Bayer 2×2 有序抖动
	DitherBayer4x4                              // Bayer 4×4 有序抖动
	DitherBayer8x8                              // Bayer 8×8 有序抖动
	DitherOrdered3x3                            // 3×3 有序抖动
)

// Rotation 图片旋转角度
type Rotation int

const (
	Rotation0   Rotation = 0   // 不旋转
	Rotation90  Rotation = 90  // 顺时针 90 度
	Rotation180 Rotation = 180 // 180 度
	Rotation270 Rotation = 270 // 顺时针 270 度（逆时针 90 度）
)

// BlockMatch 单个像素的匹配结果
type BlockMatch struct {
	Name      string // 方块名，如 "minecraft:stone"
	HeightOff int    // 高度偏移：-1 暗色 / 0 中色 / +1 亮色
}

// Bayer 有序抖动矩阵
var bayer2x2 = [2][2]float32{{0, 2}, {3, 1}}
var bayer4x4 = [4][4]float32{
	{0, 8, 2, 10},
	{12, 4, 14, 6},
	{3, 11, 1, 9},
	{15, 7, 13, 5},
}
var bayer8x8 = [8][8]float32{
	{0, 32, 8, 40, 2, 34, 10, 42},
	{48, 16, 56, 24, 50, 18, 58, 26},
	{12, 44, 4, 36, 14, 46, 6, 38},
	{60, 28, 52, 20, 62, 30, 54, 22},
	{3, 35, 11, 43, 1, 33, 9, 41},
	{51, 19, 59, 27, 49, 17, 57, 25},
	{15, 47, 7, 39, 13, 45, 5, 37},
	{63, 31, 55, 23, 61, 29, 53, 21},
}
var ordered3x3 = [3][3]float32{{7, 2, 6}, {4, 0, 1}, {3, 8, 5}}

// rgbToHsv 将 RGB (0~255) 转换为 HSV（H:0~360, S:0~1, V:0~1）
func rgbToHsv(r, g, b uint8) (float64, float64, float64) {
	rf := float64(r) / 255.0
	gf := float64(g) / 255.0
	bf := float64(b) / 255.0
	mx := max(rf, max(gf, bf))
	mn := min(rf, min(gf, bf))
	diff := mx - mn

	var h, s, v float64
	v = mx

	if diff < 1e-6 {
		h = 0
	} else {
		switch mx {
		case rf:
			h = 60.0 * ((gf - bf) / diff)
			if h < 0 {
				h += 360
			}
		case gf:
			h = 60.0 * (2.0 + (bf-rf)/diff)
		case bf:
			h = 60.0 * (4.0 + (rf-gf)/diff)
		}
	}

	if mx < 1e-6 {
		s = 0
	} else {
		s = diff / mx
	}

	return h, s, v
}

// colorToVec 将 RGB 颜色转换到目标色彩空间的向量
func colorToVec(r, g, b uint8, cs ColorSpace) [3]float64 {
	switch cs {
	case ColorSpaceRgb:
		return [3]float64{float64(r) / 255.0, float64(g) / 255.0, float64(b) / 255.0}
	case ColorSpaceHsv:
		h, s, v := rgbToHsv(r, g, b)
		return [3]float64{h / 360.0, s, v}
	default: // ColorSpaceLab
		c := colorful.Color{R: float64(r), G: float64(g), B: float64(b)}
		l, a, bb := c.Lab()
		return [3]float64{l, a, bb}
	}
}

// vecDistSq 计算两个色彩空间向量的欧氏距离平方
func vecDistSq(a, b [3]float64) float64 {
	dx := a[0] - b[0]
	dy := a[1] - b[1]
	dz := a[2] - b[2]
	return dx*dx + dy*dy + dz*dz
}

// preconvertBlockColors 将颜色表中的所有方块预转换到目标色彩空间
func preconvertBlockColors(cs ColorSpace) [][3]float64 {
	converted := make([][3]float64, len(mapColorTable))
	for i, m := range mapColorTable {
		r := uint8(clampFloat(m.c.R, 0, 255))
		g := uint8(clampFloat(m.c.G, 0, 255))
		b := uint8(clampFloat(m.c.B, 0, 255))
		converted[i] = colorToVec(r, g, b, cs)
	}
	return converted
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// findBestBlock 在预转换的颜色表中找最近匹配
func findBestBlock(target [3]float64, converted [][3]float64) (string, int) {
	bestName := "minecraft:white_concrete"
	bestMode := heightMiddle
	bestDist := 1e100

	for i, m := range mapColorTable {
		dist := vecDistSq(target, converted[i])
		if dist < bestDist {
			bestDist = dist
			bestName = m.name
			bestMode = m.mode
		}
	}

	switch bestMode {
	case heightHigher:
		return bestName, 1
	case heightLower:
		return bestName, -1
	default:
		return bestName, 0
	}
}

// ProcessMapArt 处理一整张图片，应用抖动算法和色彩空间匹配。
// 返回 targetH×targetW 的方块网格。
//
// 参数：
//   - img:     原始图片
//   - targetW: 目标宽度（像素数）
//   - targetH: 目标高度（像素数）
//   - cs:      色彩空间（RGB/HSV/LAB）
//   - dither:  抖动算法
//   - rot:     旋转角度（0/90/180/270）
//
// 返回：
//   - 网格 [y][x]BlockMatch
func ProcessMapArt(img image.Image, targetW, targetH int, cs ColorSpace, dither DitherAlgo, rot Rotation) [][]BlockMatch {
	// 缩放图片并按指定角度旋转
	srcW, srcH := img.Bounds().Dx(), img.Bounds().Dy()
	pixels := make([][3]uint8, targetH*targetW)
	for y := 0; y < targetH; y++ {
		for x := 0; x < targetW; x++ {
			var sx, sy int
			switch rot {
			case Rotation0:
				sx = x * srcW / targetW
				sy = y * srcH / targetH
			case Rotation90:
				// 顺时针 90°：目标 (x,y) ← 源 (y, targetW-1-x)
				sx = y * srcW / targetH
				sy = (targetW - 1 - x) * srcH / targetW
			case Rotation180:
				sx = srcW - 1 - x*srcW/targetW
				sy = srcH - 1 - y*srcH/targetH
			case Rotation270:
				// 逆时针 90°：目标 (x,y) ← 源 (targetH-1-y, x)
				sx = (targetH - 1 - y) * srcW / targetH
				sy = x * srcH / targetW
			default:
				sx = x * srcW / targetW
				sy = y * srcH / targetH
			}
			r, g, b, a := img.At(sx, sy).RGBA()
			if a>>8 < 128 {
				pixels[y*targetW+x] = [3]uint8{0, 0, 0} // 透明，设为黑色
			} else {
				pixels[y*targetW+x] = [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
			}
		}
	}

	// 预转换颜色表到目标色彩空间
	converted := preconvertBlockColors(cs)

	// 初始化结果网格
	grid := make([][]BlockMatch, targetH)
	for y := 0; y < targetH; y++ {
		grid[y] = make([]BlockMatch, targetW)
	}

	// 误差缓冲区（用于误差扩散抖动）
	useDithering := dither >= DitherFloydSteinberg && dither <= DitherJarvisJudiceNinke
	errBuff := make([][]float64, targetH)
	if useDithering {
		for y := 0; y < targetH; y++ {
			errBuff[y] = make([]float64, targetW*3) // 每个像素 3 通道误差
		}
	}

	bayerSpread := 0.15

	for y := 0; y < targetH; y++ {
		for x := 0; x < targetW; x++ {
			px := pixels[y*targetW+x]
			if px[0] == 0 && px[1] == 0 && px[2] == 0 {
				// 透明像素
				grid[y][x] = BlockMatch{Name: "minecraft:air", HeightOff: 0}
				continue
			}

			// 转换当前像素到目标色彩空间
			target := colorToVec(px[0], px[1], px[2], cs)

			// 叠加误差
			if useDithering {
				ei := x * 3
				target[0] += errBuff[y][ei]
				target[1] += errBuff[y][ei+1]
				target[2] += errBuff[y][ei+2]
			}

			// 有序抖动偏移
			var bayerOff float64
			switch dither {
			case DitherBayer2x2:
				bayerOff = (float64(bayer2x2[y%2][x%2])/4.0 - 0.5) * bayerSpread
			case DitherBayer4x4:
				bayerOff = (float64(bayer4x4[y%4][x%4])/16.0 - 0.5) * bayerSpread
			case DitherBayer8x8:
				bayerOff = (float64(bayer8x8[y%8][x%8])/64.0 - 0.5) * bayerSpread
			case DitherOrdered3x3:
				bayerOff = (float64(ordered3x3[y%3][x%3])/9.0 - 0.5) * bayerSpread
			}
			if bayerOff != 0 {
				target[0] += bayerOff
				target[1] += bayerOff
				target[2] += bayerOff
			}

			// 找最近匹配方块
			name, hOff := findBestBlock(target, converted)
			grid[y][x] = BlockMatch{Name: name, HeightOff: hOff}

			// 误差扩散
			if useDithering {
				// 找到匹配方块在颜色表中的 RGB 值
				blockRgb := findBlockRgb(name, hOff)
				blockVec2 := colorToVec(blockRgb[0], blockRgb[1], blockRgb[2], cs)

				err0 := target[0] - blockVec2[0]
				err1 := target[1] - blockVec2[1]
				err2 := target[2] - blockVec2[2]

				// 根据算法扩散到相邻像素
				switch dither {
				case DitherFloydSteinberg:
					safeAddErr(errBuff, y, x+1, targetW, targetH, err0, err1, err2, 7.0/16)
					safeAddErr(errBuff, y+1, x-1, targetW, targetH, err0, err1, err2, 3.0/16)
					safeAddErr(errBuff, y+1, x, targetW, targetH, err0, err1, err2, 5.0/16)
					safeAddErr(errBuff, y+1, x+1, targetW, targetH, err0, err1, err2, 1.0/16)
				case DitherAtkinson:
					safeAddErr(errBuff, y, x+1, targetW, targetH, err0, err1, err2, 1.0/8)
					safeAddErr(errBuff, y, x+2, targetW, targetH, err0, err1, err2, 1.0/8)
					safeAddErr(errBuff, y+1, x-1, targetW, targetH, err0, err1, err2, 1.0/8)
					safeAddErr(errBuff, y+1, x, targetW, targetH, err0, err1, err2, 1.0/8)
					safeAddErr(errBuff, y+1, x+1, targetW, targetH, err0, err1, err2, 1.0/8)
					safeAddErr(errBuff, y+2, x, targetW, targetH, err0, err1, err2, 1.0/8)
				case DitherBurkes:
					safeAddErr(errBuff, y, x+1, targetW, targetH, err0, err1, err2, 8.0/32)
					safeAddErr(errBuff, y, x+2, targetW, targetH, err0, err1, err2, 4.0/32)
					safeAddErr(errBuff, y+1, x-2, targetW, targetH, err0, err1, err2, 2.0/32)
					safeAddErr(errBuff, y+1, x-1, targetW, targetH, err0, err1, err2, 4.0/32)
					safeAddErr(errBuff, y+1, x, targetW, targetH, err0, err1, err2, 8.0/32)
					safeAddErr(errBuff, y+1, x+1, targetW, targetH, err0, err1, err2, 4.0/32)
					safeAddErr(errBuff, y+1, x+2, targetW, targetH, err0, err1, err2, 2.0/32)
				case DitherStucki:
					safeAddErr(errBuff, y, x+1, targetW, targetH, err0, err1, err2, 8.0/42)
					safeAddErr(errBuff, y, x+2, targetW, targetH, err0, err1, err2, 4.0/42)
					safeAddErr(errBuff, y+1, x-2, targetW, targetH, err0, err1, err2, 2.0/42)
					safeAddErr(errBuff, y+1, x-1, targetW, targetH, err0, err1, err2, 4.0/42)
					safeAddErr(errBuff, y+1, x, targetW, targetH, err0, err1, err2, 8.0/42)
					safeAddErr(errBuff, y+1, x+1, targetW, targetH, err0, err1, err2, 4.0/42)
					safeAddErr(errBuff, y+1, x+2, targetW, targetH, err0, err1, err2, 2.0/42)
					safeAddErr(errBuff, y+2, x-2, targetW, targetH, err0, err1, err2, 1.0/42)
					safeAddErr(errBuff, y+2, x-1, targetW, targetH, err0, err1, err2, 2.0/42)
					safeAddErr(errBuff, y+2, x, targetW, targetH, err0, err1, err2, 4.0/42)
					safeAddErr(errBuff, y+2, x+1, targetW, targetH, err0, err1, err2, 2.0/42)
					safeAddErr(errBuff, y+2, x+2, targetW, targetH, err0, err1, err2, 1.0/42)
				case DitherJarvisJudiceNinke:
					safeAddErr(errBuff, y, x+1, targetW, targetH, err0, err1, err2, 7.0/48)
					safeAddErr(errBuff, y, x+2, targetW, targetH, err0, err1, err2, 5.0/48)
					safeAddErr(errBuff, y+1, x-2, targetW, targetH, err0, err1, err2, 3.0/48)
					safeAddErr(errBuff, y+1, x-1, targetW, targetH, err0, err1, err2, 5.0/48)
					safeAddErr(errBuff, y+1, x, targetW, targetH, err0, err1, err2, 7.0/48)
					safeAddErr(errBuff, y+1, x+1, targetW, targetH, err0, err1, err2, 5.0/48)
					safeAddErr(errBuff, y+1, x+2, targetW, targetH, err0, err1, err2, 3.0/48)
					safeAddErr(errBuff, y+2, x-2, targetW, targetH, err0, err1, err2, 1.0/48)
					safeAddErr(errBuff, y+2, x-1, targetW, targetH, err0, err1, err2, 3.0/48)
					safeAddErr(errBuff, y+2, x, targetW, targetH, err0, err1, err2, 5.0/48)
					safeAddErr(errBuff, y+2, x+1, targetW, targetH, err0, err1, err2, 3.0/48)
					safeAddErr(errBuff, y+2, x+2, targetW, targetH, err0, err1, err2, 1.0/48)
				}
			}
		}
	}

	return grid
}

// safeAddErr 安全地将误差值累加到误差缓冲区的指定位置
func safeAddErr(errBuff [][]float64, y, x, w, h int, e0, e1, e2, weight float64) {
	if y < 0 || y >= h || x < 0 || x >= w {
		return
	}
	ei := x * 3
	errBuff[y][ei] += e0 * weight
	errBuff[y][ei+1] += e1 * weight
	errBuff[y][ei+2] += e2 * weight
}

// findBlockRgb 根据方块名和高度模式查找在 colors.json 中的 RGB 值
func findBlockRgb(name string, hOff int) [3]uint8 {
	for _, m := range mapColorTable {
		if m.name == name {
			var mode int
			switch hOff {
			case 1:
				mode = heightHigher
			case -1:
				mode = heightLower
			default:
				mode = heightMiddle
			}
			if m.mode == mode {
				return [3]uint8{uint8(clampFloat(m.c.R, 0, 255)), uint8(clampFloat(m.c.G, 0, 255)), uint8(clampFloat(m.c.B, 0, 255))}
			}
		}
	}
	return [3]uint8{0, 0, 0}
}
