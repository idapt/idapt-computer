package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

type VersionInfo struct {
	Version      string `json:"version"`
	DownloadURL  string `json:"downloadUrl"`
	SHA256       string `json:"sha256"`
	Signature    string `json:"signature"`
	SignatureURL string `json:"signatureUrl"`
	SigURL       string `json:"sigUrl"`
	MinVersion   string `json:"minVersion"`
}

type Updater struct {
	appURL         string
	currentVersion string
	binaryPath     string
	client         *http.Client
}

func New(appURL, currentVersion, binaryPath string) *Updater {
	return &Updater{
		appURL:         appURL,
		currentVersion: currentVersion,
		binaryPath:     binaryPath,
		client:         &http.Client{Timeout: 30 * time.Second},
	}
}

func NormalizeVersion(v string) string {
	return strings.TrimPrefix(v, "cli-v")
}

func CompareVersions(a, b string) int {
	aParts := strings.SplitN(a, ".", 4)
	bParts := strings.SplitN(b, ".", 4)

	limit := 3
	if len(aParts) < limit {
		limit = len(aParts)
	}
	if len(bParts) < limit {
		limit = len(bParts)
	}

	for i := 0; i < limit; i++ {
		if aParts[i] < bParts[i] {
			return -1
		}
		if aParts[i] > bParts[i] {
			return 1
		}
	}

	if len(aParts) > 3 && len(bParts) > 3 && aParts[3] != bParts[3] {
		return 1
	}

	return 0
}

func IsValidVersionFormat(v string) bool {
	parts := strings.SplitN(v, ".", 4)
	if len(parts) < 3 {
		return false
	}
	if len(parts[0]) != 4 {
		return false
	}
	return true
}

func (u *Updater) Check() (*VersionInfo, error) {
	url := u.appURL + "/api/cli/version"
	resp, err := u.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("check version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("version endpoint returned %d", resp.StatusCode)
	}

	var info VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("parse version response: %w", err)
	}

	localVer := NormalizeVersion(u.currentVersion)
	remoteVer := NormalizeVersion(info.Version)

	if localVer == remoteVer {
		return nil, nil // up to date
	}

	if !IsValidVersionFormat(remoteVer) {
		log.Printf("update: server returned version %q which is not a valid CLI version (expected YYYY.MM.DD.HASH)", info.Version)
		return nil, nil
	}

	if CompareVersions(remoteVer, localVer) <= 0 {
		return nil, nil
	}

	return &info, nil
}

func (u *Updater) Apply(info *VersionInfo) error {
	if info.DownloadURL == "" {
		return fmt.Errorf("no download URL")
	}
	if !isSecureUpdateURL(info.DownloadURL) {
		return fmt.Errorf("download URL must be https")
	}
	if info.SHA256 == "" {
		return fmt.Errorf("missing SHA256")
	}

	tmpPath, sameDir, err := openDownloadTarget(u.binaryPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	resp, err := u.client.Get(info.DownloadURL)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		os.Remove(tmpPath)
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	tmpFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("open temp file: %w", err)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write binary: %w", err)
	}
	tmpFile.Close()

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != info.SHA256 {
		os.Remove(tmpPath)
		return fmt.Errorf("SHA256 mismatch: expected %s, got %s", info.SHA256, actualHash)
	}

	signature, err := u.resolveSignature(info)
	if err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := VerifyHexSig(tmpPath, signature); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("signature verification failed: %w", err)
	}

	if err := replaceBinary(tmpPath, u.binaryPath, sameDir); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace binary: %w", err)
	}

	log.Printf("update: installed version %s (SHA256: %s)", info.Version, actualHash)
	return nil
}

func openDownloadTarget(binaryPath string) (path string, sameDir bool, err error) {
	preferred := binaryPath + ".new"
	f, err := os.OpenFile(preferred, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err == nil {
		f.Close()
		return preferred, true, nil
	}
	if !errors.Is(err, os.ErrPermission) {
		return "", false, err
	}
	tmp, err := os.CreateTemp("", "idapt-update-*")
	if err != nil {
		return "", false, err
	}
	name := tmp.Name()
	tmp.Close()
	if err := os.Chmod(name, 0o755); err != nil {
		os.Remove(name)
		return "", false, err
	}
	return name, false, nil
}

func replaceBinary(tmpPath, binaryPath string, sameDir bool) error {
	if sameDir {
		return os.Rename(tmpPath, binaryPath)
	}
	if os.Geteuid() == 0 {
		return moveCrossFS(tmpPath, binaryPath)
	}
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("destination %s requires elevated permissions but sudo is not available; re-run as root", binaryPath)
	}
	cmd := exec.Command(sudo, "install", "-m", "0755", tmpPath, binaryPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo install %s: %w", binaryPath, err)
	}
	return nil
}

func moveCrossFS(src, dst string) error {
	staged := dst + ".new"
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(staged, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(staged)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(staged)
		return err
	}
	if err := os.Rename(staged, dst); err != nil {
		os.Remove(staged)
		return err
	}
	_ = os.Remove(src)
	return nil
}

func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".idapt-write-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

func (u *Updater) resolveSignature(info *VersionInfo) (string, error) {
	if strings.TrimSpace(info.Signature) != "" {
		return strings.TrimSpace(info.Signature), nil
	}

	sigURL := info.SignatureURL
	if sigURL == "" {
		sigURL = info.SigURL
	}
	if sigURL == "" {
		sigURL = info.DownloadURL + ".sig"
	}
	if !isSecureUpdateURL(sigURL) {
		return "", fmt.Errorf("signature URL must be https")
	}

	resp, err := u.client.Get(sigURL)
	if err != nil {
		return "", fmt.Errorf("download signature: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("signature download returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", fmt.Errorf("read signature: %w", err)
	}
	signature := strings.TrimSpace(string(body))
	if signature != "" {
		if decoded, err := hex.DecodeString(signature); err == nil && len(decoded) == ed25519.SignatureSize {
			return signature, nil
		}
	}
	if len(body) == ed25519.SignatureSize {
		return hex.EncodeToString(body), nil
	}
	return "", fmt.Errorf("missing or invalid signature")
}

func isSecureUpdateURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	if parsed.Scheme != "http" {
		return false
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
