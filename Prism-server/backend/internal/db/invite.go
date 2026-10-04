package db

import (
	"fmt"
	"time"
)

type InviteStats struct {
	Code        string        `json:"code"`
	InviteCount int           `json:"invite_count"`
	TotalNuts   int           `json:"total_nuts"`
	Invitees    []InviteeInfo `json:"invitees"`
}

type InviteeInfo struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Status   string `json:"status"`
}

func GetMyInviteCode(userID int64) (string, error) {
	var code string
	err := DB.QueryRow(`SELECT invite_code FROM users WHERE id = ?`, userID).Scan(&code)
	return code, err
}

func GetInviteStats(userID int64) (*InviteStats, error) {
	code, err := GetMyInviteCode(userID)
	if err != nil {
		return nil, err
	}

	s := &InviteStats{Code: code, Invitees: []InviteeInfo{}}
	DB.QueryRow(`SELECT COUNT(*) FROM invite_records WHERE inviter_id = ?`, userID).Scan(&s.InviteCount)
	DB.QueryRow(`SELECT COALESCE(SUM(reward_nuts),0) FROM invite_records WHERE inviter_id = ?`, userID).Scan(&s.TotalNuts)

	rows, err := DB.Query(`SELECT u.id, COALESCE(u.username,''), COALESCE(u.login_count,0)
		FROM users u WHERE u.invited_by = ? ORDER BY u.created_at DESC`, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var info InviteeInfo
			var lc int
			rows.Scan(&info.UserID, &info.Username, &lc)
			if lc >= 10 {
				info.Status = "active"
			} else if lc > 0 {
				info.Status = "first_join"
			} else {
				info.Status = "joined"
			}
			s.Invitees = append(s.Invitees, info)
		}
	}
	return s, nil
}

func RedeemInviteCode(inviteCode string, inviteeID int64) error {
	var inviterID int64
	if err := DB.QueryRow(`SELECT id FROM users WHERE invite_code = ?`, inviteCode).Scan(&inviterID); err != nil {
		return fmt.Errorf("邀请码无效")
	}
	if inviterID == inviteeID {
		return fmt.Errorf("不能邀请自己")
	}

	var existing int
	DB.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ? AND invited_by IS NOT NULL AND invited_by != 0`, inviteeID).Scan(&existing)
	if existing > 0 {
		return fmt.Errorf("已被邀请过")
	}

	_, err := DB.Exec(`UPDATE users SET invited_by = ? WHERE id = ?`, inviterID, inviteeID)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	DB.Exec(`INSERT INTO invite_records (inviter_id, invitee_id, reward_type, reward_nuts, created_at) VALUES (?,?,'register',120,?)`, inviterID, inviteeID, now)
	AddNuts(inviterID, 120, "invite_register", &inviteeID, nil)
	AddNuts(inviteeID, 20, "invited_register", nil, nil)
	return nil
}

func ProcessInviteJoinRewards(inviteeID int64) {
	var inviterID int64
	var lc int
	if err := DB.QueryRow(`SELECT COALESCE(invited_by,0), COALESCE(login_count,0) FROM users WHERE id = ?`, inviteeID).Scan(&inviterID, &lc); err != nil || inviterID == 0 {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if lc == 1 {
		var n int
		DB.QueryRow(`SELECT COUNT(*) FROM invite_records WHERE inviter_id=? AND invitee_id=? AND reward_type='first_join'`, inviterID, inviteeID).Scan(&n)
		if n == 0 {
			DB.Exec(`INSERT INTO invite_records (inviter_id,invitee_id,reward_type,reward_nuts,created_at) VALUES (?,?,'first_join',6,?)`, inviterID, inviteeID, now)
			AddNuts(inviterID, 6, "invite_first_join", &inviteeID, nil)
		}
	}

	if lc >= 10 {
		var n int
		DB.QueryRow(`SELECT COUNT(*) FROM invite_records WHERE inviter_id=? AND invitee_id=? AND reward_type='join_10'`, inviterID, inviteeID).Scan(&n)
		if n == 0 {
			DB.Exec(`INSERT INTO invite_records (inviter_id,invitee_id,reward_type,reward_nuts,created_at) VALUES (?,?,'join_10',10,?)`, inviterID, inviteeID, now)
			AddNuts(inviterID, 10, "invite_join_10", &inviteeID, nil)
		}
	}
}
