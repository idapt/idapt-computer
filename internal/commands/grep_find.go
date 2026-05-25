package commands

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	re, err := regexp.Compile(p.Pattern)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, "bad regex: "+err.Error(), start)
	}

	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}

	type match struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	matches := []match{}
	truncated := false

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if !p.Recursive && path != p.Path {
				return filepath.SkipDir
			}
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(body), "\n") {
			if re.MatchString(line) {
				if len(matches) >= limit {
					truncated = true
					return filepath.SkipAll
				}
				matches = append(matches, match{
					Path: path,
					Line: i + 1,
					Text: line,
				})
			}
		}
		return nil
	}
	if p.Recursive {
		_ = filepath.WalkDir(p.Path, walkFn)
	} else {
		_ = walkFn(p.Path, fileEntry{path: p.Path}, nil)
	}

	dataBytes, _ := json.Marshal(map[string]any{
		"matches":   matches,
		"truncated": truncated,
	})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
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
	limit := p.Limit
	if limit <= 0 {
		limit = 100
	}

	paths := []string{}
	truncated := false

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if !p.Recursive && path != p.Path {
				return filepath.SkipDir
			}
			return nil
		}
		matched, _ := filepath.Match(p.Glob, filepath.Base(path))
		if matched {
			if len(paths) >= limit {
				truncated = true
				return filepath.SkipAll
			}
			paths = append(paths, path)
		}
		return nil
	}
	if p.Recursive {
		_ = filepath.WalkDir(p.Path, walkFn)
	} else {
		entries, _ := os.ReadDir(p.Path)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(p.Path, e.Name())
			matched, _ := filepath.Match(p.Glob, e.Name())
			if matched {
				if len(paths) >= limit {
					truncated = true
					break
				}
				paths = append(paths, path)
			}
		}
	}

	dataBytes, _ := json.Marshal(map[string]any{
		"paths":     paths,
		"truncated": truncated,
	})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

type fileEntry struct {
	path string
}

func (e fileEntry) Name() string               { return filepath.Base(e.path) }
func (e fileEntry) IsDir() bool                { return false }
func (e fileEntry) Type() fs.FileMode          { return 0 }
func (e fileEntry) Info() (fs.FileInfo, error) { return os.Stat(e.path) }
