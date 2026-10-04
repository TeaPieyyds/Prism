// 包 media 提供皮肤 PNG → 3D 雕像的转换功能。
//
// 从 NexusEgo 移植：
//   - UV 纹理映射：标准 Minecraft 皮肤布局 → 6 个人体部件
//   - 5 种方块调色板：混凝土/羊毛/陶瓦/混合/扩展
//   - classic/slim 手臂支持
//   - 缩放、外扩厚度、alpha 透明裁切
//
// 输出：[]SkinBlock 列表，直接用于 bot setblock/fill 放置。
package media

import (
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"math"
	"sort"
	"sync"

	"github.com/lucasb-eyer/go-colorful"
)

// ─── 公开类型 ───

type SkinArmType string

const (
	SkinArmClassic SkinArmType = "classic"
	SkinArmSlim    SkinArmType = "slim"
)

type SkinBlockSet string

const (
	SkinBlockSetConcrete   SkinBlockSet = "concrete"
	SkinBlockSetWool       SkinBlockSet = "wool"
	SkinBlockSetTerracotta SkinBlockSet = "terracotta"
	SkinBlockSetMixed      SkinBlockSet = "mixed"
	SkinBlockSetExtended   SkinBlockSet = "extended"
)

// SkinBuildOptions 控制雕像生成参数。
type SkinBuildOptions struct {
	Scale          int          // 缩放倍数（1=每像素1方块，2=每像素2x2x2）
	ArmType        SkinArmType  // classic=4宽手臂, slim=3宽手臂
	BlockSet       SkinBlockSet // 方块调色板
	AlphaCutoff    uint8        // Alpha 裁切阈值（<此值视为透明）
	OuterThickness int          // overlay 外扩厚度（0=不扩）
	Solid          bool         // 是否用填充方块填满内部
	FillBlock      string       // 内部填充方块名
	Rotation       int          // 顺时针旋转角度（0/90/180/270），绕 Y 轴
}

// SkinBlock 描述雕像中的一个方块。
type SkinBlock struct {
	X, Y, Z     int32
	BlockName   string
	BlockStates string
}

// SkinStatueInfo 包含雕像的尺寸和方块数信息。
type SkinStatueInfo struct {
	Width       int // X 方向
	Height      int // Y 方向
	Length      int // Z 方向
	BlockCount  int
}

// DefaultSkinBuildOptions 返回安全的默认参数。
func DefaultSkinBuildOptions() SkinBuildOptions {
	return SkinBuildOptions{
		Scale:          2,
		ArmType:        SkinArmClassic,
		BlockSet:       SkinBlockSetMixed,
		AlphaCutoff:    16,
		OuterThickness: 1,
		Solid:          false,
		FillBlock:      "minecraft:stone",
		Rotation:       0,
	}
}

// NormalizeSkinBuildOptions 补全零值字段为默认值。
func NormalizeSkinBuildOptions(opts SkinBuildOptions) SkinBuildOptions {
	def := DefaultSkinBuildOptions()
	if opts.Scale <= 0 {
		opts.Scale = def.Scale
	}
	if opts.Scale > 8 {
		opts.Scale = 8 // 最大缩放 8 倍，防止超世界边界
	}
	if string(opts.ArmType) == "" {
		opts.ArmType = def.ArmType
	}
	if string(opts.BlockSet) == "" {
		opts.BlockSet = def.BlockSet
	}
	if opts.AlphaCutoff == 0 {
		opts.AlphaCutoff = def.AlphaCutoff
	}
	if opts.OuterThickness < 0 {
		opts.OuterThickness = 0
	}
	if opts.OuterThickness > 4 {
		opts.OuterThickness = 4 // 限制最大厚度
	}
	if opts.FillBlock == "" {
		opts.FillBlock = def.FillBlock
	}
	return opts
}

// DetectArmType 自动检测手臂类型：检查右臂第4列的像素密度。
// 如果第4列（x=43，64x坐标）的可见像素数少于前三列平均值的40%，判定为 slim。
func DetectArmType(img image.Image) SkinArmType {
	skin, err := loadSkin(img)
	if err != nil {
		return SkinArmClassic
	}

	// 检查右臂外侧（West face，UV 40,20，4x12 像素）
	unit := skin.unit
	colCount := make([]int, 4)
	for col := 0; col < 4; col++ {
		u := (40 + col) * unit
		ct := 0
		for row := 20 * unit; row < 32*unit; row++ {
			px := skin.img.NRGBAAt(u, row)
			if px.A > 16 {
				ct++
			}
		}
		colCount[col] = ct
	}

	avgFirst3 := (colCount[0] + colCount[1] + colCount[2]) / 3
	if avgFirst3 > 0 && colCount[3]*100/avgFirst3 < 40 {
		return SkinArmSlim
	}
	return SkinArmClassic
}

