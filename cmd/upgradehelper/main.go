package main

import (
	"github.com/spf13/cobra"
)

var (
	AppVersion = "dev"
	GitCommit  = "commit"
)

func main() {
	cobra.CheckErr(rootCmd.Execute())
}
