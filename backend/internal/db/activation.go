package db

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"
)

type ActivationCode struct {
	ID        int64  `json:"id"`
	Code      string `json:"code"`
	Amount    int    `json:"amount"`
	Question  string `json:"question,omitempty"`
	CreatedBy *int64 `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

func randCode() string {
	chars := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 12)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return string(b[:4]) + "-" + string(b[4:8]) + "-" + string(b[8:12])
}

// hashAnswer stores question-code answers as a SHA-256 hash, never in plaintext.
func hashAnswer(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// MaxActivationCodeAmount 普通激活码/答题码单码积分上限。
const MaxActivationCodeAmount = 2000

func GenerateActivationCodes(amount int, count int, createdBy int64) ([]string, error) {
	if amount > MaxActivationCodeAmount {
		return nil, fmt.Errorf("激活码金额不能超过 %d", MaxActivationCodeAmount)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var codes []string
	for i := 0; i < count; i++ {
		code := randCode()
		_, err := DB.Exec(`INSERT INTO activation_codes (code, amount, created_by, created_at) VALUES (?, ?, ?, ?)`,
			code, amount, createdBy, now)
		if err != nil {
			return nil, fmt.Errorf("生成失败: %w", err)
		}
		codes = append(codes, code)
	}
	return codes, nil
}

// GetActivationCodeInfo returns redemption metadata for a code. found is false
// when the code does not exist or was already consumed (consumed rows are deleted).
func GetActivationCodeInfo(code string) (amount int, question string, answerHash string, found bool) {
	err := DB.QueryRow(`SELECT amount, question, answer_hash FROM activation_codes WHERE code = ?`, code).
		Scan(&amount, &question, &answerHash)
	if err != nil {
		return 0, "", "", false
	}
	return amount, question, answerHash, true
}

// VerifyAnswer reports whether the plaintext answer matches a stored answer hash.
func VerifyAnswer(answerHash, answer string) bool {
	return hashAnswer(answer) == answerHash
}

func RedeemActivationCode(code string, userID int64) (int, error) {
	var amount int
	// Use DELETE-then-check to prevent race conditions (rapid clicks)
	// DELETE returns the amount of the deleted row via a RETURNING clause
	err := DB.QueryRow(`DELETE FROM activation_codes WHERE code = ? RETURNING amount`, code).Scan(&amount)
	if err != nil {
		return 0, fmt.Errorf("激活码无效或已被使用")
	}
	_, err = AddNuts(userID, amount, "redeem", nil, nil)
	if err != nil {
		return 0, err
	}
	return amount, nil
}

func ListActivationCodes() ([]ActivationCode, error) {
	rows, err := DB.Query(`SELECT id, code, amount, created_by, created_at, question FROM activation_codes ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var codes []ActivationCode
	for rows.Next() {
		var c ActivationCode
		if err := rows.Scan(&c.ID, &c.Code, &c.Amount, &c.CreatedBy, &c.CreatedAt, &c.Question); err != nil {
			return nil, err
		}
		codes = append(codes, c)
	}
	return codes, nil
}

// GetUserCreatedCodes returns the activation codes a user created via
// points-to-code conversion that have not yet been redeemed. Redeemed codes
// are hard-deleted from the table, so any remaining row is still valid.
func GetUserCreatedCodes(userID int64) ([]ActivationCode, error) {
	rows, err := DB.Query(
		`SELECT id, code, amount, created_by, created_at, question FROM activation_codes
		 WHERE created_by = ? AND source = 'user_to_code' ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var codes []ActivationCode
	for rows.Next() {
		var c ActivationCode
		if err := rows.Scan(&c.ID, &c.Code, &c.Amount, &c.CreatedBy, &c.CreatedAt, &c.Question); err != nil {
			return nil, err
		}
		codes = append(codes, c)
	}
	if codes == nil {
		codes = []ActivationCode{}
	}
	return codes, nil
}
