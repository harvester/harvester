package ovsdb

import "encoding/json"

// MonitorSelect represents a monitor select according to RFC7047
type MonitorSelect struct {
	initial *bool
	insert  *bool
	delete  *bool
	modify  *bool
}

// NewMonitorSelect returns a new MonitorSelect with the provided values
func NewMonitorSelect(initial, insert, delete, modify bool) *MonitorSelect {
	return &MonitorSelect{
		initial: &initial,
		insert:  &insert,
		delete:  &delete,
		modify:  &modify,
	}
}

// NewDefaultMonitorSelect returns a new MonitorSelect with default values
func NewDefaultMonitorSelect() *MonitorSelect {
	return NewMonitorSelect(true, true, true, true)
}

// Initial returns whether or not an initial response will be sent
func (m MonitorSelect) Initial() bool {
	if m.initial == nil {
		return true
	}
	return *m.initial
}

// Insert returns whether we will receive updates for inserts
func (m MonitorSelect) Insert() bool {
	if m.insert == nil {
		return true
	}
	return *m.insert
}

// Delete returns whether we will receive updates for deletions
func (m MonitorSelect) Delete() bool {
	if m.delete == nil {
		return true
	}
	return *m.delete
}

// Modify returns whether we will receive updates for modifications
func (m MonitorSelect) Modify() bool {
	if m.modify == nil {
		return true
	}
	return *m.modify
}

type monitorSelect struct {
	Initial *bool `json:"initial,omitempty"`
	Insert  *bool `json:"insert,omitempty"`
	Delete  *bool `json:"delete,omitempty"`
	Modify  *bool `json:"modify,omitempty"`
}

func (m MonitorSelect) MarshalJSON() ([]byte, error) {
	ms := monitorSelect{
		Initial: m.initial,
		Insert:  m.insert,
		Delete:  m.delete,
		Modify:  m.modify,
	}
	return json.Marshal(ms)
}

func (m *MonitorSelect) UnmarshalJSON(data []byte) error {
	var ms monitorSelect
	err := json.Unmarshal(data, &ms)
	if err != nil {
		return err
	}
	m.initial = ms.Initial
	m.insert = ms.Insert
	m.delete = ms.Delete
	m.modify = ms.Modify
	return nil
}
