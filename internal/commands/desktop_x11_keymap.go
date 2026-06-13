//go:build linux

package commands
import (
	"fmt"
	"strings"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const (
	ksShiftL    = 0xffe1
	ksControlL  = 0xffe3
	ksAltL      = 0xffe9
	ksSuperL    = 0xffeb
	ksISOLevel3 = 0xfe03 // AltGr
)

var modifierKeysyms = map[string]xproto.Keysym{
	"ctrl": ksControlL, "control": ksControlL,
	"alt": ksAltL, "option": ksAltL,
	"shift": ksShiftL,
	"super": ksSuperL, "win": ksSuperL, "logo": ksSuperL,
	"meta": ksSuperL, "cmd": ksSuperL, "command": ksSuperL,
	"altgr": ksISOLevel3,
}

var namedKeysyms = map[string]xproto.Keysym{
	"return": 0xff0d, "enter": 0xff0d,
	"tab":    0xff09,
	"escape": 0xff1b, "esc": 0xff1b,
	"backspace": 0xff08,
	"delete":    0xffff, "del": 0xffff,
	"space": 0x0020,
	"up":    0xff52, "down": 0xff54, "left": 0xff51, "right": 0xff53,
	"home": 0xff50, "end": 0xff57,
	"page_up": 0xff55, "pageup": 0xff55, "prior": 0xff55,
	"page_down": 0xff56, "pagedown": 0xff56, "next": 0xff56,
	"insert": 0xff63,
	"menu":   0xff67,
	"f1":     0xffbe, "f2": 0xffbf, "f3": 0xffc0, "f4": 0xffc1,
	"f5": 0xffc2, "f6": 0xffc3, "f7": 0xffc4, "f8": 0xffc5,
	"f9": 0xffc6, "f10": 0xffc7, "f11": 0xffc8, "f12": 0xffc9,
}

func keysymForRune(r rune) xproto.Keysym {
	switch r {
	case '\n', '\r':
		return 0xff0d // Return
	case '\t':
		return 0xff09 // Tab
	case '\b':
		return 0xff08 // BackSpace
	}
	if (r >= 0x20 && r <= 0x7e) || (r >= 0xa0 && r <= 0xff) {
		return xproto.Keysym(r)
	}
	return 0
}

func keysymForToken(tok string) (ks xproto.Keysym, isModifier bool, ok bool) {
	rs := []rune(tok)
	if len(rs) == 1 {
		if k := keysymForRune(rs[0]); k != 0 {
			return k, false, true
		}
	}
	low := strings.ToLower(tok)
	if k, found := modifierKeysyms[low]; found {
		return k, true, true
	}
	if k, found := namedKeysyms[low]; found {
		return k, false, true
	}
	return 0, false, false
}

type x11KeyLoc struct {
	code  xproto.Keycode
	shift bool // glyph lives on shift level 1
	set   bool
}

type x11Keymap struct {
	byKeysym map[xproto.Keysym]x11KeyLoc
	shift    xproto.Keycode // Shift_L keycode
}

func loadX11Keymap(conn *xgb.Conn) (*x11Keymap, error) {
	setup := xproto.Setup(conn)
	minKc := setup.MinKeycode
	count := int(setup.MaxKeycode) - int(setup.MinKeycode) + 1
	if count <= 0 {
		return nil, fmt.Errorf("x11 keyboard map: bad keycode range")
	}
	reply, err := xproto.GetKeyboardMapping(conn, minKc, byte(count)).Reply()
	if err != nil {
		return nil, fmt.Errorf("x11 GetKeyboardMapping: %w", err)
	}
	per := int(reply.KeysymsPerKeycode)
	km := &x11Keymap{byKeysym: make(map[xproto.Keysym]x11KeyLoc, count*2)}
	for i := 0; i < count; i++ {
		kc := xproto.Keycode(int(minKc) + i)
		base := i * per
		for level := 0; level < per && level < 2; level++ {
			if base+level >= len(reply.Keysyms) {
				break
			}
			ks := reply.Keysyms[base+level]
			if ks == 0 {
				continue
			}
			if _, exists := km.byKeysym[ks]; !exists {
				km.byKeysym[ks] = x11KeyLoc{code: kc, shift: level == 1, set: true}
			}
		}
	}
	if loc, ok := km.byKeysym[ksShiftL]; ok {
		km.shift = loc.code
	}
	return km, nil
}

func (km *x11Keymap) lookupKeysym(ks xproto.Keysym) (x11KeyLoc, bool) {
	loc, ok := km.byKeysym[ks]
	return loc, ok
}

func (km *x11Keymap) lookupRune(r rune) (x11KeyLoc, bool) {
	ks := keysymForRune(r)
	if ks == 0 {
		return x11KeyLoc{}, false
	}
	return km.lookupKeysym(ks)
}

func (km *x11Keymap) resolveChord(chord string) ([]xproto.Keycode, x11KeyLoc, error) {
	parts := strings.Split(chord, "+")
	var mods []xproto.Keycode
	var key x11KeyLoc
	for i, part := range parts {
		tok := strings.TrimSpace(part)
		if tok == "" {
			continue
		}
		ks, isMod, ok := keysymForToken(tok)
		if !ok {
			return nil, x11KeyLoc{}, fmt.Errorf("unknown key %q in chord %q", tok, chord)
		}
		loc, found := km.lookupKeysym(ks)
		if !found {
			return nil, x11KeyLoc{}, fmt.Errorf("key %q is not on the current keyboard layout", tok)
		}
		if isMod && i < len(parts)-1 {
			mods = append(mods, loc.code)
		} else {
			key = loc
			key.set = true
		}
	}
	return mods, key, nil
}

func (km *x11Keymap) resolveModifiers(text string) ([]xproto.Keycode, error) {
	var mods []xproto.Keycode
	for _, part := range strings.Split(text, "+") {
		tok := strings.TrimSpace(part)
		if tok == "" {
			continue
		}
		ks, _, ok := keysymForToken(tok)
		if !ok {
			return nil, fmt.Errorf("unknown modifier %q", tok)
		}
		loc, found := km.lookupKeysym(ks)
		if !found {
			return nil, fmt.Errorf("modifier %q is not on the current keyboard layout", tok)
		}
		mods = append(mods, loc.code)
	}
	return mods, nil
}
