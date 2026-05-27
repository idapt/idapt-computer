package commands

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/idapt/idapt-cli/internal/idaptpaths"
	"github.com/klauspost/compress/zstd"
)

const (
	localInferenceRuntime = "ollama"
	defaultOllamaBaseURL  = "http://127.0.0.1:11434/v1"
)

var localInferenceInstallLock = make(chan struct{}, 1)

type localInferenceStatusResult struct {
	Runtime   string  `json:"runtime"`
	Mode      string  `json:"mode"`
	Running   bool    `json:"running"`
	BaseURL   *string `json:"baseUrl"`
	Version   *string `json:"version"`
	GPU       *string `json:"gpu"`
	ModelsDir *string `json:"modelsDir"`
}

type localInferenceModelListResult struct {
	Models []localInferenceModel `json:"models"`
}

type localInferenceModel struct {
	Name       string  `json:"name"`
	Size       *int64  `json:"size"`
	ModifiedAt *string `json:"modifiedAt"`
}

type localInferenceChatResult struct {
	Runtime   string `json:"runtime"`
	Completed bool   `json:"completed"`
}

type localInferencePaths struct {
	Root       string
	RuntimeDir string
	Bin        string
	ModelsDir  string
	LogPath    string
	PIDPath    string
	Downloads  string
}

type localInferenceInstallResult struct {
	localInferenceStatusResult
	Installed        bool    `json:"installed"`
	AlreadyInstalled bool    `json:"alreadyInstalled"`
	Resumed          bool    `json:"resumed"`
	DownloadURL      *string `json:"downloadUrl,omitempty"`
	ArchiveBytes     *int64  `json:"archiveBytes,omitempty"`
}

type localInferenceProgressEvent struct {
	Phase               string   `json:"phase"`
	Status              string   `json:"status"`
	URL                 *string  `json:"url,omitempty"`
	Path                *string  `json:"path,omitempty"`
	TotalBytes          *int64   `json:"totalBytes,omitempty"`
	DownloadedBytes     *int64   `json:"downloadedBytes,omitempty"`
	ExistingBytes       *int64   `json:"existingBytes,omitempty"`
	Percent             *float64 `json:"percent,omitempty"`
	SpeedBytesPerSecond *float64 `json:"speedBytesPerSecond,omitempty"`
	ETASeconds          *float64 `json:"etaSeconds,omitempty"`
	Resumed             bool     `json:"resumed,omitempty"`
}

