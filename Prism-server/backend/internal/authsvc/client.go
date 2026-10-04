package authsvc

import (
	"context"
	"fmt"
	"strings"

	g79 "github.com/Yeah114/g79client"
	"github.com/adb-lanlu/prism-oss/internal/netproxy"
)

func NewClient(ctx context.Context, pm *netproxy.Manager) (*g79.Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if pm == nil {
		return g79.NewClient()
	}
	httpClient, _, err := pm.AcquireHTTPClient()
	if err != nil {
		return nil, err
	}
	return g79.NewClientWithHTTPClient(httpClient)
}

func extractFirstJSONObject(raw string) string {
	start := strings.Index(raw, "{")
	if start < 0 {
		return ""
	}
	inString := false
	escaped := false
	depth := 0
	for i := start; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == '{' {
			depth++
			continue
		}
		if ch == '}' {
			depth--
			if depth == 0 {
				return strings.TrimSpace(raw[start : i+1])
			}
		}
	}
	return ""
}

func sanitizeCookie(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if obj := extractFirstJSONObject(raw); obj != "" {
		return obj
	}
	return raw
}

func AuthenticateWithCookie(ctx context.Context, cookie string, pm *netproxy.Manager) (*g79.Client, error) {
	cli, err := NewClient(ctx, pm)
	if err != nil {
		return nil, err
	}
	cookie = sanitizeCookie(cookie)
	if err := cli.G79AuthenticateWithCookie(cookie); err != nil {
		return nil, fmt.Errorf("cookie 验证失败: %w", err)
	}
	return cli, nil
}
