package media

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

// MIDIFile represents a parsed standard MIDI file
type MIDIFile struct {
	Format   uint16
	Tracks   uint16
	Division uint16
	Tempo    uint32 // microseconds per quarter note
	Notes    []MIDINote
}

// readVarLen reads a MIDI variable-length value
func readVarLen(data []byte, pos *int) uint64 {
	var v uint64
	for {
		b := data[*pos]
		*pos++
		v = (v << 7) | uint64(b&0x7f)
		if b&0x80 == 0 {
			break
		}
	}
	return v
}

// MCInstrumentTable maps MIDI program numbers to Minecraft note block sounds.
// Based on @adb_蓝露 reference table.
var MCInstrumentTable = map[uint8]string{
	0: "note.harp", 1: "note.harp", 2: "note.pling", 3: "note.harp",
	4: "note.harp", 5: "note.harp", 6: "note.harp", 7: "note.harp",
	8: "note.xylophone", 9: "note.bell", 10: "note.pling",
	11: "note.iron_xylophone", 12: "note.xylophone", 13: "note.xylophone",
	14: "note.chime", 15: "note.iron_xylophone",
	16: "note.didgeridoo", 17: "note.didgeridoo", 18: "note.didgeridoo",
	19: "note.didgeridoo", 20: "note.didgeridoo", 21: "note.didgeridoo",
	22: "note.didgeridoo", 23: "note.didgeridoo",
	24: "note.guitar", 25: "note.guitar", 26: "note.guitar", 27: "note.guitar",
	28: "note.guitar", 29: "note.guitar", 30: "note.guitar", 31: "note.guitar",
	32: "note.bass", 33: "note.bass", 34: "note.bass", 35: "note.bass",
	36: "note.bass", 37: "note.bass", 38: "note.bass", 39: "note.bass",
	40: "note.flute", 41: "note.flute", 42: "note.flute", 43: "note.flute",
	44: "note.harp", 45: "note.harp", 46: "note.harp", 47: "note.harp",
	48: "note.harp", 49: "note.harp", 50: "note.harp", 51: "note.harp",
	52: "note.harp", 53: "note.harp", 54: "note.harp", 55: "note.harp",
	56: "note.bell", 57: "note.didgeridoo", 58: "note.didgeridoo",
	59: "note.bell", 60: "note.bell", 61: "note.didgeridoo",
	62: "note.bell", 63: "note.didgeridoo",
	64: "note.flute", 65: "note.flute", 66: "note.flute", 67: "note.flute",
	68: "note.flute", 69: "note.flute", 70: "note.flute", 71: "note.flute",
	72: "note.flute", 73: "note.flute", 74: "note.flute", 75: "note.flute",
	76: "note.flute", 77: "note.flute", 78: "note.flute", 79: "note.flute",
	80: "note.bit", 81: "note.bit", 82: "note.bit", 83: "note.bit",
	84: "note.bit", 85: "note.bit", 86: "note.bit", 87: "note.bit",
	88: "note.bit", 89: "note.bit", 90: "note.bit", 91: "note.bit",
	92: "note.bit", 93: "note.bit", 94: "note.bit", 95: "note.bit",
	96: "note.bit", 97: "note.bit",
	98: "note.iron_xylophone", 99: "note.iron_xylophone",
	100: "note.iron_xylophone", 101: "note.iron_xylophone",
	102: "note.iron_xylophone", 103: "note.chime",
	104: "note.banjo", 105: "note.banjo", 106: "note.banjo", 107: "note.banjo",
	108: "note.banjo", 109: "note.banjo", 110: "note.didgeridoo", 111: "note.didgeridoo",
	112: "note.cow_bell", 113: "note.cow_bell", 114: "note.cow_bell",
	115: "note.bd", 116: "note.bd", 117: "note.bd", 118: "note.bd",
	119: "note.cow_bell",
	120: "note.bit", 121: "note.bit", 122: "note.bit", 123: "note.bit",
	124: "note.bell", 125: "note.bell", 126: "note.bell", 127: "note.bell",
}

