package commands

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const fsOpSubcommand = "__fsop"

func FsOpSubcommand() string { return fsOpSubcommand }

type fsOpKind string

const (
	fsOpRead fsOpKind = "read"
	fsOpList fsOpKind = "list"
	fsOpStat fsOpKind = "stat"
	fsOpGrep fsOpKind = "grep"
	fsOpFind fsOpKind = "find"
)

type fsOpSpec struct {
	Kind      fsOpKind `json:"kind"`
	Path      string   `json:"path"`
	Offset    int      `json:"offset,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	FromEnd   bool     `json:"fromEnd,omitempty"`
	Pattern   string   `json:"pattern,omitempty"`
	Glob      string   `json:"glob,omitempty"`
	Recursive bool     `json:"recursive,omitempty"`
}

type fsOpResult struct {
	JSON json.RawMessage `json:"json,omitempty"`
	ErrCode string `json:"errCode,omitempty"`
	ErrMsg  string `json:"errMsg,omitempty"`
}

func encodeFsOpSpec(spec fsOpSpec) ([]byte, error) { return json.Marshal(spec) }
func decodeFsOpResult(b []byte) (fsOpResult, error) {
	var r fsOpResult
	if err := json.Unmarshal(b, &r); err != nil {
		return fsOpResult{}, err
	}
	return r, nil
}

func RunFsOpChild(specJSON string) {
	var spec fsOpSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		writeFsOpResult(fsOpResult{ErrCode: ErrInvalidPayload, ErrMsg: err.Error()})
		return
	}
	res, _ := runFsOpInProcess(spec)
	writeFsOpResult(res)
}

func writeFsOpResult(r fsOpResult) {
	b, err := json.Marshal(r)
	if err != nil {
		os.Stderr.WriteString(err.Error())
		os.Exit(1)
	}
	os.Stdout.Write(b)
}

func runFsOpInProcess(spec fsOpSpec) (fsOpResult, error) {
	switch spec.Kind {
	case fsOpRead:
		return fsOpDoRead(spec)
	case fsOpList:
		return fsOpDoList(spec)
	case fsOpStat:
		return fsOpDoStat(spec)
	case fsOpGrep:
		return fsOpDoGrep(spec)
	case fsOpFind:
		return fsOpDoFind(spec)
	default:
		return fsOpResult{ErrCode: ErrInvalidPayload, ErrMsg: "unknown fsop kind"}, nil
	}
}

func fsOpErr(err error) fsOpResult {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fsOpResult{ErrCode: ErrPathNotFound, ErrMsg: err.Error()}
	case errors.Is(err, fs.ErrPermission):
		return fsOpResult{ErrCode: ErrPermissionDenied, ErrMsg: err.Error()}
	default:
		return fsOpResult{ErrCode: ErrIO, ErrMsg: err.Error()}
	}
}

func fsOpDoRead(spec fsOpSpec) (fsOpResult, error) {
	info, err := os.Stat(spec.Path)
	if err != nil {
		return fsOpErr(err), nil
	}
	if info.IsDir() {
		return fsOpResult{ErrCode: ErrInvalidPayload, ErrMsg: "is a directory"}, nil
	}
	limit := spec.Limit
	if limit <= 0 || limit > MaxOutputBytes {
		limit = MaxOutputBytes
	}
	offset := spec.Offset
	if spec.FromEnd {
		offset = int(info.Size()) - limit
		if offset < 0 {
			offset = 0
		}
	}
	f, err := os.Open(spec.Path)
	if err != nil {
		return fsOpErr(err), nil
	}
	defer f.Close()
	if offset > 0 {
		if _, err := f.Seek(int64(offset), 0); err != nil {
			return fsOpResult{ErrCode: ErrIO, ErrMsg: err.Error()}, nil
		}
	}
	buf := make([]byte, limit)
	n, err := f.Read(buf)
	if err != nil && err.Error() != "EOF" {
		return fsOpResult{ErrCode: ErrIO, ErrMsg: err.Error()}, nil
	}
	body := buf[:n]
	out := struct {
		ContentB64 string `json:"contentB64"`
		TotalBytes int    `json:"totalBytes"`
		HasMore    bool   `json:"hasMore"`
	}{
		ContentB64: base64.StdEncoding.EncodeToString(body),
		TotalBytes: int(info.Size()),
		HasMore:    offset+n < int(info.Size()),
	}
	data, _ := json.Marshal(out)
	return fsOpResult{JSON: data}, nil
}

type remoteFsEntry struct {
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

func fsOpDoList(spec fsOpSpec) (fsOpResult, error) {
	entries, err := os.ReadDir(spec.Path)
	if err != nil {
		return fsOpErr(err), nil
	}
	out := struct {
		Entries []remoteFsEntry `json:"entries"`
		HomeDir string          `json:"homeDir"`
	}{}
	out.HomeDir, _ = os.UserHomeDir()
	for _, e := range entries {
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		out.Entries = append(out.Entries, remoteFsEntry{
			Name:         e.Name(),
			Path:         filepath.Join(spec.Path, e.Name()),
			IsDirectory:  e.IsDir(),
			IsSymlink:    info.Mode()&os.ModeSymlink != 0,
			Size:         info.Size(),
			ModifiedAtMs: info.ModTime().UnixMilli(),
			Permissions:  info.Mode().Perm().String(),
		})
	}
	data, _ := json.Marshal(out)
	return fsOpResult{JSON: data}, nil
}

func fsOpDoStat(spec fsOpSpec) (fsOpResult, error) {
	info, err := os.Lstat(spec.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			data, _ := json.Marshal(map[string]any{"entry": nil})
			return fsOpResult{JSON: data}, nil
		}
		return fsOpErr(err), nil
	}
	entry := remoteFsEntry{
		Name:         filepath.Base(spec.Path),
		Path:         spec.Path,
		IsDirectory:  info.IsDir(),
		IsSymlink:    info.Mode()&os.ModeSymlink != 0,
		Size:         info.Size(),
		ModifiedAtMs: info.ModTime().UnixMilli(),
		Permissions:  info.Mode().Perm().String(),
	}
	data, _ := json.Marshal(map[string]any{"entry": entry})
	return fsOpResult{JSON: data}, nil
}

func fsOpDoGrep(spec fsOpSpec) (fsOpResult, error) {
	re, err := regexp.Compile(spec.Pattern)
	if err != nil {
		return fsOpResult{ErrCode: ErrInvalidPayload, ErrMsg: "bad regex: " + err.Error()}, nil
	}
	limit := spec.Limit
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
	walkFn := func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			if !spec.Recursive && path != spec.Path {
				return filepath.SkipDir
			}
			return nil
		}
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		for i, line := range strings.Split(string(body), "\n") {
			if re.MatchString(line) {
				if len(matches) >= limit {
					truncated = true
					return filepath.SkipAll
				}
				matches = append(matches, match{Path: path, Line: i + 1, Text: line})
			}
		}
		return nil
	}
	if spec.Recursive {
		_ = filepath.WalkDir(spec.Path, walkFn)
	} else {
		_ = walkFn(spec.Path, fileEntry{path: spec.Path}, nil)
	}
	data, _ := json.Marshal(map[string]any{"matches": matches, "truncated": truncated})
	return fsOpResult{JSON: data}, nil
}

func fsOpDoFind(spec fsOpSpec) (fsOpResult, error) {
	limit := spec.Limit
	if limit <= 0 {
		limit = 100
	}
	paths := []string{}
	truncated := false
	walkFn := func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			if !spec.Recursive && path != spec.Path {
				return filepath.SkipDir
			}
			return nil
		}
		if matched, _ := filepath.Match(spec.Glob, filepath.Base(path)); matched {
			if len(paths) >= limit {
				truncated = true
				return filepath.SkipAll
			}
			paths = append(paths, path)
		}
		return nil
	}
	if spec.Recursive {
		_ = filepath.WalkDir(spec.Path, walkFn)
	} else {
		entries, _ := os.ReadDir(spec.Path)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if matched, _ := filepath.Match(spec.Glob, e.Name()); matched {
				if len(paths) >= limit {
					truncated = true
					break
				}
				paths = append(paths, filepath.Join(spec.Path, e.Name()))
			}
		}
	}
	data, _ := json.Marshal(map[string]any{"paths": paths, "truncated": truncated})
	return fsOpResult{JSON: data}, nil
}
