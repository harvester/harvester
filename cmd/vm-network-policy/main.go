package main

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/harvester/harvester/pkg/vmnetworkpolicy/controllers"
)

func main() {
	logrus.Info("Starting vmnetworkpolicy controller")
	err := controllers.NewVMNetworkPolicyHandler(os.Getenv("KUBECONFIG"))
	if err != nil {
		panic(fmt.Errorf("failed to find kubeconfig: %v", err))
	}
}
