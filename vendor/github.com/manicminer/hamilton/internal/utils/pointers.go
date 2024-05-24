package utils

// BoolPtr returns a pointer to the provided boolean variable.
func BoolPtr(b bool) *bool {
	return &b
}

// IntPtr returns a pointer to the provided int variable.
func IntPtr(i int) *int {
	return &i
}

// Int32Ptr returns a pointer to the provided int32 variable.
func Int32Ptr(i int32) *int32 {
	return &i
}

// StringPtr returns a pointer to the provided string variable.
func StringPtr(s string) *string {
	return &s
}

// ArrayStringPtr returns a pointer to the provided array of strings.
func ArrayStringPtr(s []string) *[]string {
	return &s
}
