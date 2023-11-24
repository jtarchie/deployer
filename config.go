package main

import (
	"bytes"
	"fmt"
	"os"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/jtarchie/deployer/providers"
	"gopkg.in/yaml.v3"
)

type Config struct {
	RunDirectory string                 `yaml:"run_directory"`
	Service      string                 `yaml:"service"`
	Image        string                 `yaml:"image"`
	DigitalOcean providers.DigitalOcean `yaml:"digital_ocean"`
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

	config := Config{
		RunDirectory: "/var/run/deployer",
	}

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
