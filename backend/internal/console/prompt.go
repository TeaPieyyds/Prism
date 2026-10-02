package console

import (
	"bufio"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrUserQuit = errors.New("用户主动退出")

func PromptText(reader *bufio.Reader, q string) (string, error) {
	for {
		fmt.Print(q)
		raw, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		v := strings.TrimSpace(raw)
		if strings.EqualFold(v, "q") {
			return "", ErrUserQuit
		}
		if v == "" {
			fmt.Println(Warn("不能为空，请重新输入。"))
			continue
		}
		return v, nil
	}
}

func PromptOptionalText(reader *bufio.Reader, q string) (string, error) {
	fmt.Print(q)
	raw, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(raw)
	if strings.EqualFold(v, "q") {
		return "", ErrUserQuit
	}
	return v, nil
}

func PromptChoice(reader *bufio.Reader, q string, min, max int) (int, error) {
	for {
		fmt.Print(q)
		raw, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}
		v := strings.TrimSpace(raw)
		if strings.EqualFold(v, "q") {
			return 0, ErrUserQuit
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < min || n > max {
			fmt.Printf("请输入 %d-%d 的数字。\n", min, max)
			continue
		}
		return n, nil
	}
}

func PromptIntWithDefaultMin(reader *bufio.Reader, q string, def, min int) (int, error) {
	for {
		fmt.Print(q)
		raw, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}
		v := strings.TrimSpace(raw)
		if strings.EqualFold(v, "q") {
			return 0, ErrUserQuit
		}
		if v == "" {
			return def, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < min {
			fmt.Printf("请输入 >= %d 的整数。\n", min)
			continue
		}
		return n, nil
	}
}