type localInferenceDownloadMetadata struct {
	SourceURL    string `json:"sourceUrl"`
	FinalURL     string `json:"finalUrl"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	TotalBytes   int64  `json:"totalBytes,omitempty"`
	AcceptRanges bool   `json:"acceptRanges,omitempty"`
}

type localInferenceDownloadManifest struct {
	localInferenceDownloadMetadata
	DownloadedBytes int64  `json:"downloadedBytes,omitempty"`
	UpdatedAt       string `json:"updatedAt"`
}

type localInferenceDownloadResult struct {
	Metadata          localInferenceDownloadMetadata
	Bytes             int64
	Resumed           bool
	AlreadyDownloaded bool
}

func runLocalInferenceStatus(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	payload, err := parseLocalRuntimePayload(env)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	status := buildLocalInferenceStatus(ctx, payload)
	dataBytes, _ := json.Marshal(status)
	return Result{
		ID:         env.ID,
		OK:         true,
		DurationMs: time.Since(start).Milliseconds(),
		Data:       dataBytes,
	}
}

func runLocalInferenceRuntimeInstall(ctx context.Context, env *Envelope, poster ChunkPoster) Result {
	start := time.Now()
	payload, err := parseLocalRuntimePayload(env)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	offset := int64(0)
	emit := func(event localInferenceProgressEvent) error {
		return postLocalInferenceProgress(ctx, poster, env.ID, &offset, event)
	}
	if err := acquireLocalInferenceInstallLock(ctx, emit); err != nil {
		return errResult(env.ID, ErrCancelled, "command cancelled", start)
	}
	defer releaseLocalInferenceInstallLock()

	paths, err := defaultLocalInferencePaths()
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	if err := os.MkdirAll(paths.Downloads, 0700); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	if _, err := os.Stat(paths.Bin); err == nil {
		_ = emit(localInferenceProgressEvent{
			Phase:  "ready",
			Status: "managed Ollama runtime is already installed",
			Path:   stringPtr(paths.Bin),
		})
		status := buildLocalInferenceStatus(ctx, payload)
		status.Mode = "managed"
		dataBytes, _ := json.Marshal(localInferenceInstallResult{
			localInferenceStatusResult: status,
			Installed:                  true,
			AlreadyInstalled:           true,
		})
		return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
	}

	downloadURL, err := ollamaDownloadURL(payload.Version)
	if err != nil {
		return errResult(env.ID, ErrUnsupportedKind, err.Error(), start)
	}
	_ = emit(localInferenceProgressEvent{
		Phase:  "resolving",
		Status: "resolving Ollama runtime download",
		URL:    stringPtr(downloadURL),
	})
	archivePath := filepath.Join(paths.Downloads, "ollama.tar.zst")
	downloadResult, err := downloadFileResumable(ctx, downloadURL, archivePath, emit)
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	stagingDir := paths.RuntimeDir + ".staging"
	if err := os.RemoveAll(stagingDir); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	if err := os.MkdirAll(stagingDir, 0700); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	_ = emit(localInferenceProgressEvent{
		Phase:  "extracting",
		Status: "extracting Ollama runtime",
		Path:   stringPtr(stagingDir),
	})
	if err := extractTarZst(ctx, archivePath, stagingDir); err != nil {
		_ = os.RemoveAll(stagingDir)
		if ctx.Err() == nil {
			_ = os.Remove(archivePath)
		}
		return errResult(env.ID, ErrIO, "extracting Ollama bundle failed: "+err.Error(), start)
	}
	stagedBin := filepath.Join(stagingDir, "bin", "ollama")
	if err := os.Chmod(stagedBin, 0755); err != nil && !os.IsNotExist(err) {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	if _, err := os.Stat(stagedBin); err != nil {
		_ = os.RemoveAll(stagingDir)
		return errResult(env.ID, ErrIO, "Ollama bundle did not contain bin/ollama", start)
	}
	if err := os.RemoveAll(paths.RuntimeDir); err != nil {
		_ = os.RemoveAll(stagingDir)
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	if err := os.Rename(stagingDir, paths.RuntimeDir); err != nil {
		_ = os.RemoveAll(stagingDir)
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	_ = emit(localInferenceProgressEvent{
		Phase:  "ready",
		Status: "managed Ollama runtime installed",
		Path:   stringPtr(paths.Bin),
	})

	status := buildLocalInferenceStatus(ctx, payload)
	status.Mode = "managed"
	archiveBytes := downloadResult.Bytes
	dataBytes, _ := json.Marshal(localInferenceInstallResult{
		localInferenceStatusResult: status,
		Installed:                  true,
		AlreadyInstalled:           false,
		Resumed:                    downloadResult.Resumed,
		DownloadURL:                stringPtr(downloadResult.Metadata.FinalURL),
		ArchiveBytes:               &archiveBytes,
	})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runLocalInferenceRuntimeStart(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	payload, err := parseLocalRuntimePayload(env)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	status := buildLocalInferenceStatus(ctx, payload)
	if status.Running {
		dataBytes, _ := json.Marshal(status)
		return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
	}

	bin, mode, err := resolveOllamaBinary(payload.Managed)
	if err != nil {
		return errResult(env.ID, ErrPathNotFound, err.Error(), start)
	}
	paths, err := defaultLocalInferencePaths()
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	rootURL, err := localInferenceRootURL(payload.BaseURL)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	u, _ := url.Parse(rootURL)
	if err := os.MkdirAll(paths.Root, 0700); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	if err := os.MkdirAll(paths.ModelsDir, 0700); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	logFile, err := os.OpenFile(paths.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	defer logFile.Close()

	cmd := exec.CommandContext(context.Background(), bin, "serve")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = localInferenceEnv(u.Host, paths)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	_ = os.WriteFile(paths.PIDPath, []byte(strconv.Itoa(pid)+"\n"), 0600)

	waitUntil := time.Now().Add(15 * time.Second)
	for time.Now().Before(waitUntil) {
		select {
		case <-ctx.Done():
			return errResult(env.ID, ErrCancelled, "command cancelled", start)
		default:
		}
		time.Sleep(300 * time.Millisecond)
		status = buildLocalInferenceStatus(ctx, payload)
		if status.Running {
			status.Mode = mode
			dataBytes, _ := json.Marshal(status)
			return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
		}
	}
	return errResult(env.ID, ErrIO, "Ollama did not become ready within 15s; check local-inference logs", start)
}

func runLocalInferenceRuntimeStop(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	payload, err := parseLocalRuntimePayload(env)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	paths, err := defaultLocalInferencePaths()
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	pidBytes, err := os.ReadFile(paths.PIDPath)
	if err == nil {
		pid, _ := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
		if pid > 0 {
			if proc, err := os.FindProcess(pid); err == nil {
				_ = syscall.Kill(-pid, syscall.SIGTERM)
				_ = proc.Signal(syscall.SIGTERM)
			}
		}
		_ = os.Remove(paths.PIDPath)
	}
	select {
	case <-ctx.Done():
	case <-time.After(800 * time.Millisecond):
	}
	status := buildLocalInferenceStatus(ctx, payload)
	dataBytes, _ := json.Marshal(status)
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runLocalInferenceRuntimeLogs(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	if _, err := parseLocalRuntimePayload(env); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	paths, err := defaultLocalInferencePaths()
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	content, err := readTail(paths.LogPath, 64*1024)
	if err != nil {
		if os.IsNotExist(err) {
			content = ""
		} else {
			return errResult(env.ID, ErrIO, err.Error(), start)
		}
	}
	dataBytes, _ := json.Marshal(map[string]string{"logs": content})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runLocalInferenceModelList(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	payload, err := parseLocalRuntimePayload(env)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	rootURL, err := localInferenceRootURL(payload.BaseURL)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rootURL+"/api/tags", nil)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	resp, err := localHTTPClient().Do(req)
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return errResult(env.ID, ErrIO, fmt.Sprintf("Ollama returned HTTP %d", resp.StatusCode), start)
	}
	var parsed struct {
		Models []struct {
			Name       string `json:"name"`
			Size       *int64 `json:"size"`
			ModifiedAt string `json:"modified_at"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10*1024*1024)).Decode(&parsed); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	out := localInferenceModelListResult{Models: make([]localInferenceModel, 0, len(parsed.Models))}
	for _, model := range parsed.Models {
		var modified *string
		if model.ModifiedAt != "" {
			modified = &model.ModifiedAt
		}
		out.Models = append(out.Models, localInferenceModel{Name: model.Name, Size: model.Size, ModifiedAt: modified})
	}
	dataBytes, _ := json.Marshal(out)
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runLocalInferenceModelPull(ctx context.Context, env *Envelope, poster ChunkPoster) Result {
	start := time.Now()
	payload, err := parseLocalModelPayload(env)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	rootURL, err := localInferenceRootURL(payload.BaseURL)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	body, _ := json.Marshal(map[string]any{"name": payload.Model, "stream": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rootURL+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := localHTTPClient().Do(req)
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return errResult(env.ID, ErrIO, fmt.Sprintf("Ollama returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg))), start)
	}
	offset := int64(0)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := append(append([]byte{}, scanner.Bytes()...), '\n')
		if err := postLocalInferenceChunk(ctx, poster, env.ID, "progress", &offset, line); err != nil {
			return errResult(env.ID, ErrIO, err.Error(), start)
		}
	}
	if err := scanner.Err(); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: []byte("{}")}
}

func runLocalInferenceModelRemove(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	payload, err := parseLocalModelPayload(env)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	rootURL, err := localInferenceRootURL(payload.BaseURL)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	body, _ := json.Marshal(map[string]any{"name": payload.Model})
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, rootURL+"/api/delete", bytes.NewReader(body))
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := localHTTPClient().Do(req)
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return errResult(env.ID, ErrIO, fmt.Sprintf("Ollama returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg))), start)
	}
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: []byte("{}")}
}

