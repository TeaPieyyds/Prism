package db

import (
	"fmt"
	"time"
)

type CheckinStatus struct {
	CheckedIn  bool `json:"checked_in"`
	Streak     int  `json:"streak"`
	RewardNuts int  `json:"reward_nuts"`
}

var checkinRewards = []int{1, 1, 1, 1, 1, 1, 3}

var cstLoc = time.FixedZone("CST", 8*3600)

func todayCST() string { return time.Now().In(cstLoc).Format("2006-01-02") }
func yesterdayCST() string { return time.Now().In(cstLoc).AddDate(0, 0, -1).Format("2006-01-02") }

func CheckinToday(userID int64) (*CheckinStatus, error) {
	today := todayCST()
	yesterday := yesterdayCST()

	// Check if already checked in
	var exists int
	DB.QueryRow(`SELECT COUNT(*) FROM user_checkins WHERE user_id = ? AND checkin_date = ?`, userID, today).Scan(&exists)
	if exists > 0 {
		return nil, fmt.Errorf("今日已签到")
	}

	// Calculate streak from yesterday
	var prevStreak int
	DB.QueryRow(`SELECT COALESCE(MAX(streak), 0) FROM user_checkins WHERE user_id = ? AND checkin_date = ?`, userID, yesterday).Scan(&prevStreak)

	streak := prevStreak + 1
	if streak > 7 {
		streak = 1
	}
	reward := checkinRewards[streak-1]

	// Insert checkin record
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := DB.Exec(`INSERT INTO user_checkins (user_id, checkin_date, streak, reward_nuts, created_at) VALUES (?, ?, ?, ?, ?)`,
		userID, today, streak, reward, now)
	if err != nil {
		return nil, err
	}

	// Credit nuts
	AddNuts(userID, reward, "daily_checkin", nil, nil)

	return &CheckinStatus{CheckedIn: true, Streak: streak, RewardNuts: reward}, nil
}

func GetCheckinStatus(userID int64) (*CheckinStatus, error) {
	today := todayCST()

	var streak, reward int
	err := DB.QueryRow(`SELECT streak, reward_nuts FROM user_checkins WHERE user_id = ? AND checkin_date = ?`,
		userID, today).Scan(&streak, &reward)
	if err != nil {
		yesterday := yesterdayCST()
		DB.QueryRow(`SELECT COALESCE(MAX(streak), 0) FROM user_checkins WHERE user_id = ? AND checkin_date = ?`,
			userID, yesterday).Scan(&streak)
		return &CheckinStatus{CheckedIn: false, Streak: streak, RewardNuts: 0}, nil
	}
	return &CheckinStatus{CheckedIn: true, Streak: streak, RewardNuts: reward}, nil
}
