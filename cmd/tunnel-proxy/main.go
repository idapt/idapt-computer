package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/idapt/idapt-computer/internal/tunnelproxy"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	cfg, err := tunnelproxy.LoadConfig()
	if err != nil {
		log.Fatalf("tunnel-proxy: config: %v", err)
	}
	srv, err := tunnelproxy.NewServer(cfg)
	if err != nil {
		log.Fatalf("tunnel-proxy: init: %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Run() }()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-errCh:
		if err != nil {
			log.Fatalf("tunnel-proxy: serve: %v", err)
		}
	case sig := <-sigCh:
		log.Printf("tunnel-proxy: received %s, shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("tunnel-proxy: shutdown: %v", err)
		}
	}
}
