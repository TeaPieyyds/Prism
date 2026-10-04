package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"strings"
	"time"
)

type RedPacket struct {
	ID             int64  `json:"id"`
	Code           string `json:"code"`
	Total          int    `json:"total"`
	Remaining      int    `json:"remaining"`
	Count          int    `json:"count"`
	RemainingCount int    `json:"remaining_count"`
	CreatedBy      int64  `json:"created_by"`
	CreatedByType  string `json:"created_by_type"`
	CreatedAt      string `json:"created_at"`
}

// randRedPacketCode returns a code with an "RB-" prefix so the redeem handler
// can route red-packet claims immediately and avoid colliding with the 12-char
// activation code namespace.
func randRedPacketCode() string {
	chars := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 8)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return "RB-" + string(b[:4]) + "-" + string(b[4:8])
}

func randInt64(min, max int64) int64 {
	if max <= min {
		return min
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(max-min+1))
	return min + n.Int64()
}

// CreateRedPacket creates a red packet with the given pool value (no cost to
// the caller). createdByType is "admin" or "user".
func CreateRedPacket(total, count int, createdBy int64, createdByType string) (string, error) {
	if err := validateRedPacket(total, count); err != nil {
		return "", err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	code := randRedPacketCode()
	_, err := DB.Exec(`INSERT INTO red_packets (code, total, remaining, count, remaining_count, created_by, created_by_type, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		code, total, total, count, count, createdBy, createdByType, now)
	if err != nil {
		return "", fmt.Errorf("创建红包失败: %w", err)
	}
	return code, nil
}

// CreateRedPacketFromNuts deducts `amount` from the user's balance and creates
// a red packet whose pool is 9/10 of the spent amount (10% retained by the
// platform), matching the existing points-to-code conversion economics.
func CreateRedPacketFromNuts(userID int64, amount, count int) (string, int, error) {
	pool := amount * 9 / 10
	if err := validateRedPacket(pool, count); err != nil {
		return "", 0, err
	}
	tx, err := DB.Begin()
	if err != nil {
		return "", 0, fmt.Errorf("事务启动失败: %w", err)
	}
	defer tx.Rollback()

	var balance int
	err = tx.QueryRow(`UPDATE users SET nuts_balance = nuts_balance - ? WHERE id = ? RETURNING nuts_balance`, amount, userID).Scan(&balance)
	if err != nil {
		return "", 0, fmt.Errorf("扣除积分失败: %w", err)
	}
	if balance < 0 {
		return "", 0, fmt.Errorf("板栗不足")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.Exec(`INSERT INTO nuts_transactions (user_id, amount, reason, balance_after, created_at) VALUES (?, ?, ?, ?, ?)`,
		userID, -amount, "create_redpacket", balance, now)
	if err != nil {
		return "", 0, fmt.Errorf("记录交易失败: %w", err)
	}

	code := randRedPacketCode()
	_, err = tx.Exec(`INSERT INTO red_packets (code, total, remaining, count, remaining_count, created_by, created_by_type, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		code, pool, pool, count, count, userID, "user", now)
	if err != nil {
		return "", 0, fmt.Errorf("创建红包失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", 0, fmt.Errorf("事务提交失败: %w", err)
	}
	return code, pool, nil
}

// MaxRedPacketAmount 单个红包总积分上限。
const MaxRedPacketAmount = 5000

func validateRedPacket(total, count int) error {
	if total > MaxRedPacketAmount {
		return fmt.Errorf("红包总积分不能超过 %d", MaxRedPacketAmount)
	}
	if count < 1 {
		return fmt.Errorf("份数至少为 1")
	}
	if count > 500 {
		return fmt.Errorf("份数最多 500")
	}
	if total < count {
		return fmt.Errorf("总积分需至少等于份数，确保每人至少 1 积分")
	}
	return nil
}

// ClaimRedPacket atomically claims a red packet for a user using a WeChat-style
// decreasing draw: each claim takes a random amount that leaves at least 1 for
// every remaining share and never exceeds the pool. BEGIN IMMEDIATE acquires the
// write lock before reading so concurrent claims cannot overdraw the pool.
func ClaimRedPacket(code string, userID int64) (int, error) {
	ctx := context.Background()
	conn, err := DB.Conn(ctx)
	if err != nil {
		return 0, fmt.Errorf("获取连接失败: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return 0, fmt.Errorf("红包领取繁忙，请稍后再试")
	}
	committed := false
	defer func() {
		if !committed {
			conn.ExecContext(ctx, "ROLLBACK")
		}
	}()

	var pid, remaining, remainingCount int64
	err = conn.QueryRowContext(ctx, `SELECT id, remaining, remaining_count FROM red_packets WHERE code = ?`, code).
		Scan(&pid, &remaining, &remainingCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("红包不存在或已被抢完")
		}
		return 0, fmt.Errorf("查询红包失败: %w", err)
	}
	if remainingCount <= 0 || remaining <= 0 {
		return 0, fmt.Errorf("红包已被抢完")
	}

	var existing int
	err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM red_packet_claims WHERE packet_id = ? AND user_id = ?`, pid, userID).Scan(&existing)
	if err != nil {
		return 0, fmt.Errorf("查询领取记录失败: %w", err)
	}
	if existing > 0 {
		return 0, fmt.Errorf("你已抢过该红包")
	}

	var amt int64
	if remainingCount == 1 {
		amt = remaining
	} else {
		// Reserve at least 1 for each remaining share, and cap so a claim never
		// takes more than ~2x the fair share (decreasing draw).
		upper := remaining - (remainingCount - 1)
		if byAvg := remaining / remainingCount * 2; byAvg < upper {
			upper = byAvg
		}
		if upper < 1 {
			upper = 1
		}
		amt = randInt64(1, upper)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := conn.ExecContext(ctx, `UPDATE red_packets SET remaining = remaining - ?, remaining_count = remaining_count - 1 WHERE id = ?`, amt, pid); err != nil {
		return 0, fmt.Errorf("更新红包失败: %w", err)
	}

	var balance int
	if err := conn.QueryRowContext(ctx, `UPDATE users SET nuts_balance = nuts_balance + ? WHERE id = ? RETURNING nuts_balance`, amt, userID).Scan(&balance); err != nil {
		return 0, fmt.Errorf("更新余额失败: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO red_packet_claims (packet_id, user_id, amount, claimed_at) VALUES (?,?,?,?)`, pid, userID, amt, now); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return 0, fmt.Errorf("你已抢过该红包")
		}
		return 0, fmt.Errorf("记录领取失败: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO nuts_transactions (user_id, amount, reason, balance_after, created_at) VALUES (?,?,?,?,?)`,
		userID, amt, "redpacket_claim", balance, now); err != nil {
		return 0, fmt.Errorf("记录交易失败: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return 0, fmt.Errorf("领取失败，请稍后再试")
	}
	committed = true
	return int(amt), nil
}

// ListRedPackets returns the red packets a user created, most recent first.
func ListRedPackets(createdBy int64) ([]RedPacket, error) {
	rows, err := DB.Query(`SELECT id, code, total, remaining, count, remaining_count, created_by, created_by_type, created_at FROM red_packets WHERE created_by = ? ORDER BY id DESC`, createdBy)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []RedPacket
	for rows.Next() {
		var p RedPacket
		if err := rows.Scan(&p.ID, &p.Code, &p.Total, &p.Remaining, &p.Count, &p.RemainingCount, &p.CreatedBy, &p.CreatedByType, &p.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	if list == nil {
		list = []RedPacket{}
	}
	return list, nil
}
