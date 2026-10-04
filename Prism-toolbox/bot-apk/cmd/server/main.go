package main

import (
	"fmt"
	"os"
	mainpkg "bot-apk"
)

func main() {
	port := "8080"
	dataDir := "/tmp/td_data"
	os.MkdirAll(dataDir, 0755)
	fmt.Printf("http://127.0.0.1:%s\n", port)
	fmt.Println("data:", dataDir)
	if err := mainpkg.StartServer(port, dataDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
