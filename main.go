package main

import (
	"log/slog"
	"os"

	"github.com/sooua/send.to/cmd"
)

func main() {
	app := cmd.New()
	err := app.Run(os.Args)
	if err != nil {
		slog.Error("Fatal error", "error", err)
		os.Exit(1)
	}
}
