package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/docker/docker/api/types"
	docker "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
)

type Deploy struct {
	Config string `help:"config file to deploy from" required:""`
}

func (d *Deploy) Run() error {
	config, err := readConfig(d.Config)
	if err != nil {
		return fmt.Errorf("could not load config file: %w", err)
	}

	// build image

	client, err := docker.NewClientWithOpts(docker.FromEnv)
	if err != nil {
		return fmt.Errorf("could not connect to local docker host: %w", err)
	}

	buildContext := kong.ExpandPath(filepath.Join(filepath.Dir(d.Config), config.Builder.Context))
	contents, _ := os.ReadFile(filepath.Join(buildContext, ".dockerignore"))
	excludedPatterns := append([]string{".git/"}, strings.Split(string(contents), "\n")...)

	tar, err := archive.TarWithOptions(
		buildContext,
		&archive.TarOptions{
			ExcludePatterns: excludedPatterns,
		},
	)
	if err != nil {
		return fmt.Errorf("could not build tar of image contents: %w", err)
	}

	response, err := client.ImageBuild(context.TODO(), tar, types.ImageBuildOptions{
		Dockerfile: config.Builder.Dockerfile,
		Tags: []string{
			fmt.Sprintf("deployer-%s:%d", config.Service, time.Now().UnixNano()),
		},
	})
	if err != nil {
		return fmt.Errorf("could not build image: %w", err)
	}

	var lastLine string

	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		lastLine = scanner.Text()
		slog.Info("build.line", slog.String("line", scanner.Text()))
	}

	fmt.Println(lastLine)

	return nil
}
