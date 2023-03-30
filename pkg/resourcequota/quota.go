package resourcequota

import "k8s.io/apimachinery/pkg/api/resource"

type Quotas struct {
	Limit QuotaLimit `json:"limit,omitempty"`
}

type QuotaLimit struct {
	LimitsCPU    resource.Quantity `json:"limitsCpu,omitempty"`
	LimitsMemory resource.Quantity `json:"limitsMemory,omitempty"`
}

type MaintenanceQuotaPercentage struct {
	Limit MaintenanceQuotaLimitPercentage `json:"limit,omitempty"`
}

type MaintenanceQuotaLimitPercentage struct {
	LimitsCPUPercent    int64 `json:"limitsCpuPercent,omitempty"`
	LimitsMemoryPercent int64 `json:"LimitsMemoryPercent,omitempty"`
}