func runLocalInferenceChat(ctx context.Context, env *Envelope, poster ChunkPoster) Result {
	start := time.Now()
	payload, err := parseLocalChatPayload(env)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	openAIBaseURL, err := localInferenceOpenAIBaseURL(payload.BaseURL)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIBaseURL+"/chat/completions", bytes.NewReader(payload.Body))
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := localHTTPClient().Do(req)
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return errResult(env.ID, ErrIO, fmt.Sprintf("Ollama returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg))), start)
	}
	offset := int64(0)
	buf := make([]byte, 16*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if err := postLocalInferenceChunk(ctx, poster, env.ID, "provider-sse", &offset, buf[:n]); err != nil {
				return errResult(env.ID, ErrIO, err.Error(), start)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return errResult(env.ID, ErrIO, readErr.Error(), start)
		}
	}
	dataBytes, _ := json.Marshal(localInferenceChatResult{Runtime: localInferenceRuntime, Completed: true})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func parseLocalRuntimePayload(env *Envelope) (LocalInferenceRuntimePayload, error) {
	var payload LocalInferenceRuntimePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return payload, err
	}
	if payload.Runtime != localInferenceRuntime {
		return payload, fmt.Errorf("runtime must be %q", localInferenceRuntime)
	}
	if payload.BaseURL == "" {
		payload.BaseURL = defaultOllamaBaseURL
	}
	return payload, nil
}

