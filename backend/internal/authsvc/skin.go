package authsvc

import (
	"fmt"

	g79 "github.com/Yeah114/g79client"
)

type SkinInfo struct {
	ItemID          string
	SkinDownloadURL string
	SkinIsSlim      bool
}

const defaultSkinItemID = "4672395235685216085"

func GetSkinInfo(cli *g79.Client) (SkinInfo, error) {
	userSettingList, err := cli.GetUserSettingList()
	if err != nil {
		return SkinInfo{}, fmt.Errorf("GetUserSettingList: %w", err)
	}
	if userSettingList.Code != 0 {
		return SkinInfo{}, fmt.Errorf("GetUserSettingList: %s(%d)", userSettingList.Message, userSettingList.Code)
	}

	itemID := userSettingList.Entity.SkinData.ItemID
	if itemID == "-1" || itemID == "" {
		if err := cli.ChangeSkin(defaultSkinItemID); err != nil {
			return SkinInfo{}, fmt.Errorf("ChangeSkin: %w", err)
		}
		itemID = defaultSkinItemID
	}

	downloadInfo, err := cli.GetDownloadInfo(itemID)
	if err != nil {
		return SkinInfo{}, fmt.Errorf("GetDownloadInfo: %w", err)
	}
	if downloadInfo.Code != 0 {
		return SkinInfo{}, fmt.Errorf("GetDownloadInfo: %s(%d)", downloadInfo.Message, downloadInfo.Code)
	}

	return SkinInfo{ItemID: itemID, SkinDownloadURL: downloadInfo.Entity.ResURL, SkinIsSlim: true}, nil
}