const maxSkinBlockCount = 500000 // 雕像最大方块数安全限制

// BuildSkinStatue 将皮肤图片转换为雕像方块列表。
// img 必须为标准 MC 皮肤尺寸（64x32 / 64x64 / 128x128）。
// 返回的方块坐标以雕像底部中心为原点。
func BuildSkinStatue(img image.Image, opts SkinBuildOptions) ([]SkinBlock, *SkinStatueInfo, error) {
	opts = NormalizeSkinBuildOptions(opts)

	skin, err := loadSkin(img)
	if err != nil {
		return nil, nil, err
	}

	// 检查雕像高度是否超出世界限制 (Y=-64~319, 共384格)
	w, h, l := skinStatueDimensions(skin, opts.Scale)
	heightWithOuter := h + opts.OuterThickness*2
	if heightWithOuter > 383 {
		return nil, nil, fmt.Errorf("雕像高度 %d 超出世界限制(383)，请降低缩放比例", heightWithOuter)
	}

	palette, err := loadSkinPalette(opts.BlockSet)
	if err != nil {
		return nil, nil, err
	}

	blocks := buildSkinStatueBlocks(skin, palette, opts)
	if len(blocks) == 0 {
		return nil, nil, fmt.Errorf("皮肤中没有可见像素（AlphaCutoff=%d 可能过高）", opts.AlphaCutoff)
	}
	if len(blocks) > maxSkinBlockCount {
		return nil, nil, fmt.Errorf("雕像方块数 %d 超过安全限制(%d)，请降低缩放", len(blocks), maxSkinBlockCount)
	}

	// 填充相邻部件之间的 1 格缝隙
	fillSkinGaps(blocks, opts)

	// 转换为输出格式
	var result []SkinBlock
	shift := opts.OuterThickness
	for pos, blockName := range blocks {
		result = append(result, SkinBlock{
			X:           int32(pos.x + shift),
			Y:           int32(pos.y + shift),
			Z:           int32(pos.z + shift),
			BlockName:   blockName,
			BlockStates: "[]",
		})
	}

	// 应用朝向旋转（绕 Y 轴顺时针）
	if opts.Rotation != 0 {
		rotateSkinBlocks(result, w+opts.OuterThickness*2, h, l+opts.OuterThickness*2, opts)
	}

	// 按 Y 从高到低排序（头先显示），确保预览一致
	sort.Slice(result, func(i, j int) bool {
		if result[i].Y != result[j].Y {
			return result[i].Y > result[j].Y
		}
		if result[i].Z != result[j].Z {
			return result[i].Z < result[j].Z
		}
		return result[i].X < result[j].X
	})

	return result, &SkinStatueInfo{
		Width:      w + opts.OuterThickness*2,
		Height:     h + opts.OuterThickness*2,
		Length:     l + opts.OuterThickness*2,
		BlockCount: len(result),
	}, nil
}

// ─── 内部类型 ───

type skinImage struct {
	width  int
	height int
	unit   int // 64 基准倍数（1 或 2）
	img    *image.NRGBA
}

type skinPos struct{ x, y, z int }

type skinSize struct{ w, h, d int }

type skinFace string

const (
	skinFaceNorth skinFace = "north"
	skinFaceSouth skinFace = "south"
	skinFaceWest  skinFace = "west"
	skinFaceEast  skinFace = "east"
	skinFaceUp    skinFace = "up"
	skinFaceDown  skinFace = "down"
)

type skinFaceUV struct{ u, v, w, h int }

type skinPart struct {
	name      string
	origin    skinPos
	size      skinSize
	baseUV    map[skinFace]skinFaceUV
	overlayUV map[skinFace]skinFaceUV
}

type skinPalette struct {
	entries []skinPaletteSpec
}

var (
	skinPaletteMu   sync.Mutex
	skinPaletteCache = make(map[SkinBlockSet]skinPalette)
)

// ─── 图片加载与校验 ───

func loadSkin(img image.Image) (skinImage, error) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// 校验尺寸
	valid := (w == 64 && h == 64) || (w == 64 && h == 32) || (w == 128 && h == 128)
	if !valid {
		return skinImage{}, fmt.Errorf("不支持的皮肤尺寸: %dx%d，仅支持 64x64 / 64x32 / 128x128", w, h)
	}

	unit := w / 64
	if unit < 1 {
		unit = 1
	}

	// 转换为 NRGBA 方便像素读取
	nrgba := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			nrgba.SetNRGBA(x, y, colorNRGBAModel(img.At(x, y)))
		}
	}

	return skinImage{
		width:  w,
		height: h,
		unit:   unit,
		img:    nrgba,
	}, nil
}