func parseLocalModelPayload(env *Envelope) (LocalInferenceModelPayload, error) {
	var payload LocalInferenceModelPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return payload, err
	}
	if payload.Runtime != localInferenceRuntime {
		return payload, fmt.Errorf("runtime must be %q", localInferenceRuntime)
	}
	if strings.TrimSpace(payload.Model) == "" {
		return payload, fmt.Errorf("model required")
	}
	if payload.BaseURL == "" {
		payload.BaseURL = defaultOllamaBaseURL
	}
	return payload, nil
}

func parseLocalChatPayload(env *Envelope) (LocalInferenceChatPayload, error) {
	var payload LocalInferenceChatPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return payload, err
	}
	if payload.Runtime != localInferenceRuntime {
		return payload, fmt.Errorf("runtime must be %q", localInferenceRuntime)
	}
	if len(payload.Body) == 0 {
		return payload, fmt.Errorf("body required")
	}
	if payload.BaseURL == "" {
		payload.BaseURL = defaultOllamaBaseURL
	}
	return payload, nil
}

func buildLocalInferenceStatus(ctx context.Context, payload LocalInferenceRuntimePayload) localInferenceStatusResult {
	baseURL, err := localInferenceOpenAIBaseURL(payload.BaseURL)
	if err != nil {
		baseURL = payload.BaseURL
	}
	rootURL, _ := localInferenceRootURL(baseURL)
	paths, pathErr := defaultLocalInferencePaths()
	mode := "existing"
	var modelsDir *string
	if pathErr == nil {
		modelsDir = &paths.ModelsDir
		if _, err := os.Stat(paths.Bin); err == nil {
			mode = "managed"
		}
	}
	baseURLPtr := baseURL
	version := ollamaVersion(ctx, rootURL)
	running := version != nil || ollamaTagsReachable(ctx, rootURL)
	return localInferenceStatusResult{
		Runtime:   localInferenceRuntime,
		Mode:      mode,
		Running:   running,
		BaseURL:   &baseURLPtr,
		Version:   version,
		GPU:       detectLocalGPU(),
		ModelsDir: modelsDir,
	}
}

func ollamaVersion(ctx context.Context, rootURL string) *string {
	if rootURL == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rootURL+"/api/version", nil)
	if err != nil {
		return nil
	}
	resp, err := localHTTPClient().Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil
	}
	var parsed struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&parsed); err != nil || parsed.Version == "" {
		return nil
	}
	return &parsed.Version
}

func ollamaTagsReachable(ctx context.Context, rootURL string) bool {
	if rootURL == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rootURL+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := localHTTPClient().Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

func OllamaLoadedModelIDs(ctx context.Context) []string {
	rootURL, _ := localInferenceRootURL(defaultOllamaBaseURL)
	if rootURL == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rootURL+"/api/ps", nil)
	if err != nil {
		return nil
	}
	resp, err := localHTTPClient().Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil
	}
	var parsed struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&parsed); err != nil {
		return nil
	}
	loaded := make([]string, 0, len(parsed.Models))
	seen := make(map[string]struct{}, len(parsed.Models))
	for _, m := range parsed.Models {
		id := m.Name
		if id == "" {
			id = m.Model
		}
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		loaded = append(loaded, id)
	}
	return loaded
}

