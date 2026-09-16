// Command addon-generator assembles, renders, and validates the addons
// under addons/ (see addons/README.md for the packaging/maturity contract).
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"

	"github.com/harvester/harvester/pkg/addons/render"
)

func main() {
	var generateAddons, generateTemplates, validate bool
	var addonsPath, destPath string

	cmd := &cli.Command{
		Name:  "addon-generator",
		Usage: "Assemble, render, and validate Harvester addons under addons/",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "generateAddons",
				Usage:       "generate disabled addon yaml manifests (built-in addons only)",
				Destination: &generateAddons,
			},
			&cli.BoolFlag{
				Name:        "generateTemplates",
				Usage:       "generate the assembled rancherd bootstrap template",
				Destination: &generateTemplates,
			},
			&cli.BoolFlag{
				Name:        "validate",
				Usage:       "validate the addons/ packaging contract (invariants + label derivation)",
				Destination: &validate,
			},
			&cli.StringFlag{
				Name:        "addonsPath",
				Value:       "addons",
				Usage:       "path to the addons/ directory",
				Destination: &addonsPath,
			},
			&cli.StringFlag{
				Name:        "path",
				Value:       ".",
				Usage:       "destination for output files",
				Destination: &destPath,
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			if !generateAddons && !generateTemplates && !validate {
				return fmt.Errorf("one of -generateAddons, -generateTemplates, or -validate must be specified")
			}

			versionFile := addonsPath + "/version_info"

			// validation always runs first: generation on top of an invalid
			// contract would only produce misleading output.
			if errs := render.Validate(addonsPath); len(errs) > 0 {
				for _, err := range errs {
					logrus.Error(err)
				}
				return fmt.Errorf("validation failed with %d error(s)", len(errs))
			}
			if validate {
				logrus.Info("addons/ packaging contract OK")
			}

			if generateTemplates {
				if err := render.GenerateTemplates(addonsPath, destPath, versionFile); err != nil {
					return fmt.Errorf("error generating templates: %w", err)
				}
			}

			if generateAddons {
				if err := render.GenerateAddons(addonsPath, destPath, versionFile); err != nil {
					return fmt.Errorf("error generating addons: %w", err)
				}
			}

			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		logrus.Fatal(err)
	}
}
