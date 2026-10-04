package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

const tokenPrefix = "xiaoruo/"

func generateRandomToken() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func normalizeToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("TOKEN 不能为空")
	}
	return token, nil
}

func withPrefix(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, tokenPrefix) {
		return content
	}
	return tokenPrefix + content
}

func loadLocalCookie() (string, error) {
	for _, name := range []string{"cookie.txt", "cookies.txt"} {
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		c := strings.TrimSpace(string(data))
		if c != "" {
			return c, nil
		}
	}
	return "", fmt.Errorf("需要本地 cookie.txt/cookies.txt（当前目录未找到或为空）")
}

func loadCookieTxtOnly() (string, error) {
	data, err := os.ReadFile("cookie.txt")
	if err != nil {
		return "", fmt.Errorf("随机/自定义 token 需要本地 cookie.txt")
	}
	c := strings.TrimSpace(string(data))
	if c == "" {
		return "", fmt.Errorf("cookie.txt 为空")
	}
	return c, nil
}