func localInferenceRootURL(raw string) (string, error) {
	if raw == "" {
		raw = defaultOllamaBaseURL
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" {
		return "", fmt.Errorf("local inference URL must use http")
	}
	if u.User != nil {
		return "", fmt.Errorf("local inference URL must not include credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("local inference URL must not include query or fragment")
	}
	if u.Port() == "" {
		return "", fmt.Errorf("local inference URL must include an explicit port")
	}
	host := u.Hostname()
	if host != "127.0.0.1" && host != "::1" {
		return "", fmt.Errorf("local inference URL must use a loopback literal")
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	if path != "" && path != "/v1" {
		return "", fmt.Errorf("local inference URL path must be / or /v1")
	}
	return (&url.URL{Scheme: "http", Host: u.Host}).String(), nil
}

func localInferenceOpenAIBaseURL(raw string) (string, error) {
	root, err := localInferenceRootURL(raw)
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(raw)
	if strings.TrimRight(u.EscapedPath(), "/") == "/v1" {
		return root + "/v1", nil
	}
	return root + "/v1", nil
}

func defaultLocalInferencePaths() (localInferencePaths, error) {
	root := os.Getenv("IDAPT_LOCAL_INFERENCE_HOME")
	if root == "" {
		dataDir, err := idaptpaths.DataDir()
		if err != nil {
			return localInferencePaths{}, err
		}
		root = filepath.Join(dataDir, "local-inference")
	}
	ollamaRoot := filepath.Join(root, "ollama")
	runtimeDir := filepath.Join(ollamaRoot, "runtime")
	return localInferencePaths{
		Root:       ollamaRoot,
		RuntimeDir: runtimeDir,
		Bin:        filepath.Join(runtimeDir, "bin", "ollama"),
		ModelsDir:  filepath.Join(ollamaRoot, "models"),
		LogPath:    filepath.Join(ollamaRoot, "ollama.log"),
		PIDPath:    filepath.Join(ollamaRoot, "ollama.pid"),
		Downloads:  filepath.Join(ollamaRoot, "downloads"),
	}, nil
}

func resolveOllamaBinary(managed bool) (string, string, error) {
	paths, err := defaultLocalInferencePaths()
	if err != nil {
		return "", "unknown", err
	}
	if managed {
		if _, err := os.Stat(paths.Bin); err == nil {
			bin, err := validateOllamaBinaryPath(paths.Bin)
			return bin, "managed", err
		}
		return "", "managed", fmt.Errorf("managed Ollama is not installed; run local-inference install first")
	}
	if configured := os.Getenv("IDAPT_OLLAMA_BINARY"); configured != "" {
		bin, err := validateOllamaBinaryPath(configured)
		return bin, "existing", err
	}
	if _, err := os.Stat(paths.Bin); err == nil {
		bin, err := validateOllamaBinaryPath(paths.Bin)
		return bin, "managed", err
	}
	if found, err := exec.LookPath("ollama"); err == nil {
		bin, err := validateOllamaBinaryPath(found)
		return bin, "existing", err
	}
	return "", "unknown", fmt.Errorf("Ollama binary not found")
}

func validateOllamaBinaryPath(bin string) (string, error) {
	if strings.ContainsRune(bin, 0) {
		return "", fmt.Errorf("Ollama binary path contains a NUL byte")
	}
	if !filepath.IsAbs(bin) {
		return "", fmt.Errorf("Ollama binary path must be absolute")
	}
	clean := filepath.Clean(bin)
	resolved, err := filepath.EvalSymlinks(clean)
	if err == nil {
		clean = resolved
	}
	info, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("Ollama binary not accessible: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("Ollama binary path is a directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		return "", fmt.Errorf("Ollama binary is not executable")
	}
	return clean, nil
}

func localInferenceEnv(host string, paths localInferencePaths) []string {
	env := os.Environ()
	env = append(env, "OLLAMA_HOST="+host)
	env = append(env, "OLLAMA_MODELS="+paths.ModelsDir)
	if paths.RuntimeDir != "" {
		libPath := filepath.Join(paths.RuntimeDir, "lib", "ollama")
		if existing := os.Getenv("LD_LIBRARY_PATH"); existing != "" {
			env = append(env, "LD_LIBRARY_PATH="+libPath+":"+existing)
		} else {
			env = append(env, "LD_LIBRARY_PATH="+libPath)
		}
	}
	return env
}

func ollamaDownloadURL(version string) (string, error) {
	if override := os.Getenv("IDAPT_OLLAMA_DOWNLOAD_URL"); override != "" {
		return override, nil
	}
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("managed Ollama install currently supports Linux only; install Ollama separately or set IDAPT_OLLAMA_BINARY")
	}
	arch := runtime.GOARCH
	switch arch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("managed Ollama install does not support architecture %s", arch)
	}
	if version != "" {
		return "", fmt.Errorf("version-pinned managed install is not supported yet; omit version to install the current Ollama Linux bundle")
	}
	return fmt.Sprintf("https://ollama.com/download/ollama-linux-%s.tar.zst", arch), nil
}

func downloadFile(ctx context.Context, rawURL string, path string) error {
	_, err := downloadFileResumable(ctx, rawURL, path, nil)
	return err
}

