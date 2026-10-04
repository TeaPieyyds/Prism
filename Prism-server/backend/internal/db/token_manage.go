package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// APIToken represents a user-created secondary API token.
// The primary token lives in users.token and is NOT stored here.
// The auto-increment id acts as the "几号令牌" (token number).
type APIToken struct {
	ID              int64   `json:"id"`
	UserID          int64   `json:"user_id"`
	Name            string  `json:"name"`
	Token           string  `json:"token,omitempty"`
	MaxCalls        int     `json:"max_calls"`     // -1 = unlimited
	MaxNuts         int     `json:"max_nuts"`      // -1 = unlimited
	CallCount       int     `json:"call_count"`    // accumulated calls
	NutsConsumed    int     `json:"nuts_consumed"` // accumulated nuts consumed
	Disabled        bool    `json:"disabled"`
	ActiveAccountID *int64  `json:"active_account_id,omitempty"`
	ActiveAccountName string `json:"active_account_name,omitempty"`
	CreatedAt       string  `json:"created_at"`
	_disabledRaw    int
	_activeRaw      sql.NullInt64
}

// afterScan maps raw DB columns into the public APIToken fields.
func (t *APIToken) afterScan() {
	t.Disabled = t._disabledRaw != 0
	if t._activeRaw.Valid {
		v := t._activeRaw.Int64
		t.ActiveAccountID = &v
	}
}

func generateAPIToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "adb//" + hex.EncodeToString(b)
}

// ResolveToken determines whether a token is the user's primary token or a
// secondary api_tokens row. It returns the user in both cases; apiToken is nil
// when the token is the primary token. Secondary tokens that are disabled fail.
func ResolveToken(token string) (*User, *APIToken, error) {
	u, err := GetUserByToken(token)
	if err != nil {
		return nil, nil, fmt.Errorf("登录凭据无效")
	}
	// Check whether this is a secondary api_tokens row (primary returns ErrNoRows).
	var t APIToken
	err = DB.QueryRow(
		`SELECT id, user_id, name, token, max_calls, max_nuts, call_count, nuts_consumed, disabled, active_account_id, created_at
		 FROM api_tokens WHERE token = ?`, token).
		Scan(&t.ID, &t.UserID, &t.Name, &t.Token, &t.MaxCalls, &t.MaxNuts, &t.CallCount, &t.NutsConsumed, &t._disabledRaw, &t._activeRaw, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return u, nil, nil // primary token
	}
	if err != nil {
		return nil, nil, err
	}
	t.afterScan()
	if t.Disabled {
		return nil, nil, fmt.Errorf("该令牌已被停用")
	}
	return u, &t, nil
}

// CreateAPIToken creates a secondary token for the user and returns its raw value.
// The raw value is shown to the user only once.
func CreateAPIToken(userID int64, name string, maxCalls, maxNuts int) (string, error) {
	token := generateAPIToken()
	if maxCalls < -1 {
		maxCalls = -1
	}
	if maxNuts < -1 {
		maxNuts = -1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var n int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM api_tokens WHERE user_id = ?`, userID).Scan(&n); err == nil && n >= 40 {
		return "", fmt.Errorf("你的子令牌数量已达上限（40 个），无法继续创建")
	}
	_, err := DB.Exec(
		`INSERT INTO api_tokens (user_id, name, token, max_calls, max_nuts, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, name, token, maxCalls, maxNuts, now)
	if err != nil {
		return "", fmt.Errorf("创建令牌失败: %w", err)
	}
	return token, nil
}

// ListAPITokens lists a user's secondary tokens with their token values, so the
// UI can offer a copy button per token.
func ListAPITokens(userID int64) ([]APIToken, error) {
	rows, err := DB.Query(
		`SELECT t.id, t.user_id, t.name, t.token, t.max_calls, t.max_nuts, t.call_count, t.nuts_consumed, t.disabled, t.active_account_id, COALESCE(g.display_name,''), t.created_at
		 FROM api_tokens t
		 LEFT JOIN game_accounts g ON g.id = t.active_account_id
		 WHERE t.user_id = ? ORDER BY t.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []APIToken
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Token, &t.MaxCalls, &t.MaxNuts, &t.CallCount, &t.NutsConsumed, &t._disabledRaw, &t._activeRaw, &t.ActiveAccountName, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.afterScan()
		list = append(list, t)
	}
	if list == nil {
		list = []APIToken{}
	}
	return list, nil
}

// GetAPITokenByID loads a secondary token belonging to userID (ownership check).
func GetAPITokenByID(id, userID int64) (*APIToken, error) {
	var t APIToken
	err := DB.QueryRow(
		`SELECT id, user_id, name, token, max_calls, max_nuts, call_count, nuts_consumed, disabled, active_account_id, created_at
		 FROM api_tokens WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&t.ID, &t.UserID, &t.Name, &t.Token, &t.MaxCalls, &t.MaxNuts, &t.CallCount, &t.NutsConsumed, &t._disabledRaw, &t._activeRaw, &t.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("令牌不存在")
		}
		return nil, err
	}
	t.afterScan()
	return &t, nil
}

