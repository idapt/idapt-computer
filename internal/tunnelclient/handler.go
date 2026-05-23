package tunnelclient

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/idapt/idapt-cli/internal/auth"
)

const maxBodySize = 1 << 20 // 1 MiB

func NewHandler(m *Manager, machineToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := auth.ValidateMachineHMAC(r, machineToken); err != nil {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
		switch r.Method {
		case http.MethodGet:
			handleList(w, r, m)
		case http.MethodPost:
			handleExpose(w, r, m)
		case http.MethodDelete:
			handleUnexpose(w, r, m)
		default:
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func handleList(w http.ResponseWriter, r *http.Request, m *Manager) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	tunnels, err := m.Sync(ctx)
	if err != nil {
		tunnels = m.Cached()
		log.Printf("tunnel: list sync failed, served cache: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tunnels": tunnels})
}

func handleExpose(w http.ResponseWriter, r *http.Request, m *Manager) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read body failed")
		return
	}
	var req struct {
		Port     int    `json:"port"`
		AuthMode string `json:"authMode"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.AuthMode == "" {
		req.AuthMode = AuthModePrivate
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	result, err := m.Expose(ctx, req.Port, req.AuthMode)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Printf("tunnel: exposed port %d as %s", result.Port, result.Hostname)
	writeJSON(w, http.StatusOK, result)
}

func handleUnexpose(w http.ResponseWriter, r *http.Request, m *Manager) {
	port, err := strconv.Atoi(r.URL.Query().Get("port"))
	if err != nil || port < 1 || port > 65535 {
		writeErr(w, http.StatusBadRequest, "a valid ?port= query parameter is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	if err := m.Unexpose(ctx, port); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Printf("tunnel: unexposed port %d", port)
	writeJSON(w, http.StatusOK, map[string]any{"unexposed": port})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
