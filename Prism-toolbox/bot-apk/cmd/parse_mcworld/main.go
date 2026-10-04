package main

import (
	"fmt"
	"os"
	"strings"

	"bot-apk/building"
)

func main() {
	path := ""
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	data, err := building.LoadStructureFileCached(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("尺寸: %dx%dx%d\n", data.SizeX, data.SizeY, data.SizeZ)
	fmt.Printf("调色板: %d 种方块\n", len(data.Palette))
	fmt.Printf("方块数: %d\n", len(data.Blocks))
	fmt.Printf("NBT方块: %d\n", len(data.NBTBlocks))

	for pos, idx := range data.Blocks {
		block := data.Palette[idx]
		if strings.Contains(block.Name, "barrel") || strings.Contains(block.Name, "chest") || strings.Contains(block.Name, "shulker") {
			x, y, z := unpackPos(pos)
			fmt.Printf("\n=== %s at (%d,%d,%d) ===\n", block.Name, x, y, z)
			fmt.Printf("  States: %s\n", block.States)
			nbt := data.GetNBT(x, y, z)
			if nbt != nil {
				fmt.Printf("  NBT:\n")
				for k, v := range nbt {
					switch val := v.(type) {
					case map[string]any:
						fmt.Printf("    %s: (compound)\n", k)
						for k2, v2 := range val {
							fmt.Printf("      %s: %v (T:%T)\n", k2, v2, v2)
						}
					case []any:
						fmt.Printf("    %s: (list len=%d)\n", k, len(val))
						for i, item := range val {
							if m, ok := item.(map[string]any); ok {
								fmt.Printf("      [%d]:\n", i)
								for mk, mv := range m {
									if mk == "tag" {
										fmt.Printf("        tag: %v\n", mv)
									} else {
										fmt.Printf("        %s: %v (T:%T)\n", mk, mv, mv)
									}
								}
							}
						}
					default:
						fmt.Printf("    %s: %v (T:%T)\n", k, v, v)
					}
				}
			}
		}
	}
}

func unpackPos(key uint64) (x, y, z int) {
	return int(int32(key >> 40)), int(int32((key >> 20) & 0xFFFFF)), int(int32(key & 0xFFFFF))
}