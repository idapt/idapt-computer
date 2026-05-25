package commands

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func runFileRead(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p FileReadPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !filepath.IsAbs(p.Path) {
		return errResult(env.ID, ErrInvalidPayload, "absolute path required", start)
	}

	if env.Container != "" {
		return errResult(env.ID, ErrInvalidPayload, "container exec not implemented", start)
	}

	info, err := os.Stat(p.Path)
	if err != nil {
		return classifyFsError(env.ID, err, start)
	}
	if info.IsDir() {
		return errResult(env.ID, ErrInvalidPayload, "is a directory", start)
	}

	limit := p.Limit
	if limit <= 0 {
		limit = MaxOutputBytes
	}
	if limit > MaxOutputBytes {
		limit = MaxOutputBytes
	}
	offset := p.Offset
	if p.FromEnd {
		offset = int(info.Size()) - limit
		if offset < 0 {
			offset = 0
		}
	}

	f, err := os.Open(p.Path)
	if err != nil {
		return classifyFsError(env.ID, err, start)
	}
	defer f.Close()
	if offset > 0 {
		if _, err := f.Seek(int64(offset), 0); err != nil {
			return errResult(env.ID, ErrIO, err.Error(), start)
		}
	}
	buf := make([]byte, limit)
	n, err := f.Read(buf)
	if err != nil && err.Error() != "EOF" {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	body := buf[:n]
	hasMore := offset+n < int(info.Size())

	type out struct {
		ContentB64 string `json:"contentB64"`
		TotalBytes int    `json:"totalBytes"`
		HasMore    bool   `json:"hasMore"`
	}
	dataBytes, _ := json.Marshal(out{
		ContentB64: base64.StdEncoding.EncodeToString(body),
		TotalBytes: int(info.Size()),
		HasMore:    hasMore,
	})

	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runFileWrite(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p FileWritePayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !filepath.IsAbs(p.Path) {
		return errResult(env.ID, ErrInvalidPayload, "absolute path required", start)
	}
	if env.Container != "" {
		return errResult(env.ID, ErrInvalidPayload, "container exec not implemented", start)
	}
	owner, err := resolveRunAsOwner(env.RunAs)
	if err != nil {
		return errResult(env.ID, ErrRunAsNotFound, err.Error(), start)
	}
	if _, err := requireMutablePathInHome(owner, p.Path); err != nil {
		return errResult(env.ID, ErrPermissionDenied, err.Error(), start)
	}

	body, err := base64.StdEncoding.DecodeString(p.ContentB64)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, "bad base64", start)
	}
	mode := os.FileMode(0o644)
	if p.Mode > 0 {
		mode = os.FileMode(p.Mode)
	}

	tmp := p.Path + ".idapt-tmp"
	if err := os.WriteFile(tmp, body, mode); err != nil {
		return classifyFsError(env.ID, err, start)
	}
	_ = chownPath(tmp, owner)
	if err := os.Rename(tmp, p.Path); err != nil {
		_ = os.Remove(tmp)
		return classifyFsError(env.ID, err, start)
	}
	_ = chownPath(p.Path, owner)

	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: json.RawMessage("{}")}
}

func runFileDelete(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p FileDeletePayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !filepath.IsAbs(p.Path) {
		return errResult(env.ID, ErrInvalidPayload, "absolute path required", start)
	}
	owner, err := resolveRunAsOwner(env.RunAs)
	if err != nil {
		return errResult(env.ID, ErrRunAsNotFound, err.Error(), start)
	}
	resolvedPath, err := requireMutablePathInHome(owner, p.Path)
	if err != nil {
		return errResult(env.ID, ErrPermissionDenied, err.Error(), start)
	}
	if samePath(resolvedPath, owner.HomeDir) {
		return errResult(env.ID, ErrPermissionDenied, "refusing to delete user home", start)
	}

	if p.Recursive {
		err = os.RemoveAll(p.Path)
	} else {
		err = os.Remove(p.Path)
	}
	if err != nil {
		return classifyFsError(env.ID, err, start)
	}
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: json.RawMessage("{}")}
}

