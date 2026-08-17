package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/benenen/lapin/internal/lapincli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(lapincli.Run(ctx, os.Args[1:], lapincli.Dependencies{
		LookupEnv: os.LookupEnv,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	}))
}
