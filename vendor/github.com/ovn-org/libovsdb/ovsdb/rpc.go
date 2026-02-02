package ovsdb

const (
	// MonitorRPC is the monitor RPC method
	MonitorRPC = "monitor"
	// ConditionalMonitorRPC is the monitor_cond
	ConditionalMonitorRPC = "monitor_cond"
	// ConditionalMonitorSinceRPC is the monitor_cond_since RPC method
	ConditionalMonitorSinceRPC = "monitor_cond_since"
)

// NewEchoArgs creates a new set of arguments for an echo RPC
func NewEchoArgs() []interface{} {
	return []interface{}{"libovsdb echo"}
}

// NewGetSchemaArgs creates a new set of arguments for a get_schemas RPC
func NewGetSchemaArgs(schema string) []interface{} {
	return []interface{}{schema}
}

// NewTransactArgs creates a new set of arguments for a transact RPC
func NewTransactArgs(database string, operations ...Operation) []interface{} {
	dbSlice := make([]interface{}, 1)
	dbSlice[0] = database

	opsSlice := make([]interface{}, len(operations))
	for i, d := range operations {
		opsSlice[i] = d
	}

	ops := append(dbSlice, opsSlice...)
	return ops
}

// NewCancelArgs creates a new set of arguments for a cancel RPC
func NewCancelArgs(id interface{}) []interface{} {
	return []interface{}{id}
}

// NewMonitorArgs creates a new set of arguments for a monitor RPC
func NewMonitorArgs(database string, value interface{}, requests map[string]MonitorRequest) []interface{} {
	return []interface{}{database, value, requests}
}

// NewMonitorCondSinceArgs creates a new set of arguments for a monitor_cond_since RPC
func NewMonitorCondSinceArgs(database string, value interface{}, requests map[string]MonitorRequest, lastTransactionID string) []interface{} {
	return []interface{}{database, value, requests, lastTransactionID}
}

// NewMonitorCancelArgs creates a new set of arguments for a monitor_cancel RPC
func NewMonitorCancelArgs(value interface{}) []interface{} {
	return []interface{}{value}
}

// NewLockArgs creates a new set of arguments for a lock, steal or unlock RPC
func NewLockArgs(id interface{}) []interface{} {
	return []interface{}{id}
}

// NotificationHandler is the interface that must be implemented to receive notifications
type NotificationHandler interface {
	// RFC 7047 section 4.1.6 Update Notification
	Update(context interface{}, tableUpdates TableUpdates)

	// ovsdb-server.7 update2 notifications
	Update2(context interface{}, tableUpdates TableUpdates2)

	// RFC 7047 section 4.1.9 Locked Notification
	Locked([]interface{})

	// RFC 7047 section 4.1.10 Stolen Notification
	Stolen([]interface{})

	// RFC 7047 section 4.1.11 Echo Notification
	Echo([]interface{})

	Disconnected()
}
