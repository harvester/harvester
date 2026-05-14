package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/harvester/harvester/pkg/installer/config"
	"github.com/harvester/harvester/pkg/installer/console"
	"github.com/harvester/harvester/pkg/installer/version"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:    "harvester-installer",
		Version: version.FriendlyVersion(),
		Usage:   "Console application to install Harvester",
		UsageText: `harvester-installer [global options] [command [command options]]

Executes the Harvester installer if no command is specified.`,

		Action: func(context.Context, *cli.Command) error {
			return console.RunConsole()
		},

		Commands: []*cli.Command{
			{
				Name:  "generate-network-config",
				Usage: "Generate NetworkManager connection profiles",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "config",
						Value: "/oem/harvester.config",
						Usage: "Harvester config file",
					},
					&cli.StringFlag{
						Name:  "connection-path",
						Value: config.NMConnectionPath,
						Usage: "Directory to save connection profiles in",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "Overwrite existing connection profiles",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					data, err := os.ReadFile(cmd.String("config"))
					if err != nil {
						return err
					}
					harvesterCfg, err := config.LoadHarvesterConfig(data)
					if err != nil {
						return err
					}
					paths, err := filepath.Glob(fmt.Sprintf("%s/%s", cmd.String("connection-path"), config.NMConnectionGlobPattern))
					if err != nil {
						return err
					}
					if len(paths) == 0 || cmd.Bool("force") {
						err = config.UpdateManagementInterfaceConfig(harvesterCfg.ManagementInterface, harvesterCfg.OS.DNSNameservers, cmd.String("connection-path"), false)
						if err != nil {
							return err
						}
						log.Printf("Generated NetworkManager connection profiles in %s from %s\n", cmd.String("connection-path"), cmd.String("config"))
						return nil
					} else {
						log.Printf("Skipped generation of NetworkManager connection profiles (%s/%s already exists, specify --force to overwrite)", cmd.String("connection-path"), config.NMConnectionGlobPattern)
						return nil

					}
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
