package media

import (
	"fmt"
	"time"
)

// SendCmdFunc is a callback for sending commands to the server
type SendCmdFunc func(cmd string) error

type PlayerConfig struct {
	Target    string
	Mode      string
	FPS       int
	Speed     float64
	LoopCount int
}

type Player struct {
	SendCmd SendCmdFunc
	Config  PlayerConfig
}

func NewPlayer(sendCmd SendCmdFunc) *Player {
	return &Player{
		SendCmd: sendCmd,
		Config: PlayerConfig{
			Target: "@a",
			Mode:   "actionbar",
			FPS:    10,
			Speed:  1.0,
		},
	}
}

// PlayASCIIFrames sends a sequence of ASCII frames as titleraw commands
func (p *Player) PlayASCIIFrames(frames [][]string, stopCh <-chan struct{}) error {
	frameDelay := time.Second / time.Duration(float64(p.Config.FPS)*p.Config.Speed)
	loop := 0

	for p.Config.LoopCount == 0 || loop < p.Config.LoopCount {
		for _, frame := range frames {
			select {
			case <-stopCh:
				return nil
			default:
			}

			cmds := GenerateTitlerawCommands(frame, p.Config.Target, p.Config.Mode)
			for _, cmd := range cmds {
				if err := p.SendCmd(cmd); err != nil {
					return fmt.Errorf("PlayASCIIFrames: %w", err)
				}
			}
			time.Sleep(frameDelay)
		}
		loop++
	}
	return nil
}

// PlayMIDINotes sends MIDI notes as playsound commands
func (p *Player) PlayMIDINotes(notes []MIDINote, stopCh <-chan struct{}) error {
	cmds := GeneratePlaysoundCommands(notes, p.Config.Target, 100)
	for _, cmd := range cmds {
		select {
		case <-stopCh:
			return nil
		default:
		}
		if err := p.SendCmd(cmd); err != nil {
			return fmt.Errorf("PlayMIDINotes: %w", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}
