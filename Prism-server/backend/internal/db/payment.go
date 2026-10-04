package db

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type PaymentOrder struct {
	ID         int64   `json:"id"`
	OrderNo    string  `json:"order_no"`
	TradeNo    string  `json:"trade_no"`
	UserID     int64   `json:"user_id"`
	AmountYuan float64 `json:"amount_yuan"`
	NutsAmount int     `json:"nuts_amount"`
	PayType    string  `json:"pay_type"`
	Status     string  `json:"status"`
	Codes      string  `json:"codes"`
	CreatedAt  string  `json:"created_at"`
	PaidAt     string  `json:"paid_at"`
}

type PayAPIResponse struct {
	Code      int    `json:"code"`
	Msg       string `json:"msg"`
	TradeNo   string `json:"trade_no"`
	PayURL    string `json:"payurl"`
	QRCode    string `json:"qrcode"`
	URLScheme string `json:"urlscheme"`
	Money     string `json:"money"`
}

var (
	payNotifyMu    sync.Mutex
	nutsRateOverride int
)

func generateOrderNo() string {
	now := time.Now()
	return fmt.Sprintf("%s%04d", now.Format("20060102150405"), rand.Intn(10000))
}

// PaySign computes MD5 signature for 码支付 API.
func PaySign(params map[string]string, key string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "sign" || k == "sign_type" || params[k] == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	raw := strings.Join(parts, "&") + key
	hash := md5.Sum([]byte(raw))
	return hex.EncodeToString(hash[:])
}

func VerifyPaySign(params map[string]string, key string) bool {
	if params["sign"] == "" {
		return false
	}
	return PaySign(params, key) == params["sign"]
}