func extractTarZst(ctx context.Context, archivePath string, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder, err := zstd.NewReader(file)
	if err != nil {
		return err
	}
	defer decoder.Close()

	destClean, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	reader := tar.NewReader(decoder)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		target, err := safeTarTarget(destClean, header.Name)
		if err != nil {
			return err
		}
		mode := os.FileMode(header.Mode)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode.Perm()); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			perm := mode.Perm()
			if perm == 0 {
				perm = 0600
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, reader)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if err := os.Chmod(target, perm); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := ensureSafeTarLink(destClean, target, header.Linkname); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		case tar.TypeLink:
			linkTarget, err := safeTarTarget(destClean, header.Linkname)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Link(linkTarget, target); err != nil {
				return err
			}
		default:
			continue
		}
	}
	return nil
}

func safeTarTarget(destClean string, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("archive contains empty path")
	}
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) || clean == "." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || clean == ".." {
		return "", fmt.Errorf("archive contains unsafe path %q", name)
	}
	target := filepath.Join(destClean, clean)
	if !isPathWithin(target, destClean) {
		return "", fmt.Errorf("archive contains unsafe path %q", name)
	}
	return target, nil
}

func ensureSafeTarLink(destClean string, target string, linkname string) error {
	if linkname == "" {
		return fmt.Errorf("archive contains empty symlink target for %q", target)
	}
	if filepath.IsAbs(linkname) {
		return fmt.Errorf("archive contains absolute symlink target %q", linkname)
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(target), linkname))
	if !isPathWithin(resolved, destClean) {
		return fmt.Errorf("archive contains escaping symlink target %q", linkname)
	}
	return nil
}

func isPathWithin(path string, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	return cleanPath == cleanRoot || strings.HasPrefix(cleanPath, cleanRoot+string(os.PathSeparator))
}

func downloadFileResumable(ctx context.Context, rawURL string, path string, emit func(localInferenceProgressEvent) error) (localInferenceDownloadResult, error) {
	meta, err := resolveDownloadMetadata(ctx, rawURL)
	if err != nil {
		return localInferenceDownloadResult{}, err
	}
	if meta.FinalURL == "" {
		meta.FinalURL = rawURL
	}
	if emit != nil {
		_ = emit(localInferenceProgressEvent{
			Phase:      "download-metadata",
			Status:     "resolved Ollama runtime download",
			URL:        stringPtr(meta.FinalURL),
			TotalBytes: int64Ptr(meta.TotalBytes),
		})
	}

	if info, err := os.Stat(path); err == nil {
		if meta.TotalBytes <= 0 || info.Size() == meta.TotalBytes {
			if emit != nil {
				_ = emit(localInferenceProgressEvent{
					Phase:           "archive-ready",
					Status:          "using cached Ollama runtime archive",
					Path:            stringPtr(path),
					DownloadedBytes: int64Ptr(info.Size()),
					TotalBytes:      int64Ptr(meta.TotalBytes),
				})
			}
			return localInferenceDownloadResult{
				Metadata:          meta,
				Bytes:             info.Size(),
				AlreadyDownloaded: true,
			}, nil
		}
		_ = os.Remove(path)
	}

	partPath := path + ".part"
	manifestPath := partPath + ".json"
	resumeBytes := int64(0)
	if info, err := os.Stat(partPath); err == nil {
		manifest, manifestErr := readDownloadManifest(manifestPath)
		if manifestErr == nil && downloadManifestMatches(manifest, meta) {
			resumeBytes = info.Size()
		} else {
			_ = os.Remove(partPath)
			_ = os.Remove(manifestPath)
		}
	}
	if meta.TotalBytes > 0 && resumeBytes > meta.TotalBytes {
		_ = os.Remove(partPath)
		_ = os.Remove(manifestPath)
		resumeBytes = 0
	}
	if resumeBytes > 0 && emit != nil {
		_ = emit(localInferenceProgressEvent{
			Phase:         "resuming",
			Status:        "resuming partial Ollama runtime download",
			Path:          stringPtr(partPath),
			ExistingBytes: int64Ptr(resumeBytes),
			TotalBytes:    int64Ptr(meta.TotalBytes),
			Resumed:       true,
		})
	}

	result, err := performResumableDownload(ctx, meta, path, partPath, manifestPath, resumeBytes, emit)
	if err != nil {
		return localInferenceDownloadResult{}, err
	}
	return result, nil
}

