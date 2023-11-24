package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/docker/docker/api/types"
	docker "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/klauspost/compress/zstd"
)

type Deploy struct {
	Config string `help:"config file to deploy from" required:""`
}

func (d *Deploy) Run() error {
	config, err := readConfig(d.Config)
	if err != nil {
		return fmt.Errorf("could not load config file: %w", err)
	}

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

	imageName := fmt.Sprintf("deployer-%s:%d", config.Service, time.Now().UnixNano())

	response, err := client.ImageBuild(context.TODO(), tar, types.ImageBuildOptions{
		Dockerfile: config.Builder.Dockerfile,
		Tags: []string{
			imageName,
		},
	})
	if err != nil {
		return fmt.Errorf("could not build image: %w", err)
	}
	defer response.Body.Close()

	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		slog.Info("build.line", slog.String("line", scanner.Text()))
	}

	imageReader, err := client.ImageSave(context.Background(), []string{imageName})
	if err != nil {
			return fmt.Errorf("could not save image: %w", err)
	}
	defer imageReader.Close()

	// Create a file to save the compressed image
	outFile, err := os.Create("test.zst")
	if err != nil {
			return fmt.Errorf("could not create output file: %w", err)
	}
	defer outFile.Close()

	// Create a zstd writer
	zstdWriter, err := zstd.NewWriter(outFile)
	if err != nil {
			return fmt.Errorf("could not create compression writer: %w", err)
	}
	defer zstdWriter.Close()

	// Copy the image data to the zstd writer (this compresses and writes it)
	_, err = io.Copy(zstdWriter, imageReader)
	if err != nil {
			return fmt.Errorf("could not save image: %w", err)
	}

	return nil
}
