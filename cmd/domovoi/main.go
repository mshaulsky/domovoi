// Command domovoi is the smart-home dashboard: it polls the configured
// sources, keeps the state of the home and paints it onto the configured
// displays.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mshaulsky/domovoi/internal/application"
)

// version is set at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("domovoi", "err", err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("config", "/etc/domovoi.yaml", "configuration file")
	check := flag.Bool("check", false, "load the configuration, build every instance, then exit")
	once := flag.Bool("once", false, "poll every source once, render every display once, then exit")
	verbose := flag.Bool("verbose", false, "log at debug level")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}
	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return application.Run(ctx, application.Options{
		ConfigPath: *path,
		Check:      *check,
		Once:       *once,
		Version:    version,
		LogLevel:   level,
	})
}