// MCInstrumentDeviation maps MC sound names to pitch deviation values.
var MCInstrumentDeviation = map[string]int{
	"note.harp":            6,
	"note.pling":           6,
	"note.guitar":          -6,
	"note.iron_xylophone":  6,
	"note.bell":            30,
	"note.xylophone":       30,
	"note.chime":           30,
	"note.banjo":           6,
	"note.flute":           18,
	"note.bass":            -18,
	"note.snare":           0,
	"note.didgeridoo":      -18,
	"note.bit":             6,
	"note.hat":             0,
	"note.bd":              0,
	"note.cow_bell":        6,
}

// MCPercussionTable maps MIDI percussion note numbers to MC sounds.
// Based on @adb_蓝露 reference table.
var MCPercussionTable = map[uint8]string{
	36: "note.bd", 37: "note.snare", 38: "note.snare", 39: "note.snare",
	40: "note.snare", 41: "note.hat", 42: "note.hat", 43: "note.hat",
	44: "note.hat", 45: "note.hat", 46: "note.hat", 49: "note.hat",
	51: "note.hat", 52: "note.hat", 53: "note.hat", 55: "note.cow_bell",
	57: "note.hat", 59: "note.hat",
}

func mcPercussionSound(note uint8) string {
	if s, ok := MCPercussionTable[note]; ok {
		return s
	}
	return "note.snare"
}

func mcSoundForProgram(prog uint8) string {
	if s, ok := MCInstrumentTable[prog]; ok {
		return s
	}
	return "note.harp"
}

func deviationForSound(sound string) int {
	if d, ok := MCInstrumentDeviation[sound]; ok {
		return d
	}
	return 6
}

type rawMidiNote struct {
	tick     uint64
	midi     uint8
	velocity uint8
	sound    string
}

