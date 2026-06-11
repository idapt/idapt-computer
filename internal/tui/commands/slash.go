package commands

import (
	"errors"
	"fmt"
	"strings"
)

var ErrUnknown = errors.New("unknown command")

type Parsed struct {
	Verb string
	Args []string
}

func IsSlash(s string) bool {
	s = strings.TrimLeft(s, " \t")
	if !strings.HasPrefix(s, "/") {
		return false
	}
	if strings.HasPrefix(s, "//") {
		return false
	}
	return true
}

func Parse(line string) (Parsed, error) {
	line = strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(line, "/") {
		return Parsed{}, fmt.Errorf("not a slash command")
	}
	line = strings.TrimSpace(line[1:])
	if line == "" {
		return Parsed{}, nil // /  → no-op
	}

	tokens, err := tokenize(line)
	if err != nil {
		return Parsed{}, err
	}
	if len(tokens) == 0 {
		return Parsed{}, nil
	}
	verb := strings.ToLower(tokens[0])
	var args []string
	if len(tokens) > 1 {
		args = tokens[1:]
	}
	if _, ok := Registry[verb]; !ok {
		return Parsed{Verb: verb, Args: args}, fmt.Errorf("%w: /%s", ErrUnknown, verb)
	}
	return Parsed{Verb: verb, Args: args}, nil
}

func tokenize(s string) ([]string, error) {
	var (
		out  []string
		buf  strings.Builder
		quot byte
	)
	flush := func() {
		if buf.Len() > 0 {
			out = append(out, buf.String())
			buf.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quot != 0 && c == '\\' && i+1 < len(s):
			buf.WriteByte(s[i+1])
			i++
		case c == quot:
			quot = 0
		case quot != 0:
			buf.WriteByte(c)
		case c == '"' || c == '\'':
			quot = c
		case c == ' ' || c == '\t':
			flush()
		default:
			buf.WriteByte(c)
		}
	}
	if quot != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	flush()
	return out, nil
}