func runFileList(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p FilePathPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !filepath.IsAbs(p.Path) {
		return errResult(env.ID, ErrInvalidPayload, "absolute path required", start)
	}

	entries, err := os.ReadDir(p.Path)
	if err != nil {
		return classifyFsError(env.ID, err, start)
	}

	type remoteEntry struct {
		Name         string `json:"name"`
		Path         string `json:"path"`
		IsDirectory  bool   `json:"isDirectory"`
		IsSymlink    bool   `json:"isSymlink"`
		Size         int64  `json:"size"`
		ModifiedAtMs int64  `json:"modifiedAtMs"`
		Permissions  string `json:"permissions"`
		Owner        string `json:"owner"`
		Group        string `json:"group"`
	}
	out := struct {
		Entries []remoteEntry `json:"entries"`
		HomeDir string        `json:"homeDir"`
	}{}
	homeDir, _ := os.UserHomeDir()
	out.HomeDir = homeDir

	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		out.Entries = append(out.Entries, remoteEntry{
			Name:         e.Name(),
			Path:         filepath.Join(p.Path, e.Name()),
			IsDirectory:  e.IsDir(),
			IsSymlink:    info.Mode()&os.ModeSymlink != 0,
			Size:         info.Size(),
			ModifiedAtMs: info.ModTime().UnixMilli(),
			Permissions:  info.Mode().Perm().String(),
			Owner:        "",
			Group:        "",
		})
	}
	dataBytes, _ := json.Marshal(out)
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runFileStat(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p FilePathPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	info, err := os.Lstat(p.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			data, _ := json.Marshal(map[string]any{"entry": nil})
			return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: data}
		}
		return classifyFsError(env.ID, err, start)
	}

	type remoteEntry struct {
		Name         string `json:"name"`
		Path         string `json:"path"`
		IsDirectory  bool   `json:"isDirectory"`
		IsSymlink    bool   `json:"isSymlink"`
		Size         int64  `json:"size"`
		ModifiedAtMs int64  `json:"modifiedAtMs"`
		Permissions  string `json:"permissions"`
		Owner        string `json:"owner"`
		Group        string `json:"group"`
	}
	entry := remoteEntry{
		Name:         filepath.Base(p.Path),
		Path:         p.Path,
		IsDirectory:  info.IsDir(),
		IsSymlink:    info.Mode()&os.ModeSymlink != 0,
		Size:         info.Size(),
		ModifiedAtMs: info.ModTime().UnixMilli(),
		Permissions:  info.Mode().Perm().String(),
	}
	dataBytes, _ := json.Marshal(map[string]any{"entry": entry})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runFileMkdir(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p FileMkdirPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !filepath.IsAbs(p.Path) {
		return errResult(env.ID, ErrInvalidPayload, "absolute path required", start)
	}
	owner, err := resolveRunAsOwner(env.RunAs)
	if err != nil {
		return errResult(env.ID, ErrRunAsNotFound, err.Error(), start)
	}
	if _, err := requireMutablePathInHome(owner, p.Path); err != nil {
		return errResult(env.ID, ErrPermissionDenied, err.Error(), start)
	}
	mode := os.FileMode(0o755)
	if p.Mode > 0 {
		mode = os.FileMode(p.Mode)
	}
	if p.Parents {
		err = os.MkdirAll(p.Path, mode)
	} else {
		err = os.Mkdir(p.Path, mode)
	}
	if err != nil {
		return classifyFsError(env.ID, err, start)
	}
	_ = chownPathAndParents(owner, p.Path)
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: json.RawMessage("{}")}
}

func runFileMove(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p FileMovePayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !filepath.IsAbs(p.From) || !filepath.IsAbs(p.To) {
		return errResult(env.ID, ErrInvalidPayload, "absolute paths required", start)
	}
	owner, err := resolveRunAsOwner(env.RunAs)
	if err != nil {
		return errResult(env.ID, ErrRunAsNotFound, err.Error(), start)
	}
	resolvedFrom, err := requireMutablePathInHome(owner, p.From)
	if err != nil {
		return errResult(env.ID, ErrPermissionDenied, err.Error(), start)
	}
	resolvedTo, err := requireMutablePathInHome(owner, p.To)
	if err != nil {
		return errResult(env.ID, ErrPermissionDenied, err.Error(), start)
	}
	if samePath(resolvedFrom, owner.HomeDir) || samePath(resolvedTo, owner.HomeDir) {
		return errResult(env.ID, ErrPermissionDenied, "refusing to move user home", start)
	}
	if err := os.Rename(p.From, p.To); err != nil {
		return classifyFsError(env.ID, err, start)
	}
	_ = chownPath(p.To, owner)
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: json.RawMessage("{}")}
}

func classifyFsError(id string, err error, start time.Time) Result {
	if err == nil {
		return Result{ID: id, OK: true, DurationMs: time.Since(start).Milliseconds()}
	}
	if errors.Is(err, fs.ErrNotExist) {
		return errResult(id, ErrPathNotFound, err.Error(), start)
	}
	if errors.Is(err, fs.ErrPermission) {
		return errResult(id, ErrPermissionDenied, err.Error(), start)
	}
	if errors.Is(err, syscall.EACCES) {
		return errResult(id, ErrPermissionDenied, err.Error(), start)
	}
	return errResult(id, ErrIO, err.Error(), start)
}

type runAsOwner struct {
	HomeDir string
	UID     int
	GID     int
}

func resolveRunAsOwner(runAs string) (*runAsOwner, error) {
	u, err := user.Lookup(runAs)
	if err != nil {
		return nil, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return nil, err
	}
	home := filepath.Clean(u.HomeDir)
	if realHome, err := filepath.EvalSymlinks(home); err == nil {
		home = realHome
	}
	if home == "." || home == string(filepath.Separator) {
		return nil, errors.New("invalid user home")
	}
	return &runAsOwner{HomeDir: home, UID: uid, GID: gid}, nil
}

func requireMutablePathInHome(owner *runAsOwner, target string) (string, error) {
	resolved, err := resolvePathForPolicy(target)
	if err != nil {
		return "", err
	}
	if !pathInside(owner.HomeDir, resolved) {
		return "", fmt.Errorf("path must be under %s", owner.HomeDir)
	}
	return resolved, nil
}

func resolvePathForPolicy(target string) (string, error) {
	clean := filepath.Clean(target)
	existing := clean
	missing := []string{}
	for {
		if _, err := os.Lstat(existing); err == nil {
			resolved, err := filepath.EvalSymlinks(existing)
			if err != nil {
				return "", err
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", fs.ErrNotExist
		}
		missing = append(missing, filepath.Base(existing))
		existing = parent
	}
}

func pathInside(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func chownPath(path string, owner *runAsOwner) error {
	if os.Geteuid() != 0 {
		return nil
	}
	return os.Chown(path, owner.UID, owner.GID)
}

func chownPathAndParents(owner *runAsOwner, path string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	resolved, err := resolvePathForPolicy(path)
	if err != nil {
		return err
	}
	if !pathInside(owner.HomeDir, resolved) {
		return fmt.Errorf("path must be under %s", owner.HomeDir)
	}
	rel, err := filepath.Rel(owner.HomeDir, resolved)
	if err != nil {
		return err
	}
	current := owner.HomeDir
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		if err := os.Chown(current, owner.UID, owner.GID); err != nil {
			return err
		}
	}
	return nil
}
