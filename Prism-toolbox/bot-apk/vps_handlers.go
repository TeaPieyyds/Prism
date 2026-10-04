//go:build !android

package main

import (
	"net/http"
	"os"
	"time"
)

func init() {
	handleSystemRestart = func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "msg": "restarting"})
		go func() {
			time.Sleep(200 * time.Millisecond)
			os.Exit(1)
		}()
	}
}
