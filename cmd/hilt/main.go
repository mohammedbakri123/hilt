package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/mohammedbakri123/hilt/internal/app"
	"github.com/mohammedbakri123/hilt/internal/cli"
	"github.com/mohammedbakri123/hilt/internal/tui"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	backend := app.New(version)
	code := cli.Run(ctx, os.Args[1:], cli.Options{
		Backend: backend,
		Commit:  commit,
		Date:    date,
		In:      os.Stdin,
		Out:     os.Stdout,
		Err:     os.Stderr,
		RunTUI: func(backend cli.Backend) error {
			return tui.Run(backend)
		},
	})
	os.Exit(code)
}