// ParseMIDIFile parses a standard MIDI file and returns extracted notes
func ParseMIDIFile(data []byte) (*MIDIFile, error) {
	if len(data) < 14 {
		return nil, fmt.Errorf("file too small")
	}

	pos := 0
	// MThd
	if string(data[pos:pos+4]) != "MThd" {
		return nil, fmt.Errorf("not a MIDI file: missing MThd")
	}
	pos += 4
	hdrLen := binary.BigEndian.Uint32(data[pos:])
	pos += 4
	format := binary.BigEndian.Uint16(data[pos:])
	pos += 2
	ntrks := binary.BigEndian.Uint16(data[pos:])
	pos += 2
	division := binary.BigEndian.Uint16(data[pos:])
	pos += 2
	pos += int(hdrLen) - 6 // skip remaining header

	ticksPerBeat := division & 0x7fff
	tempo := uint32(500000) // default 120 BPM

	var rawNotes []rawMidiNote
	// Program changes persist across tracks — must be outside the per-track loop
	channelProgram := make(map[uint8]uint8)

	for t := uint16(0); t < ntrks; t++ {
		if pos+8 > len(data) {
			break
		}
		if string(data[pos:pos+4]) != "MTrk" {
			return nil, fmt.Errorf("expected MTrk at offset %d", pos)
		}
		pos += 4
		trkLen := int(binary.BigEndian.Uint32(data[pos:]))
		pos += 4
		trkEnd := pos + trkLen

		var runningStatus uint8
		var tick uint64

		for pos < trkEnd {
			delta := readVarLen(data, &pos)
			tick += delta

			status := data[pos]
			if status < 0x80 {
				status = runningStatus
			} else {
				pos++
				if status < 0xf0 {
					runningStatus = status
				}
			}

			switch {
			case status == 0xff: // Meta
				metaType := data[pos]
				pos++
				metaLen := int(readVarLen(data, &pos))
				if metaType == 0x51 && metaLen == 3 && pos+3 <= len(data) {
					tempo = uint32(data[pos])<<16 | uint32(data[pos+1])<<8 | uint32(data[pos+2])
				}
				pos += metaLen

			case status == 0xf0 || status == 0xf7: // SysEx
				syLen := int(readVarLen(data, &pos))
				pos += syLen

			case status >= 0x80 && status < 0xf0: // MIDI event
				channel := status & 0x0f
				eventType := status & 0xf0
				if pos >= len(data) {
					break
				}
				param1 := data[pos]
				pos++
				param2 := uint8(0)
				if eventType != 0xc0 && eventType != 0xd0 {
					if pos < len(data) {
						param2 = data[pos]
						pos++
					}
				}

				switch eventType {
				case 0x90: // Note On
					if param2 > 0 {
						var sound string
						if channel == 9 {
							// Percussion channel — use note number to select percussion sound
							sound = mcPercussionSound(param1)
						} else {
							prog := channelProgram[channel]
							sound = mcSoundForProgram(prog)
						}
						rawNotes = append(rawNotes, rawMidiNote{
							tick: tick, midi: param1, velocity: param2, sound: sound,
						})
					}
				case 0xc0: // Program Change
					channelProgram[channel] = param1
				}
			}
		}
		pos = trkEnd
	}

	if len(rawNotes) == 0 {
		return nil, fmt.Errorf("no notes found in MIDI file")
	}

	// Sort by tick
	sort.Slice(rawNotes, func(i, j int) bool { return rawNotes[i].tick < rawNotes[j].tick })

	// Convert microseconds → game ticks (1 game tick = 50000 microseconds, i.e. 50ms at 20tps)
	midi := &MIDIFile{
		Format:   format,
		Tracks:   ntrks,
		Division: division,
		Tempo:    tempo,
	}

	var prevTick uint64
	for _, rn := range rawNotes {
		gameTickDelta := uint64(0)
		if len(midi.Notes) > 0 {
			tickDelta := rn.tick - prevTick
			gameTickDelta = tickDelta * uint64(tempo) / (uint64(ticksPerBeat) * 50000)
			if gameTickDelta < 1 {
				gameTickDelta = 1
			}
		}
		prevTick = rn.tick

		// Volume: velocity (0-127) → 0-100
		vol := int(rn.velocity) * 100 / 127
		if vol < 1 {
			vol = 1
		}

		midi.Notes = append(midi.Notes, MIDINote{
			Tick:     gameTickDelta, // game ticks
			Channel:  0,
			Note:     rn.midi,   // raw MIDI note number
			Velocity: uint8(vol),
			Duration: 0,
			Sound:    rn.sound,
		})
	}

	return midi, nil
}

// ToMidiSeq converts parsed MIDI to the binary midseq format
func (m *MIDIFile) ToMidiSeq() (*MidiSeq, error) {
	instSet := make(map[string]int)
	var instruments []string
	for _, n := range m.Notes {
		if _, ok := instSet[n.Sound]; !ok {
			instSet[n.Sound] = len(instruments)
			instruments = append(instruments, n.Sound)
		}
	}

	seq := &MidiSeq{
		Instruments: instruments,
	}

	for _, n := range m.Notes {
		instIdx := instSet[n.Sound]

		pitch := midiNoteToPitch(int(n.Note), n.Sound)

		delaySec := float64(n.Tick) / 20.0 // game ticks → seconds

		seq.Notes = append(seq.Notes, MidiSeqNote{
			Instrument: instruments[instIdx],
			Volume:     float64(n.Velocity) / 100.0,
			Pitch:      pitch,
			Delay:      delaySec,
		})
		seq.Duration += delaySec
	}

	return seq, nil
}

