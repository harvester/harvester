package resourcequota

type MaintenanceQuota struct {
	Limit MaintenanceQuotaLimit `json:"limit,omitempty"`
}

type MaintenanceQuotaLimit struct {
	LimitsCPUPercent    int64 `json:"limitsCpuPercent,omitempty"`
	LimitsMemoryPercent int64 `json:"LimitsMemoryPercent,omitempty"`
}

type ResourceAvailable struct {
	Limit AvailableLimit `json:"limit,omitempty"`
}

type AvailableLimit struct {
	LimitsCPU    int64 `json:"limitsCpu,omitempty"`
	LimitsMemory int64 `json:"limitsMemory,omitempty"`
}
