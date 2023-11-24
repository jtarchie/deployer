package providers

import (
	"context"
	"errors"
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

var ErrNoAPIToken = errors.New("no API token provided")

func (config *DigitalOcean) Execute(
	numInstances int,
	sshPrivateKey string,
	prefix string,
) ([]string, error) {
	if config.APIToken == "" {
		return nil, ErrNoAPIToken
	}

	client := godo.NewFromToken(config.APIToken)
	sshKeyPath := kong.ExpandPath(sshPrivateKey)

	ips, _ := checkForLatestIPs(client, prefix)
	if len(ips) == numInstances {
		return ips, nil
	}

	sshPublicKey, err := config.sshPublicKey(sshKeyPath)
	if err != nil {
		return nil, fmt.Errorf("could not load public key: %w", err)
	}

	err = config.createSSHKey(client, sshPublicKey, prefix)
	if err != nil {
		return nil, fmt.Errorf("could not create SSH key: %w", err)
	}

	err = config.createDroplets(numInstances, prefix, client, sshPublicKey)
	if err != nil {
		return nil, fmt.Errorf("could create droplets: %w", err)
	}

	ips, err = checkForLatestIPs(client, prefix)
	if err != nil {
		return nil, fmt.Errorf("could not get IPs: %w", err)
	}

	return ips, nil
}

func (config *DigitalOcean) createDroplets(numInstances int, prefix string, client *godo.Client, sshPublicKey ssh.PublicKey) error {
	names := []string{}

	for index := 0; index < numInstances; index++ {
		dropletName := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
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
		return fmt.Errorf("could not create droplets: %w", err)
	}

	var action godo.LinkAction

	for _, a := range response.Links.Actions {
		if a.Rel == "create" {
			action = a

			break
		}
	}

	_ = util.WaitForActive(context.TODO(), client, action.HREF)

	slog.Info("bootstrapping.droplets.complete", slog.Any("names", names))

	return nil
}

func (config *DigitalOcean) createSSHKey(client *godo.Client, sshPublicKey ssh.PublicKey, prefix string) error {
	key, _, _ := client.Keys.GetByFingerprint(context.TODO(), ssh.FingerprintLegacyMD5(sshPublicKey))
	if key == nil {
		slog.Info("bootstrapping.key.create")

		_, _, err := client.Keys.Create(context.TODO(), &godo.KeyCreateRequest{
			Name:      fmt.Sprintf("%s-key", prefix),
			PublicKey: string(ssh.MarshalAuthorizedKey(sshPublicKey)),
		})
		if err != nil {
			return fmt.Errorf("could not create ssh key: %w", err)
		}
	}

	return nil
}

//nolint: ireturn
func (config *DigitalOcean) sshPublicKey(sshKeyPath string) (ssh.PublicKey, error) {
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

	return sshPublicKey, nil
}

func checkForLatestIPs(client *godo.Client, prefix string) ([]string, error) {
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
