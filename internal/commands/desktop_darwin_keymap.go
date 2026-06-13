package commands
import (
	"fmt"
	"strings"
)

const (
	cgFlagShift   = 1 << 17
	cgFlagControl = 1 << 18
	cgFlagOption  = 1 << 19
	cgFlagCommand = 1 << 20
)

var macModifierFlag = map[string]uint64{
	"ctrl": cgFlagControl, "control": cgFlagControl,
	"alt": cgFlagOption, "option": cgFlagOption,
	"shift": cgFlagShift,
	"super": cgFlagCommand, "win": cgFlagCommand, "logo": cgFlagCommand,
	"meta": cgFlagCommand, "cmd": cgFlagCommand, "command": cgFlagCommand,
}

var macLetterKeycode = map[rune]uint16{
	'a': 0, 'b': 11, 'c': 8, 'd': 2, 'e': 14, 'f': 3, 'g': 5, 'h': 4,
	'i': 34, 'j': 38, 'k': 40, 'l': 37, 'm': 46, 'n': 45, 'o': 31, 'p': 35,
	'q': 12, 'r': 15, 's': 1, 't': 17, 'u': 32, 'v': 9, 'w': 13, 'x': 7,
	'y': 16, 'z': 6,
}

var macDigitKeycode = map[rune]uint16{
	'0': 29, '1': 18, '2': 19, '3': 20, '4': 21, '5': 23, '6': 22, '7': 26, '8': 28, '9': 25,
}

var macNamedKeycode = map[string]uint16{
	"return": 0x24, "enter": 0x24,
	"tab":       0x30,
	"space":     0x31,
	"backspace": 0x33, "delete": 0x33, "del": 0x33,
	"escape": 0x35, "esc": 0x35,
	"left": 0x7B, "right": 0x7C, "down": 0x7D, "up": 0x7E,
	"home": 0x73, "end": 0x77,
	"page_up": 0x74, "pageup": 0x74, "prior": 0x74,
	"page_down": 0x79, "pagedown": 0x79, "next": 0x79,
	"f1": 0x7A, "f2": 0x78, "f3": 0x63, "f4": 0x76, "f5": 0x60, "f6": 0x61,
	"f7": 0x62, "f8": 0x64, "f9": 0x65, "f10": 0x6D, "f11": 0x67, "f12": 0x6F,
}

func macKeycodeForToken(tok string) (keycode uint16, flag uint64, isModifier bool, ok bool) {
	rs := []rune(tok)
	if len(rs) == 1 {
		r := rs[0]
		lr := r
		if r >= 'A' && r <= 'Z' {
			lr = r - 'A' + 'a'
		}
		if c, found := macLetterKeycode[lr]; found {
			return c, 0, false, true
		}
		if c, found := macDigitKeycode[r]; found {
			return c, 0, false, true
		}
	}
	low := strings.ToLower(tok)
	if f, found := macModifierFlag[low]; found {
		return 0, f, true, true
	}
	if c, found := macNamedKeycode[low]; found {
		return c, 0, false, true
	}
	return 0, 0, false, false
}

func resolveMacChord(chord string) (flags uint64, keycode uint16, hasKey bool, err error) {
	parts := strings.Split(chord, "+")
	for i, part := range parts {
		tok := strings.TrimSpace(part)
		if tok == "" {
			continue
		}
		code, flag, isMod, ok := macKeycodeForToken(tok)
		if !ok {
			return 0, 0, false, fmt.Errorf("unknown key %q in chord %q", tok, chord)
		}
		if isMod && i < len(parts)-1 {
			flags |= flag
		} else {
			keycode = code
			hasKey = true
		}
	}
	return flags, keycode, hasKey, nil
}
