package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── 验证码预生成池 ──
//
// 预先生成一批验证码，请求时直接从池中弹出返回，避免每次请求现场绘制叠加在
// 高负载上。每个验证码最多被校验 captchaMaxAttempts(3) 次，达到上限或过期后丢弃。
// 池低于阈值且系统负载较低时后台补充；负载较高时不再生成新验证码，改为复用已丢弃
// 的（省去画图开销）。每天零点清空全部验证码并重新生成。

const (
	captchaPreGenLow       = 30  // 池低于此值触发补充
	captchaPreGenHigh      = 80  // 补充到此值
	captchaRefillMaxLoad   = 2.0 // 系统负载(1min)低于此值才生成新验证码
	captchaDiscardedMax    = 200 // 已丢弃可复用池的上限，防止无界增长
	captchaPoolRefillEvery = 5 * time.Second
)

// captchaPreGen 预生成的验证码：cid + 答案 + 已绘制的图片(base64)。
type captchaPreGen struct {
	cid    string
	answer int
	image  string
}

var (
	captchaPreGenMu sync.Mutex
	preGenPool      []*captchaPreGen // 已预生成、尚未下发的池
	discardedPool   []*captchaPreGen // 已丢弃但可复用的池（cid 为空，复用时重新生成）
)

func newCaptchaID() string {
	return fmt.Sprintf("captcha_%d_%d", time.Now().UnixNano(), rand.Int63())
}

// makePreGenCaptcha 现场生成一个验证码（问题 + 答案 + 绘制图片）。
func makePreGenCaptcha() *captchaPreGen {
	a := rand.Intn(30) + 5
	b := rand.Intn(30) + 5
	if rand.Intn(2) == 0 {
		a += 10
	} else {
		a, b = a+b, a
	}
	op := "+"
	answer := a + b
	if rand.Intn(2) == 0 {
		op = "-"
		if a < b {
			a, b = b, a
		}
		answer = a - b
	}
	return &captchaPreGen{cid: newCaptchaID(), answer: answer, image: drawCaptcha(a, b, op)}
}

// systemLoad 返回系统 1 分钟平均负载。
func systemLoad() float64 {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}
	parts := strings.SplitN(strings.TrimSpace(string(b)), " ", 2)
	if len(parts) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(parts[0], 64)
	return v
}

// fillCaptchaPool 补充验证码池。force=true 时忽略负载门限（用于启动初始种子）；
// force=false 时仅低负载才生成新验证码，高负载跳过（届时 serveCaptcha 复用已丢弃）。
func fillCaptchaPool(force bool) {
	captchaPreGenMu.Lock()
	defer captchaPreGenMu.Unlock()
	if len(preGenPool) >= captchaPreGenLow {
		return
	}
	if !force && systemLoad() >= captchaRefillMaxLoad {
		return
	}
	n := captchaPreGenHigh - len(preGenPool)
	for i := 0; i < n; i++ {
		preGenPool = append(preGenPool, makePreGenCaptcha())
	}
}

// refillCaptchaPool 周期补充入口，遵循负载门限。
func refillCaptchaPool() {
	fillCaptchaPool(false)
}

// serveCaptcha 从池中取一个验证码返回。优先用预生成；预生成耗尽或负载高时复用已丢弃的；
// 都没有则现场生成兜底。返回 (cid, 图片)。
func serveCaptcha() (string, string) {
	highLoad := systemLoad() >= captchaRefillMaxLoad

	captchaPreGenMu.Lock()
	var pg *captchaPreGen
	// 高负载优先复用已丢弃的（省去画图，避免加重负载）
	if len(preGenPool) == 0 || highLoad {
		if len(discardedPool) > 0 {
			pg = discardedPool[len(discardedPool)-1]
			discardedPool = discardedPool[:len(discardedPool)-1]
			pg.cid = newCaptchaID()
		}
	}
	if pg == nil && len(preGenPool) > 0 {
		pg = preGenPool[len(preGenPool)-1]
		preGenPool = preGenPool[:len(preGenPool)-1]
	}
	captchaPreGenMu.Unlock()

	if pg == nil {
		pg = makePreGenCaptcha() // 兜底：现场生成
	}

	captchaAnswersMu.Lock()
	captchaAnswers[pg.cid] = &captchaEntry{answer: pg.answer, created: time.Now(), image: pg.image}
	captchaAnswersMu.Unlock()
	return pg.cid, pg.image
}

// discardCaptcha 把一个未消费(过期/尝试超限)的验证码放入可复用池，供高负载时复用。
func discardCaptcha(e *captchaEntry) {
	if e == nil || e.image == "" {
		return
	}
	captchaPreGenMu.Lock()
	if len(discardedPool) < captchaDiscardedMax {
		discardedPool = append(discardedPool, &captchaPreGen{answer: e.answer, image: e.image})
	}
	captchaPreGenMu.Unlock()
}

// startCaptchaPool 启动预生成池：初始补充 + 周期补充 + 每天零点清空重生成。
func startCaptchaPool() {
	fillCaptchaPool(true) // 启动时无条件播种，保证池始终有底量
	captchaPreGenMu.Lock()
	log.Printf("[CAPTCHA] 验证码池就绪: 预生成 %d, 可复用 %d", len(preGenPool), len(discardedPool))
	captchaPreGenMu.Unlock()

	go func() {
		for range time.NewTicker(captchaPoolRefillEvery).C {
			refillCaptchaPool()
		}
	}()

	go func() {
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(24 * time.Hour)
			time.Sleep(time.Until(next))
			captchaPreGenMu.Lock()
			preGenPool = preGenPool[:0]
			discardedPool = discardedPool[:0]
			captchaPreGenMu.Unlock()
			captchaAnswersMu.Lock()
			captchaAnswers = map[string]*captchaEntry{}
			captchaAnswersMu.Unlock()
			log.Printf("[CAPTCHA] 零点重置，重新生成验证码池")
			fillCaptchaPool(true) // 重置后无条件重新播种
		}
	}()
}