func colorNRGBAModel(c color.Color) color.NRGBA {
	if n, ok := c.(color.NRGBA); ok {
		return n
	}
	r, g, b, a := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// ─── 尺寸计算 ───

func skinStatueDimensions(skin skinImage, scale int) (int, int, int) {
	return 16 * skin.unit * scale, 32 * skin.unit * scale, 8 * skin.unit * scale
}

// ─── UV 映射 ───

func uvRectsFromNet(u0, v0, w, h, d int, unit int) map[skinFace]skinFaceUV {
	u0 *= unit
	v0 *= unit
	w *= unit
	h *= unit
	d *= unit
	return map[skinFace]skinFaceUV{
		skinFaceUp:    {u: u0 + d, v: v0, w: w, h: d},
		skinFaceDown:  {u: u0 + d + w, v: v0, w: w, h: d},
		skinFaceWest:  {u: u0, v: v0 + d, w: d, h: h},
		skinFaceSouth: {u: u0 + d, v: v0 + d, w: w, h: h},
		skinFaceEast:  {u: u0 + d + w, v: v0 + d, w: d, h: h},
		skinFaceNorth: {u: u0 + d + w + d, v: v0 + d, w: w, h: h},
	}
}

// ─── 人体部件定义 ───

func skinPartsForClassic(skin skinImage, scale int) []skinPart {
	unit := skin.unit
	headBase := uvRectsFromNet(0, 0, 8, 8, 8, unit)
	bodyBase := uvRectsFromNet(16, 16, 8, 12, 4, unit)
	rightArmBase := uvRectsFromNet(40, 16, 4, 12, 4, unit)
	rightLegBase := uvRectsFromNet(0, 16, 4, 12, 4, unit)

	isModern := skin.width == 64*unit && skin.height == 64*unit
	leftArmBase := rightArmBase
	leftLegBase := rightLegBase
	if isModern {
		leftArmBase = uvRectsFromNet(32, 48, 4, 12, 4, unit)
		leftLegBase = uvRectsFromNet(16, 48, 4, 12, 4, unit)
	}

	var headOverlay, bodyOverlay, rightArmOverlay, rightLegOverlay, leftLegOverlay, leftArmOverlay map[skinFace]skinFaceUV
	if isModern {
		headOverlay = uvRectsFromNet(32, 0, 8, 8, 8, unit)
		bodyOverlay = uvRectsFromNet(16, 32, 8, 12, 4, unit)
		rightArmOverlay = uvRectsFromNet(40, 32, 4, 12, 4, unit)
		rightLegOverlay = uvRectsFromNet(0, 32, 4, 12, 4, unit)
		leftLegOverlay = uvRectsFromNet(0, 48, 4, 12, 4, unit)
		leftArmOverlay = uvRectsFromNet(48, 48, 4, 12, 4, unit)
	}

	s := scale * unit
	return []skinPart{
		{name: "head", origin: skinPos{4 * s, 24 * s, 0}, size: skinSize{8 * s, 8 * s, 8 * s}, baseUV: headBase, overlayUV: headOverlay},
		{name: "body", origin: skinPos{4 * s, 12 * s, 2 * s}, size: skinSize{8 * s, 12 * s, 4 * s}, baseUV: bodyBase, overlayUV: bodyOverlay},
		{name: "arm_right", origin: skinPos{0, 12 * s, 2 * s}, size: skinSize{4 * s, 12 * s, 4 * s}, baseUV: rightArmBase, overlayUV: rightArmOverlay},
		{name: "arm_left", origin: skinPos{12 * s, 12 * s, 2 * s}, size: skinSize{4 * s, 12 * s, 4 * s}, baseUV: leftArmBase, overlayUV: leftArmOverlay},
		{name: "leg_right", origin: skinPos{4 * s, 0, 2 * s}, size: skinSize{4 * s, 12 * s, 4 * s}, baseUV: rightLegBase, overlayUV: rightLegOverlay},
		{name: "leg_left", origin: skinPos{8 * s, 0, 2 * s}, size: skinSize{4 * s, 12 * s, 4 * s}, baseUV: leftLegBase, overlayUV: leftLegOverlay},
	}
}

func skinPartsForSlim(skin skinImage, scale int) ([]skinPart, error) {
	unit := skin.unit
	if skin.width != 64*unit || skin.height != 64*unit {
		return nil, fmt.Errorf("非现代皮肤(64x64)不支持 slim 细手臂")
	}

	armWidth := 3
	headBase := uvRectsFromNet(0, 0, 8, 8, 8, unit)
	bodyBase := uvRectsFromNet(16, 16, 8, 12, 4, unit)
	rightArmBase := uvRectsFromNet(40, 16, armWidth, 12, 4, unit)
	leftArmBase := uvRectsFromNet(32, 48, armWidth, 12, 4, unit)
	rightLegBase := uvRectsFromNet(0, 16, 4, 12, 4, unit)
	leftLegBase := uvRectsFromNet(16, 48, 4, 12, 4, unit)

	headOverlay := uvRectsFromNet(32, 0, 8, 8, 8, unit)
	bodyOverlay := uvRectsFromNet(16, 32, 8, 12, 4, unit)
	rightArmOverlay := uvRectsFromNet(40, 32, armWidth, 12, 4, unit)
	rightLegOverlay := uvRectsFromNet(0, 32, 4, 12, 4, unit)
	leftLegOverlay := uvRectsFromNet(0, 48, 4, 12, 4, unit)
	leftArmOverlay := uvRectsFromNet(48, 48, armWidth, 12, 4, unit)

	s := scale * unit
	return []skinPart{
		{name: "head", origin: skinPos{4 * s, 24 * s, 0}, size: skinSize{8 * s, 8 * s, 8 * s}, baseUV: headBase, overlayUV: headOverlay},
		{name: "body", origin: skinPos{4 * s, 12 * s, 2 * s}, size: skinSize{8 * s, 12 * s, 4 * s}, baseUV: bodyBase, overlayUV: bodyOverlay},
		{name: "arm_right", origin: skinPos{(4 - armWidth) * s, 12 * s, 2 * s}, size: skinSize{armWidth * s, 12 * s, 4 * s}, baseUV: rightArmBase, overlayUV: rightArmOverlay},
		{name: "arm_left", origin: skinPos{12 * s, 12 * s, 2 * s}, size: skinSize{armWidth * s, 12 * s, 4 * s}, baseUV: leftArmBase, overlayUV: leftArmOverlay},
		{name: "leg_right", origin: skinPos{4 * s, 0, 2 * s}, size: skinSize{4 * s, 12 * s, 4 * s}, baseUV: rightLegBase, overlayUV: rightLegOverlay},
		{name: "leg_left", origin: skinPos{8 * s, 0, 2 * s}, size: skinSize{4 * s, 12 * s, 4 * s}, baseUV: leftLegBase, overlayUV: leftLegOverlay},
	}, nil
}

func skinParts(skin skinImage, scale int, armType SkinArmType) ([]skinPart, error) {
	switch armType {
	case SkinArmClassic:
		return skinPartsForClassic(skin, scale), nil
	case SkinArmSlim:
		return skinPartsForSlim(skin, scale)
	default:
		return nil, fmt.Errorf("未知手臂类型: %s", armType)
	}
}

// ─── 主要构建逻辑 ───

func buildSkinStatueBlocks(skin skinImage, palette skinPalette, opts SkinBuildOptions) map[skinPos]string {
	parts, err := skinParts(skin, opts.Scale, opts.ArmType)
	if err != nil {
		return nil
	}

	blocks := make(map[skinPos]string)

	for _, part := range parts {
		ox, oy, oz := part.origin.x, part.origin.y, part.origin.z
		sw, sh, sd := part.size.w, part.size.h, part.size.d
		w, h, d := sw/opts.Scale, sh/opts.Scale, sd/opts.Scale

		// 实体填充
		if opts.Solid {
			for yb := 0; yb < sh; yb++ {
				for zb := 0; zb < sd; zb++ {
					for xb := 0; xb < sw; xb++ {
						blocks[skinPos{ox + xb, oy + yb, oz + zb}] = opts.FillBlock
					}
				}
			}
		}

		// 表面纹理（alpha 混合 overlay 层到 base 层，透明像素自动补洞）
		for yb := 0; yb < sh; yb++ {
			for zb := 0; zb < sd; zb++ {
				for xb := 0; xb < sw; xb++ {
					face, ok := surfaceFaceForSkinBlock(xb, yb, zb, sw, sh, sd)
					if !ok {
						continue
					}

					xt, yt, zt := xb/opts.Scale, yb/opts.Scale, zb/opts.Scale
					uLocal, vLocal := skinFaceTexelCoords(face, xt, yt, zt, w, h, d)

					// 读取并混合 base + overlay 像素
					uv := part.baseUV[face]
					pixel := skin.img.NRGBAAt(uv.u+uLocal, uv.v+vLocal)
					if part.overlayUV != nil {
						ouv := part.overlayUV[face]
						opixel := skin.img.NRGBAAt(ouv.u+uLocal, ouv.v+vLocal)
						pixel = blendSkinPixels(pixel, opixel, opts.AlphaCutoff)
					}

					// 透明像素：在同一列（v方向）搜索最近的可见像素填充
					if pixel.A < opts.AlphaCutoff {
						found := false
						// 向下搜索
						for vy := vLocal + 1; vy < h; vy++ {
							np := skin.img.NRGBAAt(uv.u+uLocal, uv.v+vy)
							if part.overlayUV != nil {
								ouv := part.overlayUV[face]
								op := skin.img.NRGBAAt(ouv.u+uLocal, ouv.v+vy)
								np = blendSkinPixels(np, op, opts.AlphaCutoff)
							}
							if np.A >= opts.AlphaCutoff {
								pixel = np
								found = true
								break
							}
						}
						// 向上搜索
						if !found {
							for vy := vLocal - 1; vy >= 0; vy-- {
								np := skin.img.NRGBAAt(uv.u+uLocal, uv.v+vy)
								if part.overlayUV != nil {
									ouv := part.overlayUV[face]
									op := skin.img.NRGBAAt(ouv.u+uLocal, ouv.v+vy)
									np = blendSkinPixels(np, op, opts.AlphaCutoff)
								}
								if np.A >= opts.AlphaCutoff {
									pixel = np
									found = true
									break
								}
							}
						}
						if !found {
							continue
						}
					}
					blocks[skinPos{ox + xb, oy + yb, oz + zb}] = palette.nearestBlockName(pixel.R, pixel.G, pixel.B)
				}
			}
		}

		// overlay 外扩层
		if part.overlayUV != nil && opts.OuterThickness > 0 {
			writeSkinOverlay(blocks, skin, palette, part, w, h, d, opts)
		}
	}

	return blocks
}

func writeSkinOverlay(blocks map[skinPos]string, skin skinImage, palette skinPalette, part skinPart, w, h, d int, opts SkinBuildOptions) {
	t := opts.OuterThickness
	ox, oy, oz := part.origin.x, part.origin.y, part.origin.z
	sw, sh, sd := part.size.w, part.size.h, part.size.d

	// 用于外扩颜色混合：读取 overlay 像素后与对应 base 像素做 alpha 混合
	blendOverlayPixel := func(face skinFace, uLocal, vLocal int) color.NRGBA {
		ouv := part.overlayUV[face]
		pixel := skin.img.NRGBAAt(ouv.u+uLocal, ouv.v+vLocal)
		if pixel.A < opts.AlphaCutoff {
			return color.NRGBA{A: 0} // 标记为透明
		}
		// 半透明 overlay 与 base 混合
		if pixel.A < 255 {
			if baseUV, hasBase := part.baseUV[face]; hasBase {
				bpixel := skin.img.NRGBAAt(baseUV.u+uLocal, baseUV.v+vLocal)
				pixel = blendSkinPixels(bpixel, pixel, opts.AlphaCutoff)
			}
		}
		return pixel
	}

	for face := range part.overlayUV {
		switch face {
		case skinFaceSouth, skinFaceNorth:
			for yt := 0; yt < h; yt++ {
				for xt := 0; xt < w; xt++ {
					zt := 0
					if face == skinFaceSouth {
						zt = d - 1
					}
					uLocal, vLocal := skinFaceTexelCoords(face, xt, yt, zt, w, h, d)
					pixel := blendOverlayPixel(face, uLocal, vLocal)
					if pixel.A < opts.AlphaCutoff {
						continue
					}
					blockName := palette.nearestBlockName(pixel.R, pixel.G, pixel.B)
					x0, x1 := ox+xt*opts.Scale, ox+(xt+1)*opts.Scale
					y0, y1 := oy+yt*opts.Scale, oy+(yt+1)*opts.Scale
					z0, z1 := oz-t, oz
					if face == skinFaceSouth {
						z0, z1 = oz+sd, oz+sd+t
					}
					fillSkinBlocks(blocks, x0, x1, y0, y1, z0, z1, blockName)
				}
			}
		case skinFaceWest, skinFaceEast:
			for yt := 0; yt < h; yt++ {
				for zt := 0; zt < d; zt++ {
					xt := 0
					if face == skinFaceEast {
						xt = w - 1
					}
					uLocal, vLocal := skinFaceTexelCoords(face, xt, yt, zt, w, h, d)
					pixel := blendOverlayPixel(face, uLocal, vLocal)
					if pixel.A < opts.AlphaCutoff {
						continue
					}
					blockName := palette.nearestBlockName(pixel.R, pixel.G, pixel.B)
					z0, z1 := oz+zt*opts.Scale, oz+(zt+1)*opts.Scale
					y0, y1 := oy+yt*opts.Scale, oy+(yt+1)*opts.Scale
					x0, x1 := ox-t, ox
					if face == skinFaceEast {
						x0, x1 = ox+sw, ox+sw+t
					}
					fillSkinBlocks(blocks, x0, x1, y0, y1, z0, z1, blockName)
				}
			}
		case skinFaceUp, skinFaceDown:
			for zt := 0; zt < d; zt++ {
				for xt := 0; xt < w; xt++ {
					yt := 0
					if face == skinFaceUp {
						yt = h - 1
					}
					uLocal, vLocal := skinFaceTexelCoords(face, xt, yt, zt, w, h, d)
					pixel := blendOverlayPixel(face, uLocal, vLocal)
					if pixel.A < opts.AlphaCutoff {
						continue
					}
					blockName := palette.nearestBlockName(pixel.R, pixel.G, pixel.B)
					x0, x1 := ox+xt*opts.Scale, ox+(xt+1)*opts.Scale
					z0, z1 := oz+zt*opts.Scale, oz+(zt+1)*opts.Scale
					y0, y1 := oy-t, oy
					if face == skinFaceUp {
						y0, y1 = oy+sh, oy+sh+t
					}
					fillSkinBlocks(blocks, x0, x1, y0, y1, z0, z1, blockName)
				}
			}
		}
	}
}

func fillSkinBlocks(blocks map[skinPos]string, x0, x1, y0, y1, z0, z1 int, blockName string) {
	for yb := y0; yb < y1; yb++ {
		for xb := x0; xb < x1; xb++ {
			for zb := z0; zb < z1; zb++ {
				blocks[skinPos{xb, yb, zb}] = blockName
			}
		}
	}
}

// blendSkinPixels alpha 混合 overlay 像素到 base 像素上。
// 如果 overlay 完全不透明（A=255）或 base 透明，直接返回 overlay。
// 如果 overlay 透明（A<cutoff），返回 base。
// 否则做标准的 alpha 混合：result = overlay * alpha + base * (1-alpha)
func blendSkinPixels(base, overlay color.NRGBA, cutoff uint8) color.NRGBA {
	if overlay.A < cutoff {
		return base
	}
	if overlay.A >= 255 || base.A < cutoff {
		return overlay
	}
	alpha := float64(overlay.A) / 255.0
	return color.NRGBA{
		R: uint8(float64(overlay.R)*alpha + float64(base.R)*(1-alpha) + 0.5),
		G: uint8(float64(overlay.G)*alpha + float64(base.G)*(1-alpha) + 0.5),
		B: uint8(float64(overlay.B)*alpha + float64(base.B)*(1-alpha) + 0.5),
		A: 255,
	}
}

// fillSkinGaps 填充雕像中相邻部件之间的 1 格缝隙。
// 扫描每个空位，如果沿 X/Y/Z 方向被两个方块夹在中间，则用相邻方块的颜色填充。
func fillSkinGaps(blocks map[skinPos]string, opts SkinBuildOptions) {
	// 收集所有已有方块位置
	all := make(map[skinPos]bool, len(blocks))
	for pos := range blocks {
		all[pos] = true
	}

	// 六个方向
	dirs := [][3]int{{0, 1, 0}, {0, -1, 0}, {1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}}

	// 迭代填充直到没有新缝隙
	for {
		filled := 0
		for pos := range blocks {
			for _, d := range dirs {
				gap := skinPos{pos.x + d[0], pos.y + d[1], pos.z + d[2]}
				if all[gap] {
					continue
				}
				// 检查 gap 的另一侧是否有方块
				opp := skinPos{gap.x + d[0], gap.y + d[1], gap.z + d[2]}
				if _, exists := blocks[opp]; exists {
					blocks[gap] = blocks[pos]
					all[gap] = true
					filled++
				}
			}
		}
		if filled == 0 {
			break
		}
	}
}

// rotateSkinBlocks 绕 Y 轴顺时针旋转雕像方块。
func rotateSkinBlocks(blocks []SkinBlock, w, h, l int, opts SkinBuildOptions) {
	cx := float64(w) / 2
	cz := float64(l) / 2
	angle := float64(opts.Rotation) * math.Pi / 180
	sin, cos := math.Sin(angle), math.Cos(angle)

	for i := range blocks {
		dx := float64(blocks[i].X) - cx
		dz := float64(blocks[i].Z) - cz
		blocks[i].X = int32(math.Round(cx + dx*cos - dz*sin))
		blocks[i].Z = int32(math.Round(cz + dx*sin + dz*cos))
	}
}


func surfaceFaceForSkinBlock(x, y, z, w, h, d int) (skinFace, bool) {
	switch {
	case z == d-1:
		return skinFaceSouth, true
	case z == 0:
		return skinFaceNorth, true
	case x == 0:
		return skinFaceWest, true
	case x == w-1:
		return skinFaceEast, true
	case y == h-1:
		return skinFaceUp, true
	case y == 0:
		return skinFaceDown, true
	default:
		return "", false
	}
}

func skinFaceTexelCoords(face skinFace, x, y, z, w, h, d int) (int, int) {
	switch face {
	case skinFaceSouth:
		return x, h - 1 - y
	case skinFaceNorth:
		return w - 1 - x, h - 1 - y
	case skinFaceWest:
		return z, h - 1 - y
	case skinFaceEast:
		return d - 1 - z, h - 1 - y
	case skinFaceUp:
		return x, z
	case skinFaceDown:
		return x, d - 1 - z
	default:
		return 0, 0
	}
}

// ─── 调色板 ───

func loadSkinPalette(blockSet SkinBlockSet) (skinPalette, error) {
	skinPaletteMu.Lock()
	defer skinPaletteMu.Unlock()

	if palette, ok := skinPaletteCache[blockSet]; ok {
		return palette, nil
	}

	specs, err := skinPaletteSpecsForBlockSet(blockSet)
	if err != nil {
		return skinPalette{}, err
	}

	palette := skinPalette{entries: specs}
	skinPaletteCache[blockSet] = palette
	return palette, nil
}

func (p skinPalette) nearestBlockName(r, g, b uint8) string {
	target := colorful.Color{R: float64(r), G: float64(g), B: float64(b)}
	best := p.entries[0].name
	bestDist := 1e10
	for _, entry := range p.entries {
		ec := colorful.Color{R: float64(entry.r), G: float64(entry.g), B: float64(entry.b)}
		dist := target.DistanceLab(ec)
		if dist < bestDist {
			bestDist = dist
			best = entry.name
		}
	}
	return best
}

// ─── 调色板定义 ───

type skinPaletteSpec struct {
	name string
	r    uint8
	g    uint8
	b    uint8
}

func skinPaletteSpecsForBlockSet(blockSet SkinBlockSet) ([]skinPaletteSpec, error) {
	concrete := paletteWithSuffix("concrete", adjustSkinColors(skinBaseColors16, 1.00, 0.00))
	wool := paletteWithSuffix("wool", adjustSkinColors(skinBaseColors16, 1.08, 0.04))
	terracotta := paletteWithSuffix("terracotta", adjustSkinColors(skinBaseColors16, 0.90, 0.20))
	flesh := []skinPaletteSpec{
		{name: "minecraft:bone_block", r: 225, g: 214, b: 184},
		{name: "minecraft:mushroom_stem", r: 210, g: 206, b: 196},
		{name: "minecraft:clay", r: 160, g: 166, b: 179},
		{name: "minecraft:terracotta", r: 152, g: 94, b: 68},
		{name: "minecraft:packed_mud", r: 142, g: 123, b: 104},
	}
	extra := []skinPaletteSpec{
		{name: "minecraft:snow_block", r: 250, g: 252, b: 252},
		{name: "minecraft:quartz_block", r: 236, g: 233, b: 227},
		{name: "minecraft:calcite", r: 224, g: 227, b: 230},
		{name: "minecraft:iron_block", r: 220, g: 220, b: 220},
		{name: "minecraft:smooth_stone", r: 158, g: 158, b: 158},
		{name: "minecraft:stone", r: 125, g: 125, b: 125},
		{name: "minecraft:andesite", r: 136, g: 136, b: 137},
		{name: "minecraft:diorite", r: 184, g: 184, b: 186},
		{name: "minecraft:granite", r: 150, g: 103, b: 85},
		{name: "minecraft:tuff", r: 108, g: 109, b: 102},
		{name: "minecraft:dripstone_block", r: 134, g: 107, b: 92},
		{name: "minecraft:deepslate", r: 60, g: 60, b: 63},
		{name: "minecraft:blackstone", r: 44, g: 39, b: 45},
		{name: "minecraft:coal_block", r: 17, g: 17, b: 17},
		{name: "minecraft:obsidian", r: 20, g: 18, b: 30},
		{name: "minecraft:sandstone", r: 216, g: 203, b: 155},
		{name: "minecraft:smooth_sandstone", r: 220, g: 206, b: 159},
		{name: "minecraft:red_sandstone", r: 190, g: 102, b: 33},
		{name: "minecraft:smooth_red_sandstone", r: 188, g: 100, b: 30},
		{name: "minecraft:end_stone", r: 219, g: 222, b: 158},
		{name: "minecraft:ochre_froglight", r: 246, g: 217, b: 148},
		{name: "minecraft:pearlescent_froglight", r: 240, g: 236, b: 226},
		{name: "minecraft:verdant_froglight", r: 205, g: 233, b: 182},
		{name: "minecraft:oak_planks", r: 162, g: 131, b: 79},
		{name: "minecraft:spruce_planks", r: 114, g: 84, b: 48},
		{name: "minecraft:birch_planks", r: 206, g: 200, b: 144},
		{name: "minecraft:jungle_planks", r: 160, g: 115, b: 80},
		{name: "minecraft:acacia_planks", r: 169, g: 89, b: 51},
		{name: "minecraft:dark_oak_planks", r: 66, g: 43, b: 20},
		{name: "minecraft:mangrove_planks", r: 120, g: 54, b: 48},
		{name: "minecraft:crimson_planks", r: 122, g: 57, b: 84},
		{name: "minecraft:warped_planks", r: 44, g: 109, b: 89},
		{name: "minecraft:bamboo_planks", r: 203, g: 176, b: 84},
		{name: "minecraft:cherry_planks", r: 228, g: 166, b: 156},
		{name: "minecraft:amethyst_block", r: 133, g: 97, b: 191},
		{name: "minecraft:lapis_block", r: 30, g: 67, b: 140},
		{name: "minecraft:gold_block", r: 247, g: 208, b: 61},
		{name: "minecraft:redstone_block", r: 175, g: 27, b: 27},
		{name: "minecraft:packed_ice", r: 164, g: 200, b: 252},
		{name: "minecraft:blue_ice", r: 109, g: 168, b: 255},
	}

	switch blockSet {
	case SkinBlockSetConcrete:
		return concrete, nil
	case SkinBlockSetWool:
		return wool, nil
	case SkinBlockSetTerracotta:
		return terracotta, nil
	case SkinBlockSetMixed:
		return appendSpecs(concrete, wool, terracotta, flesh), nil
	case SkinBlockSetExtended:
		return appendSpecs(concrete, wool, terracotta, flesh, extra), nil
	default:
		return nil, fmt.Errorf("未知方块调色板: %s", blockSet)
	}
}

func paletteWithSuffix(suffix string, colors []skinPaletteSpec) []skinPaletteSpec {
	out := make([]skinPaletteSpec, len(colors))
	for i, spec := range colors {
		out[i] = skinPaletteSpec{
			name: "minecraft:" + spec.name + "_" + suffix,
			r:    spec.r,
			g:    spec.g,
			b:    spec.b,
		}
	}
	return out
}

func adjustSkinColors(colors []skinPaletteSpec, brightness, desaturate float64) []skinPaletteSpec {
	out := make([]skinPaletteSpec, len(colors))
	for i, spec := range colors {
		r, g, b := adjustRGB(spec.r, spec.g, spec.b, brightness, desaturate)
		out[i] = skinPaletteSpec{name: spec.name, r: r, g: g, b: b}
	}
	return out
}

func adjustRGB(r, g, b uint8, brightness, desaturate float64) (uint8, uint8, uint8) {
	rf, gf, bf := float64(r), float64(g), float64(b)
	lum := 0.2126*rf + 0.7152*gf + 0.0722*bf
	rf = ((1-desaturate)*rf + desaturate*lum) * brightness
	gf = ((1-desaturate)*gf + desaturate*lum) * brightness
	bf = ((1-desaturate)*bf + desaturate*lum) * brightness
	return clampByte(rf), clampByte(gf), clampByte(bf)
}

func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(math.Round(v))
}

func appendSpecs(groups ...[]skinPaletteSpec) []skinPaletteSpec {
	total := 0
	for _, g := range groups {
		total += len(g)
	}
	out := make([]skinPaletteSpec, 0, total)
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// 基础 16 色
var skinBaseColors16 = []skinPaletteSpec{
	{name: "white", r: 207, g: 213, b: 214},
	{name: "orange", r: 224, g: 97, b: 0},
	{name: "magenta", r: 169, g: 48, b: 159},
	{name: "light_blue", r: 36, g: 137, b: 199},
	{name: "yellow", r: 241, g: 175, b: 21},
	{name: "lime", r: 94, g: 168, b: 24},
	{name: "pink", r: 213, g: 101, b: 142},
	{name: "gray", r: 54, g: 57, b: 61},
	{name: "light_gray", r: 125, g: 125, b: 115},
	{name: "cyan", r: 21, g: 119, b: 136},
	{name: "purple", r: 100, g: 31, b: 156},
	{name: "blue", r: 44, g: 46, b: 143},
	{name: "brown", r: 96, g: 59, b: 31},
	{name: "green", r: 73, g: 91, b: 36},
	{name: "red", r: 142, g: 32, b: 32},
	{name: "black", r: 8, g: 10, b: 15},
}
