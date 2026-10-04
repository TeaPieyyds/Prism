package main

import (
	"fmt"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/game_control/game_interface"
)

// SendWSCommandWithTimeout 通过 WS 自动化通道发送命令并等待响应。
// 如果 gameInterface 未就绪，回退到普通 WS 命令（无响应）。
func (bm *BotManager) SendWSCommandWithTimeout(cmd string, timeout time.Duration) (*packet.CommandOutput, bool, error) {
	gi, err := bm.getGameInterface()
	if err != nil {
		// 回退：普通 WS 命令，无响应
		_ = bm.SendWSCommand(cmd)
		return nil, false, fmt.Errorf("gameInterface 未就绪: %v", err)
	}
	return gi.Commands().SendWSCommandWithTimeout(cmd, timeout)
}

// SendWSCommandWithResp 通过 WS 自动化通道发送命令并等待响应（默认超时）。
func (bm *BotManager) SendWSCommandWithResp(cmd string) (*packet.CommandOutput, error) {
	gi, err := bm.getGameInterface()
	if err != nil {
		_ = bm.SendWSCommand(cmd)
		return nil, fmt.Errorf("gameInterface 未就绪: %v", err)
	}
	return gi.Commands().SendWSCommandWithResp(cmd)
}

// SendPlayerCommandWithTimeout 以玩家身份发送命令并等待响应。
func (bm *BotManager) SendPlayerCommandWithTimeout(cmd string, timeout time.Duration) (*packet.CommandOutput, bool, error) {
	gi, err := bm.getGameInterface()
	if err != nil {
		_ = bm.SendPlayerCommand(cmd)
		return nil, false, fmt.Errorf("gameInterface 未就绪: %v", err)
	}
	return gi.Commands().SendPlayerCommandWithTimeout(cmd, timeout)
}

// SendPlayerCommandWithResp 以玩家身份发送命令并等待响应（默认超时）。
func (bm *BotManager) SendPlayerCommandWithResp(cmd string) (*packet.CommandOutput, error) {
	gi, err := bm.getGameInterface()
	if err != nil {
		_ = bm.SendPlayerCommand(cmd)
		return nil, fmt.Errorf("gameInterface 未就绪: %v", err)
	}
	return gi.Commands().SendPlayerCommandWithResp(cmd)
}

// SendSettingsCommand 发送设置命令（等同于 SendWSCommand）。
func (bm *BotManager) SendSettingsCommand(cmd string, dimensional bool) error {
	gi, err := bm.getGameInterface()
	if err != nil {
		return bm.SendWSCommand(cmd)
	}
	return gi.Commands().SendSettingsCommand(cmd, dimensional)
}


// AwaitChangesGeneral 等待默认数量的游戏刻。
func (bm *BotManager) AwaitChangesGeneral() error {
	gi, err := bm.getGameInterface()
	if err != nil {
		return fmt.Errorf("gameInterface 未就绪: %v", err)
	}
	return gi.Commands().AwaitChangesGeneral()
}

// DoQuerytarget 查询目标选择器的坐标信息。
// 直接委托给 game_interface.Querytarget，返回已解析的结构化结果。
func (bm *BotManager) DoQuerytarget(target string) ([]game_interface.TargetQueryingInfo, error) {
	gi, err := bm.getGameInterface()
	if err != nil {
		return nil, fmt.Errorf("gameInterface 未就绪: %v", err)
	}
	return gi.Querytarget().DoQuerytarget(target)
}