package commands

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

func runFileGrep(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p FileGrepPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !filepath.IsAbs(p.Path) {
		return errResult(env.ID, ErrInvalidPayload, "absolute path required", start)
	}
	return dispatchFsRead(ctx, env, fsOpSpec{
		Kind:      fsOpGrep,
		Path:      p.Path,
		Pattern:   p.Pattern,
		Recursive: p.Recursive,
		Limit:     p.Limit,
	}, start)
}

func runFileFind(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	var p FileFindPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !filepath.IsAbs(p.Path) {
		return errResult(env.ID, ErrInvalidPayload, "absolute path required", start)
	}
	return dispatchFsRead(ctx, env, fsOpSpec{
		Kind:      fsOpFind,
		Path:      p.Path,
		Glob:      p.Glob,
		Recursive: p.Recursive,
		Limit:     p.Limit,
	}, start)
}

type fileEntry struct {
	path string
}

func (e fileEntry) Name() string               { return filepath.Base(e.path) }
func (e fileEntry) IsDir() bool                { return false }
func (e fileEntry) Type() fs.FileMode          { return 0 }
func (e fileEntry) Info() (fs.FileInfo, error) { return os.Stat(e.path) }
