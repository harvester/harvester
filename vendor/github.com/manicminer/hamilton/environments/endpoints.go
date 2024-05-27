package environments

type AzureADEndpoint string

const (
	AzureADGlobal AzureADEndpoint = "https://login.microsoftonline.com"
	AzureADUSGov  AzureADEndpoint = "https://login.microsoftonline.us"
	AzureADChina  AzureADEndpoint = "https://login.chinacloudapi.cn"
)

type ApiEndpoint string

const (
	MsGraphGlobalEndpoint  ApiEndpoint = "https://graph.microsoft.com"
	MsGraphChinaEndpoint   ApiEndpoint = "https://microsoftgraph.chinacloudapi.cn"
	MsGraphUSGovL4Endpoint ApiEndpoint = "https://graph.microsoft.us"
	MsGraphUSGovL5Endpoint ApiEndpoint = "https://dod-graph.microsoft.us"
	MsGraphCanaryEndpoint  ApiEndpoint = "https://canary.graph.microsoft.com"

	AadGraphGlobalEndpoint ApiEndpoint = "https://graph.windows.net"
	AadGraphChinaEndpoint  ApiEndpoint = "https://graph.chinacloudapi.cn"
	AadGraphUSGovEndpoint  ApiEndpoint = "https://graph.microsoftazure.us"

	ResourceManagerPublicEndpoint ApiEndpoint = "https://management.azure.com"
	ResourceManagerChinaEndpoint  ApiEndpoint = "https://management.chinacloudapi.cn"
	ResourceManagerUSGovEndpoint  ApiEndpoint = "https://management.usgovcloudapi.net"

	BatchManagementPublicEndpoint ApiEndpoint = "https://batch.core.windows.net"
	BatchManagementChinaEndpoint  ApiEndpoint = "https://batch.chinacloudapi.cn"
	BatchManagementUSGovEndpoint  ApiEndpoint = "https://batch.core.usgovcloudapi.net"

	DataLakePublicEndpoint ApiEndpoint = "https://datalake.azure.net"

	KeyVaultPublicEndpoint ApiEndpoint = "https://vault.azure.net"
	KeyVaultChinaEndpoint  ApiEndpoint = "https://vault.azure.cn"
	KeyVaultUSGovEndpoint  ApiEndpoint = "https://vault.usgovcloudapi.net"

	OperationalInsightsPublicEndpoint ApiEndpoint = "https://api.loganalytics.io"
	OperationalInsightsUSGovEndpoint  ApiEndpoint = "https://api.loganalytics.us"

	OSSRDBMSPublicEndpoint ApiEndpoint = "https://ossrdbms-aad.database.windows.net"
	OSSRDBMSChinaEndpoint  ApiEndpoint = "https://ossrdbms-aad.database.chinacloudapi.cn"
	OSSRDBMSUSGovEndpoint  ApiEndpoint = "https://ossrdbms-aad.database.usgovcloudapi.net"

	ServiceBusPublicEndpoint ApiEndpoint = "https://servicebus.windows.net"
	ServiceBusChinaEndpoint  ApiEndpoint = "https://servicebus.chinacloudapi.cn"
	ServiceBusUSGovEndpoint  ApiEndpoint = "https://servicebus.usgovcloudapi.net"

	ServiceManagementPublicEndpoint ApiEndpoint = "https://management.core.windows.net"
	ServiceManagementChinaEndpoint  ApiEndpoint = "https://management.core.chinacloudapi.cn"
	ServiceManagementUSGovEndpoint  ApiEndpoint = "https://management.core.usgovcloudapi.net"

	SQLDatabasePublicEndpoint ApiEndpoint = "https://database.windows.net"
	SQLDatabaseChinaEndpoint  ApiEndpoint = "https://database.chinacloudapi.cn"
	SQLDatabaseUSGovEndpoint  ApiEndpoint = "https://database.usgovcloudapi.net"

	StoragePublicEndpoint ApiEndpoint = "https://storage.azure.com"

	SynapsePublicEndpoint ApiEndpoint = "https://dev.azuresynapse.net"
)
