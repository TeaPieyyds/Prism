package main

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

// Messages 提供可热重载的错误消息配置。
// 通用重复消息路由到 messages.json,改文案只需编辑该文件,无需重建重启。
type Messages struct {
	mu   sync.RWMutex
	m    map[string]string
	path string
}

// msg 是全局消息配置单例。所有 handler 通过 msg.Get(key, fallback) 取文案:
// 命中配置返回配置值,未命中(或配置缺失)返回内联 fallback,保证缺配置也能跑。
const messagesPath = "messages.json"

var msg = LoadMessages(messagesPath)

// LoadMessages 从 path 读取 {"key":"文案"} 到缓存,并启动热重载协程。
func LoadMessages(path string) *Messages {
	ms := &Messages{path: path}
	ms.load()
	go ms.watch()
	return ms
}

func (ms *Messages) load() {
	data, err := os.ReadFile(ms.path)
	if err != nil {
		log.Printf("[MSG] 读取消息配置失败(将使用内置默认): %v", err)
		return
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		log.Printf("[MSG] 解析消息配置失败: %v", err)
		return
	}
	ms.mu.Lock()
	ms.m = m
	ms.mu.Unlock()
	log.Printf("[MSG] 已加载 %d 条消息配置", len(m))
}

// Get 返回 key 对应的配置文案;未命中则返回 fallback。
func (ms *Messages) Get(key, fallback string) string {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if v, ok := ms.m[key]; ok && v != "" {
		return v
	}
	return fallback
}

// Snapshot 返回当前配置的副本(供管理界面展示)。
func (ms *Messages) Snapshot() map[string]string {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	out := make(map[string]string, len(ms.m))
	for k, v := range ms.m {
		out[k] = v
	}
	return out
}

// SetMessages 把新配置写入该实例对应的文件并立即重载(无需等待 5s 热重载协程)。
// 值传空字符串表示该 key 回退到内联默认文案。
func (ms *Messages) SetMessages(m map[string]string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(ms.path, data, 0644); err != nil {
		return err
	}
	ms.mu.Lock()
	ms.m = m
	ms.mu.Unlock()
	return nil
}

// sysErr 把内部错误包裹成通用中文提示,不向用户泄露底层(SQL/路径)信息。
// 底层错误仅记日志便于排查,避免把数据库/内部细节暴露给前端。
func sysErr(err error) string {
	if err != nil {
		log.Printf("[SYSERR] %v", err)
	}
	return msg.Get("system_error", "哎呀,系统开小差了,请稍后重试~")
}

// watch 每 5s 轮询文件 mtime,改动则热重载(复用 proxy_pool.go 的 Ticker+ModTime 模式)。
// admin 接口写入时会调用 load 立即重载,此处作为外部直接改文件的兜底。
func (ms *Messages) watch() {
	var lastMod time.Time
	for range time.NewTicker(5 * time.Second).C {
		if st, err := os.Stat(ms.path); err == nil && !st.ModTime().Equal(lastMod) {
			lastMod = st.ModTime()
			ms.load()
		}
	}
}
