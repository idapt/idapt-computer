package commands
import "os/exec"

func ensurePrimary(displays []DesktopDisplay) {
	if len(displays) == 0 {
		return
	}
	for _, d := range displays {
		if d.Primary {
			return
		}
	}
	pick := 0
	for i, d := range displays {
		if d.OffsetX == 0 && d.OffsetY == 0 {
			pick = i
			break
		}
	}
	displays[pick].Primary = true
}

func findDisplayByID(displays []DesktopDisplay, id string) (DesktopDisplay, bool) {
	for _, d := range displays {
		if d.ID == id {
			return d, true
		}
	}
	return DesktopDisplay{}, false
}

func binAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
