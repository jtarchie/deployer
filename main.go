package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/alecthomas/kong"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Service      string       `yaml:"service"`
	Image        string       `yaml:"image"`
	DigitalOcean DigitalOcean `yaml:"digital_ocean"`
	Builder      struct {
		Dockerfile string `yaml:"dockerfile"`
		Context    string `yaml:"context"`
	} `yaml:"builder"`
	SSH struct {
		PrivateKey string `yaml:"private_key"`
	} `yaml:"ssh"`
	Servers   []string `yaml:"servers"`
	Bootstrap struct {
		Servers int `yaml:"servers"`
	} `yaml:"bootstrap"`
}

func readConfig(filename string) (*Config, error) {
	contents, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("could not read contents of file: %w", err)
	}

	var config Config

	file, err := template.
		New("").
		Funcs(sprig.FuncMap()).
		Parse(string(contents))
	if err != nil {
		return nil, fmt.Errorf("could not parse config file: %w", err)
	}

	builder := &bytes.Buffer{}

	err = file.Execute(builder, nil)
	if err != nil {
		return nil, fmt.Errorf("could not evaluate config file: %w", err)
	}

	err = yaml.Unmarshal(builder.Bytes(), &config)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal config file: %w", err)
	}

	return &config, nil
}

type Bootstrap struct {
	Config string `help:"config file to deploy from" required:""`
}

type CLI struct {
	Bootstrap Bootstrap `cmd:"" help:"bootstrap resources for the config file"`
}

func (b *Bootstrap) Run() error {
	config, err := readConfig(b.Config)
	if err != nil {
		return fmt.Errorf("could not load config file: %w", err)
	}

	outstanding := config.Bootstrap.Servers - len(config.Servers)

	slog.Info("bootstrap.start", slog.Int("outstanding", outstanding))

	if 0 < outstanding {
		ips, err := bootstrapMachines(
			outstanding,
			config.SSH.PrivateKey,
			fmt.Sprintf("deployer-%s", config.Service),
			config.DigitalOcean,
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

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	cli := &CLI{}
	ctx := kong.Parse(cli)
	// Call the Run() method of the selected parsed command.
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}
