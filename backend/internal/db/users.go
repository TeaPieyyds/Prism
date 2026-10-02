package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type User struct {
	ID                  int64  `json:"id"`
	Email               string `json:"email"`
	Username            string `json:"username"`
	PasswordHash        string `json:"-"`
	Role                string `json:"role"`
	Verified            bool   `json:"verified"`
	Disabled            bool   `json:"disabled"`
	Token               string `json:"token"`
	ActiveAccountID     *int64 `json:"active_account_id,omitempty"`
	ChallengeOverride   bool   `json:"challenge_override"`
	ChallengeOverrideAt string `json:"challenge_override_at,omitempty"`
	GrowthOverride      bool   `json:"growth_override"`
	GrowthOverrideValue int    `json:"growth_override_value"`
	NutsBalance         int    `json:"nuts_balance"`
	SubscriptionStart   string `json:"subscription_start,omitempty"`
	SubscriptionUntil   string `json:"subscription_until,omitempty"`
	Activated           bool   `json:"activated"`
	LoginCount          int    `json:"-"`
	CreatedAt           string `json:"created_at"`
	TestAccount         bool   `json:"test_account"`
	_activatedRaw       int
	_loginCountRaw      int
}

func generateToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return "adb//" + hex.EncodeToString(b)
}

func CreateUser(email, username, passwordHash string) (*User, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	token := generateToken()
	result, err := DB.Exec(
		`INSERT INTO users (email, username, password_hash, role, verified, token, active_account_id, created_at)
		 VALUES (?, ?, ?, 'user', 0, ?, NULL, ?)`,
		email, username, passwordHash, token, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return &User{
		ID:           id,
		Email:        email,
		Username:     username,
		PasswordHash: passwordHash,
		Role:         "user",
		Token:        token,
		CreatedAt:    now,
	}, nil
}

func CreateVerifiedUser(email, username, passwordHash string) (*User, error) {
	_, _ = DB.Exec(`DELETE FROM users WHERE verified = 0 AND (email = ? OR username = ?)`, email, username)
	now := time.Now().UTC().Format(time.RFC3339)
	token := generateToken()

	// Generate a unique 6-digit invite code
	var inviteCode string
	for i := 0; i < 10; i++ {
		DB.QueryRow(`SELECT CAST(ABS(RANDOM()) % 900000 + 100000 AS TEXT)`).Scan(&inviteCode)
		var exists int
		DB.QueryRow(`SELECT COUNT(*) FROM users WHERE invite_code = ?`, inviteCode).Scan(&exists)
		if exists == 0 {
			break
		}
	}

	result, err := DB.Exec(
		`INSERT INTO users (email, username, password_hash, role, verified, token, active_account_id, invite_code, created_at)
		 VALUES (?, ?, ?, 'user', 1, ?, NULL, ?, ?)`,
		email, username, passwordHash, token, inviteCode, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return &User{
		ID:           id,
		Email:        email,
		Username:     username,
		PasswordHash: passwordHash,
		Role:         "user",
		Verified:     true,
		Token:        token,
		CreatedAt:    now,
	}, nil
}

func GetUserByID(id int64) (*User, error) {
	row := DB.QueryRow(
		`SELECT id, email, username, password_hash, role, verified, disabled, token, active_account_id, challenge_override, challenge_override_at, growth_override, growth_override_value, nuts_balance, subscription_start, subscription_until, activated, login_count, created_at, test_account
		 FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func GetUserByEmail(email string) (*User, error) {
	row := DB.QueryRow(
		`SELECT id, email, username, password_hash, role, verified, disabled, token, active_account_id, challenge_override, challenge_override_at, growth_override, growth_override_value, nuts_balance, subscription_start, subscription_until, activated, login_count, created_at, test_account
		 FROM users WHERE email = ?`, email)
	return scanUser(row)
}

func GetUserByToken(token string) (*User, error) {
	row := DB.QueryRow(
		`SELECT id, email, username, password_hash, role, verified, disabled, token, active_account_id, challenge_override, challenge_override_at, growth_override, growth_override_value, nuts_balance, subscription_start, subscription_until, activated, login_count, created_at, test_account
		 FROM users WHERE token = ?`, token)
	u, err := scanUser(row)
	if err == nil {
		return u, nil
	}
	// Fallback: secondary token stored in api_tokens.
	var uid int64
	if e := DB.QueryRow(`SELECT user_id FROM api_tokens WHERE token = ? AND disabled = 0`, token).Scan(&uid); e == nil && uid != 0 {
		return GetUserByID(uid)
	}
	return u, err
}

func VerifyUser(email string) error {
	_, err := DB.Exec(`UPDATE users SET verified = 1 WHERE email = ?`, email)
	return err
}

func SetActiveAccount(userID, accountID int64) error {
	_, err := DB.Exec(`UPDATE users SET active_account_id = ? WHERE id = ?`, accountID, userID)
	return err
}

func ClearActiveAccount(userID int64) error {
	_, err := DB.Exec(`UPDATE users SET active_account_id = NULL WHERE id = ?`, userID)
	return err
}

func ListAllUsers() ([]*User, error) {
	rows, err := DB.Query(
		`SELECT id, email, username, password_hash, role, verified, disabled, token, active_account_id, challenge_override, challenge_override_at, growth_override, growth_override_value, nuts_balance, subscription_start, subscription_until, activated, login_count, created_at, test_account
		 FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func scanUser(s interface {
	Scan(dest ...any) error
}) (*User, error) {
	var u User
	var activeID sql.NullInt64
	var override bool
	var overrideAt string
	var growthOverride bool
	var growthOverrideValue int
	err := s.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash,
		&u.Role, &u.Verified, &u.Disabled, &u.Token, &activeID,
		&override, &overrideAt, &growthOverride, &growthOverrideValue, &u.NutsBalance, &u.SubscriptionStart, &u.SubscriptionUntil, &u._activatedRaw, &u._loginCountRaw, &u.CreatedAt, &u.TestAccount)
	u.Activated = u._activatedRaw != 0
	u.LoginCount = u._loginCountRaw
	if err != nil {
		return nil, err
	}
	// 订阅有效期内积分固定显示 1999
	if u.SubscriptionUntil != "" {
		if untilT, pe := time.Parse(time.RFC3339, u.SubscriptionUntil); pe == nil && time.Now().UTC().Before(untilT) {
			u.NutsBalance = 1999
		}
	}
	if activeID.Valid {
		u.ActiveAccountID = &activeID.Int64
	}
	u.ChallengeOverride = override
	u.ChallengeOverrideAt = overrideAt
	u.GrowthOverride = growthOverride
	u.GrowthOverrideValue = growthOverrideValue
	return &u, nil
}

// Verify codes
func CreateVerifyCode(email, username, passwordHash, code, inviteCode string) error {
	now := time.Now().UTC()
	expires := now.Add(30 * time.Minute)
	_, err := DB.Exec(
		`INSERT INTO verify_codes (email, username, password_hash, code, expires_at, used, invite_code) VALUES (?, ?, ?, ?, ?, 0, ?)`,
		email, username, passwordHash, code, expires.Format(time.RFC3339), inviteCode)
	return err
}

type PendingRegistration struct {
	Email        string
	Username     string
	PasswordHash string
	InviteCode   string
}

func CheckVerifyCode(email, code string) (*PendingRegistration, bool, error) {
	row := DB.QueryRow(
		`SELECT id, username, password_hash, COALESCE(invite_code,'') FROM verify_codes WHERE email = ? AND code = ? AND purpose = 'register' AND expires_at > ? AND used = 0`,
		email, code, time.Now().UTC().Format(time.RFC3339))
	var id int64
	var pending PendingRegistration
	pending.Email = email
	if err := row.Scan(&id, &pending.Username, &pending.PasswordHash, &pending.InviteCode); err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	DB.Exec(`UPDATE verify_codes SET used = 1 WHERE id = ?`, id)
	return &pending, true, nil
}

// PeekVerifyCode validates a verify code without consuming it.
// Used by the email-confirmation preview (GET) so an email security scanner
// that auto-fetches the link cannot burn the code or create an account.
func PeekVerifyCode(email, code string) (*PendingRegistration, bool, error) {
	row := DB.QueryRow(
		`SELECT id, username, password_hash, COALESCE(invite_code,'') FROM verify_codes WHERE email = ? AND code = ? AND purpose = 'register' AND expires_at > ? AND used = 0`,
		email, code, time.Now().UTC().Format(time.RFC3339))
	var id int64
	var pending PendingRegistration
	pending.Email = email
	if err := row.Scan(&id, &pending.Username, &pending.PasswordHash, &pending.InviteCode); err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &pending, true, nil
}

func IsAdminExists() (bool, error) {
	row := DB.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin' AND verified = 1`)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// IsEmailRegistered checks if an email belongs to an activated account.
func IsEmailRegistered(email string) (bool, error) {
	row := DB.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ? AND verified = 1`, email)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func IsUsernameTaken(username string) (bool, error) {
	row := DB.QueryRow(`SELECT COUNT(*) FROM users WHERE username = ? AND verified = 1`, username)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func IsUsernameTakenByOther(username string, userID int64) (bool, error) {
	row := DB.QueryRow(`SELECT COUNT(*) FROM users WHERE username = ? AND id != ? AND verified = 1`, username, userID)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func UpdateUsername(userID int64, username string) error {
	_, err := DB.Exec(`UPDATE users SET username = ? WHERE id = ?`, username, userID)
	return err
}

func UpdatePassword(userID int64, passwordHash string) error {
	_, err := DB.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID)
	return err
}

func ResetToken(userID int64) (string, error) {
	token := generateToken()
	_, err := DB.Exec(`UPDATE users SET token = ? WHERE id = ?`, token, userID)
	if err != nil {
		return "", err
	}
	return token, nil
}

func SetUserDisabled(userID int64, disabled bool) error {
	v := 0
	if disabled {
		v = 1
	}
	_, err := DB.Exec(`UPDATE users SET disabled = ? WHERE id = ?`, v, userID)
	return err
}

func SetUserTestAccount(userID int64, testAccount bool) error {
	v := 0
	if testAccount {
		v = 1
	}
	_, err := DB.Exec(`UPDATE users SET test_account = ? WHERE id = ?`, v, userID)
	return err
}

func SetChallengeOverride(userID int64, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	now := ""
	if enabled {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := DB.Exec(`UPDATE users SET challenge_override = ?, challenge_override_at = ? WHERE id = ?`, v, now, userID)
	return err
}

func SetGrowthOverride(userID int64, enabled bool, value int) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := DB.Exec(`UPDATE users SET growth_override = ?, growth_override_value = ? WHERE id = ?`, v, value, userID)
	return err
}

func SetUserRole(userID int64, role string) error {
	_, err := DB.Exec(`UPDATE users SET role = ? WHERE id = ?`, role, userID)
	return err
}

func DeleteUser(userID int64) error {
	_, err := DB.Exec(`DELETE FROM users WHERE id = ?`, userID)
	return err
}

func IncrementLoginCount(userID int64) error {
	_, err := DB.Exec(`UPDATE users SET login_count = login_count + 1 WHERE id = ?`, userID)
	return err
}

func ActivateUser(userID int64) error {
	_, err := DB.Exec(`UPDATE users SET activated = 1 WHERE id = ?`, userID)
	return err
}

func ValidateActivationKey(code string) bool {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM activation_keys WHERE code = ?`, code).Scan(&count)
	return err == nil && count > 0
}

func CreateAdmin(email, passwordHash string) (*User, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	token := generateToken()
	result, err := DB.Exec(
		`INSERT INTO users (email, username, password_hash, role, verified, token, active_account_id, created_at)
		 VALUES (?, ?, ?, 'admin', 1, ?, NULL, ?)`,
		email, email, passwordHash, token, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create admin: %w", err)
	}
	id, _ := result.LastInsertId()
	return &User{
		ID:        id,
		Email:     email,
		Username:  email,
		Role:      "admin",
		Verified:  true,
		Token:     token,
		CreatedAt: now,
	}, nil
}

func UpdateEmail(userID int64, newEmail string) error {
	_, err := DB.Exec(`UPDATE users SET email = ? WHERE id = ?`, newEmail, userID)
	return err
}

func UpdatePasswordByEmail(email, passwordHash string) error {
	_, err := DB.Exec(`UPDATE users SET password_hash = ? WHERE email = ?`, passwordHash, email)
	return err
}

func CountRecentVerifyCodes(email, purpose string, since time.Duration) int {
	// verify_codes has no created_at, use expires_at instead.
	// Codes expire after 30min, so recent codes have expires_at > now - since + 30min
	cutoff := time.Now().UTC().Add(-since + 30*time.Minute).Format(time.RFC3339)
	var count int
	DB.QueryRow(`SELECT COUNT(*) FROM verify_codes WHERE email = ? AND purpose = ? AND expires_at > ?`, email, purpose, cutoff).Scan(&count)
	return count
}

func CreateGenericVerifyCode(email, code, purpose string) error {
	now := time.Now().UTC()
	expires := now.Add(30 * time.Minute)
	_, err := DB.Exec(
		`INSERT INTO verify_codes (email, code, expires_at, used, purpose) VALUES (?, ?, ?, 0, ?)`,
		email, code, expires.Format(time.RFC3339), purpose)
	return err
}

func ConsumeVerifyCode(email, code, purpose string) (bool, error) {
	row := DB.QueryRow(
		`SELECT id FROM verify_codes WHERE email = ? AND code = ? AND purpose = ? AND expires_at > ? AND used = 0`,
		email, code, purpose, time.Now().UTC().Format(time.RFC3339))
	var id int64
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	DB.Exec(`UPDATE verify_codes SET used = 1 WHERE id = ?`, id)
	return true, nil
}

// ── Session cookie support ──

var sessionStore = make(map[string]int64) // sid -> userID
var sessionMu sync.RWMutex

func SetSession(sid string, userID int64) {
	sessionMu.Lock()
	sessionStore[sid] = userID
	sessionMu.Unlock()
}

func GetUserBySessionCookie(sid string) (*User, error) {
	sessionMu.RLock()
	userID, ok := sessionStore[sid]
	sessionMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	return GetUserByID(userID)
}

// RevokeUserSessions removes every web session cookie for a user, forcing
// re-login on all devices. Called after password changes and token resets so
// a hijacked session cannot survive a credential rotation.
func RevokeUserSessions(userID int64) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	for sid, uid := range sessionStore {
		if uid == userID {
			delete(sessionStore, sid)
		}
	}
}
