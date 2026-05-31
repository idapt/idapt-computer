package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/idapt/idapt-cli/internal/idaptpaths"
)

const checkStateFile = "update-check.json"

const DefaultNudgeTTL = 24 * time.Hour

const NudgeDisplayInterval = 24 * time.Hour

type CheckState struct {
	CheckedAt     time.Time `json:"checkedAt"`
	LatestVersion string    `json:"latestVersion"` // server form, e.g. "cli-v1.4.2"
	LastNudgedAt  time.Time `json:"lastNudgedAt"`   // when the banner was last shown
}

func (s CheckState) ShouldNudge(interval time.Duration) bool {
	return s.LastNudgedAt.IsZero() || time.Since(s.LastNudgedAt) >= interval
}

func checkStatePath() (string, error) {
	dir, err := idaptpaths.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, checkStateFile), nil
}

func LoadCheckState() CheckState {
	var st CheckState
	path, err := checkStatePath()
	if err != nil {
		return st
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return st
	}
	_ = json.Unmarshal(data, &st)
	return st
}

func SaveCheckState(latestVersion string) error {
	st := LoadCheckState()
	st.CheckedAt = time.Now()
	st.LatestVersion = latestVersion
	return writeCheckState(st)
}

func MarkNudged() error {
	st := LoadCheckState()
	st.LastNudgedAt = time.Now()
	return writeCheckState(st)
}

func writeCheckState(st CheckState) error {
	dir, err := idaptpaths.EnsureCacheDir()
	if err != nil {
		return err
	}
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, checkStateFile)
	tmp, err := os.CreateTemp(dir, ".update-check-*.tmp")
	if err != nil {
		return os.WriteFile(path, data, 0o644)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

func Nudge(current, latest string) (string, bool) {
	if latest == "" {
		return "", false
	}
	nl := NormalizeVersion(latest)
	if !IsValidVersionFormat(nl) {
		return "", false
	}
	if CompareVersions(nl, NormalizeVersion(current)) <= 0 {
		return "", false
	}
	return fmt.Sprintf("A new idapt version is available: %s → %s. Run `idapt update`.",
		NormalizeVersion(current), nl), true
}

func MaybeRefreshInBackground(appURL, current string, ttl time.Duration) {
	st := LoadCheckState()
	if !st.CheckedAt.IsZero() && time.Since(st.CheckedAt) < ttl {
		return
	}
	go func() {
		u := New(appURL, current, "")
		info, err := u.Check()
		if err != nil {
			_ = SaveCheckState(st.LatestVersion)
			return
		}
		latest := current
		if info != nil && info.Version != "" {
			latest = info.Version
		}
		_ = SaveCheckState(latest)
	}()
}
