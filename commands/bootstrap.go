package commands

import (
	"fmt"
	"log/slog"

	"github.com/jtarchie/deployer/config"
)

type Bootstrap struct {
	Config string `help:"config file to deploy from" required:""`
}

func (b *Bootstrap) Run() error {
	config, err := config.FromFile(b.Config)
	if err != nil {
		return fmt.Errorf("could not load config file: %w", err)
	}

	outstanding := config.Bootstrap.Servers - len(config.Servers)

	slog.Info("bootstrap.start", slog.Int("outstanding", outstanding))

	if 0 < outstanding {
		ips, err := config.DigitalOcean.Execute(
			outstanding,
			config.SSH.PrivateKey,
			fmt.Sprintf("deployer-%s", config.Service),
		)
		if err != nil {
			return fmt.Errorf("could not bootstrap machines: %w", err)
		}

		slog.Info("bootstrap.complete")

		fmt.Print("\nPlease add the following to your config file:\n\n")
		fmt.Println("servers:")

		for _, ip := range ips {
			fmt.Printf("- %s\n", ip)
		}

		return nil
	}

	slog.Info("bootstrap.noop")

	return nil
}
