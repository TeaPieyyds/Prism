package main

// ai_proxy.go —— 把"本地 AI 代理"整合进 prism 服务，对外暴露为 /api/prism/ai。
//
// 功能：
//   - 按降级链转发到多个 OpenAI 兼容上游，遇 429（Too Many Requests）自动换下一个。
//   - 支持流式响应（SSE）与非流式；流式逐帧透传，并设 X-Accel-Buffering:no 让 nginx 不缓冲。
//   - 配置在 ai_proxy.json（providers 数组：base/key/models；降级顺序 = 数组内嵌顺序）。
//   - 端点经 auth.SessionAuth 鉴权（登录后才能用，保护 API key 额度）。

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

// aiProvider 一个上游提供商（OpenAI 兼容端点）。
type aiProvider struct {
	Name   string   `json:"name"`
	Base   string   `json:"base"`   // 如 https://token.sensenova.cn/v1
	Key    string   `json:"key"`    // API key
	Models []string `json:"models"` // 该提供商下的模型 id，依序尝试
}

// aiProxyConfig 配置文件结构。
type aiProxyConfig struct {
	Providers []aiProvider `json:"providers"`
}

// upstream 展平后的单个尝试目标。
type upstream struct {
	name  string
	base  string
	key   string
	model string
}

// aiProxy 代理实例。
type aiProxy struct {
	chain []upstream
	hc    *http.Client

	mu  sync.Mutex
	cur int // 当前降级游标（链下标，初始 0 = 链首）。429 后粘滞降级，后续请求从此处开始。
	succ int // 当前模型的连续成功次数，达到阈值后尝试恢复到更优模型
}

// upgradeAfter 在降级模型连续成功多少次后，尝试升回上一级（更优模型）。
const upgradeAfter = 3

// buildAIConfig 读取 ai_proxy.json。
func buildAIConfig(path string) (*aiProxyConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c aiProxyConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// newAIProxy 把 providers 展平成降级链：provider 内模型依序，然后下一个 provider。
func newAIProxy(cfg *aiProxyConfig) *aiProxy {
	var chain []upstream
	for _, p := range cfg.Providers {
		for _, m := range p.Models {
			chain = append(chain, upstream{name: p.Name, base: p.Base, key: p.Key, model: m})
		}
	}
	return &aiProxy{
		chain: chain,
		// 无超时客户端：流式响应可能持续很久，靠请求 context 控制生命周期。
		hc: &http.Client{Timeout: 0},
	}
}

// handleModels 列出降级链上所有模型。
func (a *aiProxy) handleModels(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	data := make([]map[string]string, 0, len(a.chain))
	for _, u := range a.chain {
		data = append(data, map[string]string{"id": u.model, "provider": u.name})
	}
	json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
}

// handleChat 处理 chat/completions，带 429 降级与流式透传。
func (a *aiProxy) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		http.Error(w, "读取请求体失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	// 解析以读取 stream 标志（也用于校验 body 是合法 JSON）
	var reqMap map[string]any
	if err := json.Unmarshal(body, &reqMap); err != nil {
		http.Error(w, "请求体不是合法 JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	stream, _ := reqMap["stream"].(bool)

	// 从当前降级游标开始尝试（粘滞降级：429 后后续请求不再每次都从链首撞 429）。
	a.mu.Lock()
	start := a.cur
	a.mu.Unlock()

	i := start
	for {
		if i >= len(a.chain) {
			// 链尾仍 429：保持游标（已到链尾），返回 429
			a.mu.Lock()
			a.succ = 0
			a.mu.Unlock()
			log.Printf("[AI代理][降级] 从 %d 到链尾均 429", start)
			http.Error(w, "所有上游均返回 429（速率受限），请稍后重试", http.StatusTooManyRequests)
			return
		}
		up := a.chain[i]
		resp, status, err := a.forward(r, up, body)
		if err != nil {
			log.Printf("[AI代理][降级] 目标 %s/%s 请求失败: %v", up.name, up.model, err)
			i++
			continue
		}
		if status == http.StatusTooManyRequests {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			// 429：降级——游标后移，后续请求从下一个开始；本次也继续尝试下一个
			a.mu.Lock()
			a.cur = i + 1
			a.succ = 0
			a.mu.Unlock()
			log.Printf("[AI代理][降级] %s/%s 返回 429，游标降到 %d", up.name, up.model, a.cur)
			i++
			continue
		}
		if status != http.StatusOK {
			// 其它非限流错误：透传，不改游标
			log.Printf("[AI代理] 目标 %s/%s 返回 %d", up.name, up.model, status)
			writeAIResponse(w, resp, stream)
			return
		}
		// 成功：粘滞在当前模型；连续成功达到阈值则尝试升回上一级（更优模型）
		a.mu.Lock()
		a.cur = i
		a.succ++
		if a.succ >= upgradeAfter && i > 0 {
			a.cur = i - 1
			a.succ = 0
			log.Printf("[AI代理] %s/%s 连续成功 %d 次，尝试升回 %s/%s", up.name, up.model, upgradeAfter, a.chain[i-1].name, a.chain[i-1].model)
		}
		a.mu.Unlock()
		log.Printf("[AI代理] 目标 %s/%s 返回 200", up.name, up.model)
		writeAIResponse(w, resp, stream)
		return
	}
}

// forward 向单个上游发起请求，body 里的 model 字段被替换为当前目标模型。
func (a *aiProxy) forward(r *http.Request, up upstream, body []byte) (*http.Response, int, error) {
	var reqMap map[string]any
	if err := json.Unmarshal(body, &reqMap); err != nil {
		return nil, 0, err
	}
	reqMap["model"] = up.model
	payload, err := json.Marshal(reqMap)
	if err != nil {
		return nil, 0, err
	}

	url := strings.TrimRight(up.base, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+up.key)
	if accept := r.Header.Get("Accept"); accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	return resp, resp.StatusCode, nil
}

// writeAIResponse 透传上游响应。流式时逐帧 flush；设置 X-Accel-Buffering:no 让 nginx 不缓冲 SSE。
func writeAIResponse(w http.ResponseWriter, resp *http.Response, stream bool) {
	defer resp.Body.Close()
	// 通知 nginx 不要缓冲流式响应
	w.Header().Set("X-Accel-Buffering", "no")
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if !stream {
		_, _ = io.Copy(w, resp.Body)
		return
	}
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}
