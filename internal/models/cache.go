package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const CacheTTL = 15 * time.Minute

const cacheFilename = "models-cache.json"

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".idapt", cacheFilename), nil
}

type Row struct {
	ID            string  `json:"id"`
	DisplayName   string  `json:"display_name"`
	Modality      string  `json:"modality,omitempty"` // "chat" | "audio" | "image"
	Provider      string  `json:"provider"`
	ContextWindow int     `json:"context_window"`
	InputPrice    float64 `json:"input_price"`  // USD per 1M tokens
	OutputPrice   float64 `json:"output_price"` // USD per 1M tokens
	Vision        bool    `json:"vision,omitempty"`
	Locked        bool    `json:"locked,omitempty"`
	LockedReason  string  `json:"locked_reason,omitempty"`
}

type Entry struct {
	FetchedAt time.Time `json:"fetched_at"`
	Models    []Row     `json:"models"`
}

func (e Entry) Fresh(now time.Time) bool {
	return now.Sub(e.FetchedAt) < CacheTTL
}

type cacheFile struct {
	Version int              `json:"version"`
	Entries map[string]Entry `json:"entries"`
}

const currentVersion = 2

func fingerprint(baseURL, apiKey string) string {
	h := sha256.New()
	h.Write([]byte(baseURL))
	h.Write([]byte{0})
	h.Write([]byte(apiKey))
	return hex.EncodeToString(h.Sum(nil))
}

func LoadFromCache(cachePath, baseURL, apiKey string) (Entry, bool) {
	if cachePath == "" {
		return Entry{}, false
	}
	data, err := os.ReadFile(cachePath)
	if err != nil || len(data) == 0 {
		return Entry{}, false
	}
	var cf cacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return Entry{}, false
	}
	if cf.Version > currentVersion {
		return Entry{}, false
	}
	fp := fingerprint(baseURL, apiKey)
	entry, ok := cf.Entries[fp]
	if !ok {
		return Entry{}, false
	}
	return entry, true
}

func SaveToCache(cachePath, baseURL, apiKey string, rows []Row) error {
	if cachePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0700); err != nil {
		return err
	}
	fp := fingerprint(baseURL, apiKey)
	cf := cacheFile{Version: currentVersion, Entries: map[string]Entry{}}
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &cf) // tolerate corruption — overwrite
		if cf.Entries == nil {
			cf.Entries = map[string]Entry{}
		}
		if cf.Version == 0 {
			cf.Version = currentVersion
		}
	}
	cf.Entries[fp] = Entry{FetchedAt: time.Now(), Models: rows}

	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(cachePath)
	tmp, err := os.CreateTemp(dir, ".models-cache-*.tmp")
	if err != nil {
		return os.WriteFile(cachePath, data, 0600)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, cachePath)
}

func Invalidate(cachePath, baseURL, apiKey string) error {
	if cachePath == "" {
		return nil
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var cf cacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil // corrupt — let next save overwrite
	}
	delete(cf.Entries, fingerprint(baseURL, apiKey))
	out, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(cachePath, out, 0600)
}
