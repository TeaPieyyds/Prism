//go:build !android

package main

func getDeviceModel() string {
	return "desktop"
}