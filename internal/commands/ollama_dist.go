package commands
import (
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const ollamaDownloadBase = "https://ollama.com/download"

type archiveFormat int

const (
	archiveTarZst archiveFormat = iota
	archiveTarGz
	archiveZip
)

type ollamaDist struct {
	URL         string        // download URL
	ArchiveName string        // local filename (also records the format)
	Format      archiveFormat // how to extract it
	SHA256 string
}

func resolveOllamaDist(version string) (ollamaDist, error) {
	expectedHash := strings.ToLower(strings.TrimSpace(os.Getenv("IDAPT_OLLAMA_SHA256")))
	if override := os.Getenv("IDAPT_OLLAMA_DOWNLOAD_URL"); override != "" {
		return ollamaDist{
			URL:         override,
			ArchiveName: "ollama" + archiveExt(override),
			Format:      formatFromName(override),
			SHA256:      expectedHash,
		}, nil
	}
	if version != "" {
		return ollamaDist{}, fmt.Errorf(
			"version-pinned managed install is not supported yet; omit --version to install the current Ollama bundle",
		)
	}

	arch := runtime.GOARCH
	switch runtime.GOOS {
	case "linux":
		if arch != "amd64" && arch != "arm64" {
			return ollamaDist{}, unsupportedOllamaArch(arch)
		}
		name := fmt.Sprintf("ollama-linux-%s.tar.zst", arch)
		return ollamaDist{URL: ollamaDownloadBase + "/" + name, ArchiveName: name, Format: archiveTarZst, SHA256: expectedHash}, nil
	case "darwin":
		const name = "ollama-darwin.tgz"
		return ollamaDist{URL: ollamaDownloadBase + "/" + name, ArchiveName: name, Format: archiveTarGz, SHA256: expectedHash}, nil
	case "windows":
		if arch != "amd64" && arch != "arm64" {
			return ollamaDist{}, unsupportedOllamaArch(arch)
		}
		name := fmt.Sprintf("ollama-windows-%s.zip", arch)
		return ollamaDist{URL: ollamaDownloadBase + "/" + name, ArchiveName: name, Format: archiveZip, SHA256: expectedHash}, nil
	default:
		return ollamaDist{}, fmt.Errorf(
			"managed Ollama install is not supported on %s; install Ollama yourself and set IDAPT_OLLAMA_BINARY, or start with --managed=false",
			runtime.GOOS,
		)
	}
}

func verifyFileSHA256(path, expected string) error {
	if expected == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, expected)
	}
	return nil
}

func unsupportedOllamaArch(arch string) error {
	return fmt.Errorf("managed Ollama install does not support architecture %s on %s", arch, runtime.GOOS)
}

func formatFromName(name string) archiveFormat {
	switch {
	case strings.HasSuffix(name, ".zip"):
		return archiveZip
	case strings.HasSuffix(name, ".tgz"), strings.HasSuffix(name, ".tar.gz"):
		return archiveTarGz
	default:
		return archiveTarZst
	}
}

func archiveExt(name string) string {
	switch {
	case strings.HasSuffix(name, ".zip"):
		return ".zip"
	case strings.HasSuffix(name, ".tgz"):
		return ".tgz"
	case strings.HasSuffix(name, ".tar.gz"):
		return ".tar.gz"
	default:
		return ".tar.zst"
	}
}

func ollamaBinName() string {
	if runtime.GOOS == "windows" {
		return "ollama.exe"
	}
	return "ollama"
}

func ollamaConventionalBin(runtimeDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(runtimeDir, ollamaBinName())
	}
	return filepath.Join(runtimeDir, "bin", ollamaBinName())
}

func resolveManagedBin(runtimeDir string) (string, bool) {
	if p := ollamaConventionalBin(runtimeDir); isFile(p) {
		return p, true
	}
	if p, err := findOllamaBinary(runtimeDir); err == nil {
		return p, true
	}
	return "", false
}

func findOllamaBinary(root string) (string, error) {
	name := ollamaBinName()
	for _, rel := range []string{filepath.Join("bin", name), name} {
		if p := filepath.Join(root, rel); isFile(p) {
			return p, nil
		}
	}
	var found string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found != "" || d.IsDir() {
			return nil
		}
		if d.Name() == name {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if found == "" {
		return "", fmt.Errorf("Ollama executable %q not found in the downloaded bundle", name)
	}
	return found, nil
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func extractOllamaArchive(ctx context.Context, archivePath, destDir string, format archiveFormat) error {
	switch format {
	case archiveTarZst:
		return extractTarZst(ctx, archivePath, destDir)
	case archiveTarGz:
		return extractTarGz(ctx, archivePath, destDir)
	case archiveZip:
		return extractZip(ctx, archivePath, destDir)
	default:
		return fmt.Errorf("unknown archive format")
	}
}

func extractTarGz(ctx context.Context, archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	return extractTarStream(ctx, gz, destDir)
}

func extractZip(ctx context.Context, archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	destClean, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	for _, f := range r.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		target, err := safeTarTarget(destClean, f.Name)
		if err != nil {
			return err
		}
		info := f.FileInfo()
		if info.IsDir() {
			if err := os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		perm := info.Mode().Perm()
		if perm == 0 {
			perm = 0600
		}
		if err := copyZipEntry(f, target, perm); err != nil {
			return err
		}
	}
	return nil
}

func copyZipEntry(f *zip.File, target string, perm os.FileMode) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, rc)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
