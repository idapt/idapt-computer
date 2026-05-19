package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"
)
const (
	stateListen = 0x0A
)

type DiscoveredPort struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	PID      *int   `json:"pid,omitempty"`
	Process  string `json:"process,omitempty"`
}

func runPortDiscover(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil && env.RunAs != DaemonSelfUser {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}

	ports, err := discoverListeningPorts()
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	dataBytes, _ := json.Marshal(map[string]any{"ports": ports})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func discoverListeningPorts() ([]DiscoveredPort, error) {
	v4, err4 := parseProcNet("/proc/net/tcp", "tcp", false)
	if err4 != nil && !errors.Is(err4, fs.ErrNotExist) {
		return nil, err4
	}
	v6, err6 := parseProcNet("/proc/net/tcp6", "tcp6", true)
	if err6 != nil && !errors.Is(err6, fs.ErrNotExist) {
		return nil, err6
	}
	all := append(v4, v6...)
	return all, nil
}

func parseProcNet(path, protocol string, ipv6 bool) ([]DiscoveredPort, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := []DiscoveredPort{}
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // header
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		local := fields[1]
		st := fields[3]
		state, err := strconv.ParseUint(st, 16, 8)
		if err != nil || state != stateListen {
			continue
		}
		addrPort := strings.SplitN(local, ":", 2)
		if len(addrPort) != 2 {
			continue
		}
		port, err := strconv.ParseUint(addrPort[1], 16, 32)
		if err != nil {
			continue
		}
		addr := decodeHexIP(addrPort[0], ipv6)
		out = append(out, DiscoveredPort{
			Port:     int(port),
			Protocol: protocol,
			Address:  addr,
		})
	}
	return out, nil
}

func decodeHexIP(hex string, ipv6 bool) string {
	if ipv6 {
		if len(hex) != 32 {
			return hex
		}
		groups := []string{}
		for i := 0; i < 32; i += 8 {
			chunk := hex[i : i+8]
			rev := chunk[6:8] + chunk[4:6] + chunk[2:4] + chunk[0:2]
			groups = append(groups, rev)
		}
		joined := strings.Join(groups, "")
		var sb strings.Builder
		for i := 0; i < len(joined); i += 4 {
			if i > 0 {
				sb.WriteByte(':')
			}
			sb.WriteString(joined[i : i+4])
		}
		return sb.String()
	}
	if len(hex) != 8 {
		return hex
	}
	a, _ := strconv.ParseUint(hex[6:8], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)
	c, _ := strconv.ParseUint(hex[2:4], 16, 8)
	d, _ := strconv.ParseUint(hex[0:2], 16, 8)
	return fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)
}

func _runWithCtxNotIdle(ctx context.Context) {} // silence unused import
