package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/idapt/idapt-computer/internal/progress"
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
	computerID     string
	client         *http.Client

	ProgressBar func(total int64) *progress.Bar
}

func New(appURL, currentVersion, binaryPath string) *Updater {
	return &Updater{
		appURL:         appURL,
		currentVersion: currentVersion,
		binaryPath:     binaryPath,
		client:         &http.Client{Timeout: 30 * time.Second},
	}
}

func (u *Updater) SetComputerID(id string) { u.computerID = id }

func NormalizeVersion(v string) string {
	return strings.TrimPrefix(v, "computer-v")
}

type parsedVersion struct {
	major, minor, patch int
	preRelease          string // "" means "this is a release"
}

func parseVersion(v string) (parsedVersion, bool) {
	base, pre, _ := strings.Cut(v, "-")
	parts := strings.Split(base, ".")
	if len(parts) != 3 {
		return parsedVersion{}, false
	}
	nums := make([]int, 3)
	for i, p := range parts {
		if p == "" {
			return parsedVersion{}, false
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return parsedVersion{}, false
		}
		nums[i] = n
	}
	return parsedVersion{
		major:      nums[0],
		minor:      nums[1],
		patch:      nums[2],
		preRelease: pre,
	}, true
}

func CompareVersions(a, b string) int {
	pa, okA := parseVersion(a)
	pb, okB := parseVersion(b)
	if !okA || !okB {
		return 0
	}
	switch {
	case pa.major != pb.major:
		return cmpInt(pa.major, pb.major)
	case pa.minor != pb.minor:
		return cmpInt(pa.minor, pb.minor)
	case pa.patch != pb.patch:
		return cmpInt(pa.patch, pb.patch)
	}
	switch {
	case pa.preRelease == "" && pb.preRelease == "":
		return 0
	case pa.preRelease == "" && pb.preRelease != "":
		return 1
	case pa.preRelease != "" && pb.preRelease == "":
		return -1
	default:
		return 0
	}
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func IsValidVersionFormat(v string) bool {
	_, ok := parseVersion(v)
	return ok
}

func (u *Updater) Check() (*VersionInfo, error) {
	raw, hexSig, err := u.fetchManifest()
	if err != nil {
		return nil, err
	}
	m, err := ParseSignedManifest(raw, hexSig, time.Now())
	if err != nil {
		return nil, err
	}

	if st := LoadCheckState(); m.Counter < st.LastManifestCounter {
		return nil, fmt.Errorf(
			"manifest counter %d older than last accepted %d (rollback attempt?)",
			m.Counter, st.LastManifestCounter)
	}

	target, err := m.EffectiveTarget(u.computerID)
	if err != nil {
		return nil, err
	}

	_ = SaveManifestCounter(m.Counter)

	localVer := NormalizeVersion(u.currentVersion)
	remoteVer := NormalizeVersion(target.Version)

	if localVer == remoteVer {
		return nil, nil // up to date
	}
	if !IsValidVersionFormat(remoteVer) {
		log.Printf("update: manifest target %q is not a valid CLI version (expected computer-vX.Y.Z)", target.Version)
		return nil, nil
	}
	if CompareVersions(remoteVer, localVer) <= 0 {
		return nil, nil // never downgrade
	}

	return &VersionInfo{
		Version:      target.Version,
		DownloadURL:  target.BinaryURL,
		SHA256:       target.SHA256,
		Signature:    target.SignatureHex,
		SignatureURL: target.SignatureURL,
		MinVersion:   target.MinVersion,
	}, nil
}

func (u *Updater) fetchManifest() (raw []byte, hexSig string, err error) {
	base := strings.TrimRight(u.appURL, "/") + "/api/computer/manifest.json"
	raw, err = u.getBytes(base)
	if err != nil {
		return nil, "", fmt.Errorf("fetch manifest: %w", err)
	}
	sigBytes, err := u.getBytes(base + ".sig")
	if err != nil {
		return nil, "", fmt.Errorf("fetch manifest signature: %w", err)
	}
	return raw, strings.TrimSpace(string(sigBytes)), nil
}

func (u *Updater) getBytes(url string) ([]byte, error) {
	resp, err := u.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
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

	var src io.Reader = resp.Body
	if u.ProgressBar != nil {
		bar := u.ProgressBar(resp.ContentLength)
		src = bar.ProxyReader(resp.Body)
		defer bar.Done()
	}

	if _, err := io.Copy(writer, src); err != nil {
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
