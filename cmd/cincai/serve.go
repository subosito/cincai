package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/subosito/cincai"
)

func serveCmd(args []string) int {
	fs := newFlagSet("serve")
	configPath := fs.String("config", "config/cincai.yaml", "path to cincai.yaml config file")
	if wantsHelp(args) {
		printCommandHelp("cincai serve — start the gateway", "  cincai serve [--config PATH]", fs)
		return 0
	}
	if err := parseFlags(fs, args); err != nil {
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := cincai.Run(ctx, cincai.Options{ConfigPath: *configPath}); err != nil {
		fmt.Fprintf(os.Stderr, "cincai serve: %v\n", err)
		return 1
	}
	return 0
}