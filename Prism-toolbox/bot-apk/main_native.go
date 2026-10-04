//go:build !android
package main
import ("fmt";"os")
func main() {
	if err := StartServer("8080", "/tmp/td_data"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