func resolveDownloadMetadata(ctx context.Context, rawURL string) (localInferenceDownloadMetadata, error) {
	meta := localInferenceDownloadMetadata{SourceURL: rawURL, FinalURL: rawURL}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return meta, err
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return meta, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusForbidden {
		return meta, nil
	}
	if resp.StatusCode >= 400 {
		return meta, fmt.Errorf("download metadata returned HTTP %d", resp.StatusCode)
	}
	meta.FinalURL = resp.Request.URL.String()
	meta.ETag = resp.Header.Get("ETag")
	meta.LastModified = resp.Header.Get("Last-Modified")
	meta.TotalBytes = resp.ContentLength
	if meta.TotalBytes < 0 {
		meta.TotalBytes = 0
	}
	meta.AcceptRanges = strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes")
	return meta, nil
}

func performResumableDownload(ctx context.Context, meta localInferenceDownloadMetadata, path string, partPath string, manifestPath string, resumeBytes int64, emit func(localInferenceProgressEvent) error) (localInferenceDownloadResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, meta.SourceURL, nil)
	if err != nil {
		return localInferenceDownloadResult{}, err
	}
	if resumeBytes > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeBytes))
	}
	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return localInferenceDownloadResult{}, err
	}
	defer resp.Body.Close()

	appendMode := resumeBytes > 0 && resp.StatusCode == http.StatusPartialContent
	if resumeBytes > 0 && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && meta.TotalBytes > 0 && resumeBytes == meta.TotalBytes {
		if err := os.Rename(partPath, path); err != nil {
			return localInferenceDownloadResult{}, err
		}
		_ = os.Remove(manifestPath)
		return localInferenceDownloadResult{Metadata: meta, Bytes: resumeBytes, Resumed: true, AlreadyDownloaded: true}, nil
	}
	if resumeBytes > 0 && resp.StatusCode == http.StatusOK {
		resumeBytes = 0
		appendMode = false
	}
	if resp.StatusCode >= 400 {
		return localInferenceDownloadResult{}, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return localInferenceDownloadResult{}, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	if meta.FinalURL == "" || meta.FinalURL == meta.SourceURL {
		meta.FinalURL = resp.Request.URL.String()
	}
	if meta.ETag == "" {
		meta.ETag = resp.Header.Get("ETag")
	}
	if meta.LastModified == "" {
		meta.LastModified = resp.Header.Get("Last-Modified")
	}
	if meta.TotalBytes <= 0 && resp.ContentLength > 0 {
		meta.TotalBytes = resp.ContentLength + resumeBytes
	}
	meta.AcceptRanges = meta.AcceptRanges || strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes") || resp.StatusCode == http.StatusPartialContent

	flags := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
		_ = os.Remove(manifestPath)
	}
	file, err := os.OpenFile(partPath, flags, 0600)
	if err != nil {
		return localInferenceDownloadResult{}, err
	}
	manifest := localInferenceDownloadManifest{
		localInferenceDownloadMetadata: meta,
		DownloadedBytes:                resumeBytes,
		UpdatedAt:                      time.Now().UTC().Format(time.RFC3339),
	}
	_ = writeDownloadManifest(manifestPath, manifest)

	downloaded := resumeBytes
	start := time.Now()
	lastEmit := time.Time{}
	lastManifestWrite := time.Time{}
	buf := make([]byte, 1024*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := file.Write(buf[:n]); err != nil {
				_ = file.Close()
				return localInferenceDownloadResult{}, err
			}
			downloaded += int64(n)
			now := time.Now()
			if emit != nil && (lastEmit.IsZero() || now.Sub(lastEmit) >= time.Second || (meta.TotalBytes > 0 && downloaded >= meta.TotalBytes)) {
				if err := emit(downloadProgressEvent(meta, partPath, downloaded, resumeBytes, start)); err != nil {
					_ = file.Close()
					return localInferenceDownloadResult{}, err
				}
				lastEmit = now
			}
			if lastManifestWrite.IsZero() || now.Sub(lastManifestWrite) >= 5*time.Second {
				manifest.DownloadedBytes = downloaded
				manifest.UpdatedAt = now.UTC().Format(time.RFC3339)
				_ = writeDownloadManifest(manifestPath, manifest)
				lastManifestWrite = now
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			_ = file.Close()
			return localInferenceDownloadResult{}, readErr
		}
	}
	closeErr := file.Close()
	if closeErr != nil {
		return localInferenceDownloadResult{}, closeErr
	}
	if meta.TotalBytes > 0 && downloaded != meta.TotalBytes {
		manifest.DownloadedBytes = downloaded
		manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = writeDownloadManifest(manifestPath, manifest)
		return localInferenceDownloadResult{}, fmt.Errorf("download ended at %d bytes, expected %d", downloaded, meta.TotalBytes)
	}
	if emit != nil {
		_ = emit(downloadProgressEvent(meta, partPath, downloaded, resumeBytes, start))
	}
	if err := os.Rename(partPath, path); err != nil {
		return localInferenceDownloadResult{}, err
	}
	_ = os.Remove(manifestPath)
	return localInferenceDownloadResult{Metadata: meta, Bytes: downloaded, Resumed: appendMode}, nil
}