// UpdateAPIToken updates a secondary token's name/limits/disabled state.
func UpdateAPIToken(id, userID int64, name string, maxCalls, maxNuts int, disabled bool) error {
	if maxCalls < -1 {
		maxCalls = -1
	}
	if maxNuts < -1 {
		maxNuts = -1
	}
	d := 0
	if disabled {
		d = 1
	}
	res, err := DB.Exec(
		`UPDATE api_tokens SET name = ?, max_calls = ?, max_nuts = ?, disabled = ? WHERE id = ? AND user_id = ?`,
		name, maxCalls, maxNuts, d, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("令牌不存在")
	}
	return nil
}

// SetTokenActiveAccount binds a game account to a secondary token.
// Passing a nil accountID clears the binding.
func SetTokenActiveAccount(tokenID int64, accountID *int64) error {
	_, err := DB.Exec(`UPDATE api_tokens SET active_account_id = ? WHERE id = ?`, accountID, tokenID)
	return err
}

// ResolveActiveAccountID returns the active account for a request context.
// A secondary token's own binding takes precedence; otherwise it falls back to
// the user's default active account (which is the primary token's binding).
func ResolveActiveAccountID(u *User, tok *APIToken) *int64 {
	if tok != nil && tok.ActiveAccountID != nil {
		return tok.ActiveAccountID
	}
	return u.ActiveAccountID
}

// ResetAPIToken regenerates a secondary token's value and returns the new raw value.
func ResetAPIToken(id, userID int64) (string, error) {
	token := generateAPIToken()
	res, err := DB.Exec(`UPDATE api_tokens SET token = ? WHERE id = ? AND user_id = ?`, token, id, userID)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", fmt.Errorf("令牌不存在")
	}
	return token, nil
}

