package authsvc

import (
	"context"
	"fmt"
	"os"

	g79 "github.com/Yeah114/g79client"
	"github.com/Yeah114/g79client/utils"
	"github.com/Yeah114/unmcpk"
)

func TransferCheckNum(ctx context.Context, isPC bool, data, engineVersion, patchVersion string) (string, error) {
	_ = ctx
	if engineVersion == "" {
		engineVersion = g79.EngineVersion
	}
	if patchVersion == "" {
		latestVersion, err := g79.GetGlobalG79LatestVersion()
		if err != nil {
			return "", fmt.Errorf("get latest version failed")
		}
		patchVersion = latestVersion
	}
	python3Path := os.Getenv("HUXAUTH_PYTHON3")
	value, err := unmcpk.GenerateTransferCheckNum(isPC, data, engineVersion, patchVersion, python3Path)
	if err != nil {
		return "", err
	}
	return value, nil
}

func TransferStartType(uid, contentHex string) (string, error) {
	plain, err := utils.G79HttpDecrypt(contentHex)
	if err != nil {
		return "", err
	}
	return utils.G79HttpEncrypt(uid + plain)
}
