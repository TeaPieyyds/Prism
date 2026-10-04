package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	g79 "github.com/Yeah114/g79client"
	"github.com/adb-lanlu/prism-oss/internal/apisrv"
	"github.com/adb-lanlu/prism-oss/internal/authsvc"
	"github.com/adb-lanlu/prism-oss/internal/console"
	"github.com/adb-lanlu/prism-oss/internal/netproxy"
)

type App struct {
	Reader *bufio.Reader
}

func New() *App { return &App{Reader: bufio.NewReader(os.Stdin)} }

func (a *App) Run() {
	for {
		fmt.Println()
		fmt.Println(console.Title("请选择操作"))
		fmt.Println("1. 启动验证")
		fmt.Println("2. 直接进入")
		fmt.Println("3. 退出程序")

		choice, err := console.PromptChoice(a.Reader, "输入 1~3: ", 1, 3)
		if err != nil {
			if errors.Is(err, console.ErrUserQuit) {
				fmt.Println(console.Warn("已退出。"))
				return
			}
			fmt.Println(console.Err("输入错误: " + err.Error()))
			continue
		}

		switch choice {
		case 1:
			a.handleStartAuth()
		case 2:
			a.handleDirectEnter()
		case 3:
			fmt.Println(console.OK("感谢使用 Prism，再见。"))
			return
		}
	}
}

func (a *App) handleStartAuth() {
	fmt.Println()
	fmt.Println(console.Title("启动验证服务"))
	fmt.Println("1. cookie当token")
	fmt.Println("2. 随机token（需本地cookie.txt）")
	fmt.Println("3. 自定义token（需本地cookie.txt）")

	choice, err := console.PromptChoice(a.Reader, "请选择 1/2/3: ", 1, 3)
	if err != nil {
		a.printErr(err)
		return
	}

	cfg := apisrv.StartConfig{}
	switch choice {
	case 1:
		cfg.TokenMode = apisrv.TokenModeCookieAsToken
		fmt.Println(console.OK("模式: cookie当token（用户 login_token 直接当cookie）"))
	case 2:
		cookie, err := loadCookieTxtOnly()
		if err != nil {
			fmt.Println(console.Err(err.Error()))
			return
		}
		token := withPrefix(generateRandomToken())
		cfg.TokenMode = apisrv.TokenModeFixedToken
		cfg.FixedCookie = cookie
		cfg.FixedToken = token
		cfg.CookieFile = "cookie.txt"
		cfg.HotReloadFile = true
		fmt.Println(console.OK("随机token生成成功: " + token))
		fmt.Println(console.Info("已开启 cookie.txt 热重载（修改文件后请求会自动使用新cookie）"))
	case 3:
		cookie, err := loadCookieTxtOnly()
		if err != nil {
			fmt.Println(console.Err(err.Error()))
			return
		}
		content, err := console.PromptText(a.Reader, "请输入自定义token内容（将自动加 xiaoruo/ 前缀）: ")
		if err != nil {
			a.printErr(err)
			return
		}
		token := withPrefix(content)
		cfg.TokenMode = apisrv.TokenModeFixedToken
		cfg.FixedCookie = cookie
		cfg.FixedToken = token
		cfg.CookieFile = "cookie.txt"
		cfg.HotReloadFile = true
		fmt.Println(console.OK("自定义token设置成功: " + token))
		fmt.Println(console.Info("已开启 cookie.txt 热重载（修改文件后请求会自动使用新cookie）"))
	}

	pm, err := netproxy.Setup(a.Reader)
	if err != nil {
		a.printErr(err)
		return
	}
	cfg.Proxy = pm

	fmt.Println(console.Info("端口: 9191"))
	fmt.Println(console.Info("接口: /api/new"))
	fmt.Println(console.Info("接口: /api/phoenix/login"))
	fmt.Println(console.Info("接口: /api/phoenix/transfer_check_num"))
	fmt.Println(console.Info("接口: /api/phoenix/transfer_start_type"))
	fmt.Println(console.Info("接口: /api/phoenix/tan_lobby_login"))
	fmt.Println(console.Info("接口: /api/phoenix/tan_lobby_create"))
	fmt.Println(console.Info("接口: /api/phoenix/tan_lobby_transfer_server"))

	if err := apisrv.Start9191(cfg); err != nil {
		fmt.Println(console.Err("服务启动失败: " + err.Error()))
	}
}

