package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/util"
)

var getCloudProviderCmd = &cobra.Command{
	Use:   "get-cloud-provider",
	Short: "Get ClusterRole with Harvester Cloud Provider",
	Long:  `A command for getting the ClusterRole with "harvesterhci.io:cloudprovider"`,

	Run: func(*cobra.Command, []string) {
		if err := getCloudProvider(); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(getCloudProviderCmd)
}

func getCloudProvider() error {
	logrus.Debug("Getting ClusterRole with Harvester Cloud Provider")
	clientset, err := util.GetK8sClientset()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	rolebindings, err := clientset.RbacV1().RoleBindings("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	output := []string{}
	for _, rolebinding := range rolebindings.Items {
		if rolebinding.RoleRef.Name == "harvesterhci.io:cloudprovider" {
			item := fmt.Sprintf("%s/%s", rolebinding.Namespace, rolebinding.Name)
			output = append(output, item)
		}
	}

	// format the output as we wanted
	for _, item := range output {
		fmt.Printf("%s\n", item)
	}
	return nil
}
