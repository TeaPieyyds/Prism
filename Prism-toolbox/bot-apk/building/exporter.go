// Deprecated: 此文件已废弃，无人引用。
// 导出功能由 task_dispatch.go 的 runExportTask() 直接实现：
//   - mcworld 格式 → RunSubChunkExport() (export_subchunk.go)
//   - mcstructure/schematic 格式 → bm.RequestStructure() + mergeChunkStructures() (task_dispatch.go 内联)
// 本文件保留仅供历史参考，将在后续清理中移除。
package building

import (
	"fmt"
	"os"
	"time"
)

const ExportChunkSize = 16

type ExportTask struct {
	StartX     int
	StartY     int
	StartZ     int
	EndX       int
	EndY       int
	EndZ       int
	OutputPath string
	Format     string
	Speed      int
	OnProgress func(chunk int, total int)
	StopCh     <-chan struct{}
}

func (t *ExportTask) Run() error {
	sx := min(t.StartX, t.EndX)
	ex := max(t.StartX, t.EndX)
	sy := min(t.StartY, t.EndY)
	ey := max(t.StartY, t.EndY)
	sz := min(t.StartZ, t.EndZ)
	ez := max(t.StartZ, t.EndZ)

	dx := ex - sx + 1
	dy := ey - sy + 1
	dz := ez - sz + 1

	totalChunksX := (dx + ExportChunkSize - 1) / ExportChunkSize
	totalChunksZ := (dz + ExportChunkSize - 1) / ExportChunkSize
	totalChunks := totalChunksX * totalChunksZ
	chunkNum := 0

	for cx := 0; cx < totalChunksX; cx++ {
		czRange := make([]int, totalChunksZ)
		if cx%2 == 0 {
			for i := 0; i < totalChunksZ; i++ {
				czRange[i] = i
			}
		} else {
			for i := 0; i < totalChunksZ; i++ {
				czRange[i] = totalChunksZ - 1 - i
			}
		}

		for _, cz := range czRange {
			select {
			case <-t.StopCh:
				return fmt.Errorf("export cancelled at chunk (%d,%d)", cx, cz)
			default:
			}

			chunkX := sx + cx*ExportChunkSize
			chunkZ := sz + cz*ExportChunkSize
			chunkDX := min(ExportChunkSize, ex-chunkX+1)
			chunkDZ := min(ExportChunkSize, ez-chunkZ+1)

			_ = chunkX
			_ = chunkZ
			_ = chunkDX
			_ = chunkDZ
			_ = dy

			time.Sleep(time.Second / time.Duration(t.Speed))

			chunkNum++
			if t.OnProgress != nil {
				t.OnProgress(chunkNum, totalChunks)
			}
		}
	}

	switch t.Format {
	case "mcstructure":
		return t.writeMCStructure(nil)
	default:
		return fmt.Errorf("unsupported export format: %s", t.Format)
	}
}

func (t *ExportTask) writeMCStructure(_ any) error {
	f, err := os.Create(t.OutputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return nil
}