// DeleteAPIToken removes a secondary token owned by userID.
func DeleteAPIToken(id, userID int64) error {
	res, err := DB.Exec(`DELETE FROM api_tokens WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("令牌不存在")
	}
	return nil
}

// EnforceCallLimit returns an error when a limited token has already used up its
// call quota. max_calls == -1 means unlimited.
func EnforceCallLimit(t *APIToken) error {
	if t.MaxCalls != -1 && t.CallCount >= t.MaxCalls {
		return fmt.Errorf("该令牌调用次数已达上限(%d次)", t.MaxCalls)
	}
	return nil
}

// IncTokenCall atomically increments a token's call count, enforcing the call
// cap. It returns an error when the token has reached its max_calls limit.
func IncTokenCall(id int64) error {
	res, err := DB.Exec(
		`UPDATE api_tokens SET call_count = call_count + 1
		 WHERE id = ? AND (max_calls = -1 OR call_count < max_calls)`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("该令牌调用次数已达上限")
	}
	return nil
}

// IncTokenNuts adds nuts consumption to a token and returns an error when the
// cumulative consumption cap (max_nuts != -1) would be exceeded.
func IncTokenNuts(id int64, amount int) error {
	if amount <= 0 {
		return nil
	}
	var maxNuts, consumed int
	err := DB.QueryRow(`SELECT max_nuts, nuts_consumed FROM api_tokens WHERE id = ?`, id).Scan(&maxNuts, &consumed)
	if err != nil {
		return err
	}
	if maxNuts != -1 && consumed+amount > maxNuts {
		return fmt.Errorf("该令牌累计消耗积分已达上限(%d个)", maxNuts)
	}
	_, err = DB.Exec(`UPDATE api_tokens SET nuts_consumed = nuts_consumed + ? WHERE id = ?`, amount, id)
	return err
}

// ConvertNutsToCode converts the user's own nuts balance into an activation code.
// The generated code is worth floor(amount * 0.9) nuts; the remainder is forfeited.
func ConvertNutsToCode(userID int64, amount int) (string, int, error) {
	if amount <= 0 {
		return "", 0, fmt.Errorf("请输入有效积分数量")
	}
	if amount > MaxActivationCodeAmount {
		return "", 0, fmt.Errorf("兑换码金额不能超过 %d", MaxActivationCodeAmount)
	}
	codeValue := amount * 9 / 10
	if codeValue < 1 {
		return "", 0, fmt.Errorf("积分过少，至少需要 10 积分才能生成兑换码")
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
		userID, -amount, "to_code", balance, now)
	if err != nil {
		return "", 0, fmt.Errorf("记录交易失败: %w", err)
	}

	code := randCode()
	_, err = tx.Exec(`INSERT INTO activation_codes (code, amount, created_by, created_at, source) VALUES (?, ?, ?, ?, 'user_to_code')`,
		code, codeValue, userID, now)
	if err != nil {
		return "", 0, fmt.Errorf("生成兑换码失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", 0, fmt.Errorf("事务提交失败: %w", err)
	}
	return code, codeValue, nil
}

// ConvertNutsToQuestionCode converts own nuts balance into a single-use
// question-gated activation code. Redeemers must answer the creator's question
// correctly; the answer is stored as a SHA-256 hash, never in plaintext.
func ConvertNutsToQuestionCode(userID int64, amount int, question, answer string) (string, int, error) {
	if amount <= 0 {
		return "", 0, fmt.Errorf("请输入有效积分数量")
	}
	if amount > MaxActivationCodeAmount {
		return "", 0, fmt.Errorf("答题码金额不能超过 %d", MaxActivationCodeAmount)
	}
	codeValue := amount * 9 / 10
	if codeValue < 1 {
		return "", 0, fmt.Errorf("积分过少，至少需要 10 积分才能生成兑换码")
	}
	if question == "" {
		return "", 0, fmt.Errorf("问题不能为空")
	}
	if answer == "" {
		return "", 0, fmt.Errorf("答案不能为空")
	}
	if len(question) > 500 {
		return "", 0, fmt.Errorf("问题过长（最多 500 字）")
	}
	if len(answer) > 200 {
		return "", 0, fmt.Errorf("答案过长（最多 200 字）")
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
		userID, -amount, "to_question_code", balance, now)
	if err != nil {
		return "", 0, fmt.Errorf("记录交易失败: %w", err)
	}

	code := randCode()
	_, err = tx.Exec(`INSERT INTO activation_codes (code, amount, created_by, created_at, source, question, answer_hash) VALUES (?, ?, ?, ?, 'user_to_code', ?, ?)`,
		code, codeValue, userID, now, question, hashAnswer(answer))
	if err != nil {
		return "", 0, fmt.Errorf("生成兑换码失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", 0, fmt.Errorf("事务提交失败: %w", err)
	}
	return code, codeValue, nil
}
// the total number of API calls recorded for the user across all tokens.
func GetTokenStats(userID int64) ([]APIToken, int, error) {
	tokens, err := ListAPITokens(userID)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM api_call_logs WHERE user_id = ?`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	return tokens, total, nil
}
