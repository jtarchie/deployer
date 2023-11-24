package main

import (
	"github.com/jtarchie/deployer/commands"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
)

type CLI struct {
	Bootstrap commands.Bootstrap `cmd:"" help:"bootstrap resources for the config file"`
	Deploy    commands.Deploy    `cmd:"" help:"build and deploy image to servers"`
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	cli := &CLI{}
	ctx := kong.Parse(cli)
	// Call the Run() method of the selected parsed command.
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}
