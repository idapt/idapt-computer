package appstarter

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"strings"
)

var (
	//go:embed template/index.html
	indexHTMLTemplate string
	//go:embed template/app.js
	appJSTemplate string
	//go:embed template/styles.css
	stylesCSS string
)

const (
	DefaultName = "My App"
	DefaultIcon = "📦"
)

type starterManifest struct {
	Entrypoint  string   `json:"entrypoint"`
	Name        string   `json:"name"`
	Icon        string   `json:"icon"`
	Version     string   `json:"version"`
	Permissions []string `json:"permissions"`
}

func escapeHTML(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, `"`, "&quot;")
	return value
}

func normalizeName(name string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	return DefaultName
}

func normalizeIcon(icon string) string {
	if trimmed := strings.TrimSpace(icon); trimmed != "" {
		return trimmed
	}
	return DefaultIcon
}

func renderManifest(name, icon string) ([]byte, error) {
	manifest := starterManifest{
		Entrypoint:  "index.html",
		Name:        name,
		Icon:        icon,
		Version:     "0.1.0",
		Permissions: []string{},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&manifest); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func StarterFiles(name, icon string) (map[string][]byte, error) {
	resolvedName := normalizeName(name)
	resolvedIcon := normalizeIcon(icon)

	manifest, err := renderManifest(resolvedName, resolvedIcon)
	if err != nil {
		return nil, err
	}

	indexHTML := indexHTMLTemplate
	indexHTML = strings.ReplaceAll(indexHTML, "__APP_NAME__", escapeHTML(resolvedName))
	indexHTML = strings.ReplaceAll(indexHTML, "__APP_ICON__", escapeHTML(resolvedIcon))

	appJS := strings.ReplaceAll(appJSTemplate, "__APP_NAME__", resolvedName)

	return map[string][]byte{
		"idapt.json": manifest,
		"index.html": []byte(indexHTML),
		"app.js":     []byte(appJS),
		"styles.css": []byte(stylesCSS),
	}, nil
}
