package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/adb-lanlu/prism-oss/internal/db"
)

func saveMPayAccountForRequest(r *http.Request, cookieData, source, displayName, uid string) {
	bearer := getBearer(r)
	if bearer == "" {
		return
	}
	user, err := db.GetUserByToken(bearer)
	if err != nil || user == nil {
		return
	}
	// 先检查是否已存在
	dup, _ := db.LookupAccountByUID(uid)
	isDup := dup != nil

	// 先用基本信息创建账号
	info := db.AccountInfo{
		DisplayName: displayName,
		UID:         uid,
		Status:      "normal",
		Source:      source,
		CreatedBy:   &user.ID,
	}
	acc, err := db.AddAccount(&user.ID, cookieData, info)
	if err != nil {
		log.Printf("[ACCOUNTS] save mpay account failed: %v", err)
		return
	}
	// 立即分配代理，后续 g79 操作都走代理
	AssignProxy(acc.ID)

	// 用代理获取账号详情，更新到数据库
	if cli, err := newG79ClientWithProxy(acc.ID); err == nil {
		if err := cli.G79AuthenticateWithCookie(cookieData); err == nil && cli.UserDetail != nil {
			ud := cli.UserDetail
			if after, err := cli.GetPeUserLoginAfter(); err == nil && after != nil {
				if after.Entity.UsedName != "" {
					ud.UsedName = after.Entity.UsedName
				}
			}
			info.DisplayName = firstNonEmpty(ud.Name, ud.UsedName, displayName)
			info.UID = firstNonEmpty(cli.UserID, uid)
			info.GrowthLevel = ud.Level.Raw
			info.Score = ud.Score.Raw
			info.SkinNumber = fmt.Sprintf("%d", ud.SkinNumber.Int64())
			info.CapeNumber = fmt.Sprintf("%d", ud.CapeNumber.Int64())
			info.AvatarImageURL = ud.AvatarImageURL
			info.IsVip = ud.IsVIP
			db.UpdateAccountFull(acc.ID, info)
		}
	}
	_ = db.AddAuditLog(&user.ID, &acc.ID, "add_account", "game_account", info.DisplayName+" ("+info.UID+") source="+source, requestIP(r))
	if !isDup && acc.Status == "normal" {
		db.AddNuts(user.ID, 5, "add_account", nil, &acc.ID)
	}
	if user.ActiveAccountID == nil {
		if err := db.SetActiveAccount(user.ID, acc.ID); err != nil {
			log.Printf("[ACCOUNTS] set active mpay account failed: %v", err)
		}
	}
}
