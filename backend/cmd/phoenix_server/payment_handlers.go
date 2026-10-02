package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/adb-lanlu/prism-oss/internal/auth"
	"github.com/adb-lanlu/prism-oss/internal/db"
)

// GET /api/payment/products — returns payment config (exchange rate, min amount, etc.)
func handlePaymentProducts(w http.ResponseWriter, r *http.Request) {
	cfg := auth.Cfg
	jsonResp(w, M{
		"ok":          true,
		"min_amount":  cfg.PayMinAmount,
		"nuts_rate":   cfg.PayNutsRate,
		"pay_methods": []string{"alipay", "wxpay"},
	})
}

// POST /api/payment/order — create a payment order
func handleCreatePaymentOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	if !payOrderLimiter.Allow(requestIP(r)) {
		jsonResp(w, M{"ok": false, "error": msg.Get("too_frequent", "手速太快啦,请稍等片刻再试~")})
		return
	}

	cfg := auth.Cfg
	if cfg.PayPID == "" || cfg.PayKey == "" || cfg.PayAPIBase == "" {
		jsonResp(w, M{"ok": false, "error": "抱歉,支付功能还没开通,请联系管理员~"})
		return
	}

	var req struct {
		Amount  float64 `json:"amount"`
		PayType string  `json:"pay_type"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.Amount < cfg.PayMinAmount {
		jsonResp(w, M{"ok": false, "error": fmt.Sprintf("最低支付金额为 %.0f 元", cfg.PayMinAmount)})
		return
	}
	if req.PayType != "alipay" && req.PayType != "wxpay" {
		req.PayType = "alipay"
	}

	nutsAmount := int(req.Amount * float64(cfg.PayNutsRate))
	productName := fmt.Sprintf("Prism板栗 %d颗", nutsAmount)

	notifyURL := cfg.PayNotifyURL
	if notifyURL == "" {
		notifyURL = cfg.BaseURL + "/api/payment/notify"
	}

	order, payResp, submitURL, err := db.CreatePaymentOrder(cfg.PayAPIBase, cfg.PayPID, cfg.PayKey, notifyURL,
		user.ID, req.Amount, req.PayType, productName)
	if err != nil {
		log.Printf("[PAYMENT] 用户 %s 创建订单失败: %v", user.Username, err)
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}

	log.Printf("[PAYMENT] 用户 %s 创建订单 %s (金额=%.2f 板栗=%d)", user.Username, order.OrderNo, req.Amount, nutsAmount)
	db.AddAuditLog(&user.ID, nil, "create_payment_order", "payment", fmt.Sprintf("订单 %s: %.2f元 %d板栗", order.OrderNo, req.Amount, nutsAmount), requestIP(r))

	jsonResp(w, M{
		"ok":         true,
		"order":      order,
		"payurl":     payResp.PayURL,
		"qrcode":     payResp.QRCode,
		"urlscheme":  payResp.URLScheme,
		"submit_url": submitURL,
	})
}

// GET /api/payment/orders — user's payment order history
func handleUserPaymentOrders(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	orders, err := db.GetUserPaymentOrders(user.ID)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true, "orders": orders})
}

// POST /api/payment/notify — async payment callback from 码支付 (no auth, signature-verified)
func handlePaymentNotify(w http.ResponseWriter, r *http.Request) {
	cfg := auth.Cfg

	// Parse all form/query params
	params := make(map[string]string)
	if r.Method == "POST" {
		r.ParseForm()
		for k, v := range r.Form {
			params[k] = v[0]
		}
	} else {
		for k, v := range r.URL.Query() {
			params[k] = v[0]
		}
	}

	log.Printf("[PAY-NOTIFY] 收到回调: orderNo=%s tradeNo=%s status=%s", params["out_trade_no"], params["trade_no"], params["trade_status"])

	_, err := db.HandlePayNotify(params, cfg.PayPID, cfg.PayKey)
	if err != nil {
		log.Printf("[PAY-NOTIFY] 处理失败: %v", err)
		w.Write([]byte("fail"))
		return
	}
	w.Write([]byte("success"))
}

// GET /api/payment/order/status — check a single order status by order_no
func handlePaymentOrderStatus(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		jsonResp(w, M{"ok": false, "error": msg.Get("unauthorized", "哎呀,你还没登录,请先登录后再操作~")})
		return
	}
	orderNo := r.URL.Query().Get("order_no")
	if orderNo == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 order_no"})
		return
	}
	orders, _ := db.GetUserPaymentOrders(user.ID)
	for _, o := range orders {
		if o.OrderNo == orderNo {
			jsonResp(w, M{"ok": true, "order": o})
			return
		}
	}
	jsonResp(w, M{"ok": false, "error": "订单不存在"})
}

// GET /api/admin/payment/orders — admin lists all payment orders
func handleAdminPaymentOrders(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	orders, err := db.GetAllPaymentOrders(limit, offset)
	if err != nil {
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}
	jsonResp(w, M{"ok": true, "orders": orders})
}

// POST /api/admin/payment/retry — admin manually retries/reconciles an order
func handleAdminPaymentRetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResp(w, M{"ok": false, "error": msg.Get("method_not_allowed", "糟糕,请求方式不对,请刷新页面重试~")})
		return
	}
	adminUser := auth.GetUser(r.Context())
	cfg := auth.Cfg

	var req struct {
		OrderNo string `json:"order_no"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.OrderNo == "" {
		jsonResp(w, M{"ok": false, "error": "缺少 order_no"})
		return
	}

	order, err := db.QueryAndRetryOrder(cfg.PayAPIBase, cfg.PayPID, cfg.PayKey, req.OrderNo)
	if err != nil {
		log.Printf("[PAY-ADMIN] %s 补单失败 orderNo=%s: %v", adminUser.Username, req.OrderNo, err)
		jsonResp(w, M{"ok": false, "error": sysErr(err)})
		return
	}

	log.Printf("[PAY-ADMIN] %s 补单查询 orderNo=%s status=%s", adminUser.Username, req.OrderNo, order.Status)
	db.AddAuditLog(&adminUser.ID, nil, "admin_retry_payment", "payment",
		fmt.Sprintf("补单查询 %s → %s", req.OrderNo, order.Status), requestIP(r))

	jsonResp(w, M{"ok": true, "order": order})
}
