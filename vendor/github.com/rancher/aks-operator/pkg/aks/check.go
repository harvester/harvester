package aks

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/operationalinsights/mgmt/2020-08-01/operationalinsights"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/rancher/aks-operator/pkg/aks/services"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	workspaceLength     = 63
	workspaceNameLength = 46
)

// Please keep in sync with
// https://github.com/Azure/azure-cli/blob/release/src/azure-cli/azure/cli/command_modules/acs/custom.py#L3091
var locationToOmsRegionCodeMap = map[string]string{
	"australiasoutheast": "ASE",
	"australiaeast":      "EAU",
	"australiacentral":   "CAU",
	"canadacentral":      "CCA",
	"centralindia":       "CIN",
	"centralus":          "CUS",
	"eastasia":           "EA",
	"eastus":             "EUS",
	"eastus2":            "EUS2",
	"eastus2euap":        "EAP",
	"francecentral":      "PAR",
	"japaneast":          "EJP",
	"koreacentral":       "SE",
	"northeurope":        "NEU",
	"southcentralus":     "SCUS",
	"southeastasia":      "SEA",
	"uksouth":            "SUK",
	"usgovvirginia":      "USGV",
	"westcentralus":      "EUS",
	"westeurope":         "WEU",
	"westus":             "WUS",
	"westus2":            "WUS2",
	"brazilsouth":        "CQ",
	"brazilsoutheast":    "BRSE",
	"norwayeast":         "NOE",
	"southafricanorth":   "JNB",
	"northcentralus":     "NCUS",
	"uaenorth":           "DXB",
	"germanywestcentral": "DEWC",
	"ukwest":             "WUK",
	"switzerlandnorth":   "CHN",
	"switzerlandwest":    "CHW",
	"uaecentral":         "AUH",
	// mapping for azure china cloud
	"chinaeast":   "EAST2",
	"chinaeast2":  "EAST2",
	"chinanorth":  "EAST2",
	"chinanorth2": "EAST2",
}

// Please keep in sync with
// https://github.com/Azure/azure-cli/blob/release/src/azure-cli/azure/cli/command_modules/acs/custom.py#L3126
var regionToOmsRegionMap = map[string]string{
	"australiacentral":   "australiacentral",
	"australiacentral2":  "australiacentral",
	"australiaeast":      "australiaeast",
	"australiasoutheast": "australiasoutheast",
	"brazilsouth":        "brazilsouth",
	"canadacentral":      "canadacentral",
	"canadaeast":         "canadacentral",
	"centralus":          "centralus",
	"centralindia":       "centralindia",
	"eastasia":           "eastasia",
	"eastus":             "eastus",
	"eastus2":            "eastus2",
	"francecentral":      "francecentral",
	"francesouth":        "francecentral",
	"japaneast":          "japaneast",
	"japanwest":          "japaneast",
	"koreacentral":       "koreacentral",
	"koreasouth":         "koreacentral",
	"northcentralus":     "northcentralus",
	"northeurope":        "northeurope",
	"southafricanorth":   "southafricanorth",
	"southafricawest":    "southafricanorth",
	"southcentralus":     "southcentralus",
	"southeastasia":      "southeastasia",
	"southindia":         "centralindia",
	"uksouth":            "uksouth",
	"ukwest":             "ukwest",
	"westcentralus":      "eastus",
	"westeurope":         "westeurope",
	"westindia":          "centralindia",
	"westus":             "westus",
	"westus2":            "westus2",
	"norwayeast":         "norwayeast",
	"norwaywest":         "norwayeast",
	"switzerlandnorth":   "switzerlandnorth",
	"switzerlandwest":    "switzerlandwest",
	"uaenorth":           "uaenorth",
	"germanywestcentral": "germanywestcentral",
	"germanynorth":       "germanywestcentral",
	"uaecentral":         "uaecentral",
	"eastus2euap":        "eastus2euap",
	"brazilsoutheast":    "brazilsoutheast",
	// mapping for azure china cloud
	"chinaeast":   "chinaeast2",
	"chinaeast2":  "chinaeast2",
	"chinanorth":  "chinaeast2",
	"chinanorth2": "chinaeast2",
}

func CheckLogAnalyticsWorkspaceForMonitoring(ctx context.Context, client services.WorkplacesClientInterface,
	location string, group string, wsg string, wsn string) (workspaceID string, err error) {
	workspaceRegion, ok := regionToOmsRegionMap[location]
	if !ok {
		return "", fmt.Errorf("region %s not supported for Log Analytics workspace", location)
	}

	workspaceRegionCode, ok := locationToOmsRegionCodeMap[workspaceRegion]
	if !ok {
		return "", fmt.Errorf("region %s not supported for Log Analytics workspace", workspaceRegion)
	}

	workspaceResourceGroup := wsg
	if workspaceResourceGroup == "" {
		workspaceResourceGroup = group
	}

	workspaceName := wsn
	if workspaceName == "" {
		workspaceName = fmt.Sprintf("%s-%s", group, workspaceRegionCode)
	}

	// workspaceName string length can be only 63
	if len(workspaceName) > workspaceLength {
		workspaceName = generateUniqueLogWorkspace(workspaceName)
	}

	if gotRet, gotErr := client.Get(ctx, workspaceResourceGroup, workspaceName); gotErr == nil {
		return *gotRet.ID, nil
	}

	logrus.Infof("Create Azure Log Analytics Workspace %q on Resource Group %q", workspaceName, workspaceResourceGroup)

	asyncRet, asyncErr := client.CreateOrUpdate(ctx, workspaceResourceGroup, workspaceName, operationalinsights.Workspace{
		Location: to.StringPtr(workspaceRegion),
		WorkspaceProperties: &operationalinsights.WorkspaceProperties{
			Sku: &operationalinsights.WorkspaceSku{
				Name: operationalinsights.WorkspaceSkuNameEnumStandalone,
			},
		},
	})
	if asyncErr != nil {
		return "", asyncErr
	}
	workspaceID = ""
	err = wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		ret, err := client.AsyncCreateUpdateResult(asyncRet)
		if err != nil {
			return false, err
		}

		workspaceID = *ret.ID
		return true, nil
	})
	if err != nil {
		return "", err
	}
	return workspaceID, nil
}

func generateUniqueLogWorkspace(workspaceName string) string {
	if len(workspaceName) < workspaceNameLength {
		return workspaceName
	}
	s := workspaceName[0:workspaceNameLength]
	h := sha256.New()
	h.Write([]byte(workspaceName))
	hexHash := h.Sum(nil)
	shaString := fmt.Sprintf("%x", hexHash)
	return fmt.Sprintf("%s-%s", s, shaString[0:16])
}