func CreatePaymentOrder(apiBase, pid, key, notifyURL string, userID int64, amountYuan float64, payType, productName string) (*PaymentOrder, *PayAPIResponse, string, error) {
	orderNo := generateOrderNo()
	nutsAmount := int(amountYuan * float64(authNutsRate()))

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := DB.Exec(`INSERT INTO payment_orders (order_no, user_id, amount_yuan, nuts_amount, pay_type, status, created_at) VALUES (?, ?, ?, ?, ?, 'pending', ?)`,
		orderNo, userID, amountYuan, nutsAmount, payType, now)
	if err != nil {
		return nil, nil, "", fmt.Errorf("创建订单失败: %w", err)
	}

	moneyStr := fmt.Sprintf("%.2f", amountYuan)
	params := map[string]string{
		"pid":          pid,
		"type":         payType,
		"out_trade_no": orderNo,
		"notify_url":   notifyURL,
		"name":         productName,
		"money":        moneyStr,
	}
	params["sign"] = PaySign(params, key)
	params["sign_type"] = "MD5"

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	apiURL := apiBase + "/xpay/epay/mapi.php"
	resp, err := http.PostForm(apiURL, form)
	if err != nil {
		return nil, nil, "", fmt.Errorf("支付网关请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var payResp PayAPIResponse
	if err := json.Unmarshal(body, &payResp); err != nil {
		return nil, nil, "", fmt.Errorf("支付网关响应解析失败: %w", err)
	}
	if payResp.Code != 1 {
		return nil, nil, "", fmt.Errorf("支付网关错误: %s", payResp.Msg)
	}

	// Build submit URL as fallback when mapi returns no payurl/qrcode
	submitURL := ""
	if payResp.PayURL == "" && payResp.QRCode == "" && payResp.URLScheme == "" {
		submitParams := url.Values{}
		for k, v := range params {
			submitParams.Set(k, v)
		}
		submitURL = apiBase + "/xpay/epay/submit.php?" + submitParams.Encode()
	}

	var order PaymentOrder
	DB.QueryRow(`SELECT id, order_no, trade_no, user_id, amount_yuan, nuts_amount, pay_type, status, codes, created_at, paid_at FROM payment_orders WHERE order_no = ?`, orderNo).
		Scan(&order.ID, &order.OrderNo, &order.TradeNo, &order.UserID, &order.AmountYuan, &order.NutsAmount, &order.PayType, &order.Status, &order.Codes, &order.CreatedAt, &order.PaidAt)

	return &order, &payResp, submitURL, nil
}

// HandlePayNotify processes async payment callback from 码支付.
// Returns the order number on success. Idempotent: re-processing an already-paid order is safe.
func HandlePayNotify(params map[string]string, expectedPID, key string) (string, error) {
	// 1. Signature verification
	if !VerifyPaySign(params, key) {
		log.Printf("[PAY-NOTIFY] 签名验证失败, params=%v", params)
		return "", fmt.Errorf("签名验证失败")
	}

	// 2. Merchant ID check
	if params["pid"] != expectedPID {
		log.Printf("[PAY-NOTIFY] 商户号不匹配: got=%s want=%s", params["pid"], expectedPID)
		return "", fmt.Errorf("商户号不匹配")
	}

	// 3. Status check
	if params["trade_status"] != "TRADE_SUCCESS" {
		log.Printf("[PAY-NOTIFY] 支付状态非成功: %s", params["trade_status"])
		return "", fmt.Errorf("支付状态非成功: %s", params["trade_status"])
	}

	orderNo := params["out_trade_no"]
	tradeNo := params["trade_no"]
	callbackMoney := params["money"]

	// 4. Serialize notify processing to prevent race conditions
	payNotifyMu.Lock()
	defer payNotifyMu.Unlock()

	// 5. Load order and validate
	var orderID int64
	var userID int64
	var nutsAmount int
	var status string
	var orderAmount float64
	err := DB.QueryRow(`SELECT id, user_id, nuts_amount, status, amount_yuan FROM payment_orders WHERE order_no = ?`, orderNo).
		Scan(&orderID, &userID, &nutsAmount, &status, &orderAmount)
	if err != nil {
		log.Printf("[PAY-NOTIFY] 订单不存在: %s", orderNo)
		return "", fmt.Errorf("订单不存在: %s", orderNo)
	}

	// 6. Idempotency: already processed
	if status == "paid" {
		log.Printf("[PAY-NOTIFY] 订单已处理,跳过: %s", orderNo)
		return orderNo, nil
	}

	// 7. Amount validation
	var cbMoney float64
	fmt.Sscanf(callbackMoney, "%f", &cbMoney)
	if cbMoney > 0 && abs(cbMoney-orderAmount) > 0.02 {
		log.Printf("[PAY-NOTIFY] 金额不匹配: order=%.2f callback=%s, orderNo=%s", orderAmount, callbackMoney, orderNo)
		return "", fmt.Errorf("金额不匹配")
	}

	// 8. Atomic: generate code + update order in a transaction
	tx, err := DB.Begin()
	if err != nil {
		return "", fmt.Errorf("事务启动失败: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	code := randCode()
	_, err = tx.Exec(`INSERT INTO activation_codes (code, amount, created_by, created_at) VALUES (?, ?, ?, ?)`,
		code, nutsAmount, userID, now)
	if err != nil {
		log.Printf("[PAY-NOTIFY] 生成激活码失败: %v", err)
		return "", fmt.Errorf("生成激活码失败: %w", err)
	}

	codesJSON, _ := json.Marshal([]string{code})
	_, err = tx.Exec(`UPDATE payment_orders SET trade_no = ?, status = 'paid', codes = ?, paid_at = ? WHERE id = ? AND status = 'pending'`,
		tradeNo, string(codesJSON), now, orderID)
	if err != nil {
		log.Printf("[PAY-NOTIFY] 更新订单失败: %v", err)
		return "", fmt.Errorf("更新订单失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[PAY-NOTIFY] 事务提交失败: %v", err)
		return "", fmt.Errorf("事务提交失败: %w", err)
	}

	log.Printf("[PAY-NOTIFY] 支付成功 orderNo=%s tradeNo=%s userID=%d amount=%.2f nuts=%d code=%s",
		orderNo, tradeNo, userID, orderAmount, nutsAmount, code)

	return orderNo, nil
}

func GetUserPaymentOrders(userID int64) ([]PaymentOrder, error) {
	rows, err := DB.Query(`SELECT id, order_no, trade_no, user_id, amount_yuan, nuts_amount, pay_type, status, codes, created_at, paid_at FROM payment_orders WHERE user_id = ? ORDER BY id DESC LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var orders []PaymentOrder
	for rows.Next() {
		var o PaymentOrder
		if err := rows.Scan(&o.ID, &o.OrderNo, &o.TradeNo, &o.UserID, &o.AmountYuan, &o.NutsAmount, &o.PayType, &o.Status, &o.Codes, &o.CreatedAt, &o.PaidAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, nil
}

func GetAllPaymentOrders(limit, offset int) ([]PaymentOrder, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := DB.Query(`SELECT id, order_no, trade_no, user_id, amount_yuan, nuts_amount, pay_type, status, codes, created_at, paid_at FROM payment_orders ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var orders []PaymentOrder
	for rows.Next() {
		var o PaymentOrder
		if err := rows.Scan(&o.ID, &o.OrderNo, &o.TradeNo, &o.UserID, &o.AmountYuan, &o.NutsAmount, &o.PayType, &o.Status, &o.Codes, &o.CreatedAt, &o.PaidAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, nil
}

// QueryAndRetryOrder queries the payment platform for an order and auto-completes it if paid.
func QueryAndRetryOrder(apiBase, pid, key, orderNo string) (*PaymentOrder, error) {
	apiURL := fmt.Sprintf("%s/xpay/epay/api.php?act=order&pid=%s&key=%s&out_trade_no=%s", apiBase, pid, key, orderNo)
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("查询订单失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		Status  int    `json:"status"`
		TradeNo string `json:"trade_no"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if result.Code != 1 {
		return nil, fmt.Errorf("查询失败: %s", result.Msg)
	}

	var order PaymentOrder
	err = DB.QueryRow(`SELECT id, order_no, trade_no, user_id, amount_yuan, nuts_amount, pay_type, status, codes, created_at, paid_at FROM payment_orders WHERE order_no = ?`, orderNo).
		Scan(&order.ID, &order.OrderNo, &order.TradeNo, &order.UserID, &order.AmountYuan, &order.NutsAmount, &order.PayType, &order.Status, &order.Codes, &order.CreatedAt, &order.PaidAt)
	if err != nil {
		return nil, fmt.Errorf("本地订单不存在: %s", orderNo)
	}

	if order.Status == "paid" {
		return &order, nil
	}

	// Platform says paid but local is pending — reconcile
	if result.Status == 1 {
		payNotifyMu.Lock()
		defer payNotifyMu.Unlock()

		// Double-check after acquiring lock
		var currentStatus string
		DB.QueryRow(`SELECT status FROM payment_orders WHERE order_no = ?`, orderNo).Scan(&currentStatus)
		if currentStatus == "paid" {
			order.Status = "paid"
			return &order, nil
		}

		tx, err := DB.Begin()
		if err != nil {
			return nil, fmt.Errorf("事务启动失败: %w", err)
		}
		defer tx.Rollback()

		now := time.Now().UTC().Format(time.RFC3339)
		code := randCode()
		_, err = tx.Exec(`INSERT INTO activation_codes (code, amount, created_by, created_at) VALUES (?, ?, ?, ?)`,
			code, order.NutsAmount, order.UserID, now)
		if err != nil {
			return nil, fmt.Errorf("生成激活码失败: %w", err)
		}
		codesJSON, _ := json.Marshal([]string{code})
		_, err = tx.Exec(`UPDATE payment_orders SET trade_no = ?, status = 'paid', codes = ?, paid_at = ? WHERE id = ? AND status = 'pending'`,
			result.TradeNo, string(codesJSON), now, order.ID)
		if err != nil {
			return nil, fmt.Errorf("更新订单失败: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("事务提交失败: %w", err)
		}

		log.Printf("[PAY-RETRY] 补单成功 orderNo=%s tradeNo=%s userID=%d amount=%.2f nuts=%d code=%s",
			orderNo, result.TradeNo, order.UserID, order.AmountYuan, order.NutsAmount, code)

		order.Status = "paid"
		order.Codes = string(codesJSON)
		order.TradeNo = result.TradeNo
		order.PaidAt = now
	}

	return &order, nil
}

func SetNutsRate(rate int) {
	nutsRateOverride = rate
}

func authNutsRate() int {
	if nutsRateOverride > 0 {
		return nutsRateOverride
	}
	return 10
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
