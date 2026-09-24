package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"time"

	"perpetual/internal/cli"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	client := &http.Client{Timeout: 10 * time.Second}
	return cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, cli.Options{
		Client:  client,
		BaseURL: os.Getenv("PERPETUAL_API_URL"),
	})
}
