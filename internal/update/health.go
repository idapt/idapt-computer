package update

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func LocalHealthURL(cloud bool) string {
	port := "6480"
	if cloud {
		port = "80"
	}
	if v := os.Getenv("IDAPT_HTTP_PORT"); v != "" {
		port = v
	}
	return "http://127.0.0.1:" + port + "/api/health"
}

func ProbeHealth(healthURL string, timeout time.Duration) (status, version string, reachable bool) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(healthURL)
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", false
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var h healthResponse
	if json.Unmarshal(body, &h) != nil {
		return "", "", true // reachable but unparseable
	}
	return h.Status, h.Version, true
}

func WaitHealthy(healthURL, expectedVersion string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	expected := NormalizeVersion(expectedVersion)
	var lastStatus, lastVersion string
	for time.Now().Before(deadline) {
		status, version, reachable := ProbeHealth(healthURL, 3*time.Second)
		if reachable {
			lastStatus, lastVersion = status, version
			if status == "ok" && NormalizeVersion(version) == expected {
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}
	return &HealthTimeoutError{
		Expected:   expected,
		LastStatus: lastStatus,
		LastVer:    lastVersion,
		Timeout:    timeout,
	}
}

type HealthTimeoutError struct {
	Expected   string
	LastStatus string
	LastVer    string
	Timeout    time.Duration
}

func (e *HealthTimeoutError) Error() string {
	return "daemon did not become healthy on " + e.Expected + " within " +
		e.Timeout.String() + " (last status=" + e.LastStatus + " version=" + e.LastVer + ")"
}
