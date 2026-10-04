package main

import "bot-apk/building"

// loadStructureFileCached delegates to building.LoadStructureFileCached.
// This wrapper exists so callers in the main package don't need to import
// building's cache internals directly.
func loadStructureFileCached(path string, region ...[6]int) (*building.StructureData, error) {
	return building.LoadStructureFileCached(path, region...)
}

// clearBuildingCache delegates to building.ClearCache.
func clearBuildingCache() {
	building.ClearCache()
}