package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/commands"
	"github.com/idapt/idapt-cli/internal/config"
	"github.com/idapt/idapt-cli/internal/firewall"
	ifuse "github.com/idapt/idapt-cli/internal/fuse"
	"github.com/idapt/idapt-cli/internal/heartbeat"
	"github.com/idapt/idapt-cli/internal/revoke"
	"github.com/idapt/idapt-cli/internal/tunnelclient"
	"github.com/spf13/cobra"
)

var configPath string
var testSigCh chan os.Signal // set in test mode for /__test/signal/restart

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvIntOrDefault(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil {
		return fallback
	}
	return n
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the idapt daemon",
	Long: "Starts the per-computer daemon: heartbeat, the command channel, the " +
		"tunnel client, and FUSE mounts. Public traffic reaches the computer " +
		"only through the central tunnel-proxy — the daemon serves no public " +
		"TLS or reverse proxy of its own.",
	RunE: runServe,
}

func init() {
	serveCmd.Flags().StringVar(&configPath, "config", "", "Path to agent config file (default: per-user XDG path, falling back to /etc/idapt/config.json)")
}

func runServe(cmd *cobra.Command, args []string) error {
	resolved, err := config.ResolveConfigPath(configPath)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	configPath = resolved
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.IsLocalMode() {
		log.Printf("idapt %s starting in local mode (no computer pairing at %s)", Version, configPath)
	} else {
		log.Printf("idapt %s starting for computer %s", Version, cfg.ComputerID)
	}

	fwManager := firewall.NewManager()

	mux := http.NewServeMux()

	if cfg.ComputerToken != "" {
		mux.HandleFunc("POST /api/firewall", firewall.NewHandler(fwManager, cfg.ComputerToken))
		mux.HandleFunc("GET /api/firewall", firewall.NewGetHandler(fwManager, cfg.ComputerToken))
		mux.HandleFunc("GET /api/firewall/iptables", firewall.NewIptablesReadHandler(cfg.ComputerToken))
	}

	var fuseMountsRef *ifuse.MountManager
	var commandsClientRef *commands.Client
	var tunnelClientRef *tunnelclient.Client
	var tunnelMgrRef *tunnelclient.Manager

	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mountCount := 0
		if fuseMountsRef != nil {
			mountCount = len(fuseMountsRef.ActiveMounts())
		}
		commandsConnected := false
		commandsLastError := ""
		if commandsClientRef != nil {
			commandsConnected = commandsClientRef.Connected()
			commandsLastError = commandsClientRef.LastError()
		}
		tunnelConnected := false
		if tunnelClientRef != nil {
			tunnelConnected = tunnelClientRef.Connected()
		}
		resp, _ := json.Marshal(map[string]interface{}{
			"status":            "ok",
			"version":           Version,
			"fuseMounts":        mountCount,
			"commandsConnected": commandsConnected,
			"commandsLastError": commandsLastError,
			"tunnelConnected":   tunnelConnected,
			"commandsEnabled": cfg.ComputerToken != "" &&
				os.Getenv("IDAPT_COMMANDS_DISABLED") != "1",
		})
		w.Write(resp)
	}
	mux.HandleFunc("GET /api/health", healthHandler)

	if os.Getenv("IDAPT_TEST_MODE") == "1" {
		testSigCh = make(chan os.Signal, 1)
		mux.HandleFunc("POST /__test/signal/restart", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
			go func() {
				time.Sleep(100 * time.Millisecond)
				testSigCh <- syscall.SIGUSR1
			}()
		})
		mux.HandleFunc("POST /__test/set-computer-id", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				ComputerID    string `json:"computerId"`
				ComputerToken string `json:"computerToken"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.ComputerID != "" {
				os.Setenv("IDAPT_COMPUTER_ID", body.ComputerID)
				log.Printf("TEST MODE: computer ID updated to %s", body.ComputerID)
			}
			if body.ComputerToken != "" {
				os.Setenv("IDAPT_COMPUTER_TOKEN", body.ComputerToken)
				log.Printf("TEST MODE: computer token updated")
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
		})
		mux.HandleFunc("POST /__test/exec", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Command string `json:"command"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			out, err := exec.Command("sh", "-c", body.Command).CombinedOutput()
			w.Header().Set("Content-Type", "application/json")
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"output": string(out), "error": errStr,
			})
		})
		mux.HandleFunc("POST /__test/block-app", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Block bool `json:"block"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			appHost := cfg.AppURL
			if parsed, err := url.Parse(cfg.AppURL); err == nil && parsed.Hostname() != "" {
				appHost = parsed.Hostname()
			}
			var args []string
			if body.Block {
				args = []string{"-A", "OUTPUT", "-d", appHost, "-j", "DROP"}
			} else {
				args = []string{"-D", "OUTPUT", "-d", appHost, "-j", "DROP"}
			}
			if err := exec.Command("iptables", args...).Run(); err != nil {
				log.Printf("TEST MODE: iptables app block update failed: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
		})
		log.Printf("TEST MODE: /__test/* endpoints enabled")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if cfg.ComputerToken != "" {
		hb := heartbeat.New(
			cfg.AppURL,
			cfg.ComputerID,
			cfg.ComputerToken,
			Version,
			commands.OllamaLoadedModelIDs,
		)
		go hb.Start(ctx)
	} else {
		log.Printf("heartbeat: disabled (computerToken not configured)")
	}

	if cfg.ComputerToken != "" && os.Getenv("IDAPT_COMMANDS_DISABLED") != "1" {
		runuserCfg := commands.RunuserConfig{
			AllowRoot: os.Getenv("IDAPT_ALLOW_RUNAS_ROOT") == "1",
		}
		dedupTTL := time.Hour
		dedup := commands.NewDeduper(10_000, dedupTTL)
		health := commands.NewHealthState(Version, getEnvIntOrDefault("IDAPT_MAX_CONCURRENT_COMMANDS", 8), nil)
		poster := commands.NewHMACPoster(cfg.AppURL, cfg.ComputerID, cfg.ComputerToken)
		exec := commands.NewExecutor(
			runuserCfg,
			dedup,
			getEnvIntOrDefault("IDAPT_MAX_CONCURRENT_COMMANDS", 8),
			32,
			health,
			poster,
		)
		client := commands.NewClient(commands.ClientOpts{
			AppURL:       cfg.AppURL,
			ComputerID:    cfg.ComputerID,
			ComputerToken: cfg.ComputerToken,
			Executor:     exec,
			OnRevoked: func() {
				revoke.Trigger(configPath)
			},
		})
		commandsClientRef = client
		go client.Run(ctx)
		log.Printf("commands: subscriber started")
	}

	if cfg.TunnelProxyURL != "" && cfg.ComputerToken != "" {
		tunnelCfg := tunnelclient.NewConfigManager(tunnelclient.DefaultConfigPath)
		tunnelSyncer := tunnelclient.NewSyncer(cfg.AppURL, cfg.ComputerID, cfg.ComputerToken)
		tunnelMgr := tunnelclient.NewManager(tunnelCfg, tunnelSyncer)
		tunnelClient := tunnelclient.NewClient(cfg.TunnelProxyURL, tunnelSyncer, tunnelCfg)
		tunnelClientRef = tunnelClient
		tunnelMgrRef = tunnelMgr
		mux.HandleFunc("/api/tunnels", tunnelclient.NewHandler(tunnelMgr, cfg.ComputerToken))
		go tunnelClient.Run(ctx)
		go func() {
			syncCtx, cancelSync := context.WithTimeout(ctx, 30*time.Second)
			defer cancelSync()
			if _, err := tunnelMgr.Sync(syncCtx); err != nil {
				log.Printf("tunnel: initial registry sync failed: %v", err)
			}
		}()
		log.Printf("tunnel: client started (proxy %s)", cfg.TunnelProxyURL)
	} else {
		log.Printf("tunnel: disabled (tunnelProxyUrl or computerToken not set)")
	}

	fuseMM := ifuse.NewMountManager()
	fuseMountsRef = fuseMM // expose to health endpoint closure
	if len(cfg.Mounts) > 0 {
		fuseAPIClient, fuseErr := buildFuseAPIClient(cfg)
		if fuseErr != nil {
			log.Printf("fuse-mount: disabled (API client error: %v)", fuseErr)
		} else {
			for _, m := range cfg.Mounts {
				maxCache := int64(m.MaxCacheSizeGB) * 1024 * 1024 * 1024
				if maxCache == 0 {
					maxCache = 10 * 1024 * 1024 * 1024 // default 10GB
				}
				mountCfg := ifuse.MountConfig{
					WorkspaceID:       m.WorkspaceID,
					MountPoint:      m.MountPoint,
					CacheDir:        m.CacheDir,
					MaxCacheSize:    maxCache,
					ExcludePatterns: m.ExcludePatterns,
				}
				if err := fuseMM.Mount(ctx, mountCfg, fuseAPIClient); err != nil {
					log.Printf("fuse-mount: failed to mount %s at %s: %v", m.WorkspaceID, m.MountPoint, err)
				}
			}
		}
	} else {
		log.Printf("fuse-mount: no mounts configured")
	}

	errCh := make(chan error, 4)
	defaultPort := "80"
	httpAddr := ":"
	if cfg.IsLocalMode() {
		defaultPort = "6480"
		httpAddr = "127.0.0.1:"
	}
	httpPort := getEnvOrDefault("IDAPT_HTTP_PORT", defaultPort)
	if v := os.Getenv("IDAPT_HTTP_ADDR"); v != "" {
		httpAddr = v + ":"
	}
	httpServer := &http.Server{
		Addr:    httpAddr + httpPort,
		Handler: mux,
	}
	go func() {
		log.Printf("management API listening on %s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("HTTP server: %w", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGUSR1, syscall.SIGHUP)
	if testSigCh != nil {
		go func() {
			for sig := range testSigCh {
				sigCh <- sig
			}
		}()
	}

	for {
		select {
		case sig := <-sigCh:
			if sig == syscall.SIGHUP {
				log.Printf("Received SIGHUP — reloading tunnel config from disk")
				if tunnelMgrRef != nil {
					if err := tunnelMgrRef.Config().ReloadFromDisk(); err != nil {
						log.Printf("WARN: tunnel config reload failed: %v", err)
					} else {
						go func() {
							rsCtx, rsCancel := context.WithTimeout(context.Background(), 30*time.Second)
							defer rsCancel()
							if _, err := tunnelMgrRef.Sync(rsCtx); err != nil {
								log.Printf("WARN: tunnel resync after reload failed: %v", err)
							}
						}()
					}
				}
				continue // keep running, don't shut down
			}
			if sig == syscall.SIGUSR1 {
				log.Printf("Received SIGUSR1 — restarting with updated binary...")

				cancel() // stop heartbeat / commands / tunnel client

				drainCtx, drainCancel := context.WithTimeout(context.Background(), 5*time.Second)
				fuseMM.Shutdown(drainCtx) // flush + unmount FUSE
				httpServer.Shutdown(drainCtx)
				drainCancel()

				exe, err := os.Executable()
				if err != nil {
					log.Fatalf("Failed to resolve executable path for restart: %v", err)
				}
				log.Printf("Exec'ing new binary: %s %v", exe, os.Args)
				if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
					log.Fatalf("Exec failed (systemd will restart with new binary): %v", err)
				}
			}
			log.Printf("Received %s, shutting down gracefully...", sig)
		case err := <-errCh:
			log.Printf("Server error: %v, shutting down...", err)
		}
		break // exit the for loop to proceed with shutdown
	}

	cancel() // stop heartbeat / commands / tunnel client

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	fuseMM.Shutdown(shutdownCtx) // unmount FUSE first (flush dirty files)
	httpServer.Shutdown(shutdownCtx)

	log.Printf("idapt stopped")
	return nil
}

func buildFuseAPIClient(cfg *config.Config) (*ifuse.FuseAPIClient, error) {
	apiClient, err := api.NewClient(api.ClientConfig{
		BaseURL: cfg.AppURL,
		APIKey:  cfg.ComputerToken, // uses computer token for auth
	})
	if err != nil {
		return nil, err
	}
	return ifuse.NewFuseAPIClient(apiClient), nil
}
