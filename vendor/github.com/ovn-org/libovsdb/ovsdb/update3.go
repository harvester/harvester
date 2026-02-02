package ovsdb

import (
	"encoding/json"
	"fmt"
)

type MonitorCondSinceReply struct {
	Found             bool
	LastTransactionID string
	Updates           TableUpdates2
}

func (m MonitorCondSinceReply) MarshalJSON() ([]byte, error) {
	v := []interface{}{m.Found, m.LastTransactionID, m.Updates}
	return json.Marshal(v)
}

func (m *MonitorCondSinceReply) UnmarshalJSON(b []byte) error {
	var v []json.RawMessage
	err := json.Unmarshal(b, &v)
	if err != nil {
		return err
	}
	if len(v) != 3 {
		return fmt.Errorf("expected a 3 element json array. there are %d elements", len(v))
	}

	var found bool
	err = json.Unmarshal(v[0], &found)
	if err != nil {
		return err
	}

	var lastTransactionID string
	err = json.Unmarshal(v[1], &lastTransactionID)
	if err != nil {
		return err
	}

	var updates TableUpdates2
	err = json.Unmarshal(v[2], &updates)
	if err != nil {
		return err
	}

	m.Found = found
	m.LastTransactionID = lastTransactionID
	m.Updates = updates
	return nil
}