// EncodeMidiSeq encodes a MidiSeq to the binary midseq format
func EncodeMidiSeq(seq *MidiSeq) []byte {
	var buf bytes.Buffer

	// Write instruments separated by 0xff
	for i, inst := range seq.Instruments {
		if i > 0 {
			buf.WriteByte(0xff)
		}
		buf.WriteString(inst)
	}

	// SEQ: marker + null
	buf.WriteString("SEQ:")
	buf.WriteByte(0)

	// Write notes separated by 0xfe
	instIndex := make(map[string]int)
	for i, inst := range seq.Instruments {
		instIndex[inst] = i
	}

	for _, n := range seq.Notes {
		idx := instIndex[n.Instrument]
		vol := int(n.Volume * 100)
		if vol > 255 {
			vol = 255
		}
		if vol < 1 {
			vol = 1
		}

		// Calculate pitch shift (reverse of the decode)
		deviation := deviationForSound(n.Instrument)
		var pitchShift int
		if n.Instrument == "note.snare" || n.Instrument == "note.bd" || n.Instrument == "note.hat" {
			pitchShift = 60 + deviation
		} else {
			// pitch = 2^((pitchShift - 60) / 12)
			// pitchShift = 12 * log2(pitch) + 60 + deviation
			pitchShift = int(12*log2approx(n.Pitch)) + 60 + deviation
		}
		if pitchShift < 0 {
			pitchShift = 0
		}
		if pitchShift > 255 {
			pitchShift = 255
		}

		// Delay in game ticks (1/20s)
		delayTicks := int(n.Delay * 20)
		if delayTicks < 1 {
			delayTicks = 1
		}
		if delayTicks > 255 {
			delayTicks = 255
		}

		buf.Write([]byte{byte(idx), byte(vol), byte(pitchShift), byte(delayTicks)})
	}

	return buf.Bytes()
}

// MidiSeqFromMIDIFile parses a MIDI file and returns a MidiSeq directly
func MidiSeqFromMIDIFile(data []byte) (*MidiSeq, error) {
	mf, err := ParseMIDIFile(data)
	if err != nil {
		return nil, err
	}
	return mf.ToMidiSeq()
}

func log2approx(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Approximate log2 using math.Log
	// Simple power-of-2 approximation
	result := 0.0
	for x >= 2.0 {
		x /= 2.0
		result++
	}
	for x < 1.0 {
		x *= 2.0
		result--
	}
	// Linear interpolation for fractional part
	result += (x - 1.0) * 1.442695 // 1/ln(2) ≈ 1.442695
	return result
}

// PreviewNote is a simplified note for frontend preview
type PreviewNote struct {
	Instrument string  `json:"instrument"`
	Pitch      float64 `json:"pitch"`
	Volume     float64 `json:"volume"`
	DelayMs    int     `json:"delay_ms"` // milliseconds for frontend timing
	Tick       uint64  `json:"tick"`     // game ticks for backend playback
}

// ToPreviewNotes converts MIDI notes to frontend-previewable notes
func (m *MIDIFile) ToPreviewNotes() []PreviewNote {
	var notes []PreviewNote
	for _, n := range m.Notes {
		pitch := midiNoteToPitch(int(n.Note), n.Sound)
		notes = append(notes, PreviewNote{
			Instrument: n.Sound,
			Pitch:      pitch,
			Volume:     float64(n.Velocity) / 127.0,
			DelayMs:    int(n.Tick) * 50, // game ticks * 50ms
			Tick:       n.Tick,
		})
	}
	return notes
}

// midiNoteToPitch converts a raw MIDI note number + MC sound to Minecraft pitch value
func midiNoteToPitch(midiNote int, sound string) float64 {
	if sound == "note.snare" || sound == "note.bd" || sound == "note.hat" {
		return 1.0
	}
	deviation := deviationForSound(sound)
	// pitch = 2^((midi - 60 - deviation) / 12)
	pitchResized := midiNote - 60 - deviation
	return math.Pow(2, float64(pitchResized)/12.0)
}
