package main

import (
	"github.com/spf13/cobra"

	"github.com/harvester/harvester/cmd/upgradehelper/cmd"
	_ "github.com/harvester/harvester/cmd/upgradehelper/cmd/restorevm"
	_ "github.com/harvester/harvester/cmd/upgradehelper/cmd/versionguard"
	_ "github.com/harvester/harvester/cmd/upgradehelper/cmd/vmlivemigratedetector"
)

func main() {
	cobra.CheckErr(cmd.RootCmd.Execute())
}
