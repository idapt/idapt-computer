package update

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"
)

type HealthSnapshot struct {
	Status             string `json:"status"`
	Version            string `json:"version"`
	ComputerID         string `json:"computerId"`
	ComputerResourceID string `json:"computerResourceId"`
	Domain             string `json:"domain"`
	Cloud              bool   `json:"cloud"`
	CommandsEnabled    bool   `json:"commandsEnabled"`
	CommandsConnected  bool   `json:"commandsConnected"`
	CommandsLastError  string `json:"commandsLastError"`
	TunnelConnected    bool   `json:"tunnelConnected"`
	TunnelLastError    string `json:"tunnelLastError"`
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
	snapshot, reachable := ProbeHealthSnapshot(healthURL, timeout)
	return snapshot.Status, snapshot.Version, reachable
}

func ProbeHealthSnapshot(healthURL string, timeout time.Duration) (HealthSnapshot, bool) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(healthURL)
	if err != nil {
		return HealthSnapshot{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return HealthSnapshot{}, false
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var h HealthSnapshot
	if json.Unmarshal(body, &h) != nil {
		return HealthSnapshot{}, true // reachable but unparseable
	}
	return h, true
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
