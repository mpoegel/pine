package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	pine "github.com/mpoegel/pine/pkg/pine"
)

func main() {
	config := pine.Config{}
	flag.StringVar(&config.TreeDir, "d", "/usr/local/etc/forest.d", "directory to find service configs")
	flag.StringVar(&config.UdsEndpoint, "e", "/var/run/pine.sock", "UDS endpoint for talking to pine")
	flag.BoolVar(&config.UnprivilegedMode, "unprivileged", false, "run as unprivileged user")

	flag.Parse()

	if err := run(config); err != nil {
		slog.Error("pine failed", "err", err)
		os.Exit(1)
	}
}

func run(config pine.Config) error {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	daemon := pine.NewDaemon(config)

	return daemon.Run(ctx)
}
