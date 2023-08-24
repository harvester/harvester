package main

import (
	"fmt"
	"os"

	"github.com/harvester/harvester/pkg/firewall/controllers"
	"github.com/sirupsen/logrus"
)

func main() {
	logrus.Info("Starting firewall controller")
	err := controllers.NewVMNetworkPolicyHandler(os.Getenv("KUBECONFIG"))
	if err != nil {
		panic(fmt.Errorf("failed to find kubeconfig: %v", err))
	}
}
