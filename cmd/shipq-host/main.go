package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/emms-garcia/shipq/internal/adapters/macos"
	"github.com/emms-garcia/shipq/internal/host"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "shipq-host: unavailable")
		os.Exit(1)
	}
}

func run() error {
	config, err := host.DefaultConfig()
	if err != nil {
		return err
	}
	helper := os.Getenv("SHIPQ_PLATFORM_HELPER")
	if helper == "" {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		helper = filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "Library", "Helpers", "ShipqPlatform.app", "Contents", "MacOS", "ShipqPlatform"))
	}
	if platform, err := macos.New(helper); err == nil {
		config.Status = host.CapabilityStatus(config.ProfileID, platform)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runtime, err := host.Start(ctx, config)
	if err != nil {
		return err
	}
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return runtime.Shutdown(shutdown)
}
