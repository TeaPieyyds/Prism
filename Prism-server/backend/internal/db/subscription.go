package db

import (
	"fmt"
	"time"
)

// IsSubscriptionActive 返回当前是否处于订阅期及到期时间。
// subscription_until 为空或已过期视为不活跃。
func IsSubscriptionActive(userID int64) (bool, time.Time, error) {
	var until string
	err := DB.QueryRow(`SELECT COALESCE(subscription_until,'') FROM users WHERE id=?`, userID).Scan(&until)
	if err != nil {
		return false, time.Time{}, err
	}
	if until == "" {
		return false, time.Time{}, nil
	}
	untilT, err := time.Parse(time.RFC3339, until)
	if err != nil {
		return false, time.Time{}, err
	}
	return time.Now().UTC().Before(untilT), untilT, nil
}

// GrantSubscriptionDays 叠加授予天数并返回新的到期时间。
// 有效期内 → 从到期时间续期（保留 subscription_start）；
// 否则 subscription_start = now。
func GrantSubscriptionDays(userID int64, days int) (string, error) {
	if days <= 0 {
		return "", fmt.Errorf("天数必须大于0")
	}
	now := time.Now().UTC()
	var start, until string
	err := DB.QueryRow(`SELECT COALESCE(subscription_start,''), COALESCE(subscription_until,'') FROM users WHERE id=?`, userID).Scan(&start, &until)
	if err != nil {
		return "", err
	}
	base := now
	if until != "" {
		if ut, e := time.Parse(time.RFC3339, until); e == nil && ut.After(now) {
			base = ut
		}
	}
	if start == "" || base.Equal(now) {
		start = now.Format(time.RFC3339)
	}
	newUntil := base.AddDate(0, 0, days).Format(time.RFC3339)
	_, err = DB.Exec(`UPDATE users SET subscription_start=?, subscription_until=? WHERE id=?`, start, newUntil, userID)
	if err != nil {
		return "", err
	}
	return newUntil, nil
}

// RevokeSubscription 清空订阅。
func RevokeSubscription(userID int64) error {
	_, err := DB.Exec(`UPDATE users SET subscription_start='', subscription_until='' WHERE id=?`, userID)
	return err
}
