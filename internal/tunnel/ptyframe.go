package tunnel

import (
	"encoding/binary"
	"fmt"
	"io"
)
type PTYFrameType byte

const (
	PTYFrameData PTYFrameType = 0x01
	PTYFrameResize PTYFrameType = 0x02
	PTYFrameExit PTYFrameType = 0x03
)

const ptyFrameHeaderLen = 5

const PTYMaxFrameLen = 1 << 20 // 1 MiB

const PTYOutputChunk = 32 * 1024

func WritePTYFrame(w io.Writer, t PTYFrameType, payload []byte) error {
	if len(payload) > PTYMaxFrameLen {
		return fmt.Errorf("tunnel: pty frame too large (%d > %d)", len(payload), PTYMaxFrameLen)
	}
	buf := make([]byte, ptyFrameHeaderLen+len(payload))
	buf[0] = byte(t)
	binary.BigEndian.PutUint32(buf[1:ptyFrameHeaderLen], uint32(len(payload)))
	copy(buf[ptyFrameHeaderLen:], payload)
	_, err := w.Write(buf)
	return err
}

func WritePTYData(w io.Writer, p []byte) error {
	return WritePTYFrame(w, PTYFrameData, p)
}

func WritePTYResize(w io.Writer, cols, rows uint16) error {
	var b [4]byte
	binary.BigEndian.PutUint16(b[0:2], cols)
	binary.BigEndian.PutUint16(b[2:4], rows)
	return WritePTYFrame(w, PTYFrameResize, b[:])
}

func WritePTYExit(w io.Writer, code int32) error {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(code))
	return WritePTYFrame(w, PTYFrameExit, b[:])
}

func ReadPTYFrame(r io.Reader) (PTYFrameType, []byte, error) {
	var hdr [ptyFrameHeaderLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[1:ptyFrameHeaderLen])
	if n > PTYMaxFrameLen {
		return 0, nil, fmt.Errorf("tunnel: pty frame too large (%d > %d)", n, PTYMaxFrameLen)
	}
	if n == 0 {
		return PTYFrameType(hdr[0]), nil, nil
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return PTYFrameType(hdr[0]), payload, nil
}

func ParsePTYResize(payload []byte) (cols, rows uint16, ok bool) {
	if len(payload) != 4 {
		return 0, 0, false
	}
	return binary.BigEndian.Uint16(payload[0:2]), binary.BigEndian.Uint16(payload[2:4]), true
}

func ParsePTYExit(payload []byte) (code int32, ok bool) {
	if len(payload) != 4 {
		return 0, false
	}
	return int32(binary.BigEndian.Uint32(payload)), true
}
