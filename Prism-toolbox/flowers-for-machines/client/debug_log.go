package client

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// prismDebugLogPath 调试日志文件路径（Android 设备下载目录）。
// 桌面调试时可改为本地路径。
var prismDebugLogPath = "/storage/emulated/0/Download/prism_debug.log"

var (
	debugMu   sync.Mutex
	debugFile *os.File
)

// SetDebugLogPath 覆盖调试日志输出路径（供桌面调试使用）。
func SetDebugLogPath(path string) {
	prismDebugLogPath = path
}

// DebugLog 写入一行带时间戳的调试日志到指定文件。
// 文件打开失败时静默忽略（不影响正常流程）。
func DebugLog(format string, args ...any) {
	debugMu.Lock()
	defer debugMu.Unlock()

	if debugFile == nil {
		f, err := os.OpenFile(prismDebugLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return
		}
		debugFile = f
	}

	ts := time.Now().Format("2006-01-02 15:04:05.000")
	line := fmt.Sprintf("[%s] %s\n", ts, fmt.Sprintf(format, args...))
	_, _ = debugFile.WriteString(line)
}