package node

type CPUMigrationCapabilities struct {
	TotalNodes   int                             `json:"totalNodes"`
	GlobalModels []string                        `json:"globalModels"`
	Models       map[string]CPUModelCapabilities `json:"models"`
}

type CPUModelCapabilities struct {
	// ReadyCount indicates the number of nodes that are ready with this CPU model.
	// If ReadyCount > 1, it means there are at least two ready nodes with this CPU model.
	// If ReadyCount <= 1, migration is not possible as there is at most one ready node with this CPU model.
	ReadyCount int `json:"readyCount"`

	// MigrationSafe indicates whether VMs with this CPU model can be safely migrated.
	// MigrationSafe is true if there are more than one ready nodes with this CPU model.
	MigrationSafe bool `json:"migrationSafe"`
}
