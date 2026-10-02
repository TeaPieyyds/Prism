package console

import (
	"fmt"
	"strings"
)

const (
	cReset = "\033[0m"
	cCyan  = "\033[36m"
	cGreen = "\033[32m"
	cRed   = "\033[31m"
	cYel   = "\033[33m"
	cBlue  = "\033[34m"
)

func PrintBanner() {
	fmt.Println(cCyan + `██╗  ██╗██╗   ██╗██╗  ██╗ █████╗ ██╗   ██╗████████╗██╗  ██╗
██║  ██║██║   ██║╚██╗██╔╝██╔══██╗██║   ██║╚══██╔══╝██║  ██║
███████║██║   ██║ ╚███╔╝ ███████║██║   ██║   ██║   ███████║
██╔══██║██║   ██║ ██╔██╗ ██╔══██║██║   ██║   ██║   ██╔══██║
██║  ██║╚██████╔╝██╔╝ ██╗██║  ██║╚██████╔╝   ██║   ██║  ██║
╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝    ╚═╝   ╚═╝  ╚═╝` + cReset)
	fmt.Println(Info("Prism - 网易我的世界验证项目"))
}

func Title(s string) string { return cBlue + "[+] " + s + cReset }
func OK(s string) string    { return cGreen + "[√] " + s + cReset }
func Err(s string) string   { return cRed + "[x] " + s + cReset }
func Warn(s string) string  { return cYel + "[!] " + s + cReset }
func Info(s string) string  { return cCyan + "[*] " + s + cReset }

func MaskToken(token string) string {
	if len(token) <= 14 {
		return token
	}
	return token[:10] + strings.Repeat("*", len(token)-14) + token[len(token)-4:]
}
