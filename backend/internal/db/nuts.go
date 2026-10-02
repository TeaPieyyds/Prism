package db

import (
	"fmt"
	"time"
)

type NutsTransaction struct {
	ID           int64  `json:"id"`
	UserID       int64  `json:"user_id"`
	Amount       int    `json:"amount"`
	Reason       string `json:"reason"`
	RefUserID    *int64 `json:"ref_user_id,omitempty"`
	RefAccountID *int64 `json:"ref_account_id,omitempty"`
	BalanceAfter int    `json:"balance_after"`
	CreatedAt    string `json:"created_at"`
}

func GetNutsBalance(userID int64) (int, error) {
	// 订阅有效期内固定返回 1999
	if subActive, _, _ := IsSubscriptionActive(userID); subActive {
		return 1999, nil
	}
	var b int
	err := DB.QueryRow(`SELECT nuts_balance FROM users WHERE id = ?`, userID).Scan(&b)
	return b, err
}

func AddNuts(userID int64, amount int, reason string, refUserID, refAccountID *int64) (int, error) {
	// 订阅有效期内积分增减无效：不改变余额，固定显示 1999
	if subActive, _, _ := IsSubscriptionActive(userID); subActive {
		return 1999, nil
	}
	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var balance int
	err = tx.QueryRow(`UPDATE users SET nuts_balance = nuts_balance + ? WHERE id = ? RETURNING nuts_balance`, amount, userID).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("更新余额失败: %w", err)
	}
	if balance < 0 {
		return 0, fmt.Errorf("板栗不足")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.Exec(`INSERT INTO nuts_transactions (user_id, amount, reason, ref_user_id, ref_account_id, balance_after, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, amount, reason, refUserID, refAccountID, balance, now)
	if err != nil {
		return 0, fmt.Errorf("记录交易失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return balance, nil
}

func GetNutsTransactions(userID int64, limit int) ([]NutsTransaction, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := DB.Query(`SELECT id, user_id, amount, reason, ref_user_id, ref_account_id, balance_after, created_at FROM nuts_transactions WHERE user_id = ? ORDER BY id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNutsTransactions(rows)
}

func GetAllNutsTransactions(limit, offset int) ([]NutsTransaction, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := DB.Query(`SELECT id, user_id, amount, reason, ref_user_id, ref_account_id, balance_after, created_at FROM nuts_transactions ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNutsTransactions(rows)
}

func scanNutsTransactions(rows interface{ Next() bool; Scan(...any) error }) ([]NutsTransaction, error) {
	var list []NutsTransaction
	for rows.Next() {
		var t NutsTransaction
		var refUID, refAID *int64
		if err := rows.Scan(&t.ID, &t.UserID, &t.Amount, &t.Reason, &refUID, &refAID, &t.BalanceAfter, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.RefUserID = refUID
		t.RefAccountID = refAID
		list = append(list, t)
	}
	return list, nil
}
