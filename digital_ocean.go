package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/alecthomas/kong"
	"github.com/digitalocean/godo"
	"github.com/digitalocean/godo/util"
	"go.step.sm/crypto/keyutil"
	"go.step.sm/crypto/pemutil"
	"golang.org/x/crypto/ssh"
)

type DigitalOcean struct {
	APIToken string `yaml:"api_token"`
	Region   string `yaml:"region"`
	Size     string `yaml:"size"`
}

func createDigitalOceanDroplets(
	numInstances int,
	sshPrivateKey string,
	prefix string,
	config DigitalOcean,
) ([]string, error) {
	if config.APIToken == "" {
		return nil, fmt.Errorf("API token not provided")
	}

	client := godo.NewFromToken(config.APIToken)
	sshKeyPath := kong.ExpandPath(sshPrivateKey)

	ips, _ := checkForIPs(client, prefix)
	if len(ips) == numInstances {
		return ips, nil
	}

	privateKey, err := pemutil.Read(sshKeyPath)
	if err != nil {
		return nil, fmt.Errorf("could not read private key: %w", err)
	}

	publicKey, err := keyutil.PublicKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("could not read public key: %w", err)
	}

	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("could not get ssh public key: %w", err)
	}

	key, _, _ := client.Keys.GetByFingerprint(context.TODO(), ssh.FingerprintLegacyMD5(sshPublicKey))
	if key == nil {
		slog.Info("bootstrapping.key.create")

		_, _, err = client.Keys.Create(context.TODO(), &godo.KeyCreateRequest{
			Name:      fmt.Sprintf("%s-key", prefix),
			PublicKey: string(ssh.MarshalAuthorizedKey(sshPublicKey)),
		})
		if err != nil {
			return nil, fmt.Errorf("could not create ssh key: %w", err)
		}
	}

	names := []string{}
	for index := 0; index < numInstances; index++ {
		dropletName := fmt.Sprintf("-%d", prefix, time.Now().UnixNano())
		names = append(names, dropletName)
	}

	slog.Info("bootstrapping.droplets.create", slog.Any("names", names))

	_, response, err := client.Droplets.CreateMultiple(context.TODO(), &godo.DropletMultiCreateRequest{
		Names:  names,
		Region: config.Region,
		Size:   config.Size,
		Image: godo.DropletCreateImage{
			Slug: "docker-20-04",
		},
		SSHKeys: []godo.DropletCreateSSHKey{
			{
				Fingerprint: ssh.FingerprintLegacyMD5(sshPublicKey),
			},
		},
		Tags: []string{prefix},
	})
	if err != nil {
		return nil, fmt.Errorf("could not create droplets: %w", err)
	}

	var action *godo.LinkAction
	for _, a := range response.Links.Actions {
		if a.Rel == "create" {
			action = &a
			break
		}
	}

	_ = util.WaitForActive(context.TODO(), client, action.HREF)
	slog.Info("bootstrapping.droplets.complete", slog.Any("names", names))

	ips, err = checkForIPs(client, prefix)
	if err != nil {
		return nil, fmt.Errorf("could not get IPs: %w", err)
	}

	return ips, nil
}

func checkForIPs(client *godo.Client, prefix string) ([]string, error) {
	droplets, _, err := client.Droplets.ListByTag(
		context.TODO(),
		prefix,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("could not load droplets with tag: %w", err)
	}

	ips := []string{}
	for _, droplet := range droplets {
		ips = append(ips, droplet.Networks.V4[0].IPAddress)
	}

	return ips, nil
}
