package commands

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
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

	return dispatchFsRead(ctx, env, fsOpSpec{
		Kind:    fsOpRead,
		Path:    p.Path,
		Offset:  p.Offset,
		Limit:   p.Limit,
		FromEnd: p.FromEnd,
	}, start)
}

func dispatchFsRead(ctx context.Context, env *Envelope, spec fsOpSpec, start time.Time) Result {
	owner, err := resolveRunAsOwner(env.RunAs)
	if err != nil {
		return errResult(env.ID, ErrRunAsNotFound, err.Error(), start)
	}
	res, err := runFsOpAsUser(ctx, owner, env.RunAs, spec)
	if err != nil {
		return errResult(env.ID, ErrInternal, err.Error(), start)
	}
	if res.ErrCode != "" {
		return errResult(env.ID, res.ErrCode, res.ErrMsg, start)
	}
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: res.JSON}
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

	tmp := p.Path + ".idapt-tmp-" + randSuffix()
	f, err := openConfined(owner, tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return classifyFsError(env.ID, err, start)
	}
	if _, werr := f.Write(body); werr != nil {
		_ = f.Close()
		_ = removeConfined(owner, tmp, false)
		return classifyFsError(env.ID, werr, start)
	}
	if os.Geteuid() == 0 {
		_ = f.Chown(owner.UID, owner.GID)
	}
	if cerr := f.Close(); cerr != nil {
		_ = removeConfined(owner, tmp, false)
		return classifyFsError(env.ID, cerr, start)
	}
	if err := renameConfined(owner, tmp, p.Path); err != nil {
		_ = removeConfined(owner, tmp, false)
		return classifyFsError(env.ID, err, start)
	}
	_ = chownConfinedNoFollow(owner, p.Path)

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

	if err := removeConfined(owner, p.Path, p.Recursive); err != nil {
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

	return dispatchFsRead(ctx, env, fsOpSpec{Kind: fsOpList, Path: p.Path}, start)
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
	if !filepath.IsAbs(p.Path) {
		return errResult(env.ID, ErrInvalidPayload, "absolute path required", start)
	}
	return dispatchFsRead(ctx, env, fsOpSpec{Kind: fsOpStat, Path: p.Path}, start)
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
		err = mkdirAllConfined(owner, p.Path, mode)
	} else {
		err = mkdirConfined(owner, p.Path, mode)
	}
	if err != nil {
		return classifyFsError(env.ID, err, start)
	}
	if !p.Parents {
		_ = chownConfinedNoFollow(owner, p.Path)
	}
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
	if err := renameConfined(owner, p.From, p.To); err != nil {
		return classifyFsError(env.ID, err, start)
	}
	_ = chownConfinedNoFollow(owner, p.To)
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

func randSuffix() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}
