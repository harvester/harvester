package ovsdb

import (
	"encoding/json"
	"fmt"
	"regexp"
)

var validUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// UUID is a UUID according to RFC7047
type UUID struct {
	GoUUID string `json:"uuid"`
}

// MarshalJSON will marshal an OVSDB style UUID to a JSON encoded byte array
func (u UUID) MarshalJSON() ([]byte, error) {
	var uuidSlice []string
	err := ValidateUUID(u.GoUUID)
	if err == nil {
		uuidSlice = []string{"uuid", u.GoUUID}
	} else {
		uuidSlice = []string{"named-uuid", u.GoUUID}
	}

	return json.Marshal(uuidSlice)
}

// UnmarshalJSON will unmarshal a JSON encoded byte array to a OVSDB style UUID
func (u *UUID) UnmarshalJSON(b []byte) (err error) {
	var ovsUUID []string
	if err := json.Unmarshal(b, &ovsUUID); err == nil {
		u.GoUUID = ovsUUID[1]
	}
	return err
}

func ValidateUUID(uuid string) error {
	if len(uuid) != 36 {
		return fmt.Errorf("uuid exceeds 36 characters")
	}

	if !validUUID.MatchString(uuid) {
		return fmt.Errorf("uuid does not match regexp")
	}

	return nil
}

func IsNamedUUID(uuid string) bool {
	return len(uuid) > 0 && !validUUID.MatchString(uuid)
}

func IsValidUUID(uuid string) bool {
	if err := ValidateUUID(uuid); err != nil {
		return false
	}
	return true
}
