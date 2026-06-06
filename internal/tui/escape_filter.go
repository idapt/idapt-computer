package tui

import "unicode"

func looksLikeUnparsedCSI(s string) bool {
	if len(s) < 3 || len(s) > 16 {
		return false
	}
	if s[0] != '[' {
		return false
	}
	last := s[len(s)-1]
	if last != '~' && !(last >= 'a' && last <= 'z') && !(last >= 'A' && last <= 'Z') {
		return false
	}
	for i := 1; i < len(s)-1; i++ {
		r := rune(s[i])
		if r == ';' {
			continue
		}
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
