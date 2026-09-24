package main

import (
	"context"
	"os"
	"os/signal"

	"perpetual/internal/cli"
)

func main() {
	os.Exit(run())
}

// run leaves the HTTP client unset so cli.Run applies its own default timeout.
func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, cli.Options{
		BaseURL: os.Getenv("PERPETUAL_API_URL"),
	})
}
