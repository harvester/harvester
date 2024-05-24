package environments

import (
	"fmt"
)

var ApiUnavailable = Api{}

// API represent an API configuration for Microsoft Graph or Azure Active Directory Graph.
type Api struct {
	// The Application ID for the API.
	AppId ApiAppId

	// The endpoint for the API, including scheme.
	Endpoint ApiEndpoint
}

func (a Api) IsAvailable() bool {
	return a != ApiUnavailable
}

func (a Api) DefaultScope() string {
	return fmt.Sprintf("%s/.default", a.Endpoint)
}

func (a Api) Resource() string {
	return fmt.Sprintf("%s/", a.Endpoint)
}

var (
	MsGraphGlobal = Api{
		AppId:    PublishedApis["MicrosoftGraph"],
		Endpoint: MsGraphGlobalEndpoint,
	}

	MsGraphChina = Api{
		AppId:    PublishedApis["MicrosoftGraph"],
		Endpoint: MsGraphChinaEndpoint,
	}

	MsGraphUSGovL4 = Api{
		AppId:    PublishedApis["MicrosoftGraph"],
		Endpoint: MsGraphUSGovL4Endpoint,
	}

	MsGraphUSGovL5 = Api{
		AppId:    PublishedApis["MicrosoftGraph"],
		Endpoint: MsGraphUSGovL5Endpoint,
	}

	MsGraphCanary = Api{
		AppId:    PublishedApis["MicrosoftGraph"],
		Endpoint: MsGraphCanaryEndpoint,
	}

	AadGraphGlobal = Api{
		AppId:    PublishedApis["AzureActiveDirectoryGraph"],
		Endpoint: AadGraphGlobalEndpoint,
	}

	AadGraphChina = Api{
		AppId:    PublishedApis["AzureActiveDirectoryGraph"],
		Endpoint: AadGraphChinaEndpoint,
	}

	AadGraphUSGov = Api{
		AppId:    PublishedApis["AzureActiveDirectoryGraph"],
		Endpoint: AadGraphUSGovEndpoint,
	}

	ResourceManagerPublic = Api{
		AppId:    PublishedApis["AzureServiceManagement"],
		Endpoint: ResourceManagerPublicEndpoint,
	}

	ResourceManagerChina = Api{
		AppId:    PublishedApis["AzureServiceManagement"],
		Endpoint: ResourceManagerChinaEndpoint,
	}

	ResourceManagerUSGov = Api{
		AppId:    PublishedApis["AzureServiceManagement"],
		Endpoint: ResourceManagerUSGovEndpoint,
	}

	BatchManagementPublic = Api{
		AppId:    PublishedApis["AzureBatch"],
		Endpoint: BatchManagementPublicEndpoint,
	}

	BatchManagementChina = Api{
		AppId:    PublishedApis["AzureBatch"],
		Endpoint: BatchManagementChinaEndpoint,
	}

	BatchManagementUSGov = Api{
		AppId:    PublishedApis["AzureBatch"],
		Endpoint: BatchManagementUSGovEndpoint,
	}

	DataLakePublic = Api{
		AppId:    PublishedApis["AzureDataLake"],
		Endpoint: DataLakePublicEndpoint,
	}

	KeyVaultPublic = Api{
		AppId:    PublishedApis["AzureKeyVault"],
		Endpoint: KeyVaultPublicEndpoint,
	}

	KeyVaultChina = Api{
		AppId:    PublishedApis["AzureKeyVault"],
		Endpoint: KeyVaultChinaEndpoint,
	}

	KeyVaultUSGov = Api{
		AppId:    PublishedApis["AzureKeyVault"],
		Endpoint: KeyVaultUSGovEndpoint,
	}

	OperationalInsightsPublic = Api{
		AppId:    PublishedApis["LogAnalytics"],
		Endpoint: OperationalInsightsPublicEndpoint,
	}

	OperationalInsightsUSGov = Api{
		AppId:    PublishedApis["LogAnalytics"],
		Endpoint: OperationalInsightsUSGovEndpoint,
	}

	OSSRDBMSPublic = Api{
		AppId:    PublishedApis["OssRdbms"],
		Endpoint: OSSRDBMSPublicEndpoint,
	}

	OSSRDBMSChina = Api{
		AppId:    PublishedApis["OssRdbms"],
		Endpoint: OSSRDBMSChinaEndpoint,
	}

	OSSRDBMSUSGov = Api{
		AppId:    PublishedApis["OssRdbms"],
		Endpoint: OSSRDBMSUSGovEndpoint,
	}

	ServiceBusPublic = Api{
		AppId:    PublishedApis["AzureServiceBus"],
		Endpoint: ServiceBusPublicEndpoint,
	}

	ServiceBusChina = Api{
		AppId:    PublishedApis["AzureServiceBus"],
		Endpoint: ServiceBusChinaEndpoint,
	}

	ServiceBusUSGov = Api{
		AppId:    PublishedApis["AzureServiceBus"],
		Endpoint: ServiceBusUSGovEndpoint,
	}

	ServiceManagementPublic = Api{
		AppId:    PublishedApis["AzureServiceManagement"],
		Endpoint: ServiceManagementPublicEndpoint,
	}

	ServiceManagementChina = Api{
		AppId:    PublishedApis["AzureServiceManagement"],
		Endpoint: ServiceManagementChinaEndpoint,
	}

	ServiceManagementUSGov = Api{
		AppId:    PublishedApis["AzureServiceManagement"],
		Endpoint: ServiceManagementUSGovEndpoint,
	}

	SQLDatabasePublic = Api{
		AppId:    PublishedApis["AzureSqlDatabase"],
		Endpoint: SQLDatabasePublicEndpoint,
	}

	SQLDatabaseChina = Api{
		AppId:    PublishedApis["AzureSqlDatabase"],
		Endpoint: SQLDatabaseChinaEndpoint,
	}

	SQLDatabaseUSGov = Api{
		AppId:    PublishedApis["AzureSqlDatabase"],
		Endpoint: SQLDatabaseUSGovEndpoint,
	}

	StoragePublic = Api{
		AppId:    PublishedApis["AzureStorage"],
		Endpoint: StoragePublicEndpoint,
	}

	SynapsePublic = Api{
		AppId:    PublishedApis["AzureSynapseGateway"],
		Endpoint: SynapsePublicEndpoint,
	}
)

type ApiCliName string

const (
	MsGraphCliName         ApiCliName = "ms-graph"
	AadGraphCliName        ApiCliName = "aad-graph"
	ResourceManagerCliName ApiCliName = "arm"
	BatchCliName           ApiCliName = "batch"
	DataLakeCliName        ApiCliName = "data-lake"
	OSSRDBMSCliName        ApiCliName = "oss-rdbms"
)
