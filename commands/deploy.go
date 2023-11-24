package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	config "github.com/jtarchie/deployer/config"

	"github.com/alecthomas/kong"
	"github.com/docker/docker/api/types"
	docker "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/klauspost/compress/zstd"
	"github.com/melbahja/goph"
	"golang.org/x/crypto/ssh"
)

type Deploy struct {
	Config string `help:"config file to deploy from" required:""`
}

var ErrNoServersProvider = errors.New("no servers were provided")

func (d *Deploy) Run() error {
	config, err := config.FromFile(d.Config)
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

	imageFilename := filepath.Join(filepath.Dir(d.Config), "image.zst")
	// Create a file to save the compressed image
	outFile, err := os.Create(imageFilename)
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

	if len(config.Servers) == 0 {
		return ErrNoServersProvider
	}

	auth, err := goph.Key(kong.ExpandPath(config.SSH.PrivateKey), "")
	if err != nil {
		return fmt.Errorf("could not setup auth with ssh key: %w", err)
	}

	for _, server := range config.Servers {
		var port uint = 22

		client, err := goph.NewConn(&goph.Config{
			User:     "root",
			Addr:     server,
			Port:     port,
			Auth:     auth,
			Callback: VerifyHost,
		})
		if err != nil {
			return fmt.Errorf("could not connect to server %s: %w", server, err)
		}
		defer client.Close()

		_, err = client.Run(fmt.Sprintf("mkdir -p %s/images", config.RunDirectory))
		if err != nil {
			return fmt.Errorf("could not create run directory: %w", err)
		}

		err = client.Upload(imageFilename, filepath.Join(config.RunDirectory, "images", "image.zst"))
		if err != nil {
			return fmt.Errorf("could not copy image to server %s: %w", server, err)
		}
	}

	return nil
}

func VerifyHost(host string, remote net.Addr, key ssh.PublicKey) error {
	// hostFound: is host in known hosts file.
	// err: error if key not in known hosts file OR host in known hosts file but key changed!
	hostFound, err := goph.CheckKnownHost(host, remote, key, "")

	// Host in known hosts but key mismatch!
	// Maybe because of MAN IN THE MIDDLE ATTACK!
	if hostFound && err != nil {
		return fmt.Errorf("key mismatch in known hosts file for %s: %w", host, err)
	}

	// handshake because public key already exists.
	if hostFound && err == nil {
		return nil
	}

	// Add the new host to known hosts file.
	err = goph.AddKnownHost(host, remote, key, "")
	if err != nil {
		return fmt.Errorf("could not add server %s to known hosts: %w", host, err)
	}

	return nil
}
