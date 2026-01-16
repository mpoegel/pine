package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"

	pine "github.com/mpoegel/pine/pkg/pine"
)

func main() {
	config := pine.Config{}
	flag.StringVar(&config.TreeDir, "d", "/usr/local/etc/forest.d", "directory to find service configs")

	flag.Parse()

	if run(config) != nil {
		os.Exit(1)
	}
}

func run(config pine.Config) error {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	daemon := pine.NewDaemon(config)

	return daemon.Run(ctx)
}