func downloadProgressEvent(meta localInferenceDownloadMetadata, path string, downloaded int64, resumeBytes int64, start time.Time) localInferenceProgressEvent {
	elapsed := time.Since(start).Seconds()
	speed := float64(downloaded-resumeBytes) / elapsed
	if elapsed <= 0 {
		speed = 0
	}
	var percent *float64
	var eta *float64
	if meta.TotalBytes > 0 {
		value := (float64(downloaded) / float64(meta.TotalBytes)) * 100
		percent = &value
		if speed > 0 && downloaded < meta.TotalBytes {
			etaValue := float64(meta.TotalBytes-downloaded) / speed
			eta = &etaValue
		}
	}
	return localInferenceProgressEvent{
		Phase:               "downloading",
		Status:              "downloading Ollama runtime",
		URL:                 stringPtr(meta.FinalURL),
		Path:                stringPtr(path),
		TotalBytes:          int64Ptr(meta.TotalBytes),
		DownloadedBytes:     int64Ptr(downloaded),
		ExistingBytes:       int64Ptr(resumeBytes),
		Percent:             percent,
		SpeedBytesPerSecond: &speed,
		ETASeconds:          eta,
		Resumed:             resumeBytes > 0,
	}
}

func readDownloadManifest(path string) (localInferenceDownloadManifest, error) {
	var manifest localInferenceDownloadManifest
	content, err := os.ReadFile(path)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(content, &manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func writeDownloadManifest(path string, manifest localInferenceDownloadManifest) error {
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func downloadManifestMatches(manifest localInferenceDownloadManifest, meta localInferenceDownloadMetadata) bool {
	if manifest.SourceURL != meta.SourceURL {
		return false
	}
	if meta.TotalBytes <= 0 && meta.ETag == "" && meta.LastModified == "" {
		return false
	}
	if meta.TotalBytes > 0 && manifest.TotalBytes > 0 && manifest.TotalBytes != meta.TotalBytes {
		return false
	}
	if meta.ETag != "" && manifest.ETag != "" && manifest.ETag != meta.ETag {
		return false
	}
	if meta.LastModified != "" && manifest.LastModified != "" && manifest.LastModified != meta.LastModified {
		return false
	}
	return true
}

func acquireLocalInferenceInstallLock(ctx context.Context, emit func(localInferenceProgressEvent) error) error {
	select {
	case localInferenceInstallLock <- struct{}{}:
		return nil
	default:
		if emit != nil {
			_ = emit(localInferenceProgressEvent{
				Phase:  "waiting",
				Status: "another managed Ollama install is already running",
			})
		}
	}
	select {
	case localInferenceInstallLock <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseLocalInferenceInstallLock() {
	select {
	case <-localInferenceInstallLock:
	default:
	}
}

func postLocalInferenceProgress(ctx context.Context, poster ChunkPoster, commandID string, offset *int64, event localInferenceProgressEvent) error {
	if poster == nil {
		return nil
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return postLocalInferenceChunk(ctx, poster, commandID, "progress", offset, data)
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func int64Ptr(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func localHTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Minute}
}

func postLocalInferenceChunk(ctx context.Context, poster ChunkPoster, commandID string, channel string, offset *int64, data []byte) error {
	chunkOffset := *offset
	*offset += int64(len(data))
	postCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return poster.PostChunk(postCtx, Chunk{
		ID:      commandID,
		Offset:  chunkOffset,
		Channel: channel,
		DataB64: base64.StdEncoding.EncodeToString(data),
	})
}

func readTail(path string, limit int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	var offset int64
	if info.Size() > limit {
		offset = info.Size() - limit
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return "", err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func detectLocalGPU() *string {
	if _, err := os.Stat("/dev/nvidia0"); err == nil {
		value := "nvidia"
		return &value
	}
	if _, err := os.Stat("/dev/kfd"); err == nil {
		value := "amd"
		return &value
	}
	if runtime.GOOS == "darwin" {
		value := "metal"
		return &value
	}
	return nil
}
