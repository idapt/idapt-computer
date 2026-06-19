package tunnel

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type StreamKind string

const (
	StreamKindHTTP StreamKind = "http"
	StreamKindSSH StreamKind = "ssh"
)

const maxControlLine = 8 * 1024

type StreamHeader struct {
	Kind StreamKind `json:"kind"`
	Port int        `json:"port"`
	RemoteAddr string `json:"remoteAddr,omitempty"`
}

type StreamReply struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (h StreamHeader) Validate() error {
	if h.Kind != StreamKindHTTP && h.Kind != StreamKindSSH {
		return fmt.Errorf("tunnel: unknown stream kind %q", h.Kind)
	}
	if h.Port < 1 || h.Port > 65535 {
		return fmt.Errorf("tunnel: port %d out of range", h.Port)
	}
	return nil
}

func NewReader(r io.Reader) *bufio.Reader {
	return bufio.NewReaderSize(r, maxControlLine)
}

func WriteHeader(w io.Writer, h StreamHeader) error {
	if err := h.Validate(); err != nil {
		return err
	}
	return writeJSONLine(w, h)
}

func ReadHeader(r *bufio.Reader) (StreamHeader, error) {
	var h StreamHeader
	if err := readJSONLine(r, &h); err != nil {
		return StreamHeader{}, err
	}
	if err := h.Validate(); err != nil {
		return StreamHeader{}, err
	}
	return h, nil
}

func WriteReply(w io.Writer, reply StreamReply) error {
	return writeJSONLine(w, reply)
}

func ReadReply(r *bufio.Reader) (StreamReply, error) {
	var reply StreamReply
	if err := readJSONLine(r, &reply); err != nil {
		return StreamReply{}, err
	}
	return reply, nil
}

func writeJSONLine(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b)+1 > maxControlLine {
		return errors.New("tunnel: control line too large")
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func readJSONLine(r *bufio.Reader, v any) error {
	line, err := r.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return errors.New("tunnel: control line exceeds limit")
	}
	if err != nil {
		return fmt.Errorf("tunnel: read control line: %w", err)
	}
	if err := json.Unmarshal(line, v); err != nil {
		return fmt.Errorf("tunnel: decode control line: %w", err)
	}
	return nil
}
