package resourcequota

type MaintenanceQuota struct {
	Limit MaintenanceQuotaLimit `json:"limit,omitempty"`
}

type MaintenanceQuotaLimit struct {
	LimitsCpuPercent    int64 `json:"limitsCpuPercent,omitempty"`
	LimitsMemoryPercent int64 `json:"LimitsMemoryPercent,omitempty"`
}

type ResourceAvailable struct {
	Limit AvailableLimit `json:"limit,omitempty"`
}

type AvailableLimit struct {
	LimitsCpu    int64 `json:"limitsCpu,omitempty"`
	LimitsMemory int64 `json:"limitsMemory,omitempty"`
}
