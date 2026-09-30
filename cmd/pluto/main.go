package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/atsuya-m/pluto/internal/adapter/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Execute(ctx)
	stop()
	os.Exit(code)
}
