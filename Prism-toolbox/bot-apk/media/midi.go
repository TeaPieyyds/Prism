package media

import (
	"fmt"
	"math"
	"sort"
)

// MIDINote represents a single MIDI note event
type MIDINote struct {
	Tick     uint64
	Channel  uint8
	Note     uint8
	Velocity uint8
	Duration uint64
	Sound    string
}

// InstrumentMap maps MIDI program numbers to Minecraft note block sounds
var InstrumentMap = map[uint8]string{
	0:  "note.harp",
	1:  "note.harp",
	2:  "note.pling",
	3:  "note.harp",
	8:  "note.iron_xylophone",
	9:  "note.bell",
	14: "note.chime",
	32: "note.bass",
	41: "note.flute",
	80: "note.bit",
	105: "note.banjo",
	112: "note.cow_bell",
	115: "note.basedrum",
}

// PercussionMap maps MIDI percussion notes (channel 10) to Minecraft sounds
var PercussionMap = map[uint8]string{
	36: "note.bd",
	37: "note.snare",
	38: "note.snare",
	41: "note.hat",
	42: "note.hat",
	55: "note.cow_bell",
}

// NoteToPitch converts MIDI note number to Minecraft pitch value (float)
func NoteToPitch(note uint8) float64 {
	pitch := math.Pow(2, float64(int(note)-45)/12)
	return math.Round(pitch*100) / 100
}

// GeneratePlaysoundCommands converts MIDI notes to Minecraft playsound commands
func GeneratePlaysoundCommands(notes []MIDINote, target string, speedPercent int) []string {
	sort.Slice(notes, func(i, j int) bool {
		return notes[i].Tick < notes[j].Tick
	})

	var commands []string
	for _, note := range notes {
		pitch := NoteToPitch(note.Note)
		sound := note.Sound
		if sound == "" {
			sound = "note.harp"
		}
		cmd := fmt.Sprintf(
			"execute as %s at @s run playsound %s @s ~~~ 1 %.2f",
			target, sound, pitch,
		)
		commands = append(commands, cmd)
	}
	return commands
}
