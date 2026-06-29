package credential

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/idapt/idapt-computer/internal/idaptpaths"
)

type Credentials struct {
	APIKey string `json:"apiKey,omitempty"`

	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresAt int64 `json:"expiresAt,omitempty"`
}

func (c Credentials) HasOAuth() bool {
	return c.RefreshToken != ""
}

func DefaultPath() (string, error) {
	dir, err := idaptpaths.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

func Load(path string) (Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Credentials{}, nil
		}
		return Credentials{}, fmt.Errorf("reading credentials: %w", err)
	}
	if len(data) == 0 {
		return Credentials{}, nil
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credentials{}, fmt.Errorf("parsing credentials: %w", err)
	}
	return creds, nil
}

func Save(path string, creds Credentials) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("chmod credentials %s: %w", path, err)
	}
	return nil
}

func Clear(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing credentials: %w", err)
	}
	return nil
}