func (a *App) handleDirectEnter() {
	fmt.Println()
	fmt.Println(console.Title("直接进入"))
	target, detail, passcode, err := a.chooseEnterTarget()
	if err != nil {
		a.printErr(err)
		return
	}

	cookie, err := loadLocalCookie()
	if err != nil {
		fmt.Println(console.Err(err.Error()))
		return
	}
	fmt.Println(console.OK("已读取本地 cookie/cookies.txt"))

	pm, err := netproxy.Setup(a.Reader)
	if err != nil {
		a.printErr(err)
		return
	}

	fmt.Println(console.Info("自动检测版本中（顺序: 3.8 -> 3.7 -> 3.6，每版本最多3次）"))
	for _, version := range []string{"3.8", "3.7", "3.6"} {
		for i := 1; i <= 3; i++ {
			fmt.Printf("尝试版本 %s 第 %d 次...\n", version, i)
			if err := a.tryEnterOnce(version, target, detail, passcode, cookie, pm); err != nil {
				pm.ReportResult(err)
				fmt.Println(console.Warn(err.Error()))
				continue
			}
			fmt.Println(console.OK("进入成功 -> " + version))
			return
		}
	}
	fmt.Println(console.Err("所有版本尝试失败。"))
}

func (a *App) chooseEnterTarget() (enterTarget, string, string, error) {
	fmt.Println("1. 租赁服 (Rental Game)")
	fmt.Println("2. 我的山头 (Domain Game)")
	fmt.Println("3. 本地联机 (Tan Lobby)")
	fmt.Println("4. 联机大厅 (Online Lobby)")
	fmt.Println("5. 网络游戏 (Network Game)")
	fmt.Println("6. 主城乐园 (Main City)")

	idx, err := console.PromptChoice(a.Reader, "请选择进入位置 1-6: ", 1, 6)
	if err != nil {
		return enterTarget{}, "", "", err
	}
	t := targets[idx-1]
	detail, err := console.PromptText(a.Reader, t.Hint+": ")
	if err != nil {
		return enterTarget{}, "", "", err
	}
	passcode := ""
	if t.Name == "rental" || t.Name == "online" {
		passcode, err = console.PromptOptionalText(a.Reader, "请输入房间密码（可留空）: ")
		if err != nil {
			return enterTarget{}, "", "", err
		}
	}
	return t, detail, passcode, nil
}

func (a *App) tryEnterOnce(version string, target enterTarget, detail, passcode, cookie string, pm *netproxy.Manager) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cli, err := authsvc.AuthenticateWithCookie(ctx, cookie, pm)
	if err != nil {
		return err
	}

	if target.Name == "tan" {
		res, err := authsvc.TanLobbyLogin(ctx, cli, authsvc.TanLobbyLoginParams{RoomID: detail})
		if err != nil {
			return err
		}
		fmt.Printf("TanLobby => Raknet: %s, Signal: %s\n", res.RaknetServerAddress, res.SignalingServerAddress)
		return nil
	}

	serverCode := toServerCode(target, detail)
	clientPublicKey := generateClientPublicKey()
	if clientPublicKey == "" {
		return fmt.Errorf("生成 ClientPublicKey 失败")
	}

	provider := func(ctx context.Context) (*g79.Client, error) {
		c, err := authsvc.NewClient(ctx, pm)
		if err != nil {
			return nil, err
		}
		if err := c.G79AuthenticateWithCookie(cookie); err != nil {
			return nil, err
		}
		return c, nil
	}

	result, err := authsvc.Login(ctx, cli, authsvc.LoginParams{
		ServerCode:      serverCode,
		ServerPassword:  strings.TrimSpace(passcode),
		ClientPublicKey: clientPublicKey,
	}, provider)
	if err != nil {
		return fmt.Errorf("%s 进入失败: %w", version, err)
	}
	fmt.Printf("IP: %s\n", result.IP)
	fmt.Printf("UID: %s\n", result.UID)
	fmt.Printf("ChainInfo长度: %d\n", len(result.ChainInfo))
	return nil
}

func toServerCode(target enterTarget, detail string) string {
	switch target.Name {
	case "domain":
		return "DomainGame:" + detail
	case "online":
		return "LobbyGame:" + detail
	case "network":
		return "NetworkGame:" + detail
	case "main_city":
		return "MainCity"
	default:
		return detail
	}
}

func (a *App) printErr(err error) {
	if err == nil {
		return
	}
	if errors.Is(err, console.ErrUserQuit) {
		fmt.Println(console.Warn("已取消。"))
		return
	}
	fmt.Println(console.Err(err.Error()))
}
