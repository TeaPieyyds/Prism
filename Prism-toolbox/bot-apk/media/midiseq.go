package media

import (
	"bytes"
	"fmt"
	"strings"
)

// MidiSeq is a pre-converted MIDI sequence (ToolSound format)
type MidiSeq struct {
	Instruments []string
	Notes       []MidiSeqNote
	Duration    float64 // seconds
}

type MidiSeqNote struct {
	Instrument string
	Volume     float64
	Pitch      float64
	Delay      float64 // seconds
}

// MM_INSTRUMENT_DEVIATION_TABLE maps instrument names to MIDI pitch modifiers
var instrumentDeviation = map[string]int{
	"note.bell":             10,
	"note.chime":            11,
	"note.flute":            12,
	"note.guitar":           13,
	"note.xylophone":        14,
	"note.iron_xylophone":   15,
	"note.cow_bell":         16,
	"note.didgeridoo":       17,
	"note.bit":              18,
	"note.banjo":            19,
	"note.pling":            20,
	"note.harp":             21,
	"note.bass":             22,
	"note.bassattack":       23,
}

// ParseMidiSeq parses a .midseq binary file
func ParseMidiSeq(data []byte) (*MidiSeq, error) {
	// Find SEQ: marker (may have \x00 after it)
	parts := bytes.SplitN(data, []byte("SEQ:"), 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("ParseMidiSeq: invalid format, missing SEQ: marker")
	}

	// Skip leading null bytes after SEQ:
	noteData := parts[1]
	for len(noteData) > 0 && noteData[0] == 0 {
		noteData = noteData[1:]
	}

	// Parse instrument list (separated by \xff)
	instBytes := bytes.Split(parts[0], []byte{0xff})
	instruments := make([]string, len(instBytes))
	for i, b := range instBytes {
		instruments[i] = string(b)
	}

	// Parse note sequence
	seq := &MidiSeq{Instruments: instruments}
	chunks := bytes.Split(noteData, []byte{0xfe})

	for _, chunk := range chunks {
		if len(chunk) < 4 {
			continue
		}
		instIdx := int(chunk[0])
		vol := float64(chunk[1]) / 100.0
		pitchRaw := int(chunk[2])
		delayTicks := int(chunk[3])

		if instIdx >= len(instruments) {
			continue
		}
		inst := instruments[instIdx]

		// Apply pitch modifier
		modifier := 6
		if m, ok := instrumentDeviation[inst]; ok {
			modifier = m
		} else if strings.Contains(inst, "note.") {
			modifier = 6 // default for unknown note instruments
		}

		pitchResized := pitchRaw - modifier
		pitch := 1.0
		if inst != "note.snare" && inst != "note.bd" && inst != "note.hat" {
			pitch = power2(float64(pitchResized-60) / 12.0)
		}
		delay := float64(delayTicks) / 20.0

		seq.Notes = append(seq.Notes, MidiSeqNote{
			Instrument: inst,
			Volume:     vol,
			Pitch:      pitch,
			Delay:      delay,
		})
		seq.Duration += delay
	}

	return seq, nil
}

func power2(x float64) float64 {
	// Simple pow(2, x) implementation
	y := 1.0
	neg := x < 0
	if neg { x = -x }
	intPart := int(x)
	fracPart := x - float64(intPart)
	for i := 0; i < intPart; i++ { y *= 2 }
	// Approximate 2^frac
	y *= 1.0 + fracPart*0.693147 + fracPart*fracPart*0.240226
	if neg { y = 1.0 / y }
	return y
}

// GeneratePlaysoundCommandsFromSeq converts parsed MIDI sequence to playsound commands
func GeneratePlaysoundCommandsFromSeq(seq *MidiSeq, target string) []string {
	var cmds []string
	for _, n := range seq.Notes {
		cmd := fmt.Sprintf("execute as %s at @s run playsound %s @s ~~~ %.2f %.3f",
			target, n.Instrument, n.Volume, n.Pitch)
		cmds = append(cmds, cmd)
	}
	return cmds
}
