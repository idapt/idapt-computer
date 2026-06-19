package hardware

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Info struct {
	CPUCount   int     `json:"cpuCount"`
	RAMTotalGb *int    `json:"ramGb"`
	GPUVendor  *string `json:"gpuVendor"`
	GPUVRAMGb  *int    `json:"gpuVramGb"`
}

var (
	cached     Info
	cachedOnce sync.Once
)

func Detect() Info {
	cachedOnce.Do(func() {
		cached = detect()
	})
	return cached
}

func detect() Info {
	info := Info{CPUCount: runtime.NumCPU()}
	if ram := detectRAMGb(); ram > 0 {
		info.RAMTotalGb = &ram
	}
	vendor := detectGPUVendor()
	info.GPUVendor = vendor
	if vendor != nil {
		if vram := detectGPUVRAMGb(*vendor, info.RAMTotalGb); vram > 0 {
			info.GPUVRAMGb = &vram
		}
	}
	return info
}

func detectGPUVendor() *string {
	if _, err := os.Stat("/dev/nvidia0"); err == nil {
		v := "nvidia"
		return &v
	}
	if _, err := os.Stat("/dev/kfd"); err == nil {
		v := "amd"
		return &v
	}
	if runtime.GOOS == "darwin" {
		v := "metal"
		return &v
	}
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		v := "nvidia"
		return &v
	}
	return nil
}

func detectRAMGb() int {
	switch runtime.GOOS {
	case "linux":
		return linuxRAMGb()
	case "darwin":
		return darwinRAMGb()
	case "windows":
		return windowsRAMGb()
	default:
		return 0
	}
}

func linuxRAMGb() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	return parseMemTotalKbToGb(string(data))
}

func parseMemTotalKbToGb(meminfo string) int {
	for _, line := range strings.Split(meminfo, "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line) // "MemTotal:", "<kB>", "kB"
		if len(fields) >= 2 {
			if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil && kb > 0 {
				return int((kb + (1024*1024)/2) / (1024 * 1024)) // round kB → GB
			}
		}
		return 0
	}
	return 0
}

func darwinRAMGb() int {
	out, err := runCmd(2*time.Second, "sysctl", "-n", "hw.memsize")
	if err != nil {
		return 0
	}
	b, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil || b <= 0 {
		return 0
	}
	const gb = 1 << 30
	return int((b + gb/2) / gb)
}

func detectGPUVRAMGb(vendor string, ramGb *int) int {
	switch vendor {
	case "nvidia":
		return nvidiaVRAMGb()
	case "amd":
		return amdVRAMGb()
	case "metal":
		if ramGb != nil {
			return *ramGb
		}
		return 0
	default:
		return 0
	}
}

func nvidiaVRAMGb() int {
	out, err := runCmd(
		3*time.Second,
		"nvidia-smi",
		"--query-gpu=memory.total",
		"--format=csv,noheader,nounits",
	)
	if err != nil {
		return 0
	}
	first := strings.TrimSpace(strings.SplitN(strings.TrimSpace(out), "\n", 2)[0])
	mib, err := strconv.ParseFloat(first, 64)
	if err != nil || mib <= 0 {
		return 0
	}
	return int((mib + 512) / 1024) // round MiB → GiB
}

func amdVRAMGb() int {
	out, err := runCmd(3*time.Second, "rocm-smi", "--showmeminfo", "vram", "--csv")
	if err != nil {
		return 0
	}
	var best int64
	for _, field := range strings.FieldsFunc(out, func(r rune) bool {
		return r == ',' || r == '\n' || r == ' ' || r == '\r' || r == '\t'
	}) {
		if n, err := strconv.ParseInt(strings.TrimSpace(field), 10, 64); err == nil && n > best {
			best = n
		}
	}
	if best <= 0 {
		return 0
	}
	const gb = 1 << 30
	return int((best + gb/2) / gb)
}

func runCmd(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
