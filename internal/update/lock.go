package update

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/idapt/idapt-computer/internal/idaptpaths"
)

const updateLockFile = "update.lock"

const lockStaleAfter = 15 * time.Minute

func AcquireUpdateLock() (release func(), err error) {
	dir, err := idaptpaths.EnsureCacheDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, updateLockFile)
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = f.WriteString(strconv.Itoa(os.Getpid()) + "\n" + time.Now().UTC().Format(time.RFC3339))
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > lockStaleAfter {
			_ = os.Remove(path)
			continue
		}
		return nil, fmt.Errorf("another update is already in progress (lock: %s)", path)
	}
	return nil, fmt.Errorf("could not acquire update lock at %s", path)
}
