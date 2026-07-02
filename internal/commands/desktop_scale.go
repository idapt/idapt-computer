package commands
import (
	"math"
	"strings"
)

func parseDarwinBackingScale(profiler string) float64 {
	var lastResW float64
	sawRetina := false
	for _, raw := range strings.Split(profiler, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "Resolution:"):
			if w := firstIntField(line); w > 0 {
				lastResW = float64(w)
			}
			if strings.Contains(line, "Retina") {
				sawRetina = true
			}
		case strings.HasPrefix(line, "UI Looks like:"):
			if w := firstIntField(line); w > 0 && lastResW >= float64(w) {
				if s := lastResW / float64(w); s >= 1 && s <= 4 {
					return s
				}
			}
		}
	}
	if sawRetina {
		return 2
	}
	return 1
}

func firstIntField(s string) int {
	n := 0
	inNum := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
			inNum = true
		} else if inNum {
			break
		}
	}
	return n
}

func scaleDownCoord(v int, scale float64) int {
	if scale <= 1 {
		return v
	}
	return int(math.Round(float64(v) / scale))
}

func scaleUpCoord(v int, scale float64) int {
	if scale <= 1 {
		return v
	}
	return int(math.Round(float64(v) * scale))
}
