package building

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RenderTopDown 生成俯视图 PNG
// 从上往下看，每个方块占一个像素
func RenderTopDown(data *StructureData, pixelSize int, outputPath string) error {
	if pixelSize <= 0 {
		pixelSize = 1
	}
	// 俯视图：X为宽，Z为高
	imgW := data.SizeX * pixelSize
	imgH := data.SizeZ * pixelSize
	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))

	// 先填充背景（浅色）
	backgroundColor := color.RGBA{240, 238, 232, 255}
	for y := 0; y < imgH; y++ {
		for x := 0; x < imgW; x++ {
			img.Set(x, y, backgroundColor)
		}
	}

	// 构建位置映射：从顶部看，每个XZ位置取最高Y的方块
	type topBlockInfo struct {
		c color.RGBA
		y int32
	}
	topBlocks := make(map[[2]int]topBlockInfo)
	for pos, idx := range data.Blocks {
		entry := data.Palette[idx]
		if IsAirBlock(entry.Name) {
			continue
		}
		x, y, z := unpackPos(pos)
		pos2 := [2]int{int(x), int(z)}
		existing, ok := topBlocks[pos2]
		if !ok || y > existing.y {
			topBlocks[pos2] = topBlockInfo{c: hexToRGBA(blockCategoryColor(entry.Name)), y: y}
		}
	}

	// 绘制方块
	for pos2, info := range topBlocks {
		c := info.c
		px := pos2[0] * pixelSize
		pz := pos2[1] * pixelSize
		for dy := 0; dy < pixelSize; dy++ {
			for dx := 0; dx < pixelSize; dx++ {
				img.Set(px+dx, pz+dy, c)
			}
		}
	}

	// 写文件
	os.MkdirAll(filepath.Dir(outputPath), 0755)
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// RenderElevation 生成立面图（前/后/左/右）
// direction: "front", "back", "left", "right"
func RenderElevation(data *StructureData, direction string, pixelSize int, outputPath string) error {
	if pixelSize <= 0 {
		pixelSize = 1
	}

	// 确定投影平面
	var imgW, imgH int
	var projectFn func(int, int, int) (int, int)
	switch direction {
	case "front":
		imgW = data.SizeX * pixelSize
		imgH = data.SizeY * pixelSize
		projectFn = func(x, y, z int) (int, int) { return x, data.SizeY - 1 - y }
	case "back":
		imgW = data.SizeX * pixelSize
		imgH = data.SizeY * pixelSize
		projectFn = func(x, y, z int) (int, int) { return data.SizeX - 1 - x, data.SizeY - 1 - y }
	case "left":
		imgW = data.SizeZ * pixelSize
		imgH = data.SizeY * pixelSize
		projectFn = func(x, y, z int) (int, int) { return z, data.SizeY - 1 - y }
	case "right":
		imgW = data.SizeZ * pixelSize
		imgH = data.SizeY * pixelSize
		projectFn = func(x, y, z int) (int, int) { return data.SizeZ - 1 - z, data.SizeY - 1 - y }
	default:
		imgW = data.SizeX * pixelSize
		imgH = data.SizeY * pixelSize
		projectFn = func(x, y, z int) (int, int) { return x, data.SizeY - 1 - y }
	}

	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))

	// 填充背景
	backgroundColor := color.RGBA{240, 238, 232, 255}
	for y := 0; y < imgH; y++ {
		for x := 0; x < imgW; x++ {
			img.Set(x, y, backgroundColor)
		}
	}

	// 按距离排序，远的先画（被近的覆盖）
	type blockProj struct {
		px, py int
		dist   int
		c      color.RGBA
	}
	var projections []blockProj

	for pos, idx := range data.Blocks {
		entry := data.Palette[idx]
		if IsAirBlock(entry.Name) {
			continue
		}
		x, y, z := unpackPos(pos)
		px, py := projectFn(int(x), int(y), int(z))
		dist := 0
		switch direction {
		case "front":
			dist = int(z)
		case "back":
			dist = data.SizeZ - 1 - int(z)
		case "left":
			dist = int(x)
		case "right":
			dist = data.SizeX - 1 - int(x)
		}
		projections = append(projections, blockProj{px, py, dist, hexToRGBA(blockCategoryColor(entry.Name))})
	}

	// 按距离从远到近排序（远的先画，近的覆盖）
	sort.Slice(projections, func(i, j int) bool {
		return projections[i].dist > projections[j].dist
	})

	// 直接绘制
	for _, p := range projections {
		for dy := 0; dy < pixelSize; dy++ {
			for dx := 0; dx < pixelSize; dx++ {
				img.Set(p.px*pixelSize+dx, p.py*pixelSize+dy, p.c)
			}
		}
	}

	os.MkdirAll(filepath.Dir(outputPath), 0755)
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// RenderIsometric 生成等轴测视角 PNG
// yaw: 水平旋转角度 (0-360), pitch: 俯仰角度 (0-90), outputWidth: 期望输出图片宽度（像素）
func RenderIsometric(data *StructureData, yaw, pitch float64, outputWidth int, outputPath string) error {
	// 根据建筑尺寸和期望输出宽度计算像素大小
	maxDim := max(data.SizeX, max(data.SizeY, data.SizeZ))
	if maxDim < 1 {
		maxDim = 1
	}
	pixelSize := outputWidth / (maxDim * 3)
	if pixelSize < 1 {
		pixelSize = 1
	}
	if pixelSize > 20 {
		pixelSize = 20
	}

	imgSize := (maxDim + 2) * pixelSize * 3
	// 如果实际尺寸超过期望宽度太多，缩小 pixelSize
	for imgSize > outputWidth*2 && pixelSize > 1 {
		pixelSize--
		imgSize = (maxDim + 2) * pixelSize * 3
	}

	img := image.NewRGBA(image.Rect(0, 0, imgSize, imgSize))

	// 填充背景
	bgColor := color.RGBA{240, 238, 232, 255}
	for y := 0; y < imgSize; y++ {
		for x := 0; x < imgSize; x++ {
			img.Set(x, y, bgColor)
		}
	}

	centerX := imgSize / 2
	centerY := imgSize / 3

	// 角度转弧度
	radYaw := yaw * math.Pi / 180.0
	radPitch := pitch * math.Pi / 180.0

	// 收集所有方块
	type block2D struct {
		sx, sy int
		depth  float64
		c      color.RGBA
	}
	var blocks2D []block2D
	cx := float64(data.SizeX) / 2.0
	cy := float64(data.SizeY) / 2.0
	cz := float64(data.SizeZ) / 2.0

	cosY := math.Cos(radYaw)
	sinY := -math.Sin(radYaw) // 取反：相机绕 Y 轴旋转，方块应反向旋转
	cosP := math.Cos(radPitch)
	sinP := math.Sin(radPitch)

	if data.BlocksFlat != nil {
		stride := data.SizeY * data.SizeX
		for z := 0; z < data.SizeZ; z++ {
			for y := 0; y < data.SizeY; y++ {
				base := z*stride + y*data.SizeX
				for x := 0; x < data.SizeX; x++ {
					palIdx := data.BlocksFlat[base+x]
					if palIdx == 0 {
						continue
					}
					entry := data.Palette[palIdx]
					if IsAirBlock(entry.Name) || IsInvisibleBlock(entry.Name) {
						continue
					}
					fx := float64(x) - cx
					fy := float64(y) - cy
					fz := float64(z) - cz

		// 绕 Y 轴旋转 (yaw) — 相机视角变换
		rx := fx*cosY - fz*sinY
		rz := fx*sinY + fz*cosY

		// 绕 X 轴旋转 (pitch)
		ry := -fy*cosP + rz*sinP
		rz2 := fy*sinP + rz*cosP

		// 等轴测投影到屏幕（标准等轴测矩阵）
		sx := int(rx*0.8-rz2*0.8) * pixelSize
		sy := int(rx*0.4+rz2*0.4-ry) * pixelSize

		// 居中
		sx += centerX
		sy += centerY

		// 深度排序 (远-近)
		depth := rx + rz2 - ry

			blocks2D = append(blocks2D, block2D{sx, sy, depth, hexToRGBA(blockCategoryColor(entry.Name))})
				}
			}
		}
	} else {
		for pos, idx := range data.Blocks {
			entry := data.Palette[idx]
			if IsAirBlock(entry.Name) || IsInvisibleBlock(entry.Name) {
				continue
			}
			x, y, z := unpackPos(pos)
			fx := float64(int(x)) - cx
			fy := float64(int(y)) - cy
			fz := float64(int(z)) - cz
			rx := fx*cosY - fz*sinY
			rz := fx*sinY + fz*cosY
			ry := -fy*cosP + rz*sinP
			rz2 := fy*sinP + rz*cosP
			sx := int(rx*0.8-rz2*0.8) * pixelSize
			sy := int(rx*0.4+rz2*0.4-ry) * pixelSize
			sx += centerX
			sy += centerY
			depth := rx + rz2 - ry
			blocks2D = append(blocks2D, block2D{sx, sy, depth, hexToRGBA(blockCategoryColor(entry.Name))})
		}
	}

	// 按深度排序（远的先画）
	sort.Slice(blocks2D, func(i, j int) bool {
		return blocks2D[i].depth > blocks2D[j].depth
	})

	// 绘制方块
	for _, b := range blocks2D {
		if b.sx < 0 || b.sy < 0 || b.sx+pixelSize >= imgSize || b.sy+pixelSize >= imgSize {
			continue
		}
		for dy := 0; dy < pixelSize; dy++ {
			for dx := 0; dx < pixelSize; dx++ {
				img.Set(b.sx+dx, b.sy+dy, b.c)
			}
		}
	}

	os.MkdirAll(filepath.Dir(outputPath), 0755)
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// hexToRGBA 将 "#rrggbb" 转为 color.RGBA
func hexToRGBA(hex string) color.RGBA {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return color.RGBA{233, 69, 96, 255} // 默认粉色
	}
	r := hexToByte(hex[0:2])
	g := hexToByte(hex[2:4])
	b := hexToByte(hex[4:6])
	return color.RGBA{r, g, b, 255}
}

func hexToByte(s string) uint8 {
	var v uint8
	for _, c := range s {
		v *= 16
		switch {
		case c >= '0' && c <= '9':
			v += uint8(c - '0')
		case c >= 'a' && c <= 'f':
			v += uint8(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			v += uint8(c - 'A' + 10)
		}
	}
	return v
}