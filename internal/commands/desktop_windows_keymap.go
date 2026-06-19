package commands
import (
	"fmt"
	"strings"
)

const (
	vkBack    = 0x08
	vkTab     = 0x09
	vkReturn  = 0x0D
	vkShift   = 0x10
	vkControl = 0x11
	vkMenu    = 0x12 // Alt
	vkEscape  = 0x1B
	vkSpace   = 0x20
	vkPrior   = 0x21 // Page Up
	vkNext    = 0x22 // Page Down
	vkEnd     = 0x23
	vkHome    = 0x24
	vkLeft    = 0x25
	vkUp      = 0x26
	vkRight   = 0x27
	vkDown    = 0x28
	vkInsert  = 0x2D
	vkDelete  = 0x2E
	vkLWin    = 0x5B
)

var modifierVK = map[string]uint16{
	"ctrl": vkControl, "control": vkControl,
	"alt": vkMenu, "option": vkMenu,
	"shift": vkShift,
	"super": vkLWin, "win": vkLWin, "logo": vkLWin,
	"meta": vkLWin, "cmd": vkLWin, "command": vkLWin,
}

var namedVK = map[string]uint16{
	"return": vkReturn, "enter": vkReturn,
	"tab":    vkTab,
	"escape": vkEscape, "esc": vkEscape,
	"backspace": vkBack,
	"delete":    vkDelete, "del": vkDelete,
	"space": vkSpace,
	"up":    vkUp, "down": vkDown, "left": vkLeft, "right": vkRight,
	"home": vkHome, "end": vkEnd,
	"page_up": vkPrior, "pageup": vkPrior, "prior": vkPrior,
	"page_down": vkNext, "pagedown": vkNext, "next": vkNext,
	"insert": vkInsert,
	"f1":     0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73, "f5": 0x74, "f6": 0x75,
	"f7": 0x76, "f8": 0x77, "f9": 0x78, "f10": 0x79, "f11": 0x7A, "f12": 0x7B,
}

func vkForToken(tok string) (vk uint16, isModifier bool, ok bool) {
	rs := []rune(tok)
	if len(rs) == 1 {
		r := rs[0]
		switch {
		case r >= 'a' && r <= 'z':
			return uint16(r - 'a' + 'A'), false, true
		case r >= 'A' && r <= 'Z':
			return uint16(r), false, true
		case r >= '0' && r <= '9':
			return uint16(r), false, true
		}
	}
	low := strings.ToLower(tok)
	if v, found := modifierVK[low]; found {
		return v, true, true
	}
	if v, found := namedVK[low]; found {
		return v, false, true
	}
	return 0, false, false
}

func resolveVKChord(chord string) (mods []uint16, key uint16, hasKey bool, err error) {
	parts := strings.Split(chord, "+")
	for i, part := range parts {
		tok := strings.TrimSpace(part)
		if tok == "" {
			continue
		}
		vk, isMod, ok := vkForToken(tok)
		if !ok {
			return nil, 0, false, fmt.Errorf("unknown key %q in chord %q", tok, chord)
		}
		if isMod && i < len(parts)-1 {
			mods = append(mods, vk)
		} else {
			key = vk
			hasKey = true
		}
	}
	return mods, key, hasKey, nil
}
