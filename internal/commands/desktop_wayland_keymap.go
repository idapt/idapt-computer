//go:build linux

package commands
import (
	"fmt"
	"strings"
)

const (
	evKeyEsc       = 1
	evKeyBackspace = 14
	evKeyTab       = 15
	evKeyEnter     = 28
	evKeyLeftCtrl  = 29
	evKeyLeftShift = 42
	evKeyLeftAlt   = 56
	evKeySpace     = 57
	evKeyRightAlt  = 100
	evKeyHome      = 102
	evKeyUp        = 103
	evKeyPageUp    = 104
	evKeyLeft      = 105
	evKeyRight     = 106
	evKeyEnd       = 107
	evKeyDown      = 108
	evKeyPageDown  = 109
	evKeyInsert    = 110
	evKeyDelete    = 111
	evKeyLeftMeta  = 125
)

type evdevKey struct {
	code  uint32
	shift bool
}

var evdevUnshifted = map[rune]uint32{
	'a': 30, 'b': 48, 'c': 46, 'd': 32, 'e': 18, 'f': 33, 'g': 34, 'h': 35,
	'i': 23, 'j': 36, 'k': 37, 'l': 38, 'm': 50, 'n': 49, 'o': 24, 'p': 25,
	'q': 16, 'r': 19, 's': 31, 't': 20, 'u': 22, 'v': 47, 'w': 17, 'x': 45,
	'y': 21, 'z': 44,
	'1': 2, '2': 3, '3': 4, '4': 5, '5': 6, '6': 7, '7': 8, '8': 9, '9': 10, '0': 11,
	'-': 12, '=': 13, '[': 26, ']': 27, '\\': 43, ';': 39, '\'': 40, '`': 41,
	',': 51, '.': 52, '/': 53, ' ': evKeySpace,
}

var evdevShifted = map[rune]uint32{
	'!': 2, '@': 3, '#': 4, '$': 5, '%': 6, '^': 7, '&': 8, '*': 9, '(': 10, ')': 11,
	'_': 12, '+': 13, '{': 26, '}': 27, '|': 43, ':': 39, '"': 40, '~': 41,
	'<': 51, '>': 52, '?': 53,
}

var namedEvdev = map[string]uint32{
	"return": evKeyEnter, "enter": evKeyEnter,
	"tab":    evKeyTab,
	"escape": evKeyEsc, "esc": evKeyEsc,
	"backspace": evKeyBackspace,
	"delete":    evKeyDelete, "del": evKeyDelete,
	"space": evKeySpace,
	"up":    evKeyUp, "down": evKeyDown, "left": evKeyLeft, "right": evKeyRight,
	"home": evKeyHome, "end": evKeyEnd,
	"page_up": evKeyPageUp, "pageup": evKeyPageUp, "prior": evKeyPageUp,
	"page_down": evKeyPageDown, "pagedown": evKeyPageDown, "next": evKeyPageDown,
	"insert": evKeyInsert,
	"f1":     59, "f2": 60, "f3": 61, "f4": 62, "f5": 63, "f6": 64,
	"f7": 65, "f8": 66, "f9": 67, "f10": 68, "f11": 87, "f12": 88,
}

var modifierEvdev = map[string]uint32{
	"ctrl": evKeyLeftCtrl, "control": evKeyLeftCtrl,
	"alt": evKeyLeftAlt, "option": evKeyLeftAlt,
	"shift": evKeyLeftShift,
	"super": evKeyLeftMeta, "win": evKeyLeftMeta, "logo": evKeyLeftMeta,
	"meta": evKeyLeftMeta, "cmd": evKeyLeftMeta, "command": evKeyLeftMeta,
	"altgr": evKeyRightAlt,
}

func evdevForRune(r rune) (evdevKey, bool) {
	switch r {
	case '\n', '\r':
		return evdevKey{code: evKeyEnter}, true
	case '\t':
		return evdevKey{code: evKeyTab}, true
	case '\b':
		return evdevKey{code: evKeyBackspace}, true
	}
	if r >= 'a' && r <= 'z' {
		return evdevKey{code: evdevUnshifted[r]}, true
	}
	if r >= 'A' && r <= 'Z' {
		return evdevKey{code: evdevUnshifted[r-'A'+'a'], shift: true}, true
	}
	if c, ok := evdevUnshifted[r]; ok {
		return evdevKey{code: c}, true
	}
	if c, ok := evdevShifted[r]; ok {
		return evdevKey{code: c, shift: true}, true
	}
	return evdevKey{}, false
}

func evdevForToken(tok string) (key evdevKey, isModifier bool, ok bool) {
	rs := []rune(tok)
	if len(rs) == 1 {
		if k, found := evdevForRune(rs[0]); found {
			return k, false, true
		}
	}
	low := strings.ToLower(tok)
	if c, found := modifierEvdev[low]; found {
		return evdevKey{code: c}, true, true
	}
	if c, found := namedEvdev[low]; found {
		return evdevKey{code: c}, false, true
	}
	return evdevKey{}, false, false
}

func resolveEvdevChord(chord string) (mods []uint32, key evdevKey, hasKey bool, err error) {
	parts := strings.Split(chord, "+")
	for i, part := range parts {
		tok := strings.TrimSpace(part)
		if tok == "" {
			continue
		}
		k, isMod, ok := evdevForToken(tok)
		if !ok {
			return nil, evdevKey{}, false, fmt.Errorf("unknown key %q in chord %q", tok, chord)
		}
		if isMod && i < len(parts)-1 {
			mods = append(mods, k.code)
		} else {
			key = k
			hasKey = true
		}
	}
	return mods, key, hasKey, nil
}
