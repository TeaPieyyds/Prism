//go:build android

package main

import "os"
import "path/filepath"

var cachedDeviceModel string

func getDeviceModel() string {
	if cachedDeviceModel != "" {
		return cachedDeviceModel
	}
	// 从 Java 写入的文件读取设备型号
	data, err := os.ReadFile(filepath.Join(appDataDir, "device_model"))
	if err == nil && len(data) > 0 {
		cachedDeviceModel = string(data)
		return cachedDeviceModel
	}
	return "android"
}